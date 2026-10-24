// Real browser layout regression against read-only synthetic application APIs.
import assert from 'node:assert/strict'
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'
import { createKeyRequestPreview } from './key-request-preview.mjs'

const output = resolve(process.env.MYAPI_BROWSER_ARTIFACTS || '.local-tests/console-layout')
mkdirSync(output, { recursive: true })
const { server, fixture } = createKeyRequestPreview()
const { chromium } = await import(process.env.MYAPI_PLAYWRIGHT_MODULE ? pathToFileURL(process.env.MYAPI_PLAYWRIGHT_MODULE).href : 'playwright')
let browser
const report = { synthetic: true, providerRequests: 0, journeys: [], result: 'running' }
try {
  await new Promise(done => server.listen(0, '127.0.0.1', done))
  const origin = `http://127.0.0.1:${server.address().port}`
  browser = await chromium.launch({ headless: true })
  for (const [language, width, height] of [['zh', 1280, 720], ['en', 1440, 900], ['ru', 1024, 640], ['zh', 390, 844], ['ru', 320, 740]]) {
    const t = key => JSON.parse(readFileSync(resolve(`web/src/i18n/locales/${language}.json`))).translation[key] || key
    const context = await browser.newContext({ viewport: { width, height }, reducedMotion: 'reduce', serviceWorkers: 'block' })
    await context.addInitScript(lang => localStorage.setItem('i18nextLng', lang), language)
    const violations = [], errors = []
    await context.route('**/*', route => {
      const request = route.request(), url = new URL(request.url())
      if (url.origin !== origin || request.method() !== 'GET' && url.pathname !== '/api/user/auth/refresh') {
        violations.push(request.method() + ' ' + url.pathname)
        return route.abort()
      }
      return route.continue()
    })
    const page = await context.newPage()
    page.setDefaultTimeout(15000)
    page.on('pageerror', error => errors.push(error.message))
    const journey = { language, width, height, screenshots: [], result: 'running' }
    report.journeys.push(journey)
    try {
      await page.goto(origin + '/keys', { waitUntil: 'networkidle' })
      // Every layout lists each Key as one control that opens its inspector.
      await page.locator('[data-inspect-id]').first().waitFor()
      if (width >= 1024) {
        // Without a saved preference the sidebar starts as an icon rail below 1280px.
        const startsAsRail = width < 1280
        assert.equal(await page.locator('.personal-console').getAttribute('data-collapsed'), String(startsAsRail), 'Default sidebar state follows the window width')
        if (startsAsRail) await page.getByRole('button', { name: t('Expand sidebar'), exact: true }).click()
        const nav = page.getByRole('navigation', { name: t('Personal console navigation') })
        const aligned = await page.evaluate(() => {
          const brand = document.querySelector('.pc-sidebar .pc-brand').getBoundingClientRect()
          const header = document.querySelector('.pc-header').getBoundingClientRect()
          return brand.top === header.top && brand.height === header.height
        })
        assert(aligned, 'Sidebar brand and workspace toolbar share one aligned header row')
        assert.equal(await nav.evaluate(e => getComputedStyle(e).scrollbarWidth), 'none', 'Sidebar has no persistent scrollbar rail')
        await nav.focus()
        assert(await nav.evaluate(e => e === document.activeElement), 'Overflowing navigation can receive keyboard focus')
        await page.keyboard.press('End')
        await page.waitForFunction(() => { const e = document.querySelector('.pc-desktop-nav'); return e.scrollHeight <= e.clientHeight || e.scrollTop > 0 })
        const last = nav.getByRole('link').last()
        await last.focus()
        assert(await last.evaluate(e => { const r = e.getBoundingClientRect(), p = e.closest('nav').getBoundingClientRect(); return r.top >= p.top && r.bottom <= p.bottom + 1 }), 'Last navigation destination stays reachable')
        await page.getByRole('button', { name: t('Collapse sidebar'), exact: true }).click()
        assert(await page.locator('.personal-console').getAttribute('data-collapsed') === 'true')
        assert(await page.evaluate(() => {
          const image = document.querySelector('.pc-sidebar .pc-brand img').getBoundingClientRect()
          const sidebar = document.querySelector('.pc-sidebar').getBoundingClientRect()
          return image.left >= sidebar.left && image.right <= sidebar.right && document.documentElement.scrollWidth <= innerWidth
        }), 'Collapsed branding stays inside its sidebar without overflowing the workspace')
        await page.getByRole('button', { name: t('Expand sidebar'), exact: true }).click()
      } else {
        await page.getByRole('button', { name: t('More'), exact: true }).click()
        const more = page.getByRole('dialog', { name: t('All modules'), exact: true })
        await more.getByRole('link', { name: t('System status'), exact: true }).waitFor()
        await page.keyboard.press('Escape')
      }
      for (const [name, path] of [['keys', '/keys'], ['connections', '/connections?connection=1'], ['requests', '/usage-logs/common?tokenId=21']]) {
        await page.goto(origin + path, { waitUntil: 'networkidle' })
        assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'No horizontal viewport overflow: ' + name)
        if (width >= 1024) {
          const progress = page.locator('.myapi-navigation-progress').last()
          await progress.waitFor({ state: 'attached' })
          assert(await progress.evaluate(e => { const r = e.getBoundingClientRect(), w = document.querySelector('.pc-workspace').getBoundingClientRect(); return r.left >= w.left && r.right <= w.right + 1 }), 'Page loading indicator stays inside workspace')
        }
        await page.waitForFunction(() => Array.from(document.querySelectorAll('.myapi-navigation-progress')).every(e => {
          const bar = e.firstElementChild
          return !bar || bar.getBoundingClientRect().width === 0 || getComputedStyle(bar).opacity === '0'
        }))
        const file = `console-layout-${name}-${language}-${width}.png`
        await page.screenshot({ path: resolve(output, file), animations: 'disabled' })
        journey.screenshots.push(file)
      }
      assert.deepEqual(violations, [])
      assert.deepEqual(errors, [])
      journey.result = 'passed'
    } catch (error) {
      journey.result = 'failed'; journey.error = error.message
      await page.screenshot({ path: resolve(output, `console-layout-failure-${language}-${width}.png`) }).catch(() => {})
      throw error
    } finally { await context.close() }
  }
  assert(fixture.requests.every(r => r.method === 'GET' || r.path === '/api/user/auth/refresh'))
  report.result = 'passed'
} catch (error) {
  report.result = 'failed'; report.error = error.message
  throw error
} finally {
  writeFileSync(resolve(output, 'console-layout-qualification.json'), JSON.stringify(report, null, 2))
  if (browser) await browser.close()
  await new Promise(done => server.close(done))
}
