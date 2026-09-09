// Run against a built web/dist, never against production. All API requests are
// fulfilled using explicit synthetic fixtures; unexpected requests fail the test.
import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { createReadStream, existsSync, mkdirSync, readFileSync, statSync } from 'node:fs'
import { extname, resolve, sep } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { quotaFixtures } from './browser-fixtures.mjs'

const repo = fileURLToPath(new URL('../../', import.meta.url))
const root = resolve(repo, 'web/dist')
const output = resolve(process.env.MYAPI_BROWSER_ARTIFACTS || `${repo}/.local-tests/quota-browser`)
const zh = JSON.parse(readFileSync(resolve(repo, 'web/src/i18n/locales/zh.json'), 'utf8')).translation
const label = (key) => zh[key] || key
const mime = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.png': 'image/png', '.woff2': 'font/woff2' }
assert(existsSync(resolve(root, 'index.html')), 'Build the frontend before browser regression')
const server = createServer((request, response) => {
  const pathname = new URL(request.url || '/', 'http://localhost').pathname
  let path
  try { path = resolve(root, `.${decodeURIComponent(pathname)}`) } catch { response.writeHead(400).end(); return }
  if (!path.startsWith(`${root}${sep}`) && path !== root) { response.writeHead(403).end(); return }
  if (!existsSync(path) || statSync(path).isDirectory()) path = resolve(root, 'index.html')
  response.writeHead(200, { 'content-type': mime[extname(path)] || 'application/octet-stream', 'cache-control': 'no-store' })
  createReadStream(path).pipe(response)
})
const driver = process.env.MYAPI_PLAYWRIGHT_MODULE
const { chromium } = await import(driver ? pathToFileURL(resolve(driver)).href : 'playwright')
let browser
let page
const errors = []
const unexpected = new Set()
try {
  await new Promise((done) => server.listen(0, '127.0.0.1', done))
  const origin = `http://127.0.0.1:${server.address().port}`
  browser = await chromium.launch({ headless: true, executablePath: process.env.MYAPI_CHROMIUM_PATH || undefined, args: ['--disable-dev-shm-usage'] })
  const context = await browser.newContext({ viewport: { width: 1280, height: 720 }, locale: 'zh-CN', reducedMotion: 'reduce', timezoneId: 'Asia/Shanghai' })
  await context.addInitScript(() => { localStorage.setItem('i18nextLng', 'zhCN'); localStorage.setItem('theme', 'light') })
  let latestError = false
  const historyRequests = []
  const changeRequests = []
  await context.route('**/api/**', async (route) => {
    const url = new URL(route.request().url())
    if (url.pathname.endsWith('/quota/history')) historyRequests.push(Object.fromEntries(url.searchParams))
    if (url.pathname.endsWith('/quota/changes')) changeRequests.push(Object.fromEntries(url.searchParams))
    const response = quotaFixtures({ latestError }).response(url)
    if (!response) unexpected.add(`${route.request().method()} ${url.pathname}`)
    await route.fulfill({ status: response ? 200 : 501, json: response || { success: false, message: 'Unconfigured browser fixture' } })
  })
  // Block external traffic as well: these tests need only locally built assets.
  await context.route('**/*', (route) => {
    const url = new URL(route.request().url())
    if (url.origin === origin || url.protocol === 'data:') return route.fallback()
    return route.abort()
  })
  page = await context.newPage()
  page.setDefaultTimeout(15000)
  page.on('pageerror', (error) => errors.push(error.message))
  page.on('console', (message) => { if (message.type() === 'error') errors.push(message.text()) })
  mkdirSync(output, { recursive: true })
  const trend = () => page.getByTestId('quota-history-trend').first()
  async function checkChart(scope, style) {
    await scope.getByLabel(label('Chart style'), { exact: true }).selectOption(style)
    const chart = scope.getByTestId(`quota-history-chart-${style}`)
    const shape = { line: '.recharts-line-curve', area: '.recharts-area-area', bar: '.recharts-bar-rectangle' }[style]
    await chart.locator(shape).first().waitFor({ state: 'visible' })
    const bounds = await chart.boundingBox()
    assert(bounds && bounds.width > 150 && bounds.height > 100, `real ${style} chart has a usable size`)
  }
  async function checkControls(scope) {
    for (const style of ['line', 'area', 'bar']) await checkChart(scope, style)
    await scope.getByLabel(label('Metric'), { exact: true }).selectOption('rate_per_minute')
    await checkChart(scope, 'line')
    assert((await scope.innerText()).includes(label('Estimated consumption per minute')), 'estimated rate label is localized')
    for (const grain of ['minute', '5m', '15m', 'hour', 'day', 'week', 'raw', 'auto']) {
      await scope.getByLabel(label('Chart granularity'), { exact: true }).selectOption(grain)
      // A fresh React Query entry may be reused when switching back; its
      // original request still must have asked for this exact granularity.
      for (let attempt = 0; attempt < 50 && !historyRequests.some((request) => request.granularity === grain); attempt++) await page.waitForTimeout(50)
      assert(historyRequests.some((request) => request.granularity === grain), `history API receives ${grain}`)
    }
    const previous = historyRequests.length
    await scope.getByLabel(label('Time range'), { exact: true }).selectOption('7d')
    await page.waitForTimeout(120)
    assert(historyRequests.slice(previous).some((request) => request.range === '7d'), 'range changes query actual history')
    const beforeMethod = historyRequests.length
    await scope.getByRole('combobox', { name: label('Consumption rate'), exact: true }).selectOption('ewma')
    await page.waitForTimeout(120)
    assert.equal(historyRequests.length, beforeMethod, 'switching the displayed analysis method does not refetch history')
    await scope.getByRole('combobox', { name: label('Analysis window'), exact: true }).selectOption('21600')
    for (let attempt = 0; attempt < 50 && !historyRequests.slice(beforeMethod).some((request) => request.rate_window === '21600'); attempt++) await page.waitForTimeout(50)
    assert(historyRequests.slice(beforeMethod).some((request) => request.rate_window === '21600'), 'analysis window reaches the history API independently')
    const beforeHalfLife = historyRequests.length
    await scope.getByRole('combobox', { name: label('EWMA half-life'), exact: true }).selectOption('3600')
    for (let attempt = 0; attempt < 50 && !historyRequests.slice(beforeHalfLife).some((request) => request.ewma_half_life === '3600'); attempt++) await page.waitForTimeout(50)
    assert(historyRequests.slice(beforeHalfLife).some((request) => request.ewma_half_life === '3600'), 'EWMA half-life reaches the history API independently')
    await scope.getByLabel(label('Time range'), { exact: true }).selectOption('custom')
    const localDate = (value) => new Date(value + 8 * 3600000).toISOString().slice(0, 16)
    const now = Date.now()
    await scope.getByLabel(label('Custom range start'), { exact: true }).fill(localDate(now - 181 * 86400000))
    await scope.getByLabel(label('Custom range end'), { exact: true }).fill(localDate(now))
    assert(await scope.getByRole('button', { name: label('Apply Filters'), exact: true }).isDisabled(), 'an invalid range cannot be submitted')
    await scope.getByText(label('Choose a valid time range of at most 180 days.'), { exact: true }).waitFor({ state: 'visible' })
    const customStart = localDate(now - 2 * 3600000)
    await scope.getByLabel(label('Custom range start'), { exact: true }).fill(customStart)
    const expectedStart = new Date(`${customStart}+08:00`).toISOString()
    const beforeCustom = historyRequests.length
    await scope.getByRole('button', { name: label('Apply Filters'), exact: true }).click()
    for (let attempt = 0; attempt < 50 && !historyRequests.slice(beforeCustom).some((request) => request.start === expectedStart); attempt++) await page.waitForTimeout(50)
    assert(historyRequests.slice(beforeCustom).some((request) => request.range === 'custom' && request.start === expectedStart), 'custom date applies to the real history query')
    await scope.getByLabel(label('Metric'), { exact: true }).selectOption('consumption')
    await checkChart(scope, 'bar')
  }
  async function showWholeChart(scope, bottomLimit) {
    const chart = scope.getByTestId('quota-history-chart-bar')
    const viewport = page.viewportSize()
    for (let step = 0; step < 20; step++) {
      const box = await chart.boundingBox()
      assert(box, 'chart has layout bounds')
      if (box.y >= 100 && box.y + box.height <= bottomLimit) break
      // Scroll the page gutter: the comparison plot intentionally consumes wheel events for zoom.
      const scrollX = await chart.evaluate((node) => {
        if (node.closest('[role=dialog]')) return null
        for (let parent = node.parentElement; parent; parent = parent.parentElement) {
          if (/(auto|scroll)/.test(getComputedStyle(parent).overflowY) && parent.scrollHeight > parent.clientHeight) return parent.getBoundingClientRect().right - 18
        }
        return 4
      })
      await page.mouse.move(scrollX ?? Math.min(viewport.width - 60, box.x + box.width / 2), viewport.height / 2)
      const delta = box.y + box.height > bottomLimit ? box.y + box.height - bottomLimit : box.y - 100
      await page.mouse.wheel(0, Math.max(-220, Math.min(220, delta)))
      await page.waitForTimeout(80)
    }
    const box = await chart.boundingBox()
    if (!(box && box.y >= 99 && box.y + box.height <= bottomLimit + 1)) console.error('scroll ancestry', await chart.evaluate(node => { const rows=[]; for(let p=node;p;p=p.parentElement) rows.push({tag:p.tagName,cls:p.className,overflow:getComputedStyle(p).overflowY,scroll:p.scrollTop,h:p.clientHeight,sh:p.scrollHeight,rect:p.getBoundingClientRect().toJSON()}); return rows }))
    assert(box && box.y >= 99 && box.y + box.height <= bottomLimit + 1, `the whole chart, including its time axis, is reachable by real scrolling: ${JSON.stringify({box, bottomLimit, viewport})}`)
  }
  await page.goto(`${origin}/dashboard/overview`, { waitUntil: 'networkidle' })
  await page.getByRole('button', { name: label('Account quota changes'), exact: true }).click()
  const overview = page.getByTestId('quota-overview-card').first()
  await overview.waitFor({ state: 'visible' })
  const overviewText = await overview.innerText()
  for (const key of ['Remaining quota', 'Latest observed interval', 'Average consumption per minute', 'Estimated consumption per hour', 'Estimated time from analysis point']) {
    assert(overviewText.includes(label(key)), `overview includes ${key}`)
  }
  await overview.getByTestId('quota-overview-sparkline').locator('polyline, circle').first().waitFor({ state: 'visible' })
  for (const key of ['Time range', 'Chart granularity', 'Metric', 'Chart style', 'Analysis window', 'EWMA half-life']) {
    assert.equal(await overview.getByLabel(label(key), { exact: true }).count(), 0, `overview omits detailed ${key} control`)
  }
  assert(changeRequests.some((request) => request.range === '24h' && request.rate_window === '3600' && request.ewma_half_life === '1800' && request.overview_points === '48' && request.limit === '4' && request.sort === 'observed_desc'), 'overview uses one bounded analysis query')
  await overview.screenshot({ path: resolve(output, 'overview-quota-summary.png') })

  await page.goto(`${origin}/channels?tab=quota`, { waitUntil: 'networkidle' })
  await page.getByText(label('Sampling details and diagnostics'), { exact: true }).click()
  await trend().waitFor({ state: 'visible' })
  await checkControls(trend())
  await trend().screenshot({ path: resolve(output, 'channel-consumption.png') })

  // Latest failure must remain visible without discarding valid historical data.
  latestError = true
  await page.reload({ waitUntil: 'networkidle' })
  await page.getByText(label('Sampling details and diagnostics'), { exact: true }).click()
  await trend().waitFor({ state: 'visible' })
  await checkChart(trend(), 'bar')
  assert((await trend().innerText()).includes(label('Latest raw sample')), 'latest status displayed separately')
  await trend().screenshot({ path: resolve(output, 'channel-latest-error.png') })
  latestError = false
  await page.reload({ waitUntil: 'networkidle' })

  await page.getByRole('tab', { name: label('Channel management'), exact: true }).click()

  // Exercise the existing channel-row entry, rather than a test-only component.
  await page.getByRole('button', { name: label('Open menu'), exact: true }).last().click()
  await page.getByRole('menuitem', { name: label('Query Balance'), exact: true }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('tab', { name: label('History trend'), exact: true }).click()
  const dialogTrend = dialog.getByTestId('quota-history-trend').first()
  await dialogTrend.waitFor({ state: 'visible' })
  await checkControls(dialogTrend)
  await dialog.screenshot({ path: resolve(output, 'codex-dialog-consumption.png') })
  const dialogBox = await dialog.boundingBox()
  await showWholeChart(dialogTrend, dialogBox.y + dialogBox.height - 90)
  await dialog.screenshot({ path: resolve(output, 'codex-dialog-chart.png') })
  await page.keyboard.press('Escape')

  // Exercise the real Codex channel editor and local-login import wizard.
  await page.getByRole('button', { name: label('Open menu'), exact: true }).last().click()
  await page.getByRole('menuitem', { name: label('Edit'), exact: true }).click()
  const channelEditor = page.getByRole('dialog').filter({ hasText: label('Edit Channel') }).last()
  await channelEditor.waitFor({ state: 'visible' })
  await channelEditor.getByRole('button', { name: label('Import local Codex'), exact: true }).click()
  const localAuthDialog = page.getByRole('dialog', { name: label('Import local Codex'), exact: true })
  await localAuthDialog.getByText(label('Local Codex account detected'), { exact: true }).waitFor({ state: 'visible' })
  assert((await localAuthDialog.innerText()).includes('acct…1234'), 'local-auth wizard shows only the masked account hint')
  assert(!(await localAuthDialog.innerText()).includes('access_token'), 'automatic local-auth view never renders credential fields')
  await localAuthDialog.screenshot({ path: resolve(output, 'codex-local-auth-ready.png') })
  await localAuthDialog.getByRole('button', { name: label('Manual import'), exact: true }).click()
  await localAuthDialog.getByLabel(label('Codex credential JSON'), { exact: true }).waitFor({ state: 'visible' })
  await page.setViewportSize({ width: 390, height: 844 })
  assert(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1), 'local-auth wizard has no page overflow at 390px')
  await localAuthDialog.screenshot({ path: resolve(output, 'codex-local-auth-manual-390x844.png') })
  const manualCancel = localAuthDialog.getByRole('button', { name: label('Cancel'), exact: true })
  for (let step = 0; step < 12; step++) {
    const box = await manualCancel.boundingBox()
    if (box && box.y >= 0 && box.y + box.height <= 844) break
    await page.mouse.move(195, 720)
    await page.mouse.wheel(0, 500)
    await page.waitForTimeout(80)
  }
  const manualCancelBox = await manualCancel.boundingBox()
  assert(manualCancelBox && manualCancelBox.y >= 0 && manualCancelBox.y + manualCancelBox.height <= 844, 'local-auth manual actions are reachable by real dialog scrolling')
  const manualCancelReachable = await manualCancel.evaluate((button) => {
    const rect = button.getBoundingClientRect()
    const target = document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2)
    return target === button || button.contains(target)
  })
  assert(manualCancelReachable, 'local-auth manual actions are not obscured')
  await localAuthDialog.screenshot({ path: resolve(output, 'codex-local-auth-manual-actions-390x844.png') })
  await page.keyboard.press('Escape')
  await page.keyboard.press('Escape')

  for (const viewport of [{ width: 320, height: 740 }, { width: 390, height: 844 }, { width: 1280, height: 600 }]) {
    await page.setViewportSize(viewport)
    await page.goto(`${origin}/channels?tab=quota`, { waitUntil: 'networkidle' })
    await page.getByText(label('Sampling details and diagnostics'), { exact: true }).click()
    await trend().waitFor({ state: 'visible' })
    await checkChart(trend(), 'bar')
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1), `no page overflow at ${viewport.width}`)
    await showWholeChart(trend(), viewport.height - 80)
    await page.screenshot({ path: resolve(output, `channel-chart-${viewport.width}x${viewport.height}.png`) })
    await page.getByRole('tab', { name: label('Channel management'), exact: true }).click()
    const rowMenu = page.getByRole('button', { name: label('Open menu'), exact: true }).last()
    // Real wheel scrolling, not scrollIntoView (which can scroll an
    // overflow:hidden ancestor programmatically and hide a production bug).
    await page.mouse.move(viewport.width - 60, Math.min(350, viewport.height - 100))
    for (let step = 0; step < 12; step++) {
      const box = await rowMenu.boundingBox()
      if (box && box.y > 70 && box.y + box.height < viewport.height - 60) break
      await page.mouse.wheel(0, 450)
      await page.waitForTimeout(80)
    }
    const box = await rowMenu.boundingBox()
    assert(box && box.y > 70 && box.y + box.height < viewport.height - 60, `channel table can be reached with wheel at ${viewport.width}x${viewport.height}`)
    const reachable = await rowMenu.evaluate((button) => { const rect = button.getBoundingClientRect(); const target = document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2); return target === button || button.contains(target) })
    assert(reachable, 'channel actions are not obscured by a fixed footer or clipped chart')
    await page.screenshot({ path: resolve(output, `channels-${viewport.width}x${viewport.height}.png`), fullPage: true })
  }
  assert.deepEqual([...unexpected], [], 'all application endpoints have explicit fixtures')
  assert.deepEqual(errors, [], 'no browser runtime errors')
  console.log('Quota browser regression passed: compact overview summary/sparkline; detailed line/area/bar and analysis controls; latest-error history; 320px/390px/low-height layout. Synthetic fixtures only.')
} catch (error) {
  if (page) {
    await page.screenshot({ path: resolve(output, 'failure.png'), fullPage: true }).catch(() => {})
    console.error('Browser diagnostics:', JSON.stringify({ errors, unexpected: [...unexpected], pageText: (await page.locator('body').innerText().catch(() => '')).slice(-3500) }))
  }
  throw error
} finally {
  if (browser) await browser.close()
  await new Promise((done) => server.close(done))
}
