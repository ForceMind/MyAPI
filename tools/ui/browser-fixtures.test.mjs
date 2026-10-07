import assert from 'node:assert/strict'
import test from 'node:test'
import { createUIFixture, FIXTURE_TIME } from './browser-fixtures.mjs'

const url = path => new URL(path, 'http://synthetic.invalid')

test('unknown GET and every unrequested mutation fail closed rather than returning synthetic success', () => {
  const fixture = createUIFixture()
  for (const [method, path] of [['GET', '/api/data/new-private-endpoint'], ['GET', '/api/unconfigured'], ['POST', '/api/channel'], ['DELETE', '/api/user/self'], ['PUT', '/api/option']]) {
    const response = fixture.response(url(path), method)
    assert(response.violation, `${method} ${path}`)
    assert.equal(response.body, undefined)
  }
})

test('ordinary sessions can read their own data and never receive administrative fixture responses', () => {
  const fixture = createUIFixture({ role: 1 })
  for (const path of ['/api/user/self', '/api/token', '/api/token/auto-groups', '/api/data/self', '/api/log/self', '/api/user/sessions']) assert.equal(fixture.response(url(path)).body.success, true, path)
  for (const path of ['/api/channel', '/api/user', '/api/authz/catalog', '/api/log', '/api/data', '/api/option', '/api/system-info/instances', '/api/channel/quota/changes']) assert.match(fixture.response(url(path)).violation, /Ordinary user requested admin data/, path)
})

test('ordinary administrator can read channels but cannot obtain Root-only settings or system data', () => {
  const fixture = createUIFixture({ role: 10 })
  assert.equal(fixture.response(url('/api/channel')).body.success, true)
  for (const path of ['/api/option', '/api/option/typed-bulk/revision', '/api/system-info/instances', '/api/system-task/list', '/api/quota-writer/status']) assert.match(fixture.response(url(path)).violation, /Non-Root/, path)
})

test('anonymous bootstrap uses the real 401 boundary while public setup and status remain readable', () => {
  const fixture = createUIFixture({ role: 0, setupComplete: false })
  const response = fixture.response(url('/api/user/auth/refresh'), 'POST')
  assert.equal(response.status, 401)
  assert.equal(response.body.success, false)
  assert.equal(fixture.response(url('/api/setup')).body.data.status, false)
  assert.equal(fixture.response(url('/api/setup')).body.data.root_init, false)
  assert.equal(fixture.response(url('/api/status')).body.success, true)
  assert.match(fixture.response(url('/api/user/self')).violation, /Anonymous private request/)
})

test('authenticated bootstrap has a complete, synthetic session and stable timestamps without leaking the clock override', () => {
  const originalNow = Date.now
  const fixture = createUIFixture({ role: 1, language: 'fr' })
  assert.equal(Date.now, originalNow)
  const bundle = fixture.response(url('/api/user/auth/refresh'), 'POST').body.data
  assert.equal(bundle.token_type, 'Bearer')
  assert.equal(bundle.access_expires_at, FIXTURE_TIME / 1000 + 3600)
  assert.equal(bundle.user.role, 1)
  assert.equal(bundle.user.language, 'fr')
  assert.equal(bundle.session.current, true)
  assert.match(bundle.access_token, /synthetic.*not-a-credential/)
  const history = fixture.response(url('/api/log/self')).body.data.items
  assert.equal(history[0].created_at, FIXTURE_TIME / 1000)
  assert.deepEqual(history, createUIFixture({ role: 1 }).response(url('/api/log/self')).body.data.items)
})

test('admin and user sidebar fixtures are independent narrowing layers with real backend field names', () => {
  const fixture = createUIFixture({ role: 10, sidebar: true })
  const status = fixture.response(url('/api/status')).body.data
  assert.equal(JSON.parse(status.SidebarModulesAdmin).console.log, false)
  assert.equal(JSON.parse(fixture.user.sidebar_modules).console.token, false)
  assert.equal(fixture.user.permissions.sidebar_settings, true)
})

test('disabled commerce retains history while exposing no purchase capability', () => {
  const fixture = createUIFixture()
  const capabilities = fixture.response(url('/api/status')).body.data.user_funding_capabilities
  assert.equal(capabilities.mode, 'disabled')
  for (const key of ['can_top_up', 'can_redeem', 'can_transfer_affiliate_rewards', 'can_purchase_subscription']) assert.equal(capabilities[key], false, key)
  assert.equal(capabilities.can_view_funding_history, true)
  assert.equal(fixture.response(url('/api/option')).body.data.find(option => option.key === 'user_funding_setting.mode')?.value, 'disabled', 'the saved settings key agrees with public capabilities')
  assert.equal(fixture.response(url('/api/user/topup')).body.data.items[0].trade_no, 'synthetic-old-order')
})

test('controlled unavailable and empty responses stay distinct from recovered populated data', () => {
  const fixture = createUIFixture()
  fixture.state.options = 'error'
  assert.equal(fixture.response(url('/api/option')).status, 503)
  assert.equal(fixture.response(url('/api/option')).body.success, false)
  fixture.state.options = 'ready'
  assert.equal(fixture.response(url('/api/option')).body.success, true)
  fixture.state.keys = 'empty'
  assert.deepEqual(fixture.response(url('/api/token')).body.data.items, [])
  fixture.state.keys = 'populated'
  assert.equal(fixture.response(url('/api/token')).body.data.items.length, 1)
})

test('configured About content supplies the semantic heading used by the public-page journey', () => {
  const fixture = createUIFixture({ role: 0 })
  const about = fixture.response(url('/api/about')).body.data
  assert.match(about, /^# About\n\n/)
  assert.match(about, /Synthetic UI qualification/)
})
