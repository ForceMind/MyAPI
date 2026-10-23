// Qualification of shared page alignment with isolated synthetic APIs.
import assert from 'node:assert/strict'
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'
import { createPageLayoutPreview } from './page-layout-preview.mjs'

const output = resolve(process.env.MYAPI_BROWSER_ARTIFACTS || '.local-tests/page-layout')
mkdirSync(output, { recursive: true })
const baseline = process.env.MYAPI_UI_BASELINE === '1'
const { server, state } = createPageLayoutPreview()
const { chromium } = await import(pathToFileURL(process.env.MYAPI_PLAYWRIGHT_MODULE || '/tmp/myapi-round1-browser/node_modules/playwright/index.mjs').href)
const pages = [
  ['overview', '/dashboard/overview'],
  ['accounts', '/upstream-accounts'],
  ['models', '/models/metadata'],
  ['channels', '/channels'],
  ['analytics', '/dashboard/models'],
  ['content', '/full-content-logs'],
  ['profile', '/profile'],
  ['system', '/system-info'],
  ['settings', '/system-settings/site/system-info'],
]
const report = { synthetic: true, providerRequests: 0, baseline, journeys: [] }
let browser
try {
  await new Promise(done => server.listen(0, '127.0.0.1', done))
  const origin = `http://127.0.0.1:${server.address().port}`
  browser = await chromium.launch({ headless: true })
  const cases = baseline ? [['zh', 1280], ['zh', 390]] : [['zh', 1280], ['zh', 390], ['ru', 320], ['fr', 320], ['en', 768], ['zh', 1024, 'dark']]
  for (const [language, width, theme = 'light'] of cases) {
    const translations = JSON.parse(readFileSync(`web/src/i18n/locales/${language}.json`)).translation
    const t = key => translations[key] || key
    const context = await browser.newContext({ viewport: { width, height: 900 }, hasTouch: width < 1024, isMobile: width < 640, colorScheme: theme, reducedMotion: 'reduce', serviceWorkers: 'block' })
    await context.addCookies([{ name: 'vite-ui-theme', value: theme, url: origin }])
    await context.addInitScript(lang => localStorage.setItem('i18nextLng', lang), language)
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
    page.setDefaultTimeout(15000)
    page.on('pageerror', error => errors.push(error.message))
    for (const [name, path] of pages.filter(([name]) => !process.env.MYAPI_UI_PAGE || process.env.MYAPI_UI_PAGE.split(',').includes(name))) {
      await page.goto(origin + path, { waitUntil: 'networkidle' })
      await page.getByRole('heading', { level: 1 }).waitFor()
      await page.waitForFunction(() => Array.from(document.querySelectorAll('.myapi-navigation-progress')).every(e => !e.firstElementChild || e.firstElementChild.getBoundingClientRect().width === 0 || getComputedStyle(e.firstElementChild).opacity === '0'))
      const geometry = await page.evaluate(() => {
        const title = document.querySelector('main h1'), main = title.closest('main')
        const header = main.querySelector('.myapi-page-header'), content = main.querySelector('.myapi-page-content')
        const rect = e => e ? { x: e.getBoundingClientRect().x, y: e.getBoundingClientRect().y, width: e.getBoundingClientRect().width, height: e.getBoundingClientRect().height } : null
        return { title: rect(title), main: rect(main), header: rect(header), content: rect(content), overflow: document.documentElement.scrollWidth > innerWidth,
          h1Count: document.querySelectorAll('main h1').length,
          inputs: Array.from(document.querySelectorAll('main input')).map(input => ({ height: input.getBoundingClientRect().height, slot: input.dataset.slot, type: input.type, hidden: getComputedStyle(input).visibility, label: input.getAttribute('aria-label'), placeholder: input.placeholder, minHeight: getComputedStyle(input).minHeight })),
          controls: Array.from(document.querySelectorAll('.myapi-page-actions button')).map(e => ({ ...rect(e), text: e.textContent })) }
      })
      const screenshot = `${name}-${language}-${width}.png`
      await page.screenshot({ path: resolve(output, screenshot), animations: 'disabled' })
      report.journeys.push({ name, language, width, theme, screenshot, geometry })
      assert.equal(geometry.overflow, false, name + ' does not overflow viewport')
      assert.equal(geometry.h1Count, 1, name + ' has one page title')
      if (!baseline) {
        assert(geometry.header && geometry.content, name + ' uses shared page layout')
        assert(Math.abs(geometry.title.x - geometry.content.x - (width >= 1024 ? 24 : 16)) <= 1, name + ' title and content share horizontal gutter')
        assert(Math.abs(geometry.title.y - geometry.main.y - 24) <= 1, name + ' title starts at the standard vertical gutter')
        // Base UI also renders clipped 1px native form proxies. Measure the
        // actual visible input controls rather than those submission proxies.
        assert(await page.locator('main input[data-slot="input"], main input[data-slot="input-group-control"]').evaluateAll(inputs => inputs.every(input => {
          const height = input.getBoundingClientRect().height
          return height === 0 || height >= 40
        })), name + ' visible text controls share a 40px minimum height')
        for (const control of geometry.controls) {
          assert(control.x >= geometry.main.x && control.x + control.width <= geometry.main.x + geometry.main.width + 1, name + ' header actions stay in the page')
          if (width < 1024) assert(control.height >= 44, name + ' mobile header actions are at least 44px, including menu triggers')
        }
        if (name === 'accounts') {
          if (width < 1024) {
            await page.getByRole('combobox', { name: t('Select upstream account'), exact: true }).selectOption('2')
          } else {
            await page.getByRole('complementary', { name: t('Select upstream account'), exact: true }).getByRole('button').last().click()
          }
          await page.getByRole('heading', { name: 'Synthetic long account name · 测试长名称的订阅账号与窗口切换', exact: true }).waitFor()
          assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'Long selected account does not overflow')
        }
        if (name === 'analytics' && width < 1024) {
          const group = page.getByLabel(t('Time Granularity'), { exact: true })
          assert(await group.evaluate(e => Array.from(e.querySelectorAll('button')).every(button => {
            const child = button.getBoundingClientRect(), parent = e.getBoundingClientRect()
            return child.height >= 40 && child.top >= parent.top && child.bottom <= parent.bottom
          })), 'Chart selectors remain touch sized and fully visible inside their group')
        }
        if (name === 'content' && width < 640) {
          const filters = page.getByRole('button', { name: t('Filters'), exact: true })
          await filters.click()
          await page.getByLabel(t('Request ID'), { exact: true }).fill('SYNTHETIC-CONTEXT-ERROR')
          assert.equal(await filters.getAttribute('aria-expanded'), 'true')
          assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'Expanded filters fit the narrow viewport')
          await page.screenshot({ path: resolve(output, `content-filters-${language}-${width}.png`), animations: 'disabled' })
        }
        if (name === 'content' && width >= 1024) {
          const table = page.getByRole('table')
          const header = table.getByRole('columnheader', { name: t('Actions'), exact: true })
          const action = table.getByRole('row').nth(1).getByRole('cell').last()
          const aligned = async () => {
            const head = await header.boundingBox(), cell = await action.boundingBox()
            assert(Math.abs(head.x - cell.x) <= 1 && Math.abs(head.width - cell.width) <= 1, 'Saved-content action header and cells stay aligned while horizontally scrolling')
          }
          await aligned()
          await page.locator('[data-slot="table-container"]').evaluate(e => { e.scrollLeft = e.scrollWidth })
          await aligned()
        }
      }
    }
    if (!baseline) {
      for (const scenario of ['empty', 'error']) {
        state.accounts = scenario
        await page.goto(origin + '/upstream-accounts', { waitUntil: 'networkidle' })
        await page.getByText(t(scenario === 'empty' ? 'No upstream account windows were reported' : 'Unable to load upstream account windows'), { exact: true }).waitFor()
        assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
        await page.screenshot({ path: resolve(output, `accounts-${scenario}-${language}-${width}.png`), animations: 'disabled' })
        report.journeys.push({ name: `accounts-${scenario}`, language, width, result: 'passed' })
      }
      state.accounts = 'populated'
    }
    assert.deepEqual(errors, [])
    assert.deepEqual(blocked, [])
    await context.close()
  }
  report.result = 'passed'
} catch (error) {
  report.result = 'failed'; report.error = error.message
  throw error
} finally {
  writeFileSync(resolve(output, 'page-layout-qualification.json'), JSON.stringify(report, null, 2))
  if (browser) await browser.close()
  await new Promise(done => server.close(done))
}
