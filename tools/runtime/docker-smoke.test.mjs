import assert from 'node:assert/strict'
import test from 'node:test'
import { probeFreshSQLite, probeRelayFixture, validateSmokeTarget } from './docker-smoke.mjs'
import { fakeOpenAIListenHost, startFakeOpenAI } from './fake-openai.mjs'

const sha = 'a'.repeat(40)
const baseUrl = 'http://127.0.0.1:18080'
const json = (body, status = 200) => new Response(JSON.stringify(body), { status })

test('relay fixture remains an explicit isolated smoke capability', async () => {
  assert.equal(typeof probeRelayFixture, 'function')
  await assert.rejects(probeRelayFixture({ baseUrl, edition: 'full', sha }), /ISOLATED_SMOKE_OPT_IN_REQUIRED/)
  let writes = 0
  await assert.rejects(probeRelayFixture({
    baseUrl, edition: 'full', sha, username: 'root', password: 'password', isolated: true,
    upstreamBaseUrl: 'http://127.0.0.1:19090/not-the-control-plane',
    fetchImpl: async () => { writes += 1; throw new Error('must not fetch') },
  }), /INVALID_SMOKE_UPSTREAM_TARGET/)
  assert.equal(writes, 0)
  await assert.rejects(probeFreshSQLite({
    baseUrl, edition: 'full', sha, isolated: true, relayFixture: true,
    fetchImpl: async () => { writes += 1; throw new Error('must not fetch') },
  }), /INVALID_SMOKE_UPSTREAM_TARGET/)
  assert.equal(writes, 0)
  let appRequests = 0
  await assert.rejects(probeRelayFixture({
    baseUrl, edition: 'full', sha, username: 'root', password: 'password', isolated: true,
    upstreamBaseUrl: 'http://127.0.0.1:19090',
    fetchImpl: async (url) => {
      if (new URL(url).origin === 'http://127.0.0.1:19090') {
        return json({ count: 1, path_ok: true, model_ok: true, max_tokens_ok: true, stream_ok: true, bearer_ok: true, request_ok: true })
      }
      appRequests += 1
      throw new Error('app request must not occur')
    },
  }), /SMOKE_FIXTURE_UPSTREAM_NOT_FRESH/)
  assert.equal(appRequests, 0)
})

test('synthetic OpenAI only exposes a non-sensitive request summary', async (t) => {
  const upstream = await startFakeOpenAI({ port: 0 })
  t.after(() => upstream.close())
  const upstreamUrl = `http://${upstream.host}:${upstream.port}`
  const initial = await (await fetch(upstreamUrl + '/__smoke__/control')).json()
  assert.deepEqual(initial, { count: 0, path_ok: false, model_ok: false, max_tokens_ok: false, stream_ok: false, bearer_ok: false, request_ok: false })
  const response = await fetch(upstreamUrl + '/v1/chat/completions', {
    method: 'POST', headers: { Authorization: 'Bearer synthetic-upstream-key', 'Content-Type': 'application/json' },
    body: JSON.stringify({ model: 'smoke-model', messages: [], max_tokens: 8, stream: false }),
  })
  assert.equal(response.status, 200)
  const control = await (await fetch(upstreamUrl + '/__smoke__/control')).json()
  assert.deepEqual(control, { count: 1, path_ok: true, model_ok: true, max_tokens_ok: true, stream_ok: true, bearer_ok: true, request_ok: true })
  assert.equal(response.headers.get('x-api-key'), 'synthetic-response-header-secret')
  assert.equal(JSON.stringify(control).includes('synthetic-upstream-key'), false)
})

test('synthetic OpenAI listener only allows explicit loopback or Docker namespace hosts', async (t) => {
  assert.equal(fakeOpenAIListenHost({}), '127.0.0.1')
  assert.equal(fakeOpenAIListenHost({ FAKE_OPENAI_LISTEN_HOST: '0.0.0.0' }), '0.0.0.0')
  assert.throws(() => fakeOpenAIListenHost({ FAKE_OPENAI_LISTEN_HOST: 'localhost' }), /INVALID_FAKE_OPENAI_LISTENER/)
  assert.throws(() => fakeOpenAIListenHost({ FAKE_OPENAI_LISTEN_HOST: '192.0.2.1' }), /INVALID_FAKE_OPENAI_LISTENER/)
  assert.throws(() => startFakeOpenAI({ host: 'localhost', port: 0 }), /INVALID_FAKE_OPENAI_LISTENER/)
  assert.throws(() => startFakeOpenAI({ host: '192.0.2.1', port: 19090 }), /INVALID_FAKE_OPENAI_LISTENER/)
})

test('relay fixture verifies exact wallet, key, usage, log and redaction contracts without reporting credentials', async () => {
  let controlReads = 0
  const optionKeys = []
  const report = await probeRelayFixture({
    baseUrl,
    edition: 'full',
    sha,
    username: 'smokeadmin',
    password: 'root-secret',
    isolated: true,
    upstreamBaseUrl: 'http://127.0.0.1:19090',
    fullContentExpected: true,
    fetchImpl: async (url, options = {}) => {
      assert.equal(options.redirect, 'error')
      const parsed = new URL(url)
      if (parsed.origin === 'http://127.0.0.1:19090') {
        assert.equal(parsed.pathname, '/__smoke__/control')
        assert.equal(options.method, undefined)
        controlReads += 1
        return json(controlReads === 1
          ? { count: 0, path_ok: false, model_ok: false, max_tokens_ok: false, stream_ok: false, bearer_ok: false, request_ok: false }
          : { count: 1, path_ok: true, model_ok: true, max_tokens_ok: true, stream_ok: true, bearer_ok: true, request_ok: true })
      }
      assert.equal(parsed.origin, baseUrl)
      const body = options.body ? JSON.parse(options.body) : null
      const auth = options.headers?.Authorization
      switch (`${options.method || 'GET'} ${parsed.pathname}`) {
        case 'POST /api/user/login':
          if (body.username === 'smokeadmin') {
            assert.equal(body.password, 'root-secret')
            return json({ success: true, data: { access_token: 'root-session' } })
          }
          assert.equal(body.username, 'smokeuser')
          assert.equal(body.password.length, 18)
          return json({ success: true, data: { access_token: 'ordinary-session' } })
        case 'GET /api/status':
          return json({ success: true, data: { enable_batch_update: false } })
        case 'PUT /api/option/':
          assert.equal(auth, 'Bearer root-session')
          optionKeys.push([body.key, body.value])
          return json({ success: true })
        case 'POST /api/user/':
          assert.equal(auth, 'Bearer root-session')
          assert.equal(body.username, 'smokeuser')
          assert.equal(body.role, 1)
          assert.equal(body.password.length, 18)
          return json({ success: true })
        case 'GET /api/user/search':
          assert.equal(auth, 'Bearer root-session')
          assert.equal(parsed.searchParams.get('page_size'), '10')
          return json({ success: true, data: { items: [{ id: 11, username: 'smokeuser', role: 1 }] } })
        case 'POST /api/user/manage':
          assert.equal(auth, 'Bearer root-session')
          assert.deepEqual(body, { id: 11, action: 'add_quota', mode: 'override', value: 1_000_000 })
          return json({ success: true })
        case 'POST /api/token/':
          assert.equal(auth, 'Bearer ordinary-session')
          assert.equal(body.model_limits, 'smoke-model')
          assert.equal(body.remain_quota, 1_000_000)
          return json({ success: true })
        case 'GET /api/token/search':
          assert.equal(auth, 'Bearer ordinary-session')
          assert.equal(parsed.searchParams.get('keyword'), 'smoke-restricted-key')
          assert.equal(parsed.searchParams.get('page_size'), '10')
          return json({ success: true, data: { items: [{ id: 12, name: 'smoke-restricted-key' }] } })
        case 'POST /api/token/12/key':
          assert.equal(auth, 'Bearer ordinary-session')
          return json({ success: true, data: { key: 'restricted-api-key' } })
        case 'POST /api/channel/':
          assert.equal(auth, 'Bearer root-session')
          assert.equal(body.channel.type, 1)
          assert.equal(body.channel.base_url, 'http://127.0.0.1:19090')
          assert.equal(body.channel.models, 'smoke-model')
          return json({ success: true })
        case 'GET /api/channel/search':
          assert.equal(auth, 'Bearer root-session')
          assert.equal(parsed.searchParams.get('keyword'), 'smoke-openai-channel')
          assert.equal(parsed.searchParams.get('page_size'), '10')
          return json({ success: true, data: { items: [{ id: 13, name: 'smoke-openai-channel', type: 1 }] } })
        case 'POST /v1/chat/completions': {
          if (!auth) return json({ success: false }, 401)
          assert.equal(auth, 'Bearer restricted-api-key')
          assert.equal(body.model, 'smoke-model')
          assert.equal(body.max_tokens, 8)
          return new Response(JSON.stringify({ usage: { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 } }), {
            status: 200, headers: { 'X-Oneapi-Request-Id': 'request-fixture' },
          })
        }
        case 'GET /api/user/11':
          assert.equal(auth, 'Bearer root-session')
          return json({ success: true, data: { quota: 999_985, used_quota: 15, request_count: 1 } })
        case 'GET /api/token/12':
          assert.equal(auth, 'Bearer ordinary-session')
          return json({ success: true, data: { remain_quota: 999_985, used_quota: 15 } })
        case 'GET /api/channel/13':
          assert.equal(auth, 'Bearer root-session')
          return json({ success: true, data: { id: 13, used_quota: 15 } })
        case 'GET /api/log/self':
          assert.equal(auth, 'Bearer ordinary-session')
          assert.equal(parsed.searchParams.get('request_id'), 'request-fixture')
          return json({ success: true, data: { total: 1, items: [{ user_id: 11, token_id: 12, channel: 13, request_id: 'request-fixture', quota: 15, prompt_tokens: 10, completion_tokens: 5, other: '{}' }] } })
        case 'GET /api/log/':
          assert.equal(auth, 'Bearer root-session')
          assert.equal(parsed.searchParams.get('request_id'), 'request-fixture')
          return json({ success: true, data: { total: 1, items: [{ request_id: 'request-fixture' }] } })
        case 'GET /api/full-content-logs/request-fixture':
          if (auth === 'Bearer ordinary-session') return json({ success: false }, 403)
          assert.equal(auth, 'Bearer root-session')
          return json({ success: true, data: {
            request_headers: { Authorization: ['[REDACTED]'] },
            response_headers: { 'X-Api-Key': ['[REDACTED]'] },
            request_body: '{"api_key":"[REDACTED]"}',
            response_body: 'synthetic fixed response',
          } })
        default:
          assert.fail(`unexpected smoke request ${options.method || 'GET'} ${parsed.pathname}`)
      }
    },
  })
  assert.deepEqual(optionKeys, [
    ['ModelRatio', '{"smoke-model":1}'], ['CompletionRatio', '{"smoke-model":1}'], ['GroupRatio', '{"default":1}'],
    ['GroupGroupRatio', '{}'], ['LogConsumeEnabled', 'true'],
  ])
  assert.equal(controlReads, 2)
  assert.equal(report.passed, true)
  assert.equal(report.checks.every((check) => check.ok === true), true)
  for (const privateValue of ['root-secret', 'root-session', 'ordinary-session', 'restricted-api-key', 'synthetic-request-log-value', 'synthetic-upstream-key', 'synthetic-response-header-secret']) {
    assert.equal(JSON.stringify(report).includes(privateValue), false)
  }
})

test('smoke initialization requires explicit opt-in and a literal loopback target', async () => {
  for (const url of ['invalid', 'http://127.0.0.1:0', 'https://127.0.0.1:18080', 'http://localhost:18080', 'http://192.0.2.1:18080',
    'http://127.0.0.1:18080/api', 'http://127.0.0.1:18080?secret=value', 'http://user:pass@127.0.0.1:18080']) {
    assert.throws(() => validateSmokeTarget(url, 'full', sha))
  }
  assert.throws(() => validateSmokeTarget(baseUrl, 'unknown', sha))
  assert.throws(() => validateSmokeTarget(baseUrl, 'full', 'main'))
  await assert.rejects(probeFreshSQLite({ baseUrl, edition: 'full', sha }), /OPT_IN_REQUIRED/)
})

test('existing or non-SQLite instances are rejected without writes', async () => {
  for (const data of [
    { status: true, root_init: true, database_type: 'sqlite' },
    { status: false, root_init: true, database_type: 'sqlite' },
    { status: false, root_init: false, database_type: 'mysql' },
    {},
  ]) {
    let requests = 0
    await assert.rejects(probeFreshSQLite({ baseUrl, edition: 'full', sha, isolated: true,
      fetchImpl: async (_url, options) => {
        requests += 1
        assert.equal(options.method, undefined)
        assert.equal(options.redirect, 'error')
        return json({ success: true, data })
      },
    }), /FRESH_SQLITE/)
    assert.equal(requests, 1)
  }
})

for (const edition of ['full', 'lan']) {
  test(`${edition} fresh fixture initializes, authenticates and reports no credentials`, async () => {
    let setupBody
    let initialized = false
    const report = await probeFreshSQLite({ baseUrl, edition, sha, isolated: true,
      fetchImpl: async (url, options) => {
        assert.equal(options.redirect, 'error')
        const path = new URL(url).pathname
        if (path === '/api/setup' && options.method === 'POST') {
          setupBody = JSON.parse(options.body)
          assert.equal(setupBody.password, setupBody.confirmPassword)
          assert.equal(setupBody.SelfUseModeEnabled, edition === 'lan')
          initialized = true
          return json({ success: true })
        }
        if (path === '/api/setup') return json({ success: true, data: { status: false, root_init: false, database_type: 'sqlite' } })
        assert.ok(initialized)
        if (path === '/api/status') return json({ success: true, data: { setup: true, self_use_mode_enabled: edition === 'lan' } })
        if (path === '/api/user/login') {
          assert.equal(JSON.parse(options.body).password, setupBody.password)
          return json({ success: true, data: { access_token: 'private-fixture-access-token' } })
        }
        if (path === '/api/user/self' && !options.headers?.Authorization) return json({ success: false }, 401)
        assert.equal(options.headers.Authorization, 'Bearer private-fixture-access-token')
        assert.ok(['/api/user/self', '/api/channel/quota/changes', '/api/log/'].includes(path))
        return json({ success: true, data: { private_body: 'private-fixture-response' } })
      },
    })
    assert.equal(report.passed, true)
    assert.equal(report.edition, edition)
    assert.equal(report.sha, sha)
    for (const secret of [setupBody.password, 'private-fixture-access-token', 'private-fixture-response']) {
      assert.equal(JSON.stringify(report).includes(secret), false)
    }
  })
}

for (const failure of ['setup', 'anonymous', 'mode', 'login']) {
  test(`fresh fixture rejects ${failure} failure without reporting success`, async () => {
    const paths = []
    const run = probeFreshSQLite({ baseUrl, edition: 'full', sha, isolated: true,
      fetchImpl: async (url, options) => {
        const path = new URL(url).pathname
        paths.push(path)
        if (path === '/api/setup' && options.method !== 'POST') {
          return json({ success: true, data: { status: false, root_init: false, database_type: 'sqlite' } })
        }
        if (path === '/api/setup') return json({ success: failure !== 'setup' })
        if (path === '/api/user/self') return json({ success: failure === 'anonymous' }, failure === 'anonymous' ? 200 : 401)
        if (path === '/api/status') return json({ success: true, data: { setup: true, self_use_mode_enabled: failure === 'mode' } })
        if (path === '/api/user/login') return json({ success: false, message: 'private server detail' }, 401)
        assert.fail(`unexpected request ${path}`)
      },
    })
    if (failure === 'login') {
      const report = await run
      assert.equal(report.passed, false)
      assert.equal(JSON.stringify(report).includes('private server detail'), false)
      assert.equal(paths.includes('/api/log/'), false)
    } else {
      await assert.rejects(run, new RegExp({ setup: 'SETUP_FAILED', anonymous: 'ANONYMOUS_ACCESS_NOT_REJECTED', mode: 'RUNTIME_MODE_MISMATCH' }[failure]))
      assert.equal(paths.includes('/api/user/login'), false)
    }
  })
}
