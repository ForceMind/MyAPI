import test from 'node:test'
import assert from 'node:assert/strict'
import { mkdtemp, writeFile, rm } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import { verifyBundle, officialArtifactURL } from './github.mjs'
import { digest } from './safety.mjs'
import { fixtureImages } from '../runtime/smoke-image-bundle.mjs'

async function fixture(t) {
  const directory = await mkdtemp(path.join(os.tmpdir(), 'myapi-bundle-'))
  t.after(() => rm(directory, { recursive: true, force: true }))
  const artifact = { source_sha: 'a'.repeat(40), tree_sha: 'b'.repeat(40), run_id: '123', producer_attempt: '1', artifact_id: '456', image_id: 'sha256:' + 'c'.repeat(64), version: '0.2.0-beta.35' }
  const tar = Buffer.from('opaque archive authenticated before Docker receives it')
  const manifest = { schemaVersion: 1, sourceSha: artifact.source_sha, headSha: artifact.source_sha, treeSha: artifact.tree_sha, runId: '123', producerAttempt: '1', edition: 'full', personal: '0', platform: 'linux/amd64',
    tar: { file: 'images.tar', bytes: tar.length, sha256: digest(tar) }, images: [
      { role: 'app', tag: `myapi:smoke-full-${artifact.source_sha}`, id: artifact.image_id, registryRef: null },
      { role: 'bun', tag: 'myapi-smoke-fixture:bun-full-123-1', id: 'sha256:' + 'd'.repeat(64), registryRef: fixtureImages.bun },
    ] }
  async function save() { const serialized = JSON.stringify(manifest); artifact.manifest_sha256 = digest(serialized); await writeFile(path.join(directory, 'manifest.json'), serialized, { mode: 0o600 }) }
  await save(); await writeFile(path.join(directory, 'images.tar'), tar, { mode: 0o600 })
  const calls = []
  let wrongId = false
  const docker = { async load(file) { calls.push(['load', file]) }, async inspectImage(reference) {
    calls.push(['inspect', reference]); const image = manifest.images.find(item => item.tag === reference || item.id === reference)
    return { Id: wrongId ? 'sha256:' + 'f'.repeat(64) : image.id, Os: 'linux', Architecture: 'amd64', RepoTags: [image.tag], Config: { Labels: { 'io.myapi.smoke.source': artifact.source_sha, 'io.myapi.smoke.head': artifact.source_sha, 'io.myapi.smoke.tree': artifact.tree_sha, 'io.myapi.edition': 'full' } } }
  } }
  return { directory, artifact, manifest, docker, calls, save, wrongId: () => { wrongId = true } }
}

test('bundle authenticates every schema identity and archive hash before load, then requires exact loaded IDs', async t => {
  const f = await fixture(t)
  assert.deepEqual(await verifyBundle(f.directory, f.artifact, f.docker), { artifact_identity: true, exact_image: true })
  assert.equal(f.calls[0][0], 'load')
  const mismatch = await fixture(t); mismatch.wrongId()
  await assert.rejects(verifyBundle(mismatch.directory, mismatch.artifact, mismatch.docker), { code: 'IMAGE_ID_MISMATCH' })
})

test('wrong manifest app ID, tampered tar, personal mode and source mismatch never reach load', async t => {
  for (const mode of ['image', 'tar', 'personal', 'source']) {
    const f = await fixture(t)
    if (mode === 'image') f.artifact.image_id = 'sha256:' + '9'.repeat(64)
    if (mode === 'tar') await writeFile(path.join(f.directory, 'images.tar'), 'tampered')
    if (mode === 'personal') { f.manifest.personal = '1'; await f.save() }
    if (mode === 'source') { f.manifest.headSha = '9'.repeat(40); await f.save() }
    await assert.rejects(verifyBundle(f.directory, f.artifact, f.docker))
    assert.equal(f.calls.length, 0)
  }
})

test('artifact redirect allowlist rejects arbitrary hosts, credentials and lookalikes', () => {
  assert.equal(officialArtifactURL('https://productionresultssa1.blob.core.windows.net/path?sig=secret'), true)
  assert.equal(officialArtifactURL('https://results-receiver.actions.githubusercontent.com/path'), true)
  for (const value of ['https://evil.blob.core.windows.net/path', 'https://api.github.com/path', 'https://productionresultssa1.blob.core.windows.net.evil.example/path', 'http://productionresultssa1.blob.core.windows.net/path', 'https://token@productionresultssa1.blob.core.windows.net/path', 'https://productionresultssa1.blob.core.windows.net:444/path']) assert.equal(officialArtifactURL(value), false)
})

test('GitHub metadata binds fixed workflow/repository/dev/attempt/tree/artifact digest; bearer never follows storage redirect', async t => {
  const { GitHubClient } = await import('./github.mjs')
  const directory = await mkdtemp(path.join(os.tmpdir(), 'myapi-github-')); t.after(() => rm(directory, { recursive: true, force: true }))
  const tokenFile = path.join(directory, 'github.token'); await writeFile(tokenFile, 'github-read-only-fixture-token', { mode: 0o600 })
  const artifact = { run_id: '123', producer_attempt: '2', artifact_id: '456', source_sha: 'a'.repeat(40), tree_sha: 'b'.repeat(40), version: '0.2.0-beta.35' }
  let branch = 'dev', storageToken, redirect = 'https://productionresultssa1.blob.core.windows.net/archive?sig=fixture'
  const bytes = Buffer.from('not a zip; digest mismatch prevents extraction')
  const responses = () => ({
    '/actions/runs/123/attempts/2': { id: 123, run_attempt: 2, status: 'completed', conclusion: 'success', head_branch: branch, head_sha: artifact.source_sha, path: '.github/workflows/docker-smoke.yml', event: 'workflow_dispatch', workflow_id: 99, repository: { id: 8, full_name: 'ForceMind/MyAPI' }, head_repository: { full_name: 'ForceMind/MyAPI' } },
    '/actions/workflows/99': { path: '.github/workflows/docker-smoke.yml', state: 'active' },
    ['/git/commits/' + artifact.source_sha]: { sha: artifact.source_sha, tree: { sha: artifact.tree_sha } },
    ['/git/trees/' + artifact.tree_sha]: { sha: artifact.tree_sha, truncated: false, tree: [{ path: 'VERSION', type: 'blob', mode: '100644', sha: 'c'.repeat(40) }] },
    ['/git/blobs/' + 'c'.repeat(40)]: { sha: 'c'.repeat(40), encoding: 'base64', size: artifact.version.length + 1, content: Buffer.from(artifact.version + '\n').toString('base64') },
    '/actions/artifacts/456': { id: 456, expired: false, name: `myapi-smoke-images-123-2-${artifact.source_sha}-full`, size_in_bytes: bytes.length, digest: 'sha256:' + 'f'.repeat(64), workflow_run: { id: 123, head_sha: artifact.source_sha, head_branch: 'dev', repository_id: 8, head_repository_id: 8 } },
  })
  const client = new GitHubClient(tokenFile, { async fetcher(url, options) {
    if (url.startsWith('https://api.github.com/repos/ForceMind/MyAPI')) {
      assert.equal(options.headers.Authorization, 'Bearer github-read-only-fixture-token')
      const route = url.slice('https://api.github.com/repos/ForceMind/MyAPI'.length)
      if (route.endsWith('/zip')) return new Response(null, { status: 302, headers: { location: redirect } })
      return new Response(JSON.stringify(responses()[route]), { status: 200 })
    }
    storageToken = options.headers?.Authorization
    return new Response(bytes, { status: 200 })
  } })
  const verified = await client.verify(artifact); assert.equal(verified.zip_sha256, 'f'.repeat(64))
  branch = 'main'; await assert.rejects(client.verify(artifact), { code: 'GITHUB_RUN_MISMATCH' }); branch = 'dev'
  redirect = 'https://evil.example/archive'; await assert.rejects(client.download(artifact, verified, directory), { code: 'GITHUB_REDIRECT_REJECTED' })
  redirect = 'https://productionresultssa1.blob.core.windows.net/archive?sig=fixture'
  await assert.rejects(client.download(artifact, verified, directory), { code: 'ZIP_DIGEST_MISMATCH' }); assert.equal(storageToken, undefined)
})
