import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { execFileSync } from 'node:child_process'
import { chmodSync, linkSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, symlinkSync, unlinkSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import test from 'node:test'
import { exportImageBundle, fixtureImages, verifyAndLoadImageBundle } from './smoke-image-bundle.mjs'

// These are local driver contracts. The fake archive is JSON, not a Docker tar;
// actual save/load, platform support and image persistence remain CI acceptance.
const expected = { sourceSha: 'a'.repeat(40), headSha: 'b'.repeat(40), treeSha: 'c'.repeat(40),
  runId: '123456789', producerAttempt: '2', edition: 'full', personal: '1' }
const digest = value => createHash('sha256').update(value).digest('hex')

function fixture(edition = 'full', personal = '1') {
  const root = mkdtempSync(path.join(tmpdir(), 'myapi-image-bundle-'))
  const options = { ...expected, edition, personal, directory: path.join(root, 'bundle'), githubEnv: path.join(root, 'github.env') }
  writeFileSync(options.githubEnv, '', { mode: 0o600 })
  const calls = []
  const images = new Map()
  const appTag = `myapi:smoke-${edition}-${expected.sourceSha}`
  for (const [index, role] of ['app', 'bun', 'redis'].entries()) {
    const reference = fixtureImages[role] || appTag
    const info = { Id: `sha256:${String(index + 1).repeat(64)}`, Os: 'linux', Architecture: 'amd64',
      RepoTags: role === 'app' ? [appTag] : [],
      RepoDigests: role === 'app' ? [] : [`${role === 'bun' ? 'oven/bun' : 'redis'}@${reference.split('@')[1]}`],
      Config: { Labels: role === 'app' ? { 'io.myapi.edition': edition, 'io.myapi.smoke.source': expected.sourceSha,
        'io.myapi.smoke.head': expected.headSha, 'io.myapi.smoke.tree': expected.treeSha } : {} } }
    images.set(reference, info)
    images.set(info.Id, info)
  }
  let afterLoad
  const runDocker = async args => {
    calls.push(args)
    assert.equal(args[0], 'image', 'no container/volume operation belongs in a clean image bundle')
    if (args[1] === 'inspect') {
      const info = images.get(args.at(-1))
      if (!info) throw new Error('private Docker diagnostics must not escape')
      return JSON.stringify(info)
    }
    if (args[1] === 'ls') return images.get(args.at(-1).replace('reference=', ''))?.Id || ''
    if (args[1] === 'tag') {
      const info = images.get(args[2])
      assert(info)
      info.RepoTags.push(args[3])
      images.set(args[3], info)
      return ''
    }
    if (args[1] === 'save') {
      assert.equal(args[2], '--platform=linux/amd64')
      assert.equal(args[3], '--output')
      assert.equal(args.length, personal === '1' ? 8 : 7, 'export exactly the selected clean roles, never every local image')
      const saved = args.slice(5).map(tag => ({ tag, info: images.get(tag) }))
      writeFileSync(args[4], JSON.stringify(saved), { mode: 0o600 })
      return ''
    }
    if (args[1] === 'load') {
      assert.deepEqual(args.slice(2, 4), ['--platform=linux/amd64', '--input'])
      assert.equal(args.at(-1), '--quiet')
      for (const { tag, info } of JSON.parse(readFileSync(args[4], 'utf8'))) {
        info.RepoDigests = [] // Docker load need not restore registry digests.
        images.set(tag, info)
        images.set(info.Id, info)
      }
      afterLoad?.(images)
      return ''
    }
    assert.fail('unexpected Docker command')
  }
  return { options, images, appTag, calls, runDocker,
    afterLoad(fn) { afterLoad = fn },
    cleanup() { rmSync(root, { recursive: true, force: true }) }, root }
}

async function exported(f) {
  const report = await exportImageBundle(f.options, f)
  f.options.manifestSha256 = report.manifestSha256
  return report
}

function editManifest(f, edit, authenticate = true) {
  const filename = path.join(f.options.directory, 'manifest.json')
  const manifest = JSON.parse(readFileSync(filename, 'utf8'))
  edit(manifest)
  const bytes = JSON.stringify(manifest)
  writeFileSync(filename, bytes)
  if (authenticate) f.options.manifestSha256 = digest(bytes)
}

for (const [edition, personal] of [['full', '0'], ['full', '1'], ['lan', '0'], ['lan', '1']]) {
  test(`${edition} personal=${personal}: export binds clean pinned fixtures, load verifies IDs without registry digest persistence`, async () => {
    const f = fixture(edition, personal)
    try {
      const receipt = await exported(f)
      assert.match(receipt.manifestSha256, /^[a-f0-9]{64}$/)
      assert.deepEqual(readdirSync(f.options.directory).sort(), ['images.tar', 'manifest.json'])
      const manifest = JSON.parse(readFileSync(path.join(f.options.directory, 'manifest.json')))
      assert.equal(manifest.producerAttempt, '2')
      assert.equal(manifest.personal, personal)
      assert.equal(manifest.images.length, personal === '1' ? 3 : 2)
      assert.equal(manifest.platform, 'linux/amd64')
      assert.equal(manifest.images[0].tag, f.appTag)
      assert.equal(manifest.images[1].registryRef, fixtureImages.bun)
      if (personal === '1') assert.equal(manifest.images[2].registryRef, fixtureImages.redis)
      f.images.clear() // A distinct consumer daemon starts with no saved tags.
      const result = await verifyAndLoadImageBundle(f.options, f)
      assert.equal(result.passed, true)
      assert.equal(f.images.get(f.appTag).Id, manifest.images[0].id)
      assert.deepEqual(f.images.get(f.appTag).RepoDigests, [])
      assert.equal(readFileSync(f.options.githubEnv, 'utf8'), manifest.images.map(image =>
        `SMOKE_${image.role.toUpperCase()}_IMAGE_ID=${image.id}\n`).join(''))
      if (personal === '0') {
        assert(!readFileSync(f.options.githubEnv, 'utf8').includes('REDIS'))
        assert(!f.calls.some(args => args.some(value => value.includes('redis'))))
      }
      assert(f.calls.every(args => !args.some(value => ['pull', 'build', 'run', 'create', 'commit', 'export'].includes(value))))
    } finally { f.cleanup() }
  })
}

for (const [name, change] of [
  ['merge SHA', value => { value.sourceSha = 'main' }], ['head SHA', value => { value.headSha = '' }],
  ['tree SHA', value => { value.treeSha = ['c'.repeat(40)] }], ['run', value => { value.runId = '../123' }],
  ['attempt', value => { value.producerAttempt = '0' }], ['edition', value => { value.edition = 'external' }],
  ['missing personal mode', value => { delete value.personal }], ['personal mode', value => { value.personal = '2' }],
  ['non-string personal mode', value => { value.personal = false }],
]) {
  test(`invalid ${name} makes no Docker call`, async () => {
    const f = fixture()
    try {
      change(f.options)
      await assert.rejects(exportImageBundle(f.options, f), /IMAGE_BUNDLE_INVALID_OPTIONS/)
      await assert.rejects(verifyAndLoadImageBundle({ ...f.options, manifestSha256: '0'.repeat(64) }, f), /IMAGE_BUNDLE_INVALID_OPTIONS/)
      assert.equal(f.calls.length, 0)
    } finally { f.cleanup() }
  })
}

for (const kind of ['tag-only', 'wrong-repository', 'wrong-platform', 'wrong-source-label']) {
  test(`producer rejects ${kind} image evidence before saving`, async () => {
    const f = fixture()
    try {
      if (kind === 'tag-only') f.images.get(fixtureImages.bun).RepoDigests = []
      if (kind === 'wrong-repository') f.images.get(fixtureImages.redis).RepoDigests = [`untrusted/redis@${fixtureImages.redis.split('@')[1]}`]
      if (kind === 'wrong-platform') f.images.get(fixtureImages.bun).Architecture = 'arm64'
      if (kind === 'wrong-source-label') f.images.get(f.appTag).Config.Labels['io.myapi.smoke.source'] = expected.headSha
      await assert.rejects(exportImageBundle(f.options, f), /IMAGE_BUNDLE_(FIXTURE|IMAGE)_MISMATCH/)
      assert(!f.calls.some(args => args[1] === 'save'))
    } finally { f.cleanup() }
  })
}

for (const [name, edit] of [
  ['source', m => { m.sourceSha = 'd'.repeat(40) }], ['head', m => { m.headSha = 'd'.repeat(40) }],
  ['tree', m => { m.treeSha = 'd'.repeat(40) }], ['run', m => { m.runId = '999' }],
  ['attempt', m => { m.producerAttempt = '3' }], ['edition', m => { m.edition = 'lan' }],
  ['personal mode', m => { m.personal = '0' }],
  ['platform', m => { m.platform = 'linux/arm64' }], ['schema', m => { m.schemaVersion = 2 }],
  ['extra metadata', m => { m.credentials = 'must-not-be-consumed' }],
  ['traversal', m => { m.tar.file = '../images.tar' }], ['absolute path', m => { m.tar.file = '/tmp/images.tar' }],
  ['extra image', m => { m.images.push(m.images[0]) }], ['duplicate identity', m => { m.images[1].id = m.images[0].id }],
  ['non-string identity', m => { m.images[0].id = [m.images[0].id] }],
  ['tag', m => { m.images[0].tag = 'myapi:latest' }], ['digest', m => { m.images[2].registryRef = 'redis:7-alpine' }],
]) {
  test(`consumer rejects authenticated manifest with wrong ${name} before Docker load`, async () => {
    const f = fixture()
    try {
      await exported(f)
      editManifest(f, edit)
      const count = f.calls.length
      await assert.rejects(verifyAndLoadImageBundle(f.options, f), /IMAGE_BUNDLE_MANIFEST_MISMATCH/)
      assert.equal(f.calls.length, count)
      assert.equal(readFileSync(f.options.githubEnv, 'utf8'), '')
    } finally { f.cleanup() }
  })
}

test('external manifest SHA and internal tar SHA are both mandatory before loading', async () => {
  for (const damage of ['empty-sha', 'wrong-external-sha', 'manifest', 'tar', 'same-size-tar']) {
    const f = fixture()
    try {
      await exported(f)
      if (damage === 'empty-sha') f.options.manifestSha256 = ''
      if (damage === 'wrong-external-sha') f.options.manifestSha256 = '0'.repeat(64)
      if (damage === 'manifest') editManifest(f, m => { m.runId = '999' }, false)
      if (damage === 'tar') writeFileSync(path.join(f.options.directory, 'images.tar'), 'tampered archive')
      if (damage === 'same-size-tar') {
        const filename = path.join(f.options.directory, 'images.tar')
        const bytes = readFileSync(filename); bytes[0] ^= 1; writeFileSync(filename, bytes)
      }
      const count = f.calls.length
      await assert.rejects(verifyAndLoadImageBundle(f.options, f), /IMAGE_BUNDLE_(INVALID_OPTIONS|MANIFEST_MISMATCH|TAR_MISMATCH)/)
      assert.equal(f.calls.length, count)
    } finally { f.cleanup() }
  }
})

for (const personal of ['0', '1']) {
  test(`personal=${personal} rejects an authenticated manifest with an inconsistent Redis role`, async () => {
    const f = fixture('full', personal)
    try {
      await exported(f)
      editManifest(f, manifest => {
        if (personal === '1') manifest.images.pop()
        else manifest.images.push({ role: 'redis', id: `sha256:${'3'.repeat(64)}`,
          tag: `myapi-smoke-fixture:redis-full-${expected.runId}-${expected.producerAttempt}`, registryRef: fixtureImages.redis })
      })
      const count = f.calls.length
      await assert.rejects(verifyAndLoadImageBundle(f.options, f), /IMAGE_BUNDLE_MANIFEST_MISMATCH/)
      assert.equal(f.calls.length, count)
      assert.equal(readFileSync(f.options.githubEnv, 'utf8'), '')
    } finally { f.cleanup() }
  })
}

test('producer refuses to overwrite a colliding run-scoped fixture tag', async () => {
  const f = fixture()
  try {
    const alias = `myapi-smoke-fixture:bun-full-${expected.runId}-${expected.producerAttempt}`
    f.images.set(alias, { Id: `sha256:${'9'.repeat(64)}` })
    await assert.rejects(exportImageBundle(f.options, f), /IMAGE_BUNDLE_TAG_COLLISION/)
    assert(!f.calls.some(args => args[1] === 'tag' || args[1] === 'save'))
    assert.equal(f.images.get(alias).Id, `sha256:${'9'.repeat(64)}`)
  } finally { f.cleanup() }
})

for (const kind of ['directory-symlink', 'manifest-symlink', 'tar-symlink', 'tar-hardlink', 'extra-file', 'world-writable']) {
  test(`consumer rejects ${kind} without exposing an archive to Docker`, async () => {
    const f = fixture()
    try {
      await exported(f)
      if (kind === 'directory-symlink') {
        const alias = path.join(f.root, 'alias'); symlinkSync(f.options.directory, alias); f.options.directory = alias
      } else if (kind === 'manifest-symlink' || kind === 'tar-symlink') {
        const filename = path.join(f.options.directory, kind.startsWith('manifest') ? 'manifest.json' : 'images.tar')
        const outside = path.join(f.root, 'outside')
        writeFileSync(outside, readFileSync(filename)); unlinkSync(filename); symlinkSync(outside, filename)
      } else if (kind === 'tar-hardlink') linkSync(path.join(f.options.directory, 'images.tar'), path.join(f.root, 'linked'))
      else if (kind === 'extra-file') writeFileSync(path.join(f.options.directory, 'runtime.db'), 'must not be bundled')
      else chmodSync(f.options.directory, 0o777)
      const count = f.calls.length
      await assert.rejects(verifyAndLoadImageBundle(f.options, f), /IMAGE_BUNDLE_(DIRECTORY_UNSAFE|FILES_INVALID|MANIFEST_MISMATCH|TAR_MISMATCH)/)
      assert.equal(f.calls.length, count)
    } finally { f.cleanup() }
  })
}

test('export refuses dirty reuse and a symlinked parent before creating a bundle', async () => {
  const f = fixture()
  try {
    mkdirSync(f.options.directory)
    writeFileSync(path.join(f.options.directory, 'existing.tar'), 'old data')
    await assert.rejects(exportImageBundle(f.options, f), /IMAGE_BUNDLE_FILES_INVALID/)
    const alias = path.join(f.root, 'parent-alias')
    symlinkSync(f.options.directory, alias)
    await assert.rejects(exportImageBundle({ ...f.options, directory: path.join(alias, 'new-bundle') }, f), /IMAGE_BUNDLE_DIRECTORY_UNSAFE/)
    assert.deepEqual(readdirSync(f.options.directory), ['existing.tar'])
    assert.equal(f.calls.length, 0)
  } finally { f.cleanup() }
})

for (const changed of ['id', 'tag', 'platform', 'labels']) {
  test(`post-load ${changed} mismatch cannot publish any consumer image reference`, async () => {
    const f = fixture()
    try {
      await exported(f)
      f.images.clear()
      f.afterLoad(images => {
        const app = images.get(f.appTag)
        if (changed === 'id') app.Id = `sha256:${'9'.repeat(64)}`
        if (changed === 'tag') app.RepoTags = []
        if (changed === 'platform') app.Architecture = 'arm64'
        if (changed === 'labels') app.Config.Labels['io.myapi.smoke.tree'] = expected.headSha
      })
      await assert.rejects(verifyAndLoadImageBundle(f.options, f), /IMAGE_BUNDLE_IMAGE_MISMATCH/)
      assert.equal(readFileSync(f.options.githubEnv, 'utf8'), '')
    } finally { f.cleanup() }
  })
}

test('preexisting foreign app tag and unsafe environment output fail before loading', async () => {
  for (const kind of ['collision', 'env-symlink', 'env-in-bundle']) {
    const f = fixture()
    try {
      await exported(f)
      if (kind === 'collision') f.images.set(f.appTag, { Id: `sha256:${'9'.repeat(64)}` })
      if (kind === 'env-symlink') {
        const target = path.join(f.root, 'private-env'); writeFileSync(target, 'untouched')
        unlinkSync(f.options.githubEnv); symlinkSync(target, f.options.githubEnv)
      }
      if (kind === 'env-in-bundle') f.options.githubEnv = path.join(f.options.directory, 'manifest.json')
      await assert.rejects(verifyAndLoadImageBundle(f.options, f), /IMAGE_BUNDLE_(TAG_COLLISION|ENV_UNSAFE)/)
      assert(!f.calls.some(args => args[1] === 'load'))
    } finally { f.cleanup() }
  }
})

test('failure of the final Redis inspection leaves all consumer references unpublished', async () => {
  const f = fixture()
  try {
    await exported(f)
    f.images.clear()
    f.afterLoad(images => {
      const redis = [...images.values()].find(info => info.Id === `sha256:${'3'.repeat(64)}`)
      redis.Architecture = 'arm64'
    })
    await assert.rejects(verifyAndLoadImageBundle(f.options, f), /IMAGE_BUNDLE_IMAGE_MISMATCH/)
    assert.equal(readFileSync(f.options.githubEnv, 'utf8'), '')
  } finally { f.cleanup() }
})

for (const command of ['save', 'load']) {
  test(`failed Docker ${command} cannot publish a usable receipt or environment and hides diagnostics`, async () => {
    const f = fixture()
    try {
      if (command === 'load') await exported(f)
      const runDocker = args => {
        if (args[1] === command) throw new Error('private-Docker-error-body')
        return f.runDocker(args)
      }
      await assert.rejects((command === 'save' ? exportImageBundle : verifyAndLoadImageBundle)(f.options, { runDocker }), error => {
        assert.match(error.message, /^IMAGE_BUNDLE_/)
        assert(!error.stack.includes('private-Docker-error-body'))
        return true
      })
      assert.equal(readFileSync(f.options.githubEnv, 'utf8'), '')
      if (command === 'save') assert(!readdirSync(f.options.directory).includes('manifest.json'))
    } finally { f.cleanup() }
  })
}

test('Docker failures and CLI misuse return only fixed safe errors', async () => {
  const f = fixture()
  try {
    await assert.rejects(exportImageBundle(f.options, { runDocker: async () => { throw new Error('private-env-token') } }), error => {
      assert.match(error.message, /^IMAGE_BUNDLE_/)
      assert(!error.stack.includes('private-env-token'))
      assert.equal(error.cause, undefined)
      return true
    })
    let stderr
    try { execFileSync(process.execPath, [new URL('./smoke-image-bundle.mjs', import.meta.url).pathname, 'verify-load', '--secret', 'private-env-token'], { stdio: 'pipe' }) }
    catch (error) { stderr = error.stderr.toString() }
    assert.deepEqual(JSON.parse(stderr), { command: 'smoke:image-bundle', passed: false, code: 'IMAGE_BUNDLE_INVALID_OPTIONS' })
  } finally { f.cleanup() }
})
