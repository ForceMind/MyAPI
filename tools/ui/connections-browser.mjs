// Synthetic browser qualification of connection tasks. No real providers,
// credentials, business mutations or production data are used by this runner.
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { createReadStream, existsSync, mkdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { createServer } from 'node:http'
import { extname, resolve, sep } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { createUIFixture, FIXTURE_TIME } from './browser-fixtures.mjs'
import { connectionLocation, createConnectionKeyFixture } from './connection-key-browser-fixtures.mjs'

const repo = fileURLToPath(new URL('../../', import.meta.url))
const root = resolve(repo, 'web/dist')
const output = resolve(process.env.MYAPI_BROWSER_ARTIFACTS || '.local-tests/connections')
const head = execFileSync('git', ['rev-parse', 'HEAD'], { cwd: repo, encoding: 'utf8' }).trim()
const codes = { en: 'en', zh: 'zhCN', 'zh-TW': 'zhTW', fr: 'fr', ja: 'ja', ru: 'ru', vi: 'vi' }
const translations = Object.fromEntries(Object.keys(codes).map(lang => [lang, JSON.parse(readFileSync(resolve(repo, `web/src/i18n/locales/${lang}.json`))).translation]))
const report = { head, workflowHead: process.env.GITHUB_SHA || null, scope: 'Connection grid and connection page, persisted Key identity, pool pagination, scoped edit cancellation, Apply/Reset request return context and failure boundaries. Six widths by seven locales, synthetic APIs only.', result: 'running', journeys: [], screenshots: [], errors: [] }
const persist = () => writeFileSync(resolve(output, 'connections-qualification.json'), JSON.stringify(report, null, 2))
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
// Match the real GetChannel DTO, including non-optional channel_info. The same
// fixture drives an unmocked workspace + editor integration regression.
const editorFixture = JSON.parse(readFileSync(resolve(repo, 'tools/ui/connection-editor-fixture.json')))
const longName = editorFixture.name
const connection = id => ({ ...editorFixture, id, type: id === 1 ? 1 : 57, name: id === 1 ? longName : 'Synthetic subscription connection' })
let browser
try {
  await new Promise(done => server.listen(0, '127.0.0.1', done))
  const origin = `http://127.0.0.1:${server.address().port}`
  const { chromium } = await import(process.env.MYAPI_PLAYWRIGHT_MODULE ? pathToFileURL(process.env.MYAPI_PLAYWRIGHT_MODULE).href : 'playwright')
  browser = await chromium.launch({ headless: true })
  const cases = Object.keys(codes).flatMap(language => [320, 390, 768, 1024, 1280, 1440].map(width => ({ language, width })))
  for (const item of cases) {
    const t = key => { assert(translations[item.language][key], `Missing ${item.language}: ${key}`); return translations[item.language][key] }
    const mobile = item.width < 1024
    const fixture = createUIFixture({ role: 100, language: codes[item.language], personalPolicy: true })
    const associations = createConnectionKeyFixture()
    const context = await browser.newContext({ viewport: { width: item.width, height: 960 }, reducedMotion: 'reduce', colorScheme: 'light', serviceWorkers: 'block' })
    await context.addInitScript(language => localStorage.setItem('i18nextLng', language), codes[item.language])
    const errors = [], violations = [], external = [], requests = [], consoleErrors = [], responses = []
    let failDetail = false, empty = false, deny = false
    await context.route('**/*', async route => {
      const request = route.request(), url = new URL(request.url()), path = url.pathname.replace(/\/$/, '')
      if (url.origin !== origin) { external.push(url.origin); return route.abort('blockedbyclient') }
      if (!path.startsWith('/api/') && !path.startsWith('/pg/')) return route.continue()
      requests.push({ method: request.method(), path, params: Object.fromEntries(url.searchParams) })
      if (request.method() !== 'GET' && path !== '/api/user/auth/refresh') { violations.push(`Unexpected write ${request.method()} ${path}`); return route.abort('blockedbyclient') }
      if (path === '/api/user/self' && deny) return route.fulfill({ status: 403, json: { success: false } })
      const association = associations.response(url, request.method())
      if (association) {
        if (association.violation) { violations.push(association.violation); return route.fulfill({ status: 503, json: { success: false } }) }
        return route.fulfill({ status: association.status, json: association.body })
      }
      if (path === '/api/channel/models') return route.fulfill({ json: { success: true, data: [{ id: 'synthetic-text-model' }] } })
      if (path === '/api/channel' || path === '/api/channel/search') return route.fulfill({ json: { success: true, data: { total: empty ? 0 : 22, page: Number(url.searchParams.get('p') || 1), page_size: 20, items: empty ? [] : [connection(1), connection(2)] } } })
      if (/^\/api\/channel\/[12]$/.test(path)) return route.fulfill({ status: failDetail ? 503 : 200, json: failDetail ? { success: false } : { success: true, data: connection(Number(path.split('/').at(-1))) } })
      if (path === '/api/channel/quota/changes') {
        // The grid reads its cards' observations at once; a page reads one.
        const ids = (url.searchParams.get('channel_ids') || '').split(',').map(Number)
        assert(ids.length > 0 && ids.every(id => [1, 2].includes(id)), 'Saved observations are scoped by connection ID')
        return route.fulfill({ json: { success: true, data: { items: ids.includes(2) ? [{ channel_id: 2, name: 'Synthetic subscription connection', metric_type: 'codex_rate_limit', unit: 'percent', status: 'error', observed_at: FIXTURE_TIME / 1000, source: 'codex_wham_usage_primary', window_seconds: 18000, current_available: 45 }] : [] } } })
      }
      const response = fixture.response(url, request.method())
      if (response.violation) { violations.push(response.violation); return route.fulfill({ status: 503, json: { success: false } }) }
      await route.fulfill({ status: response.status, json: response.body })
    })
    const page = await context.newPage()
    page.setDefaultTimeout(15000)
    await page.clock.setFixedTime(FIXTURE_TIME)
    page.on('pageerror', error => errors.push(error.message))
    // React/router boundaries can render an error page without pageerror. Keep
    // console and response evidence (including deliberate 403/503 fixtures).
    page.on('console', message => { if (message.type() === 'error') consoleErrors.push(message.text()) })
    page.on('response', response => {
      const url = new URL(response.url())
      if (url.origin === origin && (url.pathname.startsWith('/api/') || url.pathname.startsWith('/pg/'))) responses.push({ path: url.pathname, status: response.status() })
    })
    const capture = async state => {
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'No horizontal viewport overflow')
      const file = `connections-${state}-${item.language}-${item.width}.png`
      await page.screenshot({ path: resolve(output, file), animations: 'disabled' })
      report.screenshots.push({ file, state, ...item }); persist()
    }
    try {
      await page.goto(`${origin}/connections?filter=Synthetic&page=2&status=enabled`, { waitUntil: 'networkidle' })
      const inventory = page.getByTestId('connections-inventory')
      const first = inventory.locator('[data-connection-link="1"]')
      await first.waitFor()
      assert(requests.some(request => request.path === '/api/channel/search' && request.params.keyword === 'Synthetic' && request.params.p === '2' && request.params.status === 'enabled'))
      await capture('list')
      await first.focus(); await page.keyboard.press('Enter')
      const detail = page.getByTestId('connection-page')
      await detail.getByRole('heading', { name: longName, exact: true }).waitFor()
      // The connection is a page of its own on every width (design v2); the
      // grid's filters and page travel with it and come back on Back.
      assert.equal(await inventory.count(), 0, 'The connection page replaces the grid')
      assert.equal(new URL(page.url()).pathname, '/connections/1', 'A card opens the connection page')
      assert(await detail.getByRole('heading', { level: 1 }).evaluate(node => node === document.activeElement), 'Page heading receives keyboard focus')
      const tab = name => detail.getByRole('tab', { name: t(name), exact: true })
      // Windows is the first tab and stays out of the URL.
      await detail.getByText(t('No saved subscription windows for this connection. This does not mean the connection is unavailable.'), { exact: true }).waitFor()
      await capture('detail')
      await tab('Routing and models').click()
      await page.waitForURL(url => url.searchParams.get('tab') === 'routing')
      await tab('Windows').click()
      await page.waitForURL(url => !url.searchParams.has('tab'))
      const selectedUrl = page.url()
      // Phones go back with the top-bar Back; wider layouts with the breadcrumb.
      const closeDetail = () => mobile
        ? page.locator('.pc-header-mobile').getByRole('button', { name: t('Back'), exact: true }).click()
        : page.getByRole('navigation', { name: t('Current page'), exact: true }).getByRole('button', { name: t('Connections'), exact: true }).click()

      await detail.getByRole('link', { name: t('Associated Key shares'), exact: true }).click()
      await page.getByRole('heading', { name: `${t('Associated Key shares')} · #1`, level: 1, exact: true }).waitFor()
      await page.getByRole('link', { name: `${t('Pool #{{id}}').replace('{{id}}', '8')} · five_hour`, exact: true }).waitFor()
      assert.equal(await page.getByRole('region', { name: t('All saved share windows'), exact: true }).getByRole('listitem').count(), 20, 'A full saved-pool page retains the server page boundary')
      await capture('key-pools-full-page')
      await page.getByRole('region', { name: t('All saved share windows'), exact: true }).getByRole('button', { name: t('Next'), exact: true }).click()
      const savedPool = page.getByRole('link', { name: `${t('Pool #{{id}}').replace('{{id}}', '7')} · five_hour`, exact: true })
      await savedPool.waitFor()
      assert.equal(JSON.parse(new URL(page.url()).searchParams.get('connectionContext')).poolPage, 2)
      await capture('key-pools')
      await savedPool.click()
      const allocations = page.getByTestId('pool-key-allocations')
      await allocations.getByTestId('pool-key-21').waitFor()
      const keyLink = id => allocations.getByRole('link', { name: t('Key #{{id}}').replace('{{id}}', String(id)), exact: true })
      assert.notEqual(await keyLink(21).getAttribute('href'), await keyLink(22).getAttribute('href'), 'Same-name Keys retain different stable identities')
      await keyLink(21).click()
      await page.getByRole('heading', { name: t('Key #{{id}}').replace('{{id}}', '21'), level: 3, exact: true }).waitFor()
      const keyUrl = page.url()
      await capture('associated-key')
      await page.getByRole('button', { name: t('Manage Key shares'), exact: true }).click()
      const shareEditor = page.getByRole('dialog', { name: t('Subscription shares'), exact: true })
      await shareEditor.waitFor()
      await shareEditor.getByText(t('Approximate usage allocation'), { exact: true }).waitFor()
      await capture('key-edit')
      await page.keyboard.press('Escape')
      await shareEditor.waitFor({ state: 'hidden' })
      assert.equal(page.url(), keyUrl, 'Cancel preserves the exact Key and pool context')
      await page.getByText(t('Requests are matched by the saved Key ID, including before a rename.'), { exact: true }).waitFor()
      await page.getByRole('link', { name: t('View Key requests'), exact: true }).click()
      await page.getByRole('heading', { name: t('Call records'), level: 1, exact: true }).waitFor()
      const logsMain = page.getByRole('main', { name: t('Call records'), exact: true })
      const keyReturn = logsMain.getByRole('link', { name: t('Associated Key shares'), exact: true })
      await keyReturn.waitFor()
      const expectedContext = JSON.parse(new URL(keyUrl).searchParams.get('connectionContext'))
      const assertKeyReturn = async () => {
        assert.equal(new URL(page.url()).searchParams.get('tokenId'), '21', 'Selected Key ID remains pinned after Apply/Reset')
        const href = new URL(await keyReturn.getAttribute('href'), origin)
        assert.deepEqual(JSON.parse(href.searchParams.get('connectionContext')), expectedContext, 'Visible return retains connection filter/page, pool page, pool and Key')
      }
      await assertKeyReturn()
      let filterArea = logsMain
      if (mobile) {
        await logsMain.getByRole('button', { name: t('Filter'), exact: false }).click()
        filterArea = page.getByRole('dialog', { name: t('Filter'), exact: true })
        await filterArea.waitFor()
      }
      await filterArea.getByPlaceholder(t('Model Name'), { exact: true }).fill('synthetic-text-model')
      await filterArea.getByRole('button', { name: t('Search'), exact: true }).click()
      await page.waitForURL(url => url.searchParams.get('model') === 'synthetic-text-model')
      await assertKeyReturn()
      if (mobile) {
        await logsMain.getByRole('button', { name: t('Filter'), exact: false }).click()
        filterArea = page.getByRole('dialog', { name: t('Filter'), exact: true })
        await filterArea.waitFor()
      }
      await filterArea.getByRole('button', { name: t('Reset'), exact: true }).click()
      await page.waitForURL(url => !url.searchParams.has('model') && !url.searchParams.has('channel'))
      await assertKeyReturn()
      await capture('key-request-return')
      await keyReturn.click()
      await page.getByRole('heading', { name: t('Key #{{id}}').replace('{{id}}', '21'), level: 3, exact: true }).waitFor()
      assert.equal(await shareEditor.count(), 0, 'Returning from logs must not reopen an editor')
      await page.getByRole('link', { name: t('All saved share windows'), exact: true }).click()
      await savedPool.waitFor()
      assert.equal(JSON.parse(new URL(page.url()).searchParams.get('connectionContext')).poolPage, 2)
      await page.getByRole('link', { name: t('Back to connection'), exact: true }).click()
      await detail.getByRole('heading', { name: longName, exact: true }).waitFor()
      assert.deepEqual(connectionLocation(page.url()), connectionLocation(selectedUrl), 'Key workflow returns to original selected connection and inventory filters')

      // The connection's call records are linked from its Requests tab.
      await tab('Requests_section').click()
      await page.waitForURL(url => url.searchParams.get('tab') === 'requests')
      const requestsUrl = page.url()
      const logs = detail.getByRole('link', { name: t('View connection requests'), exact: true })
      assert.equal(JSON.parse(new URL(await logs.getAttribute('href'), origin).searchParams.get('channel')), '1', 'TanStack preserves a numeric-looking channel ID as a string')
      await Promise.all([
        page.waitForResponse(response => { const url = new URL(response.url()); return url.pathname === '/api/log' && url.searchParams.get('channel') === '1' }),
        logs.click(),
      ])
      await page.getByRole('heading', { name: t('Call records'), level: 1, exact: true }).waitFor()
      assert(requests.some(request => request.path === '/api/log' && request.params.channel === '1'), 'Backend reads are scoped to the selected connection')
      await page.goBack({ waitUntil: 'networkidle' })
      assert.deepEqual(connectionLocation(page.url()), connectionLocation(requestsUrl), 'Returning from requests preserves selection and filters')
      await detail.getByRole('heading', { name: longName, exact: true }).waitFor()
      await closeDetail()
      await first.waitFor()
      assert(await first.evaluate(node => node === document.activeElement), 'Return restores selected-row focus')
      assert.equal(new URL(page.url()).searchParams.get('filter'), 'Synthetic')
      assert.equal(new URL(page.url()).searchParams.get('page'), '2')
      await inventory.locator('[data-connection-link="2"]').click()
      await detail.getByRole('heading', { name: 'Synthetic subscription connection', exact: true }).waitFor()
      await detail.getByText(t('Usage sample failed'), { exact: true }).waitFor()
      assert.equal(await detail.getByText('55%', { exact: true }).count(), 0, 'Failed observations do not invent current usage')
      await capture('failed-observation')
      if ((item.language === 'en' && item.width === 1280) || (item.language === 'zh' && item.width === 390)) {
        associations.state.closed = true
        associations.state.deleted = true
        await page.goto(keyUrl, { waitUntil: 'networkidle' })
        const historicalEdit = page.getByRole('button', { name: t('Manage Key shares'), exact: true })
        await historicalEdit.waitFor()
        assert(await historicalEdit.isDisabled(), 'Closed-pool/deleted-Key history cannot be edited')
        await capture('key-history')
        associations.state.closed = false
        associations.state.deleted = false
        associations.state.usage = 'error'
        await page.goto(keyUrl, { waitUntil: 'networkidle' })
        await page.getByText(t('Unable to load window usage'), { exact: true }).waitFor()
        associations.state.usage = 'ready'
        await page.getByRole('button', { name: t('Retry'), exact: true }).click()
        await historicalEdit.waitFor()
        await page.getByRole('link', { name: t('All saved share windows'), exact: true }).click()
        const poolListUrl = page.url()
        associations.state.pools = 'empty'
        await page.goto(poolListUrl, { waitUntil: 'networkidle' })
        await page.getByText(t('No saved Key allocations for this connection.'), { exact: true }).waitFor()
        await capture('key-empty')
        associations.state.pools = 'error'
        await page.goto(poolListUrl, { waitUntil: 'networkidle' })
        await page.getByText(t('Unable to load Key shares'), { exact: true }).waitFor()
        associations.state.pools = 'ready'
        await page.getByRole('button', { name: t('Retry'), exact: true }).click()
        await savedPool.waitFor()
        deny = true
        await page.goto(keyUrl, { waitUntil: 'networkidle' })
        await page.getByText(t('Root access is required to match Key shares to this account.'), { exact: true }).waitFor()
        assert.equal(await allocations.count(), 0, 'Fresh permission denial hides saved allocation data')
        await capture('key-permission-error')
        deny = false
        // An earlier inspector link opens the connection page in place.
        await page.goto(`${origin}/connections?connection=1`, { waitUntil: 'networkidle' })
        await detail.getByRole('button', { name: t('Edit connection'), exact: true }).click()
        assert.equal(new URL(page.url()).pathname, '/connections/1', 'An earlier inspector link is redirected to the connection page')
        const editor = page.getByRole('dialog').filter({ hasText: t('Edit Channel') })
        await editor.waitFor(); await capture('edit')
        await page.keyboard.press('Escape'); await editor.waitFor({ state: 'hidden' })
        assert.equal(new URL(page.url()).pathname, '/connections/1', 'Cancel edit retains selection')
        failDetail = true
        await page.goto(`${origin}/connections/1`, { waitUntil: 'networkidle' })
        await detail.getByText(t('Unable to load this connection. It may have been removed or access may have changed.'), { exact: true }).waitFor()
        await capture('service-error')
        failDetail = false
        await detail.getByRole('button', { name: t('Retry'), exact: true }).click()
        await detail.getByRole('heading', { name: longName, exact: true }).waitFor()
        empty = true
        await page.goto(`${origin}/connections`, { waitUntil: 'networkidle' })
        await page.getByText(t('No connections yet'), { exact: true }).waitFor(); await capture('empty')
        deny = true
        await page.goto(`${origin}/connections/1`, { waitUntil: 'networkidle' })
        await page.getByText(t('Connection access could not be verified.'), { exact: true }).waitFor(); await capture('permission-error')
      }
      assert.deepEqual(errors, [], 'No runtime errors')
      assert.equal(requests.filter(request => /^\/api\/token(?:\/search|\/\d+(?:\/key)?)?$/.test(request.path)).length, 0, 'Saved relationships never fall back to a broad Key list, owner object or secret read')
      assert.deepEqual(violations, [], 'No unconfigured reads or mutations')
      assert.deepEqual(external, [], 'No provider/external requests')
      report.journeys.push({ ...item, result: 'passed', requests })
    } catch (error) { await capture('failed').catch(() => {}); report.errors.push({ ...item, error: String(error), url: page.url(), requests, responses, consoleErrors, violations, external, runtime: errors }); throw error }
    finally { await context.close(); persist() }
  }
  report.result = 'passed'
} catch (error) { report.result = 'failed'; report.errors.push(error.stack || String(error)); throw error }
finally { persist(); if (browser) await browser.close(); await new Promise(done => server.close(done)); console.log(JSON.stringify({ result: report.result, journeys: report.journeys.length, output })) }
