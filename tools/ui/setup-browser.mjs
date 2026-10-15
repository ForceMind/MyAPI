// Production UI with an isolated installation/login state machine, never a live API.
import assert from 'node:assert/strict'
import { createReadStream, existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { createServer } from 'node:http'
import { extname, resolve, sep } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

import { createUIFixture } from './browser-fixtures.mjs'

const root = resolve(fileURLToPath(new URL('../../web/dist/', import.meta.url)))
const output = resolve(process.env.MYAPI_BROWSER_ARTIFACTS || '.local-tests/setup-browser')
const version = readFileSync(new URL('../../VERSION', import.meta.url), 'utf8').trim()
const { chromium } = await import(process.env.MYAPI_PLAYWRIGHT_MODULE
  ? pathToFileURL(process.env.MYAPI_PLAYWRIGHT_MODULE).href : 'playwright')
const mime = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.png': 'image/png', '.woff2': 'font/woff2', '.svg': 'image/svg+xml' }
assert(existsSync(resolve(root, 'index.html')), 'Build frontend first')
mkdirSync(output, { recursive: true })
const server = createServer((request, response) => {
  let file = resolve(root, `.${new URL(request.url, 'http://localhost').pathname}`)
  if (!file.startsWith(root + sep) && file !== root) return response.writeHead(403).end()
  if (!existsSync(file) || !extname(file)) file = resolve(root, 'index.html')
  response.writeHead(200, { 'content-type': mime[extname(file)] || 'application/octet-stream' })
  createReadStream(file).pipe(response)
})
const report = { evidence: 'Synthetic UI installation/login/Profile only; no database, package, production or real-device installation', passed: [], errors: [] }
const languages = { en: 'en', zh: 'zhCN', 'zh-TW': 'zhTW', fr: 'fr', ru: 'ru', ja: 'ja', vi: 'vi' }
const titles = ['Database check', 'Administrator account', 'Usage mode', 'Review & initialize']
let browser
try {
  await new Promise(done => server.listen(0, '127.0.0.1', done))
  const origin = `http://127.0.0.1:${server.address().port}`
  browser = await chromium.launch({ headless: true })
  const layouts = Object.keys(languages).flatMap(language => ['light', 'dark'].map(theme => ({ language, theme, width: 320, database: 'sqlite' })))
  layouts.push(...[390, 768, 1280].map(width => ({ language: 'en', theme: 'light', width, database: 'sqlite' })),
    ...['mysql', 'postgres'].map(database => ({ language: 'en', theme: 'light', width: 320, database })))
  const flows = [
    { scenario: 'fresh', width: 320 }, { scenario: 'fresh', width: 1280 },
    { scenario: 'existing-root', width: 320 },
    { scenario: 'business-failure', width: 320 },
    { scenario: 'http-unknown', width: 1280 }, { scenario: 'stalled-post', width: 320 },
  ]
  for (const options of [...layouts, ...flows]) {
    const { width, language = 'en', theme = 'light', database = 'sqlite', scenario = 'layout' } = options
    const dictionary = JSON.parse(readFileSync(new URL(`../../web/src/i18n/locales/${language}.json`, import.meta.url), 'utf8')).translation
    const label = key => dictionary[key] || key
    const context = await browser.newContext({ viewport: { width, height: 900 }, reducedMotion: 'reduce', colorScheme: theme, serviceWorkers: 'block' })
    const fixture = createUIFixture({ role: 100, language: languages[language] })
    let initialized = false
    let authenticated = false
    let submits = 0
    let logins = 0
    let profileReads = 0
    let recoveryReads = 0
    let releasePost
    const heldPost = new Promise(done => { releasePost = done })
    let finishPost
    const obsoletePostFinished = new Promise(done => { finishPost = done })
    const errors = []
    const existingRoot = scenario === 'existing-root'
    const credentials = { username: 'setup-root', password: 'synthetic-password-2026', confirmPassword: 'synthetic-password-2026' }
    fixture.user.username = credentials.username
    const name = `setup-${scenario}-${language}-${theme}-${database}-${width}`
    try {
      await context.addCookies([{ name: 'vite-ui-theme', value: theme, url: origin }])
      await context.addInitScript(code => localStorage.setItem('i18nextLng', code), languages[language])
      await context.route('**/*', async route => {
        const request = route.request()
        const url = new URL(request.url())
        if (url.origin !== origin) return route.abort('blockedbyclient')
        if (!url.pathname.startsWith('/api/') && !url.pathname.startsWith('/pg/')) return route.continue()
        if (url.pathname === '/api/setup') {
          if (request.method() === 'GET') {
            if (submits) recoveryReads++
            return route.fulfill({ json: { success: true, data: { status: initialized, root_init: initialized || existingRoot, database_type: database } } })
          }
          assert.equal(request.method(), 'POST')
          assert.notEqual(scenario, 'layout', 'Layout checks never mutate')
          assert.equal(initialized, false, 'Never replay installation after completion')
          submits++
          assert.deepEqual(request.postDataJSON(), { ...(existingRoot ? {} : credentials), SelfUseModeEnabled: true, DemoSiteEnabled: false }, 'Exact setup payload preserves existing credentials')
          if (scenario === 'business-failure' && submits === 1) return route.fulfill({ json: { success: false, message: 'Synthetic setup rejected' } })
          if (scenario === 'http-unknown') return route.fulfill({ status: 503, json: { success: false, message: 'Synthetic gateway response is not an application outcome' } })
          if (scenario === 'stalled-post') {
            await heldPost
            // Late completion after the client deadline cannot change UI state.
            try { await route.fulfill({ json: { success: false, message: 'Synthetic obsolete response' } }) }
            catch { /* Client already cancelled this request. */ }
            finally { finishPost() }
            return
          }
          initialized = true
          return route.fulfill({ json: { success: true, message: 'Synthetic setup completed' } })
        }
        if (url.pathname === '/api/user/login') {
          assert.equal(request.method(), 'POST')
          assert(initialized, 'Login follows initialization')
          assert.deepEqual(request.postDataJSON(), { username: credentials.username, password: credentials.password })
          logins++
          authenticated = true
          const response = fixture.response(new URL('/api/user/auth/refresh', origin), 'POST')
          response.body.data.access_expires_at = Math.floor(Date.now() / 1000) + 3600
          return route.fulfill({ status: response.status, json: response.body })
        }
        assert(request.method() === 'GET' || (url.pathname === '/api/user/auth/refresh' && request.method() === 'POST'), 'Only explicitly synthetic setup/login writes are permitted')
        if (url.pathname === '/api/user/auth/refresh' && !authenticated) return route.fulfill({ status: 401, json: { success: false } })
        const response = fixture.response(url, request.method())
        assert(!response.violation, response.violation)
        if (url.pathname === '/api/user/auth/refresh') response.body.data.access_expires_at = Math.floor(Date.now() / 1000) + 3600
        if (url.pathname === '/api/user/self') { assert(authenticated); profileReads++ }
        await route.fulfill({ status: response.status, json: response.body })
      })
      const page = await context.newPage()
      page.setDefaultTimeout(25000)
      page.on('pageerror', error => errors.push(error.message))
      await page.goto(`${origin}/setup`, { waitUntil: 'domcontentloaded' })
      await page.getByRole('heading', { name: label('Database check'), exact: true }).waitFor()
      const list = page.getByRole('list', { name: label('System setup wizard'), exact: true })
      // Range rectangles test rendered words, not class names or snapshots.
      for (const title of titles) {
        const text = list.getByText(label(title), { exact: true })
        const geometry = await text.evaluate(element => {
          const node = element.firstChild
          const brokenWords = []
          for (const word of element.textContent.matchAll(/[\p{Script=Latin}\p{Script=Cyrillic}]+/gu)) {
            const range = document.createRange()
            range.setStart(node, word.index)
            range.setEnd(node, word.index + word[0].length)
            const lines = new Set([...range.getClientRects()].map(rect => Math.round(rect.top)))
            if (lines.size > 1) brokenWords.push(word[0])
          }
          return { fits: element.scrollWidth <= element.clientWidth, brokenWords }
        })
        assert(geometry.fits, `${name}: step heading overflows`)
        assert.deepEqual(geometry.brokenWords, [], `${name}: heading splits a word`)
      }
      const badge = page.locator('[data-slot="status-badge"]').first()
      const badgeLabel = badge.locator('span').last()
      assert.equal(await badgeLabel.evaluate(element => element.scrollWidth <= element.clientWidth), true, `${name}: database badge is truncated`)
      const badgeBox = await badge.boundingBox()
      const descriptionBox = await badge.locator('..').locator('p').last().boundingBox()
      assert(badgeBox && descriptionBox)
      assert(badgeBox.y >= descriptionBox.y + descriptionBox.height - 1 || badgeBox.x >= descriptionBox.x + descriptionBox.width - 1, `${name}: database badge overlaps description`)
      assert(badgeBox.x >= 0 && badgeBox.x + badgeBox.width <= width, `${name}: database badge leaves the viewport`)
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, `${name}: installation page overflows`)
      assert.equal(await page.locator('html').evaluate(element => element.classList.contains('dark')), theme === 'dark')
      await page.screenshot({ path: resolve(output, `${name}-database.png`), fullPage: true })
      if (scenario !== 'layout') {
        await page.getByRole('button', { name: 'Next', exact: true }).click()
        if (existingRoot) assert.equal(await page.getByLabel('Administrator username', { exact: true }).count(), 0)
        else {
          await page.getByLabel('Administrator username', { exact: true }).fill(credentials.username)
          await page.getByLabel('Password', { exact: true }).fill(credentials.password)
          await page.getByLabel('Confirm password', { exact: true }).fill(credentials.confirmPassword)
        }
        await page.getByRole('button', { name: 'Next', exact: true }).click()
        await page.locator('label[for="usage-mode-self"]').first().click()
        await page.getByRole('button', { name: 'Next', exact: true }).click()
        const initialize = page.getByRole('button', { name: 'Initialize system', exact: true })
        await initialize.click()
        if (scenario === 'business-failure') {
          await page.getByText('Synthetic setup rejected', { exact: true }).waitFor()
          assert.equal(await initialize.isEnabled(), true)
          assert.equal(await page.getByRole('button', { name: 'Back', exact: true }).isEnabled(), true)
          await page.getByRole('button', { name: 'Back', exact: true }).click()
          assert.equal(await page.getByRole('radio', { name: /Personal use/ }).isChecked(), true)
          await page.getByRole('button', { name: 'Back', exact: true }).click()
          assert.equal(await page.getByLabel('Administrator username', { exact: true }).inputValue(), credentials.username)
          assert.equal(await page.getByLabel('Password', { exact: true }).inputValue(), credentials.password)
          assert.equal(await page.getByLabel('Confirm password', { exact: true }).inputValue(), credentials.confirmPassword)
          await page.getByRole('button', { name: 'Next', exact: true }).click()
          await page.getByRole('button', { name: 'Next', exact: true }).click()
          await initialize.click()
        }
        if (['http-unknown', 'stalled-post'].includes(scenario)) {
          const check = page.getByRole('button', { name: 'Check installation status', exact: true })
          await check.waitFor()
          await page.waitForFunction(() => {
            const button = [...document.querySelectorAll('button')].find(element => element.textContent === 'Check installation status')
            return button && !button.disabled
          })
          assert(recoveryReads > 0, 'Unknown write result triggers a read, never a replay')
          assert.equal(submits, 1)
          assert.equal(await page.getByRole('button', { name: 'Back', exact: true }).isDisabled(), true)
          assert.equal(await page.getByRole('button', { name: /Initializ/ }).last().isDisabled(), true)
          await page.screenshot({ path: resolve(output, `${name}-unknown.png`), fullPage: true })
          // A second false read must still leave the write locked.
          const before = recoveryReads
          await check.click()
          await page.waitForFunction(() => [...document.querySelectorAll('button')].some(element => element.textContent === 'Check installation status' && !element.disabled))
          assert(recoveryReads > before)
          assert.equal(submits, 1)
          assert.equal(await page.getByRole('button', { name: 'Back', exact: true }).isDisabled(), true)
          initialized = true
          await check.click()
        }
        await page.waitForURL(url => url.pathname.replace(/\/$/, '') !== '/setup')
        assert.equal(new URL(page.url()).pathname, '/')
        await page.goto(`${origin}/sign-in`, { waitUntil: 'domcontentloaded' })
        await page.getByRole('textbox', { name: 'Username or Email', exact: true }).fill(credentials.username)
        await page.getByLabel('Password', { exact: true }).fill(credentials.password)
        await page.getByRole('button', { name: 'Sign in', exact: true }).click()
        await page.waitForURL(`${origin}/dashboard`)
        if (width === 320) {
          await page.getByRole('button', { name: 'Account menu', exact: true }).click()
          await page.getByRole('menuitem', { name: 'Profile', exact: true }).click()
        } else {
          await page.locator('a[href="/profile"]').filter({ visible: true }).click()
        }
        await page.getByRole('heading', { name: 'Synthetic UI operator', level: 3 }).waitFor()
        assert.equal(logins, 1)
        assert.equal(submits, scenario === 'business-failure' ? 2 : 1)
        assert(profileReads > 0, 'Profile actually fetched after sidebar navigation')
        await page.screenshot({ path: resolve(output, `${name}-profile.png`), fullPage: true })
      }
      const buildRevision = await page.locator('html').getAttribute('data-build-rev')
      assert(buildRevision?.startsWith(`rv.${version}.`))
      if (process.env.GITHUB_SHA) assert(buildRevision.includes(process.env.GITHUB_SHA))
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, 'No horizontal page overflow')
      assert.deepEqual(errors, [], 'No runtime errors')
      releasePost()
      if (scenario === 'stalled-post') {
        await obsoletePostFinished
        await page.getByRole('heading', { name: 'Synthetic UI operator', level: 2 }).waitFor()
        assert.equal(new URL(page.url()).pathname.replace(/\/$/, ''), '/profile')
        assert.equal(submits, 1)
      }
      assert.deepEqual(errors, [], 'Late responses do not replace the recovered state')
      report.passed.push({ scenario, language, theme, database, width, submits, logins, profileReads, recoveryReads, buildRevision })
    } finally { releasePost(); await context.close() }
  }
} catch (error) { report.errors.push(error.message); throw error }
finally {
  writeFileSync(resolve(output, 'setup-result.json'), JSON.stringify(report, null, 2))
  if (browser) await browser.close()
  await new Promise(done => server.close(done))
}
console.log(JSON.stringify(report))
