// Production-build chat-client acceptance with synthetic, intercepted APIs.
// No provider calls, real credentials, billing, or deployment acceptance.
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { createReadStream, existsSync, mkdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { createServer } from 'node:http'
import { extname, resolve, sep } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { createUIFixture, FIXTURE_TIME } from './browser-fixtures.mjs'

const repo = fileURLToPath(new URL('../../', import.meta.url))
const root = resolve(repo, 'web/dist')
const output = resolve(process.env.MYAPI_BROWSER_ARTIFACTS || `${repo}/.local-tests/playground-browser`)
const locales = { en: 'en', zh: 'zhCN', 'zh-TW': 'zhTW', fr: 'fr', ru: 'ru', ja: 'ja', vi: 'vi' }
const translation = Object.fromEntries(Object.keys(locales).map(code => [code, JSON.parse(readFileSync(resolve(repo, `web/src/i18n/locales/${code}.json`), 'utf8')).translation]))
const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aD1sAAAAASUVORK5CYII=', 'base64')
const pdf = Buffer.from('%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\n%%EOF\n')
const report = { schema: 1, commit: process.env.GITHUB_SHA || execFileSync('git', ['rev-parse', 'HEAD'], { cwd: repo, encoding: 'utf8' }).trim(), result: 'running', evidence: 'Synthetic API UI qualification; no live provider or billing claim.', journeys: [], screenshots: [], violations: [], pageErrors: [] }
assert(existsSync(resolve(root, 'index.html')), 'Build production assets first')
mkdirSync(output, { recursive: true })
const persist = () => writeFileSync(resolve(output, 'playground-qualification.json'), JSON.stringify(report, null, 2))
const mime = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.png': 'image/png', '.woff2': 'font/woff2', '.ico': 'image/x-icon' }
const server = createServer((request, response) => {
  let file
  try { file = resolve(root, `.${decodeURIComponent(new URL(request.url || '/', 'http://localhost').pathname)}`) }
  catch { response.writeHead(400).end(); return }
  if (!file.startsWith(`${root}${sep}`)) { response.writeHead(403).end(); return }
  if (!existsSync(file) || statSync(file).isDirectory()) file = resolve(root, 'index.html')
  response.writeHead(200, { 'content-type': mime[extname(file)] || 'application/octet-stream', 'cache-control': 'no-store' })
  createReadStream(file).pipe(response)
})
let browser
try {
  await new Promise(done => server.listen(0, '127.0.0.1', done))
  const origin = `http://127.0.0.1:${server.address().port}`
  const { chromium } = await import(process.env.MYAPI_PLAYWRIGHT_MODULE ? pathToFileURL(process.env.MYAPI_PLAYWRIGHT_MODULE).href : 'playwright')
  browser = await chromium.launch({ headless: true, executablePath: process.env.MYAPI_CHROMIUM_PATH || undefined, args: ['--disable-dev-shm-usage'] })
  for (const { width, language } of [{ width: 1280, language: 'en' }, ...Object.keys(locales).map(language => ({ width: 320, language }))]) {
    const name = `chat-attachments-${language}-${width}`
    const label = key => translation[language][key] || key
    const fixture = createUIFixture({ role: 1, language: locales[language] })
    const context = await browser.newContext({ viewport: { width, height: 900 }, hasTouch: width === 320, isMobile: width === 320, reducedMotion: 'reduce', serviceWorkers: 'block' })
    await context.addInitScript(code => localStorage.setItem('i18nextLng', code), locales[language])
    const sent = []
    let failNext = false
    await context.route('**/*', async route => {
      const request = route.request(), url = new URL(request.url())
      if (url.origin !== origin) { if (['data:', 'blob:'].includes(url.protocol)) return route.continue(); report.violations.push(`${name}: external request`); return route.abort() }
      if (url.pathname === '/pg/chat/completions') {
        assert.equal(request.method(), 'POST')
        assert.equal(request.headers()['x-myapi-key-id'], '21')
        const payload = request.postDataJSON()
        assert(!('group' in payload), 'body must not override key group')
        sent.push(payload)
        if (failNext) { failNext = false; return route.fulfill({ status: 400, json: { error: { code: 'playground_file_provider_unsupported', message: 'Synthetic unsupported PDF route' } } }) }
        const chunk = { id: 'synthetic-chat', object: 'chat.completion.chunk', model: payload.model, choices: [{ index: 0, delta: { content: 'Synthetic attachment received' }, finish_reason: null }] }
        return route.fulfill({ status: 200, contentType: 'text/event-stream', body: `data: ${JSON.stringify(chunk)}\n\ndata: [DONE]\n\n` })
      }
      if (url.pathname === '/pg/models') assert.equal(request.headers()['x-myapi-key-id'], '21')
      if (url.pathname.startsWith('/api/') || url.pathname.startsWith('/pg/')) {
        const result = fixture.response(url, request.method())
        if (result.violation) report.violations.push(`${name}: ${result.violation}`)
        return route.fulfill({ status: result.status || 501, json: result.body || { success: false } })
      }
      if (request.method() !== 'GET') { report.violations.push(`${name}: unexpected ${request.method()}`); return route.abort() }
      return route.continue()
    })
    const page = await context.newPage()
    page.setDefaultTimeout(15000)
    await page.clock.setFixedTime(new Date(FIXTURE_TIME))
    page.on('pageerror', error => report.pageErrors.push(`${name}: ${error.message}`))
    await page.goto(`${origin}/playground`)
    const key = page.getByRole('combobox', { name: label('API key'), exact: true })
    await key.waitFor()
    assert.equal(await key.inputValue(), '', 'no automatic first key')
    const input = page.getByRole('textbox', { name: label('Message'), exact: true })
    await input.fill('Synthetic unsent text')
    const send = page.getByRole('button', { name: label('Send'), exact: true })
    assert(await send.isDisabled(), 'no key must block sending')
    await input.press('Enter')
    assert.equal(sent.length, 0, 'Enter without a key must not dispatch')
    await key.selectOption('21')
    await input.fill('')
    const upload = page.getByLabel(label('Upload attachments'), { exact: true })
    await upload.setInputFiles({ name: 'sample.png', mimeType: 'image/png', buffer: png })
    await page.getByRole('img', { name: 'sample.png', exact: true }).waitFor()
    await page.waitForFunction(() => [...document.images].some(image => image.alt === 'sample.png' && image.complete && image.naturalWidth === 1))
    await page.waitForFunction(() => !document.querySelector('button[type="submit"]')?.disabled)
    assert(await send.isEnabled(), 'image-only prompt is sendable with a key')
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1), 'viewport must not overflow')
    const previewName = `${name}-preview.png`
    await page.screenshot({ path: resolve(output, previewName), fullPage: true }); report.screenshots.push(previewName)
    await send.click()
    await page.getByText('Synthetic attachment received', { exact: true }).waitFor()
    assert.equal(sent.length, 1)
    const firstParts = sent[0].messages.at(-1).content
    assert.equal(firstParts.filter(part => part.type === 'image_url').length, 1)
    assert.equal(firstParts.find(part => part.type === 'image_url').image_url.url, `data:image/png;base64,${png.toString('base64')}`)
    await page.waitForFunction(() => (localStorage.getItem('playground_messages:user:2') || '').includes('missingAttachments'))
    const stored = await page.evaluate(() => localStorage.getItem('playground_messages:user:2') || '')
    assert(stored.includes('missingAttachments'), 'persist an explicit missing-attachment marker')
    assert(!stored.includes(png.toString('base64')), 'image bytes must not persist')
    await upload.setInputFiles({ name: 'sample.pdf', mimeType: 'application/pdf', buffer: pdf })
    await page.getByRole('img', { name: 'sample.pdf', exact: true }).waitFor()
    failNext = true
    await send.click()
    await page.getByRole('button', { name: label('Remove attachment {{name}}').replace('{{name}}', 'sample.pdf'), exact: true }).waitFor()
    await send.waitFor({ state: 'visible' })
    await page.waitForFunction(() => !document.querySelector('button[type="submit"]')?.disabled)
    assert.equal(sent.length, 2)
    const errorName = `${name}-pdf-error.png`
    await page.screenshot({ path: resolve(output, errorName), fullPage: true }); report.screenshots.push(errorName)
    await send.click()
    await page.waitForFunction(() => [...document.querySelectorAll('body *')].filter(element => element.children.length === 0 && element.textContent === 'Synthetic attachment received').length >= 2)
    assert.equal(sent.length, 3, 'only explicit retry dispatches again')
    const parts = sent[2].messages.flatMap(message => Array.isArray(message.content) ? message.content : [])
    const files = parts.filter(part => part.type === 'file')
    assert.equal(files.length, 1, 'retry does not duplicate failed PDF turn')
    assert.equal(files[0].file.filename, 'sample.pdf')
    assert.equal(files[0].file.file_data, `data:application/pdf;base64,${pdf.toString('base64')}`)
    await page.reload()
    await page.getByText(label('Attachments from this message are no longer available. Remove this message or start a new conversation before sending.')).first().waitFor()
    await input.fill('Do not silently drop prior attachments')
    await send.click()
    assert.equal(sent.length, 3, 'missing attachments must not dispatch a degraded prompt')
    report.journeys.push({ name, passed: true, explicitDispatches: sent.length })
    await context.close(); persist()
  }
  assert.deepEqual(report.violations, [])
  assert.deepEqual(report.pageErrors, [])
  report.result = 'passed'
} catch (error) {
  report.result = 'failed'; report.error = String(error?.stack || error); throw error
} finally {
  persist(); await browser?.close(); await new Promise(done => server.close(done))
}
