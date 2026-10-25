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
        // Design direction 3: one 56px top bar holds the brand, the primary
        // areas with Admin and More, and the actions; there is no sidebar.
        assert.equal(await page.locator('.pc-sidebar, [data-myapi-sidebar]').count(), 0, 'The console has no sidebar')
        const nav = page.getByRole('navigation', { name: t('Personal console navigation') })
        const bar = await page.evaluate(() => {
          const header = document.querySelector('[data-myapi-header]').getBoundingClientRect()
          const brand = document.querySelector('.pc-brand').getBoundingClientRect()
          const nav = document.querySelector('.pc-topnav')
          const items = [...nav.children].map(e => e.getBoundingClientRect())
          const actions = document.querySelector('.pc-header-actions').getBoundingClientRect()
          return {
            height: header.height,
            brandInside: brand.top >= header.top && brand.bottom <= header.bottom && brand.left >= 0,
            navRow: items.every(r => r.top >= header.top - 1 && r.bottom <= header.bottom + 1),
            clear: items.at(-1).right <= actions.left && actions.right <= innerWidth && nav.scrollWidth <= nav.clientWidth + 1,
            overflow: document.documentElement.scrollWidth > innerWidth,
          }
        })
        assert(bar.height === 56 && bar.brandInside && bar.navRow, `Brand, navigation and actions share one 56px row: ${JSON.stringify(bar)}`)
        assert(bar.clear && !bar.overflow, `Every top-bar destination keeps its full label clear of the actions: ${JSON.stringify(bar)}`)
        // Keyboard: the links and the two menu buttons are tab stops, and a
        // menu opened with Enter returns focus to its button on Escape.
        const first = nav.getByRole('link').first()
        await first.focus()
        const stops = (await nav.getByRole('link').count()) + (await nav.getByRole('button').count())
        for (let index = 1; index < stops; index++) await page.keyboard.press('Tab')
        const more = nav.getByRole('button', { name: t('More'), exact: true })
        assert(await more.evaluate(e => e === document.activeElement), 'The last top-bar destination is reachable by Tab')
        await page.keyboard.press('Enter')
        await page.getByRole('menu').waitFor()
        await page.keyboard.press('Escape')
        await page.getByRole('menu').waitFor({ state: 'hidden' })
        assert(await more.evaluate(e => e === document.activeElement), 'Escape returns focus to the menu button')
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
