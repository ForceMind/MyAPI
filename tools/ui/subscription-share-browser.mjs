// Production-build UI qualification with synthetic APIs only. No provider,
// production credentials, quota settlement, or real subscription is accessed.
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { createReadStream, existsSync, mkdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { createServer } from 'node:http'
import { extname, resolve, sep } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { createUIFixture, FIXTURE_TIME } from './browser-fixtures.mjs'

const repo = fileURLToPath(new URL('../../', import.meta.url))
const build = resolve(repo, 'web/dist')
const output = resolve(process.env.MYAPI_BROWSER_ARTIFACTS || `${repo}/.local-tests/subscription-share-browser`)
const version = readFileSync(resolve(repo, 'VERSION'), 'utf8').trim()
const sourceHead = execFileSync('git', ['rev-parse', 'HEAD'], { cwd: repo, encoding: 'utf8' }).trim()
const languages = { en: 'en', zh: 'zhCN', 'zh-TW': 'zhTW', fr: 'fr', ru: 'ru', ja: 'ja', vi: 'vi' }
const mime = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.png': 'image/png', '.woff2': 'font/woff2' }
assert(existsSync(resolve(build, 'index.html')), 'Build the actual frontend first')
mkdirSync(output, { recursive: true })
const server = createServer((request, response) => {
  const url = new URL(request.url || '/', 'http://localhost')
  let path
  try { path = resolve(build, `.${decodeURIComponent(url.pathname)}`) }
  catch { response.writeHead(400).end(); return }
  if (!path.startsWith(`${build}${sep}`) && path !== build) { response.writeHead(403).end(); return }
  if (!existsSync(path) || statSync(path).isDirectory()) path = resolve(build, 'index.html')
  response.writeHead(200, { 'content-type': mime[extname(path)] || 'application/octet-stream', 'cache-control': 'no-store' })
  createReadStream(path).pipe(response)
})
const report = { synthetic: true, version, sourceHead, fixtureTime: new Date(FIXTURE_TIME).toISOString(), journeys: [], errors: [] }
let browser
try {
  await new Promise(done => server.listen(0, '127.0.0.1', done))
  const origin = `http://127.0.0.1:${server.address().port}`
  const driver = process.env.MYAPI_PLAYWRIGHT_MODULE
  const { chromium } = await import(driver ? pathToFileURL(resolve(driver)).href : 'playwright')
  browser = await chromium.launch({ headless: true, args: ['--disable-dev-shm-usage'] })
  for (const [language, languageCode] of Object.entries(languages)) {
    const translations = JSON.parse(readFileSync(resolve(repo, `web/src/i18n/locales/${language}.json`), 'utf8')).translation
    const t = key => { assert(translations[key], `Missing ${language} key: ${key}`); return translations[key] }
    for (const width of [320, 1280]) {
      for (const role of [100, 1]) {
        const name = `subscription-share-${language}-${width}-${role === 100 ? 'root' : 'owner'}`
        const context = await browser.newContext({ viewport: { width, height: 900 }, reducedMotion: 'reduce', timezoneId: 'Asia/Shanghai', serviceWorkers: 'block' })
        const fixture = createUIFixture({ role, language: languageCode, personalPolicy: true })
        const now = FIXTURE_TIME / 1000
        const pool = { pool_id: 7, snapshot_id: 70, revision: 2, channel_name: role === 100 ? 'Synthetic share account — international window' : '', ...(role === 100 ? {channel_id: 9} : {}), plan_type: 'plus', window_slot: 'primary', window_type: 'five_hour', window_seconds: 18000, reset_at: now + 3600, observed_at: now - 30, sample_age_seconds: 30, max_age_seconds: 300, state: 'active', reason: 'eligible', allocated_bps: 6000, share_bps: role === 100 ? 2000 : 0, estimated_used_micro: 12_500_000, unattributed_micro: 1_000_000, pending_attempts: 2, unsampled_attempts: 1, uncertain_attempts: 1, approximate: true, baseline_time: now - 600, replacement_required: false, rebaseline_required: false }
        const writes = []
        const closes = []
        let existingMenuKeyReads = 0
        const unexpected = []
        await context.addInitScript(code => { localStorage.setItem('i18nextLng', code); localStorage.setItem('theme', 'light') }, languageCode)
        await context.route('**/*', async route => {
          try {
            const request = route.request()
            const url = new URL(request.url())
            if (url.origin !== origin) { unexpected.push(`external:${url.origin}`); return route.abort() }
            // Explicit copy reads an existing Key. It never creates or rotates it.
            if (url.pathname === '/api/token/21/key') {
              assert.equal(request.method(), 'POST')
              assert.equal(request.postData(), null)
              existingMenuKeyReads++
              return route.fulfill({ json: { success: true, data: { key: 'synthetic-share-browser-only' } } })
            }
            if (url.pathname === '/api/subscription-share/pools/7/close') {
              assert.equal(role, 100)
              assert.equal(request.method(), 'POST')
              const body = request.postDataJSON()
              assert.match(body.id, /^[a-f0-9]{64}$/)
              assert.equal(body.expected_revision, pool.revision)
              assert.equal(body.confirmed_close, true)
              closes.push(body)
              pool.state = 'closed'
              pool.reason = 'pool_closed'
              pool.revision++
              return route.fulfill({ json: { success: true, data: null } })
            }
            if (url.pathname === '/api/token/21/subscription-share') {
              if (request.method() === 'PUT') {
                assert.equal(role, 100, 'Owner cannot mutate')
                const body = request.postDataJSON()
                assert.match(body.id, /^[a-f0-9]{64}$/)
                assert.equal(body.pool_id, 7)
                assert.equal(body.snapshot_id, 70)
                assert.equal(body.expected_revision, pool.revision)
                assert.equal(body.share_bps, 2500)
                assert.equal(body.confirmed_approximate, true)
                assert(!body.replace_window, 'An ordinary share edit cannot rebaseline')
                writes.push(body)
                pool.allocated_bps += body.share_bps - pool.share_bps
                pool.share_bps = body.share_bps
                pool.revision++
              } else assert.equal(request.method(), 'GET')
              return route.fulfill({ json: { success: true, data: { token_id: 21, pools: [pool] } } })
            }
            if (!url.pathname.startsWith('/api/') && !url.pathname.startsWith('/pg/')) return route.continue()
            const result = fixture.response(url, request.method())
            if (result.violation) unexpected.push(result.violation)
            assert(!result.violation, result.violation)
            return route.fulfill({ status: result.status, json: result.body })
          } catch (error) {
            const message = `${name}: synthetic API contract failed: ${error.message}`
            unexpected.push(message)
            report.errors.push(message)
            await route.abort('failed')
          }
        })
        const page = await context.newPage()
        page.setDefaultTimeout(15000)
        page.on('pageerror', error => report.errors.push(`${name}: ${error.message}`))
        page.on('console', message => { if (message.type() === 'error') report.errors.push(`${name}: ${message.text()}`) })
        await page.clock.setFixedTime(new Date(FIXTURE_TIME))
        const screenshots = []
        let capturedKeyList = false
        let keyListGeometry
        const open = async () => {
          await page.goto(`${origin}/keys`, { waitUntil: 'networkidle' })
          if (!capturedKeyList) {
            const keyList = page.getByRole('main', { name: t('API Keys'), exact: true })
            await keyList.waitFor({ state: 'visible' })
            await keyList.getByRole('button', { name: t('Window share'), exact: true }).waitFor({ state: 'visible' })
            keyListGeometry = await keyList.evaluate(element => {
              const rect = element.getBoundingClientRect()
              return { left: rect.left, right: rect.right, viewport: innerWidth, scrollWidth: element.scrollWidth, clientWidth: element.clientWidth, pageScrollWidth: document.documentElement.scrollWidth }
            })
            assert(keyListGeometry.left >= -1 && keyListGeometry.right <= width + 1 && keyListGeometry.scrollWidth <= keyListGeometry.clientWidth + 1 && keyListGeometry.pageScrollWidth <= width + 1, `${name}: Key list fits without horizontal clipping`)
            const keyListImage = `${name}-key-list.png`
            await page.screenshot({ path: resolve(output, keyListImage), animations: 'disabled' })
            screenshots.push(keyListImage)
            capturedKeyList = true
          }
          await page.getByRole('button', { name: t('Window share'), exact: true }).last().click()
          if (role === 100) {
            const editor = page.getByRole('main', {name: t('Adjust window share'), exact: true})
            await editor.waitFor({state: 'visible'})
            await editor.getByLabel(t('Subscription account'), {exact: true}).selectOption('9')
            await editor.getByLabel(t('Choose one subscription window'), {exact: true}).selectOption('7:70')
            return editor
          }
          const dialog = page.getByRole('dialog', { name: t('Subscription shares'), exact: true })
          await dialog.waitFor({ state: 'visible' })
          await dialog.getByText(`${t('Native window')} 7`, { exact: true }).waitFor({ state: 'visible' })
          return dialog
        }
        const dialog = await open()
        const revision = await page.locator('html').getAttribute('data-build-rev')
        assert(revision?.startsWith(`rv.${version}.`), 'Actual build matches candidate version')
        if (process.env.GITHUB_SHA) assert(revision.includes(process.env.GITHUB_SHA), 'Actual build matches CI checkout')
        const notice = dialog.getByText(t('Approximate usage allocation'), { exact: true })
        await notice.scrollIntoViewIfNeeded()
        const topImage = `${name}-notice.png`
        await page.screenshot({ path: resolve(output, topImage), animations: 'disabled' })
        const target = role === 100 ? dialog.getByLabel(t('Share of total window'), { exact: true }) : dialog.getByRole('region', { name: t('Your subscription share') })
        await target.scrollIntoViewIfNeeded()
        const geometry = await dialog.evaluate(element => {
          const rect = element.getBoundingClientRect()
          return { left: rect.left, right: rect.right, viewport: innerWidth, scrollWidth: element.scrollWidth, clientWidth: element.clientWidth }
        })
        assert(geometry.left >= -1 && geometry.right <= width + 1 && geometry.scrollWidth <= geometry.clientWidth + 1, `${name}: dialog fits without horizontal clipping`)
        const poolImage = `${name}-pool.png`
        await page.screenshot({ path: resolve(output, poolImage), animations: 'disabled' })
        screenshots.push(topImage, poolImage)
        if (role === 100) {
          await target.fill('25')
          const save = dialog.getByRole('button', { name: t('Save changes'), exact: true })
          assert(await save.isDisabled(), 'Approximation confirmation is required')
          await dialog.getByRole('checkbox', { name: t('I understand this is an estimate with no verified error bound and may stop requests only after the share is exceeded.'), exact: true }).check()
          await save.click()
          await dialog.waitFor({ state: 'hidden' })
          const updated = await open()
          await updated.getByText('25%', { exact: true }).first().waitFor()
          assert.equal(writes.length, 1, 'One explicit write only')
          await updated.getByRole('button', {name: t('Advanced window controls'), exact: true}).click()
          const advanced = page.getByRole('dialog', {name: t('Subscription shares'), exact: true})
          await advanced.getByRole('button', { name: t('Close pool'), exact: true }).click()
          const confirmation = page.getByRole('alertdialog', { name: t('Close this subscription pool?'), exact: true })
          await confirmation.waitFor({ state: 'visible' })
          const closeButton = confirmation.getByRole('button', { name: t('Close pool'), exact: true })
          assert(await closeButton.isDisabled(), 'Closing requires its own explicit confirmation')
          const confirmImage = `${name}-close-confirmation.png`
          await page.screenshot({ path: resolve(output, confirmImage), animations: 'disabled' })
          screenshots.push(confirmImage)
          await confirmation.getByRole('checkbox', { name: t('I understand this pool cannot be enabled again in this version.'), exact: true }).check()
          await closeButton.click()
          await confirmation.waitFor({ state: 'hidden' })
          await advanced.getByText(t('Closed — history retained'), { exact: true }).waitFor()
          assert.equal(closes.length, 1)
          assert.equal(await advanced.getByRole('radio').count(), 0, 'Closed pools cannot be configured again')
          assert.equal(await advanced.getByRole('button', { name: t('Close pool'), exact: true }).count(), 0)
          const closedImage = `${name}-closed.png`
          await page.screenshot({ path: resolve(output, closedImage), animations: 'disabled' })
          screenshots.push(closedImage)
        } else {
          assert.equal(await dialog.getByLabel(t('Share percentage'), { exact: true }).count(), 0)
          assert.equal(await dialog.getByRole('button', { name: t('Save share'), exact: true }).count(), 0)
          assert.equal(await dialog.getByText('0%', { exact: true }).count(), 1, 'A zero-percent binding remains visible')
          assert.equal(writes.length, 0)
          assert(!(await dialog.innerText()).includes('Synthetic share account'), 'Private channel notes are absent')
        }
        assert.deepEqual(unexpected, [])
        report.journeys.push({ name, language, width, role, buildRevision: revision, writes: writes.length, closes: closes.length, existingMenuKeyReads, keyListGeometry, geometry, screenshots, passed: true })
        await context.close()
      }
    }
  }
  assert.equal(report.journeys.length, 28)
  assert.deepEqual(report.errors, [])
  report.passed = true
} catch (error) {
  report.passed = false
  report.failure = String(error.stack || error)
  throw error
} finally {
  writeFileSync(resolve(output, 'subscription-share-qualification.json'), JSON.stringify(report, null, 2) + '\n')
  await browser?.close()
  await new Promise(done => server.close(done))
}
