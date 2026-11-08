import { spawnSync } from 'node:child_process'
import { createHash, randomBytes } from 'node:crypto'
import { existsSync, lstatSync, mkdirSync, readFileSync, readdirSync, realpathSync, rmSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'
import { fileURLToPath } from 'node:url'
import { probeFreshSQLite } from './docker-smoke.mjs'
import { verifySQLiteRestore } from './sqlite-restore-smoke.mjs'

function demand(condition, code) {
  if (!condition) throw new Error(code)
}

export function hardeningScope(env) {
  demand(env.GITHUB_ACTIONS === 'true' && env.GITHUB_EVENT_NAME === 'workflow_dispatch' && env.GITHUB_REF === 'refs/heads/dev' &&
    env.MYAPI_B6_ASSESSMENT === '1' && env.MYAPI_SMOKE_API_ONLY === '1' && env.MYAPI_SMOKE_BROWSER === '0' && env.MYAPI_SMOKE_PERSONAL === '0', 'B6_SCOPE_REJECTED')
  demand(/^[a-f0-9]{40}$/.test(env.GITHUB_SHA || '') && /^[1-9][0-9]{0,19}$/.test(env.GITHUB_RUN_ID || '') && /^[1-9][0-9]{0,19}$/.test(env.GITHUB_RUN_ATTEMPT || ''), 'B6_IDENTITY_REJECTED')
  demand(typeof env.RUNNER_TEMP === 'string' && path.isAbsolute(env.RUNNER_TEMP) && path.normalize(env.RUNNER_TEMP) === env.RUNNER_TEMP &&
    env.RUNNER_TEMP !== '/' && !/[,\u0000-\u001f\u007f-\u009f]/u.test(env.RUNNER_TEMP), 'B6_PATH_REJECTED')
  demand(/^sha256:[a-f0-9]{64}$/.test(env.SMOKE_APP_IMAGE_ID || '') && /^sha256:[a-f0-9]{64}$/.test(env.SMOKE_BUN_IMAGE_ID || '') && env.SMOKE_APP_IMAGE_ID !== env.SMOKE_BUN_IMAGE_ID, 'B6_IMAGE_REJECTED')
  const suffix = `${env.GITHUB_SHA}-${env.GITHUB_RUN_ID}-${env.GITHUB_RUN_ATTEMPT}`
  let archiveBytes = null
  if (env.SMOKE_BUNDLE_TAR_BYTES !== undefined) {
    archiveBytes = Number(env.SMOKE_BUNDLE_TAR_BYTES)
    demand(/^[1-9][0-9]*$/.test(env.SMOKE_BUNDLE_TAR_BYTES) && Number.isSafeInteger(archiveBytes), 'B6_ARCHIVE_METADATA_REJECTED')
  }
  return {
    sha: env.GITHUB_SHA, run: `${env.GITHUB_RUN_ID}-${env.GITHUB_RUN_ATTEMPT}`,
    root: path.join(env.RUNNER_TEMP, `myapi-b6-${suffix}`), network: `myapi-b6-network-${suffix}`,
    names: Object.fromEntries(['source', 'fake', 'candidate', 'recovery', 'reader', 'fs'].map(role => [role, `myapi-b6-${role}-${suffix}`])),
    appImage: env.SMOKE_APP_IMAGE_ID, bunImage: env.SMOKE_BUN_IMAGE_ID, archiveBytes,
  }
}

export function assessmentTransportTarget(value) {
  const url = new URL(value)
  demand(url.protocol === 'http:' && url.hostname === '127.0.0.1' && !url.username && !url.password && !url.hash &&
    ['18080', '18081', '19090'].includes(url.port) && /^\/(api|v1|__smoke__)\//.test(url.pathname), 'B6_TRANSPORT_SCOPE_REJECTED')
  const role = url.port === '18081' ? 'reader' : 'fake'
  url.port = url.port === '19090' ? '19090' : '3000'
  return { role, url: url.href }
}

const requestProgram = `
try {
  const input = JSON.parse(await Bun.stdin.text());
  const url = new URL(input.url);
  if (url.protocol !== 'http:' || url.hostname !== '127.0.0.1' || !['3000','19090'].includes(url.port) || url.username || url.password || url.hash || !/^\\/(api|v1|__smoke__)\\//.test(url.pathname)) process.exit(1);
  const response = await fetch(url, {method: input.method, headers: input.headers, body: input.body, redirect: 'error', signal: AbortSignal.timeout(10000)});
  const body = Buffer.from(await response.arrayBuffer());
  if (body.length > 2 * 1024 * 1024) process.exit(1);
  const headers = [...response.headers].filter(([key]) => key.toLowerCase() !== 'set-cookie');
  for (const cookie of response.headers.getSetCookie()) headers.push(['set-cookie', cookie]);
  console.log(JSON.stringify({status: response.status, headers, body: body.toString('base64')}));
} catch { process.exit(1); }
`

export function validateRootManifest(entries, { dataMode = 0o755 } = {}) {
  demand(dataMode === 0o755 || dataMode === 0o700, 'B6_DATA_LAYOUT_REJECTED')
  demand(Array.isArray(entries) && entries.length > 3 && entries.length < 10000, 'B6_MANIFEST_INVALID')
  const paths = new Set()
  for (const row of entries) {
    demand(row && typeof row === 'object' && !Array.isArray(row) && Object.keys(row).sort().join(',') === 'gid,mode,path,sha256,size,type,uid', 'B6_MANIFEST_INVALID')
    demand(typeof row.path === 'string' && (row.path === '.' || /^(data|logs|config)(\/|$)/.test(row.path) && !row.path.includes('\\') &&
      !/[\u0000-\u001f\u007f]/u.test(row.path) && row.path.split('/').every(segment => segment && segment !== '.' && segment !== '..')) && !paths.has(row.path), 'B6_MANIFEST_INVALID')
    paths.add(row.path)
    demand(row.uid === 0 && row.gid === 0 && Number.isInteger(row.mode) && row.mode >= 0 && row.mode <= 0o7777, 'B6_ROOT_OWNERSHIP_MISSING')
    demand(row.type === 'directory' ? row.size === null && row.sha256 === null :
      row.type === 'file' && Number.isSafeInteger(row.size) && row.size >= 0 && /^[a-f0-9]{64}$/.test(row.sha256), 'B6_MANIFEST_INVALID')
  }
  for (const name of ['.', 'data', 'logs', 'config']) {
    demand(entries.some(row => row.path === name && row.type === 'directory' && row.mode === (name === 'data' ? dataMode : 0o700)), 'B6_ROOT_ONLY_DIRECTORY_MISSING')
  }
  demand(entries.some(row => row.path === 'data/my-api.db' && row.type === 'file' && row.size > 16), 'B6_DATABASE_MISSING')
  demand(entries.some(row => row.path === 'config/my-api.env' && row.type === 'file' && row.mode === 0o600 && row.size > 0), 'B6_CONFIGURATION_MISSING')
  demand(entries.some(row => row.path.startsWith('logs/') && row.type === 'file'), 'B6_LOG_EVIDENCE_MISSING')
  return [...entries].sort((left, right) => left.path.localeCompare(right.path))
}

function manifestDigest(entries) {
  return createHash('sha256').update(JSON.stringify(entries)).digest('hex')
}

// The CLI supplies callbacks that inspect the exact owned containers. Unit
// contracts exercise the evidence transitions without Docker or a database.
export async function assessPreservedClone({ original, dataMode = 0o755, rejectionStage = 'application', snapshot, upstreamCount, startNonroot, outcome, removeNonroot, startRoot, verifyRoot, stopRoot }) {
  demand([snapshot, upstreamCount, startNonroot, outcome, removeNonroot, startRoot, verifyRoot, stopRoot].every(callback => typeof callback === 'function'), 'B6_EVIDENCE_CALLBACK_MISSING')
  demand(dataMode === 0o755 && rejectionStage === 'application' || dataMode === 0o700 && rejectionStage === 'oci_workdir', 'B6_DATA_LAYOUT_REJECTED')
  const before = validateRootManifest(original, { dataMode })
  demand(JSON.stringify(validateRootManifest(await snapshot(), { dataMode })) === JSON.stringify(before), 'B6_CLONE_NOT_PRESERVED')
  demand(await upstreamCount() === 2, 'B6_INITIAL_UPSTREAM_EVIDENCE_MISSING')
  await startNonroot()
  const rejected = await outcome()
  if (rejectionStage === 'application') {
    demand(rejected?.status === 'exited' && rejected.running === false && rejected.exitCode === 1 && rejected.oomKilled === false &&
      rejected.uid === 10001 && rejected.gid === 10001 && rejected.processStarted === true && rejected.startExitCode === 0 &&
      rejected.reason === 'permission-denied-startup', 'B6_NONROOT_OUTCOME_UNVERIFIED')
  } else {
    demand(rejected && ['created', 'exited'].includes(rejected.status) && rejected.running === false && rejected.pid === 0 &&
      rejected.oomKilled === false && rejected.uid === null && rejected.gid === null && rejected.processStarted === false &&
      Number.isInteger(rejected.exitCode) && rejected.exitCode >= 0 && Number.isInteger(rejected.startExitCode) && rejected.startExitCode > 0 &&
      rejected.reason === 'oci-workdir-permission-denied', 'B6_OCI_REFUSAL_UNVERIFIED')
  }
  const afterRefusal = validateRootManifest(await snapshot(), { dataMode })
  demand(JSON.stringify(afterRefusal) === JSON.stringify(before), 'B6_REFUSAL_CHANGED_FIXTURE')
  await removeNonroot()
  await startRoot()
  await verifyRoot()
  const stopped = await stopRoot()
  demand(stopped?.status === 'exited' && stopped.running === false && stopped.exitCode === 0 && stopped.oomKilled === false, 'B6_STOP_UNCONFIRMED')
  demand(await upstreamCount() === 2, 'B6_UPSTREAM_REPLAY_DETECTED')
  return {
    assessment_completed: true, compatibility: false, default_user_changed: false, default_user_change_authorized: false,
    verdict: 'nonroot-default-not-safe-for-tested-root-volume',
    tested_data_mode: dataMode.toString(8).padStart(4, '0'), rejection_stage: rejectionStage, process_started: rejected.processStarted,
    nonroot_process_uid: rejected.uid, nonroot_process_gid: rejected.gid,
    nonroot_container_exit_code: rejected.exitCode, docker_start_exit_code: rejected.startExitCode,
    nonroot_uid_evidence: rejectionStage === 'application'
      ? 'Observed fixed UID/GID marker immediately before same-PID exec of the unchanged, non-setuid application.'
      : 'OCI rejected the /data working directory before the marker or application exec; process UID was not observed.',
    refusal_bytes_owner_mode_preserved: true, original_root_business_state_verified: true,
    fake_upstream_requests: { before_nonroot: 2, after_root_recovery: 2, additional: 0 },
    preservation_phases: { refusal: 'Complete clone bytes, UID, GID and mode unchanged.', root_recovery: 'Exact business-state observation; login/session writes may occur.', stopped_source: 'Frozen source manifest unchanged after confirmed stop.' },
    old_volume_manifest: before, refusal_manifest_sha256: manifestDigest(afterRefusal),
    immutable_old_volume_manifest_sha256: manifestDigest(before),
    meaning: 'The expected refusal and recovery were verified. This is not permission to switch the image USER.',
  }
}

// Fixed CI-only fixture operations. No deployment path is accepted. The copy
// preserves metadata with cp -a; it never repairs a migration candidate.
const filesystemProgram = `
import { lstatSync, readdirSync, readFileSync, mkdirSync, writeFileSync, existsSync, rmSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { spawnSync } from 'node:child_process';
const root = '/assessment';
const action = process.env.MYAPI_B6_FS_ACTION;
if (action === 'initialize') {
  const dataMode = process.env.MYAPI_B6_DATA_MODE;
  if (!['0755','0700'].includes(dataMode)) throw Error('invalid layout');
  if (existsSync(root + '/source') || existsSync(root + '/clone')) throw Error('existing fixture');
  mkdirSync(root + '/source', { mode: 0o700 });
  for (const name of ['data','logs','config']) mkdirSync(root + '/source/' + name, { mode: name === 'data' && dataMode === '0755' ? 0o755 : 0o700 });
  writeFileSync(root + '/source/config/my-api.env', process.env.MYAPI_B6_CONFIG, { flag: 'wx', mode: 0o600 });
  console.log(JSON.stringify({ initialized: true }));
} else if (action === 'clone') {
  if (existsSync(root + '/clone')) throw Error('existing clone');
  const result = spawnSync('/bin/cp', ['-a', root + '/source', root + '/clone'], { shell: false });
  if (result.error || result.status !== 0) throw Error('copy failed');
  console.log(JSON.stringify({ copied: true }));
} else if (action === 'snapshot-source' || action === 'snapshot-clone') {
  const base = root + (action === 'snapshot-source' ? '/source' : '/clone');
  const header = readFileSync(base + '/data/my-api.db').subarray(0,16).toString();
  if (header !== 'SQLite format 3\\0') throw Error('not SQLite');
  const entries = [];
  function walk(relative) {
    const absolute = base + '/' + relative;
    const stat = lstatSync(absolute);
    if (stat.isSymbolicLink() || (!stat.isDirectory() && !stat.isFile()) || (stat.isFile() && stat.nlink !== 1)) throw Error('unsafe fixture');
    entries.push({ path: relative, type: stat.isDirectory() ? 'directory' : 'file', uid: stat.uid, gid: stat.gid,
      mode: stat.mode & 0o7777, size: stat.isFile() ? stat.size : null,
      sha256: stat.isFile() ? createHash('sha256').update(readFileSync(absolute)).digest('hex') : null });
    if (stat.isDirectory()) for (const name of readdirSync(absolute).sort()) walk(relative === '.' ? name : relative + '/' + name);
  }
  walk('.');
  console.log(JSON.stringify(entries));
} else if (action === 'cleanup') {
  for (const name of ['source','clone']) rmSync(root + '/' + name, { recursive: true, force: true });
  console.log(JSON.stringify({ cleaned: true }));
} else throw Error('unknown action');
`

const imageFormat = '{"id":{{json .Id}},"os":{{json .Os}},"arch":{{json .Architecture}},"user":{{json .Config.User}},"labels":{{json .Config.Labels}},"size":{{json .Size}},"layers":{{json .RootFS.Layers}}}'
const containerFormat = '{"id":{{json .Id}},"image":{{json .Image}},"user":{{json .Config.User}},"cwd":{{json .Config.WorkingDir}},"labels":{{json .Config.Labels}},"status":{{json .State.Status}},"running":{{json .State.Running}},"pid":{{json .State.Pid}},"runtimeError":{{json .State.Error}},"exitCode":{{json .State.ExitCode}},"oomKilled":{{json .State.OOMKilled}},"mounts":{{json .Mounts}},"network":{{json .HostConfig.NetworkMode}},"ports":{{json .HostConfig.PortBindings}}}'
const networkFormat = '{"id":{{json .Id}},"name":{{json .Name}},"labels":{{json .Labels}},"internal":{{json .Internal}}}'

function docker(args, { env = process.env, timeout = 60000 } = {}) {
  const result = spawnSync('docker', args, { env, shell: false, encoding: 'utf8', timeout, maxBuffer: 4 * 1024 * 1024 })
  demand(!result.error && !result.signal && result.status === 0, 'B6_DOCKER_COMMAND_FAILED')
  return result.stdout.trim()
}

async function runLayout(env, testedDataMode) {
  const base = hardeningScope(env)
  const scope = { ...base, root: `${base.root}-${testedDataMode}`, network: `${base.network}-${testedDataMode}`,
    names: Object.fromEntries(Object.entries(base.names).map(([role, name]) => [role, `${name}-${testedDataMode}`])) }
  const dataMode = testedDataMode === '0755' ? 0o755 : 0o700
  const repository = fileURLToPath(new URL('../../', import.meta.url))
  const head = spawnSync('git', ['-c', 'core.fileMode=false', '-C', repository, 'rev-parse', 'HEAD'], { encoding: 'utf8' })
  demand(!head.error && head.status === 0 && head.stdout.trim() === scope.sha, 'B6_SOURCE_SHA_MISMATCH')
  const parent = lstatSync(env.RUNNER_TEMP)
  demand(parent.isDirectory() && !parent.isSymbolicLink() && realpathSync(env.RUNNER_TEMP) === env.RUNNER_TEMP && parent.uid === process.getuid(), 'B6_PATH_REJECTED')
  demand(!existsSync(scope.root), 'B6_EXISTING_FIXTURE_REJECTED')
  const inspectImage = id => JSON.parse(docker(['image', 'inspect', '--format', imageFormat, id]))
  const appImage = inspectImage(scope.appImage)
  const bunImage = inspectImage(scope.bunImage)
  for (const [image, id] of [[appImage, scope.appImage], [bunImage, scope.bunImage]]) {
    demand(image.id === id && image.os === 'linux' && image.arch === 'amd64' && image.labels?.['io.myapi.smoke.source'] === scope.sha &&
      Number.isSafeInteger(image.size) && image.size > 0 && Array.isArray(image.layers) && image.layers.length > 0 && image.layers.every(layer => /^sha256:[a-f0-9]{64}$/.test(layer)), 'B6_IMAGE_IDENTITY_MISMATCH')
  }
  demand(appImage.labels?.['io.myapi.edition'] === 'full' && ['', '0', 'root', '0:0', 'root:root'].includes(appImage.user), 'B6_BASELINE_IMAGE_USER_UNEXPECTED')
  const history = docker(['image', 'history', '--no-trunc', '--format', '{{.Size}}', scope.appImage]).split('\n')
  demand(history.length > 0 && history.every(size => /^\d+(\.\d+)?(B|kB|MB|GB|TB)$/.test(size)), 'B6_IMAGE_SIZE_UNAVAILABLE')
  for (const name of Object.values(scope.names)) demand(docker(['container', 'ls', '-a', '--filter', `name=^/${name}$`, '--format', '{{.ID}}']) === '', 'B6_EXISTING_CONTAINER_REJECTED')
  demand(docker(['network', 'ls', '--filter', `name=^${scope.network}$`, '--format', '{{.ID}}']) === '', 'B6_EXISTING_NETWORK_REJECTED')

  mkdirSync(scope.root, { mode: 0o700 })
  const ownership = JSON.stringify({ sha: scope.sha, run: scope.run })
  writeFileSync(path.join(scope.root, 'ownership.json'), ownership, { flag: 'wx', mode: 0o600 })
  const created = new Set()
  const descriptors = new Map()
  let networkId = null
  let assessment = null
  let fixtureChecks = null
  let sourceUID = null
  let sourceHostOwner = null
  let cloneHostOwner = null
  let failure = null
  let cleaned = false
  let candidateStart = null
  const secret = randomBytes(32).toString('hex')
  const applicationEnvironment = {
    MYAPI_EDITION: 'full', SESSION_SECRET: secret, MEMORY_CACHE_ENABLED: 'false', BATCH_UPDATE_ENABLED: 'false',
    SESSION_COOKIE_SECURE: 'false', TRUSTED_PROXIES: 'none', FULL_CONTENT_LOG_ENABLED: 'false',
    CHANNEL_QUOTA_SYNC_ENABLED: 'false', SQL_MAX_OPEN_CONNS: '1', SQL_MAX_IDLE_CONNS: '1', GOMEMLIMIT: '512MiB',
  }
  const applicationEnv = { ...env, ...applicationEnvironment }
  const hostOwner = folder => {
    const stat = lstatSync(path.join(scope.root, folder))
    demand(stat.isDirectory() && !stat.isSymbolicLink() && stat.uid === 0 && stat.gid === 0 && (stat.mode & 0o7777) === 0o700, 'B6_HOST_ROOT_OWNERSHIP_UNVERIFIED')
    return { uid: stat.uid, gid: stat.gid, mode: '0700' }
  }
  const labels = role => ['--label', `io.myapi.b6.sha=${scope.sha}`, '--label', `io.myapi.b6.run=${scope.run}`, '--label', `io.myapi.b6.role=${role}`, '--label', `io.myapi.b6.layout=${testedDataMode}`]
  const inspect = role => {
    const descriptor = descriptors.get(role)
    demand(descriptor, 'B6_CONTAINER_OWNERSHIP_UNKNOWN')
    const value = JSON.parse(docker(['container', 'inspect', '--format', containerFormat, scope.names[role]]))
    demand(/^[a-f0-9]{64}$/.test(value.id) && (!descriptor.id || value.id === descriptor.id) && value.image === descriptor.image && value.user === descriptor.user && value.network === descriptor.network && (!descriptor.cwd || value.cwd === descriptor.cwd) &&
      value.labels?.['io.myapi.b6.sha'] === scope.sha && value.labels?.['io.myapi.b6.run'] === scope.run && value.labels?.['io.myapi.b6.role'] === role && value.labels?.['io.myapi.b6.layout'] === testedDataMode &&
      Array.isArray(value.mounts), 'B6_CONTAINER_BINDING_MISMATCH')
    const binds = value.mounts.filter(mount => mount.Type === 'bind')
    const otherMounts = value.mounts.filter(mount => mount.Type !== 'bind')
    demand(binds.length === descriptor.mounts.length && (role === 'fake' || role === 'reader'
      ? otherMounts.length <= 1 && otherMounts.every(mount => mount.Type === 'tmpfs' && mount.Destination === '/tmp' && mount.RW === true)
      : otherMounts.length === 0), 'B6_CONTAINER_BINDING_MISMATCH')
    for (const expected of descriptor.mounts) {
      demand(value.mounts.some(mount => mount.Type === 'bind' && mount.Source === expected.source && mount.Destination === expected.destination && mount.RW === expected.rw), 'B6_CONTAINER_BINDING_MISMATCH')
    }
    const ports = value.ports || {}
    const expectedPorts = descriptor.ports || {}
    demand(Object.keys(ports).sort().join(',') === Object.keys(expectedPorts).sort().join(','), 'B6_CONTAINER_BINDING_MISMATCH')
    for (const [containerPort, hostPort] of Object.entries(expectedPorts)) {
      demand(Array.isArray(ports[containerPort]) && ports[containerPort].length === 1 && ports[containerPort][0].HostIp === '127.0.0.1' && ports[containerPort][0].HostPort === hostPort, 'B6_CONTAINER_BINDING_MISMATCH')
    }
    return value
  }
  const remove = role => {
    const listed = docker(['container', 'ls', '-a', '--filter', `name=^/${scope.names[role]}$`, '--format', '{{.ID}}'])
    if (listed === '') return
    const container = inspect(role)
    docker(['container', 'rm', '--force', container.id])
    demand(docker(['container', 'ls', '-a', '--filter', `name=^/${scope.names[role]}$`, '--format', '{{.ID}}']) === '', 'B6_CONTAINER_REMOVAL_UNCONFIRMED')
  }
  // The internal bridge intentionally has no host-published-port path. A
  // source-bound Bun process transports each single request inside the exact
  // owned network namespace; it cannot reach arbitrary destinations.
  const isolatedFetch = async (value, options = {}) => {
    const target = assessmentTransportTarget(value)
    options.signal?.throwIfAborted()
    demand(options.body === undefined || options.body === null || typeof options.body === 'string', 'B6_TRANSPORT_BODY_REJECTED')
    const container = inspect(target.role)
    demand(container.running === true, 'B6_TRANSPORT_NOT_RUNNING')
    const request = { url: target.url, method: options.method || 'GET', headers: [...new Headers(options.headers)], body: options.body }
    const result = spawnSync('docker', ['exec', '-i', container.id, 'bun', '-e', requestProgram],
      { input: JSON.stringify(request), encoding: 'utf8', timeout: 15000, maxBuffer: 4 * 1024 * 1024 })
    demand(!result.error && result.status === 0, 'B6_TRANSPORT_FAILED')
    const response = JSON.parse(result.stdout)
    demand(Number.isInteger(response.status) && response.status >= 100 && response.status <= 599 && Array.isArray(response.headers) &&
      typeof response.body === 'string' && /^[A-Za-z0-9+/]*={0,2}$/.test(response.body), 'B6_TRANSPORT_RESPONSE_INVALID')
    const body = Buffer.from(response.body, 'base64')
    return new Response([101, 204, 205, 304].includes(response.status) ? null : body, { status: response.status, headers: response.headers })
  }
  const filesystem = action => {
    demand(realpathSync(scope.root) === scope.root && readFileSync(path.join(scope.root, 'ownership.json'), 'utf8') === ownership, 'B6_DIRECTORY_OWNERSHIP_MISMATCH')
    descriptors.set('fs', { image: scope.bunImage, user: '0:0', network: 'none', mounts: [{ source: scope.root, destination: '/assessment', rw: true }] })
    created.add('fs')
    const config = Object.entries(applicationEnvironment).map(([key, value]) => `${key}=${value}`).join('\n') + '\n'
    const id = docker(['create', '--pull=never', '--name', scope.names.fs, ...labels('fs'), '--user', '0:0', '--network', 'none',
      '--cpus', '0.5', '--memory', '256m', '--pids-limit', '64', '--mount', `type=bind,src=${scope.root},dst=/assessment`,
      '--env', 'MYAPI_B6_FS_ACTION', '--env', 'MYAPI_B6_CONFIG', '--env', 'MYAPI_B6_DATA_MODE', '--entrypoint', 'bun', scope.bunImage, '-e', filesystemProgram],
    { env: { ...env, MYAPI_B6_FS_ACTION: action, MYAPI_B6_CONFIG: config, MYAPI_B6_DATA_MODE: testedDataMode } })
    demand(/^[a-f0-9]{64}$/.test(id) && inspect('fs').id === id, 'B6_CREATE_UNCONFIRMED')
    descriptors.get('fs').id = id
    const raw = docker(['start', '--attach', id])
    const stopped = inspect('fs')
    demand(stopped.status === 'exited' && stopped.running === false && stopped.exitCode === 0 && stopped.oomKilled === false, 'B6_FILESYSTEM_EFFECT_UNCONFIRMED')
    remove('fs')
    return JSON.parse(raw)
  }
  const createApp = (role, folder, uid, port, marker = false) => {
    const mounts = [{ source: path.join(scope.root, folder, 'data'), destination: '/data', rw: true }, { source: path.join(scope.root, folder, 'logs'), destination: '/app/logs', rw: true }]
    descriptors.set(role, { image: scope.appImage, user: uid, cwd: '/data', network: scope.network, mounts,
      ports: { '3000/tcp': String(port), ...(role === 'source' ? { '19090/tcp': '19090' } : {}) } })
    created.add(role)
    const args = ['create', '--pull=never', '--name', scope.names[role], ...labels(role), '--user', uid, '--network', scope.network,
      '--publish', `127.0.0.1:${port}:3000`, '--cpus', '1', '--memory', '768m', '--pids-limit', '256',
      ...mounts.flatMap(mount => ['--mount', `type=bind,src=${mount.source},dst=${mount.destination}`]),
      ...Object.keys(applicationEnvironment).flatMap(key => ['--env', key])]
    if (role === 'source') args.push('--publish', '127.0.0.1:19090:19090')
    if (marker) args.push('--entrypoint', '/bin/sh')
    args.push(scope.appImage)
    if (marker) args.push('-c', 'printf "MYAPI_B6_UID=%s:GID=%s\\n" "$(id -u)" "$(id -g)"; exec /my-api --log-dir /app/logs')
    else args.push('--log-dir', '/app/logs')
    const id = docker(args, { env: applicationEnv })
    demand(/^[a-f0-9]{64}$/.test(id) && inspect(role).id === id, 'B6_CREATE_UNCONFIRMED')
    descriptors.get(role).id = id
    if (role === 'candidate') {
      candidateStart = spawnSync('docker', ['start', id], { env: applicationEnv, encoding: 'utf8', timeout: 60000, maxBuffer: 1024 * 1024 })
      demand(!candidateStart.error && !candidateStart.signal && Number.isInteger(candidateStart.status), 'B6_START_UNCONFIRMED')
      if (testedDataMode === '0755') demand(candidateStart.status === 0, 'B6_START_UNCONFIRMED')
    } else {
      docker(['start', id])
    }
    return id
  }
  const stop = role => {
    const current = inspect(role)
    demand(current.running === true, 'B6_STOP_SOURCE_NOT_RUNNING')
    docker(['stop', '--time', '150', current.id], { timeout: 180000 })
    const stopped = inspect(role)
    demand(stopped.status === 'exited' && stopped.running === false && stopped.exitCode === 0 && stopped.oomKilled === false, 'B6_STOP_UNCONFIRMED')
    return stopped
  }
  const ready = async (url, role) => {
    let httpStatus = null
    let transportCode = null
    for (let attempt = 0; attempt < 60; attempt++) {
      try {
        const response = await isolatedFetch(url, { redirect: 'error', signal: AbortSignal.timeout(2000) })
        httpStatus = response.status
        const body = await response.json()
        if (response.status === 200 && body?.success === true) return
      } catch (error) {
        const code = error?.cause?.code
        transportCode = ['ECONNREFUSED', 'ECONNRESET', 'ETIMEDOUT', 'EHOSTUNREACH', 'ENETUNREACH'].includes(code) ? code : 'unknown'
      }
      await delay(1000)
    }
    // Readiness evidence contains no application output, environment values or
    // response payload. An independent in-container read separates startup
    // failure from the host's published-port path without changing assertions.
    const state = inspect(role)
    let insideReady = null
    if (state.running === true) {
      const observed = spawnSync('docker', ['exec', state.id, '/bin/sh', '-c', 'wget -q -T 3 -O - http://127.0.0.1:3000/api/status'],
        { encoding: 'utf8', timeout: 5000, maxBuffer: 1024 * 1024 })
      if (!observed.error && observed.status === 0) {
        try { insideReady = JSON.parse(observed.stdout)?.success === true } catch { /* Unknown remains null. */ }
      }
    }
    process.stderr.write(JSON.stringify({ stage: 'readiness', role, http_status: httpStatus, transport_code: transportCode,
      container_status: state.status, running: state.running, exit_code: state.exitCode, oom_killed: state.oomKilled,
      inside_container_ready: insideReady, internal_network: true }) + '\n')
    throw new Error('B6_READINESS_FAILED')
  }

  try {
    networkId = docker(['network', 'create', '--internal', ...labels('network'), scope.network])
    const network = JSON.parse(docker(['network', 'inspect', '--format', networkFormat, networkId]))
    demand(network.id === networkId && network.name === scope.network && network.internal === true && network.labels?.['io.myapi.b6.sha'] === scope.sha && network.labels?.['io.myapi.b6.run'] === scope.run && network.labels?.['io.myapi.b6.layout'] === testedDataMode, 'B6_NETWORK_UNVERIFIED')
    demand(filesystem('initialize').initialized === true, 'B6_FIXTURE_INITIALIZATION_FAILED')
    sourceHostOwner = hostOwner('source')
    const sourceId = createApp('source', 'source', '0:0', 18080)
    descriptors.set('fake', { image: scope.bunImage, user: '1000:1000', network: `container:${sourceId}`, mounts: [] })
    created.add('fake')
    const fakeId = docker(['run', '--pull=never', '--detach', '--name', scope.names.fake, ...labels('fake'), '--network', `container:${sourceId}`,
      '--user', '1000:1000', '--read-only', '--tmpfs', '/tmp:rw,noexec,nodev,nosuid,size=32m', '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges',
      '--cpus', '0.25', '--memory', '128m', '--pids-limit', '64', '--env', 'BUN_RUNTIME_TRANSPILER_CACHE_PATH=0', '--env', 'FAKE_OPENAI_LISTEN_HOST=0.0.0.0',
      '--entrypoint', 'bun', scope.bunImage, '/fixture/fake-openai.mjs'])
    demand(/^[a-f0-9]{64}$/.test(fakeId) && inspect('fake').id === fakeId, 'B6_CREATE_UNCONFIRMED')
    descriptors.get('fake').id = fakeId
    await ready('http://127.0.0.1:18080/api/status', 'source')
    const identity = docker(['exec', sourceId, '/bin/sh', '-c', 'awk \'/^Uid:/{print "uid=" $3} /^Gid:/{print "gid=" $3}\' /proc/1/status; stat -c "mode=%a" /my-api'])
    demand(identity === 'uid=0\ngid=0\nmode=755', 'B6_SOURCE_PROCESS_UID_UNVERIFIED')
    sourceUID = { uid: 0, gid: 0, application_mode: '0755' }
    for (let attempt = 0; attempt < 30; attempt++) {
      try {
        const response = await isolatedFetch('http://127.0.0.1:19090/__smoke__/health', { signal: AbortSignal.timeout(2000) })
        if (response.ok) break
      } catch { /* Fixed synthetic upstream readiness only. */ }
      demand(attempt < 29, 'B6_FAKE_UPSTREAM_UNAVAILABLE')
      await delay(1000)
    }
    const fixture = await probeFreshSQLite({ baseUrl: 'http://127.0.0.1:18080', edition: 'full', sha: scope.sha, isolated: true,
      relayFixture: true, upstreamBaseUrl: 'http://127.0.0.1:19090', writer: 'legacy', fetchImpl: isolatedFetch,
      restoreFixture: async args => {
        await verifySQLiteRestore({ ...args, restore: async verify => {
          stop('source')
          const original = validateRootManifest(filesystem('snapshot-source'), { dataMode })
          demand(filesystem('clone').copied === true, 'B6_CLONE_FAILED')
          cloneHostOwner = hostOwner('clone')
          assessment = await assessPreservedClone({ original, dataMode, rejectionStage: testedDataMode === '0755' ? 'application' : 'oci_workdir', snapshot: () => filesystem('snapshot-clone'),
            upstreamCount: () => {
              const fake = inspect('fake')
              demand(fake.running === true, 'B6_FAKE_UPSTREAM_UNAVAILABLE')
              const count = docker(['exec', fake.id, 'bun', '-e', 'const r = await fetch("http://127.0.0.1:19090/__smoke__/control", {signal: AbortSignal.timeout(5000)}); const b = await r.json(); if (!r.ok || !Number.isSafeInteger(b.count)) process.exit(1); console.log(b.count)'])
              demand(/^[0-9]+$/.test(count), 'B6_UPSTREAM_COUNT_UNAVAILABLE')
              return Number(count)
            },
            startNonroot: () => createApp('candidate', 'clone', '10001:10001', 18081, true),
            outcome: async () => {
              let state
              for (let attempt = 0; attempt < 30; attempt++) {
                state = inspect('candidate')
                if (state.status === 'exited' || testedDataMode === '0700' && state.running === false) break
                demand(state.running === true && attempt < 29, 'B6_NONROOT_DID_NOT_REFUSE')
                await delay(1000)
              }
              const raw = spawnSync('docker', ['logs', scope.names.candidate], { encoding: 'utf8', timeout: 10000, maxBuffer: 1024 * 1024 })
              demand(!raw.error && raw.status === 0, 'B6_REFUSAL_LOG_UNAVAILABLE')
              const logs = `${raw.stdout}${raw.stderr}`
              const markers = [...logs.matchAll(/^MYAPI_B6_UID=(\d+):GID=(\d+)$/gm)]
              if (testedDataMode === '0700') {
                demand(candidateStart && candidateStart.status !== 0 && markers.length === 0, 'B6_OCI_REFUSAL_UNVERIFIED')
                const diagnostic = `${candidateStart.stderr}${state.runtimeError}`
                return { ...state, uid: null, gid: null, processStarted: false, startExitCode: candidateStart.status,
                  reason: /permission denied/i.test(diagnostic) && /chdir|cwd|working directory/i.test(diagnostic) && diagnostic.includes('/data') ? 'oci-workdir-permission-denied' : 'unknown' }
              }
              demand(markers.length === 1, 'B6_NONROOT_PROCESS_UID_UNVERIFIED')
              return { ...state, uid: Number(markers[0][1]), gid: Number(markers[0][2]), processStarted: true, startExitCode: candidateStart?.status,
                reason: /failed to open log file|permission denied/i.test(logs) ? 'permission-denied-startup' : 'unknown' }
            },
            removeNonroot: () => remove('candidate'),
            startRoot: () => {
              const recoveryId = createApp('recovery', 'clone', '0:0', 18081)
              descriptors.set('reader', { image: scope.bunImage, user: '1000:1000', network: `container:${recoveryId}`, mounts: [] })
              created.add('reader')
              const readerId = docker(['run', '--pull=never', '--detach', '--name', scope.names.reader, ...labels('reader'), '--network', `container:${recoveryId}`,
                '--user', '1000:1000', '--read-only', '--tmpfs', '/tmp:rw,noexec,nodev,nosuid,size=32m', '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges',
                '--cpus', '0.25', '--memory', '128m', '--pids-limit', '64', '--env', 'BUN_RUNTIME_TRANSPILER_CACHE_PATH=0',
                '--entrypoint', 'bun', scope.bunImage, '-e', 'setInterval(() => {}, 1000)'])
              demand(/^[a-f0-9]{64}$/.test(readerId) && inspect('reader').id === readerId, 'B6_CREATE_UNCONFIRMED')
              descriptors.get('reader').id = readerId
            },
            verifyRoot: async () => {
              await ready('http://127.0.0.1:18081/api/status', 'recovery')
              const recovery = inspect('recovery')
              demand(recovery.running === true && docker(['exec', recovery.id, '/bin/sh', '-c', 'awk \'/^Uid:/{print "uid=" $3} /^Gid:/{print "gid=" $3}\' /proc/1/status']) === 'uid=0\ngid=0', 'B6_RECOVERY_PROCESS_UID_UNVERIFIED')
              await verify('http://127.0.0.1:18081')
            }, stopRoot: () => stop('recovery'),
          })
          demand(JSON.stringify(validateRootManifest(filesystem('snapshot-source'), { dataMode })) === JSON.stringify(original), 'B6_ORIGINAL_FIXTURE_CHANGED')
        } })
        return { name: 'root-owned old volume refuses nonroot unchanged and recovers its exact synthetic business state', ok: true }
      } })
    demand(fixture.passed === true && assessment?.assessment_completed === true, 'B6_BUSINESS_EVIDENCE_MISSING')
    fixtureChecks = fixture.checks.map(({ name, ok }) => ({ name, ok }))
  } catch (error) {
    failure = error
  } finally {
    try {
      for (const role of ['reader', 'candidate', 'recovery', 'fake', 'source', 'fs']) if (created.has(role)) remove(role)
      if (networkId) {
        const network = JSON.parse(docker(['network', 'inspect', '--format', networkFormat, networkId]))
        demand(network.id === networkId && network.labels?.['io.myapi.b6.sha'] === scope.sha && network.labels?.['io.myapi.b6.run'] === scope.run && network.labels?.['io.myapi.b6.layout'] === testedDataMode, 'B6_NETWORK_OWNERSHIP_UNKNOWN')
        docker(['network', 'rm', networkId])
      }
      demand(filesystem('cleanup').cleaned === true, 'B6_CLEANUP_FAILED')
      demand(realpathSync(scope.root) === scope.root && readFileSync(path.join(scope.root, 'ownership.json'), 'utf8') === ownership &&
        readdirSync(scope.root).join('\n') === 'ownership.json', 'B6_DIRECTORY_OWNERSHIP_MISMATCH')
      rmSync(scope.root, { recursive: true })
      cleaned = true
    } catch {
      failure = new Error('B6_OWNED_CLEANUP_UNCONFIRMED')
    }
  }
  if (failure) throw failure
  demand(cleaned, 'B6_OWNED_CLEANUP_UNCONFIRMED')
  return { schema_version: 1, command: 'container-hardening-assessment', source_sha: scope.sha, run: scope.run,
    ...assessment, source_process: sourceUID, source_image_id: scope.appImage, image_config_user: appImage.user,
    fixture_host_root_owners: { source: sourceHostOwner, clone: cloneHostOwner },
    fixture_identity: { network_id: networkId, source_container_id: descriptors.get('source').id,
      nonroot_container_id: descriptors.get('candidate').id, recovery_container_id: descriptors.get('recovery').id },
    stopped_source_fixture_unchanged: true,
    size: { image_size_bytes: appImage.size, rootfs_diff_ids: appImage.layers, history_sizes_human: history,
      transfer_archive_bytes: scope.archiveBytes, transfer_archive_provenance: scope.archiveBytes === null ? 'not-created' : 'workflow-reported', registry_compressed_bytes: null },
    fixture_checks: fixtureChecks, cleanup_verified: true, browser: 'not_run', real_provider_requests: false, default_user_change_authorized: false }
}

export async function runContainerHardening(env = process.env) {
  hardeningScope(env)
  const cases = []
  try {
    for (const mode of ['0755', '0700']) cases.push(await runLayout(env, mode))
  } catch (error) {
    const failed = new Error(error instanceof Error && /^(B6|SMOKE)_[A-Z_]+$/.test(error.message) ? error.message : 'B6_ASSESSMENT_FAILED')
    failed.completedCases = cases
    throw failed
  }
  return { schema_version: 1, command: 'container-hardening-assessment', source_sha: env.GITHUB_SHA,
    assessment_completed: true, compatibility: false, default_user_changed: false, default_user_change_authorized: false,
    verdict: 'nonroot-default-not-safe-for-tested-root-volumes', cases, size: cases[0].size,
    cleanup_verified: cases.every(item => item.cleanup_verified), browser: 'not_run', real_provider_requests: false }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    demand(process.argv.length === 2, 'B6_ARGUMENTS_REJECTED')
    console.log(JSON.stringify(await runContainerHardening(), null, 2))
  } catch (error) {
    const code = error instanceof Error && /^(B6|SMOKE)_[A-Z_]+$/.test(error.message) ? error.message : 'B6_ASSESSMENT_FAILED'
    console.log(JSON.stringify({ schema_version: 1, command: 'container-hardening-assessment', assessment_completed: false, compatibility: null, default_user_changed: false, default_user_change_authorized: false, error: code, completed_cases: error?.completedCases || [] }))
    process.exitCode = 1
  }
}
