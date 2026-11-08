import { createHash, randomBytes } from 'node:crypto'
import { execFileSync, fork } from 'node:child_process'
import { once } from 'node:events'
import { constants } from 'node:fs'
import { chmod, copyFile, lstat, mkdir, mkdtemp, open, readFile, realpath, rm, writeFile } from 'node:fs/promises'
import http from 'node:http'
import net from 'node:net'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { createNativeHost } from '../../electron/test/fixtures/native-host.mjs'

const repository = fileURLToPath(new URL('../..', import.meta.url))
const sha256 = value => createHash('sha256').update(value).digest('hex')
const demand = (ok, code) => { if (!ok) throw new Error(code) }
const safeError = error => /^[A-Z][A-Z0-9_]{0,79}$/.test(error?.message) ? error.message : 'DESKTOP_QUALIFICATION_FAILED'
const bounded = (promise, timeout, code) => new Promise((resolve, reject) => {
  const timer = setTimeout(() => reject(new Error(code)), timeout)
  Promise.resolve(promise).then(value => { clearTimeout(timer); resolve(value) }, error => { clearTimeout(timer); reject(error) })
})

export function qualificationScope(env, args, platform = process.platform, arch = process.arch) {
  demand(args.length === 0 && env.MYAPI_DESKTOP_BACKEND_QUALIFICATION === '1' && env.GITHUB_ACTIONS === 'true' &&
    env.GITHUB_EVENT_NAME === 'workflow_dispatch' && env.GITHUB_REF === 'refs/heads/dev' &&
    env.GITHUB_REPOSITORY === 'ForceMind/MyAPI', 'DESKTOP_SCOPE_REJECTED')
  demand(platform === 'darwin' && arch === 'arm64' || platform === 'win32' && arch === 'x64', 'DESKTOP_PLATFORM_REJECTED')
  demand(/^[a-f0-9]{40}$/.test(env.GITHUB_SHA) && /^[1-9][0-9]*$/.test(env.GITHUB_RUN_ID) &&
    /^[1-9][0-9]*$/.test(env.GITHUB_RUN_ATTEMPT) && /^[a-f0-9]{64}$/.test(env.MYAPI_DESKTOP_BINARY_SHA256) &&
    typeof env.RUNNER_TEMP === 'string' && env.RUNNER_TEMP.length <= 4096 && path.isAbsolute(env.RUNNER_TEMP) &&
    !/[\u0000-\u001f\u007f]/.test(env.RUNNER_TEMP), 'DESKTOP_IDENTITY_REJECTED')
  return { source_sha: env.GITHUB_SHA, run_id: env.GITHUB_RUN_ID, run_attempt: env.GITHUB_RUN_ATTEMPT,
    platform, architecture: arch, binary_sha256: env.MYAPI_DESKTOP_BINARY_SHA256 }
}

export async function binaryDigest(filename, expected) {
  demand(/^[a-f0-9]{64}$/.test(expected), 'DESKTOP_BINARY_HASH_INVALID')
  const stat = await lstat(filename)
  demand(stat.isFile() && !stat.isSymbolicLink(), 'DESKTOP_BINARY_TYPE_REJECTED')
  const descriptor = await open(filename, constants.O_RDONLY | (constants.O_NOFOLLOW || 0))
  try {
    const before = await descriptor.stat()
    demand(before.dev === stat.dev && before.ino === stat.ino, 'DESKTOP_BINARY_CHANGED')
    const digest = sha256(await descriptor.readFile())
    const after = await descriptor.stat()
    demand(before.size === after.size && before.mtimeMs === after.mtimeMs && before.ctimeMs === after.ctimeMs &&
      digest === expected, 'DESKTOP_BINARY_CHANGED')
    return digest
  } finally { await descriptor.close() }
}

export async function createOwnedFixture(parent, scope) {
  const actual = await realpath(parent)
  const parentStat = await lstat(actual)
  demand(parentStat.isDirectory() && actual !== path.parse(actual).root &&
    (process.platform === 'win32' || parentStat.uid === process.getuid()), 'DESKTOP_TEMP_REJECTED')
  const root = await mkdtemp(path.join(actual, `myapi-desktop-${scope.source_sha}-${scope.run_id}-${scope.run_attempt}-`))
  await chmod(root, 0o700)
  const ownership = { schema_version: 1, ...scope, nonce: randomBytes(16).toString('hex') }
  await writeFile(path.join(root, 'ownership.json'), JSON.stringify(ownership), { flag: 'wx', mode: 0o600 })
  return { root, ownership }
}

export async function removeOwnedFixture(fixture) {
  const stat = await lstat(fixture.root)
  const marker = path.join(fixture.root, 'ownership.json')
  const markerStat = await lstat(marker)
  demand(stat.isDirectory() && !stat.isSymbolicLink() && await realpath(fixture.root) === fixture.root &&
    markerStat.isFile() && !markerStat.isSymbolicLink() &&
    sha256(await readFile(marker)) === sha256(JSON.stringify(fixture.ownership)), 'DESKTOP_CLEANUP_OWNERSHIP_UNKNOWN')
  await rm(fixture.root, { recursive: true })
}

export function childEnvironment(env) {
  const result = {}
  for (const name of ['PATH', 'SystemRoot', 'WINDIR', 'COMSPEC', 'PATHEXT', 'LANG', 'LC_ALL', 'TZ']) {
    if (typeof env[name] === 'string') result[name] = env[name]
  }
  return result
}

async function availablePort() {
  const server = net.createServer()
  await new Promise((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve) })
  const port = server.address().port
  await new Promise((resolve, reject) => server.close(error => error ? reject(error) : resolve()))
  return port
}

function localAPI(port) {
  let accessToken = null
  return {
    setToken(value) { demand(typeof value === 'string' && value.length > 0, 'DESKTOP_AUTH_TOKEN_MISSING'); accessToken = value },
    request(method, pathname, body) {
      demand((method === 'GET' && ['/api/status', '/api/user/self'].includes(pathname)) ||
        (method === 'POST' && ['/api/setup', '/api/user/login'].includes(pathname)) ||
        (method === 'PUT' && pathname === '/api/user/self'), 'DESKTOP_API_SCOPE_REJECTED')
      const payload = body === undefined ? null : Buffer.from(JSON.stringify(body))
      return new Promise((resolve, reject) => {
        const request = http.request({ hostname: '127.0.0.1', port, method, path: pathname, timeout: 10000,
          headers: { ...(payload ? { 'Content-Type': 'application/json', 'Content-Length': payload.length } : {}),
            ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}) } }, response => {
          const chunks = []; let size = 0
          response.on('data', chunk => { size += chunk.length; if (size > 1024 * 1024) response.destroy(new Error('DESKTOP_API_RESPONSE_TOO_LARGE')); else chunks.push(chunk) })
          response.on('error', () => reject(new Error('DESKTOP_API_READ_FAILED')))
          response.on('end', () => {
            try {
              const value = JSON.parse(Buffer.concat(chunks))
              demand(response.statusCode === 200 && value.success === true, 'DESKTOP_API_REJECTED')
              resolve(value.data ?? null)
            } catch { reject(new Error('DESKTOP_API_REJECTED')) }
          })
        })
        request.on('timeout', () => request.destroy(new Error('DESKTOP_API_TIMEOUT')))
        request.on('error', () => reject(new Error('DESKTOP_API_CONNECTION_FAILED')))
        request.end(payload)
      })
    },
  }
}

function businessProjection(value) {
  const keys = ['id', 'username', 'display_name', 'role', 'status', 'group', 'quota', 'used_quota', 'request_count', 'setting']
  demand(value && keys.every(key => Object.hasOwn(value, key)), 'DESKTOP_BUSINESS_FIELDS_MISSING')
  demand(Number.isSafeInteger(value.id) && value.id > 0 && value.username === 'desktopqa' && value.role === 100 &&
    value.status === 1 && value.quota === 0 && value.used_quota === 0 && value.request_count === 0, 'DESKTOP_BUSINESS_MISMATCH')
  demand(value.display_name === 'Desktop QA marker' && typeof value.setting === 'string', 'DESKTOP_MARKER_MISSING')
  return Object.fromEntries(keys.map(key => [key, value[key]]))
}

async function closeOwned(child, closedPromise) {
  if (child.exitCode !== null || child.signalCode !== null) { await bounded(closedPromise, 5000, 'DESKTOP_CHILD_CLOSE_UNKNOWN'); return }
  child.kill('SIGTERM')
  try { await bounded(closedPromise, 10000, 'DESKTOP_CLEANUP_TERM_TIMEOUT') }
  catch { child.kill('SIGKILL'); await bounded(closedPromise, 5000, 'DESKTOP_CLEANUP_CLOSE_UNKNOWN') }
}

export async function runQualification(env = process.env, args = process.argv.slice(2)) {
  const report = { schema_version: 1, source_sha: null, run_id: null, run_attempt: null, platform: process.platform,
    architecture: process.arch, binary_sha256: null, passed: false, phase: 'scope', error_code: null, checks: {}, lifecycle: [],
    qualified_scope: 'owned_backend_lifecycle_and_persistence', graceful_stop_verified: false,
    termination_semantics: 'not_verified', accounting_verified: false,
    cleanup: { confirmed: false, owned_children_closed: false, fixture_removed: false },
    electron_started: false, gui_verified: false, model_requests: 0, production_touched: false }
  let fixture = null, host = null, canary = null, canaryClosed = null
  try {
    const scope = qualificationScope(env, args)
    Object.assign(report, scope)
    report.phase = 'source'
    const git = (...parts) => execFileSync('git', ['-c', 'core.fileMode=false', ...parts], { cwd: repository, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim()
    demand(git('rev-parse', 'HEAD') === scope.source_sha && git('status', '--porcelain', '--untracked-files=no') === '', 'DESKTOP_SOURCE_IDENTITY_MISMATCH')
    const version = (await readFile(path.join(repository, 'VERSION'), 'utf8')).trim()
    demand(/^\d+\.\d+\.\d+(?:[.-][0-9A-Za-z.-]+)?$/.test(version), 'DESKTOP_VERSION_INVALID')
    const binaryName = process.platform === 'win32' ? 'my-api.exe' : 'my-api'
    const sourceBinary = path.join(repository, binaryName)
    await binaryDigest(sourceBinary, scope.binary_sha256)
    report.checks.source_and_binary_bound = true
    fixture = await createOwnedFixture(env.RUNNER_TEMP, scope)
    const resources = path.join(fixture.root, 'resources space 资源')
    const userData = path.join(fixture.root, 'profile space 用户')
    await mkdir(path.join(resources, 'bin'), { recursive: true, mode: 0o700 })
    await mkdir(userData, { mode: 0o700 })
    const binary = path.join(resources, 'bin', binaryName)
    await copyFile(sourceBinary, binary, constants.COPYFILE_EXCL)
    if (process.platform !== 'win32') await chmod(binary, 0o700)
    await binaryDigest(binary, scope.binary_sha256)
    const safeEnv = childEnvironment(env)
    Object.assign(safeEnv, { TMPDIR: fixture.root, TMP: fixture.root, TEMP: fixture.root })
    canary = fork(path.join(repository, 'electron/test/fixtures/service-child.mjs'), [], { cwd: fixture.root,
      env: { ...safeEnv, MYAPI_DESKTOP_CHILD_FIXTURE: '1' }, execArgv: [], stdio: ['ignore', 'ignore', 'ignore', 'ipc'] })
    canaryClosed = once(canary, 'close')
    const canaryMessage = await bounded(once(canary, 'message'), 10000, 'DESKTOP_CANARY_UNREADY')
    demand(canaryMessage[0]?.ready === true, 'DESKTOP_CANARY_UNREADY')
    const port = await availablePort()
    host = await createNativeHost({ electronDirectory: path.join(repository, 'electron'), resources, userData,
      env: safeEnv, version, port })
    report.phase = 'start'
    const ready = await host.waitForState('running')
    demand(host.children.length === 1 && ready.owned === true && ready.generation === 1 &&
      Number.isSafeInteger(host.children[0].pid) && host.children[0].pid > 0 &&
      ready.addresses.length === 1 && ready.addresses[0] === `http://127.0.0.1:${port}/v1`, 'DESKTOP_START_IDENTITY_MISMATCH')
    const api = localAPI(port)
    const status = await api.request('GET', '/api/status')
    demand(status.product_edition === 'lite' && status.version === version && status.setup === false, 'DESKTOP_RUNTIME_IDENTITY_MISMATCH')
    report.checks.real_lite_status = true
    report.phase = 'configure'
    const password = 'Qa-' + randomBytes(24).toString('hex')
    const login = async () => {
      const value = await api.request('POST', '/api/user/login', { username: 'desktopqa', password })
      api.setToken(value?.access_token)
    }
    await api.request('POST', '/api/setup', { username: 'desktopqa', password, confirmPassword: password, SelfUseModeEnabled: true, DemoSiteEnabled: false })
    await login()
    await api.request('PUT', '/api/user/self', { username: 'desktopqa', display_name: 'Desktop QA marker' })
    const before = businessProjection(await api.request('GET', '/api/user/self'))
    const secretPath = path.join(userData, 'myapi-session-secret')
    const secret = await readFile(secretPath)
    demand(/^[a-f0-9]{64}\n$/.test(secret.toString()), 'DESKTOP_SECRET_INVALID')
    const database = await lstat(path.join(userData, 'data', 'my-api.db'))
    demand(database.isFile() && !database.isSymbolicLink() && database.size > 0, 'DESKTOP_DATABASE_MISSING')
    const secretStat = await lstat(secretPath)
    const profileStat = await lstat(userData)
    demand(secretStat.isFile() && !secretStat.isSymbolicLink() && profileStat.isDirectory() &&
      !profileStat.isSymbolicLink() && await realpath(userData) === userData &&
      (process.platform === 'win32' || profileStat.uid === process.getuid() && (profileStat.mode & 0o777) === 0o700 &&
        secretStat.uid === process.getuid() && (secretStat.mode & 0o777) === 0o600), 'DESKTOP_FILESYSTEM_IDENTITY_MISMATCH')
    report.checks.owned_filesystem_bound = true
    report.checks.local_configuration_saved = true
    const first = host.children[0]
    host.panel().close()
    const hidden = await host.invoke('snapshot')
    demand(!host.panel().isVisible() && hidden.state === 'running' && !first.closed && first.child.pid === first.pid, 'DESKTOP_WINDOW_CLOSE_CHANGED_SERVICE')
    const cancelled = await host.invoke('stop') // The facade defaults to Cancel.
    demand(cancelled.state === 'running' && first.requested_signals.length === 0 && !first.closed && host.exits === 0, 'DESKTOP_CANCEL_CHANGED_SERVICE')
    report.checks.close_and_cancel_preserve_process = true
    report.phase = 'stop'
    host.answers.push(1)
    const stopped = await bounded(host.invoke('stop'), 15000, 'DESKTOP_STOP_TIMEOUT')
    await bounded(first.closedPromise, 5000, 'DESKTOP_STOP_CLOSE_UNKNOWN')
    demand(stopped.state === 'stopped' && stopped.owned === false && first.closed && host.exits === 0 &&
      (Number.isInteger(first.exit_code) || typeof first.signal_code === 'string') &&
      !first.requested_signals.includes('SIGKILL'), 'DESKTOP_STOP_UNCONFIRMED')
    demand(canary.exitCode === null && canary.signalCode === null && canary.connected, 'DESKTOP_UNRELATED_CHILD_STOPPED')
    if (process.platform === 'darwin') demand(first.exit_code === 0 && first.signal_code === null, 'DESKTOP_STOP_NOT_GRACEFUL')
    report.checks.owned_stop_confirmed = true
    report.checks.unrelated_child_preserved = true
    report.phase = 'restart'
    host.answers.push(1)
    await host.invoke('start')
    const restarted = await host.waitForState('running')
    demand(host.children.length === 2 && restarted.generation === 2 &&
      Number.isSafeInteger(host.children[1].pid) && host.children[1].pid > 0, 'DESKTOP_RESTART_IDENTITY_MISMATCH')
    const afterStatus = await api.request('GET', '/api/status')
    demand(afterStatus.product_edition === 'lite' && afterStatus.version === version && afterStatus.setup === true, 'DESKTOP_RESTART_SETUP_CHANGED')
    // The pre-stop authenticated session must still work with the same secret.
    demand(JSON.stringify(businessProjection(await api.request('GET', '/api/user/self'))) === JSON.stringify(before) &&
      Buffer.compare(secret, await readFile(secretPath)) === 0, 'DESKTOP_PERSISTENCE_CHANGED')
    await login()
    demand(JSON.stringify(businessProjection(await api.request('GET', '/api/user/self'))) === JSON.stringify(before), 'DESKTOP_RELOGIN_CHANGED')
    await binaryDigest(binary, scope.binary_sha256)
    report.checks.restart_business_and_secret_preserved = true
    report.phase = 'quit'
    host.answers.push(1)
    await bounded(host.invoke('quit'), 15000, 'DESKTOP_QUIT_TIMEOUT')
    const second = host.children[1]
    await bounded(second.closedPromise, 5000, 'DESKTOP_QUIT_CLOSE_UNKNOWN')
    demand(host.exits === 1 && second.closed && !second.requested_signals.includes('SIGKILL') &&
      (Number.isInteger(second.exit_code) || typeof second.signal_code === 'string') &&
      canary.exitCode === null && canary.signalCode === null, 'DESKTOP_QUIT_UNCONFIRMED')
    if (process.platform === 'darwin') demand(second.exit_code === 0 && second.signal_code === null, 'DESKTOP_QUIT_NOT_GRACEFUL')
    report.checks.quit_waited_for_close = true
    report.graceful_stop_verified = process.platform === 'darwin'
    report.termination_semantics = process.platform === 'darwin' ? 'normal_exit_zero_without_sigkill' : 'windows_graceful_flush_not_verified'
    report.phase = 'completed'
    report.passed = true
  } catch (error) { report.error_code = safeError(error) }
  finally {
    try {
      for (const record of host?.children ?? []) await closeOwned(record.child, record.closedPromise)
      if (canary) {
        if (canary.exitCode === null && canary.signalCode === null && canary.connected) canary.send('exit')
        await bounded(canaryClosed, 5000, 'DESKTOP_CANARY_CLOSE_UNKNOWN')
      }
      report.cleanup.owned_children_closed = true
      if (fixture) await removeOwnedFixture(fixture)
      report.cleanup.fixture_removed = true
      report.cleanup.confirmed = true
    } catch (error) { report.passed = false; report.error_code ??= safeError(error) }
    report.lifecycle = (host?.children ?? []).map(({ generation, pid, requested_signals, exit_code, signal_code, closed }) =>
      ({ generation, pid, requested_signals, exit_code, signal_code, closed }))
  }
  return report
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const report = await runQualification()
  process.stdout.write(JSON.stringify(report) + '\n')
  process.exitCode = report.passed ? 0 : 1
}
