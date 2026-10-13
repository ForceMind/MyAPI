// Production frontend with isolated synthetic APIs; never connects to production.
import assert from 'node:assert/strict'
import { createServer } from 'node:http'
import { createReadStream, existsSync, mkdirSync, writeFileSync } from 'node:fs'
import { extname, resolve, sep } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { createUIFixture } from './browser-fixtures.mjs'

const root = resolve(fileURLToPath(new URL('../../web/dist/', import.meta.url)))
const output = resolve(process.env.MYAPI_BROWSER_ARTIFACTS || '.local-tests/profile-browser')
const { chromium } = await import(process.env.MYAPI_PLAYWRIGHT_MODULE ? pathToFileURL(process.env.MYAPI_PLAYWRIGHT_MODULE).href : 'playwright')
const mime = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.png': 'image/png', '.woff2': 'font/woff2', '.svg': 'image/svg+xml' }
assert(existsSync(resolve(root, 'index.html')), 'Build frontend first')
mkdirSync(output, { recursive: true })
const server = createServer((request, response) => {
  let path = resolve(root, `.${new URL(request.url, 'http://localhost').pathname}`)
  if (!path.startsWith(root + sep) && path !== root) return response.writeHead(403).end()
  if (!existsSync(path) || !extname(path)) path = resolve(root, 'index.html')
  response.writeHead(200, { 'content-type': mime[extname(path)] || 'application/octet-stream' })
  createReadStream(path).pipe(response)
})
const report = { evidence: 'Synthetic profile navigation and failure recovery only; no production acceptance', passed: [], errors: [] }
let browser
try {
  await new Promise(done => server.listen(0, '127.0.0.1', done))
  const origin = `http://127.0.0.1:${server.address().port}`
  browser = await chromium.launch({ headless: true })
  for (const scenario of (process.env.MYAPI_PROFILE_SCENARIO ? [process.env.MYAPI_PROFILE_SCENARIO] : ['healthy', 'null-settings', 'failure-retry', 'stalled-request', 'stalled-auth-refresh', 'stalled-bootstrap'])) {
    for (const width of (scenario.startsWith('stalled-') ? [1280] : [1280, 320])) {
      const context = await browser.newContext({ viewport: { width, height: 900 }, reducedMotion: 'reduce', serviceWorkers: 'block' })
      const fixture = createUIFixture({ role: 1, language: 'en' })
      let profileReads = 0
      let refreshHeld = false
      let refreshReads = 0
      let refreshRecovered = false
      let releaseStall
      const stalled = new Promise(done => { releaseStall = done })
      const errors = []
      try {
        await context.addInitScript(() => localStorage.setItem('i18nextLng', 'en'))
        await context.route('**/*', async route => {
          const request = route.request()
          const url = new URL(request.url())
          if (url.origin !== origin) return route.abort('blockedbyclient')
          if (!url.pathname.startsWith('/api/') && !url.pathname.startsWith('/pg/')) return route.continue()
          assert(request.method() === 'GET' || url.pathname === '/api/user/auth/refresh', 'No business mutations')
          assert(!url.pathname.startsWith('/api/user/checkin'), 'Personal profile never mounts the credit-awarding check-in feature')
          const response = fixture.response(url, request.method())
          assert(!response.violation, response.violation)
          if (url.pathname === '/api/status') response.body.data.checkin_enabled = true
          if (url.pathname === '/api/user/auth/refresh') {
            refreshReads++
            // Only the first failing refresh is held. Keep it held through
            // Retry so recovery must start a new refresh generation.
            if (!refreshHeld && (scenario === 'stalled-bootstrap' ||
              (profileReads > 0 && scenario === 'stalled-auth-refresh'))) {
              refreshHeld = true
              await stalled
            } else if (refreshHeld) {
              refreshRecovered = true
            }
            response.body.data.access_expires_at = Math.floor(Date.now() / 1000) + 3600
          }
          if (url.pathname === '/api/user/self') {
            profileReads++
            if (scenario === 'stalled-request' && profileReads === 1) await stalled
            if (scenario === 'stalled-auth-refresh' && !refreshRecovered) return route.fulfill({ status: 401, json: { success: false } })
            if (scenario === 'failure-retry' && profileReads === 1) return route.fulfill({ status: 503, json: { success: false } })
            if (scenario === 'null-settings') response.body.data.setting = 'null'
          }
          try { await route.fulfill({ status: response.status, json: response.body }) } catch (error) {
            if (!scenario.startsWith('stalled-')) throw error
          }
        })
        const page = await context.newPage()
        page.setDefaultTimeout(30000)
        page.on('pageerror', error => errors.push(error.message))
        if (scenario === 'stalled-bootstrap') {
          await page.goto(`${origin}/profile`, { waitUntil: 'domcontentloaded' })
          await page.waitForURL(`${origin}/profile`)
        } else {
          await page.goto(`${origin}/keys`, { waitUntil: 'networkidle' })
          if (width === 320) await page.getByRole('button', { name: 'Toggle sidebar', exact: true }).click()
          await page.locator('a[href="/profile"]').first().click()
          await page.waitForURL(`${origin}/profile`)
        }
        if (scenario === 'failure-retry' || ['stalled-request', 'stalled-auth-refresh'].includes(scenario)) {
          const alert = page.getByRole('alert').filter({ hasText: 'Failed to load profile' })
          await alert.waitFor()
          if (scenario === 'stalled-auth-refresh') assert(refreshHeld, 'Hold the auth refresh triggered by the profile read')
          await page.screenshot({ path: resolve(output, `${scenario}-${width}-error.png`) })
          if (scenario !== 'stalled-auth-refresh') releaseStall()
          await alert.getByRole('button', { name: 'Retry', exact: true }).click()
        }
        await page.getByRole('heading', { name: 'Synthetic UI operator', level: 2 }).waitFor()
        await page.getByRole('tab', { name: width === 320 ? 'Settings' : 'Settings & Preferences', exact: true }).click()
        await page.getByRole('button', { name: 'Webhook', exact: true }).click()
        await page.getByRole('textbox', { name: 'Webhook URL', exact: true }).fill('https://synthetic.invalid/hook')
        assert.equal(await page.getByRole('textbox', { name: 'Webhook URL', exact: true }).inputValue(), 'https://synthetic.invalid/hook')
        assert.equal(await page.getByText('Current Balance', { exact: true }).count(), 0, 'Personal profile hides wallet balance')
        assert.equal(await page.getByText('Total Usage', { exact: true }).count(), 0, 'Personal profile hides price-converted usage')
        await page.getByText('API Requests', { exact: true }).waitFor()
        assert.equal(await page.getByText('Daily Check-in', { exact: true }).count(), 0, 'Personal profile hides check-in even when the separate check-in setting is enabled')
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, 'No page overflow')
        assert.equal(profileReads, scenario === 'stalled-auth-refresh' ? 3 : ['healthy', 'null-settings', 'stalled-bootstrap'].includes(scenario) ? 1 : 2, 'Profile and sidebar share one self read per attempt, including the authenticated replay')
        if (['stalled-auth-refresh', 'stalled-bootstrap'].includes(scenario)) {
          assert(refreshRecovered, 'Recover with a new refresh while the original request remains held')
        }
        assert.deepEqual(errors, [], 'No page runtime errors')
        await page.screenshot({ path: resolve(output, `${scenario}-${width}.png`) })
        report.passed.push({ scenario, width, profileReads, refreshReads, refreshRecovered })
      } finally { releaseStall(); await context.close() }
    }
  }
} catch (error) { report.errors.push(error.message); throw error }
finally {
  writeFileSync(resolve(output, 'result.json'), JSON.stringify(report, null, 2))
  if (browser) await browser.close()
  await new Promise(done => server.close(done))
}
console.log(JSON.stringify(report))
