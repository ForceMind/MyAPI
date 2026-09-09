// Render the built quota UI using synthetic APIs, or an explicitly supplied
// redacted read-only quota fixture. No live API or upstream requests are allowed.
import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { createReadStream, existsSync, mkdirSync, readFileSync, statSync } from 'node:fs'
import { extname, resolve, sep } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { quotaFixtures } from '../quota/browser-fixtures.mjs'

const repo = fileURLToPath(new URL('../../', import.meta.url))
const root = resolve(repo, 'web/dist')
const output = resolve(process.env.MYAPI_UI_REDESIGN_BROWSER_ARTIFACTS || '/tmp/myapi-ui-redesign-browser')
const translations = JSON.parse(readFileSync(resolve(repo, 'web/src/i18n/locales/zh.json'), 'utf8')).translation
const label = (key) => translations[key] || key
const mime = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.png': 'image/png', '.woff2': 'font/woff2' }
assert(existsSync(resolve(root, 'index.html')), 'Build the frontend before browser regression')
const server = createServer((request, response) => {
  let path
  try { path = resolve(root, `.${decodeURIComponent(new URL(request.url || '/', 'http://localhost').pathname)}`) }
  catch { response.writeHead(400).end(); return }
  if (!path.startsWith(`${root}${sep}`) && path !== root) { response.writeHead(403).end(); return }
  if (!existsSync(path) || statSync(path).isDirectory()) path = resolve(root, 'index.html')
  response.writeHead(200, { 'content-type': mime[extname(path)] || 'application/octet-stream', 'cache-control': 'no-store' })
  createReadStream(path).pipe(response)
})
const driver = process.env.MYAPI_PLAYWRIGHT_MODULE
const { chromium } = await import(driver ? pathToFileURL(resolve(driver)).href : 'playwright')
let browser
const now = Math.floor(Date.now() / 1000)
const failures = []
const unexpected = new Set()
const requests = []
let role = 100
let summaryFails = false
let emptyOverview = false
let channelRead = true
const policy = { enabled: false, sticky_enabled: true, session_ttl_seconds: 86400, quota_max_age_seconds: 600 }
const channels = [{ id: 1, name: 'Codex · 浏览器测试', type: 57, status: 1, priority: 0, weight: 30, quota: { state: 'fresh', available: 85, unit: 'percent', observed_at: now } }]
try {
  await new Promise(done => server.listen(0, '127.0.0.1', done))
  const origin = `http://127.0.0.1:${server.address().port}`
  browser = await chromium.launch({ headless: true, executablePath: process.env.MYAPI_CHROMIUM_PATH || undefined, args: ['--disable-dev-shm-usage'] })
  const context = await browser.newContext({ viewport: { width: 1280, height: 720 }, locale: 'zh-CN', reducedMotion: 'reduce' })
  await context.addInitScript(() => { localStorage.setItem('i18nextLng', 'zhCN'); localStorage.setItem('theme', 'light') })
  const fixtures = quotaFixtures()
  await context.route('**/api/**', async route => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/\/$/, '')
    requests.push(path)
    if (path !== '/api/user/auth/refresh') assert.equal(request.method(), 'GET', `overview must be read-only: ${path}`)
    let response
    let status = 200
    if (path === '/api/channel/routing') response = { success: true, data: { policy, channels: emptyOverview ? [] : channels } }
    else if (path.endsWith('/request-summary')) {
      const start = Number(url.searchParams.get('start_timestamp'))
      const end = Number(url.searchParams.get('end_timestamp'))
      assert(start > 0 && end > start, 'request metrics use an explicit common period')
      status = summaryFails ? 500 : 200
      response = summaryFails ? { success: false, message: 'Synthetic summary unavailable' } : { success: true, data: {
        start_timestamp: start, end_timestamp: end, total_requests: emptyOverview ? 0 : 100, successful_requests: emptyOverview ? 0 : 92, failed_requests: emptyOverview ? 0 : 8,
        success_rate: emptyOverview ? null : 92, coverage: { complete: false, reason: 'recorded_logs_only', identified_requests: emptyOverview ? 0 : 100, unidentified_log_rows: 0, consume_logs_enabled: true, error_logs_enabled: true, window_semantics: 'log_events_within_range', deduplication: 'request_id_success_precedence' },
      } }
    } else if (emptyOverview && path === '/api/channel/quota/changes') response = { success: true, data: {
      items: [], range: url.searchParams.get('range') || '24h', generated_at: now,
      source_complete: true, items_complete: true,
    } }
    else if (path === '/api/log' || path === '/api/log/self') response = { success: true, data: { items: [], total: 0 } }
    else response = fixtures.response(url)
    const permissions = { admin_permissions: { channel: { read: channelRead, operate: channelRead, write: channelRead, sensitive_write: channelRead, secret_view: channelRead } } }
    if (path === '/api/user/auth/refresh' && response?.data?.user) response.data.user = { ...response.data.user, role, permissions }
    if (path === '/api/user/self' && response?.data) response.data = { ...response.data, role, permissions }
    if (!response) unexpected.add(path)
    await route.fulfill({ status: response ? status : 501, json: response || { success: false } })
  })
  await context.route('**/*', route => new URL(route.request().url()).origin === origin ? route.fallback() : route.abort())
  const page = await context.newPage()
  page.setDefaultTimeout(15000)
  page.on('pageerror', error => failures.push(error.message))
  mkdirSync(output, { recursive: true })
  await page.goto(`${origin}/dashboard/overview`, { waitUntil: 'networkidle' })
  await page.getByText('92%', { exact: false }).first().waitFor()
  const quotaChart = page.getByTestId('quota-comparison-chart')
  await quotaChart.locator('.recharts-line-curve').first().waitFor()
  for (const viewport of [{ width: 1280, height: 720 }, { width: 1440, height: 900 }, { width: 390, height: 844 }, { width: 320, height: 740 }]) {
    await page.setViewportSize(viewport)
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), `overview width ${viewport.width} must not overflow`)
    if (viewport.width === 1280) {
      const bounds = await quotaChart.boundingBox()
      await page.screenshot({ path: resolve(output, 'overview-first-viewport.png'), fullPage: true })
      assert(bounds && bounds.y >= 0 && bounds.y + bounds.height <= 720, `overview chart and time axis must fit the initial desktop viewport: ${JSON.stringify(bounds)}`)
    }
    await page.screenshot({ path: resolve(output, `overview-${viewport.width}.png`), fullPage: true })
  }
  await page.evaluate(() => localStorage.setItem('theme', 'dark'))
  await page.reload({ waitUntil: 'networkidle' })
  await page.screenshot({ path: resolve(output, 'overview-dark.png'), fullPage: true })
  summaryFails = true
  await page.reload({ waitUntil: 'networkidle' })
  assert.equal(await page.getByText('92%', { exact: false }).count(), 0, 'failed summary does not show stale success as current')
  await page.screenshot({ path: resolve(output, 'overview-error.png'), fullPage: true })
  summaryFails = false

  emptyOverview = true
  await page.evaluate(() => localStorage.setItem('theme', 'light'))
  await page.reload({ waitUntil: 'networkidle' })
  await page.getByText(label('No quota history data yet'), { exact: true }).waitFor()
  assert.equal(await quotaChart.locator('.recharts-line-curve').count(), 0, 'empty quota state does not render a synthetic series')
  await page.screenshot({ path: resolve(output, 'overview-empty.png'), fullPage: true })
  emptyOverview = false

  role = 10
  channelRead = false
  requests.length = 0
  await page.reload({ waitUntil: 'networkidle' })
  assert(!requests.some(path => path.startsWith('/api/channel')), 'an administrator without channel read permission never requests channel metadata')
  assert.equal(await page.getByTestId('quota-comparison-chart').count(), 0, 'channel quota UI is hidden without channel read permission')
  assert((await page.getByText(label('Unavailable'), { exact: true }).count()) >= 2, 'permission-limited operational summary values are unavailable')
  await page.screenshot({ path: resolve(output, 'overview-channel-read-denied.png'), fullPage: true })

  role = 1
  requests.length = 0
  await page.reload({ waitUntil: 'networkidle' })
  assert(requests.includes('/api/log/self/request-summary'), 'ordinary user requests only personal request summary')
  assert(!requests.some(path => path.startsWith('/api/channel')), 'ordinary overview never requests channel metadata')
  assert(!requests.includes('/api/log/request-summary'), 'ordinary overview never requests global summary')
  await page.screenshot({ path: resolve(output, 'overview-personal.png'), fullPage: true })
  role = 100
  channelRead = true
  await page.setViewportSize({ width: 1280, height: 720 })
  await page.goto(`${origin}/channels`, { waitUntil: 'networkidle' })
  assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), 'desktop channels page must not overflow')
  await page.screenshot({ path: resolve(output, 'channels-desktop.png'), fullPage: true })
  const edit = page.getByRole('button', { name: label('Edit'), exact: true }).first()
  await edit.click()
  const drawer = page.getByRole('dialog', { name: label('Edit Channel'), exact: false })
  await drawer.waitFor()
  const desktopDrawerBounds = await drawer.boundingBox()
  assert(desktopDrawerBounds && desktopDrawerBounds.width < 1280 && Math.abs(desktopDrawerBounds.x + desktopDrawerBounds.width - 1280) <= 1, 'desktop channel editor is a right-side drawer')
  await page.screenshot({ path: resolve(output, 'channel-desktop-drawer.png'), fullPage: true })
  await page.keyboard.press('Escape')
  await drawer.waitFor({ state: 'hidden' })
  assert(await edit.evaluate(element => element === document.activeElement), 'closing the desktop editor restores focus to the edit action')

  await page.setViewportSize({ width: 390, height: 844 })
  const mobileMenu = page.getByRole('button', { name: label('Open menu'), exact: true }).last()
  await mobileMenu.click()
  await page.getByRole('menuitem', { name: label('Edit'), exact: true }).click()
  await drawer.waitFor()
  const drawerBounds = await drawer.boundingBox()
  assert(drawerBounds && Math.abs(drawerBounds.width - 390) <= 1, 'mobile channel editor fills the viewport')
  const name = drawer.getByLabel(label('Name *'), { exact: true })
  await name.fill('Browser unsaved channel')
  await page.keyboard.press('Escape')
  const confirm = page.getByRole('alertdialog', { name: label('Unsaved changes') })
  await confirm.waitFor()
  await confirm.getByRole('button', { name: label('Cancel'), exact: true }).click()
  assert.equal(await name.inputValue(), 'Browser unsaved channel', 'canceling dismissal retains the draft')
  await page.screenshot({ path: resolve(output, 'channel-mobile-draft.png'), fullPage: true })
  await page.keyboard.press('Escape')
  await confirm.getByRole('button', { name: label('Leave'), exact: true }).click()
  await drawer.waitFor({ state: 'hidden' })
  assert(await mobileMenu.evaluate(element => element === document.activeElement), 'closing the mobile editor restores focus to the row actions')
  assert.deepEqual([...unexpected], [], 'all API requests are explicitly covered')
  assert.deepEqual(failures, [], 'no browser runtime errors')
  console.log(JSON.stringify({ result: 'pass', artifacts: output }))
} finally {
  await browser?.close()
  await new Promise(done => server.close(done))
}
