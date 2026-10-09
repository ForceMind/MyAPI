import assert from 'node:assert/strict'
import test from 'node:test'
import { createUIFixture, FIXTURE_TIME, SETTLEMENT_REVIEW_IDS, SETTLEMENT_REVIEW_METADATA, SETTLEMENT_REVIEW_DIAGNOSTIC } from './browser-fixtures.mjs'

test('playground selection exposes metadata without a credential and rejects anonymous reads', () => {
  const owner = createUIFixture({ role: 1 })
  const result = owner.response(new URL('http://localhost/pg/keys'))
  assert.equal(result.status, 200)
  assert.equal(result.body.data.items.length, 1)
  assert.equal(result.body.data.items[0].id, 21)
  assert.equal('key' in result.body.data.items[0], false)
  assert.equal('token' in result.body.data.items[0], false)
  assert(createUIFixture({ role: 0 }).response(new URL('http://localhost/pg/keys')).violation)
})

test('selected-key model discovery includes the success envelope required by the production client', () => {
  const response = createUIFixture({ role: 1 }).response(new URL('http://localhost/pg/models'))
  assert.equal(response.status, 200)
  // PlaygroundListModels returns the OpenAI list with MyAPI's success field.
  // Omitting it makes parseUserModelOptions reject every otherwise valid model.
  assert.deepEqual(response.body, {
    success: true,
    object: 'list',
    data: [{ id: 'synthetic-text-model', object: 'model', owned_by: 'synthetic-provider' }],
  })
  assert(createUIFixture({ role: 0 }).response(new URL('http://localhost/pg/models')).violation)
})

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
  for (const path of ['/api/channel', '/api/user', '/api/authz/catalog', '/api/log', '/api/data', '/api/data/users', '/api/data/flow', '/api/perf-metrics/summary', '/api/deployments/settings', '/api/option', '/api/system-info/instances', '/api/channel/quota/changes']) assert.match(fixture.response(url(path)).violation, /Ordinary user requested admin data/, path)
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

test('owner and admin analytics explicitly distinguish populated, empty and unavailable reads', () => {
  for (const role of [1, 10]) {
    const fixture = createUIFixture({ role })
    const paths = role === 1 ? ['/api/data/self', '/api/data/flow/self'] : ['/api/data', '/api/data/users', '/api/data/flow']
    for (const path of paths) {
      fixture.state.analytics = 'populated'
      const rows = fixture.response(url(path)).body.data
      assert(rows.length > 0)
      assert(rows.every(row => row.username === fixture.user.username && row.model_name === 'synthetic-text-model'))
      assert.equal(rows.reduce((sum, row) => sum + row.count, 0), 20)
      fixture.state.analytics = 'empty'
      assert.deepEqual(fixture.response(url(path)).body.data, [])
      fixture.state.analytics = 'error'
      assert.equal(fixture.response(url(path)).status, 503)
      assert.equal(fixture.response(url(path)).body.success, false)
    }
    assert(fixture.response(url('/api/data/unconfigured')).violation)
  }
})

test('unavailable deployment settings cannot become disabled success or grant list, hardware or connection access', () => {
  const fixture = createUIFixture({ role: 10 })
  assert.deepEqual(fixture.response(url('/api/deployments/settings')).body.data, { enabled: false })
  fixture.state.deploymentSettings = 'error'
  assert.equal(fixture.response(url('/api/deployments/settings')).status, 503)
  for (const [method, path] of [['GET', '/api/deployments'], ['GET', '/api/deployments/hardware'], ['POST', '/api/deployments/test-connection'], ['POST', '/api/deployments']]) assert(fixture.response(url(path), method).violation, `${method} ${path}`)
})

test('prompt-learning fixtures grant exactly owner policy and first-page history reads without any mutation', () => {
  const fixture = createUIFixture({ role: 1 })
  assert.deepEqual(fixture.response(url('/api/user/prompt-learning')).body.data, { scope: 'self', enabled: false, generation: 0 })
  for (const kind of ['versions', 'runs']) {
    const path = `/api/user/prompt-learning/${kind}?p=1&page_size=20`
    assert.deepEqual(fixture.response(url(path)).body.data, { items: [], total: 0, page: 1, page_size: 20 })
    assert(fixture.response(url(`/api/user/prompt-learning/${kind}?p=2&page_size=20`)).violation)
    fixture.state.promptLearning = 'error'
    assert.equal(fixture.response(url(path)).status, 503)
    fixture.state.promptLearning = 'ready'
  }
  for (const [method, path] of [['PUT', '/api/user/prompt-learning'], ['POST', '/api/user/prompt-learning/versions'], ['POST', '/api/user/prompt-learning/runs/1/cancel']]) assert(fixture.response(url(path), method).violation)
  assert(fixture.response(url('/api/user/prompt-learning/new-private-read')).violation)
  assert(createUIFixture({ role: 0 }).response(url('/api/user/prompt-learning')).violation)
})

test('pricing enablement is fixture-only and keeps full pricing group contracts separate from missing and error', () => {
  assert.equal(createUIFixture().response(url('/api/status')).body.data.HeaderNavModules, undefined)
  const fixture = createUIFixture({ role: 0, pricingEnabled: true })
  assert.deepEqual(JSON.parse(fixture.response(url('/api/status')).body.data.HeaderNavModules), { pricing: { enabled: true, requireAuth: false } })
  fixture.state.pricing = 'populated'
  const pricing = fixture.response(url('/api/pricing')).body
  assert.deepEqual(pricing.data, [{ id: 41, model_name: 'synthetic-text-model', quota_type: 0, model_ratio: 1, completion_ratio: 2, enable_groups: ['default'] }])
  assert.deepEqual(pricing.vendors, [])
  assert.deepEqual(pricing.group_ratio, { default: 1 })
  assert.deepEqual(pricing.usable_group, { default: { desc: 'Synthetic', ratio: 1 } })
  assert.deepEqual(pricing.supported_endpoint, {})
  assert.deepEqual(pricing.auto_groups, [])
  fixture.state.pricing = 'empty'
  assert.deepEqual(fixture.response(url('/api/pricing')).body.data, [])
  fixture.state.pricing = 'error'
  assert.equal(fixture.response(url('/api/pricing')).status, 503)
  assert.equal(fixture.response(url('/api/pricing')).body.success, false)
})

test('pricing performance permits only the exact enabled-public model and summary reads, never provider access', () => {
  const fixture = createUIFixture({ role: 1, pricingEnabled: true })
  const path = '/api/perf-metrics?model=synthetic-text-model&hours=24'
  assert.deepEqual(fixture.response(url(path)).body.data, { model_name: 'synthetic-text-model', groups: [] })
  assert.deepEqual(fixture.response(url('/api/perf-metrics/summary?hours=24')).body.data, { models: [] })
  assert.deepEqual(createUIFixture({ role: 0, pricingEnabled: true }).response(url('/api/perf-metrics/summary?hours=24')).body.data, { models: [] })
  for (const invalid of ['/api/perf-metrics', '/api/perf-metrics?model=other&hours=24', `${path}&upstream=true`, '/api/perf-metrics/summary', '/api/perf-metrics/summary?hours=48', '/api/perf-metrics/summary?hours=24&private=true', '/api/oauth/github', '/api/user/login/2fa']) assert(fixture.response(url(invalid)).violation, invalid)
  assert(createUIFixture().response(url(path)).violation)
  assert(fixture.response(url('/api/chat/completions'), 'POST').violation)
})

test('playground disabled status narrows only the explicitly configured sidebar module', () => {
  const fixture = createUIFixture({ role: 1, playgroundDisabled: true })
  assert.deepEqual(JSON.parse(fixture.response(url('/api/status')).body.data.SidebarModulesAdmin), { chat: { enabled: true, playground: false } })
  assert.equal(fixture.response(url('/api/user/models?group=default')).body.success, true)
  assert.equal(fixture.response(url('/api/user/self/groups')).body.success, true)
})

test('unavailable performance summary stays distinct from a confirmed empty summary', () => {
  const fixture = createUIFixture({ role: 10 })
  assert.deepEqual(fixture.response(url('/api/perf-metrics/summary?hours=24')).body.data, { models: [] })
  fixture.state.performance = 'error'
  assert.equal(fixture.response(url('/api/perf-metrics/summary?hours=24')).status, 503)
})

test('settlement opt-in is isolated per session and preserves the original log and pending fixtures', () => {
  const original = createUIFixture()
  const before = original.response(url('/api/log')).body
  const enabled = createUIFixture({ settlementReviews: true })
  assert.deepEqual(enabled.response(url('/api/log')).body.data.items.map(log => log.request_id), SETTLEMENT_REVIEW_IDS)
  assert.deepEqual(original.response(url('/api/log')).body, before)
  assert.deepEqual(createUIFixture().response(url('/api/log')).body, before)
  assert.deepEqual(original.response(url('/api/usage-reviews/pending')).body.data.items, [])
  for (const id of SETTLEMENT_REVIEW_IDS) assert(original.response(url(`/api/usage-review/${id}`)).violation)
})

test('settlement details preserve pending and lagging raw journal states with explicit safe capabilities', () => {
  const fixture = createUIFixture({ settlementReviews: true })
  const [pending, journal, manual] = SETTLEMENT_REVIEW_IDS.map(id => fixture.response(url(`/api/usage-review/${id}`)).body.data)
  assert.deepEqual([pending.state, pending.reserved_quota, pending.actual_quota, pending.settlement_status, pending.recovery_block_reason, pending.can_reconcile_usage, pending.can_recover_text_dispatch], ['usage_unknown', 100, null, 'pending', 'automatic_settlement_pending', false, false])
  assert.deepEqual([journal.state, journal.reserved_quota, journal.actual_quota, journal.settlement_status, journal.recovery_block_reason, journal.can_reconcile_usage, journal.can_recover_text_dispatch], ['prepared', 0, 60, 'applied_journal_pending', 'automatic_settlement_applied', false, false])
  assert.deepEqual([manual.state, manual.actual_quota, manual.settlement_status, manual.recovery_block_reason, manual.can_reconcile_usage], ['usage_unknown', null, 'none', '', true])
  assert.equal(pending.text_dispatch_pending, true, 'automatic pending blocks the previously available dispatch recovery')
  assert.equal(journal.text_dispatch_pending, false)
  for (const review of [pending, journal, manual]) {
    assert.equal(review.user_id, 2)
    assert.equal(review.token_id, 21)
    assert.equal(review.decision, undefined)
    assert.equal(review.review_metadata, SETTLEMENT_REVIEW_METADATA)
    assert.equal(review.reason, SETTLEMENT_REVIEW_DIAGNOSTIC)
  }
})

test('settlement owner reads mirror the existing DTO without inventing response-level redaction', () => {
  const root = createUIFixture({ settlementReviews: true })
  const owner = createUIFixture({ role: 1, settlementReviews: true })
  for (const id of SETTLEMENT_REVIEW_IDS) {
    const path = `/api/usage-review/${id}`
    assert.deepEqual(owner.response(url(path)), root.response(url(path)))
    assert.equal(owner.response(url(path)).body.data.user_id, owner.user.id)
    assert(createUIFixture({ role: 10, settlementReviews: true }).response(url(path)).violation, 'an ordinary administrator is not the owner')
    assert(createUIFixture({ role: 0, settlementReviews: true }).response(url(path)).violation)
  }
  assert(owner.response(url('/api/usage-reviews/pending?writer=authoritative&after=0')).violation)
  assert(owner.response(url('/api/log')).violation)
  const logs = owner.response(url('/api/log/self')).body.data.items
  assert(logs.every(log => log.user_id === owner.user.id && !('channel' in log) && !('channel_name' in log)))
})

test('settlement fixture permits only exact bounded reads and rejects every reconciliation or dispatch mutation', () => {
  const fixture = createUIFixture({ settlementReviews: true })
  for (const writer of ['authoritative', 'legacy']) {
    const result = fixture.response(url(`/api/usage-reviews/pending?writer=${writer}&after=0`))
    assert.equal(result.status, 200)
    assert.equal(result.body.data.next_after, '')
    assert(result.body.data.items.length > 0)
    assert(result.body.data.items.every(review => review.writer === writer))
  }
  for (const path of [
    '/api/usage-reviews/pending',
    '/api/usage-reviews/pending?writer=authoritative&after=1',
    '/api/usage-reviews/pending?writer=other&after=0',
    '/api/usage-reviews/pending?writer=legacy&after=0&writer=authoritative',
    '/api/usage-reviews/pending?writer=legacy&after=0&after=1',
    '/api/usage-reviews/pending?writer=legacy&after=0&private=true',
    '/api/usage-review/unconfigured',
    `/api/usage-review/${SETTLEMENT_REVIEW_IDS[0]}?private=true`,
    `/api/usage-review/${SETTLEMENT_REVIEW_IDS[0]}/reconcile`,
  ]) assert(fixture.response(url(path)).violation, path)
  for (const id of SETTLEMENT_REVIEW_IDS) {
    for (const suffix of ['', '/reconcile', '/recover-dispatch']) {
      for (const method of ['POST', 'PUT', 'PATCH', 'DELETE']) assert(fixture.response(url(`/api/usage-review/${id}${suffix}`), method).violation, `${method} ${id}${suffix}`)
    }
  }
})
