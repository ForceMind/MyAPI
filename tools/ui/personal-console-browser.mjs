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
const report = { head, version, workflowHead: process.env.GITHUB_SHA || null, scope: 'Platform call records and additive Token/cache statistics: scope, filters, known subtotals, unknown usage, explicit cached zero, empty range, retry, seven languages and PC/mobile. Synthetic APIs only; no providers or production.', result: 'running', journeys: [], tokenStatistics: [], screenshots: [], errors: [] }
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
  cases.push(...[390, 768, 1024, 1440].map(width => ({ language: 'zh', width })))
  for (const item of cases) {
    const mobile = item.width < 1024
    const t = key => {
      assert(translations[item.language][key], `Missing ${item.language} key ${key}`)
      return translations[item.language][key]
    }
    const context = await browser.newContext({ viewport: { width: item.width, height: 960 }, reducedMotion: 'reduce', colorScheme: 'light', serviceWorkers: 'block' })
    const fixture = createUIFixture({ role: 100, language: codes[item.language], longText: item.language === 'fr' })
    const errors = [], external = [], queries = [], statisticsQueries = []
    const statisticsMode = item.language === 'en' && item.width === 1280 ? 'mixed' : item.language === 'fr' && item.width === 320 ? 'zero-cache' : 'unknown'
    let statisticsFailed = false
    let failProfile = false
    await context.addInitScript(language => localStorage.setItem('i18nextLng', language), codes[item.language])
    await context.route('**/*', async route => {
      const request = route.request(), url = new URL(request.url())
      if (url.origin !== origin) { external.push(url.origin); return route.abort('blockedbyclient') }
      if (!url.pathname.startsWith('/api/') && !url.pathname.startsWith('/pg/')) return route.continue()
      assert(request.method() === 'GET' || url.pathname === '/api/user/auth/refresh', 'No business mutation or Key reveal')
      if (url.pathname === '/api/user/self' && failProfile) return route.fulfill({ status: 503, json: { success: false } })
      if (/^\/api\/log(\/self)?\/stat$/.test(url.pathname)) {
        statisticsQueries.push({ path: url.pathname, params: Object.fromEntries(url.searchParams) })
        const selected = url.searchParams.get('request_id')
        if (selected === 'STATS-RETRY' && !statisticsFailed) { statisticsFailed = true; return route.fulfill({ status: 503, json: { success: false } }) }
        const empty = selected === 'STATS-EMPTY' || selected === 'STATS-RETRY'
        const known = !empty && statisticsMode !== 'unknown'
        const input = statisticsMode === 'mixed' ? '1000' : '100'
        const output = statisticsMode === 'mixed' ? '100' : '0'
        const token_usage = {
          records: empty ? 0 : 21, known_records: known ? 1 : 0, unknown_records: empty ? 0 : known ? 20 : 21,
          input_tokens: empty ? '0' : null, output_tokens: empty ? '0' : null, total_tokens: empty ? '0' : null,
          known_input_tokens: known ? input : '0', known_output_tokens: known ? output : '0', known_total_tokens: known ? String(BigInt(input) + BigInt(output)) : '0',
          cache_known_records: known ? 1 : 0, cache_unknown_records: empty ? 0 : known ? 20 : 21,
          cache_read_tokens: known && statisticsMode === 'mixed' ? '250' : '0', cache_input_tokens: known ? input : '0',
          cache_hit_rate: known ? statisticsMode === 'mixed' ? 0.25 : 0 : null,
        }
        return route.fulfill({ json: { success: true, data: { quota: 0, rpm: 0, tpm: 0, token_usage } } })
      }
      if (/^\/api\/log(\/self)?$/.test(url.pathname)) {
        queries.push({ path: url.pathname, params: Object.fromEntries(url.searchParams) })
        const pageNumber = Number(url.searchParams.get('p') || 1)
        const log = id => {
          const row = { id, user_id: 1, token_id: 21, channel: 1, channel_name: 'synthetic-channel', created_at: FIXTURE_TIME / 1000 - id, type: 5, content: 'Synthetic completed response', username: 'synthetic-operator', token_name: 'synthetic-key', model_name: 'synthetic-text-model', quota: 125000, prompt_tokens: 0, completion_tokens: 0, use_time: 2, is_stream: true, request_id: `REQ-SYNTHETIC-${id}`, group: 'default', ip: '', other: JSON.stringify({ request_outcome: 'completed', usage_outcome: 'missing' }) }
          if (id === 501 && statisticsMode !== 'unknown') {
            row.type = 2; row.prompt_tokens = statisticsMode === 'mixed' ? 1000 : 100; row.completion_tokens = statisticsMode === 'mixed' ? 100 : 0
            row.other = JSON.stringify({ usage_accuracy: 'reported', token_usage_evidence: { schema: 1, input_tokens: row.prompt_tokens, output_tokens: row.completion_tokens, cache_read_tokens: statisticsMode === 'mixed' ? 250 : 0 } })
          }
          return row
        }
        const empty = ['STATS-EMPTY', 'STATS-RETRY'].includes(url.searchParams.get('request_id'))
        const items = empty ? [] : pageNumber === 1 ? Array.from({ length: 20 }, (_, i) => log(501 + i)) : [log(521)]
        return route.fulfill({ json: { success: true, data: { items, total: empty ? 0 : 21, page: pageNumber, page_size: 20 } } })
      }
      if (url.pathname === '/api/full-content-logs/REQ-SYNTHETIC-521') {
        return route.fulfill({ json: { success: true, data: { request_id: 'REQ-SYNTHETIC-521', timestamp: new Date(FIXTURE_TIME).toISOString(), method: 'POST', path: '/v1/responses', status: 200, duration_ms: 2000, chunk_count: 1, request_bytes: 2, response_bytes: 50, request_body: '{"input":"Synthetic request"}', response_body: 'data: {"choices":[{"delta":{"content":"SYNTHETIC-SAVED-RESPONSE"}}]}\n\ndata: [DONE]\n\n', response_content_type: 'text/event-stream', request_headers: {}, response_headers: {}, query: {} } } })
      }
      const response = fixture.response(url, request.method())
      assert(!response.violation, response.violation)
      if (url.pathname === '/api/user/auth/refresh') response.body.data.access_expires_at = FIXTURE_TIME / 1000 + 3600
      await route.fulfill({ status: response.status, json: response.body })
    })
    const page = await context.newPage()
    await page.clock.setFixedTime(FIXTURE_TIME)
    page.setDefaultTimeout(20000)
    page.on('pageerror', error => errors.push(error.message))
    const capture = async name => {
      await page.locator('main .pointer-events-none.opacity-60').waitFor({ state: 'hidden' })
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, 'No horizontal viewport overflow')
      const file = `platform-${name}-${item.language}-${item.width}.png`
      await page.screenshot({ path: resolve(output, file), animations: 'disabled' })
      report.screenshots.push({ file, ...item, state: name }); persist()
    }
    try {
      await page.goto(`${origin}/profile`, { waitUntil: 'networkidle' })
      await page.getByRole('heading', { name: fixture.user.display_name, level: 2, exact: true }).waitFor()
      const revision = await page.locator('html').getAttribute('data-build-rev')
      assert(revision?.startsWith(`rv.${version}.${process.env.GITHUB_SHA || head}.`))
      const nav = page.getByRole('navigation', { name: t(mobile ? 'Mobile console navigation' : 'Personal console navigation') })
      assert(await nav.isVisible())
      if (mobile) {
        await nav.getByRole('button', { name: t('More'), exact: true }).click()
        const more = page.getByRole('dialog', { name: t('All modules') })
        for (const [key, href] of [['Bodies & SSE', '/full-content-logs'], ['Tasks', '/usage-logs/task'], ['Drawing', '/usage-logs/drawing'], ['System status', '/system-info'], ['Settings', '/system-settings/site']]) {
          assert.equal(await more.getByRole('link', { name: t(key), exact: true }).getAttribute('href'), href)
        }
        await capture('more')
        await page.keyboard.press('Escape')
      } else {
        assert.equal(await nav.getByRole('link', { name: t('Bodies & SSE'), exact: true }).getAttribute('href'), '/full-content-logs')
      }
      // Profile moved to the account menu; the call records entry is the
      // active destination whose contrast is checked on desktop.
      const callRecords = nav.getByRole('link', { name: t(mobile ? 'Records' : 'Requests_nav'), exact: true })
      await callRecords.click()
      await page.getByRole('heading', { name: t('Call records'), level: 1 }).waitFor()
      // Below 1280px the sidebar starts as an icon rail without visible labels.
      if (item.width >= 1280) await assertTextContrast(callRecords.locator('span'), 'active platform navigation', 'light')
      // This bookmarked server-filtered list must survive detail and return.
      await page.goto(`${origin}/usage-logs/common?pageSize=20&model=synthetic-text-model`, { waitUntil: 'networkidle' })
      const statistics = page.getByRole('region', { name: t('Token usage statistics'), exact: true })
      await statistics.waitFor()
      const values = await statistics.locator('dd').allTextContents()
      if (statisticsMode === 'mixed') {
        assert(values.includes('1,100'), 'Partial usage presents the exact known subtotal')
        assert(values.some(value => value.replace(/\s/g, '') === '25%'), 'Cache rate uses paired input evidence')
        await statistics.getByText(t('Known Token subtotal'), { exact: true }).waitFor()
      } else if (statisticsMode === 'zero-cache') {
        assert(values.some(value => value.replace(/\s/g, '') === '0%'), 'An explicitly reported cache zero stays zero')
      } else assert.deepEqual(values, Array(4).fill(t('Unknown')), 'Missing usage and absent cache evidence remain unknown')
      await capture('token-statistics')
      const methodTarget = await statistics.locator('summary').boundingBox()
      assert(methodTarget && methodTarget.width >= 44 && methodTarget.height >= 44, 'The statistics method remains an accessible touch target')
      await statistics.locator('summary').click()
      await statistics.getByText(t('Cache hit rate uses summed cached input divided by summed input for records with cache evidence. Missing usage is excluded, not zero.'), { exact: true }).waitFor({ state: 'visible' })
      await statistics.locator('summary').click()
      report.tokenStatistics.push({ ...item, mode: statisticsMode, values, result: 'passed' })
      assert(queries.some(query => query.path === '/api/log' && query.params.model_name === 'synthetic-text-model'), 'Admin scope and model filter reach backend')
      assert.equal(await page.getByRole('columnheader', { name: t('Cost'), exact: true }).count(), 0)
      let filterScope = page.getByRole('main')
      if (mobile) {
        await page.getByRole('button', { name: new RegExp(`^${t('Filter')}`) }).click()
        filterScope = page.getByRole('dialog', { name: t('Filter'), exact: true })
      } else {
        await page.getByRole('button', { name: t('Expand'), exact: true }).click()
      }
      await filterScope.getByPlaceholder(t('Request ID'), { exact: true }).fill('REQ-SYNTHETIC-')
      await Promise.all([
        page.waitForResponse(response => /\/api\/log\?/.test(response.url()) && new URL(response.url()).searchParams.get('request_id') === 'REQ-SYNTHETIC-'),
        filterScope.getByRole('button', { name: t('Search'), exact: true }).click(),
      ])
      assert(queries.some(query => query.params.request_id === 'REQ-SYNTHETIC-'), 'Submitted Request ID filter reaches server')
      assert(statisticsQueries.some(query => query.params.request_id === 'REQ-SYNTHETIC-'), 'Statistics use the same Request ID filter')
      if (!mobile) {
        const action = await page.getByTitle(t('Click to view full details'), { exact: true }).first().boundingBox()
        assert(action && action.x >= 0 && action.x + action.width <= item.width, 'Primary details action is visible without horizontal scrolling')
      }
      await capture('records')
      await page.getByRole('button', { name: t('Go to next page'), exact: true }).click()
      await page.getByText('REQ-SYNTHETIC-521', { exact: true }).first().waitFor()
      assert(queries.some(query => query.params.p === '2'), 'Server pagination reads page 2')
      const before = page.url()
      await page.getByTitle(t('Click to view full details'), { exact: true }).first().click()
      const detail = page.getByRole('dialog', { name: new RegExp(t('Log Details')) })
      assert(await detail.getByText(t('Request succeeded'), { exact: true }).count(), 'Missing usage remains successful')
      assert(await detail.getByText(t('Unknown'), { exact: true }).count() >= 2, 'Input/output stay unknown')
      assert.equal(await detail.getByText(t('Total Cost'), { exact: true }).count(), 0)
      await capture('detail')
      await detail.getByRole('button', { name: t('View request content'), exact: true }).click()
      const content = page.getByRole('dialog', { name: t('Full Content Log Detail'), exact: true })
      await content.getByRole('tab', { name: new RegExp(t('Response')) }).click()
      await content.getByText('SYNTHETIC-SAVED-RESPONSE', { exact: false }).waitFor()
      await capture('saved-content')
      await page.keyboard.press('Escape')
      await detail.waitFor()
      await page.keyboard.press('Escape')
      assert.equal(page.url(), before, 'Filter and page URL survive return')
      await page.getByText('REQ-SYNTHETIC-521', { exact: true }).first().waitFor()
      await Promise.all([page.waitForResponse(response => /\/api\/log\/self\?/.test(response.url())), page.waitForResponse(response => new URL(response.url()).pathname === '/api/log/self/stat'), page.getByRole('tab', { name: t('Only Mine'), exact: true }).click()])
      assert(queries.some(query => query.path === '/api/log/self'), 'Only Mine uses self API')
      assert(statisticsQueries.some(query => query.path === '/api/log/self/stat'), 'Only Mine uses self statistics')
      if (item.language === 'en' && item.width === 1280) {
        await page.goto(`${origin}/usage-logs/common?requestId=STATS-EMPTY`, { waitUntil: 'networkidle' })
        await page.getByText(t('No call records match this range.'), { exact: true }).waitFor()
        assert.deepEqual(await page.getByTestId('log-token-statistics').locator('dd').allTextContents(), ['0', '0', '0', t('Unknown')])
        await capture('token-statistics-empty')
        await page.goto(`${origin}/usage-logs/common?requestId=STATS-RETRY`, { waitUntil: 'networkidle' })
        const error = page.getByRole('alert').filter({ hasText: t('Failed to load usage statistics') })
        await error.waitFor()
        assert.equal(await page.getByTestId('log-token-statistics').count(), 0, 'A failed read does not display a fabricated zero total')
        await capture('token-statistics-failed')
        await error.getByRole('button', { name: t('Retry'), exact: true }).click()
        await page.getByText(t('No call records match this range.'), { exact: true }).waitFor()
        report.tokenStatistics.push({ name: 'empty-and-retry', result: 'passed' })
      }
      if (item.language === 'zh') {
        failProfile = true
        await page.goto(`${origin}/profile`, { waitUntil: 'networkidle' })
        await page.getByRole('alert').filter({ hasText: t('Failed to load profile') }).waitFor()
        failProfile = false
        await page.getByRole('button', { name: t('Retry'), exact: true }).click()
        await page.getByRole('heading', { name: fixture.user.display_name, level: 2, exact: true }).waitFor()
      }
      assert.deepEqual(errors, [], 'No runtime errors'); assert.deepEqual(external, [], 'Zero external requests')
      report.journeys.push({ ...item, revision, queries, statisticsQueries, result: 'passed' })
    } finally { await context.close() }
  }
  // Public default home and signed-in root entry are separate paths.
  for (const role of [0, 100]) {
    const fixture = createUIFixture({ role, language: 'zhCN', personalPolicy: true })
    const context = await browser.newContext({ viewport: { width: 1280, height: 960 }, serviceWorkers: 'block' })
    await context.addInitScript(() => localStorage.setItem('i18nextLng', 'zhCN'))
    const homeAudit = { violations: [], external: [], runtime: [], requests: [] }
    await context.route('**/*', async route => {
      const request = route.request(), url = new URL(request.url())
      if (url.origin !== origin) { homeAudit.external.push(url.origin); return route.abort('blockedbyclient') }
      if (!url.pathname.startsWith('/api/') && !url.pathname.startsWith('/pg/')) return route.continue()
      homeAudit.requests.push({ method: request.method(), path: url.pathname })
      const response = fixture.response(url, request.method())
      if (response.violation) {
        homeAudit.violations.push(response.violation)
        return route.fulfill({ status: 503, json: { success: false, message: response.violation } })
      }
      await route.fulfill({ status: response.status, json: response.body })
    })
    const page = await context.newPage()
    page.on('pageerror', error => homeAudit.runtime.push(error.message))
    await page.clock.setFixedTime(FIXTURE_TIME)
    try {
      await page.goto(origin, { waitUntil: 'domcontentloaded' })
      if (role === 0) {
        await page.getByRole('heading', { name: translations.zh['Your gateway. Every request in view.'], exact: true }).waitFor()
        await page.screenshot({ path: resolve(output, 'platform-home-zh-1280.png') })
        report.screenshots.push({ file: 'platform-home-zh-1280.png', state: 'public-home', width: 1280, language: 'zh' })
      } else {
        await page.waitForURL('**/dashboard/overview', { waitUntil: 'domcontentloaded' })
        await page.getByTestId('personal-workbench').waitFor()
        await page.getByTestId('overview-keys').getByText('1', { exact: true }).first().waitFor()
      }
      assert.deepEqual(homeAudit.violations, [], 'Home entry only uses configured fixture APIs')
      assert.deepEqual(homeAudit.external, [], 'Home entry has zero external requests')
      assert.deepEqual(homeAudit.runtime, [], 'Home entry has no runtime errors')
    } catch (error) {
      const file = `platform-home-failed-${role}`
      const failure = { role, url: page.url(), error: String(error), ...homeAudit }
      writeFileSync(resolve(output, `${file}.json`), JSON.stringify(failure, null, 2))
      try { await page.screenshot({ path: resolve(output, `${file}.png`), timeout: 5000, animations: 'disabled' }) }
      catch (screenshotError) { failure.screenshotError = String(screenshotError); writeFileSync(resolve(output, `${file}.json`), JSON.stringify(failure, null, 2)) }
      throw error
    }
    report.journeys.push({ name: 'home-entry', role, result: 'passed' })
    await context.close()
  }
  report.result = 'passed'
} catch (error) { report.result = 'failed'; report.errors.push(error.stack || String(error)); throw error }
finally { persist(); if (browser) await browser.close(); await new Promise(done => server.close(done)); console.log(JSON.stringify(report)) }
