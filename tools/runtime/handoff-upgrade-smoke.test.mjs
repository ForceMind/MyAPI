import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import { verifyHandoffBeforeRelay, verifyHandoffSnapshot } from './handoff-upgrade-smoke.mjs'

const json = (data) => new Response(JSON.stringify({ success: true, data }))
const credentials = { username: 'smokeadmin', password: 'synthetic-root-password', userName: 'smokeuser', userPassword: 'synthetic-user-password' }
const absentIntent = Symbol('absent settlement intent')

function fixture({ usage = 15, tamper = '', phaseToTamper = usage === 0 ? 'upgraded' : 'restored', phases = usage === 0 ? ['upgraded', 'restored'] : ['rejected', 'restored'], intentId = absentIntent, omitFact = false } = {}) {
  let phase = 'source'
  let relays = 0
  const calls = []
  const fetchImpl = async (url, options = {}) => {
    const { origin, pathname } = new URL(url)
    calls.push({ origin, pathname, method: options.method || 'GET' })
    assert.equal(options.redirect, 'error')
    if (pathname.startsWith('/v1/')) { relays += 1; assert.fail('verification must not issue relay requests') }
    if (origin.endsWith(':19090')) return new Response(JSON.stringify({ count: tamper === 'relay-count' ? usage + 1 : usage === 0 ? 0 : 1, request_ok: usage === 15 }))
    assert.ok(['http://127.0.0.1:18080', 'http://127.0.0.1:18081'].includes(origin))
    const altered = phase === phaseToTamper
    const user = options.headers?.Authorization === 'Bearer synthetic-user-session'
    const requestCount = usage === 0 ? 0 : 1
    switch (pathname) {
      case '/api/setup': return json({ status: true })
      case '/api/status': return json({ setup: true, user_funding_mode: altered && tamper === 'funding' ? 'wallet' : 'disabled' })
      case '/api/user/login': {
        const body = JSON.parse(options.body)
        assert.equal(body.password, body.username === 'smokeadmin' ? 'synthetic-root-password' : 'synthetic-user-password')
        return json({ access_token: body.username === 'smokeadmin' ? 'synthetic-root-session' : 'synthetic-user-session' })
      }
      case '/api/user/self': return json({ id: user ? 11 : 1, role: user ? 1 : 100, quota: user ? 1000000 - usage : 100000000,
        used_quota: user ? usage : 0, request_count: user ? requestCount : 0,
        ...(phase === 'upgraded' ? { self_use_no_balance: tamper === 'policy', usage_policy_revision: tamper === 'revision' ? 1 : 0 } : {}) })
      case '/api/token/search': return json({ items: [{ id: 12, name: 'smoke-restricted-key' }] })
      case '/api/token/12': return json({ id: 12, remain_quota: altered && tamper === 'quota' ? 999984 : 1000000 - usage, used_quota: usage,
        unlimited_quota: altered && tamper === 'unlimited', model_limits_enabled: true, model_limits: 'smoke-model', status: 1, group: 'default', expired_time: -1 })
      case '/api/token/12/key': return json({ key: altered && tamper === 'key' ? 'changed-synthetic-key' : 'synthetic-key' })
      case '/api/channel/search': return json({ items: [{ id: 13, name: 'smoke-openai-channel' }] })
      case '/api/channel/13': return json({ id: 13, used_quota: usage, status: 1, base_url: 'http://127.0.0.1:19090', models: 'smoke-model' })
      case '/api/log/': {
        if (usage === 0) return json({ total: 0, items: [] })
        const settlement = { state: tamper === 'settlement-state' ? 'pending' : 'applied' }
        if (!omitFact) settlement.fact_id = tamper === 'fact' ? 0 : 22
        if (intentId !== absentIntent) settlement.intent_id = intentId
        if (altered && tamper === 'intent-change') settlement.intent_id = 23
        return json({ total: 1, items: [{ id: 5, user_id: 11, token_id: 12, channel: 13, quota: 15,
          request_id: altered && tamper === 'log' ? 'changed-request' : 'original-request',
          other: JSON.stringify({ admin_info: { billing_settlement: settlement } }) }] })
      }
      default: assert.fail(`unexpected API ${pathname}`)
    }
  }
  const restore = async (verify) => {
    for (const next of phases) { phase = next; await verify('http://127.0.0.1:18081', next) }
  }
  return { fetchImpl, restore, calls, relays: () => relays }
}

test('old configured SQLite without settlement intent/fact history migrates before the first relay', async () => {
  const f = fixture({ usage: 0 })
  const report = await verifyHandoffBeforeRelay({ ...credentials, ...f })
  assert.equal(report.ok, true)
  assert.equal(f.relays(), 0)
  assert.equal(f.calls.filter((call) => call.pathname === '/api/user/login').length, 6)
  for (const secret of [...Object.values(credentials), 'synthetic-key', 'synthetic-root-session']) assert.equal(JSON.stringify(report).includes(secret), false)
})

test('upgraded old users never gain zero-wallet policy or invented policy revision', async () => {
  for (const tamper of ['policy', 'revision']) {
    await assert.rejects(verifyHandoffBeforeRelay({ ...credentials, ...fixture({ usage: 0, tamper }) }), /HANDOFF_POLICY_MISMATCH/)
  }
})

test('historical settlement rejection is followed by exact old-image recovery without replay', async () => {
  const f = fixture()
  const report = await verifyHandoffSnapshot({ ...credentials, ...f })
  assert.equal(report.ok, true)
  assert.equal(f.relays(), 0)
  assert.equal(f.calls.filter((call) => call.pathname === '/api/user/login').length, 4)
  assert.equal(f.calls.some((call) => call.method !== 'GET' && call.pathname !== '/api/user/login' && call.pathname !== '/api/token/12/key'), false)
  for (const secret of [...Object.values(credentials), 'synthetic-key', 'synthetic-root-session', 'original-request']) assert.equal(JSON.stringify(report).includes(secret), false)
})

test('positive intent identity is accepted only when it remains identical after old-image recovery', async () => {
  const valid = await verifyHandoffSnapshot({ ...credentials, ...fixture({ intentId: 21 }) })
  assert.equal(valid.ok, true)
  await assert.rejects(verifyHandoffSnapshot({ ...credentials, ...fixture({ intentId: 21, tamper: 'intent-change' }) }), /HANDOFF_STATE_MISMATCH/)
  await assert.rejects(verifyHandoffSnapshot({ ...credentials, ...fixture({ tamper: 'intent-change' }) }), /HANDOFF_STATE_MISMATCH/)
})

test('restored old image retains finite limits, exact usage, key, log and settlement proof', async () => {
  for (const tamper of ['quota', 'unlimited', 'key', 'log', 'funding']) {
    const f = fixture({ tamper })
    await assert.rejects(verifyHandoffSnapshot({ ...credentials, ...f }), /HANDOFF_STATE_MISMATCH/)
    assert.equal(f.relays(), 0)
  }
  for (const tamper of ['settlement-state', 'fact']) {
    await assert.rejects(verifyHandoffSnapshot({ ...credentials, ...fixture({ tamper, phaseToTamper: 'source' }) }), /HANDOFF_SETTLEMENT_PROOF_MISMATCH/)
  }
  for (const intentId of [null, 0, '21', 1.5]) {
    await assert.rejects(verifyHandoffSnapshot({ ...credentials, ...fixture({ intentId }) }), /HANDOFF_SETTLEMENT_PROOF_MISMATCH/)
  }
  await assert.rejects(verifyHandoffSnapshot({ ...credentials, ...fixture({ omitFact: true }) }), /HANDOFF_SETTLEMENT_PROOF_MISMATCH/)
  await assert.rejects(verifyHandoffSnapshot({ ...credentials, ...fixture({ tamper: 'relay-count' }) }), /HANDOFF_UPSTREAM_MISMATCH/)
})

test('rehearsal refuses missing, reordered or foreign restoration phases', async () => {
  for (const phases of [[], ['restored'], ['restored', 'rejected'], ['rejected']]) {
    await assert.rejects(verifyHandoffSnapshot({ ...credentials, ...fixture({ phases }) }), /HANDOFF_(STATE_MISMATCH|SCOPE_REJECTED)/)
  }
  const foreign = fixture()
  foreign.restore = async (verify) => verify('https://example.com', 'rejected')
  await assert.rejects(verifyHandoffSnapshot({ ...credentials, ...foreign }), /HANDOFF_SCOPE_REJECTED/)
})

test('workflow pins the handoff baseline without publishing and keeps fresh install checks separate', () => {
  const workflow = readFileSync(new URL('../../.github/workflows/docker-smoke.yml', import.meta.url), 'utf8')
  assert.match(workflow, /ref: 71277bf6055ce68b8cd1d11f1d10f1907d7696fd/)
  const include = workflow.split('\n').find(line => line.trim().startsWith('include: ${{ fromJSON('))
  assert.ok(include)
  assert.match(include, /contains\(fromJSON\('\["codex\/personal-app-journey-20261009","codex\/docker-smoke-image-reuse-20261010"\]'/)
  assert.match(include, /\|\| '\[\{"edition":"full","scenario":"handoff","writer":"legacy"\}\]'/)
  assert.match(workflow, /scenario: \[fresh\]/)
  assert.match(workflow, /run: node tools\/runtime\/handoff-upgrade-smoke.mjs/)
  assert.match(workflow, /MYAPI_SMOKE_HANDOFF_UPGRADE: '1'/)
  assert.doesNotMatch(workflow, /push: true|packages: write|docker login/)
})
