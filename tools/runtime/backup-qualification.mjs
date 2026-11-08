import { spawn, spawnSync } from 'node:child_process'
import { createHash, randomBytes } from 'node:crypto'
import { copyFileSync, existsSync, lstatSync, mkdirSync, readFileSync, readdirSync, realpathSync, rmSync, statSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import { createServer } from 'node:http'
import { isIP } from 'node:net'
import { networkInterfaces } from 'node:os'
import { setTimeout as delay } from 'node:timers/promises'
import { fileURLToPath } from 'node:url'
import { probeFreshSQLite } from './docker-smoke.mjs'
import { verifySQLiteRestore } from './sqlite-restore-smoke.mjs'
import { assessmentTransportTarget } from './container-hardening.mjs'
import { containerDigest } from '../operations/docker.mjs'

function demand(value, code) { if (!value) throw new Error(code) }
const hash = value => createHash('sha256').update(value).digest('hex')
const overlap = (a, b) => a === b || a.startsWith(b + '/') || b.startsWith(a + '/')
const repository = fileURLToPath(new URL('../../', import.meta.url))
const idPattern = /^[a-f0-9]{64}$/
const imagePattern = /^sha256:[a-f0-9]{64}$/

function scopeOf(env) {
  demand(process.platform === 'linux' && process.arch === 'x64' && process.getuid() === 0 &&
    env.GITHUB_ACTIONS === 'true' && env.GITHUB_EVENT_NAME === 'workflow_dispatch' && env.GITHUB_REF === 'refs/heads/dev' &&
    env.MYAPI_B4_QUALIFICATION === '1' && env.MYAPI_SMOKE_API_ONLY === '1' && env.MYAPI_SMOKE_BROWSER === '0' && env.MYAPI_SMOKE_PERSONAL === '0', 'B4_SCOPE_REJECTED')
  demand(/^[a-f0-9]{40}$/.test(env.GITHUB_SHA || '') && /^[1-9][0-9]{0,19}$/.test(env.GITHUB_RUN_ID || '') &&
    /^[1-9][0-9]{0,19}$/.test(env.GITHUB_RUN_ATTEMPT || ''), 'B4_IDENTITY_REJECTED')
  demand(imagePattern.test(env.SMOKE_APP_IMAGE_ID || '') && imagePattern.test(env.SMOKE_BUN_IMAGE_ID || '') &&
    env.SMOKE_APP_IMAGE_ID !== env.SMOKE_BUN_IMAGE_ID, 'B4_IMAGE_REJECTED')
  const suffix = `${env.GITHUB_SHA}-${env.GITHUB_RUN_ID}-${env.GITHUB_RUN_ATTEMPT}`
  return {
    sha: env.GITHUB_SHA, run: env.GITHUB_RUN_ID, attempt: env.GITHUB_RUN_ATTEMPT,
    root: `/tmp/myapi-b4-${suffix}`, network: `myapi-b4-network-${suffix}`,
    iface: `b4${hash(suffix).slice(0, 12)}`, ruleComment: `myapi-b4:${env.GITHUB_SHA}:${env.GITHUB_RUN_ID}:${env.GITHUB_RUN_ATTEMPT}`,
    names: Object.fromEntries(['source', 'fake', 'recovery', 'recovery-fake', 'reader'].map(role => [role, `myapi-b4-${role}-${suffix}`])),
    appImage: env.SMOKE_APP_IMAGE_ID, bunImage: env.SMOKE_BUN_IMAGE_ID,
  }
}

function command(executable, args, options = {}) {
  const result = spawnSync(executable, args, { encoding: 'utf8', timeout: 60000, maxBuffer: 8 * 1024 * 1024, ...options })
  demand(!result.error && result.signal === null && result.status === 0, 'B4_FIXTURE_COMMAND_FAILED')
  return result.stdout.trim()
}
const docker = args => command('docker', ['--host', 'unix:///var/run/docker.sock', ...args])

function privateIPv4(value) {
  if (typeof value !== 'string' || isIP(value) !== 4) return false
  const parts = value.split('.').map(Number)
  return parts[0] === 10 || parts[0] === 192 && parts[1] === 168 || parts[0] === 172 && parts[1] >= 16 && parts[1] <= 31
}

function firewall(operation, rule) {
  const result = spawnSync('/usr/sbin/iptables', ['-w', '5', operation, rule.chain, ...rule.args],
    { encoding: 'utf8', timeout: 10000, maxBuffer: 65536 })
  demand(!result.error && result.signal === null && (result.status === 0 || operation === '-C' && result.status === 1), 'B4_FIREWALL_STATE_UNKNOWN')
  return result.status
}

function dockerProbe(id, input) {
  return new Promise((resolve, reject) => {
    const child = spawn('docker', ['--host', 'unix:///var/run/docker.sock', 'exec', '-i', id, 'bun', '-e', canaryProgram],
      { stdio: ['pipe', 'pipe', 'pipe'] })
    let output = '', exceeded = false, timedOut = false
    const timer = setTimeout(() => { timedOut = true; child.kill('SIGKILL') }, 10000)
    child.stdout.on('data', chunk => {
      output += chunk
      if (output.length > 4096) { exceeded = true; child.kill('SIGKILL') }
    })
    child.stderr.resume()
    child.stdin.on('error', () => {})
    child.once('error', () => { clearTimeout(timer); reject(new Error('B4_CANARY_PROBE_UNCONFIRMED')) })
    child.once('close', (code, signal) => {
      clearTimeout(timer)
      if (code !== 0 || signal !== null || timedOut || exceeded) return reject(new Error('B4_CANARY_PROBE_UNCONFIRMED'))
      try { resolve(JSON.parse(output)) } catch { reject(new Error('B4_CANARY_PROBE_UNCONFIRMED')) }
    })
    child.stdin.end(JSON.stringify(input))
  })
}

function tree(directory) {
  const entries = []
  function visit(filename, relative) {
    const info = lstatSync(filename, { bigint: true })
    demand(!info.isSymbolicLink() && (info.isDirectory() || info.isFile()) && (info.isDirectory() || info.nlink === 1n), 'B4_UNSAFE_FIXTURE_TREE')
    demand(entries.length < 10000 && info.uid === 0n, 'B4_FIXTURE_OWNERSHIP_MISMATCH')
    entries.push({
      path: relative, uid: Number(info.uid), gid: Number(info.gid), mode: Number(info.mode & 0o7777n),
      type: info.isDirectory() ? 'directory' : 'file', mtime_ns: info.mtimeNs.toString(),
      size: info.isFile() ? info.size.toString() : null, sha256: info.isFile() ? hash(readFileSync(filename)) : null,
    })
    if (info.isDirectory()) for (const child of readdirSync(filename).sort()) visit(path.join(filename, child), relative === '.' ? child : relative + '/' + child)
  }
  visit(directory, '.')
  return entries
}
const treeDigest = directory => hash(JSON.stringify(tree(directory)))
// Integrity verification creates disposable siblings of data/ inside the
// bundle. Its outer directory timestamp is not payload metadata.
const bundleDigest = directory => hash(JSON.stringify(tree(directory).map(entry => entry.path === '.' ? { ...entry, mtime_ns: null } : entry)))

function processIdentity(info, directory) {
  demand(Number.isInteger(info.State.Pid) && info.State.Pid > 0, 'B4_PROCESS_IDENTITY_MISSING')
  const status = readFileSync(`/proc/${info.State.Pid}/status`, 'utf8')
  const uid = /^Uid:\s+(\d+)\s+(\d+)\s+/m.exec(status)
  const gid = /^Gid:\s+(\d+)\s+(\d+)\s+/m.exec(status)
  const mounted = statSync(`/proc/${info.State.Pid}/root/data`, { bigint: true })
  const actual = statSync(directory, { bigint: true })
  demand(uid && gid && uid[1] === '0' && uid[2] === '0' && gid[1] === '0' && gid[2] === '0' &&
    mounted.dev === actual.dev && mounted.ino === actual.ino, 'B4_PROCESS_IDENTITY_MISMATCH')
  return { uid: 0, gid: 0, host_pid_observed: true, data_inode_bound: true }
}

function writerEvidence(value) {
  return value && value.old_generation_gone === true && Number.isSafeInteger(value.tree_entries) && value.tree_entries > 0 &&
    ['processes', 'tasks', 'fds', 'mapping_regions'].every(key => Number.isSafeInteger(value[key]) && value[key] >= 0) &&
    idPattern.test(value.fingerprint ?? '')
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

const canaryProgram = `
try {
  const input = JSON.parse(await Bun.stdin.text());
  const url = new URL(input.url);
  if (url.protocol !== 'http:' || url.hostname !== input.gateway || url.port !== '19091' || url.pathname !== '/__myapi_b4_canary__' || url.search || url.hash || url.username || url.password) process.exit(1);
  try {
    const response = await fetch(url, {method: 'GET', redirect: 'error', signal: AbortSignal.timeout(2500)});
    await response.arrayBuffer();
    console.log(JSON.stringify({attempted: true, reached: true, status: response.status}));
  } catch (error) {
    const timeout = error?.name === 'TimeoutError' || error?.name === 'AbortError';
    const network = ['ECONNREFUSED', 'ENETUNREACH', 'EHOSTUNREACH', 'ETIMEDOUT', 'ConnectionRefused'].includes(error?.code);
    if (!timeout && !network) process.exit(1);
    console.log(JSON.stringify({attempted: true, reached: false, category: timeout ? 'timeout' : 'network'}));
  }
} catch { process.exit(1); }
`

const auxProgram = `
import json, os, sqlite3, sys
root = sys.argv[1]
assert os.path.realpath(root) == root and os.stat(root).st_uid == 0
db = sqlite3.connect(os.path.join(root, "aux-wal.db"))
assert db.execute("PRAGMA journal_mode=WAL").fetchone()[0] == "wal"
db.execute("CREATE TABLE wal_proof(marker TEXT NOT NULL)")
db.execute("INSERT INTO wal_proof VALUES (?)", ("B4_SYNTHETIC_AUX_WAL",))
db.commit()
assert os.path.getsize(os.path.join(root, "aux-wal.db-wal")) > 32
os._exit(0)
`

const verifyAuxProgram = `
import json, os, sqlite3, sys
root = sys.argv[1]
db = sqlite3.connect("file:" + os.path.join(root, "aux-wal.db") + "?mode=ro", uri=True)
db.execute("PRAGMA query_only=ON")
assert db.execute("PRAGMA quick_check").fetchall() == [("ok",)]
assert db.execute("SELECT marker FROM wal_proof").fetchall() == [("B4_SYNTHETIC_AUX_WAL",)]
db.close()
print('{"verified":true}')
`

export async function runBackupQualification(env = process.env) {
  const scope = scopeOf(env)
  process.umask(0o077)
  const git = command('git', ['-c', 'core.fileMode=false', '-c', `safe.directory=${repository}`, '-C', repository, 'rev-parse', 'HEAD'],
    { env: { ...process.env, GIT_OPTIONAL_LOCKS: '0' } })
  demand(git === scope.sha, 'B4_SOURCE_SHA_MISMATCH')
  for (const id of [scope.appImage, scope.bunImage]) {
    const image = JSON.parse(docker(['image', 'inspect', id]))[0]
    demand(image.Id === id && image.Os === 'linux' && image.Architecture === 'amd64' &&
      image.Config?.Labels?.['io.myapi.smoke.source'] === scope.sha, 'B4_IMAGE_IDENTITY_MISMATCH')
    if (id === scope.appImage) demand(image.Config.Labels['io.myapi.edition'] === 'full', 'B4_EDITION_MISMATCH')
  }
  demand(!existsSync(scope.root), 'B4_EXISTING_FIXTURE_REJECTED')
  demand(/^b4[a-f0-9]{12}$/.test(scope.iface) && scope.iface.length <= 15 && !existsSync(`/sys/class/net/${scope.iface}`), 'B4_EXISTING_INTERFACE_REJECTED')
  const rules = [
    { chain: 'FORWARD', args: ['-i', scope.iface, '-m', 'comment', '--comment', scope.ruleComment, '-j', 'DROP'], attempted: false, verified: false },
    { chain: 'INPUT', args: ['-i', scope.iface, '-m', 'conntrack', '!', '--ctstate', 'ESTABLISHED,RELATED', '-m', 'comment', '--comment', scope.ruleComment, '-j', 'DROP'], attempted: false, verified: false },
  ]
  for (const rule of rules) demand(firewall('-C', rule) === 1, 'B4_EXISTING_FIREWALL_RULE_REJECTED')
  for (const name of Object.values(scope.names)) demand(docker(['container', 'ls', '-aq', '--filter', `name=^/${name}$`]) === '', 'B4_EXISTING_CONTAINER_REJECTED')
  demand(docker(['network', 'ls', '-q', '--filter', `name=^${scope.network}$`]) === '', 'B4_EXISTING_NETWORK_REJECTED')
  mkdirSync(scope.root, { mode: 0o700 })
  const ownership = JSON.stringify({ schema_version: 1, source_sha: scope.sha, run_id: scope.run, attempt: scope.attempt, app_image: scope.appImage, bun_image: scope.bunImage })
  writeFileSync(path.join(scope.root, 'ownership.json'), ownership, { flag: 'wx', mode: 0o600 })
  const data = path.join(scope.root, 'source')
  const state = path.join(scope.root, 'state')
  const backups = path.join(scope.root, 'backups')
  const isolated = path.join(scope.root, 'isolated')
  const restored = path.join(isolated, 'restored')
  const existingTarget = path.join(isolated, 'existing')
  for (const directory of [data, state, backups, isolated, existingTarget, path.join(data, 'config'), path.join(data, 'logs')]) mkdirSync(directory, { mode: 0o700 })
  const secret = randomBytes(32).toString('hex')
  const applicationEnvironment = {
    MYAPI_EDITION: 'full', SESSION_SECRET: secret, MEMORY_CACHE_ENABLED: 'false', BATCH_UPDATE_ENABLED: 'false',
    SESSION_COOKIE_SECURE: 'false', TRUSTED_PROXIES: 'none', FULL_CONTENT_LOG_ENABLED: 'false', CHANNEL_QUOTA_SYNC_ENABLED: 'false',
    SQL_MAX_OPEN_CONNS: '1', SQL_MAX_IDLE_CONNS: '1', GOMEMLIMIT: '512MiB',
  }
  const applicationEnv = { PATH: process.env.PATH, LANG: 'C.UTF-8', ...applicationEnvironment }
  const configBytes = Object.entries(applicationEnvironment).map(([key, value]) => key + '=' + value).join('\n') + '\n'
  writeFileSync(path.join(data, 'config', 'my-api.env'), configBytes, { flag: 'wx', mode: 0o600 })
  const configFile = path.join(scope.root, 'backup.json')
  const descriptors = new Map()
  let networkId = null
  let failure = null
  let checks = null
  let backupReceipt = null
  let restoreReceipt = null
  let stoppedSourceHash = null
  let bundleHash = null
  let auxSHA = null
  let sourceProcess = null
  let recoveryProcess = null
  let cleanupVerified = false
  let stage = 'fixture-initialization'
  let cliFailure = null
  let canary = null
  let canaryHits = 0
  let canaryConnections = 0
  let isolationEvidence = null
  let expectedCanaryConnections = null
  const rulesPresent = () => { for (const rule of rules) demand(rule.verified && firewall('-C', rule) === 0, 'B4_ISOLATION_RULE_MISSING') }
  const labels = role => ['--label', `io.myapi.b4.sha=${scope.sha}`, '--label', `io.myapi.b4.run=${scope.run}`,
    '--label', `io.myapi.b4.attempt=${scope.attempt}`, '--label', `io.myapi.b4.role=${role}`]
  const inspect = role => {
    const expected = descriptors.get(role)
    demand(expected, 'B4_CONTAINER_OWNERSHIP_UNKNOWN')
    const info = JSON.parse(docker(['container', 'inspect', expected.id || scope.names[role]]))[0]
    demand(idPattern.test(info.Id) && (!expected.id || info.Id === expected.id) && info.Image === expected.image &&
      info.Config?.Labels?.['io.myapi.b4.sha'] === scope.sha && info.Config.Labels['io.myapi.b4.run'] === scope.run &&
      info.Config.Labels['io.myapi.b4.attempt'] === scope.attempt && info.Config.Labels['io.myapi.b4.role'] === role &&
      info.HostConfig.NetworkMode === expected.network && info.Config.User === expected.user &&
      (!['source', 'recovery'].includes(role) || info.Config.WorkingDir === '/data' &&
        JSON.stringify(info.HostConfig.Dns) === JSON.stringify(['127.0.0.1'])), 'B4_CONTAINER_BINDING_MISMATCH')
    const binds = info.Mounts.filter(item => item.Type === 'bind')
    demand(binds.length === expected.mounts.length && expected.mounts.every(item => binds.some(mount =>
      mount.Source === item.source && mount.Destination === item.destination && mount.RW === true)), 'B4_CONTAINER_MOUNT_MISMATCH')
    demand(info.Mounts.filter(item => item.Type !== 'bind').every(item => item.Type === 'tmpfs' && item.Destination === '/tmp'), 'B4_CONTAINER_MOUNT_MISMATCH')
    return info
  }
  const create = (role, image, network, mounts, args, childEnv = process.env) => {
    descriptors.set(role, { id: null, image, network, user: role === 'source' || role === 'recovery' ? '0:0' : '1000:1000', mounts })
    const id = command('docker', ['--host', 'unix:///var/run/docker.sock', 'create', '--pull=never', '--name', scope.names[role], ...labels(role),
      '--user', descriptors.get(role).user, '--network', network, ...mounts.flatMap(item => ['--mount', `type=bind,src=${item.source},dst=${item.destination}`]),
      '--cpus', '1', '--memory', '768m', '--pids-limit', '256', ...args], { env: childEnv })
    demand(idPattern.test(id), 'B4_CONTAINER_CREATE_UNCONFIRMED')
    descriptors.get(role).id = id
    inspect(role)
    docker(['start', id])
    return id
  }
  const sidecar = (role, donor, fake) => create(role, scope.bunImage, `container:${donor}`, [], [
    '--read-only', '--tmpfs', '/tmp:rw,noexec,nodev,nosuid,size=32m', '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges',
    '--env', 'BUN_RUNTIME_TRANSPILER_CACHE_PATH=0', '--env', 'FAKE_OPENAI_LISTEN_HOST=0.0.0.0',
    '--entrypoint', 'bun', scope.bunImage, ...(fake ? ['/fixture/fake-openai.mjs'] : ['-e', 'setInterval(() => {}, 1000)']),
  ])
  const app = (role, directory) => create(role, scope.appImage, scope.network, [{ source: directory, destination: '/data' }], [
    '--dns', '127.0.0.1', '--publish', `127.0.0.1:${role === 'source' ? '18080' : '18081'}:3000`,
    ...(role === 'source' ? ['--restart', 'unless-stopped', '--publish', '127.0.0.1:19090:19090'] : []),
    ...Object.keys(applicationEnvironment).flatMap(key => ['--env', key]), scope.appImage, '--log-dir', '/data/logs',
  ], applicationEnv)
  const fetchIn = async (role, value, options = {}) => {
    const target = assessmentTransportTarget(value)
    options.signal?.throwIfAborted()
    const info = inspect(role)
    demand(info.State.Running === true && (options.body == null || typeof options.body === 'string'), 'B4_TRANSPORT_REJECTED')
    const result = spawnSync('docker', ['--host', 'unix:///var/run/docker.sock', 'exec', '-i', info.Id, 'bun', '-e', requestProgram], {
      input: JSON.stringify({ url: target.url, method: options.method || 'GET', headers: [...new Headers(options.headers)], body: options.body }),
      encoding: 'utf8', timeout: 15000, maxBuffer: 4 * 1024 * 1024,
    })
    demand(!result.error && result.signal === null && result.status === 0, 'B4_TRANSPORT_FAILED')
    const reply = JSON.parse(result.stdout)
    demand(Number.isInteger(reply.status) && reply.status >= 100 && reply.status <= 599 && Array.isArray(reply.headers) &&
      typeof reply.body === 'string' && /^[A-Za-z0-9+/]*={0,2}$/.test(reply.body), 'B4_TRANSPORT_RESPONSE_INVALID')
    return new Response([101, 204, 205, 304].includes(reply.status) ? null : Buffer.from(reply.body, 'base64'), { status: reply.status, headers: reply.headers })
  }
  const isolatedFetch = (value, options = {}) => fetchIn(assessmentTransportTarget(value).role, value, options)
  const ready = async (role, url) => {
    for (let attempt = 0; attempt < 60; attempt++) {
      try { const response = await fetchIn(role, url); if (response.ok) return } catch { /* GET readiness only. */ }
      demand(inspect(['reader', 'recovery-fake'].includes(role) ? 'recovery' : 'source').State.Running === true, 'B4_APPLICATION_EXITED')
      await delay(1000)
    }
    throw new Error('B4_READINESS_FAILED')
  }
  const count = async role => {
    const response = await fetchIn(role, 'http://127.0.0.1:19090/__smoke__/control')
    const value = await response.json()
    demand(response.status === 200 && Number.isInteger(value.count) && value.count >= 0, 'B4_UPSTREAM_COUNT_INVALID')
    return value.count
  }
  const hostHealth = async origin => {
    demand(['http://127.0.0.1:18080', 'http://127.0.0.1:18081'].includes(origin), 'B4_HOST_TARGET_REJECTED')
    const response = await fetch(origin + '/api/status', { redirect: 'error', signal: AbortSignal.timeout(5000) })
    const body = await response.json()
    demand(response.status === 200 && body?.success === true, 'B4_HOST_PUBLISHED_HEALTH_UNCONFIRMED')
  }
  const cli = (kind, extra = [], expectedFailure = null) => {
    const result = spawnSync(process.execPath, ['cli/myapi.mjs', kind, '--config', configFile, '--instance', 'primary', ...extra, '--json'], {
      cwd: repository, env: { PATH: process.env.PATH, LANG: 'C.UTF-8' }, encoding: 'utf8', timeout: 360000, maxBuffer: 65536,
    })
    demand(!result.error && result.signal === null, 'B4_CLI_UNCONFIRMED')
    if (expectedFailure) {
      demand(result.status === 1 && result.stderr.includes(expectedFailure), 'B4_CLI_EXPECTED_REJECTION_MISSING')
      return null
    }
    if (result.status !== 0) {
      try {
        const failed = JSON.parse(result.stdout)
        if (failed.schema_version === 1 && typeof failed.code === 'string' && /^[A-Z_]+$/.test(failed.code)) {
          cliFailure = { state: failed.state, code: failed.code }
        }
      } catch {
        // cli/myapi.mjs emits exactly "MyAPI: CODE" on this failure path.
        // Accept one whole safe-code line only; never print raw diagnostics.
        const lines = result.stderr.split(/\r?\n/)
        if (lines.at(-1) === '') lines.pop()
        const safe = lines.length === 1 ? /^MyAPI: ([A-Z][A-Z0-9_]{0,79})$/.exec(lines[0]) : null
        if (safe && safe[0] === lines[0]) cliFailure = { state: null, code: safe[1] }
      }
      throw new Error('B4_PRODUCTION_CLI_FAILED')
    }
    const value = JSON.parse(result.stdout)
    demand(value.schema_version === 1 && idPattern.test(value.id) && idPattern.test(value.digest) &&
      value.kind === kind && value.instance_id === 'primary' && value.source_image_id === scope.appImage &&
      value.source_container_id === descriptors.get('source').id && value.retention?.policy === 'keep-all' &&
      value.retention.deletions === 0 && value.restarts_instance === false && value.starts_recovery_instance === false &&
      value.overwrites_directory === false, 'B4_CLI_RECEIPT_INVALID')
    return value
  }
  const stopped = () => {
    const value = inspect('source')
    demand(value.State.Status === 'exited' && value.State.Running === false && value.State.Pid === 0 &&
      value.State.ExitCode === 0 && value.State.OOMKilled === false && value.HostConfig.RestartPolicy.Name === 'no', 'B4_SOURCE_STOP_UNCONFIRMED')
    return value
  }
  const targetUnmounted = directory => {
    for (const id of docker(['container', 'ls', '-aq', '--no-trunc']).split('\n').filter(Boolean)) {
      const value = JSON.parse(docker(['container', 'inspect', id]))[0]
      demand(!(value.Mounts || []).some(mount => mount.Type === 'bind' && overlap(path.resolve(mount.Source), directory)), 'B4_CLI_STARTED_RESTORE_CONTAINER')
    }
  }
  try {
    stage = 'network-isolation'
    networkId = docker(['network', 'create', '--driver', 'bridge', '--opt', `com.docker.network.bridge.name=${scope.iface}`, ...labels('network'), scope.network])
    demand(idPattern.test(networkId), 'B4_NETWORK_CREATE_UNCONFIRMED')
    const network = JSON.parse(docker(['network', 'inspect', networkId]))[0]
    demand(network.Id === networkId && network.Internal === false && network.Driver === 'bridge' && network.EnableIPv6 === false &&
      network.Options?.['com.docker.network.bridge.name'] === scope.iface && existsSync(`/sys/class/net/${scope.iface}`) && network.Name === scope.network &&
      network.Labels['io.myapi.b4.sha'] === scope.sha && network.Labels['io.myapi.b4.run'] === scope.run &&
      network.Labels['io.myapi.b4.attempt'] === scope.attempt && network.Labels['io.myapi.b4.role'] === 'network', 'B4_NETWORK_IDENTITY_MISMATCH')
    const addresses = network.IPAM?.Config
    demand(Array.isArray(addresses) && addresses.length === 1 && privateIPv4(addresses[0].Gateway), 'B4_GATEWAY_UNCONFIRMED')
    const gateway = addresses[0].Gateway
    demand(networkInterfaces()[scope.iface]?.some(address => address.family === 'IPv4' && address.address === gateway), 'B4_GATEWAY_INTERFACE_MISMATCH')
    for (const rule of rules) {
      rule.attempted = true
      demand(firewall('-I', rule) === 0 && firewall('-C', rule) === 0, 'B4_ISOLATION_RULE_UNCONFIRMED')
      rule.verified = true
    }
    canary = createServer((request, response) => {
      canaryHits++
      response.writeHead(request.method === 'GET' && request.url === '/__myapi_b4_canary__' ? 200 : 404, { 'Content-Type': 'text/plain' })
      response.end('myapi-b4-owned-canary')
    })
    canary.on('connection', () => { canaryConnections++ })
    await new Promise((resolve, reject) => {
      canary.once('error', () => reject(new Error('B4_CANARY_LISTEN_UNCONFIRMED')))
      canary.listen(19091, gateway, resolve)
    })
    const sourceId = app('source', data)
    sidecar('fake', sourceId, true)
    stage = 'source-readiness'
    await ready('fake', 'http://127.0.0.1:18080/api/status')
    await ready('fake', 'http://127.0.0.1:19090/__smoke__/health')
    rulesPresent()
    stage = 'network-canary'
    const canaryURL = `http://${gateway}:19091/__myapi_b4_canary__`
    const hostControl = async () => {
      const response = await fetch(canaryURL, { redirect: 'error', signal: AbortSignal.timeout(3000) })
      demand(response.status === 200 && await response.text() === 'myapi-b4-owned-canary', 'B4_CANARY_HOST_CONTROL_FAILED')
    }
    await hostControl()
    demand(canaryHits === 1, 'B4_CANARY_UNEXPECTED_HIT')
    const beforeProbeConnections = canaryConnections
    const probe = await dockerProbe(inspect('fake').Id, { url: canaryURL, gateway })
    demand(probe?.attempted === true && probe.reached === false && ['timeout', 'network'].includes(probe.category) &&
      canaryHits === 1 && canaryConnections === beforeProbeConnections, 'B4_CANARY_CONTAINER_REACHED_HOST')
    await hostControl()
    demand(canaryHits === 2, 'B4_CANARY_UNEXPECTED_HIT')
    expectedCanaryConnections = canaryConnections
    isolationEvidence = { bridge_interface: scope.iface, rule_comment: scope.ruleComment, forward_rule_verified: true,
      input_rule_verified: true, host_canary_controls: 2, container_canary_reached: false, probe_category: probe.category,
      external_network_probe: 'not_run', dns_loopback_only: true }
    stage = 'source-published-health'
    await hostHealth('http://127.0.0.1:18080')
    sourceProcess = processIdentity(inspect('source'), data)
    stage = 'synthetic-business-fixture'
    const fixture = await probeFreshSQLite({
      baseUrl: 'http://127.0.0.1:18080', edition: 'full', sha: scope.sha, isolated: true, relayFixture: true,
      upstreamBaseUrl: 'http://127.0.0.1:19090', writer: 'legacy', fetchImpl: isolatedFetch,
      restoreFixture: async args => {
        await verifySQLiteRestore({ ...args, restore: async verify => {
          demand(await count('fake') === 2, 'B4_SEED_REQUEST_COUNT_MISMATCH')
          command('/usr/bin/python3', ['-I', '-c', auxProgram, data])
          demand(lstatSync(path.join(data, 'aux-wal.db-wal')).size > 32, 'B4_AUX_WAL_MISSING')
          writeFileSync(configFile, JSON.stringify({ docker_socket: '/var/run/docker.sock', state_dir: state, backups_dir: backups,
            instances: [{ id: 'primary', container: sourceId, data_dir: data, sqlite_file: path.join(data, 'my-api.db'), health_url: 'http://127.0.0.1:18080/' }] }),
          { flag: 'wx', mode: 0o600 })
          const beforePlan = treeDigest(data)
          const runtimeBefore = containerDigest(inspect('source'))
          stage = 'backup-plan'
          const plan = cli('backup')
          demand(plan.state === 'prepared' && plan.backup_created === false && plan.restored === false && readdirSync(backups).length === 0 &&
            treeDigest(data) === beforePlan && containerDigest(inspect('source')) === runtimeBefore &&
            inspect('source').State.Running === true, 'B4_PLAN_CHANGED_SOURCE')
          const wrongDigest = (plan.digest[0] === '0' ? '1' : '0') + plan.digest.slice(1)
          cli('backup', ['--confirm', plan.id + ':' + wrongDigest], 'PLAN_CONFIRMATION_INVALID')
          demand(treeDigest(data) === beforePlan && containerDigest(inspect('source')) === runtimeBefore, 'B4_WRONG_DIGEST_CHANGED_SOURCE')
          stage = 'backup-confirm'
          backupReceipt = cli('backup', ['--confirm', plan.id + ':' + plan.digest])
          demand(backupReceipt.state === 'succeeded' && backupReceipt.backup_created === true, 'B4_BACKUP_NOT_COMMITTED')
          stopped()
          stoppedSourceHash = treeDigest(data)
          const journal = JSON.parse(readFileSync(path.join(state, plan.id + '.json'), 'utf8'))
          demand(journal.bundle?.committed === true && journal.bundle.verified === true && journal.bundle.integrity_verified === true &&
            writerEvidence(journal.writer_before) && writerEvidence(journal.writer_after), 'B4_JOURNAL_EVIDENCE_MISSING')
          const bundle = path.join(backups, plan.id)
          bundleHash = bundleDigest(bundle)
          demand(JSON.stringify(tree(path.join(bundle, 'data'))) === JSON.stringify(tree(data)), 'B4_BACKUP_METADATA_MISMATCH')
          cli('backup', ['--confirm', plan.id + ':' + plan.digest], 'PLAN_CONFIRMATION_INVALID')
          demand(treeDigest(data) === stoppedSourceHash && bundleDigest(bundle) === bundleHash, 'B4_REPEAT_CONFIRM_CHANGED_DATA')
          cli('restore', ['--backup', plan.id, '--target-dir', existingTarget], 'RESTORE_TARGET_EXISTS')
          demand(readdirSync(existingTarget).length === 0 && treeDigest(data) === stoppedSourceHash, 'B4_EXISTING_TARGET_CHANGED')
          cli('restore', ['--backup', plan.id, '--target-dir', data], 'RESTORE_TARGET_OVERLAP')
          demand(treeDigest(data) === stoppedSourceHash, 'B4_ORIGINAL_TARGET_CHANGED')
          stage = 'restore-plan'
          const restorePlan = cli('restore', ['--backup', plan.id, '--target-dir', restored])
          demand(restorePlan.state === 'prepared' && restorePlan.restored === false && !existsSync(restored) &&
            treeDigest(data) === stoppedSourceHash, 'B4_RESTORE_PLAN_CHANGED_DATA')
          stage = 'restore-confirm'
          restoreReceipt = cli('restore', ['--backup', plan.id, '--target-dir', restored, '--confirm', restorePlan.id + ':' + restorePlan.digest])
          demand(restoreReceipt.state === 'succeeded' && restoreReceipt.restored === true &&
            JSON.stringify(tree(restored)) === JSON.stringify(tree(data)) && treeDigest(data) === stoppedSourceHash &&
            bundleDigest(bundle) === bundleHash, 'B4_RESTORE_METADATA_MISMATCH')
          stopped()
          targetUnmounted(restored)
          const validation = path.join(scope.root, 'wal-validation')
          mkdirSync(validation, { mode: 0o700 })
          for (const suffix of ['', '-wal', '-shm']) copyFileSync(path.join(restored, 'aux-wal.db' + suffix), path.join(validation, 'aux-wal.db' + suffix))
          const verifiedAux = JSON.parse(command('/usr/bin/python3', ['-I', '-c', verifyAuxProgram, validation]))
          demand(verifiedAux.verified === true && treeDigest(restored) === stoppedSourceHash &&
            bundleDigest(bundle) === bundleHash && treeDigest(data) === stoppedSourceHash, 'B4_WAL_VALIDATION_CHANGED_ORIGINAL')
          auxSHA = hash(readFileSync(path.join(restored, 'aux-wal.db-wal')))
          stage = 'recovery-readiness'
          const recoveryId = app('recovery', restored)
          sidecar('recovery-fake', recoveryId, true)
          sidecar('reader', recoveryId, false)
          await ready('reader', 'http://127.0.0.1:18081/api/status')
          await ready('recovery-fake', 'http://127.0.0.1:19090/__smoke__/health')
          rulesPresent()
          await hostHealth('http://127.0.0.1:18081')
          recoveryProcess = processIdentity(inspect('recovery'), restored)
          demand(await count('recovery-fake') === 0, 'B4_RECOVERY_REQUEST_REPLAYED')
          stage = 'recovery-business-readback'
          await verify('http://127.0.0.1:18081')
          demand(await count('fake') === 2 && await count('recovery-fake') === 0, 'B4_RECOVERY_REQUEST_REPLAYED')
          demand(treeDigest(data) === stoppedSourceHash && bundleDigest(bundle) === bundleHash &&
            readFileSync(path.join(restored, 'config/my-api.env'), 'utf8') === configBytes, 'B4_ORIGINAL_OR_BACKUP_CHANGED')
          stopped()
        } })
        return { name: 'production CLI plan-confirm backup and isolated restore preserve exact synthetic business state without replay', ok: true }
      },
    })
    demand(fixture.passed === true, 'B4_BUSINESS_QUALIFICATION_INCOMPLETE')
    checks = fixture.checks.map(({ name, ok }) => ({ name, ok }))
    rulesPresent()
    demand(canaryHits === 2 && canaryConnections === expectedCanaryConnections, 'B4_CANARY_UNEXPECTED_HIT')
  } catch (error) { failure = error; failure.qualificationStage = stage; failure.cliFailure = cliFailure }
  finally {
    try {
      const rootInfo = lstatSync(scope.root)
      const ownerInfo = lstatSync(path.join(scope.root, 'ownership.json'))
      demand(realpathSync(scope.root) === scope.root && rootInfo.isDirectory() && !rootInfo.isSymbolicLink() && rootInfo.uid === 0 &&
        (rootInfo.mode & 0o7777) === 0o700 && ownerInfo.isFile() && !ownerInfo.isSymbolicLink() && ownerInfo.nlink === 1 &&
        ownerInfo.uid === 0 && (ownerInfo.mode & 0o7777) === 0o600 &&
        readFileSync(path.join(scope.root, 'ownership.json'), 'utf8') === ownership, 'B4_DIRECTORY_OWNERSHIP_MISMATCH')
      for (const role of ['reader', 'recovery-fake', 'recovery', 'fake', 'source']) if (descriptors.has(role)) {
        const value = inspect(role)
        docker(['container', 'rm', '--force', value.Id])
        demand(docker(['container', 'ls', '-aq', '--filter', `name=^/${scope.names[role]}$`]) === '', 'B4_CONTAINER_REMOVAL_UNCONFIRMED')
      }
      for (const id of docker(['container', 'ls', '-aq', '--no-trunc']).split('\n').filter(Boolean)) {
        const value = JSON.parse(docker(['container', 'inspect', id]))[0]
        demand(!(value.Mounts || []).some(mount => mount.Type === 'bind' && overlap(path.resolve(mount.Source), scope.root)), 'B4_FOREIGN_MOUNT_BLOCKS_CLEANUP')
      }
      if (networkId) {
        const value = JSON.parse(docker(['network', 'inspect', networkId]))[0]
        demand(value.Id === networkId && value.Name === scope.network && value.Internal === false && value.Driver === 'bridge' &&
          value.EnableIPv6 === false && value.Options?.['com.docker.network.bridge.name'] === scope.iface &&
          value.Labels['io.myapi.b4.sha'] === scope.sha && value.Labels['io.myapi.b4.run'] === scope.run &&
          value.Labels['io.myapi.b4.attempt'] === scope.attempt && value.Labels['io.myapi.b4.role'] === 'network', 'B4_NETWORK_OWNERSHIP_UNKNOWN')
        docker(['network', 'rm', networkId])
        demand(docker(['network', 'ls', '-q', '--filter', `name=^${scope.network}$`]) === '', 'B4_NETWORK_REMOVAL_UNCONFIRMED')
        for (let attempt = 0; attempt < 20 && existsSync(`/sys/class/net/${scope.iface}`); attempt++) await delay(100)
        demand(!existsSync(`/sys/class/net/${scope.iface}`), 'B4_INTERFACE_REMOVAL_UNCONFIRMED')
      }
      for (const rule of rules) if (rule.attempted) {
        const present = firewall('-C', rule)
        if (present === 0) {
          demand(firewall('-D', rule) === 0 && firewall('-C', rule) === 1, 'B4_FIREWALL_CLEANUP_UNCONFIRMED')
        } else demand(!rule.verified, 'B4_ISOLATION_RULE_LOST')
      }
      if (canary?.listening) {
        canary.closeAllConnections()
        await new Promise((resolve, reject) => canary.close(error => error ? reject(error) : resolve()))
      }
      rmSync(scope.root, { recursive: true })
      demand(!existsSync(scope.root), 'B4_DIRECTORY_REMOVAL_UNCONFIRMED')
      cleanupVerified = true
    } catch (error) {
      const cleanupCode = error instanceof Error && /^B4_[A-Z_]+$/.test(error.message) ? error.message : 'B4_OWNED_CLEANUP_UNCONFIRMED'
      if (failure) failure.cleanupFailure = cleanupCode
      else { failure = new Error(cleanupCode); failure.qualificationStage = 'cleanup' }
      // Stop only our in-process listener even when an earlier ownership
      // check prevented further cleanup. Unknown Docker/firewall state stays
      // untouched and failed; the listener must not keep the failed job alive.
      if (canary?.listening) {
        canary.closeAllConnections()
        await new Promise(resolve => canary.close(() => resolve()))
      }
    }
  }
  if (failure) throw failure
  demand(cleanupVerified && backupReceipt?.backup_created === true && restoreReceipt?.restored === true, 'B4_QUALIFICATION_INCOMPLETE')
  return { schema_version: 1, command: 'backup-qualification', source_sha: scope.sha, run_id: scope.run, attempt: scope.attempt,
    qualified: true, source_image_id: scope.appImage, fixture_image_id: scope.bunImage,
    production_cli: true, plan_no_source_effect: true, wrong_digest_rejected: true, repeated_confirmation_rejected: true,
    network_isolation: isolationEvidence, host_published_health: { source: true, recovery: true },
    existing_target_rejected: true, original_target_rejected: true, backup_committed_verified: true, isolated_restore_verified: true,
    cli_did_not_start_recovery: true,
    source_left_stopped: true, original_data_unchanged_after_stop: true, backup_payload_unchanged: true,
    source_tree_sha256: stoppedSourceHash, backup_immutable_sha256: bundleHash,
    source_process: sourceProcess, recovery_process: recoveryProcess,
    bundle_directory_timestamp: 'integrity-check temporary siblings may change the outer timestamp; payload/manifest/commit remain checked',
    auxiliary_wal: { verified_on_separate_copy: true, sha256: auxSHA, primary_wal_policy: 'actual-post-shutdown-state' },
    upstream_requests: { source: 2, recovery: 0, combined: 2, additional: 0 },
    recovery_session_writes: 'allowed only in isolated recovery after metadata comparison',
    retention: { policy: 'keep-all', deletions: 0 }, checks, cleanup_verified: true,
    browser: 'not_run', production_touched: false, real_provider_requests: false }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    demand(process.argv.length === 2, 'B4_ARGUMENTS_REJECTED')
    console.log(JSON.stringify(await runBackupQualification()))
  } catch (error) {
    const code = error instanceof Error && /^(B4|SMOKE)_[A-Z_]+$/.test(error.message) ? error.message : 'B4_QUALIFICATION_FAILED'
    console.log(JSON.stringify({ schema_version: 1, command: 'backup-qualification', source_sha: process.env.GITHUB_SHA ?? null,
      qualified: false, code, stage: error?.qualificationStage ?? null, cli_failure: error?.cliFailure ?? null,
      cleanup_failure: error?.cleanupFailure ?? null }))
    process.exitCode = 1
  }
}

