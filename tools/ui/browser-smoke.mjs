// Production assets in real Chromium; explicit synthetic APIs only.
// Browser execution belongs in CI. This harness never contacts an upstream,
// signs into a real account, submits a business mutation, or validates live bills.
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { createReadStream, existsSync, mkdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { createServer } from 'node:http'
import { extname, resolve, sep } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { createUIFixture, FIXTURE_TIME, SETTLEMENT_REVIEW_IDS, SETTLEMENT_REVIEW_METADATA, SETTLEMENT_REVIEW_DIAGNOSTIC } from './browser-fixtures.mjs'
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
  commit: execFileSync('git', ['rev-parse', 'HEAD'], { cwd: repo, encoding: 'utf8' }).trim(),
  workflowCommit: process.env.GITHUB_SHA || null,
  worktreeDirty: !!execFileSync('git', ['status', '--porcelain', '--untracked-files=no'], { cwd: repo, encoding: 'utf8' }).trim(),
  fixtureTime: new Date(FIXTURE_TIME).toISOString(),
  result: 'running', journeys: [], screenshots: [], contrast: [], settlementLayouts: [], requests: [], blockedExternal: [], violations: [], pageErrors: [],
  settings: settings.map(section => ({ ...section, reached: false })),
}
const persist = () => {
  report.totals = {
    journeys: report.journeys.length, screenshots: report.screenshots.length,
    settlementJourneys: report.journeys.filter(journey => journey.name.startsWith('settlement-')).length,
    settlementScreenshots: report.screenshots.filter(screenshot => screenshot.journey.startsWith('settlement-')).length,
  }
  writeFileSync(resolve(output, 'qualification.json'), JSON.stringify(report, null, 2))
}
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
  let releaseLearning
  const learningGate = options.deferLearning ? new Promise(done => { releaseLearning = done }) : null
  const learningReads = new Set(['/api/user/prompt-learning', '/api/user/prompt-learning/versions', '/api/user/prompt-learning/runs'])
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
    if (!url.pathname.startsWith('/api/') && !url.pathname.startsWith('/pg/')) {
      if (request.method() !== 'GET' || ['xhr', 'fetch'].includes(request.resourceType())) {
        report.violations.push({ journey: journeyName, reason: `Unexpected same-origin service request: ${request.method()} ${url.pathname}` })
        return route.fulfill({ status: 501, json: { success: false } })
      }
      return route.continue()
    }
    if (optionsGate && url.pathname.replace(/\/$/, '') === '/api/option') await optionsGate
    if (learningGate && learningReads.has(url.pathname.replace(/\/$/, ''))) await learningGate
    const response = fixture.response(url, request.method())
    report.requests.push({ journey: journeyName, pagePath: new URL(request.frame().url()).pathname, role: options.role ?? 100, method: request.method(), path: url.pathname, query: url.search, status: response.status || 501 })
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
  if (options.runningClock) await page.clock.install({ time: new Date(FIXTURE_TIME) })
  else await page.clock.setFixedTime(new Date(FIXTURE_TIME))
  page.on('pageerror', error => report.pageErrors.push({ journey: journeyName, message: error.message }))
  return { page, context, fixture, language, releaseOptions, releaseLearning }
}

async function settled(page) {
  await page.evaluate(async () => {
    await document.fonts.ready
    await Promise.all(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished.catch(() => {})))
    await new Promise(done => requestAnimationFrame(() => requestAnimationFrame(done)))
  })
}

async function open(page, path, { shell = true, title, headingLevel = 1, expectedPath } = {}) {
  await page.goto(`${origin}${path}`, { waitUntil: 'networkidle' })
  if (expectedPath) await page.waitForURL(url => url.pathname.replace(/\/$/, '') === expectedPath.replace(/\/$/, ''))
  if (shell) {
    await page.locator('[data-myapi-shell="authenticated"]').waitFor()
    await page.locator('main#content').waitFor()
    await page.locator(`main#content h${headingLevel}`).first().waitFor()
  }
  if (title) await page.getByRole('heading', { level: headingLevel, name: title, exact: true }).waitFor()
  await settled(page)
}

async function screenshot(page, name, { shell = true, touch = false, headingLevel = 1 } = {}) {
  await settled(page)
  const geometry = await page.evaluate(({ authenticated, headingLevel }) => {
    const rect = element => {
      if (!element) return null
      const box = element.getBoundingClientRect()
      return { x: box.x, y: box.y, right: box.right, bottom: box.bottom, width: box.width, height: box.height, scroll: element.scrollWidth, client: element.clientWidth }
    }
    const header = document.querySelector('[data-myapi-header]')
    const main = document.querySelector('main#content')
    const title = main?.querySelector(`h${headingLevel}`)
    const controls = authenticated ? [...header.querySelectorAll('button')].filter(element => element.getBoundingClientRect().width > 0).map(element => ({ name: element.getAttribute('aria-label') || element.textContent, ...rect(element) })) : []
    const actions = authenticated ? [...document.querySelectorAll('.myapi-page-actions button')].filter(element => element.getBoundingClientRect().width > 0).map(element => ({ name: element.getAttribute('aria-label') || element.textContent, text: element.innerText.trim(), ...rect(element), whiteSpace: getComputedStyle(element).whiteSpace })) : []
    return { viewport: { width: innerWidth, height: innerHeight }, documentWidth: document.documentElement.scrollWidth, coarsePointer: matchMedia('(pointer: coarse)').matches, header: rect(header), main: rect(main), title: rect(title), controls, actions, theme: document.documentElement.classList.contains('dark') ? 'dark' : 'light' }
  }, { authenticated: shell, headingLevel })
  assert(geometry.documentWidth <= geometry.viewport.width + 1, `${name}: page-wide horizontal overflow: ${JSON.stringify(geometry)}`)
  if (shell) {
    assert(geometry.main && geometry.header && geometry.title, `${name}: authenticated semantic shell`)
    assert(geometry.main.y >= geometry.header.bottom - 1, `${name}: main is not covered by the actual header`)
    assert(geometry.main.x >= -1 && geometry.main.right <= geometry.viewport.width + 1, `${name}: main stays in the viewport`)
    assert(geometry.main.height > 100 && geometry.title.height > 0, `${name}: readable main and title`)
    assert(geometry.title.x >= geometry.main.x - 1 && geometry.title.right <= geometry.main.right + 1, `${name}: task title fits the main column`)
    for (const control of geometry.controls) assert(control.x >= -1 && control.right <= geometry.viewport.width + 1 && control.y >= geometry.header.y - 1 && control.bottom <= geometry.header.bottom + 1, `${name}: header control remains reachable: ${JSON.stringify(control)}`)
    for (const action of geometry.actions) assert(action.x >= geometry.main.x - 1 && action.right <= geometry.main.right + 1 && action.scroll <= action.client + 1 && (!action.text || action.whiteSpace === 'normal'), `${name}: translated page action wraps and fits: ${JSON.stringify(action)}`)
    if (touch) {
      assert.equal(geometry.coarsePointer, true, `${name}: browser really reports a coarse pointer`)
      assert(geometry.controls.length > 0, `${name}: measure visible header controls`)
      for (const control of geometry.controls) assert(control.width >= 44 && control.height >= 44, `${name}: header touch target must be at least 44px in each dimension: ${JSON.stringify(control)}`)
      for (const action of geometry.actions) assert(action.height >= 44, `${name}: page action touch target must be at least 44px high: ${JSON.stringify(action)}`)
    }
  }
  const file = `${name}.png`
  await page.screenshot({ path: resolve(output, file) })
  report.screenshots.push({ file, commit: report.commit, journey: journeyName, path: new URL(page.url()).pathname, geometry })
  persist()
}

async function checkTextContrast(locator, name, theme) {
  report.contrast.push(await assertTextContrast(locator, name, theme))
}

async function openSettlementLog(page, requestId, language = 'en') {
  await page.getByTitle(label('Click to view full details', language), { exact: true }).filter({ hasText: requestId, visible: true }).click()
  const dialog = page.getByRole('dialog')
  const panel = dialog.getByRole('region', { name: label('Usage pending review', language), exact: true })
  await panel.waitFor()
  return { dialog, panel }
}

function assertSettlementReadOnly(start) {
  assert.equal(report.requests.slice(start).filter(request => request.method !== 'GET' && request.path !== '/api/user/auth/refresh').length, 0, 'opening, refreshing, expanding, typing and closing never reconcile or recover a request')
}

async function assertAutomaticSettlement(panel, index, language = 'en') {
  const title = ['Automatic settlement pending', 'Settlement applied; record finalization pending'][index]
  const description = [
    'This request already has an automatic settlement record. Manual input is unavailable here.',
    'The verified usage has been settled. The request record still needs finalization.',
  ][index]
  for (const key of [title, description]) {
    assert.equal(typeof translations[language][key], 'string', `${language}: settlement status has an explicit translation`)
    assert(translations[language][key].trim(), `${language}: settlement status translation is not empty`)
    if (language !== 'en') assert.notEqual(translations[language][key], key, `${language}: status matrix cannot silently use English fallback`)
  }
  const status = panel.getByRole('status')
  await status.getByText(label(title, language), { exact: true }).waitFor()
  await status.getByText(label(description, language), { exact: true }).waitFor()
  assert.equal(await panel.locator('form, input, textarea, [role="checkbox"]').count(), 0, 'automatic status cannot expose any manual amount, evidence or confirmation control')
  for (const key of ['Advanced: manual reconciliation', 'Confirm reconciliation']) assert.equal(await panel.getByRole('button', { name: label(key, language), exact: true }).count(), 0, key)
  if (index === 0) assert.equal(await panel.locator('p').filter({ hasText: label('Confirmed quota (internal units)', language) }).count(), 0, 'unknown actual usage is never displayed as a confirmed amount, including zero')
  else await panel.getByText(`${label('Confirmed quota (internal units)', language)}: 60`, { exact: true }).waitFor()
  return status
}

async function measureSettlementLayout(panel, name) {
  const status = panel.getByRole('status')
  await status.scrollIntoViewIfNeeded()
  await settled(panel.page())
  const geometry = await status.evaluate((element, name) => {
    const bounds = element.getBoundingClientRect()
    const dialog = element.closest('[role="dialog"]')
    const box = dialog.getBoundingClientRect()
    const paragraphs = [...element.querySelectorAll('p')].map(paragraph => {
      const rect = paragraph.getBoundingClientRect()
      const range = document.createRange()
      range.selectNodeContents(paragraph)
      const lines = [...range.getClientRects()].filter(line => line.width > 0 && line.height > 0)
      return { text: paragraph.textContent, width: rect.width, height: rect.height, unclipped: paragraph.scrollWidth <= paragraph.clientWidth + 1 && paragraph.scrollHeight <= paragraph.clientHeight + 1 && getComputedStyle(paragraph).textOverflow !== 'ellipsis' && lines.length > 0 && lines.every(line => line.left >= bounds.left - 1 && line.right <= bounds.right + 1 && line.top >= rect.top - 1 && line.bottom <= rect.bottom + 1) }
    })
    const scrollers = []
    let verticallyReachable = true
    for (let parent = element.parentElement; parent; parent = parent.parentElement) {
      const rect = parent.getBoundingClientRect()
      const overflow = getComputedStyle(parent).overflowY
      if (['auto', 'scroll', 'hidden', 'clip'].includes(overflow)) {
        verticallyReachable &&= bounds.top >= rect.top - 1 && bounds.bottom <= rect.bottom + 1
        if (['auto', 'scroll'].includes(overflow)) {
          const previous = parent.scrollTop
          parent.scrollTop = parent.scrollHeight
          scrollers.push({ client: parent.clientHeight, scroll: parent.scrollHeight, reachableBottom: parent.scrollHeight - parent.clientHeight <= parent.scrollTop + 1 })
          parent.scrollTop = previous
        }
      }
      if (parent === dialog) break
    }
    return { name, viewport: { width: innerWidth, height: innerHeight }, dialog: { left: box.left, right: box.right, top: box.top, bottom: box.bottom, width: box.width, height: box.height }, paragraphs, scrollers, verticallyReachable, horizontalOverflow: dialog.scrollWidth > dialog.clientWidth + 1 }
  }, name)
  assert(geometry.dialog.left >= -1 && geometry.dialog.right <= geometry.viewport.width + 1 && geometry.dialog.top >= -1 && geometry.dialog.bottom <= geometry.viewport.height + 1, `${name}: dialog stays within the viewport`)
  assert(!geometry.horizontalOverflow && geometry.verticallyReachable && geometry.paragraphs.length === 2 && geometry.paragraphs.every(paragraph => paragraph.unclipped), `${name}: both translated status lines are readable without clipping: ${JSON.stringify(geometry)}`)
  assert(geometry.scrollers.length > 0 && geometry.scrollers.every(scroller => scroller.reachableBottom), `${name}: the real dialog can scroll to its lower content`)
  report.settlementLayouts.push(geometry)
}

async function run(name, work) {
  journeyName = name
  activePage = null
  const startViolations = report.violations.length
  const startErrors = report.pageErrors.length
  const result = { name, result: 'running' }
  const startedAt = Date.now()
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
    result.durationMs = Date.now() - startedAt
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
        if (name === 'usage' && width === 320) {
          const timing = page.locator('[data-slot="mobile-log-timing"]')
          assert(await timing.count() > 0, 'real mobile log timing fields are present')
          for (const field of await timing.all()) {
            const layout = await field.evaluate(element => {
              const [metrics, stream] = element.children
              const a = metrics.getBoundingClientRect()
              const b = stream.getBoundingClientRect()
              const bounds = element.getBoundingClientRect()
              return { contained: metrics.scrollWidth <= metrics.clientWidth + 1 && stream.scrollWidth <= stream.clientWidth + 1 && [a, b].every(rect => rect.left >= bounds.left && rect.right <= bounds.right && rect.top >= bounds.top && rect.bottom <= bounds.bottom), separate: a.right <= b.left + 1 || b.right <= a.left + 1 || a.bottom <= b.top + 1 || b.bottom <= a.top + 1 }
            })
            assert(layout.contained && layout.separate, `mobile first-token/duration and stream metrics never overlap: ${JSON.stringify(layout)}`)
          }
        }
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
        assert.equal(await mode.evaluate(element => element.tagName), 'BUTTON', 'funding mode is read from the real form select trigger')
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
    // An open modal correctly removes the underlying trigger from the
    // accessibility tree; retain the element for its aria-expanded assertion.
    const trigger = page.locator('[data-sidebar="trigger"]')
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
      const compliance = touchPage.getByRole('alert').filter({ hasText: label('Compliance confirmation required', 'fr') })
      const complianceLayout = await compliance.evaluate(element => {
        const description = element.querySelector('[data-slot="alert-description"]').getBoundingClientRect()
        const action = element.querySelector('[data-slot="alert-action"]').getBoundingClientRect()
        const bounds = element.getBoundingClientRect()
        return { descriptionBottom: description.bottom, actionTop: action.top, contained: action.left >= bounds.left && action.right <= bounds.right }
      })
      assert(complianceLayout.contained && complianceLayout.actionTop >= complianceLayout.descriptionBottom, `long compliance action follows the notice without covering it: ${JSON.stringify(complianceLayout)}`)
      // Capture the action after actual scrolling. A screenshot of the entire
      // tall alert would include content clipped by its parent scrollport.
      const confirmCompliance = compliance.getByRole('button', { name: label('Confirm compliance', 'fr'), exact: true })
      let actionGeometry
      for (let step = 0; step < 30; step++) {
        actionGeometry = await confirmCompliance.evaluate(button => {
          const rect = button.getBoundingClientRect()
          const content = button.closest('.myapi-page-content').getBoundingClientRect()
          const target = document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2)
          return { center: rect.y + rect.height / 2, viewportCenter: (content.top + content.bottom) / 2, x: content.x + content.width / 2, visible: rect.top >= content.top && rect.bottom <= content.bottom && rect.left >= content.left && rect.right <= content.right && button.scrollWidth <= button.clientWidth + 1 && (target === button || button.contains(target)) }
        })
        if (actionGeometry.visible) break
        await touchPage.mouse.move(actionGeometry.x, actionGeometry.viewportCenter)
        await touchPage.mouse.wheel(0, Math.max(-220, Math.min(220, actionGeometry.center - actionGeometry.viewportCenter)))
        await settled(touchPage)
      }
      assert(actionGeometry.visible, `translated compliance action is reachable with native scrolling: ${JSON.stringify(actionGeometry)}`)
      await screenshot(touchPage, `compliance-action-${width}`, { touch: true })
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
    for (const [name, path] of [['home', '/'], ['about', '/about'], ['privacy', '/privacy-policy'], ['agreement', '/user-agreement'], ['sign-in', '/sign-in'], ['sign-up', '/sign-up'], ['forgot-password', '/forgot-password'], ['reset', '/reset'], ...['401', '403', '404', '500', '503'].map(code => [`error-${code}`, `/${code}`])]) {
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
      // Production intentionally retries a 503 four times (1+2+4+8 seconds).
      // Wait beyond that existing retry budget, then assert the real alert.
      await failure.waitFor({ timeout: 30000 })
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

  await run('remaining-dashboard-role-data-states', async () => {
    for (const role of [1, 10]) {
      const { page, context, fixture } = await session({ role, runningClock: true })
      const sessionStart = report.requests.length
      const sections = [['models', 'Model Call Analytics', role === 1 ? '/api/data/self' : '/api/data'], ['flow', 'Flow', role === 1 ? '/api/data/flow/self' : '/api/data/flow'], ...(role === 10 ? [['users', 'User Analytics', '/api/data/users']] : [])]
      for (const [section, title, endpoint] of sections) {
        for (const state of ['populated', 'empty', 'error']) {
          fixture.state.analytics = state
          const start = report.requests.length
          await open(page, `/dashboard/${section}`, { title: label(title), expectedPath: `/dashboard/${section}` })
          const main = page.locator('main#content')
          if (section === 'models') {
            if (state === 'error') await main.getByText('--', { exact: true }).first().waitFor()
            else await main.locator(`[title="${state === 'populated' ? 20 : 0}"]`).first().waitFor()
            if (role === 1) assert.equal(await main.getByText(label('No performance data available'), { exact: true }).count(), 0, 'owner analytics never renders administrative performance')
            else await main.getByText(label('No performance data available'), { exact: true }).waitFor()
          } else if (section === 'flow') {
            if (state === 'empty') await main.getByText(label('No flow data available'), { exact: true }).waitFor()
            else if (state === 'error') {
              await main.getByRole('alert').filter({ hasText: label('Failed to load') }).waitFor({ timeout: 30000 })
              assert.equal(await main.getByText(label('No flow data available'), { exact: true }).count(), 0, 'unavailable flow is not reported as empty')
            } else {
              await main.locator('canvas').first().waitFor()
              assert.equal(await main.getByText(label('No flow data available'), { exact: true }).count(), 0)
            }
          } else {
            for (const chart of ['User Consumption Ranking', 'User Consumption Trend']) await main.getByText(label(chart), { exact: true }).waitFor()
            if (state === 'error') {
              await main.getByText(/Synthetic analytics unavailable|Request failed with status code 503/).first().waitFor({ timeout: 30000 })
              assert.equal(await main.locator('canvas').count(), 0, 'failed user analytics hides successful-looking charts')
            } else await main.locator('canvas').first().waitFor()
          }
          assert(report.requests.slice(start).some(request => request.path.replace(/\/$/, '') === endpoint && request.method === 'GET' && request.status === (state === 'error' ? 503 : 200)), `${role}/${section}/${state}: exercised the exact data contract`)
          for (const width of [320, 1280]) {
            await page.setViewportSize({ width, height: 900 })
            if (section === 'models' && state === 'populated') {
              // Canvas animations use elapsed Date.now, so these sessions use
              // a ticking clock. An axis-only canvas is not populated proof.
              await page.waitForFunction(() => {
                const canvas = document.querySelector('main#content canvas')
                const context = canvas?.getContext('2d')
                if (!context) return false
                const pixels = context.getImageData(0, 0, canvas.width, Math.floor(canvas.height * 0.85)).data
                let colored = 0
                for (let i = 0; i < pixels.length; i += 4) {
                  if (pixels[i + 3] && Math.max(pixels[i], pixels[i + 1], pixels[i + 2]) - Math.min(pixels[i], pixels[i + 1], pixels[i + 2]) > 70) colored++
                }
                return colored > 200
              })
            }
            await screenshot(page, `dashboard-${section}-role-${role}-${state}-${width}`)
          }
        }
      }
      if (role === 1) {
        for (const width of [320, 1280]) {
          await page.setViewportSize({ width, height: 900 })
          await open(page, '/dashboard/users', { shell: false, title: label('Access Forbidden'), expectedPath: '/403' })
          await screenshot(page, `dashboard-users-owner-forbidden-${width}`, { shell: false })
        }
        const ownerReads = report.requests.slice(sessionStart)
        for (const forbidden of ['/api/data', '/api/data/users', '/api/data/flow', '/api/perf-metrics/summary']) assert.equal(ownerReads.filter(request => request.path.replace(/\/$/, '') === forbidden).length, 0, `owner must never request ${forbidden}`)
      }
      await context.close(); contexts.delete(context)
    }
  })

  await run('remaining-dashboard-performance-recovery', async () => {
    const { page, fixture } = await session({ role: 10, width: 320 })
    fixture.state.analytics = 'populated'
    fixture.state.performance = 'error'
    await open(page, '/dashboard/models', { title: label('Model Call Analytics'), expectedPath: '/dashboard/models' })
    const failure = page.getByRole('alert').filter({ hasText: label('Failed to load performance data') })
    await failure.waitFor()
    assert.equal(await page.getByText(label('No performance data available'), { exact: true }).count(), 0, 'unavailable summary is not reported as empty')
    await screenshot(page, 'dashboard-performance-error-320')
    fixture.state.performance = 'empty'
    await failure.getByRole('button', { name: label('Retry'), exact: true }).click()
    await page.getByText(label('No performance data available'), { exact: true }).waitFor()
    await failure.waitFor({ state: 'hidden' })
    await screenshot(page, 'dashboard-performance-recovered-empty-320')
  })

  await run('model-drawer-unavailable-draft-recovery', async () => {
    const { page, fixture } = await session({ role: 100, width: 320 })
    fixture.state.options = 'error'
    await open(page, '/models/metadata', { title: label('Metadata'), expectedPath: '/models/metadata' })
    await page.getByRole('button', { name: label('Add Model'), exact: true }).click()
    const dialog = page.getByRole('dialog')
    const failure = dialog.getByRole('alert').filter({ hasText: label('Unable to load settings') })
    await failure.waitFor({ timeout: 30000 })
    await dialog.getByLabel(label('Model Name *'), { exact: true }).fill('synthetic-unsaved-model')
    await dialog.getByLabel(label('Description'), { exact: true }).fill('Retain this unsaved description')
    assert.equal(await dialog.getByRole('button', { name: label('Save changes'), exact: true }).isDisabled(), true, 'Root cannot report a partial save while pricing is unavailable')
    await screenshot(page, 'model-drawer-unavailable-draft-320')
    fixture.state.options = 'ready'
    await failure.getByRole('button', { name: label('Retry'), exact: true }).click()
    await dialog.getByText(label('Pricing Configuration'), { exact: true }).waitFor()
    assert.equal(await dialog.getByLabel(label('Model Name *'), { exact: true }).inputValue(), 'synthetic-unsaved-model')
    assert.equal(await dialog.getByLabel(label('Description'), { exact: true }).inputValue(), 'Retain this unsaved description')
    assert.equal(await dialog.getByRole('button', { name: label('Save changes'), exact: true }).isEnabled(), true)
    await dialog.getByLabel(label('Model ratio'), { exact: true }).fill('1.25')
    await screenshot(page, 'model-drawer-recovered-draft-320')
    await dialog.getByRole('button', { name: label('Cancel'), exact: true }).click()
    await dialog.waitFor({ state: 'hidden' })
    await page.getByRole('button', { name: label('Add Model'), exact: true }).click()
    assert.equal(await page.getByRole('dialog').getByLabel(label('Model Name *'), { exact: true }).inputValue(), '', 'a new drawer does not revive a cancelled draft')
    assert.equal(await page.getByRole('dialog').getByLabel(label('Model ratio'), { exact: true }).inputValue(), '')
  })

  await run('remaining-deployment-disabled-and-unavailable', async () => {
    for (const role of [10, 100]) {
      const { page, context, fixture } = await session({ role })
      const start = report.requests.length
      for (const width of [320, 1280]) {
        await page.setViewportSize({ width, height: 900 })
        await open(page, '/models/deployments', { title: label('Deployments'), expectedPath: '/models/deployments' })
        await page.getByRole('heading', { level: 3, name: label('Model deployment service is disabled'), exact: true }).waitFor()
        assert.equal(await page.getByRole('button', { name: label('Create deployment'), exact: true }).isDisabled(), true)
        assert.equal(await page.getByRole('button', { name: label('Go to settings'), exact: true }).count(), role === 100 ? 1 : 0, 'only Root can reach configuration from this state')
        await screenshot(page, `deployments-role-${role}-disabled-${width}`)
      }
      fixture.state.deploymentSettings = 'error'
      await open(page, '/models/deployments', { title: label('Deployments'), expectedPath: '/models/deployments' })
      const failure = page.getByRole('alert').filter({ hasText: label('Unable to load settings') })
      await failure.waitFor()
      assert.equal(await page.getByText(label('Model deployment service is disabled'), { exact: true }).count(), 0, 'unavailable configuration does not claim deployment is disabled')
      assert.equal(await page.getByRole('button', { name: label('Create deployment'), exact: true }).isDisabled(), true)
      for (const width of [320, 1280]) {
        await page.setViewportSize({ width, height: 900 })
        await screenshot(page, `deployments-role-${role}-unavailable-${width}`)
      }
      fixture.state.deploymentSettings = 'disabled'
      await failure.getByRole('button', { name: label('Retry'), exact: true }).click()
      await page.getByRole('heading', { level: 3, name: label('Model deployment service is disabled'), exact: true }).waitFor()
      await failure.waitFor({ state: 'hidden' })
      const deploymentReads = report.requests.slice(start).filter(request => request.path.startsWith('/api/deployments'))
      assert(deploymentReads.length >= 4, 'disabled, failed and recovered settings were fetched')
      assert(deploymentReads.every(request => request.method === 'GET' && request.path === '/api/deployments/settings'), 'no list, hardware or connection test is allowed while deployment is unavailable')
      if (role === 100) {
        await page.getByRole('button', { name: label('Go to settings'), exact: true }).click()
        await page.waitForURL(`${origin}/system-settings/models/model-deployment`)
        await page.getByRole('heading', { level: 1, name: label('Model Deployment'), exact: true }).waitFor()
      }
      await context.close(); contexts.delete(context)
    }
  })

  await run('remaining-playground-read-only-and-gate', async () => {
    for (const width of [320, 1280]) {
      const { page, context } = await session({ role: 1, width })
      const start = report.requests.length
      await open(page, '/dashboard/overview', { title: label('Overview'), expectedPath: '/dashboard/overview' })
      await open(page, '/playground', { title: label('Start a playground chat'), headingLevel: 2, expectedPath: '/playground' })
      const input = page.getByPlaceholder(label('Ask anything'), { exact: true })
      await input.fill('Synthetic unsent playground draft')
      assert.equal(await input.inputValue(), 'Synthetic unsent playground draft')
      await page.getByRole('button', { name: label('Parameters'), exact: true }).click()
      await page.getByText(label('Parameter settings'), { exact: true }).waitFor()
      if (width === 320) {
        const dialog = page.getByRole('dialog', { name: label('Parameter settings'), exact: true })
        await dialog.getByRole('button', { name: label('Close'), exact: true }).click()
        await dialog.waitFor({ state: 'hidden' })
      } else await page.keyboard.press('Escape')
      assert.equal(await input.inputValue(), 'Synthetic unsent playground draft', 'closing options leaves the unsent message intact')
      await screenshot(page, `playground-unsent-${width}`, { headingLevel: 2 })
      // Back leaves the route with options open; Forward must restore an
      // interactive route without a stranded modal or sending the draft.
      await page.getByRole('button', { name: label('Parameters'), exact: true }).click()
      await page.goBack({ waitUntil: 'networkidle' })
      await page.waitForURL(`${origin}/dashboard/overview`)
      assert.equal(await page.getByRole('dialog').count(), 0)
      await page.goForward({ waitUntil: 'networkidle' })
      await page.waitForURL(url => url.pathname.replace(/\/$/, '') === '/playground')
      await page.getByRole('heading', { level: 2, name: label('Start a playground chat'), exact: true }).waitFor()
      for (const endpoint of ['/pg/keys']) assert(report.requests.slice(start).some(request => request.path === endpoint && request.method === 'GET'), `playground uses own ${endpoint}`)
      assert.equal(report.requests.slice(start).filter(request => request.method !== 'GET' && request.path !== '/api/user/auth/refresh').length, 0, 'typing and options never send a provider or business mutation')
      await context.close(); contexts.delete(context)

      const disabled = await session({ role: 1, width, playgroundDisabled: true })
      const disabledStart = report.requests.length
      await open(disabled.page, '/playground', { title: label('Overview'), expectedPath: '/dashboard/overview' })
      assert.equal(await disabled.page.getByRole('heading', { name: label('Start a playground chat'), exact: true }).count(), 0)
      assert.equal(report.requests.slice(disabledStart).filter(request => request.pagePath.replace(/\/$/, '') === '/playground' && ['/pg/keys', '/pg/models'].includes(request.path)).length, 0, 'disabled route never loads playground options; redirected overview keeps its own reads')
      await screenshot(disabled.page, `playground-sidebar-disabled-${width}`)
      await disabled.context.close(); contexts.delete(disabled.context)
    }
  })

  await run('remaining-chat-missing-presets-and-recovery', async () => {
    for (const width of [320, 1280]) {
      const { page, context } = await session({ role: 1, width })
      const start = report.requests.length
      const externalStart = report.blockedExternal.length
      for (const path of ['/chat/0', '/chat2link']) {
        const chatStart = report.requests.length
        const headingLevel = path === '/chat/0' ? 2 : 1
        await open(page, path, { title: label('Chat preset not found'), headingLevel, expectedPath: path })
        assert.equal(await page.locator('main#content iframe').count(), 0, 'missing presets never create a provider frame')
        await screenshot(page, `${path === '/chat/0' ? 'chat-0' : 'chat2link'}-missing-${width}`, { headingLevel })
        assert.equal(report.requests.slice(chatStart).filter(request => request.path.replace(/\/$/, '') === '/api/token' || request.path.includes('/key')).length, 0, 'missing chat never retrieves a key list or secret before recovery navigation')
        await page.locator('main#content a[href="/dashboard"]').filter({ hasText: label('Return to dashboard') }).click()
        await page.waitForURL(`${origin}/dashboard/overview`)
        await page.getByRole('heading', { level: 1, name: label('Overview'), exact: true }).waitFor()
      }
      await open(page, '/chat/not-an-integer', { title: label('Overview'), expectedPath: '/dashboard/overview' })
      assert.equal(report.requests.slice(start).filter(request => request.path.includes('/key')).length, 0, 'recovery navigation never retrieves an active key secret')
      assert.equal(report.blockedExternal.length, externalStart, 'missing/invalid chat cannot attempt external navigation')
      await context.close(); contexts.delete(context)
    }
  })

  await run('remaining-prompt-learning-loading-error-recovery', async () => {
    for (const width of [320, 1280]) {
      const { page, context, fixture, releaseLearning } = await session({ role: 1, width, deferLearning: true })
      const start = report.requests.length
      try {
        await page.goto(`${origin}/prompt-learning`, { waitUntil: 'domcontentloaded' })
        await page.getByRole('heading', { level: 1, name: label('Prompt learning'), exact: true }).waitFor()
        assert.equal(new URL(page.url()).pathname, '/prompt-learning')
        for (const text of ['Loading learning settings…', 'Loading learning runs…', 'Loading instruction versions…']) await page.getByRole('status').filter({ hasText: label(text) }).waitFor()
        const toggle = page.getByRole('switch', { name: label('Enable learning'), exact: true })
        const save = page.getByRole('button', { name: label('Save version'), exact: true })
        assert.equal(await toggle.getAttribute('aria-disabled'), 'true')
        await page.getByRole('textbox', { name: label('Instruction content'), exact: true }).fill('Synthetic unsaved instruction')
        assert.equal(await save.isDisabled(), true, 'unconfirmed policy cannot authorize saving even a nonempty draft')
        for (const text of ['Learning is disabled', 'No learning runs yet.', 'No instruction versions yet.']) assert.equal(await page.getByText(label(text), { exact: true }).count(), 0, `loading does not claim ${text}`)
        await screenshot(page, `prompt-learning-loading-${width}`)
        fixture.state.promptLearning = 'error'
        releaseLearning()
        const failures = ['Failed to load learning policy', 'Failed to load learning runs', 'Failed to load instruction versions']
        for (const title of failures) await page.getByRole('alert').filter({ hasText: label(title) }).waitFor({ timeout: 30000 })
        assert.equal(await toggle.getAttribute('aria-disabled'), 'true')
        assert.equal(await save.isDisabled(), true)
        assert.equal(await page.getByText(label('Learning is disabled'), { exact: true }).count(), 0, 'failed policy is not a confirmed opt-out')
        await screenshot(page, `prompt-learning-unavailable-${width}`)
        fixture.state.promptLearning = 'ready'
        for (const title of failures) await page.getByRole('alert').filter({ hasText: label(title) }).getByRole('button', { name: label('Retry'), exact: true }).click()
        for (const text of ['Learning is disabled', 'No learning runs yet.', 'No instruction versions yet.']) await page.getByText(label(text), { exact: true }).waitFor()
        assert.equal(await toggle.getAttribute('aria-checked'), 'false')
        assert.notEqual(await toggle.getAttribute('aria-disabled'), 'true', 'only confirmed policy makes the switch available')
        assert.equal(await save.isEnabled(), true, 'confirmed reads allow the manual draft without submitting it')
        await screenshot(page, `prompt-learning-confirmed-disabled-${width}`)
        const reads = report.requests.slice(start).filter(request => request.path.startsWith('/api/user/prompt-learning'))
        for (const path of ['/api/user/prompt-learning', '/api/user/prompt-learning/versions', '/api/user/prompt-learning/runs']) assert(reads.some(request => request.path === path && request.status === 503) && reads.some(request => request.path === path && request.status === 200), `${path}: unavailable and recovered reads`)
        assert(reads.every(request => request.method === 'GET'), 'qualification never toggles, saves or cancels learning')
      } finally { releaseLearning() }
      await context.close(); contexts.delete(context)
    }
  })

  await run('remaining-pricing-disabled-route-guards', async () => {
    const { page } = await session({ role: 0 })
    for (const width of [320, 1280]) {
      await page.setViewportSize({ width, height: 900 })
      for (const path of ['/pricing', '/pricing/synthetic-text-model', '/rankings']) {
        const start = report.requests.length
        await open(page, path, { shell: false, title: label('Your AI gateway, in one place'), expectedPath: '/' })
        assert.equal(report.requests.slice(start).filter(request => ['/api/pricing', '/api/rankings', '/api/perf-metrics', '/api/perf-metrics/summary'].includes(request.path)).length, 0, 'disabled modules never fetch public model content')
        await screenshot(page, `${path.replaceAll('/', '-').slice(1)}-disabled-guard-${width}`, { shell: false })
      }
    }
  })

  await run('remaining-pricing-content-and-recovery', async () => {
    const search = '?search=synthetic&tokenUnit=K&group=default'
    const detailPath = '/pricing/synthetic-text-model'
    for (const width of [320, 1280]) {
      const { page, context, fixture } = await session({ role: 0, width, pricingEnabled: true })
      fixture.state.pricing = 'populated'
      const start = report.requests.length
      await open(page, `/pricing${search}`, { shell: false, title: label('Model Square'), expectedPath: '/pricing' })
      await page.getByRole('heading', { level: 3, name: 'synthetic-text-model', exact: true }).waitFor()
      await screenshot(page, `pricing-enabled-card-${width}`, { shell: false })
      await page.getByRole('button', { name: label('Details'), exact: true }).click()
      const drawer = page.getByRole('dialog', { name: 'synthetic-text-model', exact: true })
      await drawer.waitFor()
      await drawer.getByRole('heading', { level: 1, name: 'synthetic-text-model', exact: true }).waitFor()
      await drawer.getByRole('button', { name: label('Close'), exact: true }).click()
      await drawer.waitFor({ state: 'hidden' })
      assert.equal(new URL(page.url()).pathname.replace(/\/$/, ''), '/pricing', 'closing pricing details leaves the catalog route')
      await open(page, `${detailPath}${search}`, { shell: false, title: 'synthetic-text-model', expectedPath: detailPath })
      for (const title of ['Overview', 'Performance', 'API']) {
        const tab = page.getByRole('tab', { name: label(title), exact: true })
        assert(await tab.evaluate(element => [...element.querySelectorAll('span')].every(label => label.scrollWidth <= label.clientWidth + 1 && getComputedStyle(label).textOverflow !== 'ellipsis')), 'model detail tabs retain their complete visible labels on narrow screens')
      }
      await page.getByRole('tab', { name: label('Performance'), exact: true }).click()
      await page.getByText(label('Performance data is not yet available for this model.'), { exact: true }).waitFor()
      assert.equal(await page.getByText('100%', { exact: true }).count(), 0, 'empty performance is not invented healthy availability')
      await screenshot(page, `pricing-model-empty-performance-${width}`, { shell: false })
      await page.getByRole('button', { name: label('Back'), exact: true }).click()
      await page.waitForURL(url => url.pathname.replace(/\/$/, '') === '/pricing' && url.searchParams.get('search') === 'synthetic' && url.searchParams.get('tokenUnit') === 'K' && url.searchParams.get('group') === 'default')
      await page.getByRole('heading', { level: 1, name: label('Model Square'), exact: true }).waitFor()

      fixture.state.pricing = 'empty'
      await open(page, `${detailPath}${search}`, { shell: false, title: label('Model not found'), headingLevel: 2, expectedPath: detailPath })
      assert.equal(await page.getByRole('button', { name: label('Retry'), exact: true }).count(), 0, 'confirmed absent model is distinct from a failed read')
      await screenshot(page, `pricing-model-missing-${width}`, { shell: false })
      await page.getByRole('button', { name: label('Back to Models'), exact: true }).click()
      await page.waitForURL(url => url.pathname.replace(/\/$/, '') === '/pricing' && url.searchParams.get('search') === 'synthetic' && url.searchParams.get('tokenUnit') === 'K')

      fixture.state.pricing = 'error'
      await page.goto(`${origin}${detailPath}${search}`, { waitUntil: 'networkidle' })
      const failure = page.getByRole('alert').filter({ hasText: label('Failed to load models') })
      await failure.waitFor({ timeout: 30000 })
      await failure.getByRole('heading', { level: 2, name: label('Failed to load models'), exact: true }).waitFor()
      assert.equal(new URL(page.url()).pathname.replace(/\/$/, ''), detailPath)
      assert.equal(await page.getByRole('heading', { name: label('Model not found'), exact: true }).count(), 0, 'failed pricing never impersonates a missing model')
      await screenshot(page, `pricing-model-unavailable-${width}`, { shell: false })
      // Verify Back also preserves filters while the source is unavailable.
      await failure.getByRole('button', { name: label('Back to Models'), exact: true }).click()
      await page.waitForURL(url => url.pathname.replace(/\/$/, '') === '/pricing' && url.searchParams.get('search') === 'synthetic' && url.searchParams.get('tokenUnit') === 'K' && url.searchParams.get('group') === 'default')
      await page.goBack({ waitUntil: 'networkidle' })
      await failure.waitFor({ timeout: 30000 })
      fixture.state.pricing = 'populated'
      await failure.getByRole('button', { name: label('Retry'), exact: true }).click()
      await page.getByRole('heading', { level: 1, name: 'synthetic-text-model', exact: true }).waitFor()
      const recovered = new URL(page.url())
      assert.equal(recovered.pathname.replace(/\/$/, ''), detailPath)
      assert.equal(recovered.searchParams.get('search'), 'synthetic')
      assert.equal(recovered.searchParams.get('tokenUnit'), 'K')
      assert.equal(recovered.searchParams.get('group'), 'default')
      await screenshot(page, `pricing-model-recovered-${width}`, { shell: false })
      const metrics = report.requests.slice(start).filter(request => request.path === '/api/perf-metrics')
      assert(metrics.length > 0 && metrics.every(request => new URLSearchParams(request.query).get('model') === 'synthetic-text-model' && new URLSearchParams(request.query).get('hours') === '24'), 'only the bounded synthetic model performance is read')
      assert(report.requests.slice(start).some(request => request.path === '/api/perf-metrics/summary' && request.query === '?hours=24'), 'default catalog cards exercise their actual public summary contract')
      await context.close(); contexts.delete(context)
    }
  })

  await run('remaining-auth-alias-and-incomplete-flows', async () => {
    for (const width of [320, 1280]) {
      const { page, context } = await session({ role: 0, width })
      const start = report.requests.length
      const externalStart = report.blockedExternal.length
      await open(page, '/register?redirect=%2Fkeys&aff=synthetic-referral', { shell: false, title: label('Create an account'), headingLevel: 2, expectedPath: '/sign-up' })
      assert.equal(new URL(page.url()).searchParams.get('redirect'), '/keys')
      assert.equal(new URL(page.url()).searchParams.get('aff'), 'synthetic-referral')
      await screenshot(page, `register-alias-search-${width}`, { shell: false })
      await open(page, '/user/reset', { shell: false, title: label('Reset password'), headingLevel: 2, expectedPath: '/user/reset' })
      await page.getByRole('alert').filter({ hasText: label('Invalid reset link, please request a new password reset.') }).waitFor()
      assert.equal(await page.getByRole('button', { name: label('auth.resetPasswordConfirm.confirm'), exact: true }).isDisabled(), true)
      assert.equal(await page.getByRole('textbox', { name: label('Email'), exact: true }).isDisabled(), true)
      await screenshot(page, `reset-missing-parameters-${width}`, { shell: false })
      await open(page, '/otp', { shell: false, title: label('Two-factor Authentication'), headingLevel: 2, expectedPath: '/otp' })
      const verify = page.getByRole('button', { name: label('Verify and Sign In'), exact: true })
      assert.equal(await verify.isDisabled(), true)
      await page.getByRole('button', { name: label('Use backup code'), exact: true }).click()
      await page.getByRole('textbox', { name: label('Backup Code'), exact: true }).waitFor()
      assert.equal(await verify.isDisabled(), true)
      await screenshot(page, `otp-backup-mode-no-flow-${width}`, { shell: false })
      await page.getByRole('button', { name: label('Use authenticator code'), exact: true }).click()
      await page.getByText(label('Verification Code'), { exact: true }).waitFor()
      await page.getByRole('link', { name: label('Re-login'), exact: true }).click()
      await page.waitForURL(url => url.pathname === '/sign-in')
      await page.getByRole('heading', { level: 2, name: label('Sign in'), exact: true }).waitFor()
      for (const [name, path] of [['missing-provider', '/oauth'], ['wechat-missing-code', '/oauth?provider=wechat'], ['github-missing-code', '/oauth/github']]) {
        await open(page, path, { shell: false, title: label('Sign in'), headingLevel: 2, expectedPath: '/sign-in' })
        await screenshot(page, `oauth-${name}-${width}`, { shell: false })
      }
      assert.equal(report.requests.slice(start).filter(request => request.method !== 'GET' && request.path !== '/api/user/auth/refresh').length, 0, 'incomplete auth never submits a credential, reset or verification')
      assert.equal(report.requests.slice(start).filter(request => request.path.startsWith('/api/oauth') || request.path.includes('/wechat')).length, 0, 'incomplete OAuth cannot call a provider endpoint')
      assert.equal(report.blockedExternal.length, externalStart, 'incomplete auth never attempts external navigation')
      await context.close(); contexts.delete(context)
    }
  })

  await run('settlement-automatic-status-readonly', async () => {
    const profiles = [{ language: 'en', width: 1280 }, ...Object.keys(languages).map(language => ({ language, width: 320 }))]
    for (const { language, width } of profiles) {
      const { page, context } = await session({ language, width, settlementReviews: true })
      const start = report.requests.length
      await open(page, '/usage-logs/common', { title: label('Common Logs', language) })
      for (const index of [0, 1]) {
        const requestId = SETTLEMENT_REVIEW_IDS[index]
        const { dialog, panel } = await openSettlementLog(page, requestId, language)
        await assertAutomaticSettlement(panel, index, language)
        for (let repeat = 0; repeat < 2; repeat++) {
          const refreshed = page.waitForResponse(response => new URL(response.url()).pathname === `/api/usage-review/${requestId}` && response.request().method() === 'GET')
          await panel.getByRole('button', { name: label('Refresh', language), exact: true }).click()
          await refreshed
          await assertAutomaticSettlement(panel, index, language)
        }
        const name = `settlement-${index === 0 ? 'automatic-pending' : 'journal-finalization'}-${language}-${width}`
        await measureSettlementLayout(panel, name)
        await checkTextContrast(panel.getByRole('status').locator('p').first(), `${name}-title`, 'light')
        await checkTextContrast(panel.getByRole('status').locator('p').last(), `${name}-description`, 'light')
        await screenshot(page, name)
        await page.keyboard.press('Escape')
        await dialog.waitFor({ state: 'hidden' })
        // A fresh click after dismissal must still be informational. No manual
        // form may reappear because a cached raw journal state is unresolved.
        const reopened = await openSettlementLog(page, requestId, language)
        await assertAutomaticSettlement(reopened.panel, index, language)
        await reopened.dialog.getByRole('button', { name: label('Close', language), exact: true }).click()
        await reopened.dialog.waitFor({ state: 'hidden' })
      }
      if (language === 'en' && width === 1280) {
        await openSettlementLog(page, SETTLEMENT_REVIEW_IDS[0], language)
        await page.reload({ waitUntil: 'networkidle' })
        assert.equal(await page.getByRole('dialog').count(), 0, 'reload dismisses stale status state')
        const reloaded = await openSettlementLog(page, SETTLEMENT_REVIEW_IDS[0], language)
        await assertAutomaticSettlement(reloaded.panel, 0, language)
      }
      assertSettlementReadOnly(start)
      await context.close(); contexts.delete(context)
    }
  })

  await run('settlement-manual-expansion-without-submission', async () => {
    for (const width of [1280, 320]) {
      const { page, context } = await session({ width, settlementReviews: true })
      const start = report.requests.length
      await open(page, '/usage-logs/common', { title: label('Common Logs') })
      await page.getByRole('button', { name: label('Pending requests'), exact: true }).click()
      const dialog = page.getByRole('dialog', { name: label('Pending requests'), exact: true })
      await dialog.getByRole('button').filter({ hasText: SETTLEMENT_REVIEW_IDS[2] }).click()
      const panel = dialog.getByRole('region', { name: label('Usage pending review'), exact: true })
      const advanced = panel.getByRole('button', { name: label('Advanced: manual reconciliation'), exact: true })
      await advanced.waitFor()
      assert.equal(await advanced.getAttribute('aria-expanded'), 'false')
      assert.equal(await panel.locator('form, input, [role="checkbox"]').count(), 0, 'legitimate manual review starts as a deliberate collapsed action')
      const evidence = panel.getByText(label('Frozen pricing evidence'), { exact: true })
      await evidence.click()
      await panel.getByText(SETTLEMENT_REVIEW_METADATA, { exact: true }).waitFor()
      await evidence.click()
      await screenshot(page, `settlement-manual-collapsed-${width}`)
      await advanced.click()
      assert.equal(await advanced.getAttribute('aria-expanded'), 'true')
      const amount = panel.getByRole('textbox', { name: label('Confirmed quota (internal units)'), exact: true })
      const reference = panel.getByRole('textbox', { name: label('Evidence reference'), exact: true })
      await amount.fill('60')
      await reference.fill('Synthetic unsent evidence reference')
      assert.equal(await panel.getByRole('checkbox').isChecked(), false)
      assert.equal(await panel.getByRole('button', { name: label('Confirm reconciliation'), exact: true }).isEnabled(), true)
      await reference.scrollIntoViewIfNeeded()
      await screenshot(page, `settlement-manual-expanded-${width}`)
      await advanced.click()
      assert.equal(await panel.locator('form, input, [role="checkbox"]').count(), 0)
      await advanced.click()
      assert.equal(await amount.inputValue(), '60', 'collapse/reopen retains an unsent local draft')
      assert.equal(await reference.inputValue(), 'Synthetic unsent evidence reference')
      await dialog.getByRole('button', { name: label('Close'), exact: true }).click()
      await dialog.waitFor({ state: 'hidden' })
      await page.getByRole('button', { name: label('Pending requests'), exact: true }).click()
      await dialog.getByRole('button').filter({ hasText: SETTLEMENT_REVIEW_IDS[2] }).click()
      await advanced.waitFor()
      assert.equal(await advanced.getAttribute('aria-expanded'), 'false', 'dismissal cannot reopen an active manual form')
      assert.equal(await panel.locator('input').count(), 0)
      assertSettlementReadOnly(start)
      assert(report.requests.slice(start).some(request => request.path === '/api/usage-reviews/pending' && request.query === '?writer=authoritative&after=0'), 'manual review exercises the bounded Root pending queue')
      await context.close(); contexts.delete(context)
    }
  })

  await run('settlement-owner-status-and-display-privacy', async () => {
    for (const width of [1280, 320]) {
      const { page, context } = await session({ role: 1, width, settlementReviews: true })
      const start = report.requests.length
      await open(page, '/usage-logs/common', { title: label('Common Logs') })
      assert.equal(await page.getByRole('button', { name: label('Pending requests'), exact: true }).count(), 0, 'ordinary owner has no Root pending queue')
      for (const index of [0, 1, 2]) {
        const { dialog, panel } = await openSettlementLog(page, SETTLEMENT_REVIEW_IDS[index])
        if (index < 2) {
          await assertAutomaticSettlement(panel, index)
          await measureSettlementLayout(panel, `settlement-owner-${index}-${width}`)
          await screenshot(page, `settlement-owner-${index === 0 ? 'pending' : 'journal-finalization'}-${width}`)
        }
        assert.equal(await panel.locator('form, input, textarea, [role="checkbox"]').count(), 0, 'even resource-level manual eligibility cannot give an owner Root controls')
        for (const key of ['Advanced: manual reconciliation', 'Confirm reconciliation', 'Frozen pricing evidence']) assert.equal(await panel.getByText(label(key), { exact: true }).count(), 0, key)
        const text = await page.locator('body').innerText()
        for (const sentinel of [SETTLEMENT_REVIEW_METADATA, 'synthetic_frozen_pricing', SETTLEMENT_REVIEW_DIAGNOSTIC]) assert(!text.includes(sentinel), 'raw frozen pricing and diagnostic metadata cannot be rendered to the owner')
        await page.keyboard.press('Escape')
        await dialog.waitFor({ state: 'hidden' })
      }
      const reads = report.requests.slice(start)
      assert(reads.some(request => request.path === '/api/log/self'), 'owner reads only their log endpoint')
      assert.equal(reads.filter(request => ['/api/log', '/api/usage-reviews/pending'].includes(request.path)).length, 0)
      assertSettlementReadOnly(start)
      await context.close(); contexts.delete(context)
    }
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
