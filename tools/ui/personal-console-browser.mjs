// Run the compiled application in Chromium with explicitly synthetic, isolated APIs.
// This is browser evidence, never a production/provider/billing qualification.
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { createReadStream, existsSync, mkdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { createServer } from 'node:http'
import { extname, resolve, sep } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { createUIFixture, FIXTURE_TIME } from './browser-fixtures.mjs'
import { assertTextContrast } from './contrast.mjs'

const repo = fileURLToPath(new URL('../../', import.meta.url))
const root = resolve(repo, 'web/dist')
const output = resolve(process.env.MYAPI_BROWSER_ARTIFACTS || '.local-tests/personal-console')
const version = readFileSync(resolve(repo, 'VERSION'), 'utf8').trim()
const head = execFileSync('git', ['rev-parse', 'HEAD'], { cwd: repo, encoding: 'utf8' }).trim()
const codes = { en: 'en', zh: 'zhCN', 'zh-TW': 'zhTW', fr: 'fr', ja: 'ja', ru: 'ru', vi: 'vi' }
const translations = Object.fromEntries(Object.keys(codes).map(lang => [lang, JSON.parse(readFileSync(resolve(repo, `web/src/i18n/locales/${lang}.json`))).translation]))
const report = { head, version, workflowHead: process.env.GITHUB_SHA || null, scope: 'Profile and call-log browser journeys; overview/upstream and Keys are covered by personal-workbench-browser and subscription-share-browser. Compiled personal console with synthetic APIs; excludes live providers, production and historical accounting.', result: 'running', journeys: [], screenshots: [], errors: [] }
const persist = () => writeFileSync(resolve(output, 'personal-console-qualification.json'), JSON.stringify(report, null, 2))
const mime = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.png': 'image/png', '.woff2': 'font/woff2', '.svg': 'image/svg+xml', '.ico': 'image/x-icon' }
const server = createServer((request, response) => {
  let file = resolve(root, `.${new URL(request.url, 'http://localhost').pathname}`)
  if (!file.startsWith(root + sep) && file !== root) return response.writeHead(403).end()
  if (!existsSync(file) || statSync(file).isDirectory()) file = resolve(root, 'index.html')
  response.writeHead(200, { 'content-type': mime[extname(file)] || 'application/octet-stream', 'cache-control': 'no-store' })
  createReadStream(file).pipe(response)
})
assert(existsSync(resolve(root, 'index.html')), 'Build frontend first')
mkdirSync(output, { recursive: true })
let browser
try {
  await new Promise(done => server.listen(0, '127.0.0.1', done))
  const origin = `http://127.0.0.1:${server.address().port}`
  const { chromium } = await import(process.env.MYAPI_PLAYWRIGHT_MODULE ? pathToFileURL(process.env.MYAPI_PLAYWRIGHT_MODULE).href : 'playwright')
  browser = await chromium.launch({ headless: true })
  const cases = Object.keys(codes).flatMap(language => [320, 1280].map(width => ({ language, width })))
  cases.push({ language: 'zh', width: 390 }, { language: 'zh', width: 1440 })
  for (const item of cases) {
    const t = key => {
      assert(translations[item.language][key], `Missing ${item.language} key ${key}`)
      return translations[item.language][key]
    }
    const context = await browser.newContext({ viewport: { width: item.width, height: 960 }, reducedMotion: 'reduce', colorScheme: 'light', serviceWorkers: 'block' })
    const fixture = createUIFixture({ role: 100, language: codes[item.language], longText: item.language === 'fr' })
    const errors = []
    let failProfile = false
    await context.addInitScript(language => localStorage.setItem('i18nextLng', language), codes[item.language])
    await context.route('**/*', async route => {
      const request = route.request()
      const url = new URL(request.url())
      if (url.origin !== origin) return route.abort('blockedbyclient')
      if (!url.pathname.startsWith('/api/') && !url.pathname.startsWith('/pg/')) return route.continue()
      assert(request.method() === 'GET' || url.pathname === '/api/user/auth/refresh', 'No business mutation or Key reveal')
      if (url.pathname === '/api/user/self' && failProfile) return route.fulfill({ status: 503, json: { success: false } })
      const response = fixture.response(url, request.method())
      assert(!response.violation, response.violation)
      if (url.pathname === '/api/user/auth/refresh') response.body.data.access_expires_at = FIXTURE_TIME / 1000 + 3600
      if (url.pathname === '/api/log/self') {
        const log = (id, outcome, usage) => ({ id, user_id: 1, token_id: 21, created_at: FIXTURE_TIME / 1000 - id, type: 5, content: outcome === 'failed' ? 'Synthetic upstream 429' : '', username: 'synthetic-operator', token_name: 'synthetic-key', model_name: 'synthetic-text-model', quota: 0, prompt_tokens: 0, completion_tokens: 0, use_time: 2, is_stream: true, request_id: `REQ-SYNTHETIC-${id}`, group: 'default', ip: '', other: JSON.stringify({ request_outcome: outcome, usage_outcome: usage, settlement_status: 'pending_review' }) })
        response.body = { success: true, data: { items: [log(501, 'completed', 'missing'), log(502, 'failed', 'missing'), log(503, 'completed', 'reported')], total: 3, page: 1, page_size: 20 } }
      }
      await route.fulfill({ status: response.status, json: response.body })
    })
    const page = await context.newPage()
    await page.clock.setFixedTime(FIXTURE_TIME)
    page.setDefaultTimeout(20000)
    page.on('pageerror', error => errors.push(error.message))
    const capture = async name => {
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, 'No horizontal viewport overflow')
      const visibleNavigation = page.getByRole('navigation', { name: t(item.width < 768 ? 'Mobile console navigation' : 'Personal console navigation') })
      assert(await visibleNavigation.isVisible(), 'Navigation remains visible')
      const bounds = await visibleNavigation.boundingBox()
      assert(bounds && bounds.x >= 0 && bounds.x + bounds.width <= item.width, 'Navigation stays in viewport')
      const selectedNav = visibleNavigation.locator('a[aria-current="page"] span')
      if (await selectedNav.count()) await assertTextContrast(selectedNav, 'selected personal navigation', 'light')
      const cards = await page.locator('[data-slot="card"]').evaluateAll(elements => elements.filter(element => element.getBoundingClientRect().width > 0).map(element => {
        const bounds = element.getBoundingClientRect()
        return { left: bounds.left, right: bounds.right, width: element.clientWidth, scroll: element.scrollWidth }
      }))
      assert(cards.every(card => card.left >= 0 && card.right <= item.width && card.scroll <= card.width + 1), 'Visible cards do not clip content horizontally')
      const file = `console-${name}-${item.language}-${item.width}.png`
      await page.screenshot({ path: resolve(output, file), animations: 'disabled' })
      report.screenshots.push({ file, language: item.language, width: item.width, state: name })
      persist()
    }
    try {
      await page.goto(`${origin}/profile`, { waitUntil: 'networkidle' })
      await page.getByRole('heading', { name: t('Identity'), exact: true }).waitFor()
      await page.getByText(t('Only me'), { exact: true }).waitFor()
      assert.equal(await page.getByText(/Current Balance|Wallet Balance/, { exact: true }).count(), 0)
      const revision = await page.locator('html').getAttribute('data-build-rev')
      assert(revision?.startsWith(`rv.${version}.${process.env.GITHUB_SHA || head}.`))
      await capture('profile')
      const security = page.getByRole('button', { name: new RegExp(t('Manage login methods')) })
      await security.focus()
      await page.keyboard.press('Enter')
      assert.equal(await security.getAttribute('aria-expanded'), 'true')
      await page.keyboard.press('Enter')
      assert.equal(await security.getAttribute('aria-expanded'), 'false')
      const nav = page.getByRole('navigation', { name: t(item.width < 768 ? 'Mobile console navigation' : 'Personal console navigation') })
      await nav.getByRole('link', { name: t('Call records'), exact: true }).click()
      await page.getByRole('heading', { name: t('Call logs'), level: 1 }).waitFor()
      const row = page.getByRole('button', { name: t('View call {{id}}').replace('{{id}}', 'REQ-SYNTHETIC-501'), exact: true })
      await row.click()
      await page.getByRole('heading', { name: t('Request succeeded'), exact: true }).waitFor()
      await page.getByRole('heading', { name: t('Usage not provided'), exact: true }).waitFor()
      for (const tokenLabel of ['Input Tokens', 'Output Tokens']) {
        const tokenRow = page.locator('dl > div').filter({ has: page.getByText(t(tokenLabel), { exact: true }) })
        assert.equal(await tokenRow.locator('dd').innerText(), t('Unknown'), `${tokenLabel} stays unknown`)
      }
      await capture('success-unknown')
      if (item.width < 768) {
        const usageCard = page.locator('[data-slot="card"]').filter({ has: page.getByRole('heading', { name: t('Usage not provided'), exact: true }) })
        await usageCard.getByRole('button', { name: t('View this Key'), exact: true }).scrollIntoViewIfNeeded()
        const contentBounds = await page.locator('.myapi-page-content').boundingBox()
        const navigationBounds = await nav.boundingBox()
        assert(contentBounds && navigationBounds)
        for (const label of ['Input Tokens', 'Output Tokens', 'View this Key']) {
          const element = usageCard.getByText(t(label), { exact: true })
          const bounds = await element.boundingBox()
          assert(bounds && bounds.y >= contentBounds.y && bounds.y + bounds.height <= navigationBounds.y, `${label} is visible above the bottom navigation`)
        }
        await capture('success-unknown-usage')
        await page.getByRole('button', { name: t('Back to call list'), exact: true }).click()
        await page.waitForFunction(() => document.activeElement?.getAttribute('aria-pressed') === 'true')
        assert(await row.evaluate(element => document.activeElement === element), 'Return restores selected call focus')
      }
      await page.getByRole('button', { name: t('View call {{id}}').replace('{{id}}', 'REQ-SYNTHETIC-502'), exact: true }).click()
      await page.getByRole('heading', { name: t('Request failed'), exact: true }).waitFor()
      await capture('request-failed')
      if (item.language === 'zh') {
        failProfile = true
        await page.goto(`${origin}/profile`, { waitUntil: 'networkidle' })
        await page.getByRole('alert').filter({ hasText: t('Profile is temporarily unavailable') }).waitFor()
        await capture('profile-error')
        failProfile = false
        await page.getByRole('button', { name: t('Reload profile'), exact: true }).click()
        await page.getByRole('heading', { name: t('Identity'), exact: true }).waitFor()
      }
      assert.deepEqual(errors, [], 'No runtime page errors')
      report.journeys.push({ ...item, revision, result: 'passed' })
    } finally { await context.close() }
  }
  report.result = 'passed'
} catch (error) { report.result = 'failed'; report.errors.push(error.stack || String(error)); throw error }
finally { persist(); if (browser) await browser.close(); await new Promise(done => server.close(done)); console.log(JSON.stringify(report)) }
