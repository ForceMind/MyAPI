// Run against a built web/dist, never against production. All API requests are
// fulfilled using explicit synthetic fixtures; unexpected requests fail the test.
import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { createReadStream, existsSync, mkdirSync, readFileSync, statSync } from 'node:fs'
import { extname, resolve, sep } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { quotaFixtures } from './browser-fixtures.mjs'
import { tokenBudgetBrowserFixture, checkTokenBudgetBrowser } from './token-budget-browser.mjs'
import { userUsagePolicyBrowserFixture, checkUserUsagePolicyBrowser } from './user-usage-policy-browser.mjs'

// The shipped overview compares multiple stable account identities in one plot.
process.env.MYAPI_BROWSER_MULTISERIES = '1'

const repo = fileURLToPath(new URL('../../', import.meta.url))
const root = resolve(repo, 'web/dist')
const output = resolve(process.env.MYAPI_BROWSER_ARTIFACTS || `${repo}/.local-tests/quota-browser`)
const zh = JSON.parse(readFileSync(resolve(repo, 'web/src/i18n/locales/zh.json'), 'utf8')).translation
const label = (key) => zh[key] || key
const mime = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.png': 'image/png', '.woff2': 'font/woff2' }
assert(existsSync(resolve(root, 'index.html')), 'Build the frontend before browser regression')
const server = createServer((request, response) => {
  const pathname = new URL(request.url || '/', 'http://localhost').pathname
  if (pathname.startsWith('/api/')) {
    const payload = quotaFixtures().response(new URL(request.url || '/', 'http://localhost'))
    response.writeHead(payload?.fixture_http_status || (payload ? 200 : 501), { 'content-type': 'application/json', 'cache-control': 'no-store' })
    response.end(JSON.stringify(payload || { success: false, message: 'Unconfigured browser fixture' }))
    return
  }
  let path
  try { path = resolve(root, `.${decodeURIComponent(pathname)}`) } catch { response.writeHead(400).end(); return }
  if (!path.startsWith(`${root}${sep}`) && path !== root) { response.writeHead(403).end(); return }
  if (!existsSync(path) || statSync(path).isDirectory()) path = resolve(root, 'index.html')
  response.writeHead(200, { 'content-type': mime[extname(path)] || 'application/octet-stream', 'cache-control': 'no-store' })
  createReadStream(path).pipe(response)
})
if (process.env.MYAPI_BROWSER_MANUAL === '1') {
  const port = Number(process.env.MYAPI_BROWSER_PORT || 8766)
  await new Promise((done) => server.listen(port, '127.0.0.1', done))
  console.log(`Synthetic browser fixture available at http://127.0.0.1:${server.address().port}`)
  await new Promise(() => {})
}
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
  const reviewSubmissions = []
  const publicationSubmissions = []
  const budgetFixture = tokenBudgetBrowserFixture()
  const userPolicyFixture = userUsagePolicyBrowserFixture()
  const publication = { revision: 0, locked: false, active: false, receipts: [] }
  await context.route('**/api/**', async (route) => {
    const url = new URL(route.request().url())
    if (await budgetFixture.route(route, url)) return
    if (await userPolicyFixture.route(route, url)) return
    if (url.pathname.endsWith('/publication-preview')) {
      const expression = 'v1:len <= 272000 ? tier("short", p * 2 + c * 10 + cr * 0 + cc * 2.5) : tier("long", p * 4 + c * 15 + cr * 0.2 + cc * 5)'
      await route.fulfill({ json: { success: true, data: { source_sha256: 'f'.repeat(64), expected_digest: 'a'.repeat(64), revision: publication.revision, rows: [
        { model: 'fixture-cached-model', current_mode: publication.active ? 'tiered_expr' : 'ratio', current_expression: publication.active ? expression : '', locked: publication.locked, eligible: !publication.locked,
          candidate: { model: 'fixture-cached-model', expression, expression_sha256: 'b'.repeat(64), source_sha256: 'f'.repeat(64) } },
        { model: 'fixture-no-cache', current_mode: 'ratio', current_expression: '', locked: false, eligible: false, candidate: null },
      ] } } })
      return
    }
    if (url.pathname === '/api/ratio_sync/openai/publications') {
      if (route.request().method() === 'POST') {
        const body = route.request().postDataJSON()
        assert.equal(body.confirmed, true)
        assert.match(body.id, /^[0-9a-f]{64}$/)
        publicationSubmissions.push(body)
        publication.revision++
        publication.active = body.action === 'publish'
        publication.locked = publication.active
        const receipt = { id: body.id, actor_id: 1, action: body.action, revision: publication.revision, created_at: Math.floor(Date.now() / 1000) }
        publication.receipts.unshift(receipt)
        await route.fulfill({ json: { success: true, data: { receipt, runtime_ready: true } } })
      } else {
        await route.fulfill({ json: { success: true, data: { expected_digest: 'a'.repeat(64), runtime_ready: true, snapshot: { state: { revision: publication.revision } }, receipts: publication.receipts } } })
      }
      return
    }
    if (url.pathname.endsWith('/quota/history')) historyRequests.push(Object.fromEntries(url.searchParams))
    if (url.pathname.endsWith('/quota/changes')) changeRequests.push(Object.fromEntries(url.searchParams))
    if (url.pathname === '/api/usage-review/usage-review-fixture/reconcile' && route.request().method() === 'POST') reviewSubmissions.push(route.request().postDataJSON())
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
      await page.mouse.move(Math.min(viewport.width - 60, box.x + box.width / 2), viewport.height / 2)
      const delta = box.y + box.height > bottomLimit ? box.y + box.height - bottomLimit : box.y - 100
      await page.mouse.wheel(0, Math.max(-220, Math.min(220, delta)))
      await page.waitForTimeout(80)
    }
    const box = await chart.boundingBox()
    assert(box && box.y >= 99 && box.y + box.height <= bottomLimit + 1, 'the whole chart, including its time axis, is reachable by real scrolling')
  }
  await page.goto(`${origin}/dashboard/overview`, { waitUntil: 'networkidle' })
  const comparison = page.getByTestId('codex-account-quota-chart')
  await comparison.getByTestId('quota-comparison-chart').waitFor({ state: 'visible' })
  const lines = comparison.getByTestId('quota-comparison-line')
  assert.equal(await lines.count(), 3, 'three distinct account identities share the overview time axis')
  for (const line of await lines.all()) assert.equal(await line.getAttribute('stroke-width'), '1.5', 'account series remain thin lines')
  assert.equal(await page.getByTestId('quota-overview-card').count(), 0, 'details start collapsed')
  const legend = comparison.getByTestId('quota-comparison-legend').getByRole('button').first()
  await legend.click()
  await comparison.locator('[data-testid="quota-comparison-line"][data-series-key="' + 'a'.repeat(64) + '"]').waitFor({ state: 'hidden' })
  assert.equal(await legend.getAttribute('aria-pressed'), 'false')
  assert.equal(await lines.count(), 2, 'hiding one account preserves the other account lines')
  await legend.click()
  await comparison.locator('[data-testid="quota-comparison-line"][data-series-key="' + 'a'.repeat(64) + '"]').waitFor({ state: 'visible' })
  assert.equal(await lines.count(), 3)
  await comparison.screenshot({ path: resolve(output, 'overview-account-comparison.png') })
  await comparison.getByRole('button', { name: label('Show details'), exact: true }).click()
  const overview = comparison.getByTestId('quota-overview-card').first()
  await overview.waitFor({ state: 'visible' })
  const overviewText = await overview.innerText()
  for (const key of ['Remaining quota', 'Latest observed interval', 'Average consumption per minute', 'Estimated consumption per hour', 'Estimated time from analysis point']) {
    assert(overviewText.includes(label(key)), `overview includes ${key}`)
  }
  await overview.getByTestId('quota-overview-sparkline').locator('polyline, circle').first().waitFor({ state: 'visible' })
  for (const key of ['Time range', 'Chart granularity', 'Metric', 'Chart style', 'Analysis window', 'EWMA half-life']) {
    assert.equal(await overview.getByLabel(label(key), { exact: true }).count(), 0, `overview omits detailed ${key} control`)
  }
  assert(changeRequests.some((request) => request.range === '24h' && request.rate_window === '3600' && request.ewma_half_life === '1800' && request.overview_points === '48' && request.limit === '64' && request.sort === 'observed_desc'), 'overview uses one bounded analysis query')
  await overview.screenshot({ path: resolve(output, 'overview-quota-summary.png') })
  await comparison.getByRole('button', { name: label('Hide details'), exact: true }).click()
  await overview.waitFor({ state: 'hidden' })

  await page.goto(`${origin}/channels`, { waitUntil: 'networkidle' })
  await trend().waitFor({ state: 'visible' })
  await checkControls(trend())
  await trend().screenshot({ path: resolve(output, 'channel-consumption.png') })

  // Latest failure must remain visible without discarding valid historical data.
  latestError = true
  await page.reload({ waitUntil: 'networkidle' })
  await trend().waitFor({ state: 'visible' })
  await checkChart(trend(), 'bar')
  assert((await trend().innerText()).includes(label('Latest raw sample')), 'latest status displayed separately')
  await trend().screenshot({ path: resolve(output, 'channel-latest-error.png') })
  latestError = false
  await page.reload({ waitUntil: 'networkidle' })

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
    await page.goto(`${origin}/channels`, { waitUntil: 'networkidle' })
    await trend().waitFor({ state: 'visible' })
    await checkChart(trend(), 'bar')
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1), `no page overflow at ${viewport.width}`)
    await showWholeChart(trend(), viewport.height - 80)
    await page.screenshot({ path: resolve(output, `channel-chart-${viewport.width}x${viewport.height}.png`) })
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
  // Exercise the actual log-details recovery form with synthetic API evidence.
  // This proves UI behavior only; the Go/database contracts own authorization
  // and real settlement/idempotency evidence.
  process.env.MYAPI_BROWSER_USAGE_REVIEW = '1'
  await page.goto(`${origin}/usage-logs/common`, { waitUntil: 'networkidle' })
  const unknownRow = page.getByRole('row').filter({ has: page.getByText('fixture-unknown', { exact: true }) })
  await unknownRow.getByText(label('Usage pending review'), { exact: true }).waitFor({ state: 'visible' })
  await unknownRow.getByTitle(label('Click to view full details')).click()
  const reviewDialog = page.getByRole('dialog').filter({ has: page.getByLabel(label('Evidence reference'), { exact: true }) })
  await reviewDialog.getByLabel(label('Confirmed quota (internal units)'), { exact: true }).fill('120')
  await reviewDialog.getByLabel(label('Evidence reference'), { exact: true }).fill('synthetic-verified-usage-evidence')
  await reviewDialog.getByRole('button', { name: label('Confirm reconciliation'), exact: true }).click()
  await reviewDialog.getByText(label('Required'), { exact: true }).waitFor({ state: 'visible' })
  assert.equal(reviewSubmissions.length, 0, 'recovery cannot submit without explicit evidence confirmation')
  await reviewDialog.screenshot({ path: resolve(output, 'usage-review-evidence-required.png') })
  await reviewDialog.getByRole('checkbox', { name: label('I verified the evidence and frozen pricing.'), exact: true }).check()
  // The form disappears after success, so keep a dialog locator independent
  // of form controls when checking the returned resolved state.
  await reviewDialog.getByRole('button', { name: label('Confirm reconciliation'), exact: true }).click()
  const resolvedDialog = page.getByRole('dialog').filter({ has: page.getByText(label('Reconciled'), { exact: true }) })
  await resolvedDialog.getByText(label('Reconciled'), { exact: true }).waitFor({ state: 'visible' })
  assert.deepEqual(reviewSubmissions, [{ actual_quota: 120, evidence_reference: 'synthetic-verified-usage-evidence', confirmed_reliable_evidence: true }])
  assert.equal(await resolvedDialog.getByRole('button', { name: label('Confirm reconciliation'), exact: true }).count(), 0, 'resolved evidence cannot be submitted again from the form')
  await resolvedDialog.screenshot({ path: resolve(output, 'usage-review-resolved.png') })
  await page.keyboard.press('Escape')
  await resolvedDialog.waitFor({ state: 'hidden' })
  // Reuse this isolated Chromium harness for the source -> publication ->
  // guarded rollback UI. SQL/authorization behavior is tested by Go contracts.
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto(`${origin}/system-settings/models/openai-pricing-source`, { waitUntil: 'networkidle' })
  await page.getByRole('button', { name: label('Save official source version'), exact: true }).click()
  await page.getByRole('button', { name: label('Effective price publication'), exact: true }).click()
  const publicationPanel = page.getByRole('region', { name: label('Effective price publication'), exact: true })
  const reviewPrice = publicationPanel.getByRole('button', { name: `${label('Review price change')}: fixture-cached-model`, exact: true })
  await reviewPrice.click()
  await publicationPanel.getByRole('button', { name: label('Confirm'), exact: true }).click()
  await publicationPanel.getByText(label('Required'), { exact: true }).waitFor({ state: 'visible' })
  assert.equal(publicationSubmissions.length, 0, 'source save and unchecked confirmation never publish prices')
  await publicationPanel.getByRole('checkbox', { name: label('I reviewed this price change and its reference-only scope.'), exact: true }).check()
  await publicationPanel.getByRole('button', { name: label('Confirm'), exact: true }).click()
  await publicationPanel.getByText(label('Price operation saved.'), { exact: true }).waitFor({ state: 'visible' })
  await publicationPanel.getByRole('button', { name: `${label('Unlock')}: fixture-cached-model`, exact: true }).waitFor({ state: 'visible' })
  assert.equal(publicationSubmissions.length, 1)
  assert.equal(publicationSubmissions[0].action, 'publish')
  assert.equal(publicationSubmissions[0].source_sha256, 'f'.repeat(64))
  assert.deepEqual(publicationSubmissions[0].models, [{ model: 'fixture-cached-model', locked: true }])
  for (const width of [320, 1280]) {
    await page.setViewportSize({ width, height: 900 })
    // Responsive sidebar/layout transitions can outlive the first two frames.
    // Wait for finite animations, not an arbitrary sleep, before measuring the
    // settled viewport. The original no-overflow assertion remains unchanged.
    await page.evaluate(async () => {
      await new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done)))
      await Promise.all(document.getAnimations().filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity).map((animation) => animation.finished.catch(() => {})))
      await new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done)))
    })
    const layout = await page.evaluate(() => ({
      width: innerWidth, scrollWidth: document.documentElement.scrollWidth,
      overflow: [...document.querySelectorAll('body *')].filter((element) => element.getClientRects().length && element.getBoundingClientRect().right > innerWidth + 1).slice(0, 20).map((element) => ({
        tag: element.tagName, className: String(element.className), text: element.textContent?.trim().slice(0, 70),
        width: element.getBoundingClientRect().width, right: element.getBoundingClientRect().right,
        overflowX: getComputedStyle(element).overflowX, minWidth: getComputedStyle(element).minWidth,
      })),
    }))
    assert(layout.scrollWidth <= layout.width + 1, `price publication has no page-wide horizontal overflow: ${JSON.stringify(layout)}`)
    await page.screenshot({ path: resolve(output, `price-publication-${width}.png`), fullPage: true })
  }
  // Stored history/rollback must survive a reload without fetching the
  // upstream source again (including when the provider is unavailable).
  await page.reload({ waitUntil: 'networkidle' })
  await page.getByRole('button', { name: label('Effective price publication'), exact: true }).click()
  await publicationPanel.getByRole('button', { name: `${label('Rollback')}: ${publicationSubmissions[0].id}`, exact: true }).click()
  await publicationPanel.getByRole('button', { name: label('Confirm'), exact: true }).click()
  await publicationPanel.getByText(label('Required'), { exact: true }).waitFor({ state: 'visible' })
  assert.equal(publicationSubmissions.length, 1, 'rollback requires its own explicit confirmation')
  await publicationPanel.getByRole('checkbox', { name: label('I reviewed this price change and its reference-only scope.'), exact: true }).check()
  await publicationPanel.getByRole('button', { name: label('Confirm'), exact: true }).click()
  await publicationPanel.getByText(label('Price operation saved.'), { exact: true }).waitFor({ state: 'visible' })
  assert.equal(publicationSubmissions.length, 2)
  assert.equal(publicationSubmissions[1].action, 'rollback')
  assert.equal(publicationSubmissions[1].rollback_of, publicationSubmissions[0].id)
  await checkTokenBudgetBrowser({ page, origin, output, label, fixture: budgetFixture })
  await checkUserUsagePolicyBrowser({ page, origin, output, label, fixture: userPolicyFixture })
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
