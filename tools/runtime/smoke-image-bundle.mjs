#!/usr/bin/env node

// A same-run CI transport for clean images, never containers, volumes or data.
// The externally supplied manifest SHA is the trust anchor; an archive's own
// checksum alone does not authenticate it. No registry/build/run fallback.
import { createHash } from 'node:crypto'
import { execFile } from 'node:child_process'
import { createReadStream } from 'node:fs'
import { constants, lstat, mkdir, open, readdir, realpath } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

export const fixtureImages = Object.freeze({
  bun: 'oven/bun:1@sha256:0733e50325078969732ebe3b15ce4c4be5082f18c4ac1a0f0ca4839c2e4e42a7',
  redis: 'redis:7-alpine@sha256:858f009f9709ce576febc734aa78b8f6d624b82571f9ddb6bda4377c833b3499',
})

const platform = 'linux/amd64'
const files = ['images.tar', 'manifest.json']
const safeCodes = new Set(['INVALID_OPTIONS', 'DIRECTORY_UNSAFE', 'FILES_INVALID', 'MANIFEST_MISMATCH',
  'TAR_MISMATCH', 'IMAGE_MISMATCH', 'FIXTURE_MISMATCH', 'TAG_COLLISION', 'DOCKER_FAILED', 'ENV_UNSAFE', 'IO_FAILED'])
const sha40 = /^[a-f0-9]{40}$/
const sha64 = /^[a-f0-9]{64}$/
const imageId = /^sha256:[a-f0-9]{64}$/

function requireThat(condition, code) {
  if (!condition) throw new Error(`IMAGE_BUNDLE_${code}`)
}

function safeError(error) {
  const code = String(error?.message || '').replace(/^IMAGE_BUNDLE_/, '')
  return new Error(`IMAGE_BUNDLE_${safeCodes.has(code) ? code : 'IO_FAILED'}`)
}

function identity(options) {
  const { sourceSha, headSha, treeSha, runId, producerAttempt, edition, personal } = options
  requireThat([sourceSha, headSha, treeSha].every(value => typeof value === 'string' && sha40.test(value)) &&
    typeof runId === 'string' && /^[1-9][0-9]{0,19}$/.test(runId) &&
    typeof producerAttempt === 'string' && /^[1-9][0-9]{0,5}$/.test(producerAttempt) &&
    ['full', 'lan'].includes(edition) && ['0', '1'].includes(personal), 'INVALID_OPTIONS')
  return { sourceSha, headSha, treeSha, runId, producerAttempt, edition, personal, platform }
}

function rolesFor(expected) {
  return expected.personal === '1' ? ['app', 'bun', 'redis'] : ['app', 'bun']
}

function tagFor(role, expected) {
  return role === 'app' ? `myapi:smoke-${expected.edition}-${expected.sourceSha}`
    : `myapi-smoke-fixture:${role}-${expected.edition}-${expected.runId}-${expected.producerAttempt}`
}

function appLabels(expected) {
  return { 'io.myapi.smoke.source': expected.sourceSha, 'io.myapi.smoke.head': expected.headSha,
    'io.myapi.smoke.tree': expected.treeSha, 'io.myapi.edition': expected.edition }
}

function exactKeys(value, keys, code) {
  requireThat(value && typeof value === 'object' && !Array.isArray(value) &&
    Object.keys(value).sort().join('\n') === [...keys].sort().join('\n'), code)
}

async function directoryAt(directory, exporting) {
  requireThat(typeof directory === 'string' && path.isAbsolute(directory) && path.resolve(directory) === directory, 'DIRECTORY_UNSAFE')
  if (exporting) {
    // Only create the final component under an existing owned parent.
    requireThat(await realpath(path.dirname(directory)) === path.dirname(directory), 'DIRECTORY_UNSAFE')
    await mkdir(directory, { mode: 0o700 }).catch(error => { if (error.code !== 'EEXIST') throw error })
  }
  const stat = await lstat(directory)
  requireThat(stat.isDirectory() && !stat.isSymbolicLink() && await realpath(directory) === directory &&
    (stat.mode & 0o022) === 0 && (process.getuid === undefined || stat.uid === process.getuid()), 'DIRECTORY_UNSAFE')
  const entries = (await readdir(directory)).sort()
  requireThat(exporting ? entries.length === 0 : entries.join('\n') === files.join('\n'), 'FILES_INVALID')
  return directory
}

async function regularFile(filename, maxBytes, code) {
  const stat = await lstat(filename)
  requireThat(stat.isFile() && !stat.isSymbolicLink() && stat.nlink === 1 &&
    (stat.mode & 0o022) === 0 && (process.getuid === undefined || stat.uid === process.getuid()) &&
    stat.size > 0 && stat.size <= maxBytes && await realpath(filename) === filename, code)
  const handle = await open(filename, constants.O_RDONLY | constants.O_NOFOLLOW)
  const opened = await handle.stat()
  if (opened.dev !== stat.dev || opened.ino !== stat.ino || opened.size !== stat.size) {
    await handle.close()
    throw new Error(`IMAGE_BUNDLE_${code}`)
  }
  return { handle, stat }
}

async function hashFile(filename, maxBytes, code) {
  const { handle, stat } = await regularFile(filename, maxBytes, code)
  try {
    const hash = createHash('sha256')
    for await (const chunk of createReadStream(filename, { fd: handle.fd, autoClose: false, start: 0 })) hash.update(chunk)
    const after = await handle.stat()
    requireThat(after.size === stat.size && after.mtimeMs === stat.mtimeMs && after.ctimeMs === stat.ctimeMs, code)
    return { bytes: stat.size, sha256: hash.digest('hex') }
  } finally { await handle.close() }
}

async function dockerCommand(args) {
  return new Promise((resolve, reject) => {
    execFile('docker', args, { encoding: 'utf8', timeout: 180_000, killSignal: 'SIGKILL', maxBuffer: 1024 * 1024 },
      (error, stdout) => error ? reject(new Error('IMAGE_BUNDLE_DOCKER_FAILED')) : resolve(stdout))
  })
}

async function inspect(runDocker, reference) {
  let value
  try { value = JSON.parse(await runDocker(['image', 'inspect', '--format', '{{json .}}', reference])) }
  catch { throw new Error('IMAGE_BUNDLE_DOCKER_FAILED') }
  requireThat(value && typeof value.Id === 'string' && imageId.test(value.Id) && value.Os === 'linux' && value.Architecture === 'amd64' &&
    (value.Variant === undefined || value.Variant === ''), 'IMAGE_MISMATCH')
  return value
}

function checkImage(info, image, expected) {
  requireThat(info.Id === image.id && Array.isArray(info.RepoTags) && info.RepoTags.includes(image.tag), 'IMAGE_MISMATCH')
  if (image.role === 'app') requireThat(Object.entries(appLabels(expected)).every(([key, value]) => info.Config?.Labels?.[key] === value), 'IMAGE_MISMATCH')
}

async function refuseTagCollision(runDocker, tag, id) {
  const output = await runDocker(['image', 'ls', '--no-trunc', '--quiet', '--filter', `reference=${tag}`])
  const ids = output.trim() ? output.trim().split(/\s+/) : []
  requireThat(ids.length === 0 || ids.length === 1 && ids[0] === id, 'TAG_COLLISION')
  return ids.length === 1
}

/** Export only prebuilt/pulled clean images selected by this identity and mode. */
export async function exportImageBundle(options, { runDocker = dockerCommand } = {}) {
  try {
    const expected = identity(options)
    const roles = rolesFor(expected)
    const directory = await directoryAt(options.directory, true)
    const images = []
    for (const role of roles) {
      const tag = tagFor(role, expected)
      const registryRef = fixtureImages[role] || null
      const info = await inspect(runDocker, registryRef || tag)
      if (registryRef) {
        const digest = registryRef.split('@')[1]
        const repositories = role === 'bun' ? ['oven/bun', 'docker.io/oven/bun'] : ['redis', 'library/redis', 'docker.io/library/redis']
        requireThat(Array.isArray(info.RepoDigests) && repositories.some(repo => info.RepoDigests.includes(`${repo}@${digest}`)), 'FIXTURE_MISMATCH')
        if (!await refuseTagCollision(runDocker, tag, info.Id)) await runDocker(['image', 'tag', info.Id, tag])
      }
      const image = { role, tag, id: info.Id, registryRef }
      checkImage(registryRef ? await inspect(runDocker, tag) : info, image, expected)
      images.push(image)
    }
    requireThat(new Set(images.map(image => image.id)).size === roles.length, 'IMAGE_MISMATCH')
    const tarPath = path.join(directory, 'images.tar')
    await runDocker(['image', 'save', '--platform=linux/amd64', '--output', tarPath, ...images.map(image => image.tag)])
    const tar = { file: 'images.tar', ...await hashFile(tarPath, 8 * 1024 ** 3, 'TAR_MISMATCH') }
    const manifest = { schemaVersion: 1, ...expected, tar, images }
    const serialized = JSON.stringify(manifest, null, 2) + '\n'
    const handle = await open(path.join(directory, 'manifest.json'), 'wx', 0o600)
    try { await handle.writeFile(serialized) } finally { await handle.close() }
    await directoryAt(directory, false)
    return { edition: expected.edition, manifestSha256: createHash('sha256').update(serialized).digest('hex'), tarSha256: tar.sha256 }
  } catch (error) { throw safeError(error) }
}

/** Authenticate metadata before Docker sees the opaque archive; never extract it. */
export async function verifyAndLoadImageBundle(options, { runDocker = dockerCommand } = {}) {
  try {
    const expected = identity(options)
    const roles = rolesFor(expected)
    requireThat(typeof options.manifestSha256 === 'string' && sha64.test(options.manifestSha256), 'INVALID_OPTIONS')
    const directory = await directoryAt(options.directory, false)
    const manifestFile = await regularFile(path.join(directory, 'manifest.json'), 16 * 1024, 'MANIFEST_MISMATCH')
    let serialized
    try { serialized = await manifestFile.handle.readFile() } finally { await manifestFile.handle.close() }
    requireThat(createHash('sha256').update(serialized).digest('hex') === options.manifestSha256, 'MANIFEST_MISMATCH')
    let manifest
    try { manifest = JSON.parse(serialized) } catch { throw new Error('IMAGE_BUNDLE_MANIFEST_MISMATCH') }
    exactKeys(manifest, ['schemaVersion', ...Object.keys(expected), 'tar', 'images'], 'MANIFEST_MISMATCH')
    requireThat(manifest.schemaVersion === 1 && Object.entries(expected).every(([key, value]) => manifest[key] === value), 'MANIFEST_MISMATCH')
    exactKeys(manifest.tar, ['file', 'bytes', 'sha256'], 'MANIFEST_MISMATCH')
    requireThat(manifest.tar.file === 'images.tar' && Number.isSafeInteger(manifest.tar.bytes) &&
      manifest.tar.bytes > 0 && manifest.tar.bytes <= 8 * 1024 ** 3 && typeof manifest.tar.sha256 === 'string' && sha64.test(manifest.tar.sha256) &&
      Array.isArray(manifest.images) && manifest.images.length === roles.length, 'MANIFEST_MISMATCH')
    for (const [index, image] of manifest.images.entries()) {
      exactKeys(image, ['role', 'tag', 'id', 'registryRef'], 'MANIFEST_MISMATCH')
      const role = roles[index]
      requireThat(image.role === role && image.tag === tagFor(role, expected) && typeof image.id === 'string' && imageId.test(image.id) &&
        image.registryRef === (fixtureImages[role] || null), 'MANIFEST_MISMATCH')
    }
    requireThat(new Set(manifest.images.map(image => image.id)).size === roles.length, 'MANIFEST_MISMATCH')
    const tarPath = path.join(directory, 'images.tar')
    const tar = await hashFile(tarPath, 8 * 1024 ** 3, 'TAR_MISMATCH')
    requireThat(tar.bytes === manifest.tar.bytes && tar.sha256 === manifest.tar.sha256, 'TAR_MISMATCH')
    // Check the output destination before changing the local Docker image store.
    const envPath = options.githubEnv
    requireThat(typeof envPath === 'string' && path.isAbsolute(envPath) && path.resolve(envPath) === envPath &&
      !envPath.startsWith(directory + path.sep), 'ENV_UNSAFE')
    const envStat = await lstat(envPath)
    requireThat(envStat.isFile() && !envStat.isSymbolicLink() && envStat.nlink === 1 && (envStat.mode & 0o022) === 0 &&
      (process.getuid === undefined || envStat.uid === process.getuid()) && await realpath(envPath) === envPath, 'ENV_UNSAFE')
    for (const image of manifest.images) await refuseTagCollision(runDocker, image.tag, image.id)
    await runDocker(['image', 'load', '--platform=linux/amd64', '--input', tarPath, '--quiet'])
    for (const image of manifest.images) checkImage(await inspect(runDocker, image.tag), image, expected)
    // Do not publish any usable image reference until every selected role passed.
    const env = await open(envPath, constants.O_WRONLY | constants.O_APPEND | constants.O_NOFOLLOW)
    try {
      const opened = await env.stat()
      requireThat(opened.dev === envStat.dev && opened.ino === envStat.ino && opened.nlink === 1, 'ENV_UNSAFE')
      await env.writeFile(manifest.images.map(image => `SMOKE_${image.role.toUpperCase()}_IMAGE_ID=${image.id}\n`).join(''))
    } finally { await env.close() }
    return { passed: true, ...expected, images: manifest.images.map(({ role, id }) => ({ role, id })) }
  } catch (error) { throw safeError(error) }
}

async function main() {
  try {
    const [action, ...args] = process.argv.slice(2)
    requireThat(['export', 'verify-load'].includes(action), 'INVALID_OPTIONS')
    const names = { directory: 'directory', edition: 'edition', personal: 'personal', 'source-sha': 'sourceSha', 'head-sha': 'headSha',
      'tree-sha': 'treeSha', 'run-id': 'runId', 'producer-attempt': 'producerAttempt',
      ...(action === 'verify-load' ? { 'manifest-sha256': 'manifestSha256', 'github-env': 'githubEnv' } : {}) }
    const options = {}
    for (let index = 0; index < args.length; index += 2) {
      const name = args[index]?.startsWith('--') ? args[index].slice(2) : ''
      const key = Object.hasOwn(names, name) ? names[name] : undefined
      requireThat(key && args[index + 1] && !Object.hasOwn(options, key), 'INVALID_OPTIONS')
      options[key] = args[index + 1]
    }
    requireThat(Object.keys(options).length === Object.keys(names).length, 'INVALID_OPTIONS')
    if (action === 'export') {
      const report = await exportImageBundle(options)
      console.log(`${report.edition}_manifest_sha256=${report.manifestSha256}\n${report.edition}_tar_sha256=${report.tarSha256}`)
    } else console.log(JSON.stringify(await verifyAndLoadImageBundle(options)))
  } catch (error) {
    console.error(JSON.stringify({ command: 'smoke:image-bundle', passed: false, code: safeError(error).message }))
    process.exitCode = 1
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) await main()
