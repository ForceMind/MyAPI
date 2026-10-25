// Operable synthetic preview and browser qualification share the same fixture.
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { createKeyRequestPreview } from './key-request-preview.mjs'

const repo = fileURLToPath(new URL('../../', import.meta.url))
const output = resolve(process.env.MYAPI_BROWSER_ARTIFACTS || '.local-tests/key-requests')
mkdirSync(output, { recursive: true })
const report = { head: execFileSync('git', ['rev-parse', 'HEAD'], { cwd: repo, encoding: 'utf8' }).trim(), synthetic: true, providerRequests: 0, result: 'running', journeys: [] }
const persist = () => writeFileSync(resolve(output, 'key-request-qualification.json'), JSON.stringify(report, null, 2))
const { server, fixture } = createKeyRequestPreview()
let browser
try {
  await new Promise(done => server.listen(0, '127.0.0.1', done))
  const origin = `http://127.0.0.1:${server.address().port}`
  const { chromium } = await import(process.env.MYAPI_PLAYWRIGHT_MODULE ? pathToFileURL(process.env.MYAPI_PLAYWRIGHT_MODULE).href : 'playwright')
  browser = await chromium.launch({ headless: true })
  for (const { language, width } of [{ language: 'en', width: 1280 }, { language: 'zh', width: 390 }, { language: 'ru', width: 320 }]) {
    const translations = JSON.parse(readFileSync(resolve(repo, `web/src/i18n/locales/${language}.json`))).translation
    const t = key => { assert(translations[key], `Missing ${language}: ${key}`); return translations[key] }
    const context = await browser.newContext({ viewport: { width, height: 900 }, reducedMotion: 'reduce', serviceWorkers: 'block' })
    await context.addInitScript(lang => localStorage.setItem('i18nextLng', lang), language)
    const external = [], writes = [], pageErrors = [], screenshots = []
    const start = fixture.requests.length
    await context.route('**/*', async route => {
      const request = route.request(), url = new URL(request.url())
      if (url.origin !== origin) { external.push(url.origin); return route.abort('blockedbyclient') }
      if (request.method() !== 'GET' && url.pathname !== '/api/user/auth/refresh') { writes.push(`${request.method()} ${url.pathname}`); return route.abort('blockedbyclient') }
      await route.continue()
    })
    const page = await context.newPage()
    page.setDefaultTimeout(15000)
    page.on('pageerror', error => pageErrors.push(error.message))
    const capture = async state => {
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `Viewport overflow ${language} ${width} ${state}`)
      const file = `key-request-${state}-${language}-${width}.png`
      await page.screenshot({ path: resolve(output, file), animations: 'disabled' })
      screenshots.push(file)
    }
    // Design direction 3: a Key and a request are each a full page. On PC
    // the breadcrumb leads back up; below 1024px the mobile top bar's Back.
    const phone = width < 1024
    const keyPage = page.getByTestId('key-page')
    const rows = keyPage.locator('[data-request-row]')
    const request = page.getByTestId('request-page')
    const keyName = 'Synthetic same-name Key · 合成同名 Key'
    const param = name => new URL(page.url()).searchParams.get(name)
    const pathname = () => new URL(page.url()).pathname
    const focused = attribute => page.evaluate(name => document.activeElement?.getAttribute(name), attribute)
    const breadcrumb = page.getByRole('navigation', { name: t('Current page'), exact: true })
    // One level back: request -> Key, or Key -> list.
    const stepBack = async fromRequest => {
      if (phone) await page.locator('.pc-header-mobile').getByRole('button', { name: t('Back'), exact: true }).click()
      else if (fromRequest) await breadcrumb.getByRole('button', { name: keyName, exact: true }).click()
      else await breadcrumb.getByRole('button', { name: t('API Keys_nav'), exact: true }).click()
    }
    const openRequest = async (index, requestId) => {
      await rows.nth(index).click()
      // The ID heads the page and may repeat in the diagnosis record.
      await request.getByText(requestId, { exact: true }).first().waitFor()
      assert.equal(pathname(), `/keys/21/requests/${requestId}`)
    }
    const backToKey = async rowId => {
      await stepBack(true)
      await rows.first().waitFor()
      assert.equal(pathname(), '/keys/21')
      assert.equal(await focused('data-request-row'), rowId)
    }
    try {
      await page.goto(`${origin}/keys?filter=Synthetic`, { waitUntil: 'networkidle' })
      if (!phone) {
        // The row's request action grows with its wrapped label, keeps every
        // word whole and overlaps nothing else in the actions cell.
        const actions = await page.getByRole('button', { name: t('View Key requests'), exact: true }).first().evaluate(button => {
          const style = getComputedStyle(button)
          const canvas = document.createElement('canvas').getContext('2d')
          canvas.font = `${style.fontWeight} ${style.fontSize} ${style.fontFamily}`
          const inner = button.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight)
          const widest = Math.max(...button.textContent.trim().split(/\s+/).map(word => canvas.measureText(word).width))
          const boxes = [...button.parentElement.children].map(element => element.getBoundingClientRect()).filter(box => box.width > 0)
          const overlaps = boxes.some((a, i) => boxes.some((b, j) => i < j && a.left < b.right - 1 && a.right > b.left + 1 && a.top < b.bottom - 1 && a.bottom > b.top + 1))
          return { fits: button.scrollHeight <= button.clientHeight + 1 && button.scrollWidth <= button.clientWidth + 1, wordsWhole: widest <= inner + 1, overlaps, widest, inner }
        })
        assert(actions.fits && actions.wordsWhole && !actions.overlaps, `Key row request action keeps its label inside with whole words: ${JSON.stringify(actions)}`)
      }
      await page.locator('[data-inspect-id="21"]').first().click()
      await rows.first().waitFor()
      assert.equal(pathname(), '/keys/21')
      // Only Key #21's four requests: the same-name Key #22 and the record
      // without a saved Key ID stay out.
      assert.equal(await rows.count(), 4)
      await capture('key-requests')
      await openRequest(0, 'SYNTHETIC-CONTEXT-ERROR')
      await request.getByText(t('Estimated input Tokens'), { exact: true }).waitFor()
      await request.getByText(t('Input exceeds the upstream context window. Shorten the input and try again.'), { exact: true }).waitFor()
      await capture('request-diagnosis')
      // The saved body is the default content tab (not in the URL).
      await request.getByRole('tab', { name: t('Body'), exact: true }).click()
      assert.equal(await request.getByRole('tab', { name: t('Body'), exact: true }).getAttribute('aria-selected'), 'true')
      await request.getByRole('alert').filter({ hasText: /exceeds the context window/ }).waitFor()
      assert(await request.getByText('200', { exact: true }).count() > 0)
      assert.equal(param('tab'), null)
      await capture('stream-error')
      await request.getByRole('tab', { name: 'SSE', exact: true }).click()
      await request.getByText('context_length_exceeded', { exact: false }).last().waitFor()
      assert.equal(param('tab'), 'sse')
      await capture('sse-raw')
      await backToKey('100')
      await openRequest(1, 'SYNTHETIC-COMPLETED-NO-USAGE')
      await request.getByText(t('Request successful'), { exact: true }).first().waitFor()
      // Missing usage is "not provided", never zero.
      await request.getByText(t('Usage not provided'), { exact: true }).first().waitFor()
      await capture('successful-unknown-usage')
      await backToKey('99')
      await openRequest(2, 'SYNTHETIC-NOT-RECORDED')
      await request.getByRole('tab', { name: t('Body'), exact: true }).click()
      await request.getByText(t('No saved body for this request'), { exact: true }).waitFor()
      await capture('missing-body')
      await backToKey('98')
      await openRequest(3, 'SYNTHETIC-PREFLIGHT-REJECTED')
      await request.getByText(t('Blocked before upstream'), { exact: true }).waitFor()
      await request.getByText(t('Configured input limit'), { exact: true }).waitFor()
      await capture('preflight-rejected')
      await request.getByRole('tab', { name: t('Body'), exact: true }).click()
      await request.getByText('400', { exact: true }).first().waitFor()
      await capture('preflight-body')
      await backToKey('95')
      // The full call records stay one link away, pinned to the Key ID.
      await keyPage.getByRole('link', { name: t('View all in call records'), exact: true }).click()
      await page.getByText(t('Requests for Key #{{id}}').replace('{{id}}', '21'), { exact: true }).waitFor({ state: 'visible' })
      assert.equal(param('tokenId'), '21')
      assert.equal(await page.getByText('SYNTHETIC-OTHER-KEY', { exact: true }).count(), 0)
      assert.equal(await page.getByText('SYNTHETIC-LEGACY-NO-ID', { exact: true }).count(), 0)
      await capture('requests')
      const paged = new URL(page.url()); paged.searchParams.set('pageSize', '1')
      await page.goto(paged.href, { waitUntil: 'networkidle' })
      await page.getByRole('button', { name: t('Go to next page'), exact: true }).click()
      await page.getByText('SYNTHETIC-COMPLETED-NO-USAGE', { exact: true }).first().waitFor()
      await page.getByRole('button', { name: t('Go to previous page'), exact: true }).click()
      await page.getByText('SYNTHETIC-CONTEXT-ERROR', { exact: true }).first().waitFor()
      assert.equal(param('page') || '1', '1')
      await page.getByRole('link', { name: t('Back to Keys'), exact: true }).click()
      await page.locator('[data-inspect-id="21"]').first().waitFor()
      assert.equal(param('filter'), 'Synthetic')
      assert.equal(param('focusKeyId'), '21')
      // PC focuses the row's request button; a phone focuses the Key card.
      if (phone) assert.equal(await focused('data-inspect-id'), '21')
      else assert.equal(await page.evaluate(() => document.activeElement?.textContent), t('View Key requests'))
      await capture('returned-key')
      await page.locator('[data-inspect-id="21"]').first().click()
      await rows.first().waitFor()
      await stepBack(false)
      await page.locator('[data-inspect-id="21"]').first().waitFor()
      assert.equal(pathname(), '/keys')
      assert.equal(param('filter'), 'Synthetic')
      assert.equal(await focused('data-inspect-id'), '21')
      // Older keyId and inspect links open the same Key page.
      await page.goto(`${origin}/keys?keyId=21`, { waitUntil: 'networkidle' })
      await rows.first().waitFor()
      assert.equal(pathname(), '/keys/21')
      assert.equal(param('keyId'), null)
      await page.goto(`${origin}/keys?inspect=21&request=SYNTHETIC-CONTEXT-ERROR&requestTab=sse`, { waitUntil: 'networkidle' })
      await request.getByText('SYNTHETIC-CONTEXT-ERROR', { exact: true }).first().waitFor()
      assert.equal(pathname(), '/keys/21/requests/SYNTHETIC-CONTEXT-ERROR')
      assert.equal(param('tab'), 'sse')
      assert.equal(param('inspect'), null)
      // A pasted Request ID (earlier link form) opens its page directly.
      await page.goto(`${origin}/usage-logs/common?requestId=SYNTHETIC-PREFLIGHT-REJECTED&inspect=request`, { waitUntil: 'networkidle' })
      await request.getByText('SYNTHETIC-PREFLIGHT-REJECTED', { exact: true }).first().waitFor()
      await request.getByText(t('Blocked before upstream'), { exact: true }).waitFor()
      assert.equal(pathname(), '/usage-logs/request/SYNTHETIC-PREFLIGHT-REJECTED')
      await capture('direct-request')
      const reads = fixture.requests.slice(start)
      for (const path of ['/api/log', '/api/log/stat']) assert(reads.some(item => item.path === path && item.token_id === '21'), `${path} is pinned to Key ID`)
      assert(reads.some(item => item.path === '/api/log' && item.request_id === 'SYNTHETIC-PREFLIGHT-REJECTED'), 'The Request ID is looked up exactly')
      assert(!reads.some(item => /\/key$/.test(item.path)), 'No secret Key retrieval')
      assert.deepEqual(writes, []); assert.deepEqual(external, []); assert.deepEqual(pageErrors, [])
      report.journeys.push({ language, width, result: 'passed', checks: ['key-row-opens-key-page', 'visible-key-id-scope', 'same-name-and-legacy-exclusion', 'request-diagnosis', 'http200-sse-error-body-tab', 'sse-tab-raw', 'completed-with-usage-not-provided', 'missing-body', 'back-one-level-with-focus', 'stable-id-list-and-stat', 'pagination', 'return-filter-and-focus', 'close-returns-focus', 'legacy-keyId-link', 'legacy-inspect-request-link', 'request-id-direct-open', 'no-external-or-write-or-secret-requests'], screenshots })
      persist()
    } catch (error) {
      await page.screenshot({ path: resolve(output, `key-request-failure-${language}-${width}.png`) }).catch(() => {})
      report.journeys.push({ language, width, result: 'failed', error: error.message, pageErrors, external, writes, screenshots }); persist(); throw error
    } finally { await context.close() }
  }
  report.result = 'passed'; persist()
  console.log(JSON.stringify(report, null, 2))
} catch (error) { report.result = 'failed'; report.error = error.message; persist(); throw error }
finally { if (browser) await browser.close(); await new Promise(done => server.close(done)) }
