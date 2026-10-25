// Step 4 (upstream grid, connection page, account windows, model catalog,
// overview failure link, pagination, profile subscription) screenshots and
// browser checks against the synthetic preview (tools/ui/page-layout-preview.mjs
// on 127.0.0.1:4184). Every other origin is aborted. The upstream scenario
// data and the writes exercised here (enable switch, Test) are answered in
// the browser only; the preview itself stays read-only.
// Usage: node shots.mjs <outDir> [group...]
//   groups: pages widths tabs states ops overview pagination profile
import { mkdirSync, writeFileSync } from 'node:fs'
import path from 'node:path'

const { chromium } = await import(process.env.MYAPI_PLAYWRIGHT_MODULE)
const ORIGIN = 'http://127.0.0.1:4184'
const outDir = process.argv[2]
const groups = new Set(process.argv.slice(3).length ? process.argv.slice(3) : ['pages', 'widths', 'tabs', 'states', 'ops', 'overview', 'pagination', 'profile'])
mkdirSync(outDir, { recursive: true })

const LITE_CAPS = Object.fromEntries(['payments', 'commercial_subscriptions', 'redemption_codes', 'checkin', 'affiliate', 'public_pricing', 'rankings', 'perf_metrics', 'external_oauth', 'midjourney', 'model_deployments', 'site_content', 'user_registration'].map((k) => [k, false]))
const NOW = Math.floor(Date.now() / 1000)
const browser = await chromium.launch()
const checks = []
const shots = []
let blocked = 0
const check = (name, pass, detail) => {
  checks.push({ name, pass: Boolean(pass), detail })
  if (!pass) console.log('FAIL', name, JSON.stringify(detail).slice(0, 600))
}

// Synthetic upstream: two subscription accounts (one in use, one nearly
// exhausted), an API Key connection with a saved test, a local model server
// that is turned off, an auto-disabled connection and a long name.
const base = { group: 'default', weight: 0, key: '', created_time: 1, balance_updated_time: 0, channel_info: { is_multi_key: false, multi_key_size: 0, multi_key_polling_index: 0, multi_key_mode: 'random' } }
const CONNECTIONS = [
  { ...base, id: 11, type: 57, name: '示例订阅账号 A · Subscription A', status: 1, priority: 10, models: 'gpt-5-codex,gpt-5,gpt-5-mini', test_time: NOW - 120, response_time: 412 },
  { ...base, id: 12, type: 1, name: '示例 API 连接 B · API connection B', status: 1, priority: 8, models: 'synthetic-text-model,synthetic-vision-model,synthetic-embed-model,synthetic-audio-model,synthetic-image-model', test_time: NOW - 3600, response_time: 1830 },
  { ...base, id: 13, type: 4, name: '示例本地模型 C · Local C', status: 2, priority: 5, models: 'llama3,qwen2.5', test_time: 0, response_time: 0 },
  { ...base, id: 14, type: 57, name: '示例订阅账号 D · Subscription D', status: 1, priority: 6, models: 'gpt-5-codex,gpt-5', test_time: NOW - 720, response_time: 380 },
  { ...base, id: 15, type: 14, name: '自动禁用的连接 E · Auto disabled E', status: 3, priority: 4, models: 'claude-sonnet-4', test_time: NOW - 86400, response_time: 0 },
  { ...base, id: 16, type: 1, name: 'Synthetic API connection with a very long provider name that must not push the card actions off screen · 很长的连接名称', status: 1, priority: null, models: 'a-very-long-model-identifier-that-must-remain-readable-at-mobile-width', test_time: 0, response_time: 0 },
]
const windowItem = (channelId, windowType, seconds, available, extra = {}) => ({
  channel_id: channelId, series_id: `${channelId}${windowType}`.padEnd(64, '0'), name: CONNECTIONS.find((c) => c.id === channelId).name,
  account_label: CONNECTIONS.find((c) => c.id === channelId).name, metric_type: 'codex_rate_limit', window_type: windowType,
  source: seconds === 18000 ? 'codex_wham_usage_primary' : 'codex_wham_usage_secondary', window_seconds: seconds, unit: 'percent',
  status: 'success', current_available: available, observed_at: NOW - 90, previous_available: Math.min(100, available + 6), previous_observed_at: NOW - 390, ...extra,
})
const WINDOWS = [
  windowItem(11, 'five_hour', 18000, 18), windowItem(11, 'weekly', 604800, 45),
  windowItem(14, 'five_hour', 18000, 88), windowItem(14, 'weekly', 604800, 70, { status: 'error', current_available: null }),
]
const logRow = (id, channel, other = {}, type = 2, extra = {}) => ({
  id, user_id: 1, created_at: NOW - id * 60, type, content: '', username: 'synthetic-admin', token_name: id % 2 ? '示例 Key · 团队机器人' : '示例 Key · 本机 CLI', token_id: id % 2 ? 21 : 22,
  model_name: channel === 12 ? 'synthetic-text-model' : 'gpt-5-codex', quota: 0, prompt_tokens: 120, completion_tokens: 520, use_time: 2, is_stream: true,
  channel, channel_name: CONNECTIONS.find((c) => c.id === channel)?.name ?? '', group: 'default', ip: '',
  other: JSON.stringify({ request_outcome: 'completed', usage_outcome: 'reported', http_status: 200, ...other }), request_id: `SYNTHETIC-UP-${id}`, upstream_request_id: '', ...extra,
})
const LOGS = [
  logRow(1, 11), logRow(2, 11, { request_outcome: 'failed', http_status: 500, usage_outcome: 'missing' }, 5, { content: 'status_code=500' }),
  logRow(3, 11, { usage_outcome: 'missing' }, 2, { prompt_tokens: 0, completion_tokens: 0 }), logRow(4, 12),
  logRow(5, 12, { request_outcome: 'rejected', preflight_rejected: true }, 5, { content: 'input limit' }),
  logRow(6, 11, { settlement_status: 'pending_review' }, 5, { content: 'usage_pending_review' }),
]
const isFailure = (row) => row.type === 5 && !['usage_settlement_pending', 'usage_pending_review'].includes(row.content) && !row.other.includes('"preflight_rejected":true')
const SUBSCRIPTION = { id: 3, user_id: 1, plan_id: 2, status: 'active', source: 'admin', start_time: NOW - 86400 * 10, end_time: NOW + 86400 * 20, amount_total: 5000000, amount_used: 1250000, next_reset_time: NOW + 3600 * 5 }

// Answers that replace the preview's, per scenario.
function upstreamAnswer(p, url, method, state) {
  const ok = (data) => ({ status: 200, json: { success: true, data } })
  if (state.denied && p === '/api/user/self') return { status: 200, json: { success: false, message: 'Synthetic: access changed' } }
  if (method === 'GET' && (p === '/api/channel' || p === '/api/channel/search')) {
    if (state.listError) return { status: 200, json: { success: false, message: 'Synthetic failure' } }
    let items = state.empty ? [] : CONNECTIONS.map((c) => ({ ...c, status: state.status[c.id] ?? c.status, test_time: state.tested[c.id] ?? c.test_time }))
    const status = url.searchParams.get('status')
    if (status === 'enabled') items = items.filter((c) => c.status === 1)
    if (status === 'disabled') items = items.filter((c) => c.status !== 1)
    const keyword = url.searchParams.get('keyword')
    if (keyword) items = items.filter((c) => c.name.toLowerCase().includes(keyword.toLowerCase()) || String(c.id) === keyword)
    const pageSize = Number(url.searchParams.get('page_size') || 20)
    const pageNo = Number(url.searchParams.get('p') || 1)
    return ok({ total: items.length, page: pageNo, page_size: pageSize, items: items.slice((pageNo - 1) * pageSize, pageNo * pageSize) })
  }
  const one = p.match(/^\/api\/channel\/(\d+)$/)
  if (method === 'GET' && one) {
    const c = CONNECTIONS.find((item) => item.id === Number(one[1]))
    if (!c || state.detailError) return { status: 200, json: { success: false, message: 'Synthetic failure' } }
    return ok({ ...c, status: state.status[c.id] ?? c.status, test_time: state.tested[c.id] ?? c.test_time })
  }
  if (method === 'GET' && p === '/api/channel/quota/changes') {
    if (state.windowsError) return { status: 200, json: { success: false } }
    const ids = (url.searchParams.get('channel_ids') || '').split(',').filter(Boolean).map(Number)
    return ok({ items: WINDOWS.filter((w) => !ids.length || ids.includes(w.channel_id)), generated_at: NOW, items_complete: true, source_complete: true })
  }
  if (method === 'GET' && (p === '/api/log' || p === '/api/log/self') && !url.searchParams.has('request_id')) {
    let rows = LOGS
    const channel = url.searchParams.get('channel')
    if (channel) rows = rows.filter((r) => String(r.channel) === channel)
    const type = url.searchParams.get('type')
    if (type) rows = rows.filter((r) => String(r.type) === type)
    if (url.searchParams.get('result') === 'failed') { state.failedReads++; rows = rows.filter(isFailure) }
    const tokenId = url.searchParams.get('token_id')
    if (tokenId) rows = rows.filter((r) => String(r.token_id) === tokenId)
    const size = Number(url.searchParams.get('page_size') || 20)
    return ok({ items: rows.slice(0, size), total: rows.length, page: 1, page_size: size })
  }
  const test = p.match(/^\/api\/channel\/test\/(\d+)$/)
  if (test) {
    state.writes.push({ test: Number(test[1]) })
    state.tested[Number(test[1])] = NOW
    return Number(test[1]) === 12
      ? { status: 200, json: { success: false, message: 'Synthetic upstream rejected the key', error_code: 'invalid_api_key' } }
      : { status: 200, json: { success: true, time: 0.41 } }
  }
  const status = p.match(/^\/api\/channel\/(\d+)\/status$/)
  if (method === 'POST' && status) {
    const body = JSON.parse(state.lastBody || '{}')
    state.writes.push({ status: Number(status[1]), value: body.status })
    state.status[Number(status[1])] = body.status
    return ok(true)
  }
  if (method === 'GET' && p === '/api/subscription/self') {
    return ok({ billing_preference: 'subscription_first', subscriptions: state.noSubscription ? [] : [{ subscription: SUBSCRIPTION }], all_subscriptions: state.noSubscription ? [] : [{ subscription: SUBSCRIPTION }] })
  }
  return undefined
}

const scenarios = {
  lite: (p, json) => { if (p === '/api/status' && json.data) Object.assign(json.data, { product_edition: 'lite', capabilities: LITE_CAPS }) },
  user: (p, json) => {
    if ((p === '/api/user/self' || p === '/api/user/auth/refresh') && json.data) {
      const user = json.data.user ?? json.data
      if (user && typeof user === 'object' && 'role' in user) { user.role = 1; user.permissions = { sidebar_settings: true, admin_permissions: {} } }
    }
  },
}

async function open(route, { lang = 'zh', theme = 'light', width = 1280, height, mobile = width < 1024, scenario, state: initial = {}, wait = 'main#content h1' } = {}) {
  const context = await browser.newContext({ viewport: { width, height: height ?? (mobile ? 844 : 800) }, isMobile: mobile, hasTouch: mobile, deviceScaleFactor: 1, colorScheme: theme })
  await context.addCookies([{ name: 'vite-ui-theme', value: theme, url: ORIGIN }])
  await context.addInitScript((language) => {
    localStorage.setItem('i18nextLng', language)
    localStorage.removeItem('status')
  }, lang)
  const state = { status: {}, tested: {}, writes: [], failedReads: 0, ...initial }
  await context.route('**/*', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    if (url.origin !== ORIGIN && url.protocol !== 'data:') { blocked++; return route.abort('blockedbyclient') }
    const p = url.pathname.replace(/\/$/, '')
    const api = p.startsWith('/api/') || p.startsWith('/pg/')
    if (api) {
      state.lastBody = request.postData()
      const answer = upstreamAnswer(p, url, request.method(), state)
      if (answer) return route.fulfill(answer)
    }
    const rewrite = scenario && scenarios[scenario]
    if (api && rewrite) {
      const response = await route.fetch()
      const json = await response.json()
      rewrite(p, json, url)
      return route.fulfill({ response, json })
    }
    return route.continue()
  })
  const page = await context.newPage()
  const errors = []
  page.on('pageerror', (error) => errors.push(String(error)))
  await page.goto(`${ORIGIN}${route}`)
  if (wait) await page.waitForSelector(wait)
  await page.waitForTimeout(1500)
  return { context, page, errors, state }
}

const layout = (page, scope) => page.evaluate((scope) => {
  const rect = (el) => { if (!el) return null; const r = el.getBoundingClientRect(); return { top: Math.round(r.top), bottom: Math.round(r.bottom), left: Math.round(r.left), right: Math.round(r.right), width: Math.round(r.width), height: Math.round(r.height) } }
  const main = document.querySelector('main#content')
  const visible = (el) => { const r = el.getBoundingClientRect(); return r.width > 1 && r.height > 1 && getComputedStyle(el).visibility !== 'hidden' }
  const clipped = [...(main?.querySelectorAll('h1, h2, h3, dt, dd, p, button, a, [role="tab"]') ?? [])]
    .filter((el) => visible(el) && getComputedStyle(el).overflow !== 'visible' && getComputedStyle(el).textOverflow !== 'ellipsis' && (el.scrollWidth > el.clientWidth + 1))
    .map((el) => el.textContent.trim().slice(0, 40))
  // Interactive targets; a switch counts with its enlarged hit area.
  const targets = [...((scope ? document.querySelector(scope) : main)?.querySelectorAll('button, a[href], [role="tab"]') ?? [])].filter(visible).map((el) => {
    // A card's name link is stretched over the whole card.
    const r = (el.matches('[data-connection-link]') && el.closest('article')) ? el.closest('article').getBoundingClientRect() : el.getBoundingClientRect()
    const after = getComputedStyle(el, '::after')
    const extendX = after.content !== 'none' && after.position === 'absolute' ? -parseFloat(after.left || '0') - parseFloat(after.right || '0') : 0
    const extendY = after.content !== 'none' && after.position === 'absolute' ? -parseFloat(after.top || '0') - parseFloat(after.bottom || '0') : 0
    return { name: (el.getAttribute('aria-label') || el.textContent).trim().slice(0, 30), height: Math.round(r.height + Math.max(0, extendY)), width: Math.round(r.width + Math.max(0, extendX)) }
  })
  return {
    overflow: document.documentElement.scrollWidth - innerWidth,
    main: rect(main),
    h1: rect(main?.querySelector('h1')),
    clipped,
    smallTargets: targets.filter((t) => t.height < 44 || t.width < 44),
    coarse: matchMedia('(pointer: coarse)').matches,
  }
}, scope)

async function shot(page, file, fullPage = false) {
  await page.screenshot({ path: path.join(outDir, file), fullPage })
  shots.push(file)
}

async function basicChecks(name, page, errors, { width, scope }) {
  const g = await layout(page, scope)
  check(`${name}: no horizontal overflow`, g.overflow <= 0, g.overflow)
  check(`${name}: no page errors`, errors.length === 0, errors)
  check(`${name}: heading inside the viewport`, g.h1 && g.h1.left >= 0 && g.h1.right <= width, g.h1)
  check(`${name}: no text cut off without an ellipsis`, g.clipped.length === 0, g.clipped)
  if (width < 1024) {
    check(`${name}: touch targets >= 44px${scope ? ` in ${scope}` : ''}`, g.coarse && g.smallTargets.length === 0, g.smallTargets)
    const last = await page.evaluate(async () => {
      const scroller = document.querySelector('.pc-route-content')
      scroller?.scrollTo({ top: scroller.scrollHeight })
      window.scrollTo(0, document.documentElement.scrollHeight)
      await new Promise((resolve) => setTimeout(resolve, 200))
      const content = [...document.querySelectorAll('main#content *')].filter((el) => el.children.length === 0 && el.getBoundingClientRect().height > 0)
      const bottom = Math.max(...content.map((el) => el.getBoundingClientRect().bottom))
      const nav = document.querySelector('.pc-mobile-nav')?.getBoundingClientRect().top ?? innerHeight
      const result = { bottom: Math.round(bottom), nav: Math.round(nav) }
      scroller?.scrollTo({ top: 0 })
      window.scrollTo(0, 0)
      await new Promise((resolve) => setTimeout(resolve, 100))
      return result
    })
    check(`${name}: last content clears the bottom navigation`, last.bottom <= last.nav + 1, last)
  }
  return g
}

const CONN = '/connections/11'
const pages = [
  ['upstream', '/connections'],
  ['connection', CONN],
]

if (groups.has('pages')) {
  for (const lang of ['zh', 'en']) for (const theme of ['light', 'dark']) for (const width of [1280, 390]) for (const [kind, route] of pages) {
    const { context, page, errors, state } = await open(route, { lang, theme, width })
    const name = `${kind}-${width}-${theme}-${lang}`
    await basicChecks(name, page, errors, { width })
    check(`${name}: opening the page sends no test and no write`, state.writes.length === 0, state.writes)
    if (kind === 'upstream') {
      const columns = await page.locator('[data-testid="connection-grid"]').evaluate((el) => getComputedStyle(el).gridTemplateColumns.split(' ').length)
      check(`${name}: grid columns`, columns === (width >= 1280 ? 3 : 1), columns)
      // Every drawn ring writes its number.
      const rings = await page.locator('[data-testid="connection-grid"] svg[role="img"]').evaluateAll((els) => els.map((el) => el.querySelector('text')?.textContent ?? ''))
      check(`${name}: rings carry their number`, rings.length === 2 && rings.every((text) => /^\d+%$/.test(text)), rings)
    }
    await shot(page, `${name}.png`, kind === 'upstream' && width === 1280)
    await context.close()
  }
}

if (groups.has('widths')) {
  for (const lang of ['zh', 'en']) for (const width of [320, 768, 1024]) for (const [kind, route] of pages) {
    const { context, page, errors } = await open(route, { lang, width })
    const name = `${kind}-${width}-light-${lang}`
    await basicChecks(name, page, errors, { width })
    if (kind === 'upstream' && width === 1024) {
      const columns = await page.locator('[data-testid="connection-grid"]').evaluate((el) => getComputedStyle(el).gridTemplateColumns.split(' ').length)
      check(`${name}: two columns between 1024 and 1279`, columns === 2, columns)
    }
    await shot(page, `${name}.png`)
    await context.close()
  }
}

if (groups.has('tabs')) {
  for (const lang of ['zh', 'en']) for (const width of [1280, 390]) for (const tab of ['routing', 'requests', 'history']) {
    const { context, page, errors } = await open(`${CONN}?tab=${tab}`, { width, lang })
    const name = `connection-${tab}-${width}-light-${lang}`
    check(`${name}: tab from the URL`, (await page.locator('[role="tab"][aria-selected="true"]').count()) === 1, null)
    await basicChecks(name, page, errors, { width })
    await page.getByRole('tablist').first().evaluate((el) => el.scrollIntoView({ block: 'start' }))
    await page.waitForTimeout(200)
    await shot(page, `${name}.png`)
    await context.close()
  }
  for (const [name, route] of [['accounts-1280-light-zh', '/upstream-accounts'], ['accounts-390-light-zh', '/upstream-accounts'], ['accounts-1280-light-en', '/upstream-accounts'], ['models-1280-light-zh', '/models/metadata'], ['models-390-light-zh', '/models/metadata'], ['models-1280-light-en', '/models/metadata']]) {
    const width = name.includes('390') ? 390 : 1280
    const { context, page, errors } = await open(route, { width, lang: name.endsWith('en') ? 'en' : 'zh' })
    await basicChecks(name, page, errors, { width })
    await shot(page, `${name}.png`)
    await context.close()
  }
}

if (groups.has('states')) {
  const cases = [
    ['empty-upstream-1280-light-zh', '/connections', { state: { empty: true } }],
    ['empty-upstream-390-light-en', '/connections', { state: { empty: true }, width: 390, lang: 'en' }],
    ['error-upstream-1280-light-zh', '/connections', { state: { listError: true } }],
    ['error-connection-1280-light-zh', CONN, { state: { detailError: true } }],
    ['windows-error-connection-1280-light-zh', CONN, { state: { windowsError: true } }],
    ['denied-upstream-1280-light-zh', '/connections', { state: { denied: true } }],
    ['user-upstream-1280-light-zh', '/connections', { scenario: 'user', wait: null }],
    ['lite-upstream-1280-light-zh', '/connections', { scenario: 'lite' }],
    ['lite-models-1280-light-zh', '/models/deployments', { scenario: 'lite' }],
    ['dark-connection-history-1280-dark-en', `${CONN}?tab=history`, { theme: 'dark', lang: 'en' }],
    ['connection-subscription-d-1280-light-zh', '/connections/14', {}],
    ['connection-local-390-light-zh', '/connections/13', { width: 390 }],
  ]
  for (const [name, route, options] of cases) {
    const { context, page, errors } = await open(route, options)
    const text = await page.locator('body').innerText()
    if (options.scenario !== 'user') await basicChecks(name, page, errors, { width: options.width ?? 1280 })
    if (name.startsWith('empty-upstream')) check(`${name}: empty grid says so`, /还没有连接|No connections yet/.test(text), text.slice(0, 300))
    if (name.startsWith('error-upstream')) check(`${name}: read error is not empty`, text.includes('无法加载连接') && !text.includes('还没有连接'), text.slice(0, 300))
    if (name.startsWith('error-connection')) check(`${name}: unavailable connection with retry`, text.includes('重试'), text.slice(0, 300))
    if (name.startsWith('denied')) check(`${name}: unverified access shows no connections`, text.includes('无法确认连接访问权限') && !text.includes('示例订阅账号'), text.slice(0, 300))
    if (name.startsWith('user-upstream')) {
      await page.waitForTimeout(800)
      check(`${name}: an ordinary user cannot open the connections page`, new URL(page.url()).pathname !== '/connections' && !text.includes('示例订阅账号'), { url: page.url(), text: text.slice(0, 200) })
      await page.goto(`${ORIGIN}/dashboard/overview`)
      await page.waitForSelector('[data-myapi-header]')
      const nav = await page.locator('[data-myapi-header]').innerText()
      check(`${name}: an ordinary user has no upstream entry`, !nav.includes('上游'), nav)
    }
    if (name === 'lite-models-1280-light-zh') check(`${name}: Lite has no deployments section`, new URL(page.url()).pathname === '/models/metadata' && (await page.getByRole('tab', { name: '部署' }).count()) === 0, { url: page.url() })
    await shot(page, `${name}.png`, name.includes('1280'))
    await context.close()
  }
}

if (groups.has('ops')) {
  {
    const { context, page, errors, state } = await open('/connections?filter=示例&page=1')
    const card = page.locator('[data-connection-card="14"]')
    await card.waitFor()
    // Hover and keyboard focus on a card.
    await card.hover()
    await page.waitForTimeout(200)
    await shot(page, 'ops-card-hover-1280-light-zh.png')
    await card.locator('[data-connection-link]').focus()
    await page.waitForTimeout(200)
    const focusOutline = await card.evaluate((el) => getComputedStyle(el).outlineStyle)
    check('ops: a focused card shows an outline', focusOutline !== 'none', focusOutline)
    await shot(page, 'ops-card-focus-1280-light-zh.png')
    // The switch and Test act without opening the page.
    await card.getByRole('switch').click()
    await page.waitForTimeout(800)
    check('ops: the switch turns the connection off and stays on the grid', new URL(page.url()).pathname === '/connections' && state.writes.some((w) => w.status === 14 && w.value === 2), { url: page.url(), writes: state.writes })
    await card.getByRole('switch').click()
    await page.waitForTimeout(800)
    check('ops: the switch turns it on again', state.writes.some((w) => w.status === 14 && w.value === 1), state.writes)
    await card.getByRole('button', { name: '测试' }).click()
    await card.getByText('测试成功').waitFor()
    check('ops: Test runs on the card and shows the result there', new URL(page.url()).pathname === '/connections' && state.writes.some((w) => w.test === 14), { url: page.url(), writes: state.writes })
    await shot(page, 'ops-card-tested-1280-light-zh.png')
    // A click on the card body opens the page; the breadcrumb returns with
    // filters and focus on the card.
    // A point on the windows line, away from the name.
    await card.click({ position: { x: 60, y: 90 } })
    await page.waitForURL(/\/connections\/14/)
    check('ops: a click on the card body opens the connection page', true, page.url())
    await shot(page, 'ops-connection-from-card-1280-light-zh.png')
    await page.getByRole('navigation', { name: '当前页面' }).getByRole('button', { name: '连接', exact: true }).click()
    await page.waitForURL((url) => url.pathname === '/connections')
    await page.waitForTimeout(800)
    const back = new URL(page.url())
    const focused = await page.evaluate(() => document.activeElement?.getAttribute('data-connection-link'))
    check('ops: breadcrumb Back keeps the filters and focuses the card', back.searchParams.get('filter') === '示例' && focused === '14', { url: page.url(), focused })
    // Enter on a focused card opens it as well.
    await page.keyboard.press('Enter')
    await page.waitForURL(/\/connections\/14/)
    check('ops: Enter on the focused card opens it', true, page.url())
    check('ops: grid without errors', errors.length === 0, errors)
    await context.close()
  }
  {
    // Earlier inspector links and the table view.
    const { context, page, errors } = await open('/connections?connection=11&tab=routing&filter=示例&page=1', { wait: '[data-testid="connection-page"]' })
    const url = new URL(page.url())
    check('ops: ?connection=11&tab=routing opens the connection page on routing', url.pathname === CONN && url.searchParams.get('tab') === 'routing' && !url.searchParams.has('connection'), page.url())
    await page.goto(`${ORIGIN}/channels`)
    await page.waitForSelector('[data-connection-link="12"]')
    await page.waitForTimeout(800)
    await shot(page, 'ops-table-view-cards-1280-light-zh.png')
    // The table view's own table (the second view-mode button).
    await page.locator('div[role="group"] > button[aria-pressed]').nth(1).click()
    await page.locator('tr [data-connection-link="12"]').waitFor()
    await shot(page, 'ops-table-view-1280-light-zh.png')
    await page.locator('tr').filter({ has: page.locator('[data-connection-link="12"]') }).locator('td').nth(3).click()
    await page.waitForURL(/\/connections\/12/)
    check('ops: a table-view row opens the connection page', true, page.url())
    await page.goBack()
    await page.waitForURL((u) => u.pathname === '/channels')
    await page.waitForTimeout(1200)
    check('ops: Back to the table view focuses the row name', (await page.evaluate(() => document.activeElement?.getAttribute('data-connection-link'))) === '12', page.url())
    await shot(page, 'ops-table-back-1280-light-zh.png')
    check('ops: links without errors', errors.length === 0, errors)
    await context.close()
  }
  {
    // Connection page: Test, the switch, a request and Back; tabs in the URL.
    const { context, page, errors, state } = await open(`${CONN}?tab=requests`)
    await page.locator('[data-request-row="2"]').click()
    await page.waitForURL(/\/usage-logs\/request\/SYNTHETIC-UP-2/)
    await page.goBack()
    await page.waitForURL((u) => u.pathname === CONN)
    await page.waitForTimeout(1200)
    check('ops: Back from a request focuses its row', (await page.evaluate(() => document.activeElement?.getAttribute('data-request-row'))) === '2', page.url())
    const actions = page.locator('main#content .myapi-page-actions')
    await actions.getByRole('button', { name: '测试' }).click()
    await page.getByText('测试成功').waitFor()
    await shot(page, 'ops-connection-tested-1280-light-zh.png')
    await actions.getByRole('switch').click()
    await page.waitForTimeout(800)
    check('ops: the page switch turns the connection off', state.writes.some((w) => w.status === 11 && w.value === 2), state.writes)
    check('ops: connection page without errors', errors.length === 0, errors)
    await context.close()
  }
  {
    // Phone: top-bar Back returns to the grid with focus.
    const { context, page, errors } = await open('/connections', { width: 390 })
    await page.locator('[data-connection-link="12"]').click()
    await page.waitForURL(/\/connections\/12/)
    await page.waitForTimeout(800)
    const actions = await page.locator('main#content .myapi-page-actions').boundingBox()
    const nav = await page.locator('.pc-mobile-nav').boundingBox()
    check('ops: phone actions sit above the bottom navigation', actions && nav && actions.y + actions.height < nav.y, { actions, nav })
    await shot(page, 'ops-phone-connection-390-light-zh.png')
    await page.locator('.pc-header-mobile').getByRole('button', { name: '返回' }).click()
    await page.waitForURL((u) => u.pathname === '/connections')
    await page.waitForTimeout(1000)
    check('ops: phone Back returns to the grid with focus on the card', (await page.evaluate(() => document.activeElement?.getAttribute('data-connection-link'))) === '12', page.url())
    check('ops: phone without errors', errors.length === 0, errors)
    await context.close()
  }
  {
    // Add connection opens the existing editor; the preview refuses writes.
    const { context, page, errors } = await open('/connections')
    await page.locator('main#content .myapi-page-actions').getByRole('button', { name: '添加连接' }).click()
    await page.getByRole('dialog').waitFor()
    await page.waitForTimeout(800)
    check('ops: Add connection opens the editor with Save and test', (await page.getByRole('dialog').getByRole('button', { name: '保存并测试' }).count()) === 1, null)
    await shot(page, 'ops-add-connection-1280-light-zh.png')
    check('ops: editor without errors', errors.length === 0, errors)
    await context.close()
  }
}

if (groups.has('overview')) {
  for (const lang of ['zh', 'en']) {
    const { context, page, errors, state } = await open('/dashboard/overview', { lang, wait: '[data-testid="overview"]' })
    await page.waitForTimeout(800)
    const attention = page.getByRole('region', { name: lang === 'zh' ? '需要处理' : 'Needs attention' })
    const link = attention.getByRole('link').filter({ hasText: lang === 'zh' ? /失败/ : /failed/ }).first()
    const href = await link.getAttribute('href')
    check(`overview-${lang}: the failure link asks for result=failed`, href?.includes('result=failed'), href)
    check(`overview-${lang}: failures counted by result=failed`, (await link.innerText()).includes('1'), await link.innerText())
    await shot(page, `overview-failed-link-1280-light-${lang}.png`)
    const recent = page.getByRole('region', { name: lang === 'zh' ? '最近请求' : 'Recent requests' })
    await recent.getByRole('button', { name: lang === 'zh' ? '失败' : 'Failed', exact: true }).click()
    await page.waitForTimeout(800)
    check(`overview-${lang}: the Failed filter reads result=failed`, state.failedReads >= 2, state.failedReads)
    await recent.scrollIntoViewIfNeeded()
    await shot(page, `overview-failed-filter-1280-light-${lang}.png`)
    await link.click()
    await page.waitForURL(/usage-logs\/common/)
    await page.waitForTimeout(1200)
    check(`overview-${lang}: call records show the failure filter`, (await page.getByTestId('failed-requests-filter').count()) === 1, page.url())
    await shot(page, `logs-failed-only-1280-light-${lang}.png`)
    check(`overview-${lang}: no errors`, errors.length === 0, errors)
    await context.close()
  }
}

if (groups.has('pagination')) {
  for (const width of [320, 390]) {
    for (const [name, route] of [['keys', '/keys?pageSize=1'], ['logs', '/usage-logs/common?pageSize=1'], ['models', '/models/metadata?pageSize=1']]) {
      const { context, page, errors } = await open(route, { width })
      const g = await page.evaluate(() => {
        const footer = document.querySelector('.myapi-page-footer')
        const buttons = [...(footer?.querySelectorAll('button') ?? [])].filter((b) => b.getBoundingClientRect().width > 0)
        return {
          buttons: buttons.map((b) => ({ text: (b.textContent || '').trim(), width: Math.round(b.getBoundingClientRect().width), height: Math.round(b.getBoundingClientRect().height), right: Math.round(b.getBoundingClientRect().right), left: Math.round(b.getBoundingClientRect().left) })),
          viewport: innerWidth,
          overflow: document.documentElement.scrollWidth - innerWidth,
        }
      })
      check(`pagination-${name}-${width}: page buttons >= 44px`, g.buttons.length > 0 && g.buttons.every((b) => b.width >= 44 && b.height >= 44), g.buttons)
      check(`pagination-${name}-${width}: inside the viewport`, g.overflow <= 0 && g.buttons.every((b) => b.left >= 0 && b.right <= g.viewport), g)
      check(`pagination-${name}-${width}: no errors`, errors.length === 0, errors)
      await page.locator('.myapi-page-footer').scrollIntoViewIfNeeded()
      await shot(page, `pagination-${name}-${width}-light-zh.png`)
      await context.close()
    }
  }
  const { context, page } = await open('/keys?pageSize=1', { width: 1280 })
  const desk = await page.evaluate(() => [...document.querySelectorAll('.myapi-page-footer button:not([role="combobox"])')].map((b) => Math.round(b.getBoundingClientRect().height)))
  check('pagination-desktop: fine pointer keeps compact 32px buttons', desk.length > 0 && desk.every((h) => h <= 36), desk)
  await context.close()
}

if (groups.has('profile')) {
  const cases = [
    ['profile-subscription-1280-light-zh', {}],
    ['profile-subscription-390-light-zh', { width: 390 }],
    ['profile-subscription-1280-light-en', { lang: 'en' }],
    ['profile-no-subscription-1280-light-zh', { state: { noSubscription: true } }],
    ['profile-lite-subscription-1280-light-zh', { scenario: 'lite' }],
  ]
  for (const [name, options] of cases) {
    const { context, page, errors } = await open('/profile', options)
    await page.waitForTimeout(800)
    const card = page.getByTestId('my-subscription-card')
    const count = await card.count()
    if (name.includes('no-subscription')) check(`${name}: no card without subscriptions`, count === 0, count)
    else {
      check(`${name}: read-only card shown`, count === 1, count)
      const buttons = await card.getByRole('button').count()
      const links = await card.getByRole('link').allInnerTexts()
      check(`${name}: no purchase, renewal or preference control`, buttons === 0 && links.every((text) => /钱包|wallet/i.test(text)), { buttons, links })
      if (name.includes('lite')) check(`${name}: Lite has no wallet link`, links.length === 0, links)
      await card.scrollIntoViewIfNeeded()
    }
    // Elsewhere on the profile page is outside this step (see README).
    await basicChecks(name, page, errors, { width: options.width ?? 1280, scope: count ? '[data-testid="my-subscription-card"]' : undefined })
    await shot(page, `${name}.png`)
    await context.close()
  }
}

await browser.close()
const failed = checks.filter((c) => !c.pass)
writeFileSync(path.join(outDir, 'browser-checks.json'), JSON.stringify({ blocked, shots, total: checks.length, failed: failed.length, checks }, null, 2))
console.log(JSON.stringify({ blocked, shots: shots.length, checks: checks.length, failed: failed.length }))
process.exit(failed.length ? 1 : 0)
