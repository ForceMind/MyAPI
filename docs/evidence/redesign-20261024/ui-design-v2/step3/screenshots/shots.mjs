// Step 3 (Key list, Key page, request page) screenshots and browser checks
// against the synthetic preview (tools/ui/page-layout-preview.mjs on
// 127.0.0.1:4184). Every other origin is aborted. Scenario data and the two
// writes exercised here (pause/resume, copy) are answered in the browser
// only; the preview itself stays read-only and nothing is written anywhere.
// Usage: node shots.mjs <outDir> [group...]  groups: pages widths tabs states ops
import { mkdirSync, writeFileSync } from 'node:fs'
import path from 'node:path'

const { chromium } = await import(process.env.MYAPI_PLAYWRIGHT_MODULE)
const ORIGIN = 'http://127.0.0.1:4184'
const outDir = process.argv[2]
const groups = new Set(process.argv.slice(3).length ? process.argv.slice(3) : ['pages', 'widths', 'tabs', 'states', 'ops'])
mkdirSync(outDir, { recursive: true })

const LITE_CAPS = Object.fromEntries(['payments', 'commercial_subscriptions', 'redemption_codes', 'checkin', 'affiliate', 'public_pricing', 'rankings', 'perf_metrics', 'external_oauth', 'midjourney', 'model_deployments', 'site_content', 'user_registration'].map((k) => [k, false]))
const KEY = '/keys/21'
const REQUEST = '/keys/21/requests/SYNTHETIC-CONTEXT-ERROR'
const browser = await chromium.launch()
const checks = []
const shots = []
let blocked = 0
const check = (name, pass, detail) => {
  checks.push({ name, pass: Boolean(pass), detail })
  if (!pass) console.log('FAIL', name, JSON.stringify(detail))
}

// Response rewrites per scenario: (pathname, json, url) => json | undefined.
const scenarios = {
  lite: (p, json) => {
    if (p === '/api/status' && json.data) Object.assign(json.data, { product_edition: 'lite', capabilities: LITE_CAPS })
  },
  user: (p, json) => {
    if ((p === '/api/user/self' || p === '/api/user/auth/refresh') && json.data) {
      const user = json.data.user ?? json.data
      if (user && typeof user === 'object' && 'role' in user) { user.role = 1; user.permissions = { sidebar_settings: true, admin_permissions: {} } }
    }
  },
  noKeys: (p, json) => {
    if (['/api/token', '/api/token/search'].includes(p) && json.data) { json.data.items = []; json.data.total = 0 }
  },
  noRequests: (p, json, url) => {
    if (['/api/log', '/api/log/self'].includes(p) && url.searchParams.has('token_id') && json.data) { json.data.items = []; json.data.total = 0 }
  },
  captureOff: (p, json) => {
    if (p === '/api/full-content-logs' && json.data) json.data.enabled = false
  },
}
// Answers that replace the preview's (failures and the two writes).
const overrides = {
  // A refused read (a 5xx would take the whole console to its error page).
  keyError: (p) => (p === '/api/token/21' ? { status: 200, json: { success: false, message: 'Synthetic failure' } } : undefined),
  lookupError: (p, url) => (['/api/log', '/api/log/self'].includes(p) && url.searchParams.has('request_id') ? { status: 503, json: { success: false, message: 'Synthetic failure' } } : undefined),
}

async function open(route, { lang = 'zh', theme = 'light', width = 1280, height, mobile = width < 1024, scenario, override, writes, wait = 'main#content h1' } = {}) {
  const context = await browser.newContext({ viewport: { width, height: height ?? (mobile ? 844 : 800) }, isMobile: mobile, hasTouch: mobile, deviceScaleFactor: 1, colorScheme: theme })
  await context.addCookies([{ name: 'vite-ui-theme', value: theme, url: ORIGIN }])
  await context.addInitScript((language) => {
    localStorage.setItem('i18nextLng', language)
    localStorage.removeItem('status')
  }, lang)
  const state = { keyStatus: 1, writes: [] }
  await context.route('**/*', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    if (url.origin !== ORIGIN && url.protocol !== 'data:') { blocked++; return route.abort('blockedbyclient') }
    const p = url.pathname.replace(/\/$/, '')
    const api = p.startsWith('/api/') || p.startsWith('/pg/')
    if (api && writes) {
      // Pause/resume and copy, answered in the browser only.
      if (request.method() === 'PUT' && p === '/api/token' && url.searchParams.get('status_only') === 'true') {
        const body = request.postDataJSON()
        state.writes.push(body)
        state.keyStatus = body.status
        return route.fulfill({ json: { success: true } })
      }
      if (request.method() === 'POST' && p === '/api/token/21/key') {
        state.writes.push({ copy: 21 })
        return route.fulfill({ json: { success: true, data: { key: 'synthetic-copied-secret' } } })
      }
    }
    const replaced = api && override && overrides[override](p, url)
    if (replaced) return route.fulfill(replaced)
    const rewrite = scenario && scenarios[scenario]
    if (api && (rewrite || (writes && p === '/api/token/21'))) {
      const response = await route.fetch()
      const json = await response.json()
      rewrite?.(p, json, url)
      if (writes && p === '/api/token/21' && json.data) json.data.status = state.keyStatus
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

const layout = (page) => page.evaluate(() => {
  const rect = (el) => { if (!el) return null; const r = el.getBoundingClientRect(); return { top: Math.round(r.top), bottom: Math.round(r.bottom), left: Math.round(r.left), right: Math.round(r.right), width: Math.round(r.width), height: Math.round(r.height) } }
  const main = document.querySelector('main#content')
  // Screen-reader-only text (1px boxes) is not visible text.
  const visible = (el) => { const r = el.getBoundingClientRect(); return r.width > 1 && r.height > 1 && getComputedStyle(el).visibility !== 'hidden' }
  // Text cut off without an ellipsis: a leaf element whose text overflows it.
  const clipped = [...(main?.querySelectorAll('h1, h2, dt, dd, p, button, a, [role="tab"]') ?? [])]
    .filter((el) => visible(el) && getComputedStyle(el).overflow !== 'visible' && getComputedStyle(el).textOverflow !== 'ellipsis' && (el.scrollWidth > el.clientWidth + 1))
    .map((el) => el.textContent.trim().slice(0, 40))
  const targets = [...(main?.querySelectorAll('button, a[href], [role="tab"]') ?? [])].filter(visible).map((el) => ({ name: (el.getAttribute('aria-label') || el.textContent).trim().slice(0, 30), height: Math.round(el.getBoundingClientRect().height), width: Math.round(el.getBoundingClientRect().width) }))
  return {
    overflow: document.documentElement.scrollWidth - innerWidth,
    main: rect(main),
    h1: rect(main?.querySelector('h1')),
    clipped,
    smallTargets: targets.filter((t) => t.height < 44 || t.width < 44),
    bottomNav: rect(document.querySelector('.pc-mobile-nav')),
    coarse: matchMedia('(pointer: coarse)').matches,
  }
})

async function shot(page, file, fullPage = false) {
  await page.screenshot({ path: path.join(outDir, file), fullPage })
  shots.push(file)
}

async function basicChecks(name, page, errors, { width }) {
  const g = await layout(page)
  check(`${name}: no horizontal overflow`, g.overflow <= 0, g.overflow)
  check(`${name}: no page errors`, errors.length === 0, errors)
  check(`${name}: heading inside the viewport`, g.h1 && g.h1.left >= 0 && g.h1.right <= width, g.h1)
  check(`${name}: no text cut off without an ellipsis`, g.clipped.length === 0, g.clipped)
  if (width < 1024) {
    check(`${name}: touch targets >= 44px`, g.coarse && g.smallTargets.length === 0, g.smallTargets)
    // Scrolled to the end, the last content stays above the bottom navigation.
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

const pages = [
  ['keys', '/keys?filter=Synthetic'],
  ['key', KEY],
  ['request', REQUEST],
]

if (groups.has('pages')) {
  for (const lang of ['zh', 'en']) for (const theme of ['light', 'dark']) for (const width of [1280, 390]) for (const [kind, route] of pages) {
    const { context, page, errors } = await open(route, { lang, theme, width })
    const name = `${kind}-${width}-${theme}-${lang}`
    await basicChecks(name, page, errors, { width })
    await shot(page, `${name}.png`)
    await context.close()
  }
}

if (groups.has('widths')) {
  for (const lang of ['zh', 'en']) for (const width of [320, 768, 1024]) for (const [kind, route] of pages) {
    if (kind === 'keys' && lang === 'en') continue
    const { context, page, errors } = await open(route, { lang, width })
    const name = `${kind}-${width}-light-${lang}`
    await basicChecks(name, page, errors, { width })
    await shot(page, `${name}.png`)
    await context.close()
  }
}

if (groups.has('tabs')) {
  for (const width of [1280, 390]) {
    for (const [tab, label] of [['scope', '范围与分额'], ['delivery', '交付']]) {
      const { context, page, errors } = await open(`${KEY}?tab=${tab}`, { width })
      const name = `key-${tab}-${width}-light-zh`
      check(`${name}: tab selected from the URL`, (await page.getByRole('tab', { name: label, exact: true }).getAttribute('aria-selected')) === 'true', null)
      await basicChecks(name, page, errors, { width })
      // The tab's content starts below the figures; bring it into view.
      await page.getByRole('tablist').first().evaluate((el) => el.scrollIntoView({ block: 'start' }))
      await page.waitForTimeout(200)
      await shot(page, `${name}.png`)
      await context.close()
    }
    for (const tab of ['sse', 'raw']) {
      const { context, page, errors } = await open(`${REQUEST}?tab=${tab}`, { width })
      const name = `request-${tab}-${width}-light-zh`
      await basicChecks(name, page, errors, { width })
      if (width < 1024) await page.getByRole('tablist').last().evaluate((el) => el.scrollIntoView({ block: 'start' }))
      await shot(page, `${name}.png`)
      await context.close()
    }
  }
  // The full request page, diagnosis record included.
  const { context, page } = await open(REQUEST)
  await shot(page, 'request-full-1280-light-zh.png', true)
  await context.close()
}

if (groups.has('states')) {
  const cases = [
    ['lite-keys-1280-light-zh', '/keys', { scenario: 'lite' }],
    ['lite-key-1280-light-zh', KEY, { scenario: 'lite' }],
    ['lite-request-390-light-zh', REQUEST, { scenario: 'lite', width: 390 }],
    ['user-key-1280-light-zh', KEY, { scenario: 'user' }],
    ['user-request-1280-light-zh', REQUEST, { scenario: 'user' }],
    ['user-request-390-light-zh', REQUEST, { scenario: 'user', width: 390 }],
    ['empty-keys-1280-light-zh', '/keys', { scenario: 'noKeys', wait: 'main#content h1' }],
    ['empty-keys-390-light-en', '/keys', { scenario: 'noKeys', width: 390, lang: 'en' }],
    ['empty-key-requests-1280-light-zh', KEY, { scenario: 'noRequests' }],
    ['missing-request-1280-light-zh', '/usage-logs/request/SYNTHETIC-NO-SUCH-ID', {}],
    ['missing-request-390-light-en', '/usage-logs/request/SYNTHETIC-NO-SUCH-ID', { width: 390, lang: 'en' }],
    ['error-key-1280-light-zh', KEY, { override: 'keyError' }],
    ['error-request-1280-light-zh', '/usage-logs/request/SYNTHETIC-CONTEXT-ERROR', { override: 'lookupError' }],
    ['no-body-capture-on-1280-light-zh', '/keys/21/requests/SYNTHETIC-NOT-RECORDED', {}],
    ['no-body-capture-off-1280-light-en', '/keys/21/requests/SYNTHETIC-NOT-RECORDED', { scenario: 'captureOff', lang: 'en' }],
    ['rejected-request-1280-light-zh', '/keys/21/requests/SYNTHETIC-PREFLIGHT-REJECTED', {}],
  ]
  for (const [name, route, options] of cases) {
    const { context, page, errors } = await open(route, options)
    await basicChecks(name, page, errors, { width: options.width ?? 1280 })
    const text = await page.locator('main#content').innerText()
    if (options.scenario === 'user' && route === REQUEST) {
      check(`${name}: an ordinary user sees no routing details and no body`, text.includes('路由细节仅管理员可见') && text.includes('需要管理员权限'), text.slice(0, 400))
    }
    if (options.scenario === 'noKeys') check(`${name}: empty list says so`, /未找到 API Key|No API Keys/.test(text), text.slice(0, 200))
    if (options.scenario === 'noRequests') check(`${name}: empty requests say so`, text.includes('这个 Key 还没有请求'), text.slice(0, 400))
    if (name.startsWith('missing-request')) check(`${name}: not found is not success`, /没有调用记录使用这个 Request ID|No call record has this Request ID/.test(text), text.slice(0, 300))
    if (name.startsWith('error-key')) check(`${name}: unavailable Key with retry`, text.includes('重试'), text.slice(0, 300))
    if (name.startsWith('error-request')) check(`${name}: lookup failure is an error with retry`, text.includes('重试') && !text.includes('没有调用记录使用这个 Request ID'), text.slice(0, 300))
    if (name.startsWith('no-body-capture-on')) check(`${name}: body capture status explained`, text.includes('正文采集已开启'), text.slice(-600))
    if (name.startsWith('no-body-capture-off')) check(`${name}: capture off explained`, text.includes('Body capture is off'), text.slice(-600))
    await shot(page, `${name}.png`, !name.includes('390'))
    await context.close()
  }
}

if (groups.has('ops')) {
  // List -> Key page -> request page and back, with focus and list state.
  {
    const { context, page, errors } = await open('/keys?filter=Synthetic&pageSize=1&page=2', { writes: true })
    const row = page.locator('[data-inspect-id="22"]').first()
    await row.waitFor()
    await row.click()
    await page.waitForURL(/\/keys\/22/)
    check('ops: a Key row opens its page', new URL(page.url()).pathname === '/keys/22', page.url())
    await page.getByRole('navigation', { name: '当前页面' }).getByRole('button', { name: 'API Key', exact: true }).click()
    await page.waitForURL((url) => url.pathname === '/keys')
    await page.waitForTimeout(800)
    const back = new URL(page.url())
    const focused = await page.evaluate(() => document.activeElement?.getAttribute('data-inspect-id'))
    check('ops: breadcrumb Back keeps the filter and page and focuses the row', back.searchParams.get('filter') === 'Synthetic' && back.searchParams.get('page') === '2' && focused === '22', { url: page.url(), focused })
    await shot(page, 'ops-back-to-list-1280-light-zh.png')
    // A click elsewhere on a row opens the Key as well.
    await page.locator('tr').filter({ has: page.locator('[data-inspect-id="22"]') }).locator('td[data-column-id="status"]').click()
    await page.waitForURL(/\/keys\/22/)
    check('ops: a click on the row (not its controls) opens the Key', true, page.url())
    // Request page and back to the Key with focus on the request row.
    await page.goto(`${ORIGIN}${KEY}`)
    await page.locator('[data-request-row="99"]').click()
    await page.waitForURL(/requests\/SYNTHETIC-COMPLETED-NO-USAGE/)
    await page.waitForTimeout(800)
    await shot(page, 'ops-request-from-key-1280-light-zh.png')
    await page.goBack()
    await page.waitForURL((url) => url.pathname === KEY)
    await page.waitForTimeout(1000)
    check('ops: browser Back returns to the Key with focus on the request', (await page.evaluate(() => document.activeElement?.getAttribute('data-request-row'))) === '99', page.url())
    // Pause and resume, then copy, answered in the browser.
    const actions = page.locator('main#content .myapi-page-actions')
    await actions.getByRole('button', { name: '暂停 Key' }).click()
    await actions.getByRole('button', { name: '恢复 Key' }).waitFor()
    await shot(page, 'ops-paused-1280-light-zh.png')
    await actions.getByRole('button', { name: '恢复 Key' }).click()
    await actions.getByRole('button', { name: '暂停 Key' }).waitFor()
    await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: ORIGIN })
    await actions.getByRole('button', { name: '复制', exact: true }).click()
    await page.waitForTimeout(500)
    const copied = await page.evaluate(() => navigator.clipboard.readText())
    check('ops: pause, resume and copy act on this Key only after clicks', copied === 'sk-synthetic-copied-secret', { copied })
    check('ops: list and pages without errors', errors.length === 0, errors)
    await context.close()
  }
  // Earlier links land on the new pages.
  {
    const { context, page, errors } = await open('/keys?inspect=21&tab=access', { wait: '[data-testid="key-page"]' })
    check('ops: ?inspect=21&tab=access opens the Key page on scope', new URL(page.url()).pathname === KEY && new URL(page.url()).searchParams.get('tab') === 'scope', page.url())
    await page.goto(`${ORIGIN}/keys?inspect=21&request=SYNTHETIC-CONTEXT-ERROR&requestTab=sse`)
    await page.waitForSelector('[data-testid="request-page"]')
    check('ops: an earlier request link opens the request page on SSE', new URL(page.url()).pathname === REQUEST && new URL(page.url()).searchParams.get('tab') === 'sse', page.url())
    await page.goto(`${ORIGIN}/usage-logs/common?requestId=SYNTHETIC-PREFLIGHT-REJECTED&inspect=request&requestTab=body`)
    await page.waitForSelector('[data-testid="request-page"]')
    check('ops: an earlier call-record link opens the request page', new URL(page.url()).pathname === '/usage-logs/request/SYNTHETIC-PREFLIGHT-REJECTED', page.url())
    await shot(page, 'ops-legacy-request-1280-light-zh.png')
    // Command search: paste a Request ID and press Enter.
    await page.goto(`${ORIGIN}/keys`)
    await page.waitForSelector('main#content h1')
    await page.locator('[data-myapi-header]').getByRole('button', { name: '搜索' }).first().click()
    const input = page.getByRole('combobox')
    await input.fill('SYNTHETIC-COMPLETED-NO-USAGE')
    await page.keyboard.press('Enter')
    await page.waitForURL(/\/usage-logs\/request\/SYNTHETIC-COMPLETED-NO-USAGE/)
    await page.waitForSelector('[data-testid="request-result-grid"]')
    await shot(page, 'ops-command-search-1280-light-zh.png')
    check('ops: a pasted Request ID opens its page', true, page.url())
    // The overview time link opens the request page.
    await page.goto(`${ORIGIN}/dashboard/overview`)
    await page.waitForSelector('[data-testid="overview"]')
    await page.waitForTimeout(1200)
    const link = page.locator('[aria-labelledby="overview-recent-title"] tbody tr').first().getByRole('link')
    const href = await link.getAttribute('href')
    await link.click()
    await page.waitForURL(/\/usage-logs\/request\//)
    await page.waitForSelector('[data-testid="request-result-grid"]')
    check('ops: the overview time link opens the request page', href?.startsWith('/usage-logs/request/'), href)
    // The call records' Details opens the request page; Back returns.
    await page.goto(`${ORIGIN}/usage-logs/common?tokenId=21`)
    await page.getByTitle('点击查看详细信息').first().click()
    await page.waitForURL(/\/usage-logs\/request\//)
    await page.waitForSelector('[data-testid="request-result-grid"]')
    await page.getByRole('navigation', { name: '当前页面' }).getByRole('button').last().click()
    await page.waitForURL(/\/usage-logs\/common\?tokenId=21/)
    check('ops: call records Details opens the request page and Back returns', true, page.url())
    check('ops: links without errors', errors.length === 0, errors)
    await context.close()
  }
  // Phone: top bar Back, actions at the top, nothing under the bottom bar.
  {
    const { context, page, errors } = await open(KEY, { width: 390 })
    const actions = await page.locator('main#content .myapi-page-actions').boundingBox()
    const nav = await page.locator('.pc-mobile-nav').boundingBox()
    check('ops: phone actions sit above the bottom navigation', actions && nav && actions.y + actions.height < nav.y, { actions, nav })
    await shot(page, 'ops-phone-actions-390-light-zh.png')
    await page.locator('[data-request-row]').first().click()
    await page.waitForURL(/requests\//)
    await page.waitForTimeout(800)
    await page.locator('.pc-header-mobile').getByRole('button', { name: '返回' }).click()
    await page.waitForURL((url) => url.pathname === KEY)
    await page.locator('.pc-header-mobile').getByRole('button', { name: '返回' }).click()
    await page.waitForURL((url) => url.pathname === '/keys')
    check('ops: phone Back steps out one level at a time', true, page.url())
    const tabs = await open(REQUEST, { width: 320 })
    const strip = await tabs.page.getByRole('tablist').last().evaluate((el) => ({ overflowX: getComputedStyle(el).overflowX, scroll: el.scrollWidth, client: el.clientWidth }))
    check('ops: request tabs scroll sideways at 320px instead of wrapping', strip.overflowX === 'auto', strip)
    await tabs.context.close()
    check('ops: phone without errors', errors.length === 0, errors)
    await context.close()
  }
}

await browser.close()
const failed = checks.filter((c) => !c.pass)
writeFileSync(path.join(outDir, 'browser-checks.json'), JSON.stringify({ blocked, shots, total: checks.length, failed: failed.length, checks }, null, 2))
console.log(JSON.stringify({ blocked, shots: shots.length, checks: checks.length, failed: failed.length }))
process.exit(failed.length ? 1 : 0)
