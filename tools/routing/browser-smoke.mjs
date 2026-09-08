// Render the real built routing UI with synthetic API fixtures only. This
// checks presentation and request contracts; backend routing has Go tests.
import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { createReadStream, existsSync, mkdirSync, readFileSync, statSync } from 'node:fs'
import { extname, resolve, sep } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { quotaFixtures } from '../quota/browser-fixtures.mjs'

const repo = fileURLToPath(new URL('../../', import.meta.url))
const root = resolve(repo, 'web/dist')
const output = resolve(process.env.MYAPI_ROUTING_BROWSER_ARTIFACTS || '/tmp/myapi-routing-browser/artifacts')
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
const writes = []
let policy = { enabled: false, sticky_enabled: true, session_ttl_seconds: 86400, quota_max_age_seconds: 600 }
const channels = [
  { id: 701, name: '路由测试 A', type: 57, status: 1, priority: 0, weight: 30, quota: { state: 'fresh', available: 25, unit: 'percent', observed_at: 1788850000, comparison_key: 'synthetic-codex-series' } },
  { id: 702, name: '路由测试 B', type: 57, status: 1, priority: 0, weight: 70, quota: { state: 'fresh', available: 80, unit: 'percent', observed_at: 1788850000, comparison_key: 'synthetic-codex-series' } },
  { id: 703, name: '额度未知的测试渠道', type: 1, status: 1, priority: -1000000, legacy_priority: '-9223372036854775808', weight: 10, quota: { state: 'unknown' } },
]
try {
  await new Promise((done) => server.listen(0, '127.0.0.1', done))
  const origin = `http://127.0.0.1:${server.address().port}`
  browser = await chromium.launch({ headless: true, executablePath: process.env.MYAPI_CHROMIUM_PATH || undefined, args: ['--disable-dev-shm-usage'] })
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, locale: 'zh-CN', reducedMotion: 'reduce' })
  await context.addInitScript(() => { localStorage.setItem('i18nextLng', 'zhCN'); localStorage.setItem('theme', 'light') })
  const fixtures = quotaFixtures()
  await context.route('**/api/**', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/\/$/, '')
    const method = request.method()
    let response
    if (path === '/api/channel/routing' && method === 'GET') response = { success: true, data: { policy, channels } }
    else if (path === '/api/channel/routing' && method === 'PUT') {
      const body = request.postDataJSON()
      writes.push({ path, body })
      assert.equal(typeof body.enabled, 'boolean')
      assert.equal(typeof body.sticky_enabled, 'boolean')
      policy = body
      response = { success: true }
    } else if (path === '/api/channel/routing/channels/701' && method === 'PUT') {
      const body = request.postDataJSON()
      writes.push({ path, body })
      assert.equal(body.expected_priority, channels[0].priority)
      assert.equal(body.expected_weight, channels[0].weight)
      channels[0].priority = body.priority
      channels[0].weight = body.weight
      response = { success: true }
    } else if (path === '/api/channel/routing/preview' && method === 'POST') {
      const body = request.postDataJSON()
      writes.push({ path, body })
      assert.equal(body.group, 'default')
      assert.equal(body.model, 'gpt-5')
      const work = body.path === '/v1/responses'
      const channel = channels[work ? 1 : 0]
      response = { success: true, data: { workload: work ? 'work' : 'chat', reason: work ? 'work_higher_remaining_quota' : 'chat_lower_remaining_quota', candidates: [{ id: channel.id, name: channel.name, share: 1, reason: work ? 'quota_higher_pool' : 'quota_lower_pool', available: channel.quota.available, unit: 'percent' }] } }
    } else response = fixtures.response(url)
    if (!response) unexpected.add(`${method} ${path}`)
    await route.fulfill({ status: response ? 200 : 501, json: response || { success: false, message: 'Unconfigured routing browser fixture' } })
  })
  await context.route('**/*', (route) => {
    const url = new URL(route.request().url())
    if (url.origin === origin || url.protocol === 'data:') return route.fallback()
    return route.abort()
  })
  const page = await context.newPage()
  page.setDefaultTimeout(15000)
  page.on('pageerror', (error) => errors.push(error.message))
  mkdirSync(output, { recursive: true })
  await page.goto(`${origin}/channels`, { waitUntil: 'networkidle' })
  await page.getByRole('tab', { name: label('Traffic Allocation'), exact: true }).click()
  const dialog = page.getByRole('tabpanel', { name: label('Traffic Allocation') })
  await dialog.getByText('路由测试 A', { exact: true }).waitFor()
  await dialog.getByRole('switch', { name: label('Enable intelligent allocation') }).click()
  await Promise.all([
    page.waitForResponse((response) => response.url().endsWith('/api/channel/routing') && response.request().method() === 'GET'),
    dialog.getByRole('button', { name: label('Save policy'), exact: true }).click(),
  ])
  assert.equal(policy.enabled, true, 'enabling policy writes one complete object')
  const row = dialog.getByRole('row').filter({ hasText: '路由测试 A' })
  await row.getByRole('spinbutton', { name: `${label('Traffic share')} 路由测试 A` }).fill('55')
  await Promise.all([
    page.waitForResponse((response) => response.url().endsWith('/api/channel/routing') && response.request().method() === 'GET'),
    row.getByRole('button', { name: label('Save'), exact: true }).click(),
  ])
  assert.equal(channels[0].weight, 55, 'channel save sends narrow CAS fields')
  await dialog.getByLabel(label('Model'), { exact: true }).fill('gpt-5')
  await dialog.getByLabel(label('Group'), { exact: true }).fill('default')
  await dialog.getByRole('button', { name: label('Preview routing'), exact: true }).click()
  await dialog.getByText('100%', { exact: false }).first().waitFor()
  await page.screenshot({ path: resolve(output, 'routing-desktop-preview.png'), fullPage: true })
  await dialog.evaluate((node) => { node.scrollTop = 0 })
  await page.screenshot({ path: resolve(output, 'routing-desktop.png'), fullPage: true })
  await dialog.getByLabel(label('Request type'), { exact: true }).selectOption('/v1/responses')
  await Promise.all([
    page.waitForResponse((response) => response.url().endsWith('/api/channel/routing/preview')),
    dialog.getByRole('button', { name: label('Preview routing'), exact: true }).click(),
  ])
  assert.equal(writes.at(-1).body.path, '/v1/responses', 'work preview is classified by API path')
  await page.setViewportSize({ width: 390, height: 844 })
  const mobileWeight = row.getByRole('spinbutton', { name: `${label('Traffic share')} 路由测试 A` })
  await mobileWeight.scrollIntoViewIfNeeded()
  const inputBounds = await mobileWeight.boundingBox()
  assert(inputBounds && inputBounds.width >= 100 && inputBounds.x + inputBounds.width <= 390, 'mobile weight field is wide enough to read the full configured value')
  await mobileWeight.fill('1000000')
  await Promise.all([
    page.waitForResponse((response) => response.url().endsWith('/api/channel/routing') && response.request().method() === 'GET'),
    row.getByRole('button', { name: label('Save'), exact: true }).click(),
  ])
  assert.equal(channels[0].weight, 1000000, 'mobile maximum weight stays readable and saves the correct value')
  await page.screenshot({ path: resolve(output, 'routing-mobile-channel.png'), fullPage: true })
  const legacyNote = dialog.getByText(label('Existing order: {{priority}}. Choose a routing order to replace it.').replace('{{priority}}', channels[2].legacy_priority), { exact: true })
  await legacyNote.scrollIntoViewIfNeeded()
  assert((await legacyNote.textContent()).includes('-9223372036854775808'), 'legacy priority stays exact without JavaScript numeric rounding')
  await page.screenshot({ path: resolve(output, 'routing-mobile-legacy.png'), fullPage: true })
  await dialog.getByRole('button', { name: label('Preview routing'), exact: true }).scrollIntoViewIfNeeded()
  assert(await dialog.evaluate((node) => { const box = node.getBoundingClientRect(); return box.left >= 0 && box.right <= innerWidth + 1 }), 'mobile routing panel has no horizontal overflow')
  await page.screenshot({ path: resolve(output, 'routing-mobile-preview.png'), fullPage: true })
  await dialog.evaluate((node) => { node.scrollTop = 0 })
  await page.screenshot({ path: resolve(output, 'routing-mobile.png'), fullPage: true })
  await page.getByRole('tab', { name: label('Channel management'), exact: true }).click()
  await dialog.waitFor({ state: 'hidden' })
  assert(await page.getByRole('tab', { name: label('Channel management'), exact: true }).getAttribute('aria-selected') === 'true', 'channel management is selected independently of routing')
  assert.deepEqual([...unexpected], [], 'all browser API traffic uses explicit fixtures')
  assert.deepEqual(errors, [], 'no browser runtime errors')
  console.log(JSON.stringify({ result: 'pass', writes: writes.length, viewports: ['1280x900', '390x844'], artifacts: output }))
} finally {
  await browser?.close()
  await new Promise((done) => server.close(done))
}
