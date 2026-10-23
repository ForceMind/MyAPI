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
    const closeDialogs = async () => {
      while (await page.getByRole('dialog').count()) {
        const dialog = page.getByRole('dialog').last()
        const labelledBy = await dialog.getAttribute('aria-labelledby')
        await page.keyboard.press('Escape')
        await page.locator(`[role="dialog"][aria-labelledby="${labelledBy}"]`).waitFor({ state: 'hidden' })
      }
    }
    const openDetails = async () => {
      await page.getByTitle(t('Click to view full details'), { exact: true }).first().click()
      await page.getByRole('button', { name: t('View request content'), exact: true }).waitFor()
    }
    try {
      await page.goto(`${origin}/keys?filter=Synthetic`, { waitUntil: 'networkidle' })
      await page.getByRole('button', { name: t('View Key requests'), exact: true }).first().click()
      await page.getByText('SYNTHETIC-CONTEXT-ERROR', { exact: true }).first().waitFor()
      await page.getByTitle(t('Click to view full details'), { exact: true }).first().click()
      await page.getByText(t('Estimated input Tokens'), { exact: true }).waitFor()
      await page.getByText(t('Input exceeds the upstream context window. Shorten the input and try again.'), { exact: true }).waitFor()
      await closeDialogs()
      await page.getByText(t('Requests for Key #{{id}}').replace('{{id}}', '21'), { exact: true }).waitFor({ state: 'visible' })
      assert.equal(new URL(page.url()).searchParams.get('tokenId'), '21')
      assert.equal(await page.getByText('SYNTHETIC-OTHER-KEY', { exact: true }).count(), 0)
      assert.equal(await page.getByText('SYNTHETIC-LEGACY-NO-ID', { exact: true }).count(), 0)
      await capture('requests')
      const paged = new URL(page.url()); paged.searchParams.set('pageSize', '1')
      await page.goto(paged.href, { waitUntil: 'networkidle' })
      await page.getByRole('button', { name: t('Go to next page'), exact: true }).click()
      await page.getByText('SYNTHETIC-COMPLETED-NO-USAGE', { exact: true }).first().waitFor()
      await openDetails()
      await page.getByText(t('Request succeeded'), { exact: true }).waitFor()
      assert(await page.getByText(t('Unknown'), { exact: true }).count() > 0)
      await capture('successful-unknown-usage')
      await closeDialogs()
      await page.getByRole('button', { name: t('Go to previous page'), exact: true }).click()
      await page.getByText('SYNTHETIC-CONTEXT-ERROR', { exact: true }).first().waitFor()
      await openDetails()
      await page.getByRole('button', { name: t('View request content'), exact: true }).click()
      const body = page.getByRole('dialog', { name: t('Full Content Log Detail'), exact: true })
      await body.getByRole('alert').waitFor()
      assert.match(await body.getByRole('alert').innerText(), /exceeds the context window/)
      assert(await body.getByText('200', { exact: true }).count() > 0)
      await capture('stream-error')
      await body.getByRole('switch').check()
      await body.getByRole('tab', { name: `${t('Response')} (2)`, exact: true }).click()
      await body.getByText('context_length_exceeded', { exact: false }).last().waitFor()
      await capture('sse-raw')
      await closeDialogs()
      assert.equal(new URL(page.url()).searchParams.get('page') || '1', '1')
      await page.getByRole('button', { name: t('Go to next page'), exact: true }).click()
      await page.getByText('SYNTHETIC-COMPLETED-NO-USAGE', { exact: true }).first().waitFor()
      await page.getByRole('button', { name: t('Go to next page'), exact: true }).click()
      await page.getByText('SYNTHETIC-NOT-RECORDED', { exact: true }).first().waitFor()
      await openDetails()
      await page.getByRole('button', { name: t('View request content'), exact: true }).click()
      await page.getByText(t('No saved request content. It may not have been recorded or may have expired.'), { exact: true }).waitFor()
      await capture('missing-body')
      await closeDialogs()
      await page.getByRole('link', { name: t('Back to Keys'), exact: true }).click()
      await page.getByRole('button', { name: t('View Key requests'), exact: true }).first().waitFor()
      assert.equal(new URL(page.url()).searchParams.get('filter'), 'Synthetic')
      assert.equal(new URL(page.url()).searchParams.get('focusKeyId'), '21')
      assert.equal(await page.evaluate(() => document.activeElement?.textContent), t('View Key requests'))
      await capture('returned-key')
      await page.goto(`${origin}/keys?keyId=21`, { waitUntil: 'networkidle' })
      await page.getByRole('heading', { name: `${t('API Key')} #21`, exact: true }).waitFor()
      await page.getByRole('button', { name: t('View Key requests'), exact: true }).click()
      await page.getByText('SYNTHETIC-CONTEXT-ERROR', { exact: true }).first().waitFor()
      assert.equal(new URL(page.url()).searchParams.get('tokenId'), '21')
      await page.goto(`${origin}/usage-logs/common?tokenId=21&requestId=SYNTHETIC-PREFLIGHT-REJECTED`, { waitUntil: 'networkidle' })
      await page.getByText('SYNTHETIC-PREFLIGHT-REJECTED', { exact: true }).first().waitFor()
      await openDetails()
      await page.getByText(t('Blocked before upstream'), { exact: true }).waitFor()
      await page.getByText(t('Configured input limit'), { exact: true }).waitFor()
      await capture('preflight-rejected')
      await page.getByRole('button', { name: t('View request content'), exact: true }).click()
      const preflightBody = page.getByRole('dialog', { name: t('Full Content Log Detail'), exact: true })
      await preflightBody.getByText('400', { exact: true }).waitFor()
      await capture('preflight-body')
      await closeDialogs()
      const reads = fixture.requests.slice(start)
      for (const path of ['/api/log', '/api/log/stat']) assert(reads.some(item => item.path === path && item.token_id === '21'), `${path} is pinned to Key ID`)
      assert(!reads.some(item => /\/key$/.test(item.path)), 'No secret Key retrieval')
      assert.deepEqual(writes, []); assert.deepEqual(external, []); assert.deepEqual(pageErrors, [])
      report.journeys.push({ language, width, result: 'passed', checks: ['visible-key-id-scope', 'stable-id-list-and-stat', 'same-name-and-legacy-exclusion', 'pagination', 'completed-with-unknown-usage', 'http200-sse-error', 'response-tab-and-raw-sse', 'missing-body', 'return-filter-and-focus', 'saved-key-detail', 'no-external-or-write-or-secret-requests'], screenshots })
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
