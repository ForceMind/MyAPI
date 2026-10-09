import assert from 'node:assert/strict'
import test from 'node:test'
import { browserSetup, probePersonalBrowserJourney } from './personal-browser.mjs'

// These doubles protect driver orchestration and artifact hygiene only. They do
// not constitute browser, backend, relay, billing, or persistence acceptance.
const base = { baseUrl: 'http://127.0.0.1:18080', upstreamBaseUrl: 'http://127.0.0.1:19090',
  sha: 'a'.repeat(40), edition: 'full', isolated: true,
  username: 'synthetic-root', password: 'synthetic-password-never-report', writer: 'legacy' }

function setupBrowser({ failAt, responseSuccess = true } = {}) {
  const calls = []
  let route
  const locator = (kind, value) => ({
    async waitFor() {},
    async fill(text) { calls.push(['fill', kind, value, text]) },
    async click() { calls.push(['click', kind, value]); if (value === failAt) throw new Error(base.password) },
    async check() { calls.push(['check', kind, value]) },
  })
  const page = {
    setDefaultTimeout() {},
    on() {},
    async goto(url) { calls.push(['goto', url]); return { status: () => 200 } },
    getByRole(kind, options) { return locator(kind, options?.name) },
    locator(value) { return locator('selector', value) },
    async waitForResponse(predicate) {
      const response = { url: () => base.baseUrl + '/api/setup', request: () => ({ method: () => 'POST' }),
        status: () => 200, json: async () => ({ success: responseSuccess }) }
      assert.equal(predicate(response), true)
      return response
    },
  }
  const context = {
    async route(pattern, handler) { assert.equal(pattern, '**/*'); route = handler },
    async addInitScript() {},
    async newPage() { return page },
    async close() { calls.push(['context-close']) },
  }
  const browser = {
    async newContext(options) { calls.push(['context', options]); return context },
    async close() { calls.push(['browser-close']) },
  }
  return { calls, route: () => route,
    module: { chromium: { async launch(options) { calls.push(['launch', options]); return browser } } } }
}

for (const candidate of [
  { isolated: false }, { baseUrl: 'https://example.invalid' },
  { baseUrl: 'http://127.0.0.1:18080/other' }, { baseUrl: 'http://user:secret@127.0.0.1:18080' },
  { baseUrl: 'http://localhost:18080' }, { edition: 'unknown' }, { sha: 'main' },
  { writer: 'bridge' }, { username: '' }, { password: '' },
]) {
  test(`fails before browser or API work for invalid scope ${JSON.stringify(candidate)}`, async () => {
    let calls = 0
    const args = { ...base, ...candidate, playwrightModule: { chromium: { launch() { calls++ } } }, fetchImpl: () => { calls++ } }
    await assert.rejects(browserSetup(args), /^Error: SMOKE_PERSONAL_/)
    await assert.rejects(probePersonalBrowserJourney(args), /^Error: SMOKE_PERSONAL_/)
    assert.equal(calls, 0)
  })
}

test('upstream must be the fixed loopback fixture before any journey starts', async () => {
  let calls = 0
  await assert.rejects(probePersonalBrowserJourney({ ...base, upstreamBaseUrl: 'https://example.invalid',
    playwrightModule: { chromium: { launch() { calls++ } } }, fetchImpl: () => { calls++ } }), /SMOKE_PERSONAL_SCOPE_REJECTED/)
  assert.equal(calls, 0)
})

for (const edition of ['full', 'lan']) {
  test(`setup uses actual wizard controls and does not export credentials (${edition})`, async () => {
    const fixture = setupBrowser()
    const result = await browserSetup({ ...base, edition, playwrightModule: fixture.module })
    assert.equal(result.ok, true)
    assert.equal(result.sha, base.sha)
    assert.equal(JSON.stringify(result).includes(base.password), false)
    assert.equal(JSON.stringify(result).includes(base.username), false)
    assert.deepEqual(fixture.calls.find(call => call[0] === 'launch'), ['launch', { headless: true }])
    assert.equal(fixture.calls.find(call => call[0] === 'context')[1].serviceWorkers, 'block')
    assert.equal(fixture.calls.filter(call => call[0] === 'click' && call[2] === 'Next').length, 3)
    assert(fixture.calls.some(call => call[0] === 'click' && call[2] === `#usage-mode-${edition === 'lan' ? 'self' : 'external'}`))
    assert(fixture.calls.some(call => call[0] === 'click' && call[2] === 'Initialize system'))
    assert.equal(fixture.calls.at(-1)[0], 'browser-close')
    let continued = 0
    let aborted = 0
    for (const url of [base.baseUrl + '/api/setup', base.baseUrl + '/assets/app.js', 'https://example.invalid/tracker', 'http://127.0.0.1:19090/v1/chat/completions']) {
      await fixture.route()({ request: () => ({ url: () => url }), continue: () => { continued++ }, abort: () => { aborted++ } })
    }
    assert.equal(continued, 2, 'same-origin app requests remain real')
    assert.equal(aborted, 2, 'all other browser origins are blocked')
  })
}

test('browser exceptions and unexpected API bodies become fixed safe errors, with cleanup', async () => {
  for (const options of [{ failAt: 'Next' }, { responseSuccess: false }]) {
    const fixture = setupBrowser(options)
    await assert.rejects(browserSetup({ ...base, playwrightModule: fixture.module }), error => {
      assert.match(error.message, /^SMOKE_PERSONAL_/)
      assert.equal(error.stage, 'setup')
      assert(!error.message.includes(base.password))
      assert(!error.stack.includes(base.password))
      return true
    })
    assert.equal(fixture.calls.at(-1)[0], 'browser-close')
  }
})

test('browser launch errors cannot disclose provider diagnostics or synthetic secrets', async () => {
  const playwrightModule = { chromium: { async launch() { throw new Error(`launch ${base.password}`) } } }
  for (const run of [browserSetup, probePersonalBrowserJourney]) {
    await assert.rejects(run({ ...base, playwrightModule }), error => {
      assert.equal(error.message, 'SMOKE_PERSONAL_BROWSER_FAILED')
      assert.equal(error.stage, 'launch')
      assert(!error.stack.includes(base.password))
      return true
    })
  }
})

import { mkdtempSync, readFileSync, rmSync, symlinkSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'

function journeyBrowser({ writer = 'legacy', corrupt, screenshotSecret = false } = {}) {
  const reverse = Object.fromEntries(Object.entries(JSON.parse(readFileSync(new URL('../../web/src/i18n/locales/zh.json', import.meta.url))).translation).map(([key, value]) => [value, key]))
  const version = readFileSync(new URL('../../VERSION', import.meta.url), 'utf8').trim()
  const users = new Map([[1, { id: 1, username: base.username, role: 100, quota: 0, used_quota: 15, request_count: 1, self_use_no_balance: true, revision: 1 }]])
  const keys = new Map()
  const logs = new Map()
  const contexts = []
  const emitted = []
  const screenshots = []
  const apiCalls = []
  let upstreamCount = 2
  let closed = 0
  let sequence = 0
  const clone = value => JSON.parse(JSON.stringify(value))
  const response = (pathname, method, payload, status = 200, body, headers = {}) => ({
    url: () => base.baseUrl + pathname,
    request: () => ({ method: () => method, postDataJSON: () => body, headers: () => headers, url: () => base.baseUrl + pathname }),
    status: () => status, json: async () => clone(payload), headers: () => ({ 'x-oneapi-request-id': `request-private-${sequence}` }),
    text: async () => typeof payload === 'string' ? payload : JSON.stringify(payload), finished: async () => null,
  })
  const policy = user => ({ user_id: user.id, no_balance: user.self_use_no_balance, revision: user.revision, legacy_remaining_quota: user.quota })
  const budget = key => ({ policy: { enabled: true, fee_enabled: true, used: 0, reserved: corrupt === 'strict-reserve' ? 1 : 0, fee_used_usd: '0', fee_reserved_usd: '0', pending_request_id: '' }, pending: null })
  const makePage = context => {
    const page = { current: base.baseUrl + '/', fields: {}, switches: {}, listeners: {}, waiters: [], selected: '', actor: null,
      setDefaultTimeout() {}, on(event, handler) { this.listeners[event] = handler },
      async goto(url) { this.current = url; return { status: () => 200 } },
      url() { return this.current },
      async waitForURL(predicate) { assert(predicate(new URL(this.current))) },
      waitForResponse(predicate) { return new Promise(resolve => this.waiters.push({ predicate, resolve })) },
      emit(result) {
        emitted.push(result)
        this.listeners.request?.(result.request())
        for (const waiter of [...this.waiters]) if (waiter.predicate(result)) {
          this.waiters.splice(this.waiters.indexOf(waiter), 1); waiter.resolve(result)
        }
      },
      async evaluate(fn) {
        if (String(fn).includes('__APP_BUILD__')) return { global: `rv.${version}.${base.sha}.test`, html: `rv.${version}.${base.sha}.test`, meta: `rv.${version}.${base.sha}.test` }
        return !screenshotSecret
      },
      async screenshot(options) { screenshots.push(options) },
      keyboard: { async press() {} },
    }
    const normalize = value => context.locale === 'zh-CN' ? reverse[value] || value : value
    const locator = (kind, value, parents = []) => {
      value = normalize(value)
      const chain = [...parents, value]
      return {
        getByRole: (role, options) => locator(role, options?.name, chain),
        getByText: (text) => locator('text', text, chain),
        locator: (selector) => locator('selector', selector, chain),
        filter() { return this }, first() { return this }, last() { return this },
        async waitFor() {},
        async count() { return 0 },
        async fill(text) { page.fields[value] = text },
        async check() {}, async uncheck() {},
        async getAttribute(name) { if (name === 'aria-checked') return page.switches[value] ?? (value === 'No ordinary quota cap' ? 'true' : 'false') },
        async inputValue() { return page.selected },
        async isDisabled() { return page.selected === '' },
        async selectOption(id) {
          page.selected = id
          if (id) page.emit(response('/pg/models', 'GET', { success: true, data: [{ id: 'smoke-model' }] }))
        },
        async click(options) {
          if (options?.trial) return
          if (kind === 'switch') page.switches[value] = (page.switches[value] ?? (value === 'No ordinary quota cap' ? 'true' : 'false')) === 'true' ? 'false' : 'true'
          if (value === 'form button[type="submit"]') {
            const name = page.fields['form input[name="username"]']
            page.actor = [...users.values()].find(user => user.username === name)
            assert(page.actor)
            const token = `session-private-${page.actor.id}`
            page.current = base.baseUrl + '/keys'
            page.emit(response('/api/user/login', 'POST', { success: true, data: { access_token: token, user: clone(page.actor) } }))
          }
          if (value === 'Create API Key') page.switches['No ordinary quota cap'] = 'true'
          if (value === 'Save changes' || value === 'Save Changes') {
            const id = keys.size + 31
            const data = { name: page.fields.Name, remain_quota: Number(page.fields['Key quota (internal units)']), unlimited_quota: false, group: 'default' }
            const key = { ...data, id, user_id: page.actor.id, used_quota: 0, key: 'secret-masked-key-do-not-report' }
            keys.set(id, key)
            page.emit(response('/api/token/', 'POST', { success: true }, 200, data))
          }
          if (value === 'Save' && chain.includes('User usage policy')) {
            const owner = users.get(20)
            owner.self_use_no_balance = true
            owner.revision += 1
            page.emit(response('/api/user/20/usage-policy', 'PUT', { success: true, data: policy(owner) }))
          }
          if (value === 'Save' && chain.includes('API Key usage budgets')) {
            const key = [...keys.values()].find(key => key.name === 'browser-root-strict-reject')
            key.strict = true
            page.emit(response(`/api/token/${key.id}/budget`, 'PUT', { success: true, data: budget(key) }))
          }
          if (value === 'Send') {
            const key = keys.get(Number(page.selected))
            assert(key)
            sequence++
            const sent = { model: 'smoke-model', max_tokens: 8, stream: true, messages: [{ role: 'user', content: 'synthetic request' }] }
            const headers = { 'x-myapi-key-id': String(key.id) }
            let code
            if (!page.actor.self_use_no_balance) code = 'insufficient_user_quota'
            else if (key.remain_quota < 500) code = writer === 'legacy' ? 'pre_consume_token_quota_failed' : 'insufficient_user_quota'
            else if (key.strict) code = 'token_budget_unsupported_request'
            if (code) page.emit(response('/pg/chat/completions', 'POST', { error: { code, message: 'synthetic rejection' } }, code.startsWith('token_budget_') ? 400 : 403, sent, headers))
            else {
              upstreamCount++
              key.remain_quota -= 15; key.used_quota += 15
              page.actor.used_quota += 15; page.actor.request_count++
              const requestId = `request-private-${sequence}`
              logs.set(requestId, { request_id: requestId, user_id: page.actor.id, token_id: key.id, token_name: key.name,
                model_name: 'smoke-model', quota: 15, prompt_tokens: 10, completion_tokens: 5, other: '{"billing_source":"self_use"}' })
              if (corrupt === 'log-owner') logs.get(requestId).user_id = 999
              if (corrupt === 'quota') key.remain_quota++
              if (corrupt === 'upstream') upstreamCount++
              const total = corrupt === 'usage' ? 16 : 15
              page.emit(response('/pg/chat/completions', 'POST', `data: {"choices":[{"delta":{"content":"synthetic fixed response"}}]}\n\ndata: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":${total}}}\n\ndata: [DONE]\n\n`, 200, sent, headers))
              if (corrupt === 'replay') page.listeners.request?.(response('/pg/chat/completions', 'POST', {}, 200, sent, headers).request())
            }
          }
        },
      }
    }
    page.getByRole = (role, options) => locator(role, options?.name)
    page.getByText = text => locator('text', text)
    page.getByTitle = text => locator('title', text)
    page.locator = selector => locator('selector', selector)
    return page
  }
  const playwrightModule = { chromium: { async launch() { return {
    async newContext(options) {
      const context = { ...options, async route(pattern, handler) { this.handler = handler },
        async addInitScript(fn, args) { this.init = { fn: String(fn), args } }, async newPage() { this.page = makePage(this); return this.page } }
      contexts.push(context)
      return context
    }, async close() { closed++ },
  } } } }
  const fetchImpl = async (url, options) => {
    assert.equal(options.redirect, 'error')
    assert(options.signal instanceof AbortSignal)
    const parsed = new URL(url)
    if (parsed.origin === base.upstreamBaseUrl) {
      assert.equal(parsed.pathname, '/__smoke__/control')
      return new Response(JSON.stringify({ count: upstreamCount, request_ok: true }))
    }
    assert.equal(parsed.origin, base.baseUrl)
    assert.equal(options.headers.Origin, base.baseUrl)
    const actorId = Number(options.headers.Authorization.match(/^Bearer session-private-(\d+)$/)?.[1])
    const actor = users.get(actorId)
    assert(actor, 'only a genuine browser-login result may authorize API readbacks')
    const route = `${options.method} ${parsed.pathname}`
    apiCalls.push(route)
    const body = options.body ? JSON.parse(options.body) : null
    let data
    let status = 200
    if (route === 'GET /api/quota-writer/status') data = { state: { mode: corrupt === 'writer' ? 'bridge' : writer } }
    else if (route === 'GET /api/status') data = { user_funding_mode: 'disabled', enable_batch_update: false }
    else if (route === 'PUT /api/option/') assert.deepEqual(body, { key: 'general_setting.quota_display_type', value: 'TOKENS' })
    else if (route === 'GET /api/user/self') data = clone(actor)
    else if (route === 'POST /api/user/') { assert.equal(body.role, 1); assert(body.password.length >= 8 && body.password.length <= 20); users.set(20, { id: 20, username: body.username, role: 1, quota: 0, used_quota: 0, request_count: 0, self_use_no_balance: false, revision: 0 }) }
    else if (route === 'GET /api/user/search') data = { items: [clone(users.get(20))] }
    else if (parsed.pathname.match(/^\/api\/user\/\d+\/usage-policy$/)) {
      const target = Number(parsed.pathname.split('/')[3])
      if (options.method === 'PUT') status = 403
      else if (target !== actor.id && actor.role !== 100) status = 404
      else data = policy(users.get(target))
    }
    else if (route === 'GET /api/token/search') data = { items: [...keys.values()].filter(key => key.user_id === actor.id && key.name === parsed.searchParams.get('keyword')).map(clone) }
    else if (/^GET \/api\/token\/\d+$/.test(route)) data = clone(keys.get(Number(parsed.pathname.split('/')[3])))
    else if (/^GET \/api\/token\/\d+\/budget$/.test(route)) data = budget(keys.get(Number(parsed.pathname.split('/')[3])))
    else if (route === 'GET /api/log/self') {
      const log = logs.get(parsed.searchParams.get('request_id'))
      data = { total: log ? 1 : 0, items: log ? [clone(log)] : [] }
    }
    else assert.fail(`unexpected driver request: ${route}`)
    return new Response(JSON.stringify({ success: status === 200, data }), { status })
  }
  return { playwrightModule, fetchImpl, contexts, emitted, screenshots, apiCalls, closed: () => closed, upstreamCount: () => upstreamCount }
}

for (const writer of ['legacy', 'authoritative']) {
  test(`driver contract: UI dispatch orchestration, distinct ordinary owner and safe report (${writer})`, async () => {
    const fixture = journeyBrowser({ writer })
    const artifactDir = mkdtempSync(path.join(tmpdir(), 'personal-driver-contract-'))
    try {
      const report = await probePersonalBrowserJourney({ ...base, ...fixture, writer, artifactDir })
      assert.equal(report.passed, true)
      assert.equal(report.writer, writer)
      assert.equal(report.checks.length, 5)
      assert.equal(fixture.upstreamCount(), 4)
      assert.equal(fixture.contexts.length, 2)
      assert.deepEqual(fixture.contexts.map(context => [context.locale, context.viewport.width, context.page.actor.role]), [['en-US', 1280, 100], ['zh-CN', 320, 1]])
      assert(fixture.contexts[1].init.fn.includes("language === 'zh' ? 'zhCN' : language"))
      assert.equal(fixture.emitted.filter(response => new URL(response.url()).pathname === '/pg/chat/completions').length, 5)
      assert.equal(fixture.apiCalls.some(route => route.includes('/v1/') || route.includes('/pg/') || /\/key$/.test(route) || route === 'POST /api/user/login'), false)
      const serialized = JSON.stringify(report)
      for (const secret of [base.password, base.username, 'session-private-', 'secret-masked-key', 'request-private-', 'synthetic request']) assert(!serialized.includes(secret))
      assert.equal(report.screenshots.length, 14)
      assert.deepEqual(report.screenshots, fixture.screenshots.map(item => path.basename(item.path)))
      for (const item of fixture.screenshots) {
        assert.equal(path.dirname(item.path), artifactDir)
        assert.equal(item.mask.length, 3)
      }
      assert.equal(fixture.closed(), 1)
    } finally { rmSync(artifactDir, { recursive: true, force: true }) }
  })
}

for (const [corrupt, code] of [['writer', 'WRITER_MISMATCH'], ['log-owner', 'LOG_MISMATCH'],
  ['quota', 'USAGE_MISMATCH'], ['upstream', 'UPSTREAM_MISMATCH'], ['usage', 'RELAY_MISMATCH'], ['replay', 'REQUEST_MISMATCH'], ['strict-reserve', 'USAGE_MISMATCH']]) {
  test(`driver stops on ${corrupt} rather than reporting mocked success`, async () => {
    const fixture = journeyBrowser({ corrupt })
    await assert.rejects(probePersonalBrowserJourney({ ...base, ...fixture }), new RegExp(`^Error: SMOKE_PERSONAL_${code}$`))
    assert.equal(fixture.closed(), 1)
  })
}

test('screenshots reject visible credentials and symlink artifact directories', async () => {
  const root = mkdtempSync(path.join(tmpdir(), 'personal-artifact-contract-'))
  const alias = root + '-alias'
  try {
    const fixture = journeyBrowser({ screenshotSecret: true })
    await assert.rejects(probePersonalBrowserJourney({ ...base, ...fixture, artifactDir: root }), /SMOKE_PERSONAL_SCREENSHOT_REJECTED/)
    assert.equal(fixture.screenshots.length, 0)
    symlinkSync(root, alias)
    await assert.rejects(probePersonalBrowserJourney({ ...base, ...journeyBrowser(), artifactDir: alias }), /SMOKE_PERSONAL_SCREENSHOT_REJECTED/)
  } finally { rmSync(alias, { force: true }); rmSync(root, { recursive: true, force: true }) }
})
