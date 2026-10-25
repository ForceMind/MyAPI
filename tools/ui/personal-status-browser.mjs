// Production-build Chromium with isolated synthetic APIs; never live acceptance.
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { createReadStream, existsSync, mkdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { createServer } from 'node:http'
import { extname, resolve, sep } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

import { createUIFixture, FIXTURE_TIME } from './browser-fixtures.mjs'

const repo = fileURLToPath(new URL('../../', import.meta.url))
const build = resolve(repo, 'web/dist')
const output = resolve(process.env.MYAPI_BROWSER_ARTIFACTS || `${repo}/.local-tests/personal-status-browser`)
const version = readFileSync(resolve(repo, 'VERSION'), 'utf8').trim()
const head = execFileSync('git', ['rev-parse', 'HEAD'], { cwd: repo, encoding: 'utf8' }).trim()
const languageCodes = { en: 'en', zh: 'zhCN' }
const translations = Object.fromEntries(Object.keys(languageCodes).map(language => [
  language,
  JSON.parse(readFileSync(resolve(repo, `web/src/i18n/locales/${language}.json`), 'utf8')).translation,
]))
const tr = (key, language) => {
  const value = translations[language]?.[key]
  assert(typeof value === 'string' && value.trim(), `Missing ${language} translation: ${key}`)
  return value
}
for (const key of ['No-wallet policy active', 'Stored user allowance remains in effect', 'Policy status unknown', 'Call logs', 'Request succeeded', 'Request failed', 'Usage not provided', 'Usage reported', 'Unknown']) {
  for (const language of Object.keys(languageCodes)) tr(key, language)
}

assert(existsSync(resolve(build, 'index.html')), 'Build the production frontend first')
mkdirSync(output, { recursive: true })
const now = FIXTURE_TIME / 1000
const amounts = ['345k', '500k', '765k']
const makeUser = (id, username, requestCount, noBalance) => ({
  id, username, display_name: username, quota: 345000, used_quota: 155000,
  request_count: requestCount, group: 'default', account_tier_id: 'standard',
  aff_count: 2, aff_history_quota: 765000, inviter_id: 0, status: 1, role: 1,
  created_at: now - 3600, last_login_at: now - 60,
  ...(typeof noBalance === 'boolean'
    ? { self_use_no_balance: noBalance, usage_policy_revision: noBalance ? 1 : 0 }
    : {}),
})
const personalUsers = [
  makeUser(201, 'synthetic-no-wallet', 17, true),
  makeUser(202, 'synthetic-stored-allowance', 23, false),
  makeUser(203, 'synthetic-policy-unknown', 31),
]
const commercialUsers = [makeUser(204, 'synthetic-commercial-wallet', 47, true)]
const unconfirmedUsers = [makeUser(205, 'synthetic-unconfirmed-funding', 59, false)]

const makeLog = (id, requestId, other) => ({
  id, user_id: 1, created_at: now - (id - 300) * 60, type: 5, content: requestId,
  username: 'synthetic-operator', token_name: 'synthetic-key', model_name: 'synthetic-text-model',
  quota: 0, prompt_tokens: 0, completion_tokens: 0, use_time: 1, is_stream: false,
  channel: 1, channel_name: 'synthetic-channel', token_id: 21, group: 'default', ip: '',
  request_id: requestId, upstream_request_id: '', other: JSON.stringify(other),
})
const logs = [
  makeLog(301, 'synthetic-completed-usage-pending', {
    request_outcome: 'completed', usage_outcome: 'missing', settlement_status: 'pending_review',
    reserved_quota: 100, actual_quota: null,
  }),
  makeLog(302, 'synthetic-completed-settlement-pending', {
    request_outcome: 'completed', usage_outcome: 'reported', settlement_status: 'pending_review',
    reserved_quota: 100, actual_quota: null,
  }),
  makeLog(303, 'synthetic-real-request-error', {
    request_outcome: 'failed', usage_outcome: 'missing', settlement_status: 'pending_review',
    reserved_quota: 100, actual_quota: null,
  }),
]
const reviews = new Map(logs.map(log => [log.request_id, {
  request_id: log.request_id, user_id: 1, token_id: 21, state: 'usage_unknown',
  reserved_quota: 100, actual_quota: null, text_dispatch_pending: false,
  can_recover_text_dispatch: false, can_reconcile_usage: false,
  settlement_status: 'pending', recovery_block_reason: 'automatic_settlement_pending',
}]))

const report = {
  schema: 1,
  evidence: 'Production-build Chromium with isolated synthetic API responses.',
  scope: 'Synthetic Users funding presentation and usage-log metadata only; excludes live accounts, providers, billing correctness, settlement acceptance, request admission, deployment, and production.',
  mutationPolicy: 'GET only; POST /api/user/auth/refresh is the sole exception.',
  version, head, workflowHead: process.env.GITHUB_SHA || null,
  fixtureTime: new Date(FIXTURE_TIME).toISOString(), result: 'running',
  journeys: [], screenshots: [], errors: [],
}
const persist = () => writeFileSync(resolve(output, 'personal-status-qualification.json'), JSON.stringify(report, null, 2))
const mime = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.json': 'application/json', '.png': 'image/png', '.svg': 'image/svg+xml', '.ico': 'image/x-icon', '.woff2': 'font/woff2' }
const server = createServer((request, response) => {
  let file
  try { file = resolve(build, `.${decodeURIComponent(new URL(request.url || '/', 'http://localhost').pathname)}`) }
  catch { response.writeHead(400).end(); return }
  if (!file.startsWith(`${build}${sep}`) && file !== build) { response.writeHead(403).end(); return }
  if (!existsSync(file) || statSync(file).isDirectory()) file = resolve(build, 'index.html')
  response.writeHead(200, { 'content-type': mime[extname(file)] || 'application/octet-stream', 'cache-control': 'no-store' })
  createReadStream(file).pipe(response)
})

let browser, origin
const contexts = new Set()
function statusResponse(base, funding) {
  const states = {
    personal: { mode: 'disabled', epoch: 7, ready: true, enabled: false },
    commercial: { mode: 'enabled', epoch: 8, ready: true, enabled: true },
    unconfirmed: { mode: 'enabled', epoch: 0, ready: false, enabled: false },
  }
  const state = states[funding]
  assert(state, `Unknown funding fixture: ${funding}`)
  const capabilities = {
    mode: state.mode, epoch: state.epoch, ready: state.ready,
    can_top_up: state.enabled, can_redeem: state.enabled,
    can_transfer_affiliate_rewards: state.enabled,
    can_purchase_subscription: state.enabled, can_view_funding_history: true,
  }
  return { ...base, body: { ...base.body, data: {
    ...base.body.data, quota_display_type: 'TOKENS', display_in_currency: false,
    user_funding_mode: state.mode, user_funding_capabilities: capabilities,
  } } }
}
function fixtureResponse(fixture, url, method, api) {
  const path = url.pathname.replace(/\/$/, '')
  if (path === '/api/status') return statusResponse(fixture.response(url, method), api.funding)
  if (api.users && (path === '/api/user' || path === '/api/user/search')) {
    return { status: 200, body: { success: true, data: { items: api.users, total: api.users.length, page: 1, page_size: 20 } } }
  }
  if (api.logs && (path === '/api/log' || path === '/api/log/self')) {
    return { status: 200, body: { success: true, data: { items: api.logs, total: api.logs.length, page: 1, page_size: 100 } } }
  }
  if (api.logs && path.startsWith('/api/usage-review/')) {
    const review = reviews.get(decodeURIComponent(path.slice('/api/usage-review/'.length)))
    return review ? { status: 200, body: { success: true, data: review } } : { violation: `Unknown usage review: ${path}` }
  }
  return fixture.response(url, method)
}

async function session(scenario) {
  const fixture = createUIFixture({ role: 100, language: languageCodes[scenario.language], personalPolicy: true })
  const mobile = scenario.width === 320
  const audit = { requests: 0, statusRead: false, usersRead: false, logsRead: false, reviewReads: 0, mutations: [], external: [], violations: [], pageErrors: [] }
  const context = await browser.newContext({
    viewport: { width: scenario.width, height: mobile ? 1100 : 900 },
    locale: scenario.language === 'zh' ? 'zh-CN' : 'en-US', timezoneId: 'UTC',
    reducedMotion: 'reduce', colorScheme: 'light', serviceWorkers: 'block',
    hasTouch: mobile, isMobile: mobile,
  })
  contexts.add(context)
  await context.addCookies([{ name: 'vite-ui-theme', value: 'light', url: origin }, { name: 'sidebar_state', value: 'true', url: origin }])
  await context.addInitScript(code => localStorage.setItem('i18nextLng', code), languageCodes[scenario.language])
  await context.route('**/*', async route => {
    const request = route.request(), url = new URL(request.url())
    if (url.origin !== origin) {
      if (['data:', 'blob:'].includes(url.protocol)) return route.continue()
      audit.external.push(url.href); return route.abort('blockedbyclient')
    }
    if (!url.pathname.startsWith('/api/') && !url.pathname.startsWith('/pg/')) return route.continue()
    const path = url.pathname.replace(/\/$/, '')
    const refresh = request.method() === 'POST' && path === '/api/user/auth/refresh'
    audit.requests++
    if (request.method() !== 'GET' && !refresh) {
      audit.mutations.push(`${request.method()} ${path}`)
      return route.fulfill({ status: 501, json: { success: false } })
    }
    audit.statusRead ||= path === '/api/status'
    audit.usersRead ||= path === '/api/user' || path === '/api/user/search'
    audit.logsRead ||= path === '/api/log' || path === '/api/log/self'
    if (path.startsWith('/api/usage-review/')) audit.reviewReads++
    const response = fixtureResponse(fixture, url, request.method(), scenario.api)
    if (response.violation) audit.violations.push(response.violation)
    await route.fulfill({
      status: response.status || 501,
      json: response.body || { success: false, message: response.violation || 'Unconfigured synthetic response' },
    })
  })
  await context.routeWebSocket('**/*', socket => {
    audit.violations.push(`Unexpected WebSocket: ${socket.url()}`); socket.close()
  })
  const page = await context.newPage()
  page.setDefaultTimeout(20000)
  await page.clock.setFixedTime(new Date(FIXTURE_TIME))
  page.on('pageerror', error => audit.pageErrors.push(error.message))
  return { page, context, audit }
}
async function settled(page) {
  await page.evaluate(async () => {
    await document.fonts.ready
    await Promise.all(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished.catch(() => {})))
    await new Promise(done => requestAnimationFrame(() => requestAnimationFrame(done)))
  })
}
async function open(page, path, title) {
  await page.goto(`${origin}${path}`, { waitUntil: 'networkidle' })
  await page.locator('[data-myapi-shell]').waitFor()
  await page.getByRole('heading', { level: 1, name: title, exact: true }).waitFor()
  await settled(page)
  const revision = await page.locator('html').getAttribute('data-build-rev')
  assert(revision?.startsWith(`rv.${version}.`), 'Visible build revision matches VERSION')
  if (process.env.GITHUB_SHA) assert(revision.includes(process.env.GITHUB_SHA), 'Visible build revision matches CI HEAD')
  return revision
}
async function noOverflow(page, name) {
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), true, `${name}: no page-wide horizontal overflow`)
  const dialog = page.locator('[data-slot="dialog-content"]:visible')
  if (await dialog.count()) {
    assert.equal(await dialog.last().evaluate(element => element.scrollWidth <= element.clientWidth + 1 && element.getBoundingClientRect().width <= innerWidth + 1), true, `${name}: dialog fits the viewport`)
  }
}
async function shot(page, scenario, suffix) {
  await noOverflow(page, `${scenario.name}-${suffix}`)
  const file = `${scenario.name}-${suffix}.png`
  await page.screenshot({ path: resolve(output, file) })
  report.screenshots.push(file); persist()
}

async function verifyUsers(page, scenario) {
  const revision = await open(page, '/users', tr('Users', scenario.language))
  const body = () => page.locator('body').innerText()
  if (scenario.kind === 'personal') {
    for (const [key, count] of [['No-wallet policy active', 17], ['Stored user allowance remains in effect', 23], ['Policy status unknown', 31]]) {
      await page.getByText(tr(key, scenario.language), { exact: true }).waitFor()
      await page.getByText(String(count), { exact: true }).waitFor()
    }
    assert.equal(await page.getByRole('progressbar').count(), 0)
    const text = await body()
    for (const amount of amounts) assert(!text.includes(amount), `Personal Users hides ${amount}`)
    assert(!text.includes(`${tr('Revenue', scenario.language)}:`), 'Personal Users hides revenue amount')
    assert.equal(await page.getByRole('button', { name: tr('Open menu', scenario.language), exact: true }).count(), 3)
  } else if (scenario.kind === 'commercial') {
    await page.getByRole('progressbar').waitFor()
    const text = await body()
    for (const amount of amounts) assert(text.includes(amount), `Commercial Users retains ${amount}`)
    assert(text.includes(`${tr('Revenue', scenario.language)}:`), 'Commercial Users retains revenue')
  } else {
    await page.getByText(tr('Policy status unknown', scenario.language), { exact: true }).waitFor()
    await page.getByText('59', { exact: true }).waitFor()
    assert.equal(await page.getByRole('progressbar').count(), 0)
    const text = await body()
    for (const amount of amounts) assert(!text.includes(amount), `Unconfirmed funding hides ${amount}`)
  }
  await shot(page, scenario, 'users')
  return revision
}

async function verifyLogs(page, scenario) {
  const revision = await open(page, '/usage-logs/common', tr('Call records', scenario.language))
  await page.getByText(logs[0].request_id, { exact: true }).first().waitFor()
  await shot(page, scenario, 'logs')
  for (const [index, log] of logs.entries()) {
    const record = page.getByText(log.request_id, { exact: true }).first()
    const row = scenario.width < 1024 ? record.locator('xpath=ancestor::*[@data-log-row]') : record.locator('xpath=ancestor::tr')
    // The request's details open as its own page (design direction 3).
    await row.getByTitle(tr('Click to view full details', scenario.language), { exact: true }).click()
    const detail = page.getByTestId('request-page')
    await detail.getByText(tr(index < 2 ? 'Request succeeded' : 'Request failed', scenario.language), { exact: true }).first().waitFor()
    if (index === 1) {
      // Explicit upstream-reported zero is valid evidence, never Unknown.
      assert.equal(await detail.getByText(tr('Unknown', scenario.language), { exact: true }).count(), 0)
      assert(await detail.getByText('0', { exact: true }).count() >= 2)
    } else {
      assert(await detail.getByText(tr('Unknown', scenario.language), { exact: true }).count() >= 2, 'Missing usage stays unknown')
    }
    assert.equal(await detail.getByText(tr('Total Cost', scenario.language), { exact: true }).count(), 0)
    assert.equal(await page.getByText(tr('Current Balance', scenario.language), { exact: true }).count(), 0)
    if (index === 0) await shot(page, scenario, 'details')
    await page.goBack()
    await detail.waitFor({ state: 'hidden' })
  }
  await noOverflow(page, scenario.name)
  return revision
}

async function run(scenario) {
  const journey = { name: scenario.name, language: scenario.language, width: scenario.width, result: 'running' }
  const { page, context, audit } = await session(scenario)
  try {
    journey.buildRevision = await (scenario.kind === 'logs' ? verifyLogs(page, scenario) : verifyUsers(page, scenario))
    assert(audit.statusRead, 'Journey reads funding status')
    assert(scenario.kind === 'logs' ? audit.logsRead : audit.usersRead, 'Journey reads its bounded API')
    assert.deepEqual(audit.mutations, [], 'No business POST/PUT/PATCH/DELETE')
    assert.deepEqual(audit.external, [], 'No external requests')
    assert.deepEqual(audit.violations, [], 'No fixture violations')
    assert.deepEqual(audit.pageErrors, [], 'No page runtime errors')
    journey.requestCount = audit.requests; journey.result = 'passed'
  } catch (error) {
    journey.result = 'failed'; journey.error = error instanceof Error ? error.message : String(error); throw error
  } finally {
    report.journeys.push(journey); persist(); await context.close(); contexts.delete(context)
  }
}

try {
  await new Promise(done => server.listen(0, '127.0.0.1', done))
  origin = `http://127.0.0.1:${server.address().port}`
  const { chromium } = await import(process.env.MYAPI_PLAYWRIGHT_MODULE ? pathToFileURL(process.env.MYAPI_PLAYWRIGHT_MODULE).href : 'playwright')
  browser = await chromium.launch({ headless: true })
  const scenarios = [
    { name: 'users-personal-en-1280', language: 'en', width: 1280, kind: 'personal', api: { funding: 'personal', users: personalUsers } },
    { name: 'users-personal-zh-320', language: 'zh', width: 320, kind: 'personal', api: { funding: 'personal', users: personalUsers } },
    { name: 'users-commercial-en-1280', language: 'en', width: 1280, kind: 'commercial', api: { funding: 'commercial', users: commercialUsers } },
    { name: 'users-unconfirmed-zh-320', language: 'zh', width: 320, kind: 'unconfirmed', api: { funding: 'unconfirmed', users: unconfirmedUsers } },
    { name: 'logs-status-en-1280', language: 'en', width: 1280, kind: 'logs', api: { funding: 'personal', logs } },
    { name: 'logs-status-zh-320', language: 'zh', width: 320, kind: 'logs', api: { funding: 'personal', logs } },
  ]
  for (const scenario of scenarios) await run(scenario)
  assert.equal(report.journeys.length, 6); assert.equal(report.screenshots.length, 8)
  report.result = 'passed'
} catch (error) {
  report.result = 'failed'; report.errors.push(error instanceof Error ? error.message : String(error)); throw error
} finally {
  for (const context of contexts) await context.close().catch(() => {})
  if (browser) await browser.close()
  if (server.listening) await new Promise(done => server.close(done))
  persist(); console.log(JSON.stringify(report))
}
