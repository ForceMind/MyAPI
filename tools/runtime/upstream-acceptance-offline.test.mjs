import test from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, rmSync, readFileSync, writeFileSync, statSync, readdirSync, symlinkSync, chmodSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { prepareOffline, cleanupOffline } from './upstream-acceptance-offline.mjs'

function fixture(t, override) {
  const parentDir = mkdtempSync(path.join(os.tmpdir(), 'offline-unit-'))
  t.after(() => rmSync(parentDir, { recursive: true, force: true }))
  const requests = []
  let username
  let password
  let name
  let disabled = false
  let revoked = false
  const fetchImpl = async (url, options) => {
    const parsed = new URL(url)
    const body = options.body ? JSON.parse(options.body) : undefined
    const request = { pathname: parsed.pathname, method: options.method, body, options }
    requests.push(request)
    assert.equal(parsed.origin, 'http://127.0.0.1:4181')
    assert.equal(options.redirect, 'error')
    assert.equal(options.headers.Origin, parsed.origin)
    const special = override?.(request)
    if (special) return special
    let data
    if (parsed.pathname === '/api/setup' && options.method === 'GET') data = { status: false, root_init: false }
    else if (parsed.pathname === '/api/setup') {
      username = body.username; password = body.password
      assert.match(username, /^at[a-f0-9]{10}$/)
      assert.match(password, /^[A-Za-z0-9_-]{20}$/)
      assert.equal(password, body.confirmPassword)
    } else if (parsed.pathname === '/api/user/login') {
      assert.equal(body.username, username); assert.equal(body.password, password)
      data = { user: { id: 1, role: 100, username }, access_token: 'synthetic-root-session', session: { sid: 'synthetic-sid' } }
    } else {
      assert.equal(options.headers.Authorization, 'Bearer synthetic-root-session')
      if (parsed.pathname.startsWith('/api/upstream-acceptance/runs/offline-probe-')) {
        return Response.json({ success: false, code: 'upstream_acceptance_denied' }, { status: 409 })
      } else if (parsed.pathname === '/api/channel/' && options.method === 'POST') {
        name = body.channel.name
        assert.equal(body.mode, 'single')
        const key = JSON.parse(body.channel.key)
        assert.match(key.access_token, /^synthetic-offline-access-/)
        assert.match(key.refresh_token, /^synthetic-offline-refresh-/)
        data = { ids: [7] }
      } else if (parsed.pathname === '/api/channel/search') data = { total: 1, items: [{ id: 7, name, type: 57 }] }
      else if (parsed.pathname === '/api/token/search') data = { total: 1, items: [{ id: 9, name }] }
      else if (parsed.pathname === '/api/upstream-acceptance/runs') {
        assert.deepEqual(body.limits, { duration_seconds: 1200, max_total: 1, max_auth: 0,
          max_refresh: 0, max_sample: 0, max_models: 0, max_generation: 0 })
        data = { id: 'synthetic-run', total: 0 }
      } else if (parsed.pathname === '/api/upstream-acceptance/runs/synthetic-run') {
        data = { owner_current: true, run: { channel_id: 7, total: 0, max_auth: 0,
          max_refresh: 0, max_sample: 0, max_models: 0, max_generation: 0 } }
      } else if (parsed.pathname.endsWith('/stop')) data = { run: { status: 'stopped', channel_id: 7, total: 0,
        generation_in_flight: 0 }, owner_current: true, operations: 0, http_in_flight: 0, drain_error: false }
      else if (parsed.pathname === '/api/token/9') data = { id: 9, name, used_quota: 0, status: disabled ? 2 : 1 }
      else if (parsed.pathname === '/api/channel/7') data = { id: 7, name }
      else if (parsed.pathname === '/api/user/self') {
        if (revoked) return Response.json({ success: false }, { status: 401 })
        data = { id: 1, username, role: 100 }
      } else if (parsed.pathname === '/api/user/auth/logout') { revoked = true; data = { revoked_sid: 'synthetic-sid' } }
      else if (parsed.pathname === '/api/token/' && options.method === 'PUT') disabled = true
      else assert.equal(parsed.pathname, '/api/token/')
    }
    return Response.json({ success: true, ...(data === undefined ? {} : { data }) })
  }
  return { parentDir, requests, fetchImpl, baseURL: 'http://127.0.0.1:4181', isolated: true, synthetic: true }
}

test('rejects remote or ambiguous targets and missing opt-in before any request or file creation', async t => {
  const options = fixture(t)
  for (const baseURL of ['https://example.com', 'http://localhost:4181', 'http://127.0.0.1:4181/path',
    'http://user:secret@127.0.0.1:4181', 'http://127.0.0.1']) {
    await assert.rejects(prepareOffline({ ...options, baseURL }), /OFFLINE_TARGET_REJECTED/)
  }
  await assert.rejects(prepareOffline({ ...options, synthetic: false }), /OFFLINE_OPT_IN_REQUIRED/)
  assert.equal(options.requests.length, 0)
  assert.deepEqual(readdirSync(options.parentDir), [])
})

test('refuses an initialized server before generating an account or writing private files', async t => {
  const options = fixture(t, () => Response.json({ success: true, data: { status: true } }))
  await assert.rejects(prepareOffline(options), /OFFLINE_SERVER_NOT_FRESH/)
  assert.equal(options.requests.length, 1)
  assert.deepEqual(readdirSync(options.parentDir), [])
})

test('generates a local Root and synthetic channel, disables every dispatch category and cleans only owned files', async t => {
  const options = fixture(t)
  const sentinel = path.join(options.parentDir, 'keep.txt')
  writeFileSync(sentinel, 'preserve')
  const result = await prepareOffline(options)
  assert.equal(result.all_dispatch_categories_disabled, true)
  assert.equal(statSync(result.state_dir).mode & 0o777, 0o700)
  assert.equal(statSync(path.join(result.state_dir, 'owner-session.json')).mode & 0o777, 0o600)
  const rawState = readFileSync(path.join(result.state_dir, 'state.json'), 'utf8')
  const setup = options.requests.find(r => r.method === 'POST' && r.pathname === '/api/setup').body
  assert.equal(rawState.includes(setup.password), false)
  assert.equal(JSON.stringify(result).includes('synthetic-root-session'), false)
  const cleaned = await cleanupOffline({ stateDir: result.state_dir, fetchImpl: options.fetchImpl })
  assert.equal(cleaned.session_revoked, true)
  assert.equal(cleaned.server_database_removed, false)
  assert.deepEqual(readdirSync(options.parentDir), ['keep.txt'])
  assert.equal(readFileSync(sentinel, 'utf8'), 'preserve')
  assert.equal(options.requests.some(r => /oauth|refresh|usage|fetch_models|responses/.test(r.pathname)), false)
})

test('an unknown channel write is not retried and retains recovery state without leaking server errors', async t => {
  const options = fixture(t, r => {
    if (r.pathname === '/api/channel/') throw new Error('SECRET-server-body-and-token')
  })
  await assert.rejects(prepareOffline(options), error => {
    assert.match(error.message, /^OFFLINE_PREPARE_INCOMPLETE_CHANNEL_PENDING_OFFLINE_HTTP_FAILED:/)
    assert.equal(error.message.includes('SECRET'), false)
    return true
  })
  assert.equal(options.requests.filter(r => r.pathname === '/api/channel/').length, 1)
  const directory = path.join(options.parentDir, readdirSync(options.parentDir)[0])
  assert.equal(JSON.parse(readFileSync(path.join(directory, 'state.json'))).phase, 'channel_pending')
  await assert.rejects(cleanupOffline({ stateDir: directory, fetchImpl: options.fetchImpl }), /OFFLINE_STATE_REJECTED/)
})

test('stopping with an outstanding operation preserves credentials and skips key mutation and logout', async t => {
  const options = fixture(t, r => r.pathname.endsWith('/stop') ? Response.json({ success: true,
    data: { run: { status: 'stopping' }, operations: 1 } }) : undefined)
  const result = await prepareOffline(options)
  await assert.rejects(cleanupOffline({ stateDir: result.state_dir, fetchImpl: options.fetchImpl }), /OFFLINE_NOT_DRAINED/)
  assert.equal(statSync(path.join(result.state_dir, 'owner-session.json')).mode & 0o777, 0o600)
  assert.equal(options.requests.some(r => r.method === 'PUT' || r.pathname.endsWith('/logout')), false)
})

test('foreign files, symlinked state and changed identities refuse cleanup before destructive writes', async t => {
  const options = fixture(t)
  const result = await prepareOffline(options)
  const foreign = path.join(result.state_dir, 'foreign')
  writeFileSync(foreign, 'keep')
  let requests = options.requests.length
  await assert.rejects(cleanupOffline({ stateDir: result.state_dir, fetchImpl: options.fetchImpl }), /OFFLINE_UNOWNED_FILES_PRESENT/)
  assert.equal(options.requests.length, requests)
  rmSync(foreign)
  const state = path.join(result.state_dir, 'state.json')
  const content = readFileSync(state)
  rmSync(state)
  const outside = path.join(options.parentDir, 'outside.json')
  writeFileSync(outside, content, { mode: 0o600 })
  symlinkSync(outside, state)
  await assert.rejects(cleanupOffline({ stateDir: result.state_dir, fetchImpl: options.fetchImpl }))
  assert.equal(options.requests.length, requests)
  rmSync(state)
  const changed = JSON.parse(content); changed.user_id = 2
  writeFileSync(state, JSON.stringify(changed), { mode: 0o600 })
  await assert.rejects(cleanupOffline({ stateDir: result.state_dir, fetchImpl: options.fetchImpl }), /OFFLINE_IDENTITY_MISMATCH/)
  assert.equal(options.requests.slice(requests).some(r => r.method !== 'GET'), false)
})

test('a committed channel returning ID zero is read back exactly once without repeating creation', async t => {
  let name
  const options = fixture(t, r => {
    if (r.method === 'POST' && r.pathname === '/api/channel/') {
      name = r.body.channel.name
      return Response.json({ success: true, data: { ids: [0] } })
    }
    if (r.pathname === '/api/channel/search') return Response.json({ success: true,
      data: { total: 1, items: [{ id: 7, name, type: 57 }] } })
    if (r.pathname === '/api/token/search') return Response.json({ success: true,
      data: { total: 1, items: [{ id: 9, name }] } })
  })
  const result = await prepareOffline(options)
  assert.equal(result.channel_id, 7)
  assert.equal(options.requests.filter(r => r.method === 'POST' && r.pathname === '/api/channel/').length, 1)
  assert.equal(options.requests.filter(r => r.pathname === '/api/channel/search').length, 1)
})

test('ambiguous channel readback leaves private state and refuses token or run creation', async t => {
  const options = fixture(t, r => r.pathname === '/api/channel/search' ? Response.json({ success: true,
    data: { total: 2, items: [] } }) : undefined)
  await assert.rejects(prepareOffline(options), /OFFLINE_CHANNEL_RESULT_UNKNOWN/)
  assert.equal(options.requests.some(r => r.pathname === '/api/token/' || r.pathname === '/api/upstream-acceptance/runs'), false)
  assert.equal(readdirSync(options.parentDir).length, 1)
})

test('a non-Root login cannot preseed a channel even when setup and login succeeded', async t => {
  const options = fixture(t, r => r.pathname === '/api/user/login' ? Response.json({ success: true,
    data: { user: { id: 1, role: 10 }, access_token: 'synthetic-session', session: { sid: 'synthetic-sid' } } }) : undefined)
  await assert.rejects(prepareOffline(options), /OFFLINE_ROOT_SESSION_REQUIRED/)
  assert.equal(options.requests.some(r => r.pathname === '/api/channel/'), false)
})

test('a wrong logout receipt retains local credentials instead of claiming cleanup succeeded', async t => {
  const options = fixture(t, r => r.pathname === '/api/user/auth/logout' ? Response.json({ success: true,
    data: { revoked_sid: 'another-session' } }) : undefined)
  const result = await prepareOffline(options)
  await assert.rejects(cleanupOffline({ stateDir: result.state_dir, fetchImpl: options.fetchImpl }), /OFFLINE_LOGOUT_NOT_VERIFIED/)
  assert.equal(statSync(path.join(result.state_dir, 'state.json')).mode & 0o777, 0o600)
})

test('group-readable session state is rejected without sending credentials or modifying server state', async t => {
  const options = fixture(t)
  const result = await prepareOffline(options)
  const count = options.requests.length
  chmodSync(path.join(result.state_dir, 'state.json'), 0o640)
  await assert.rejects(cleanupOffline({ stateDir: result.state_dir, fetchImpl: options.fetchImpl }), /OFFLINE_STATE_REJECTED/)
  assert.equal(options.requests.length, count)
})

test('a mismatched run is never stopped and leaves all private files intact', async t => {
  const options = fixture(t, r => r.pathname === '/api/upstream-acceptance/runs/synthetic-run' ? Response.json({ success: true,
    data: { owner_current: true, run: { channel_id: 99, total: 0 } } }) : undefined)
  const result = await prepareOffline(options)
  const count = options.requests.length
  await assert.rejects(cleanupOffline({ stateDir: result.state_dir, fetchImpl: options.fetchImpl }), /OFFLINE_RUN_SCOPE_MISMATCH/)
  assert.equal(options.requests.slice(count).some(r => r.method !== 'GET'), false)
  assert.equal(statSync(path.join(result.state_dir, 'state.json')).mode & 0o777, 0o600)
})

test('a fresh server without enabled acceptance mode never receives a channel or token write', async t => {
  const options = fixture(t, r => r.pathname.startsWith('/api/upstream-acceptance/runs/offline-probe-') ?
    Response.json({ success: false, code: 'upstream_acceptance_denied' }, { status: 403 }) : undefined)
  await assert.rejects(prepareOffline(options), /OFFLINE_ACCEPTANCE_MODE_REQUIRED/)
  assert.equal(options.requests.some(r => ['/api/channel/', '/api/token/', '/api/upstream-acceptance/runs'].includes(r.pathname)), false)
})
