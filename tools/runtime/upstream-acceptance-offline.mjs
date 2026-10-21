import { randomBytes } from 'node:crypto'
import { constants, fstatSync, lstatSync, mkdtempSync, openSync, closeSync, readFileSync, readdirSync,
  realpathSync, renameSync, rmSync, writeFileSync, chmodSync } from 'node:fs'
import path from 'node:path'
import { pathToFileURL } from 'node:url'
import { acceptanceControl } from './upstream-acceptance.mjs'

// Synthetic-only preparation against an operator-owned, fresh offline server.
// This does not launch a server, read credentials, open egress or send inference.
function offlineOrigin(value) {
  let url
  try { url = new URL(value) } catch { throw new Error('OFFLINE_TARGET_REJECTED') }
  if (url.protocol !== 'http:' || !['127.0.0.1', '[::1]'].includes(url.hostname) || !url.port ||
      url.username || url.password || url.pathname !== '/' || url.search || url.hash) {
    throw new Error('OFFLINE_TARGET_REJECTED')
  }
  return url.origin
}

function privateDirectory(directory) {
  const resolved = path.resolve(directory)
  const stat = lstatSync(resolved)
  if (realpathSync(resolved) !== resolved || !stat.isDirectory() || stat.isSymbolicLink() ||
      (stat.mode & 0o777) !== 0o700 || stat.uid !== process.getuid() ||
      !path.basename(resolved).startsWith('myapi-offline-')) throw new Error('OFFLINE_DIRECTORY_REJECTED')
  return resolved
}

function saveState(directory, state) {
  privateDirectory(directory)
  const pending = path.join(directory, 'state.pending')
  writeFileSync(pending, JSON.stringify(state), { mode: 0o600, flag: 'wx' })
  renameSync(pending, path.join(directory, 'state.json'))
}

function loadState(directory) {
  const filename = path.join(directory, 'state.json')
  const fd = openSync(filename, constants.O_RDONLY | constants.O_NOFOLLOW)
  try {
    const stat = fstatSync(fd)
    if (!stat.isFile() || stat.nlink !== 1 || stat.uid !== process.getuid() ||
        (stat.mode & 0o777) !== 0o600 || stat.size > 16_384) throw new Error('OFFLINE_STATE_REJECTED')
    const state = JSON.parse(readFileSync(fd, 'utf8'))
    if (state.schema !== 1 || state.synthetic_only !== true || state.directory !== directory ||
        !/^[a-f0-9]{10}$/.test(state.nonce) || !['prepared', 'cleanup_pending'].includes(state.phase) ||
        ![state.user_id, state.channel_id, state.token_id].every(id => Number.isSafeInteger(id) && id > 0) ||
        !/^[A-Za-z0-9:_-]{1,64}$/.test(state.run_id) || typeof state.access_token !== 'string' ||
        !state.access_token || !state.sid) throw new Error('OFFLINE_STATE_REJECTED')
    offlineOrigin(state.base_url)
    return state
  } finally { closeSync(fd) }
}

// Every request has a literal loopback origin, bounded lifetime and no redirects.
// Errors deliberately exclude URLs, server messages, bodies, credentials and causes.
class OfflineAPI {
  constructor(origin, fetchImpl) { this.origin = origin; this.fetch = fetchImpl; this.token = ''; this.sid = '' }
  async request(method, pathname, body, expectedFailure = false) {
    let response
    let payload
    try {
      response = await this.fetch(new URL(pathname, this.origin), {
        method, redirect: 'error', signal: AbortSignal.timeout(10_000),
        headers: { Origin: this.origin, 'Content-Type': 'application/json', 'Accept-Encoding': 'identity',
          ...(this.token ? { Authorization: `Bearer ${this.token}`, 'X-Auth-Session': this.sid } : {}) },
        ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      })
      // Consume at most 1 MiB, including error responses.
      const reader = response.body.getReader()
      const chunks = []
      let size = 0
      try {
        for (;;) {
          const { done, value } = await reader.read()
          if (done) break
          size += value.length
          if (size > 1_048_576) throw new Error('oversize')
          chunks.push(value)
        }
        payload = JSON.parse(Buffer.concat(chunks).toString('utf8'))
      } finally { await reader.cancel() }
    } catch { throw new Error('OFFLINE_HTTP_FAILED') }
    if (expectedFailure === 'acceptance-probe') {
      // A random missing run returns 409 only after the real Root-session and
      // enabled-mode guards. Disabled mode returns 403. This is read-only.
      if (response.status !== 409 || payload?.success !== false || payload.code !== 'upstream_acceptance_denied') {
        throw new Error('OFFLINE_ACCEPTANCE_MODE_REQUIRED')
      }
      return
    }
    if (expectedFailure) {
      if (response.status !== 401 && response.status !== 403) throw new Error('OFFLINE_LOGOUT_NOT_VERIFIED')
      return
    }
    if (!response.ok || payload?.success !== true) throw new Error('OFFLINE_API_REFUSED')
    return payload.data
  }
}

export async function prepareOffline({ baseURL, parentDir, isolated = false, synthetic = false, fetchImpl = fetch } = {}) {
  const origin = offlineOrigin(baseURL)
  if (isolated !== true || synthetic !== true || typeof fetchImpl !== 'function') throw new Error('OFFLINE_OPT_IN_REQUIRED')
  const parent = path.resolve(parentDir)
  if (realpathSync(parent) !== parent || !lstatSync(parent).isDirectory()) throw new Error('OFFLINE_PARENT_REJECTED')
  const api = new OfflineAPI(origin, fetchImpl)
  const setup = await api.request('GET', '/api/setup')
  if (setup?.status !== false || setup?.root_init !== false) throw new Error('OFFLINE_SERVER_NOT_FRESH')
  const directory = mkdtempSync(path.join(parent, 'myapi-offline-'))
  chmodSync(directory, 0o700)
  const nonce = randomBytes(5).toString('hex')
  const state = { schema: 1, synthetic_only: true, directory, base_url: origin, nonce, phase: 'setup_pending' }
  saveState(directory, state)
  try {
    const username = `at${nonce}`
    let password = randomBytes(15).toString('base64url')
    await api.request('POST', '/api/setup', { username, password, confirmPassword: password,
      SelfUseModeEnabled: true, DemoSiteEnabled: false })
    state.phase = 'login_pending'; saveState(directory, state)
    const login = await api.request('POST', '/api/user/login', { username, password })
    password = undefined
    if (login?.user?.role !== 100 || login.user.username !== username || !login.access_token || !login.session?.sid) {
      throw new Error('OFFLINE_ROOT_SESSION_REQUIRED')
    }
    state.user_id = login.user.id; state.access_token = login.access_token; state.sid = login.session.sid
    api.token = state.access_token; api.sid = state.sid
    state.phase = 'mode_pending'; saveState(directory, state)
    writeFileSync(path.join(directory, 'owner-session.json'), JSON.stringify({ access_token: state.access_token }),
      { mode: 0o600, flag: 'wx' })
    await api.request('GET', `/api/upstream-acceptance/runs/offline-probe-${nonce}`, undefined, 'acceptance-probe')
    state.phase = 'channel_pending'; saveState(directory, state)
    // These are generated here, never imported. No CLI accepts real credentials.
    const key = { access_token: `synthetic-offline-access-${nonce}`, refresh_token: `synthetic-offline-refresh-${nonce}`,
      account_id: `synthetic-offline-account-${nonce}`, type: 'codex' }
    const created = await api.request('POST', '/api/channel/', { mode: 'single',
      channel: { type: 57, name: `offline-${nonce}`, status: 1, base_url: 'https://chatgpt.com',
        key: JSON.stringify(key), models: 'synthetic-offline-model', group: 'default' } })
    if (!Array.isArray(created?.ids) || created.ids.length !== 1 || !Number.isSafeInteger(created.ids[0]) || created.ids[0] < 0) {
      throw new Error('OFFLINE_CHANNEL_RESULT_UNKNOWN')
    }
    // The current batch-create API can return ids:[0] after a committed insert.
    // Read back the exact unique synthetic name; never retry the write or bind 0.
    const channels = await api.request('GET', `/api/channel/search?keyword=offline-${nonce}&p=1&page_size=10`)
    const channel = channels?.items?.[0]
    if (channels?.total !== 1 || channels.items.length !== 1 || channel.name !== `offline-${nonce}` ||
        channel.type !== 57 || !Number.isSafeInteger(channel.id) || channel.id < 1 ||
        (created.ids[0] !== 0 && created.ids[0] !== channel.id)) throw new Error('OFFLINE_CHANNEL_RESULT_UNKNOWN')
    state.channel_id = channel.id; state.phase = 'token_pending'; saveState(directory, state)
    await api.request('POST', '/api/token/', { name: `offline-${nonce}`, expired_time: Math.floor(Date.now() / 1000) + 1200,
      remain_quota: 0, unlimited_quota: false, group: 'default', model_limits_enabled: true,
      model_limits: 'synthetic-offline-model', cross_group_retry: false })
    const tokens = await api.request('GET', `/api/token/search?keyword=offline-${nonce}&p=1&page_size=10`)
    if (tokens?.total !== 1 || tokens.items?.length !== 1 || tokens.items[0].name !== `offline-${nonce}`) {
      throw new Error('OFFLINE_TOKEN_RESULT_UNKNOWN')
    }
    state.token_id = tokens.items[0].id; state.phase = 'run_pending'; saveState(directory, state)
    const run = await acceptanceControl({ baseURL: origin, accessToken: state.access_token, action: 'create',
      spec: { channel_id: state.channel_id, limits: { duration_seconds: 1200, max_total: 1,
        max_auth: 0, max_refresh: 0, max_sample: 0, max_models: 0, max_generation: 0 } }, fetchImpl })
    if (!/^[A-Za-z0-9:_-]{1,64}$/.test(run?.id) || run.total !== 0) throw new Error('OFFLINE_RUN_RESULT_UNKNOWN')
    state.run_id = run.id; state.phase = 'prepared'; saveState(directory, state)
    return { success: true, synthetic_only: true, state_dir: directory, channel_id: state.channel_id,
      token_id: state.token_id, run_id: state.run_id, all_dispatch_categories_disabled: true }
  } catch (error) {
    // A timed out write may have committed. Preserve the private state and never
    // retry, log in again, create another run or erase evidence automatically.
    const reason = /^OFFLINE_[A-Z_]+$/.test(error.message) ? error.message : 'LOCAL_IO_FAILED'
    throw new Error(`OFFLINE_PREPARE_INCOMPLETE_${state.phase.toUpperCase()}_${reason}:${directory}`)
  }
}

export async function cleanupOffline({ stateDir, fetchImpl = fetch } = {}) {
  const directory = privateDirectory(stateDir)
  if (readdirSync(directory).some(name => !['state.json', 'owner-session.json'].includes(name))) {
    throw new Error('OFFLINE_UNOWNED_FILES_PRESENT')
  }
  const state = loadState(directory)
  const api = new OfflineAPI(state.base_url, fetchImpl)
  api.token = state.access_token; api.sid = state.sid
  const user = await api.request('GET', '/api/user/self')
  const token = await api.request('GET', `/api/token/${state.token_id}`)
  const channel = await api.request('GET', `/api/channel/${state.channel_id}`)
  if (user?.id !== state.user_id || user.username !== `at${state.nonce}` || user.role !== 100 ||
      token?.id !== state.token_id || token.name !== `offline-${state.nonce}` || token.used_quota !== 0 ||
      channel?.id !== state.channel_id || channel.name !== `offline-${state.nonce}`) throw new Error('OFFLINE_IDENTITY_MISMATCH')
  const before = await acceptanceControl({ baseURL: state.base_url, accessToken: state.access_token,
    action: 'status', runID: state.run_id, fetchImpl })
  if (before?.owner_current !== true || before.run?.channel_id !== state.channel_id || before.run.total !== 0 ||
      ['max_auth', 'max_refresh', 'max_sample', 'max_models', 'max_generation'].some(key => before.run[key] !== 0)) {
    throw new Error('OFFLINE_RUN_SCOPE_MISMATCH')
  }
  state.phase = 'cleanup_pending'; saveState(directory, state)
  const status = await acceptanceControl({ baseURL: state.base_url, accessToken: state.access_token,
    action: 'stop', runID: state.run_id, fetchImpl })
  if (!['stopped', 'expired'].includes(status?.run?.status) || status.owner_current !== true ||
      status.run.channel_id !== state.channel_id || status.run.total !== 0 || status.operations !== 0 ||
      status.http_in_flight !== 0 || status.run.generation_in_flight !== 0 || status.drain_error !== false) {
    throw new Error('OFFLINE_NOT_DRAINED')
  }
  await api.request('PUT', '/api/token/?status_only=true', { id: state.token_id, status: 2 })
  const disabled = await api.request('GET', `/api/token/${state.token_id}`)
  if (disabled?.status !== 2) throw new Error('OFFLINE_KEY_NOT_DISABLED')
  const logout = await api.request('POST', '/api/user/auth/logout')
  if (logout?.revoked_sid !== state.sid) throw new Error('OFFLINE_LOGOUT_NOT_VERIFIED')
  await api.request('GET', '/api/user/self', undefined, true)
  // Never remove the server's database or process: the fixture/operator owns
  // those. Only this script's known private files are removed after revocation.
  privateDirectory(directory)
  if (readdirSync(directory).some(name => !['state.json', 'owner-session.json'].includes(name))) {
    throw new Error('OFFLINE_UNOWNED_FILES_PRESENT')
  }
  rmSync(directory, { recursive: true })
  return { success: true, synthetic_only: true, run_drained: true, key_disabled: true,
    session_revoked: true, private_files_removed: true, server_database_removed: false }
}

async function main() {
  const [action, ...args] = process.argv.slice(2)
  const options = {}
  for (let i = 0; i < args.length; i++) {
    const name = args[i]
    if (options[name] !== undefined || !['--base-url', '--parent-dir', '--state-dir', '--isolated', '--synthetic'].includes(name)) {
      throw new Error('OFFLINE_ARGUMENTS_REJECTED')
    }
    if (['--isolated', '--synthetic'].includes(name)) options[name] = true
    else {
      if (!args[i + 1] || args[i + 1].startsWith('--')) throw new Error('OFFLINE_ARGUMENTS_REJECTED')
      options[name] = args[++i]
    }
  }
  let result
  if (action === 'prepare' && options['--state-dir'] === undefined) {
    result = await prepareOffline({ baseURL: options['--base-url'], parentDir: options['--parent-dir'],
      isolated: options['--isolated'], synthetic: options['--synthetic'] })
  } else if (action === 'cleanup' && Object.keys(options).length === 1 && options['--state-dir']) {
    result = await cleanupOffline({ stateDir: options['--state-dir'] })
  } else throw new Error('OFFLINE_ARGUMENTS_REJECTED')
  process.stdout.write(`${JSON.stringify(result)}\n`)
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch(error => {
    const message = /^OFFLINE_[A-Z_]+(?::\/[^\r\n]*)?$/.test(error.message) ? error.message : 'OFFLINE_FAILED'
    process.stderr.write(`${message}\n`)
    process.exitCode = 1
  })
}
