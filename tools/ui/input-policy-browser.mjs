// Render the local estimate policy editor against read-only synthetic APIs.
import assert from 'node:assert/strict'
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'
import { createKeyRequestPreview } from './key-request-preview.mjs'

const output = resolve(process.env.MYAPI_BROWSER_ARTIFACTS || '.local-tests/input-policy')
mkdirSync(output, { recursive: true })
const { server, fixture } = createKeyRequestPreview()
const { chromium } = await import(process.env.MYAPI_PLAYWRIGHT_MODULE ? pathToFileURL(process.env.MYAPI_PLAYWRIGHT_MODULE).href : 'playwright')
const translations = JSON.parse(readFileSync('web/src/i18n/locales/zh.json')).translation
const t = key => translations[key] || key
const report = { synthetic: true, providerRequests: 0, journeys: [] }
let browser
try {
  await new Promise(done => server.listen(0, '127.0.0.1', done))
  const origin = `http://127.0.0.1:${server.address().port}`
  browser = await chromium.launch({ headless: true })
  for (const width of [1280, 390]) {
    const context = await browser.newContext({ viewport: { width, height: 900 }, serviceWorkers: 'block' })
    await context.addInitScript(() => localStorage.setItem('i18nextLng', 'zh'))
    const errors = [], blocked = []
    await context.route('**/*', route => {
      const request = route.request(), url = new URL(request.url())
      if (url.origin !== origin || request.method() !== 'GET' && url.pathname !== '/api/user/auth/refresh') {
        blocked.push(request.method() + ' ' + url.pathname)
        return route.abort()
      }
      return route.continue()
    })
    const page = await context.newPage()
    page.on('pageerror', error => errors.push(error.message))
    await page.goto(origin + '/connections?connection=1', { waitUntil: 'networkidle' })
    await page.getByRole('button', { name: t('Edit connection'), exact: true }).click()
    const editor = page.getByRole('dialog').filter({ hasText: t('Edit Channel') })
    const input = editor.getByLabel(t('Estimated input limits'), { exact: true })
    await input.waitFor()
    await input.scrollIntoViewIfNeeded()
    assert.deepEqual(JSON.parse(await input.inputValue()), { 'synthetic-text-model': 920000 })
    await input.fill('{"synthetic-text-model": 910000}')
    assert.equal(JSON.parse(await input.inputValue())['synthetic-text-model'], 910000)
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
    const screenshot = `input-policy-zh-${width}.png`
    await page.screenshot({ path: resolve(output, screenshot), animations: 'disabled' })
    await page.keyboard.press('Escape')
    await editor.waitFor({ state: 'hidden' })
    assert.equal(new URL(page.url()).searchParams.get('connection'), '1')
    assert.deepEqual(errors, [])
    assert.deepEqual(blocked, [])
    report.journeys.push({ width, screenshot, result: 'passed' })
    await context.close()
  }
  assert(fixture.requests.every(r => r.method === 'GET' || r.path === '/api/user/auth/refresh'))
  report.result = 'passed'
} catch (error) {
  report.result = 'failed'
  report.error = error.message
  throw error
} finally {
  writeFileSync(resolve(output, 'input-policy-qualification.json'), JSON.stringify(report, null, 2))
  if (browser) await browser.close()
  await new Promise(done => server.close(done))
}
