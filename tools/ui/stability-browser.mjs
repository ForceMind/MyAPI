// Production-build UI with isolated fixtures; no real account or production API.
import assert from 'node:assert/strict'
import { createReadStream, existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { createServer } from 'node:http'
import { extname, resolve, sep } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

import { createUIFixture } from './browser-fixtures.mjs'

const root = resolve(fileURLToPath(new URL('../../web/dist/', import.meta.url)))
const output = resolve(process.env.MYAPI_BROWSER_ARTIFACTS || '.local-tests/stability-browser')
const version = readFileSync(new URL('../../VERSION', import.meta.url), 'utf8').trim()
const { chromium } = await import(process.env.MYAPI_PLAYWRIGHT_MODULE
  ? pathToFileURL(process.env.MYAPI_PLAYWRIGHT_MODULE).href : 'playwright')
const mime = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.png': 'image/png', '.woff2': 'font/woff2', '.svg': 'image/svg+xml' }
assert(existsSync(resolve(root, 'index.html')), 'Build frontend first')
mkdirSync(output, { recursive: true })
const server = createServer((request, response) => {
  let file = resolve(root, `.${new URL(request.url, 'http://localhost').pathname}`)
  if (!file.startsWith(root + sep) && file !== root) return response.writeHead(403).end()
  if (!existsSync(file) || !extname(file)) file = resolve(root, 'index.html')
  response.writeHead(200, { 'content-type': mime[extname(file)] || 'application/octet-stream' })
  createReadStream(file).pipe(response)
})

const report = { evidence: 'Synthetic installation and security-state recovery only; no production or live-device qualification', passed: [], errors: [] }
let browser
try {
  await new Promise(done => server.listen(0, '127.0.0.1', done))
  const origin = `http://127.0.0.1:${server.address().port}`
  browser = await chromium.launch({ headless: true })
  const scenarios = ['setup-failure-retry', 'setup-stalled-retry', 'setup-before-auth', 'setup-old-browser-flag', 'security-failure-retry', 'security-stalled-retry']
  for (const scenario of scenarios) {
    const widths = scenario.includes('stalled') ? [1280] : [1280, 320]
    for (const width of widths) {
      const context = await browser.newContext({ viewport: { width, height: 900 }, reducedMotion: 'reduce', serviceWorkers: 'block' })
      const initialized = !['setup-before-auth', 'setup-old-browser-flag'].includes(scenario)
      const fixture = createUIFixture({ role: 1, language: 'en', setupComplete: initialized })
      const reads = new Map()
      const recoveredSecurityPaths = new Set()
      const errors = []
      let release
      const held = new Promise(done => { release = done })
      let finishStaleSecurityReads
      const staleSecurityReads = new Promise(done => { finishStaleSecurityReads = done })
      let pendingStaleSecurityReads = 0
      let authHeld = false
      try {
        await context.addInitScript((oldFlag) => {
          localStorage.setItem('i18nextLng', 'en')
          if (oldFlag) localStorage.setItem('setup_status_checked', 'true')
        }, scenario === 'setup-old-browser-flag')
        await context.route('**/*', async route => {
          const request = route.request()
          const url = new URL(request.url())
          if (url.origin !== origin) return route.abort('blockedbyclient')
          if (!url.pathname.startsWith('/api/') && !url.pathname.startsWith('/pg/')) return route.continue()
          assert(request.method() === 'GET' || url.pathname === '/api/user/auth/refresh', 'No business mutations')
          const response = fixture.response(url, request.method())
          assert(!response.violation, response.violation)
          const count = (reads.get(url.pathname) || 0) + 1
          reads.set(url.pathname, count)
          const securityStatus = ['/api/user/2fa/status', '/api/user/passkey'].includes(url.pathname)
          // The card and verification capabilities both read these endpoints.
          // Every read stays unavailable until the user retries that card.
          const recoveredSecurityRead = recoveredSecurityPaths.has(url.pathname)
          const staleSecurityRead = securityStatus && scenario === 'security-stalled-retry' && !recoveredSecurityRead
          if (staleSecurityRead) pendingStaleSecurityReads += 1
          if (url.pathname === '/api/user/auth/refresh') {
            response.body.data.access_expires_at = Math.floor(Date.now() / 1000) + 3600
            if (scenario === 'setup-before-auth') { authHeld = true; await held }
          }
          if (url.pathname === '/api/setup' && count === 1) {
            if (scenario === 'setup-failure-retry') return route.fulfill({ status: 503, json: { success: false } })
            if (scenario === 'setup-stalled-retry') await held
          }
          if (securityStatus && scenario.startsWith('security-')) {
            if (!recoveredSecurityRead) {
              if (scenario === 'security-failure-retry') return route.fulfill({ status: 503, json: { success: false } })
              await held
            }
            response.body = {
              success: true,
              data: url.pathname === '/api/user/2fa/status'
                ? { enabled: recoveredSecurityRead, locked: recoveredSecurityRead && width === 320, backup_codes_remaining: recoveredSecurityRead ? 8 : 0 }
                : { enabled: recoveredSecurityRead, last_used_at: null },
            }
          }
          try { await route.fulfill({ status: response.status, json: response.body }) }
          catch (error) { if (!scenario.includes('stalled') && scenario !== 'setup-before-auth') throw error }
          finally {
            if (staleSecurityRead) {
              pendingStaleSecurityReads -= 1
              if (pendingStaleSecurityReads === 0) finishStaleSecurityReads()
            }
          }
        })
        const page = await context.newPage()
        page.setDefaultTimeout(25000)
        page.on('pageerror', error => errors.push(error.message))
        await page.goto(`${origin}/profile`, { waitUntil: 'domcontentloaded' })
        if (!initialized) {
          // This must finish while the unrelated authentication request is held.
          await page.getByRole('heading', { name: 'Database check', exact: true }).waitFor({ timeout: 5000 })
          assert.equal(new URL(page.url()).pathname.replace(/\/$/, ''), '/setup')
          assert.equal(await page.getByRole('button', { name: 'Next', exact: true }).isEnabled(), true)
          if (scenario === 'setup-before-auth') assert(authHeld, 'Authentication remained unresolved during setup navigation')
        } else {
          if (scenario.startsWith('setup-')) {
            const alert = page.getByRole('alert').filter({ hasText: 'Failed to load setup status' })
            await alert.waitFor()
            assert.notEqual(await page.evaluate(() => localStorage.getItem('setup_status_checked')), 'true')
            await page.screenshot({ path: resolve(output, `stability-${scenario}-${width}-error.png`) })
            await alert.getByRole('button', { name: 'Retry', exact: true }).click()
          }
          await page.getByRole('heading', { name: 'Synthetic UI operator', level: 3 }).waitFor()
          if (scenario.startsWith('security-')) {
            await page.getByRole('button', { name: /Manage login methods/ }).click()
            const twoFA = page.getByRole('alert').filter({ hasText: 'Unable to load settings' })
            const passkey = page.getByRole('alert').filter({ hasText: 'Failed to load Passkey status' })
            await twoFA.waitFor()
            await passkey.waitFor()
            assert.equal(await page.getByRole('button', { name: 'Enable', exact: true }).count(), 0)
            assert.equal(await page.getByRole('button', { name: 'Enable Passkey', exact: true }).count(), 0)
            await page.screenshot({ path: resolve(output, `stability-${scenario}-${width}-error.png`) })
            // Profile is longer than a viewport. Capture the affected cards,
            // including the title and retry action, for visual acceptance.
            await page.locator('[data-slot="card"]').filter({ has: twoFA }).last().screenshot({ path: resolve(output, `stability-${scenario}-${width}-2fa-error.png`) })
            await page.locator('[data-slot="card"]').filter({ has: passkey }).last().screenshot({ path: resolve(output, `stability-${scenario}-${width}-passkey-error.png`) })
            recoveredSecurityPaths.add('/api/user/2fa/status')
            await twoFA.getByRole('button', { name: 'Retry', exact: true }).click()
            recoveredSecurityPaths.add('/api/user/passkey')
            await passkey.getByRole('button', { name: 'Retry', exact: true }).click()
            await page.getByRole('button', { name: 'Disable 2FA', exact: true }).waitFor()
            await page.getByRole('button', { name: 'Remove Passkey', exact: true }).waitFor()
            release()
            if (scenario === 'security-stalled-retry') await staleSecurityReads
            // Both recovered controls remain after obsolete intercepted reads
            // finish. Hook tests also deliver late responses to the consumer.
            await page.getByRole('button', { name: 'Disable 2FA', exact: true }).waitFor()
            await page.getByRole('button', { name: 'Remove Passkey', exact: true }).waitFor()
            assert.equal(await page.getByRole('button', { name: 'Enable', exact: true }).count(), 0)
            assert.equal(await page.getByRole('button', { name: 'Enable Passkey', exact: true }).count(), 0)
            const twoFACard = page.locator('[data-slot="card"]').filter({ has: page.getByRole('button', { name: 'Disable 2FA', exact: true }) }).last()
            const passkeyCard = page.locator('[data-slot="card"]').filter({ has: page.getByRole('button', { name: 'Remove Passkey', exact: true }) }).last()
            await twoFACard.screenshot({ path: resolve(output, `stability-${scenario}-${width}-2fa.png`) })
            await passkeyCard.screenshot({ path: resolve(output, `stability-${scenario}-${width}-passkey.png`) })
            // Security state must remain readable, including simultaneous
            // Enabled and Locked labels in a narrow profile card.
            for (const label of width === 320 ? ['Enabled', 'Locked'] : ['Enabled']) {
              assert.equal(await twoFACard.getByText(label, { exact: true }).evaluate(element => element.scrollWidth <= element.clientWidth), true, `2FA ${label} state is not truncated at ${width}px`)
            }
            assert.equal(await passkeyCard.getByText('Enabled', { exact: true }).evaluate(element => element.scrollWidth <= element.clientWidth), true, `Passkey Enabled state is not truncated at ${width}px`)
          }
        }
        const buildRevision = await page.locator('html').getAttribute('data-build-rev')
        // A root-level startup error can precede application metadata mounting.
        if (initialized) {
          assert(buildRevision?.startsWith(`rv.${version}.`), 'Visible build identity uses the repository version')
          if (process.env.GITHUB_SHA) assert(buildRevision.includes(process.env.GITHUB_SHA), 'Build identity matches the checked-out CI merge')
        }
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, 'No horizontal page overflow')
        assert.deepEqual(errors, [], 'No page runtime errors')
        await page.screenshot({ path: resolve(output, `stability-${scenario}-${width}.png`) })
        report.passed.push({ scenario, width, reads: Object.fromEntries(reads), authHeld, buildRevision })
      } finally { release(); await context.close() }
    }
  }
} catch (error) { report.errors.push(error.message); throw error }
finally {
  writeFileSync(resolve(output, 'stability-result.json'), JSON.stringify(report, null, 2))
  if (browser) await browser.close()
  await new Promise(done => server.close(done))
}
console.log(JSON.stringify(report))
