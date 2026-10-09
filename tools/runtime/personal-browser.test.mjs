import assert from 'node:assert/strict'
import test from 'node:test'
import { browserSetup, probePersonalBrowserJourney } from './personal-browser.mjs'

// These doubles protect driver orchestration and artifact hygiene only. They do
// not constitute browser, backend, relay, billing, or persistence acceptance.
const base = { baseUrl: 'http://127.0.0.1:18080', upstreamBaseUrl: 'http://127.0.0.1:19090',
  sha: 'a'.repeat(40), edition: 'full', isolated: true,
  username: 'synthetic-root', password: 'synthetic-password-never-report', writer: 'legacy', onProgress() {} }

function setupBrowser({ failAt, responseSuccess = true, closeFails = false } = {}) {
  const calls = []
  let route
  const locator = (kind, value) => ({
    async waitFor() {},
    async fill(text) { calls.push(['fill', kind, value, text]) },
    async click() {
      calls.push(['click', kind, value])
      // Base UI's public id belongs to an aria-hidden, clipped input. A real
      // pointer cannot use it; select the visible radio role instead.
      if (kind === 'selector' && /^#usage-mode-/.test(value)) throw new Error('hidden radio input')
      if (value === failAt) throw new Error(base.password)
    },
    async getAttribute(name) { return name === 'aria-checked' ? 'true' : null },
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
    async close() { calls.push(['browser-close']); if (closeFails) throw new Error(`close ${base.password}`) },
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
    assert(fixture.calls.some(call => call[0] === 'click' && call[1] === 'radio' && call[2] instanceof RegExp && call[2].test(edition === 'lan' ? 'Personal use' : 'External operations')))
    assert(!fixture.calls.some(call => call[1] === 'selector' && String(call[2]).startsWith('#usage-mode-')))
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
      assert.equal(error.stage, options.failAt ? 'setup-database' : 'setup-response')
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

function journeyBrowser({ writer = 'legacy', corrupt, screenshotSecret = false, closeFails = false, deferSwitchUpdate = false, deferDialogClose = false, forbidStreamReads = false, hangAt, closeHangs = false } = {}) {
  const reverse = Object.fromEntries(Object.entries(JSON.parse(readFileSync(new URL('../../web/src/i18n/locales/zh.json', import.meta.url))).translation).map(([key, value]) => [value, key]))
  const version = readFileSync(new URL('../../VERSION', import.meta.url), 'utf8').trim()
  const users = new Map([[1, { id: 1, username: base.username, role: 100, quota: 0, used_quota: 15, request_count: 1, self_use_no_balance: true, revision: 1 }]])
  const keys = new Map()
  const logs = new Map()
  const contexts = []
  const emitted = []
  const screenshots = []
  const apiCalls = []
  const dialogCloseEvents = []
  const completionEvents = []
  const signals = []
  const reached = Promise.withResolvers()
  const stalled = Promise.withResolvers()
  const closing = Promise.withResolvers()
  const stalledClose = Promise.withResolvers()
  const hang = () => { reached.resolve(); return stalled.promise }
  let upstreamCount = 2
  let closed = 0
  let sequence = 0
  const clone = value => JSON.parse(JSON.stringify(value))
  const response = (pathname, method, payload, status = 200, body, headers = {}) => ({
    url: () => base.baseUrl + pathname,
    request: () => ({ method: () => method, postDataJSON: () => body, headers: () => headers, url: () => base.baseUrl + pathname }),
    status: () => status, json: async () => hangAt === 'response-json' && pathname === '/api/user/login' ? hang() : clone(payload),
    headers: () => ({ 'x-oneapi-request-id': `request-private-${sequence}` }),
    text: async () => {
      assert(!(forbidStreamReads && typeof payload === 'string'), 'successful UI stream body is not a completion signal')
      if (hangAt === 'rejection-body' && pathname === '/pg/chat/completions' && status !== 200) return hang()
      return typeof payload === 'string' ? payload : JSON.stringify(payload)
    },
    finished: async () => {
      assert(!(forbidStreamReads && typeof payload === 'string'), 'frontend may close its SSE request after DONE')
      return null
    },
  })
  const policy = user => ({ user_id: user.id, no_balance: user.self_use_no_balance, revision: user.revision, legacy_remaining_quota: user.quota })
  const budget = key => ({ policy: { enabled: true, fee_enabled: true, used: 0, reserved: corrupt === 'strict-reserve' ? 1 : 0, fee_used_usd: '0', fee_reserved_usd: '0', pending_request_id: '' }, pending: null })
  const makePage = context => {
    const page = { current: base.baseUrl + '/', fields: {}, switches: {}, pendingSwitches: {}, checkboxes: {}, group: '', model: 'gpt-4o', listeners: {}, waiters: [], selected: '', actor: null,
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
        if (hangAt === 'evaluate') return hang()
        if (String(fn).includes('__APP_BUILD__')) return { global: `rv.${version}.${base.sha}.test`, html: `rv.${version}.${base.sha}.test`, meta: `rv.${version}.${base.sha}.test` }
        return !screenshotSecret
      },
      async screenshot(options) { screenshots.push(options) },
      keyboard: { async press(key) {
        const mutation = page.dialogMutation
        if (key !== 'Escape' || !mutation || mutation.closed) return
        if (mutation.pending) return // The real onOpenChange ignores close while sending.
        mutation.closed = true
        dialogCloseEvents.push([mutation.title, 'escape'])
      } },
    }
    const normalize = value => context.locale === 'zh-CN' ? reverse[value] || value : value
    const locator = (kind, value, parents = []) => {
      value = normalize(value)
      const chain = [...parents, value]
      let awaitingCheckedState = false
      return {
        getByRole: (role, options) => locator(role, options?.name, chain),
        getByText: (text) => locator('text', text, chain),
        locator: (selector) => locator('selector', selector, chain),
        and() { awaitingCheckedState = true; return this },
        filter() { return this }, first() { return this }, last() { return this },
        async waitFor(options) {
          if (kind === 'text' && value === 'synthetic fixed response') {
            assert.notEqual(corrupt, 'incomplete-text')
            completionEvents.push('assistant-complete')
          }
          if (kind === 'button' && value === 'Stop' && options?.state === 'hidden') {
            assert.notEqual(corrupt, 'still-generating')
            completionEvents.push('stop-hidden')
          }
          if (kind === 'button' && value === 'Send') {
            assert.notEqual(corrupt, 'missing-send')
            completionEvents.push('send-visible')
          }
          if (awaitingCheckedState && page.pendingSwitches[value] !== undefined) {
            page.switches[value] = page.pendingSwitches[value]; delete page.pendingSwitches[value]
          }
          const mutation = page.dialogMutation
          if (value === '[data-slot="dialog-close"]' && options?.state === 'visible' && parents.includes(mutation?.title) && mutation.pending) {
            mutation.pending = false
            dialogCloseEvents.push([mutation.title, 'close-visible'])
          }
          if (kind === 'dialog' && options?.state === 'hidden' && value === mutation?.title) {
            assert.equal(mutation.closed, true, 'pending mutation prevented dialog dismissal')
          }
        },
        async count() {
          if (kind === 'selector' && value === '.is-assistant') return page.assistants || 0
          if (kind === 'alert') return parents.includes('.is-assistant') ? Number(Boolean(page.lastAssistantError)) : page.historicalErrors || 0
          return 0
        },
        async fill(text) { page.fields[value] = text },
        async check() { page.checkboxes[value] = true }, async uncheck() { page.checkboxes[value] = false },
        async getAttribute(name) { if (name === 'aria-checked') return page.switches[value] ?? (value === 'No ordinary quota cap' ? 'true' : 'false') },
        async inputValue() { return page.selected },
        async isDisabled() { return page.selected === '' },
        async selectOption(id) {
          page.selected = id
          if (id) {
            page.model = corrupt === 'models' ? 'other-model' : 'smoke-model'
            page.emit(response('/pg/models', 'GET', { success: true, data: [{ id: page.model }] }))
          }
        },
        async click(options) {
          if (options?.trial) return
          if (kind === 'switch') (deferSwitchUpdate ? page.pendingSwitches : page.switches)[value] = (page.switches[value] ?? (value === 'No ordinary quota cap' ? 'true' : 'false')) === 'true' ? 'false' : 'true'
          if (value === 'form button[type="submit"]') {
            const name = page.fields['form input[name="username"]']
            page.actor = [...users.values()].find(user => user.username === name)
            assert(page.actor)
            const token = `session-private-${page.actor.id}`
            page.current = base.baseUrl + '/keys'
            page.emit(response('/api/user/login', 'POST', { success: true, data: { access_token: token, user: clone(page.actor) } }))
          }
          if (value === 'Create API Key') { page.switches['No ordinary quota cap'] = 'true'; page.group = '' }
          if (kind === 'option') page.group = 'default'
          if (value === 'Save changes' || value === 'Save Changes') {
            const id = keys.size + 31
            const unlimited = page.switches['No ordinary quota cap'] !== 'false'
            const data = { name: page.fields.Name, remain_quota: unlimited ? 0 : Number(page.fields['Key quota (internal units)']), unlimited_quota: unlimited, group: page.group }
            const key = { ...data, id, user_id: page.actor.id, used_quota: 0, key: 'secret-masked-key-do-not-report' }
            keys.set(id, key)
            page.emit(response('/api/token/', 'POST', { success: true }, 200, data))
          }
          if (value === 'Save' && chain.includes('User usage policy')) {
            assert.equal(page.actor.role, 100)
            assert.equal(page.checkboxes['Use Key limits without a user wallet'], true)
            assert.equal(page.checkboxes['I confirm all running instances support this policy and I understand the supported request paths.'], true)
            const owner = users.get(20)
            owner.self_use_no_balance = true
            owner.revision += 1
            if (deferDialogClose) {
              page.dialogMutation = { title: 'User usage policy', pending: true, closed: false }
              dialogCloseEvents.push([page.dialogMutation.title, 'response-pending'])
            }
            page.emit(response('/api/user/20/usage-policy', 'PUT', { success: true, data: policy(owner) }))
          }
          if (value === 'Save' && chain.includes('API Key usage budgets')) {
            assert.equal(page.actor.role, 100)
            assert.equal(page.checkboxes['Enable strict Token budget'], true)
            assert.equal(page.checkboxes['Enable USD fee budget'], true)
            assert.equal(page.checkboxes['I confirm all running instances support this budget and I understand its request restrictions.'], true)
            assert.equal(page.fields['Token total limit'], '1000')
            assert.equal(page.fields['USD total limit'], '1')
            const key = [...keys.values()].find(key => key.name === 'browser-root-strict-reject')
            key.strict = true
            if (deferDialogClose) {
              page.dialogMutation = { title: 'API Key usage budgets', pending: true, closed: false }
              dialogCloseEvents.push([page.dialogMutation.title, 'response-pending'])
            }
            page.emit(response(`/api/token/${key.id}/budget`, 'PUT', { success: true, data: budget(key) }))
          }
          if (value === 'Send') {
            page.assistants = (page.assistants || 0) + 1
            const key = keys.get(Number(page.selected))
            assert(key)
            sequence++
            const sent = { model: page.model, max_tokens: Number(page.fields['Max Tokens']), stream: true, messages: [{ role: 'user', content: 'synthetic request' }] }
            const headers = { 'x-myapi-key-id': String(key.id) }
            let code
            if (!page.actor.self_use_no_balance) code = 'insufficient_user_quota'
            else if (key.remain_quota < 500) code = writer === 'legacy' ? 'pre_consume_token_quota_failed' : 'insufficient_user_quota'
            else if (key.strict) code = 'token_budget_unsupported_request'
            page.lastAssistantError = Boolean(code) || corrupt === 'latest-error'
            if (code) page.historicalErrors = (page.historicalErrors || 0) + 1
            if (code) page.emit(response('/pg/chat/completions', 'POST', { error: { code, message: 'synthetic rejection' } }, code.startsWith('token_budget_') ? 400 : 403, sent, headers))
            else {
              upstreamCount++
              key.remain_quota -= 15; key.used_quota += 15
              page.actor.used_quota += 15; page.actor.request_count++
              const requestId = `request-private-${sequence}`
              logs.set(requestId, { request_id: requestId, user_id: page.actor.id, token_id: key.id, token_name: key.name,
                model_name: 'smoke-model', quota: 15, prompt_tokens: 10, completion_tokens: 5, other: '{"billing_source":"self_use"}' })
              if (corrupt === 'log-owner') logs.get(requestId).user_id = 999
              if (corrupt === 'usage') logs.get(requestId).completion_tokens = 6
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
        async addInitScript(fn, args) { this.init = { fn: String(fn), args } }, async newPage() { this.page = makePage(this); return this.page },
        async close() { this.closed = true } }
      contexts.push(context)
      return context
    }, async close() {
      closed++
      closing.resolve()
      if (closeFails) throw new Error(`close ${base.password}`)
      if (closeHangs) return stalledClose.promise
    },
  } } } }
  const fetchImpl = async (url, options) => {
    assert.equal(options.redirect, 'error')
    assert(options.signal instanceof AbortSignal)
    signals.push(options.signal)
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
    if (hangAt === 'api-body' && route === 'GET /api/quota-writer/status') return { status: 200, json: hang }
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
  return { playwrightModule, fetchImpl, contexts, emitted, screenshots, apiCalls, dialogCloseEvents, completionEvents, signals,
    reached: reached.promise, release: stalled.resolve, closing: closing.promise, releaseClose: stalledClose.resolve,
    closed: () => closed, upstreamCount: () => upstreamCount }
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

for (const [corrupt, code] of [['writer', 'WRITER_MISMATCH'], ['models', 'REQUEST_MISMATCH'], ['log-owner', 'LOG_MISMATCH'],
  ['quota', 'USAGE_MISMATCH'], ['upstream', 'UPSTREAM_MISMATCH'], ['usage', 'LOG_MISMATCH'], ['replay', 'REQUEST_MISMATCH'], ['strict-reserve', 'USAGE_MISMATCH'],
  ['incomplete-text', 'BROWSER_FAILED'], ['still-generating', 'BROWSER_FAILED'], ['missing-send', 'BROWSER_FAILED'], ['latest-error', 'RELAY_MISMATCH']]) {
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


for (const entry of ['setup', 'journey']) {
  test(`browser close failure rejects a completed ${entry} without exposing diagnostics`, async () => {
    const fixture = entry === 'setup' ? setupBrowser({ closeFails: true }) : journeyBrowser({ closeFails: true })
    const run = entry === 'setup'
      ? browserSetup({ ...base, playwrightModule: fixture.module })
      : probePersonalBrowserJourney({ ...base, ...fixture })
    await assert.rejects(run, error => {
      assert.equal(error.message, 'SMOKE_PERSONAL_BROWSER_CLOSE_FAILED')
      assert.equal(error.stage, 'browser-close')
      assert.equal(error.cause, undefined)
      assert(!error.stack.includes(base.password))
      assert(!JSON.stringify(error).includes(base.password))
      return true
    })
    if (entry === 'setup') assert.equal(fixture.calls.filter(call => call[0] === 'browser-close').length, 1)
    else assert.equal(fixture.closed(), 1)
  })
}

test('the original safe failure takes priority when browser cleanup also fails', async () => {
  for (const [run, fixture, code, stage] of [
    [browserSetup, setupBrowser({ responseSuccess: false, closeFails: true }), 'SETUP_FAILED', 'setup-response'],
    [probePersonalBrowserJourney, journeyBrowser({ corrupt: 'writer', closeFails: true }), 'WRITER_MISMATCH', 'writer-check'],
  ]) {
    await assert.rejects(run({ ...base, ...fixture, playwrightModule: fixture.module || fixture.playwrightModule }), error => {
      assert.equal(error.message, `SMOKE_PERSONAL_${code}`)
      assert.equal(error.stage, stage)
      assert.equal(error.cause, undefined)
      assert(!error.stack.includes(base.password))
      return true
    })
    if (fixture.calls) assert.equal(fixture.calls.filter(call => call[0] === 'browser-close').length, 1)
    else assert.equal(fixture.closed(), 1)
  }
})


test('finite Key creation waits for controlled switch state before reading it', async () => {
  const fixture = journeyBrowser({ deferSwitchUpdate: true })
  const report = await probePersonalBrowserJourney({ ...base, ...fixture })
  assert.equal(report.passed, true)
  assert.equal(fixture.upstreamCount(), 4)
})

test('policy and budget dialogs wait for mutation settlement before Escape dismissal', async () => {
  const fixture = journeyBrowser({ deferDialogClose: true })
  const report = await probePersonalBrowserJourney({ ...base, ...fixture })
  assert.equal(report.passed, true)
  assert.deepEqual(fixture.dialogCloseEvents, ['User usage policy', 'API Key usage budgets'].flatMap(title => [
    [title, 'response-pending'], [title, 'close-visible'], [title, 'escape'],
  ]))
})

test('successful streaming uses complete UI state and ledger without reading the closed SSE body', async () => {
  const fixture = journeyBrowser({ forbidStreamReads: true })
  const report = await probePersonalBrowserJourney({ ...base, ...fixture })
  assert.equal(report.passed, true)
  assert.deepEqual(fixture.completionEvents, Array.from({ length: 2 }, () => ['assistant-complete', 'stop-hidden', 'send-visible']).flat())
  assert.equal(fixture.upstreamCount(), 4)
})

test('progress contains fixed stages and safe metadata only, including immediate cleanup failure', async () => {
  const events = []
  const fixture = journeyBrowser({ corrupt: 'writer', closeFails: true })
  await assert.rejects(probePersonalBrowserJourney({ ...base, ...fixture, onProgress: event => events.push(event) }), /SMOKE_PERSONAL_WRITER_MISMATCH/)
  assert(events.some(event => event.stage === 'writer-check' && event.event === 'stage'))
  assert.deepEqual(events.at(-1), { command: 'personal:browser', phase: 'journey', event: 'cleanup-failed',
    stage: 'browser-close', sha: base.sha, writer: 'legacy', code: 'SMOKE_PERSONAL_BROWSER_CLOSE_FAILED' })
  for (const event of events) {
    assert.deepEqual(Object.keys(event).sort(), (event.event === 'cleanup-failed'
      ? ['command', 'phase', 'event', 'stage', 'sha', 'writer', 'code']
      : ['command', 'phase', 'event', 'stage', 'sha', 'writer']).sort())
  }
  for (const secret of [base.password, base.username, 'session-private-', 'secret-masked-key', 'request-private-']) assert(!JSON.stringify(events).includes(secret))
})

test('the default progress sink emits safe JSON while a broken observer cannot block cleanup', async t => {
  const records = []
  t.mock.method(console, 'error', line => records.push(JSON.parse(line)))
  const fixture = setupBrowser()
  await browserSetup({ ...base, playwrightModule: fixture.module, onProgress: undefined })
  assert.equal(records[0].stage, 'launch')
  assert.equal(records.at(-1).stage, 'browser-close')
  assert(records.every(record => record.phase === 'setup' && record.sha === base.sha && record.writer === 'legacy'))
  assert(!JSON.stringify(records).includes(base.password))
  const failed = setupBrowser({ responseSuccess: false, closeFails: true })
  await assert.rejects(browserSetup({ ...base, playwrightModule: failed.module,
    onProgress() { throw new Error(base.password) } }), /^Error: SMOKE_PERSONAL_SETUP_FAILED$/)
  assert.equal(failed.calls.at(-1)[0], 'browser-close')
})

for (const [hangAt, timeout, stage] of [
  ['response-json', 20_000, 'login'], ['rejection-body', 20_000, 'playground-rejection'],
  ['api-body', 10_000, 'writer-check'], ['evaluate', 20_000, 'login'],
]) {
  test(`${hangAt} deadline actively closes owned contexts and late resolution cannot resume business`, async t => {
    t.mock.timers.enable({ apis: ['setTimeout', 'Date'] })
    const fixture = journeyBrowser({ hangAt })
    let outcome
    const run = probePersonalBrowserJourney({ ...base, ...fixture }).then(
      value => { outcome = { value } }, error => { outcome = { error } })
    await fixture.reached
    const calls = fixture.apiCalls.length
    const requests = fixture.emitted.length
    t.mock.timers.tick(timeout)
    await new Promise(setImmediate)
    assert.equal(outcome?.error?.message, 'SMOKE_PERSONAL_TIMEOUT')
    assert.equal(outcome.error.stage, stage)
    assert(fixture.contexts.every(context => context.closed))
    assert(fixture.signals.every(signal => signal.aborted))
    assert.equal(fixture.closed(), 1)
    fixture.release('{}')
    await run
    await new Promise(setImmediate)
    assert.equal(fixture.apiCalls.length, calls)
    assert.equal(fixture.emitted.length, requests)
  })
}

for (const corrupt of [undefined, 'writer']) {
  test(`hung browser cleanup reports immediately and preserves ${corrupt ? 'original failure' : 'failure instead of success'}`, async t => {
    t.mock.timers.enable({ apis: ['setTimeout', 'Date'] })
    const fixture = journeyBrowser({ corrupt, closeHangs: true })
    const events = []
    let outcome
    const run = probePersonalBrowserJourney({ ...base, ...fixture, onProgress: event => events.push(event) }).then(
      value => { outcome = { value } }, error => { outcome = { error } })
    await fixture.closing
    t.mock.timers.tick(10_000)
    await new Promise(setImmediate)
    assert.equal(outcome?.error?.message, `SMOKE_PERSONAL_${corrupt ? 'WRITER_MISMATCH' : 'BROWSER_CLOSE_FAILED'}`)
    assert.equal(outcome.error.stage, corrupt ? 'writer-check' : 'browser-close')
    assert.equal(events.at(-1).event, 'cleanup-failed')
    assert.equal(events.at(-1).code, 'SMOKE_PERSONAL_BROWSER_CLOSE_FAILED')
    assert.equal(fixture.closed(), 1)
    fixture.releaseClose()
    await run
    assert.equal(outcome.value, undefined)
  })
}

test('ledger retries share a total deadline and abort a slow later body without another request', async t => {
  t.mock.timers.enable({ apis: ['setTimeout', 'Date'] })
  const fixture = journeyBrowser()
  const firstRead = Promise.withResolvers()
  const secondRead = Promise.withResolvers()
  const stalled = Promise.withResolvers()
  let reads = 0
  let signal
  const fetchImpl = async (url, options) => {
    if (new URL(url).pathname !== '/api/log/self') return fixture.fetchImpl(url, options)
    reads++
    signal = options.signal
    if (reads === 1) return { status: 200, json: async () => {
      t.mock.timers.tick(6_000)
      firstRead.resolve()
      return { success: true, data: { total: 0, items: [] } }
    } }
    secondRead.resolve()
    return { status: 200, json: () => stalled.promise }
  }
  let outcome
  const run = probePersonalBrowserJourney({ ...base, ...fixture, fetchImpl }).then(
    value => { outcome = { value } }, error => { outcome = { error } })
  await firstRead.promise
  await new Promise(setImmediate)
  t.mock.timers.tick(100)
  await secondRead.promise
  t.mock.timers.tick(3_900)
  await new Promise(setImmediate)
  assert.equal(outcome?.error?.message, 'SMOKE_PERSONAL_TIMEOUT')
  assert.equal(outcome.error.stage, 'ledger-check')
  assert.equal(signal.aborted, true)
  assert.equal(reads, 2)
  assert(fixture.contexts.every(context => context.closed))
  stalled.resolve({ success: true, data: { total: 0, items: [] } })
  await run
  assert.equal(reads, 2)
})
