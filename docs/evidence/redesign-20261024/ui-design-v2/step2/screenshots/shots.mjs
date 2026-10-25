// Step 2 flow board screenshots and browser checks against the synthetic
// preview (tools/ui/page-layout-preview.mjs on 127.0.0.1:4184). Every other
// origin is aborted. Scenario data is injected by rewriting preview responses
// in the browser only; nothing is written anywhere.
// Usage: node shots.mjs <outDir> [group...]  groups: pages widths states ops
import { mkdirSync, writeFileSync } from 'node:fs'
import path from 'node:path'

const { chromium } = await import(process.env.MYAPI_PLAYWRIGHT_MODULE)
const ORIGIN = 'http://127.0.0.1:4184'
const outDir = process.argv[2]
const groups = new Set(process.argv.slice(3).length ? process.argv.slice(3) : ['pages', 'widths', 'states', 'ops'])
mkdirSync(outDir, { recursive: true })

const LITE_CAPS = Object.fromEntries(['payments', 'commercial_subscriptions', 'redemption_codes', 'checkin', 'affiliate', 'public_pricing', 'rankings', 'perf_metrics', 'external_oauth', 'midjourney', 'model_deployments', 'site_content', 'user_registration'].map((k) => [k, false]))
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
  empty: (p, json) => {
    if (['/pg/keys', '/api/log', '/api/log/self', '/api/channel'].includes(p) && json.data) { json.data.items = []; json.data.total = 0 }
    if (p === '/api/channel/quota/changes' && json.data) json.data.items = []
  },
  many: (p, json, url) => {
    if (p === '/pg/keys' && json.data) {
      const base = json.data.items[0]
      json.data.items = Array.from({ length: 8 }, (_, i) => ({ ...base, id: 21 + i, name: i === 0 ? base.name : `Synthetic Key ${i + 1} · 合成 Key ${i + 1}`, status: i === 3 ? 2 : 1 }))
      json.data.total = 8
    }
    if (p === '/api/channel' && json.data && url.searchParams.get('status') !== 'enabled') {
      const base = json.data.items[0]
      json.data.items = Array.from({ length: 6 }, (_, i) => ({ ...base, id: 1 + i, name: i === 0 ? base.name : `Synthetic API connection ${i + 1}`, type: i === 0 ? 57 : 1, status: i === 4 ? 2 : 1, priority: 6 - i }))
      json.data.total = 9
    }
  },
}

async function open(route, { lang = 'zh', theme = 'light', width = 1280, height = 800, mobile = false, scenario, hold = false } = {}) {
  const context = await browser.newContext({ viewport: { width, height }, isMobile: mobile, hasTouch: mobile, deviceScaleFactor: 1, colorScheme: theme })
  await context.addCookies([{ name: 'vite-ui-theme', value: theme, url: ORIGIN }])
  await context.addInitScript((language) => {
    localStorage.setItem('i18nextLng', language)
    localStorage.removeItem('status')
  }, lang)
  await context.route('**/*', async (route) => {
    const url = new URL(route.request().url())
    if (url.origin !== ORIGIN && url.protocol !== 'data:') { blocked++; return route.abort('blockedbyclient') }
    // Loading state: the overview's own reads never answer.
    if (hold && ['/api/log', '/api/log/self', '/pg/keys', '/api/channel', '/api/channel/quota/changes'].includes(url.pathname)) return
    const rewrite = scenario && scenarios[scenario]
    if (rewrite && (url.pathname.startsWith('/api/') || url.pathname.startsWith('/pg/'))) {
      const response = await route.fetch()
      const json = await response.json()
      rewrite(url.pathname, json, url)
      return route.fulfill({ response, json })
    }
    return route.continue()
  })
  const page = await context.newPage()
  const errors = []
  page.on('pageerror', (error) => errors.push(String(error)))
  await page.goto(`${ORIGIN}${route}`)
  await page.waitForSelector('[data-testid="overview"]')
  await page.waitForTimeout(hold ? 800 : 1500)
  return { context, page, errors }
}

const layout = (page) => page.evaluate(() => {
  const rect = (el) => { if (!el) return null; const r = el.getBoundingClientRect(); return { top: Math.round(r.top), bottom: Math.round(r.bottom), left: Math.round(r.left), right: Math.round(r.right), width: Math.round(r.width), height: Math.round(r.height) } }
  const nodes = [...document.querySelectorAll('[data-flow-node]')]
  const clipped = nodes.filter((n) => [...n.querySelectorAll('span')].some((s) => s.scrollWidth > s.clientWidth + 1 && getComputedStyle(s).textOverflow !== 'ellipsis' && s.children.length === 0))
  return {
    overflow: document.documentElement.scrollWidth - innerWidth,
    board: rect(document.querySelector('[data-testid="flow-board"]')),
    core: rect(document.querySelector('[data-testid="flow-core"]')),
    nodes: nodes.length,
    minNodeHeight: Math.min(...nodes.map((n) => n.getBoundingClientRect().height)),
    clippedNodes: clipped.map((n) => n.dataset.flowNode),
    ringsWithoutText: [...document.querySelectorAll('[data-flow-node] svg[role="img"]')].filter((s) => !/\d+%/.test(s.textContent)).length,
    rings: document.querySelectorAll('[data-flow-node] svg[role="img"]').length,
    requests: rect(document.querySelector('[aria-labelledby="overview-recent-title"]')),
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
  check(`${name}: board inside viewport`, g.board && g.board.left >= 0 && g.board.right <= width, g.board)
  check(`${name}: no clipped node text`, g.clippedNodes.length === 0, g.clippedNodes)
  check(`${name}: every ring writes its value`, g.ringsWithoutText === 0, g)
  if (width < 1024) check(`${name}: node targets >= 44px`, g.nodes === 0 || g.minNodeHeight >= 44, g.minNodeHeight)
  return g
}

if (groups.has('pages')) {
  for (const lang of ['zh', 'en']) for (const theme of ['light', 'dark']) for (const width of [1280, 390]) {
    const mobile = width < 1024
    const { context, page, errors } = await open('/dashboard/overview', { lang, theme, width, height: mobile ? 844 : 800, mobile })
    const name = `overview-${width}-${theme}-${lang}`
    const g = await basicChecks(name, page, errors, { width })
    if (!mobile) check(`${name}: three columns side by side`, g.core && g.board && g.core.left > g.board.left + 200, g.core)
    await shot(page, `${name}.png`)
    await context.close()
  }
}

if (groups.has('widths')) {
  for (const lang of ['zh', 'en']) for (const width of [320, 768, 1024]) {
    const mobile = width < 1024
    const { context, page, errors } = await open('/dashboard/overview', { lang, width, height: mobile ? 800 : 768, mobile })
    const name = `overview-${width}-light-${lang}`
    await basicChecks(name, page, errors, { width })
    await shot(page, `${name}.png`)
    await context.close()
  }
}

if (groups.has('states')) {
  const cases = [
    ['lite-overview-1280-light-zh', { scenario: 'lite' }],
    ['lite-overview-1280-light-en', { scenario: 'lite', lang: 'en' }],
    ['lite-overview-390-light-zh', { scenario: 'lite', width: 390, mobile: true, height: 844 }],
    ['user-overview-1280-light-zh', { scenario: 'user' }],
    ['user-overview-390-light-zh', { scenario: 'user', width: 390, mobile: true, height: 844 }],
    ['empty-overview-1280-light-zh', { scenario: 'empty' }],
    ['empty-overview-1280-light-en', { scenario: 'empty', lang: 'en' }],
    ['folded-overview-1280-light-zh', { scenario: 'many', height: 1000 }],
    ['folded-overview-390-light-zh', { scenario: 'many', width: 390, mobile: true, height: 844 }],
    ['loading-overview-1280-light-zh', { hold: true }],
    ['selected-overview-1280-light-zh', { select: 'key-21' }],
    ['selected-overview-1280-dark-en', { select: 'connection-1', lang: 'en', theme: 'dark' }],
  ]
  for (const [name, options] of cases) {
    const { context, page, errors } = await open('/dashboard/overview', options)
    const g = await basicChecks(name, page, errors, { width: options.width ?? 1280 })
    if (options.scenario === 'user') {
      const headings = await page.locator('[data-testid="flow-board"] h3').allTextContents()
      check(`${name}: no upstream column for an ordinary user`, headings.length === 2 && !headings.some((text) => text.includes('上游')) && (await page.locator('[data-flow-node^="connection-"]').count()) === 0, headings)
    }
    if (options.scenario === 'many') {
      const keyNodes = await page.locator('[data-flow-node^="key-"]').count()
      const viewAll = await page.locator('[data-testid="flow-board"] a[href="/keys"]').count()
      const connectionAll = await page.locator('[data-testid="flow-board"] a[href="/connections"]').count()
      check(`${name}: eight Keys fold to six with view all`, keyNodes === 6 && viewAll === 1, { keyNodes, viewAll })
      check(`${name}: nine connections show six with view all`, connectionAll >= 1 && (await page.locator('[data-flow-node^="connection-"]').count()) === 6, { connectionAll })
    }
    if (options.hold) check(`${name}: columns show loading skeletons`, (await page.locator('[data-testid="flow-board"] [aria-busy="true"]').count()) >= 2, g)
    if (options.select) {
      await page.locator(`[data-flow-node="${options.select}"]`).click()
      await page.waitForTimeout(400)
      check(`${name}: chip shown`, await page.getByTestId('flow-filter-chip').isVisible(), null)
    }
    await shot(page, `${name}.png`, Boolean(options.height && options.height > 900))
    await context.close()
  }
}

if (groups.has('ops')) {
  // Click a Key node, read the URL and the filtered rows, clear with the chip.
  {
    const { context, page, errors } = await open('/dashboard/overview')
    const rows = () => page.locator('[aria-labelledby="overview-recent-title"] tbody tr').count()
    const before = await rows()
    await page.locator('[data-flow-node="key-22"]').click()
    await page.waitForURL(/key=22/)
    const filtered = await rows()
    const keys = await page.locator('[aria-labelledby="overview-recent-title"] tbody tr td:nth-child(3)').allTextContents()
    check('ops: click a Key filters rows and writes ?key=', filtered > 0 && filtered < before && keys.every((k) => k.includes('Synthetic same-name Key')), { before, filtered, keys, url: page.url() })
    check('ops: selected node is pressed', (await page.locator('[data-flow-node="key-22"]').getAttribute('aria-pressed')) === 'true', null)
    await shot(page, 'ops-key-selected-1280-light-zh.png')
    // Refresh keeps the selection.
    await page.reload()
    await page.waitForSelector('[data-testid="flow-filter-chip"]')
    check('ops: refresh keeps the selection', (await page.locator('[data-flow-node="key-22"]').getAttribute('aria-pressed')) === 'true' && (await rows()) === filtered, page.url())
    // Back after selecting another node returns to the previous selection.
    await page.locator('[data-flow-node="key-21"]').click()
    await page.waitForURL(/key=21/)
    await page.goBack()
    await page.waitForURL(/key=22/)
    await page.waitForTimeout(300)
    check('ops: Back restores the previous node', (await page.locator('[data-flow-node="key-22"]').getAttribute('aria-pressed')) === 'true', page.url())
    // Clear with the chip.
    await page.getByTestId('flow-filter-chip').getByRole('button').click()
    await page.waitForTimeout(300)
    check('ops: chip clears the filter and the URL', !/key=/.test(page.url()) && (await rows()) === before && (await page.getByTestId('flow-filter-chip').count()) === 0, page.url())
    // Keyboard: Tab to a node, Enter selects, Space clears.
    const node = page.locator('[data-flow-node="key-21"]')
    await node.focus()
    // Reach it by keyboard so :focus-visible applies as for a real user.
    await page.keyboard.press('Shift+Tab')
    await page.keyboard.press('Tab')
    const ring = await node.evaluate((el) => getComputedStyle(el).outlineStyle + ' ' + getComputedStyle(el).outlineWidth)
    check('ops: focused node shows a focus ring', !ring.startsWith('none'), ring)
    await shot(page, 'ops-key-focus-1280-light-zh.png')
    await page.keyboard.press('Enter')
    await page.waitForURL(/key=21/)
    await page.keyboard.press('Space')
    await page.waitForTimeout(300)
    check('ops: Enter selects and Space clears a focused node', !/key=/.test(page.url()), page.url())
    // Copy the access address.
    await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: ORIGIN })
    await page.getByTestId('flow-core').getByRole('button').first().click()
    const copied = await page.evaluate(() => navigator.clipboard.readText())
    check('ops: copy puts the base URL on the clipboard', copied === `${ORIGIN}/v1`, copied)
    // The time link opens the request details.
    const link = page.locator('[aria-labelledby="overview-recent-title"] tbody tr').first().getByRole('link')
    const href = await link.getAttribute('href')
    await link.click()
    await page.waitForURL(/inspect=request/)
    await page.waitForTimeout(1200)
    const detail = await page.locator('[data-inspector]').count()
    check('ops: time link opens the request details', /requestId=/.test(href) && detail > 0, { href, detail })
    await shot(page, 'ops-request-detail-1280-light-zh.png')
    check('ops: no page errors', errors.length === 0, errors)
    await context.close()
  }
  // Phone: tapping a node brings the requests into view.
  {
    const { context, page, errors } = await open('/dashboard/overview', { width: 390, height: 844, mobile: true })
    await page.locator('[data-flow-node="key-22"]').tap()
    await page.waitForTimeout(800)
    const box = await page.locator('[aria-labelledby="overview-recent-title"]').boundingBox()
    check('ops: phone tap scrolls the requests into view', box && box.y >= 0 && box.y < 844 * 0.5, box)
    await shot(page, 'ops-phone-selected-390-light-zh.png')
    check('ops: phone no page errors', errors.length === 0, errors)
    await context.close()
  }
}

await browser.close()
const failed = checks.filter((c) => !c.pass)
writeFileSync(path.join(outDir, 'browser-checks.json'), JSON.stringify({ blocked, shots, total: checks.length, failed: failed.length, checks }, null, 2))
console.log(JSON.stringify({ blocked, shots: shots.length, checks: checks.length, failed: failed.length }))
process.exit(failed.length ? 1 : 0)
