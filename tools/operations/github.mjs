import { createHash } from 'node:crypto'
import { constants, lstat, open } from 'node:fs/promises'
import path from 'node:path'
import { check, noSymlinks, privateDirectory, protectedRead } from './safety.mjs'
import { python } from './python.mjs'
import { fixtureImages } from '../runtime/smoke-image-bundle.mjs'

const api = 'https://api.github.com/repos/ForceMind/MyAPI'
const limit = 8 * 1024 ** 3
export function officialArtifactURL(value) {
  let url
  try { url = new URL(value) } catch { return false }
  return url.protocol === 'https:' && !url.username && !url.password && (!url.port || url.port === '443') &&
    (url.hostname === 'results-receiver.actions.githubusercontent.com' || /^productionresultssa[0-9]+\.blob\.core\.windows\.net$/.test(url.hostname))
}

export class GitHubClient {
  constructor(tokenFile, { fetcher = fetch } = {}) { this.tokenFile = tokenFile; this.fetcher = fetcher }
  async request(route, { redirect = false } = {}) {
    check(/^\/(?:actions|git)\/[A-Za-z0-9_./?=&%-]+$/.test(route), 'INVALID_GITHUB_ROUTE')
    const token = (await protectedRead(this.tokenFile, { uid: process.getuid() })).toString().trim()
    check(token.length >= 16 && token.length <= 512 && !/[\s\0]/.test(token), 'INVALID_GITHUB_TOKEN')
    const response = await this.fetcher(api + route, { method: 'GET', redirect: 'manual', signal: AbortSignal.timeout(30_000),
      headers: { Authorization: `Bearer ${token}`, Accept: 'application/vnd.github+json', 'X-GitHub-Api-Version': '2022-11-28', 'User-Agent': 'MyAPI-Operations' } })
    if (redirect) { check(response.status === 302, 'GITHUB_ARTIFACT_UNAVAILABLE', 503); return response.headers.get('location') }
    check(response.status === 200 && Number(response.headers.get('content-length') || 0) <= 4 * 1024 * 1024, 'GITHUB_METADATA_FAILED', 503)
    check(response.body, 'GITHUB_METADATA_FAILED', 503)
    const chunks = []; let bytes = 0
    for await (const chunk of response.body) { bytes += chunk.length; check(bytes <= 4 * 1024 * 1024, 'GITHUB_METADATA_FAILED', 503); chunks.push(chunk) }
    const text = Buffer.concat(chunks).toString('utf8')
    try { return JSON.parse(text) } catch { check(false, 'GITHUB_METADATA_FAILED', 503) }
  }
  async verify(artifact) {
    const run = await this.request(`/actions/runs/${artifact.run_id}/attempts/${artifact.producer_attempt}`)
    check(String(run.id) === artifact.run_id && String(run.run_attempt) === artifact.producer_attempt && run.status === 'completed' && run.conclusion === 'success' &&
      run.head_branch === 'dev' && run.head_sha === artifact.source_sha && run.path === '.github/workflows/docker-smoke.yml' &&
      Number.isSafeInteger(run.workflow_id) && run.workflow_id > 0 && Number.isSafeInteger(run.repository?.id) &&
      ['push', 'workflow_dispatch'].includes(run.event) && run.repository?.full_name === 'ForceMind/MyAPI' && run.head_repository?.full_name === 'ForceMind/MyAPI', 'GITHUB_RUN_MISMATCH')
    const workflow = await this.request(`/actions/workflows/${run.workflow_id}`)
    check(workflow.path === '.github/workflows/docker-smoke.yml' && workflow.state === 'active', 'GITHUB_WORKFLOW_MISMATCH')
    const qualification = await this.request(`/actions/workflows/ci.yml/runs?head_sha=${artifact.source_sha}&branch=dev&per_page=100`)
    check(Array.isArray(qualification.workflow_runs), 'GITHUB_QUALIFICATION_FAILED')
    const candidates = qualification.workflow_runs.filter(item => item.head_sha === artifact.source_sha && item.head_branch === 'dev' &&
      item.path === '.github/workflows/ci.yml' && ['push', 'workflow_dispatch'].includes(item.event) &&
      item.repository?.id === run.repository.id && item.repository?.full_name === 'ForceMind/MyAPI' && item.head_repository?.full_name === 'ForceMind/MyAPI' &&
      Number.isSafeInteger(item.id) && item.id > 0).sort((a, b) => b.id - a.id)
    const fullCI = candidates[0]
    check(fullCI?.status === 'completed' && fullCI.conclusion === 'success' && Number.isSafeInteger(fullCI.run_attempt) && fullCI.run_attempt > 0, 'GITHUB_QUALIFICATION_FAILED')
    const commit = await this.request(`/git/commits/${artifact.source_sha}`)
    check(commit.sha === artifact.source_sha && commit.tree?.sha === artifact.tree_sha, 'GITHUB_SOURCE_MISMATCH')
    const tree = await this.request(`/git/trees/${artifact.tree_sha}`)
    check(tree.sha === artifact.tree_sha && tree.truncated === false && Array.isArray(tree.tree), 'GITHUB_SOURCE_MISMATCH')
    const versionEntry = tree.tree.find(entry => entry.path === 'VERSION')
    check(versionEntry?.type === 'blob' && ['100644', '100755'].includes(versionEntry.mode) && /^[a-f0-9]{40}$/.test(versionEntry.sha), 'GITHUB_VERSION_MISMATCH')
    const versionBlob = await this.request(`/git/blobs/${versionEntry.sha}`)
    check(versionBlob.sha === versionEntry.sha && versionBlob.encoding === 'base64' && Number.isSafeInteger(versionBlob.size) && versionBlob.size > 0 && versionBlob.size <= 128 &&
      typeof versionBlob.content === 'string' && versionBlob.content.length <= 256 && Buffer.from(versionBlob.content, 'base64').toString('utf8').trim() === artifact.version, 'GITHUB_VERSION_MISMATCH')
    const item = await this.request(`/actions/artifacts/${artifact.artifact_id}`)
    check(String(item.id) === artifact.artifact_id && !item.expired && item.workflow_run?.id === run.id && item.workflow_run?.head_sha === artifact.source_sha &&
      item.workflow_run?.head_branch === 'dev' && item.workflow_run?.repository_id === run.repository.id && item.workflow_run?.head_repository_id === run.repository.id &&
      item.name === `myapi-smoke-images-${artifact.run_id}-${artifact.producer_attempt}-${artifact.source_sha}-full` &&
      Number.isSafeInteger(item.size_in_bytes) && item.size_in_bytes > 0 && item.size_in_bytes <= limit && /^sha256:[a-f0-9]{64}$/.test(item.digest), 'GITHUB_ARTIFACT_MISMATCH')
    return { zip_sha256: item.digest.slice(7), zip_bytes: item.size_in_bytes, full_ci_run_id: String(fullCI.id), full_ci_attempt: String(fullCI.run_attempt) }
  }
  async download(artifact, verified, directory) {
    const location = await this.request(`/actions/artifacts/${artifact.artifact_id}/zip`, { redirect: true })
    check(officialArtifactURL(location), 'GITHUB_REDIRECT_REJECTED')
    // Never forward the GitHub bearer token to artifact storage.
    const response = await this.fetcher(location, { method: 'GET', redirect: 'manual', signal: AbortSignal.timeout(15 * 60_000) })
    check(response.status === 200 && response.body && Number(response.headers.get('content-length') || 0) <= limit, 'GITHUB_DOWNLOAD_FAILED', 503)
    const file = await open(path.join(directory, 'artifact.zip'), 'wx', 0o600)
    const hash = createHash('sha256'); let bytes = 0
    try {
      for await (const chunk of response.body) { bytes += chunk.length; check(bytes <= limit, 'ARTIFACT_TOO_LARGE'); hash.update(chunk); await file.writeFile(chunk) }
      await file.sync()
    } finally { await file.close() }
    check(bytes === verified.zip_bytes && hash.digest('hex') === verified.zip_sha256, 'ZIP_DIGEST_MISMATCH')
    await python('extract_bundle.py', { directory, max_bytes: limit }, 15 * 60_000)
    return directory
  }
}

export async function verifyBundle(directory, artifact, docker) {
  await privateDirectory(directory)
  const serialized = await protectedRead(path.join(directory, 'manifest.json'), { uid: process.getuid(), max: 16384 })
  check(createHash('sha256').update(serialized).digest('hex') === artifact.manifest_sha256, 'MANIFEST_DIGEST_MISMATCH')
  let manifest
  try { manifest = JSON.parse(serialized) } catch { check(false, 'MANIFEST_MISMATCH') }
  const expected = { schemaVersion: 1, sourceSha: artifact.source_sha, headSha: artifact.source_sha, treeSha: artifact.tree_sha, runId: artifact.run_id,
    producerAttempt: artifact.producer_attempt, edition: 'full', personal: '0', platform: 'linux/amd64' }
  check(Object.keys(manifest).sort().join() === [...Object.keys(expected), 'tar', 'images'].sort().join() && Object.entries(expected).every(([k, v]) => manifest[k] === v), 'MANIFEST_MISMATCH')
  check(manifest.tar && Object.keys(manifest.tar).sort().join() === 'bytes,file,sha256' && manifest.tar.file === 'images.tar' &&
    Number.isSafeInteger(manifest.tar.bytes) && manifest.tar.bytes > 0 && manifest.tar.bytes <= limit && /^[a-f0-9]{64}$/.test(manifest.tar.sha256) &&
    Array.isArray(manifest.images) && manifest.images.length === 2, 'MANIFEST_MISMATCH')
  const roles = ['app', 'bun']
  for (const [index, image] of manifest.images.entries()) {
    const role = roles[index]
    check(image && Object.keys(image).sort().join() === 'id,registryRef,role,tag' && image.role === role && /^sha256:[a-f0-9]{64}$/.test(image.id) &&
      image.registryRef === (role === 'app' ? null : fixtureImages.bun) && image.tag === (role === 'app' ? `myapi:smoke-full-${artifact.source_sha}` :
        `myapi-smoke-fixture:bun-full-${artifact.run_id}-${artifact.producer_attempt}`), 'MANIFEST_MISMATCH')
  }
  check(manifest.images[0].id === artifact.image_id && manifest.images[0].id !== manifest.images[1].id, 'IMAGE_ID_MISMATCH')
  const tarPath = path.join(directory, 'images.tar')
  await noSymlinks(tarPath)
  const before = await lstat(tarPath)
  check(before.isFile() && before.nlink === 1 && before.uid === process.getuid() && (before.mode & 0o777) === 0o600, 'TAR_DIGEST_MISMATCH')
  const tar = await open(tarPath, constants.O_RDONLY | constants.O_NOFOLLOW)
  try {
    const stat = await tar.stat(); check(stat.dev === before.dev && stat.ino === before.ino && stat.isFile() && stat.nlink === 1 && stat.size === manifest.tar.bytes, 'TAR_DIGEST_MISMATCH')
    const hash = createHash('sha256')
    for await (const chunk of tar.createReadStream({ autoClose: false })) hash.update(chunk)
    const after = await tar.stat()
    check(after.size === before.size && after.mtimeMs === before.mtimeMs && after.ctimeMs === before.ctimeMs && hash.digest('hex') === manifest.tar.sha256, 'TAR_DIGEST_MISMATCH')
  } finally { await tar.close() }
  await docker.load(path.join(directory, 'images.tar'))
  for (const image of manifest.images) {
    const info = await docker.inspectImage(image.tag)
    check(info.Id === image.id && info.Os === 'linux' && info.Architecture === 'amd64' && !info.Variant && info.RepoTags?.includes(image.tag), 'IMAGE_ID_MISMATCH')
    if (image.role === 'app') check(Object.entries({ 'io.myapi.smoke.source': artifact.source_sha, 'io.myapi.smoke.head': artifact.source_sha,
      'io.myapi.smoke.tree': artifact.tree_sha, 'io.myapi.edition': 'full' }).every(([k, v]) => info.Config?.Labels?.[k] === v), 'IMAGE_LABEL_MISMATCH')
  }
  const exact = await docker.inspectImage(artifact.image_id)
  check(exact.Id === artifact.image_id, 'IMAGE_ID_MISMATCH')
  return { artifact_identity: true, exact_image: true }
}
