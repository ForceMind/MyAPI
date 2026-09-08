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
const output = resolve(process.env.MYAPI_QUOTA_COMPARISON_BROWSER_ARTIFACTS || '/tmp/myapi-quota-comparison-browser')
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
const errors = []
const unexpected = new Set()
const now = Math.floor(Date.now() / 1000)
const historyRequests = []
const recorded = process.env.MYAPI_RECORDED_QUOTA_FIXTURE ? JSON.parse(readFileSync(process.env.MYAPI_RECORDED_QUOTA_FIXTURE, 'utf8')) : null
const series = [1, 2].map(channel_id => ({ channel_id, name: `对比渠道 ${channel_id}`, unit: 'percent', window_type: 'weekly', window_seconds: 604800, metric_type: 'codex_rate_limit', source: 'synthetic', status: 'success' }))
try {
  await new Promise(done => server.listen(0, '127.0.0.1', done))
  const origin = `http://127.0.0.1:${server.address().port}`
  browser = await chromium.launch({ headless: true, executablePath: process.env.MYAPI_CHROMIUM_PATH || undefined, args: ['--disable-dev-shm-usage'] })
  const context = await browser.newContext({ viewport: { width: 1280, height: 720 }, locale: 'zh-CN', reducedMotion: 'reduce' })
  await context.addInitScript(() => { localStorage.setItem('i18nextLng', 'zhCN'); localStorage.setItem('theme', 'light') })
  const fixtures = quotaFixtures()
  await context.route('**/api/**', async route => {
    const url = new URL(route.request().url())
    let response
    if (url.pathname === '/api/channel/quota/changes' && recorded) response = recorded.catalogue
    else if (url.pathname.includes('/quota/history') && recorded) {
      const key = [url.pathname.split('/')[3], url.searchParams.get('window_type'), url.searchParams.get('source'), url.searchParams.get('plan_type')].join('|')
      response = recorded.histories[key]
    }
    else if (url.pathname === '/api/channel/quota/changes') response = { success: true, data: { items: series, source_complete: true, items_complete: true } }
    else if (/\/api\/channel\/[12]\/quota\/history/.test(url.pathname)) {
      const request = Object.fromEntries(url.searchParams)
      historyRequests.push(request)
      assert.equal(request.consumption_basis, 'available')
      assert.equal(request.exact_identity, 'true')
      const id = Number(url.pathname.split('/')[3])
      const start = Date.parse(request.start) / 1000; const end = Date.parse(request.end) / 1000
      const step = (end - start) / 24
      const points = Array.from({ length: 24 }, (_, i) => {
        const time = Math.floor(start + step * (i + (id === 2 ? 0.4 : 0)))
        const reset = id === 1 && i === 12
        const failure = id === 2 && i === 8
        return { timestamp: time, observed_at: time, status: failure ? 'error' : 'success', available: failure ? undefined : (id === 1 ? (i < 12 ? 80 - i * 1.1234567 : 95 - (i - 12) * 0.8) : 60 - i * 0.72), consumption: i && !reset && !failure && !(id === 2 && i === 9) ? (id === 1 ? 1.1234567 : 0.72) : undefined, continuity_break: reset || failure || (id === 2 && i === 9), reset, reset_at: now + 86400 }
      })
      response = { success: true, data: { ...series[id - 1], start, end, limit: 5000, complete: true, granularity: request.granularity, points, current: { status: 'success', observed_at: end, reset_at: now + 86400, available: points.at(-1).available } } }
    } else response = fixtures.response(url)
    if (!response) unexpected.add(url.pathname)
    await route.fulfill({ status: response ? 200 : 501, json: response || { success: false } })
  })
  await context.route('**/*', route => new URL(route.request().url()).origin === origin ? route.fallback() : route.abort())
  const page = await context.newPage()
  page.setDefaultTimeout(15000)
  page.on('pageerror', error => errors.push(error.message))
  mkdirSync(output, { recursive: true })
  await page.goto(`${origin}/channels`, { waitUntil: 'networkidle' })
  assert.equal(historyRequests.length, 0, 'channel management does not mount quota histories')
  await page.getByRole('tab', { name: label('Quota analysis'), exact: true }).click()
  const panel = page.getByRole('tabpanel', { name: label('Quota analysis') })
  const chart = panel.getByTestId('quota-comparison-chart')
  await chart.locator('.recharts-line-curve').nth(1).waitFor()
  assert.equal(await panel.getByRole('checkbox', { checked: true }).count(), 2, 'two channels are drawn together')
  if (recorded) {
    await page.screenshot({path: resolve(output, 'recorded-quota-desktop.png')})
    if (process.env.MYAPI_ASSERT_QUOTA_VISIBLE) {
      const b = await chart.boundingBox()
      assert(b.y >= 100 && b.y + b.height <= 680, 'default quota chart and time axis must fit the desktop viewport without scrolling')
    }
    console.log(JSON.stringify({recorded: true, curves: await chart.locator('.recharts-line-curve').count(), chartBounds: await chart.boundingBox()}))
    for (const viewport of [{width:390,height:844},{width:320,height:740}]) {
      await page.setViewportSize(viewport)
      await page.screenshot({path: resolve(output, `recorded-quota-mobile-${viewport.width}.png`)})
      if (process.env.MYAPI_ASSERT_QUOTA_VISIBLE) {
        const b = await chart.boundingBox()
        assert(b.y >= 100 && b.y + b.height <= viewport.height - 25, 'default quota chart must fit the mobile viewport without scrolling')
        assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), 'recorded quota must not overflow mobile width')
      }
    }
  } else {
  await chart.scrollIntoViewIfNeeded()
  await page.screenshot({ path: resolve(output, 'comparison-remaining-line.png'), fullPage: true })
  for (const [style, selector] of [['bar', '.recharts-bar-rectangle'], ['scatter', '.recharts-scatter-symbol'], ['area', '.recharts-area-area'], ['line', '.recharts-line-curve']]) {
    await panel.getByLabel(label('Chart type'), { exact: true }).selectOption(style)
    await chart.locator(selector).first().waitFor({ state: 'visible' })
    assert(await chart.locator(selector).count() >= 2, `${style} renders both series`)
    await page.screenshot({ path: resolve(output, `comparison-remaining-${style}.png`), fullPage: true })
  }
  await panel.getByLabel(label('Metric'), { exact: true }).selectOption('consumption')
  await panel.getByLabel(label('Chart type'), { exact: true }).selectOption('bar')
  await chart.locator('.recharts-bar-rectangle').first().waitFor()
  await page.screenshot({ path: resolve(output, 'comparison-usage-bar.png'), fullPage: true })
  const before = historyRequests.at(-1)
  const box = await chart.boundingBox()
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
  await Promise.all([page.waitForResponse(response => response.url().includes('/quota/history') && new URL(response.url()).searchParams.get('start') !== before.start), page.mouse.wheel(0, -240)])
  const after = historyRequests.at(-1)
  assert(Date.parse(after.end) - Date.parse(after.start) < Date.parse(before.end) - Date.parse(before.start), 'wheel zoom reloads a narrower history')
  await chart.locator('.recharts-bar-rectangle').first().waitFor()
  await Promise.all([page.waitForResponse(response => response.url().includes('/quota/history')), panel.getByLabel(label('Time range'), { exact: true }).selectOption('cycle')])
  assert.equal(Date.parse(historyRequests.at(-1).start) / 1000, now + 86400 - 604800, 'cycle is anchored to provider reset, not calendar Monday')
  await page.getByRole('tab', { name: label('Channel management'), exact: true }).click()
  await chart.waitFor({ state: 'hidden' })
  await page.getByRole('tab', { name: label('Quota analysis'), exact: true }).click()
  for (const viewport of [{ width: 390, height: 844 }, { width: 320, height: 740 }]) {
    await page.setViewportSize(viewport)
    await chart.scrollIntoViewIfNeeded()
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), 'no horizontal page overflow')
    await chart.locator('svg.recharts-surface').waitFor()
    await page.screenshot({ path: resolve(output, `comparison-mobile-${viewport.width}.png`), fullPage: true })
  }
  assert.deepEqual([...unexpected], [])
  assert.deepEqual(errors, [])
  console.log(JSON.stringify({ result: 'pass', historyQueries: historyRequests.length, artifacts: output }))
  }
} finally {
  await browser?.close()
  await new Promise(done => server.close(done))
}
