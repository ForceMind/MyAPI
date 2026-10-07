// Production assets in real Chromium; explicit synthetic APIs only.
// Browser execution belongs in CI. This harness never contacts an upstream,
// signs into a real account, submits a business mutation, or validates live bills.
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
const output = resolve(process.env.MYAPI_BROWSER_ARTIFACTS || `${repo}/.local-tests/ui-browser`)
const languages = { en: 'en', zh: 'zhCN', 'zh-TW': 'zhTW', fr: 'fr', ru: 'ru', ja: 'ja', vi: 'vi' }
const translations = Object.fromEntries(Object.keys(languages).map(language => [language, JSON.parse(readFileSync(resolve(repo, `web/src/i18n/locales/${language}.json`), 'utf8')).translation]))
const label = (key, language = 'en') => translations[language][key] || key
const settings = ['site', 'auth', 'billing', 'models', 'security', 'content', 'operations'].flatMap(family => {
  const source = readFileSync(resolve(repo, `web/src/features/system-settings/${family}/section-registry.tsx`), 'utf8')
  return [...source.matchAll(/\bid:\s*'([^']+)',\s*titleKey:\s*'([^']+)'/g)].map(([, id, title]) => ({ family, id, title, path: `/system-settings/${family}/${id}` }))
})
assert.equal(new Set(settings.map(section => section.path)).size, settings.length, 'settings deep links must be unique')
assert(settings.length > 0, 'read real section registries before launching')
assert(existsSync(resolve(root, 'index.html')), 'Build the production frontend before qualification')
mkdirSync(output, { recursive: true })
const report = {
  schema: 1,
  evidence: 'Production-build Chromium UI with isolated synthetic data. Not live-account, provider, billing, deployment, or production acceptance.',
  commit: process.env.GITHUB_SHA || execFileSync('git', ['rev-parse', 'HEAD'], { cwd: repo, encoding: 'utf8' }).trim(),
  fixtureTime: new Date(FIXTURE_TIME).toISOString(),
  result: 'running', journeys: [], screenshots: [], contrast: [], requests: [], blockedExternal: [], violations: [], pageErrors: [],
  settings: settings.map(section => ({ ...section, reached: false })),
}
const persist = () => writeFileSync(resolve(output, 'qualification.json'), JSON.stringify(report, null, 2))
const mime = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.png': 'image/png', '.ico': 'image/x-icon', '.woff2': 'font/woff2' }
const server = createServer((request, response) => {
  let path
  try { path = resolve(root, `.${decodeURIComponent(new URL(request.url || '/', 'http://localhost').pathname)}`) }
  catch { response.writeHead(400).end(); return }
  if (!path.startsWith(`${root}${sep}`) && path !== root) { response.writeHead(403).end(); return }
  if (!existsSync(path) || statSync(path).isDirectory()) path = resolve(root, 'index.html')
  response.writeHead(200, { 'content-type': mime[extname(path)] || 'application/octet-stream', 'cache-control': 'no-store' })
  createReadStream(path).pipe(response)
})
let browser, origin, activePage, journeyName
const contexts = new Set()

async function session(options = {}) {
  const language = options.language || 'en'
  const theme = options.theme || 'light'
  const fixture = createUIFixture({ ...options, language: languages[language] })
  let releaseOptions
  const optionsGate = options.deferOptions ? new Promise(done => { releaseOptions = done }) : null
  const context = await browser.newContext({ viewport: { width: options.width || 1280, height: 900 }, locale: 'en-US', timezoneId: 'UTC', reducedMotion: 'reduce', colorScheme: theme, serviceWorkers: 'block', hasTouch: options.hasTouch ?? false, isMobile: options.isMobile ?? false })
  contexts.add(context)
  await context.addCookies([{ name: 'vite-ui-theme', value: theme, url: origin }, { name: 'sidebar_state', value: 'true', url: origin }])
  await context.addInitScript(languageCode => { localStorage.setItem('i18nextLng', languageCode) }, languages[language])
  await context.route('**/*', async route => {
    const request = route.request()
    const url = new URL(request.url())
    if (url.origin !== origin) {
      if (url.protocol === 'data:' || url.protocol === 'blob:') return route.continue()
      report.blockedExternal.push({ journey: journeyName, url: url.href })
      return route.abort('blockedbyclient')
    }
    if (!url.pathname.startsWith('/api/')) {
      if (request.method() !== 'GET' || ['xhr', 'fetch'].includes(request.resourceType())) {
        report.violations.push({ journey: journeyName, reason: `Unexpected same-origin service request: ${request.method()} ${url.pathname}` })
        return route.fulfill({ status: 501, json: { success: false } })
      }
      return route.continue()
    }
    if (optionsGate && url.pathname.replace(/\/$/, '') === '/api/option') await optionsGate
    const response = fixture.response(url, request.method())
    report.requests.push({ journey: journeyName, role: options.role ?? 100, method: request.method(), path: url.pathname, query: url.search, status: response.status || 501 })
    if (response.violation) report.violations.push({ journey: journeyName, reason: response.violation })
    await route.fulfill({ status: response.status || 501, json: response.body || { success: false, message: response.violation } })
  })
  await context.routeWebSocket('**/*', socket => {
    report.violations.push({ journey: journeyName, reason: `Unexpected WebSocket: ${socket.url()}` })
    socket.close()
  })
  const page = await context.newPage()
  activePage = page
  page.setDefaultTimeout(15000)
  await page.clock.setFixedTime(new Date(FIXTURE_TIME))
  page.on('pageerror', error => report.pageErrors.push({ journey: journeyName, message: error.message }))
  return { page, context, fixture, language, releaseOptions }
}

async function settled(page) {
  await page.evaluate(async () => {
    await document.fonts.ready
    await Promise.all(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished.catch(() => {})))
    await new Promise(done => requestAnimationFrame(() => requestAnimationFrame(done)))
  })
}

async function open(page, path, { shell = true, title } = {}) {
  await page.goto(`${origin}${path}`, { waitUntil: 'networkidle' })
  if (shell) {
    await page.locator('[data-myapi-shell="authenticated"]').waitFor()
    await page.locator('main#content').waitFor()
    await page.locator('main#content h1').first().waitFor()
  }
  if (title) await page.getByRole('heading', { level: 1, name: title, exact: true }).waitFor()
  await settled(page)
}

async function screenshot(page, name, { shell = true, touch = false } = {}) {
  await settled(page)
  const geometry = await page.evaluate(authenticated => {
    const rect = element => {
      if (!element) return null
      const box = element.getBoundingClientRect()
      return { x: box.x, y: box.y, right: box.right, bottom: box.bottom, width: box.width, height: box.height, scroll: element.scrollWidth, client: element.clientWidth }
    }
    const header = document.querySelector('[data-myapi-header]')
    const main = document.querySelector('main#content')
    const title = main?.querySelector('h1')
    const controls = authenticated ? [...header.querySelectorAll('button')].filter(element => element.getBoundingClientRect().width > 0).map(element => ({ name: element.getAttribute('aria-label') || element.textContent, ...rect(element) })) : []
    const actions = authenticated ? [...document.querySelectorAll('.myapi-page-actions button')].filter(element => element.getBoundingClientRect().width > 0).map(element => ({ name: element.getAttribute('aria-label') || element.textContent, ...rect(element), whiteSpace: getComputedStyle(element).whiteSpace })) : []
    return { viewport: { width: innerWidth, height: innerHeight }, documentWidth: document.documentElement.scrollWidth, coarsePointer: matchMedia('(pointer: coarse)').matches, header: rect(header), main: rect(main), title: rect(title), controls, actions, theme: document.documentElement.classList.contains('dark') ? 'dark' : 'light' }
  }, shell)
  assert(geometry.documentWidth <= geometry.viewport.width + 1, `${name}: page-wide horizontal overflow: ${JSON.stringify(geometry)}`)
  if (shell) {
    assert(geometry.main && geometry.header && geometry.title, `${name}: authenticated semantic shell`)
    assert(geometry.main.y >= geometry.header.bottom - 1, `${name}: main is not covered by the actual header`)
    assert(geometry.main.x >= -1 && geometry.main.right <= geometry.viewport.width + 1, `${name}: main stays in the viewport`)
    assert(geometry.main.height > 100 && geometry.title.height > 0, `${name}: readable main and title`)
    assert(geometry.title.x >= geometry.main.x - 1 && geometry.title.right <= geometry.main.right + 1, `${name}: task title fits the main column`)
    for (const control of geometry.controls) assert(control.x >= -1 && control.right <= geometry.viewport.width + 1 && control.y >= geometry.header.y - 1 && control.bottom <= geometry.header.bottom + 1, `${name}: header control remains reachable: ${JSON.stringify(control)}`)
    for (const action of geometry.actions) assert(action.x >= geometry.main.x - 1 && action.right <= geometry.main.right + 1 && action.scroll <= action.client + 1 && action.whiteSpace === 'normal', `${name}: translated page action wraps and fits: ${JSON.stringify(action)}`)
    if (touch) {
      assert.equal(geometry.coarsePointer, true, `${name}: browser really reports a coarse pointer`)
      assert(geometry.controls.length > 0, `${name}: measure visible header controls`)
      for (const control of geometry.controls) assert(control.width >= 44 && control.height >= 44, `${name}: header touch target must be at least 44px in each dimension: ${JSON.stringify(control)}`)
    }
  }
  const file = `${name}.png`
  await page.screenshot({ path: resolve(output, file) })
  report.screenshots.push({ file, journey: journeyName, path: new URL(page.url()).pathname, geometry })
  persist()
}

async function checkTextContrast(locator, name, theme) {
  report.contrast.push(await assertTextContrast(locator, name, theme))
}

async function run(name, work) {
  journeyName = name
  activePage = null
  const startViolations = report.violations.length
  const startErrors = report.pageErrors.length
  const result = { name, result: 'running' }
  report.journeys.push(result)
  try {
    await work()
    assert.equal(report.violations.length, startViolations, JSON.stringify(report.violations.slice(startViolations)))
    assert.equal(report.pageErrors.length, startErrors, JSON.stringify(report.pageErrors.slice(startErrors)))
    result.result = 'passed'
  } catch (error) {
    result.result = 'failed'
    result.error = error.stack || String(error)
    if (activePage && !activePage.isClosed()) {
      await activePage.screenshot({ path: resolve(output, `failure-${name}.png`) }).catch(() => {})
      result.pageText = (await activePage.locator('body').innerText().catch(() => '')).slice(-7000)
    }
    console.error(`${name}: ${result.error}`)
  } finally {
    for (const context of contexts) await context.close()
    contexts.clear()
    persist()
  }
}

try {
  await new Promise(done => server.listen(0, '127.0.0.1', done))
  origin = `http://127.0.0.1:${server.address().port}`
  const driver = process.env.MYAPI_PLAYWRIGHT_MODULE
  const { chromium } = await import(driver ? pathToFileURL(resolve(driver)).href : 'playwright')
  browser = await chromium.launch({ headless: true, executablePath: process.env.MYAPI_CHROMIUM_PATH || undefined, args: ['--disable-dev-shm-usage'] })
  report.browser = browser.version()

  await run('existing-page-families', async () => {
    const { page } = await session()
    const pages = [
      ['overview', '/dashboard/overview'], ['channels', '/channels'], ['keys', '/keys'],
      ['users', '/users'], ['usage', '/usage-logs/common'], ['task-logs', '/usage-logs/task'],
      ['drawing-logs', '/usage-logs/drawing'], ['models', '/models/metadata'],
      ['content-logs', '/full-content-logs'], ['profile', '/profile'],
      ['funding-history', '/wallet'], ['subscriptions', '/subscriptions'],
      ['redemption-history', '/redemption-codes'], ['system-info', '/system-info'],
    ]
    for (const [name, path] of pages) {
      await open(page, path)
      for (const width of [320, 1280]) {
        await page.setViewportSize({ width, height: 900 })
        await screenshot(page, `${name}-${width}`)
      }
    }
    await open(page, '/wallet', { title: label('Funding history') })
    assert.equal(await page.locator('#wallet-add-funds').count(), 0, 'disabled commerce cannot expose top-up controls')
    await page.getByRole('button', { name: label('Billing History'), exact: true }).click()
    const history = page.getByRole('dialog')
    await history.getByText('synthetic-old-order', { exact: true }).waitFor()
    await screenshot(page, 'disabled-commerce-preserves-history')
    await page.keyboard.press('Escape')
    await history.waitFor({ state: 'hidden' })
  })

  await run('settings-deep-links', async () => {
    const { page } = await session()
    for (const section of report.settings) {
      await open(page, section.path, { title: label(section.title) })
      assert.equal(new URL(page.url()).pathname, section.path, `registered settings section ${section.path} is reachable`)
      assert.equal(await page.getByText(label('Unable to load settings'), { exact: true }).count(), 0, `${section.path} does not silently become an error surface`)
      if (section.path === '/system-settings/billing/payment') {
        const mode = page.getByRole('combobox', { name: label('User funding mode'), exact: true })
        await mode.waitFor()
        assert.equal(await mode.getAttribute('data-slot'), 'select-trigger', 'funding mode is read from the real form select')
        assert.equal((await mode.innerText()).trim(), label('Disabled'), 'saved disabled funding mode is not replaced by the enabled default')
      }
      section.reached = true
      if (settings.find(candidate => candidate.family === section.family).id === section.id) {
        await page.setViewportSize({ width: 320, height: 900 })
        await screenshot(page, `settings-${section.family}-320`)
        await page.setViewportSize({ width: 1280, height: 900 })
      }
    }
  })

  await run('role-and-preference-boundaries', async () => {
    for (const options of [{ role: 1 }, { role: 10 }, { role: 10, sidebar: true }]) {
      const { page } = await session(options)
      await open(page, '/dashboard/overview')
      const sidebar = page.locator('[data-myapi-sidebar]')
      for (const href of ['/system-settings/site', '/system-info']) assert.equal(await sidebar.locator(`a[href="${href}"]`).count(), 0, 'Root destinations hidden from lower roles')
      if (options.role === 1) for (const href of ['/channels', '/users', '/models/metadata']) assert.equal(await sidebar.locator(`a[href="${href}"]`).count(), 0, 'ordinary users see no admin destinations')
      await page.getByRole('button', { name: label('Search'), exact: true }).click()
      const command = page.getByRole('dialog')
      await command.getByRole('combobox').waitFor()
      for (const title of ['System Settings', 'System Info', ...(options.role === 1 ? ['Channels', 'Users', 'Models'] : []), ...(options.sidebar ? ['API Keys', 'Usage Logs'] : [])]) {
        assert.equal(await command.getByRole('option', { name: label(title), exact: true }).count(), 0, `command honors role/sidebar narrowing: ${title}`)
      }
      if (options.sidebar) {
        assert.equal(await sidebar.locator('a[href="/keys"], a[href="/usage-logs/common"]').count(), 0, 'both admin and user sidebar preferences narrow navigation')
        assert.equal(await command.getByRole('option', { name: label('Channels'), exact: true }).count(), 1, 'allowed admin commands remain available')
      }
      await screenshot(page, `navigation-role-${options.role}${options.sidebar ? '-narrowed' : ''}`)
      await page.keyboard.press('Escape')
      await command.waitFor({ state: 'hidden' })
      for (const path of ['/system-settings/site/system-info', '/system-info', ...(options.role === 1 ? ['/channels', '/users', '/models/metadata'] : [])]) {
        await open(page, path, { shell: false })
        await page.waitForURL(`${origin}/403`)
        await page.getByRole('heading', { name: label('Access Forbidden'), exact: true }).waitFor()
      }
      await page.context().close(); contexts.delete(page.context())
    }
  })

  await run('keyboard-drawers-and-history', async () => {
    const { page } = await session({ width: 320 })
    await open(page, '/keys')
    await page.keyboard.press('Tab')
    assert.equal(await page.locator('a[href="#content"]').evaluate(element => element === document.activeElement), true, 'first keyboard stop is skip navigation')
    await page.keyboard.press('Enter')
    await page.waitForFunction(() => document.activeElement?.id === 'content')
    const trigger = page.getByRole('button', { name: label('Toggle sidebar'), exact: true })
    for (const closeWithEscape of [false, true]) {
      await trigger.click()
      const drawer = page.getByRole('dialog', { name: label('Navigation'), exact: true })
      await drawer.waitFor()
      assert.equal(await trigger.getAttribute('aria-expanded'), 'true')
      if (closeWithEscape) await page.keyboard.press('Escape')
      else await drawer.getByRole('button', { name: label('Close'), exact: true }).click()
      await drawer.waitFor({ state: 'hidden' })
      await page.waitForFunction(() => document.activeElement?.getAttribute('data-sidebar') === 'trigger')
      assert.equal(await trigger.getAttribute('aria-expanded'), 'false')
    }
    await trigger.click()
    await page.getByRole('dialog', { name: label('Navigation'), exact: true }).locator('a[href="/profile"]').click()
    await page.waitForURL(`${origin}/profile`)
    await page.getByRole('dialog', { name: label('Navigation'), exact: true }).waitFor({ state: 'hidden' })
    await page.goBack({ waitUntil: 'networkidle' }); await page.waitForURL(url => url.pathname === '/keys')
    await trigger.click()
    await page.goForward({ waitUntil: 'networkidle' }); await page.waitForURL(`${origin}/profile`)
    await page.getByRole('dialog', { name: label('Navigation'), exact: true }).waitFor({ state: 'hidden' })
    await open(page, '/keys')
    const search = page.getByRole('button', { name: label('Search'), exact: true })
    await search.focus(); await page.keyboard.press('Control+k')
    const command = page.getByRole('dialog')
    await command.getByRole('combobox').fill(label('Profile'))
    await command.getByRole('option', { name: label('Profile'), exact: true }).click()
    await page.waitForURL(`${origin}/profile`)
    await command.waitFor({ state: 'hidden' })
    await page.goBack({ waitUntil: 'networkidle' })
    await page.waitForURL(url => url.pathname === '/keys')
    const create = page.getByRole('button', { name: label('Create API Key'), exact: true })
    await create.click()
    const editor = page.getByRole('dialog')
    await editor.getByRole('textbox', { name: label('Name'), exact: true }).fill('unsaved-synthetic-key')
    await screenshot(page, 'key-drawer-320')
    await editor.locator('[data-slot="sheet-footer"]').getByRole('button', { name: label('Close'), exact: true }).click()
    await editor.waitFor({ state: 'hidden' })
    await page.waitForFunction(text => document.activeElement?.textContent?.includes(text), label('Create API Key'))
    await create.click()
    assert.equal(await editor.getByRole('textbox', { name: label('Name'), exact: true }).inputValue(), '', 'closing the editor discards its unsaved draft')
    await page.keyboard.press('Escape'); await editor.waitFor({ state: 'hidden' })
  })

  await run('channel-task-navigation', async () => {
    const { page } = await session({ width: 320 })
    await open(page, '/channels')
    const initialURL = page.url()
    const model = page.getByLabel(label('Routing preview model'), { exact: true })
    await page.getByRole('navigation', { name: label('On this page'), exact: true }).locator('a[href="#channel-routing"]').click()
    await page.waitForFunction(() => document.activeElement?.id === 'channel-routing')
    await model.fill('synthetic-unsaved-preview')
    for (const target of ['channel-inventory', 'channel-evidence', 'channel-routing']) {
      await page.getByRole('navigation', { name: label('On this page'), exact: true }).locator(`a[href="#${target}"]`).click()
      await page.waitForFunction(id => document.activeElement?.id === id, target)
      assert.equal(page.url(), initialURL, 'task links preserve URL and browser history')
    }
    assert.equal(await model.inputValue(), 'synthetic-unsaved-preview', 'task navigation does not remount or discard in-progress preview input')
    await screenshot(page, 'channel-task-routing-320')
    await open(page, '/channels?quotaChannelId=1')
    const evidence = await page.locator('#channel-evidence').boundingBox()
    const main = await page.locator('main#content').boundingBox()
    assert(evidence && main && evidence.y < main.y + main.height && evidence.y + evidence.height > main.y, 'old quotaChannelId entry scrolls to preserved account evidence')
  })

  await run('responsive-themes-and-seven-languages', async () => {
    for (const theme of ['light', 'dark']) {
      const { page } = await session({ theme, longText: true })
      await open(page, '/keys')
      for (const width of [320, 768, 1280]) {
        await page.setViewportSize({ width, height: 900 })
        assert.equal(await page.locator('html').evaluate(element => element.classList.contains('dark')), theme === 'dark', 'the real stored theme is applied')
        await screenshot(page, `shell-long-text-${theme}-${width}`)
      }
      await checkTextContrast(page.locator('.myapi-page-header p').first(), 'page supporting text', theme)
      await checkTextContrast(page.getByRole('button', { name: label('Create API Key'), exact: true }), 'primary action text', theme)
      await page.reload({ waitUntil: 'networkidle' })
      assert.equal(await page.locator('html').evaluate(element => element.classList.contains('dark')), theme === 'dark', 'theme survives reload')
      await page.context().close(); contexts.delete(page.context())
    }
    const touchSession = await session({ language: 'fr', width: 320, hasTouch: true, isMobile: true })
    const touchPage = touchSession.page
    for (const width of [320, 768]) {
      await touchPage.setViewportSize({ width, height: 900 })
      await open(touchPage, '/channels')
      assert.equal(await touchPage.evaluate(() => matchMedia('(pointer: coarse)').matches), true, `${width}px session must exercise coarse-pointer CSS`)
      await screenshot(touchPage, `touch-channels-${width}`, { touch: true })
      await touchPage.getByRole('button', { name: label('Search', 'fr'), exact: true }).tap()
      const command = touchPage.getByRole('dialog')
      await command.getByRole('combobox').waitFor()
      await touchPage.keyboard.press('Escape')
      await command.waitFor({ state: 'hidden' })
      if (width === 320) {
        await touchPage.getByRole('button', { name: label('Toggle sidebar', 'fr'), exact: true }).tap()
        const drawer = touchPage.getByRole('dialog', { name: label('Navigation', 'fr'), exact: true })
        await drawer.getByRole('button', { name: label('Close', 'fr'), exact: true }).tap()
        await drawer.waitFor({ state: 'hidden' })
      }
      await open(touchPage, '/system-settings/billing/payment', { title: label('Payment Gateway', 'fr') })
      const save = touchPage.getByRole('button', { name: label('Save all settings', 'fr'), exact: true })
      await save.waitFor()
      assert.equal((await save.innerText()).trim(), label('Save all settings', 'fr'), 'the long translated settings action is rendered in full')
      await screenshot(touchPage, `touch-long-settings-action-${width}`, { touch: true })
    }
    await touchSession.context.close(); contexts.delete(touchSession.context)
    for (const language of Object.keys(languages)) {
      const { page } = await session({ language, width: 320, longText: true })
      await open(page, '/system-settings/models/openai-pricing-source', { title: label('OpenAI official pricing source', language) })
      await screenshot(page, `language-${language}-settings-320`)
      await page.setViewportSize({ width: 768, height: 900 })
      await screenshot(page, `language-${language}-settings-768`)
      await page.context().close(); contexts.delete(page.context())
    }
  })

  await run('public-auth-setup-and-errors', async () => {
    const { page } = await session({ role: 0, width: 320 })
    for (const [name, path] of [['home', '/'], ['about', '/about'], ['pricing', '/pricing'], ['rankings', '/rankings'], ['privacy', '/privacy-policy'], ['agreement', '/user-agreement'], ['sign-in', '/sign-in'], ['sign-up', '/sign-up'], ['forgot-password', '/forgot-password'], ['reset', '/reset'], ...['401', '403', '404', '500', '503'].map(code => [`error-${code}`, `/${code}`])]) {
      await open(page, path, { shell: false })
      await page.locator('h1, h2').first().waitFor()
      await screenshot(page, `public-${name}-320`, { shell: false })
    }
    await open(page, '/keys', { shell: false })
    await page.waitForURL(url => url.pathname === '/sign-in' && url.searchParams.get('redirect') === '/keys')
    await page.getByRole('textbox', { name: label('Username or Email'), exact: true }).fill('synthetic-interrupted-user')
    await page.getByRole('link', { name: label('Forgot password?'), exact: true }).click()
    await page.waitForURL(`${origin}/forgot-password`)
    await page.goBack({ waitUntil: 'networkidle' })
    await page.waitForURL(url => url.pathname === '/sign-in' && url.searchParams.get('redirect') === '/keys')
    assert.equal(await page.locator('[data-myapi-shell="authenticated"]').count(), 0, 'interrupted authentication never exposes private shell')
    const setup = await session({ role: 0, setupComplete: false, width: 320 })
    await open(setup.page, '/', { shell: false })
    await setup.page.waitForURL(`${origin}/setup`)
    await setup.page.getByText(label('Detected database'), { exact: true }).waitFor()
    await setup.page.getByRole('button', { name: label('Next'), exact: true }).click()
    await setup.page.getByRole('button', { name: label('Back'), exact: true }).waitFor()
    await screenshot(setup.page, 'setup-interrupted-320', { shell: false })
    await setup.page.getByRole('button', { name: label('Back'), exact: true }).click()
    await setup.page.getByText(label('Detected database'), { exact: true }).waitFor()
    assert.equal(report.requests.filter(request => request.method !== 'GET' && request.path !== '/api/user/auth/refresh').length, 0, 'qualification sends no business mutations')
  })

  await run('settings-loading-error-recovery', async () => {
    const { page, fixture, releaseOptions } = await session({ width: 320, deferOptions: true })
    try {
      await page.goto(`${origin}/system-settings/site/system-info`, { waitUntil: 'domcontentloaded' })
      const main = page.locator('main#content')
      await main.getByRole('status').waitFor()
      assert.equal(await main.locator('form').count(), 0, 'loading does not expose default-valued editable settings')
      await screenshot(page, 'settings-loading-320')
      fixture.state.options = 'error'
      releaseOptions()
      const failure = main.getByRole('alert').filter({ hasText: label('Unable to load settings') })
      await failure.waitFor()
      assert.equal(await main.locator('form').count(), 0, 'unavailable settings do not become an editable success state')
      await screenshot(page, 'settings-unavailable-320')
      fixture.state.options = 'ready'
      await failure.getByRole('button', { name: label('Retry'), exact: true }).click()
      await failure.waitFor({ state: 'hidden' })
      await main.locator('form').first().waitFor()
      await screenshot(page, 'settings-recovered-320')
    } finally { releaseOptions() }
  })

  await run('empty-state', async () => {
    const { page, fixture } = await session({ width: 320 })
    fixture.state.keys = 'empty'
    await open(page, '/keys')
    await page.getByText(label('No API Keys Found'), { exact: true }).waitFor()
    await screenshot(page, 'keys-empty-320')
    fixture.state.keys = 'populated'
    await page.reload({ waitUntil: 'networkidle' })
    await page.getByText('synthetic-key', { exact: true }).first().waitFor()
    assert.equal(await page.getByText(label('No API Keys Found'), { exact: true }).count(), 0, 'fresh list replaces the empty state')
  })
  report.result = report.journeys.every(journey => journey.result === 'passed') ? 'passed' : 'failed'
  assert.equal(report.result, 'passed', `UI qualification failed; inspect ${output}/qualification.json and failure screenshots`)
  console.log(`UI qualification passed: ${report.journeys.length} bounded journeys, ${report.settings.filter(section => section.reached).length} registered settings deep links, ${report.screenshots.length} screenshots. Synthetic-only evidence.`)
} finally {
  if (report.result === 'running') report.result = 'blocked'
  persist()
  if (browser) await browser.close()
  await new Promise(done => server.close(done))
}
