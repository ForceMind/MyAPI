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
const keyNames = {
  en: 'Synthetic international research workspace with a long production key name',
  zh: '用于国际研究工作区和生产环境身份核验的合成长名称密钥',
  'zh-TW': '用於國際研究工作區與正式環境身分驗證的合成長名稱金鑰',
  fr: 'Clé synthétique du projet de recherche international pour environnement de production',
  ru: 'Синтетический ключ международного исследовательского проекта для рабочей среды',
  ja: '国際研究ワークスペースと本番環境の識別を確認するための長い名前の合成キー',
  vi: 'Khóa tổng hợp có tên dài cho không gian nghiên cứu quốc tế và môi trường sản xuất',
}
const keyGroup = 'international_research_group_without_breaks_1234567890'
const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aD1sAAAAASUVORK5CYII=', 'base64')
const pdf = Buffer.from('%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\n%%EOF\n')
const report = { schema: 2, commit: process.env.GITHUB_SHA || execFileSync('git', ['rev-parse', 'HEAD'], { cwd: repo, encoding: 'utf8' }).trim(), result: 'running', evidence: 'Synthetic API UI qualification; no live provider or billing claim.', nativePickerLimit: 'Touch events open the native control; choosing a native OS option is driven by Playwright selectOption, not physical-device picker automation.', keyIdentity: [], journeys: [], screenshots: [], requests: [], violations: [], pageErrors: [] }
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

// Protect the actual rendered identity, not just the absence of page overflow.
// Range rectangles prove every rendered line stays inside its field and clipping
// ancestors, including a distinguishing suffix after an unbroken long group.
async function assertKeyIdentity(key, expected, journey, state, width) {
  await key.scrollIntoViewIfNeeded()
  const measured = await key.evaluate(select => {
    const identityId = select.getAttribute('aria-describedby')
    const identity = identityId ? document.querySelector(`#${CSS.escape(identityId)}`) : null
    if (!identity) return null
    const box = select.getBoundingClientRect(), field = identity.parentElement.getBoundingClientRect()
    const range = document.createRange()
    range.selectNodeContents(identity)
    const lines = [...range.getClientRects()].filter(rect => rect.width > 0 && rect.height > 0)
    const clips = []
    for (let parent = identity.parentElement; parent; parent = parent.parentElement) {
      const style = getComputedStyle(parent)
      if (['hidden', 'clip', 'auto', 'scroll'].includes(style.overflowX) || ['hidden', 'clip', 'auto', 'scroll'].includes(style.overflowY)) {
        const rect = parent.getBoundingClientRect()
        clips.push({ rect, x: style.overflowX !== 'visible', y: style.overflowY !== 'visible' })
      }
    }
    const style = getComputedStyle(identity)
    return {
      value: select.value,
      selectedLabel: select.selectedOptions[0]?.text,
      identity: identity.textContent,
      selectWidth: box.width,
      selectHeight: box.height,
      fieldWidth: field.width,
      lineCount: lines.length,
      unclipped: lines.length > 0 && lines.every(rect => rect.left >= field.left - 1 && rect.right <= field.right + 1 && rect.left >= -1 && rect.right <= innerWidth + 1 && clips.every(clip => (!clip.x || (rect.left >= clip.rect.left - 1 && rect.right <= clip.rect.right + 1)) && (!clip.y || (rect.top >= clip.rect.top - 1 && rect.bottom <= clip.rect.bottom + 1)))),
      wraps: style.whiteSpace !== 'nowrap' && style.textOverflow !== 'ellipsis' && style.webkitLineClamp === 'none',
    }
  })
  report.keyIdentity.push({ journey, state, ...measured })
  assert(measured, `${journey}: selected key must have a visible full identity`)
  assert.equal(measured.identity, expected)
  assert.equal(measured.selectedLabel, expected, 'visible identity must match the actual selected option')
  assert(measured.unclipped && measured.wraps, `${journey}/${state}: the complete key identity must wrap without clipping`)
  assert(measured.selectWidth >= (width === 320 ? 240 : 190), 'key selector must not collapse beside the tools')
  if (width === 320) {
    assert(measured.selectHeight >= 44, 'mobile key control must be touch-sized')
    assert(measured.lineCount > 1, 'long mobile identity must occupy multiple readable lines')
  }
}

let browser, activePage, activeJourney, activeStep
try {
  await new Promise(done => server.listen(0, '127.0.0.1', done))
  const origin = `http://127.0.0.1:${server.address().port}`
  const { chromium } = await import(process.env.MYAPI_PLAYWRIGHT_MODULE ? pathToFileURL(process.env.MYAPI_PLAYWRIGHT_MODULE).href : 'playwright')
  browser = await chromium.launch({ headless: true, executablePath: process.env.MYAPI_CHROMIUM_PATH || undefined, args: ['--disable-dev-shm-usage'] })
  for (const { width, language } of [{ width: 1280, language: 'en' }, ...Object.keys(locales).map(language => ({ width: 320, language }))]) {
    const name = `chat-attachments-${language}-${width}`
    activeJourney = name
    activeStep = 'load composer and select key'
    const label = key => translation[language][key] || key
    const fixture = createUIFixture({ role: 1, language: locales[language] })
    const context = await browser.newContext({ viewport: { width, height: 900 }, hasTouch: width === 320, isMobile: width === 320, reducedMotion: 'reduce', serviceWorkers: 'block' })
    await context.addInitScript(code => localStorage.setItem('i18nextLng', code), locales[language])
    const sent = []
    let failNext = false
    let releaseModels, releaseChat
    const modelsReady = new Promise(resolve => { releaseModels = resolve })
    const chatReady = new Promise(resolve => { releaseChat = resolve })
    const primaryName = `${keyNames[language]} · primary-21`
    const secondaryName = `${keyNames[language]} · secondary-22`
    const primaryIdentity = `${primaryName} · ${keyGroup}`
    const secondaryIdentity = `${secondaryName} · ${keyGroup}`
    await context.route('**/*', async route => {
      const request = route.request(), url = new URL(request.url())
      if (url.origin !== origin) { if (['data:', 'blob:'].includes(url.protocol)) return route.continue(); report.violations.push(`${name}: external request`); return route.abort() }
      if (url.pathname === '/pg/chat/completions') {
        assert.equal(request.method(), 'POST')
        assert.equal(request.headers()['x-myapi-key-id'], '22')
        const payload = request.postDataJSON()
        assert(!('group' in payload), 'body must not override key group')
        sent.push(payload)
        if (failNext) { failNext = false; return route.fulfill({ status: 400, json: { error: { code: 'playground_file_provider_unsupported', message: 'Synthetic unsupported PDF route' } } }) }
        await chatReady
        const chunk = { id: 'synthetic-chat', object: 'chat.completion.chunk', model: payload.model, choices: [{ index: 0, delta: { content: 'Synthetic attachment received' }, finish_reason: null }] }
        return route.fulfill({ status: 200, contentType: 'text/event-stream', body: `data: ${JSON.stringify(chunk)}\n\ndata: [DONE]\n\n` })
      }
      if (url.pathname === '/pg/models') {
        assert(['21', '22'].includes(request.headers()['x-myapi-key-id']), 'models must belong to an explicitly selected available key')
        await modelsReady
      }
      if (url.pathname.startsWith('/api/') || url.pathname.startsWith('/pg/')) {
        const result = fixture.response(url, request.method())
        if (url.pathname === '/pg/keys') {
          const base = result.body.data.items[0]
          result.body.data.items = [
            { ...base, name: primaryName, group: keyGroup },
            { ...base, id: 22, name: secondaryName, group: keyGroup },
            { ...base, id: 23, name: 'Synthetic expired key', expired_time: 1 },
          ]
          result.body.data.total = 3
        }
        report.requests.push({ journey: name, method: request.method(), path: url.pathname, status: result.status || 501 })
        if (result.violation) report.violations.push(`${name}: ${result.violation}`)
        return route.fulfill({ status: result.status || 501, json: result.body || { success: false } })
      }
      if (request.method() !== 'GET') { report.violations.push(`${name}: unexpected ${request.method()}`); return route.abort() }
      return route.continue()
    })
    const page = await context.newPage()
    activePage = page
    page.setDefaultTimeout(15000)
    await page.clock.setFixedTime(new Date(FIXTURE_TIME))
    page.on('pageerror', error => report.pageErrors.push(`${name}: ${error.message}`))
    await page.goto(`${origin}/playground`)
    const key = page.getByRole('combobox', { name: label('API key'), exact: true })
    await key.waitFor()
    await page.waitForFunction(() => document.querySelector('select')?.options.length === 4)
    assert.equal(await key.inputValue(), '', 'no automatic first key')
    const input = page.getByRole('textbox', { name: label('Message'), exact: true })
    await input.fill('Synthetic unsent text')
    const send = page.getByRole('button', { name: label('Send'), exact: true })
    assert(await send.isDisabled(), 'no key must block sending')
    await input.press('Enter')
    assert.equal(sent.length, 0, 'Enter without a key must not dispatch')
    activeStep = 'keyboard choice keeps full identity while models load'
    await key.focus()
    // Native selects commit End/arrow choices directly without a custom menu.
    // The last option is expired: keyboard navigation must skip it.
    await key.press('End')
    assert.equal(await key.inputValue(), '22')
    assert(await send.isDisabled(), 'a selected key without loaded models must not enable Send')
    await assertKeyIdentity(key, secondaryIdentity, name, 'keyboard-model-loading', width)
    releaseModels()
    await send.click({ trial: true })
    activeStep = 'touch native selector then select and clear explicitly'
    if (width === 320) {
      await key.evaluate(select => select.addEventListener('touchend', () => select.dataset.touchOpened = 'true', { once: true }))
      await key.tap()
      await key.press('Escape')
      assert.equal(await key.getAttribute('data-touch-opened'), 'true', 'a real touch event must reach the key control')
      assert.equal(await key.inputValue(), '22', 'dismissing the native picker preserves identity')
    }
    // Native popup choices belong to the browser/OS, not the page DOM.
    await key.selectOption('21')
    await assertKeyIdentity(key, primaryIdentity, name, 'native-selection-api-after-touch', width)
    await key.selectOption('')
    assert.equal(await key.inputValue(), '')
    assert.equal(await key.getAttribute('aria-describedby'), null)
    assert(await send.isDisabled(), 'clearing the key disables Send again')
    await input.press('Enter')
    assert.equal(sent.length, 0, 'clearing the key must also block Enter')
    await key.focus()
    await key.press('End')
    assert.equal(await key.inputValue(), '22')
    await assertKeyIdentity(key, secondaryIdentity, name, 'keyboard-reselected', width)
    await input.fill('')
    activeStep = 'preview and send image-only prompt'
    const upload = page.getByLabel(label('Upload attachments'), { exact: true })
    await upload.setInputFiles({ name: 'sample.png', mimeType: 'image/png', buffer: png })
    await page.getByRole('img', { name: 'sample.png', exact: true }).waitFor()
    await page.waitForFunction(() => [...document.images].some(image => image.alt === 'sample.png' && image.complete && image.naturalWidth === 1))
    // Wait on the named composer control, not an unrelated submit button.
    // Trial performs all actionability checks without dispatching a request.
    await send.click({ trial: true })
    assert(await send.isEnabled(), 'image-only prompt is sendable with a key')
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1), 'viewport must not overflow')
    const previewName = `${name}-preview.png`
    await page.screenshot({ path: resolve(output, previewName), fullPage: true }); report.screenshots.push(previewName)
    await send.click()
    const stop = page.getByRole('button', { name: label('Stop'), exact: true })
    await stop.waitFor()
    assert(await stop.isEnabled(), 'pending requests expose an enabled Stop control')
    assert(await key.isDisabled(), 'in-flight key identity cannot change')
    assert(await input.isDisabled(), 'pending request disables the composer')
    assert.equal(await send.count(), 0, 'pending request cannot be double-sent')
    await assertKeyIdentity(key, secondaryIdentity, name, 'request-pending', width)
    const pendingName = `${name}-key-pending.png`
    await page.screenshot({ path: resolve(output, pendingName), fullPage: true }); report.screenshots.push(pendingName)
    releaseChat()
    await page.getByText('Synthetic attachment received', { exact: true }).waitFor()
    await send.waitFor({ state: 'visible' })
    await page.waitForFunction(name => [...document.querySelectorAll('button')].some(button => button.getAttribute('aria-label') === name && button.disabled), label('Send'))
    assert(await send.isDisabled(), 'successful attachment-only send clears the draft')
    assert(await key.isEnabled(), 'key selection is restored after completion')
    await assertKeyIdentity(key, secondaryIdentity, name, 'request-complete', width)
    assert.equal(sent.length, 1)
    const firstParts = sent[0].messages.at(-1).content
    assert.equal(firstParts.filter(part => part.type === 'image_url').length, 1)
    assert.equal(firstParts.find(part => part.type === 'image_url').image_url.url, `data:image/png;base64,${png.toString('base64')}`)
    await page.waitForFunction(() => (localStorage.getItem('playground_messages:user:2') || '').includes('missingAttachments'))
    const stored = await page.evaluate(() => localStorage.getItem('playground_messages:user:2') || '')
    assert(stored.includes('missingAttachments'), 'persist an explicit missing-attachment marker')
    assert(!stored.includes(png.toString('base64')), 'image bytes must not persist')
    activeStep = 'reject PDF and explicitly retry preserved draft'
    await upload.setInputFiles({ name: 'sample.pdf', mimeType: 'application/pdf', buffer: pdf })
    await page.getByRole('img', { name: 'sample.pdf', exact: true }).waitFor()
    failNext = true
    await send.click()
    await page.getByRole('button', { name: label('Remove attachment {{name}}').replace('{{name}}', 'sample.pdf'), exact: true }).waitFor()
    await send.waitFor({ state: 'visible' })
    await send.click({ trial: true })
    assert.equal(sent.length, 2)
    await assertKeyIdentity(key, secondaryIdentity, name, 'request-error-draft-preserved', width)
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
    activeStep = 'reload and reject unavailable attachment history'
    await page.reload()
    await page.getByText(label('Attachments from this message are no longer available. Remove this message or start a new conversation before sending.')).first().waitFor()
    await input.fill('Do not silently drop prior attachments')
    await send.click()
    assert.equal(sent.length, 3, 'missing attachments must not dispatch a degraded prompt')
    report.journeys.push({ name, passed: true, explicitDispatches: sent.length, keyboardSelection: true, touchControl: width === 320, nativeOptionSelection: 'Playwright selectOption', sendStates: ['no-key', 'models-loading', 'ready', 'pending', 'complete', 'error'] })
    await context.close(); activePage = null; persist()
  }
  assert.deepEqual(report.violations, [])
  assert.deepEqual(report.pageErrors, [])
  report.result = 'passed'
} catch (error) {
  report.result = 'failed'; report.error = String(error?.stack || error)
  report.failure = { journey: activeJourney, step: activeStep }
  if (activePage && !activePage.isClosed()) {
    try {
      report.failure.state = await activePage.evaluate(() => ({
        path: location.pathname,
        notices: [...document.querySelectorAll('[role="status"], [role="alert"]')].map(element => element.textContent),
        selects: [...document.querySelectorAll('select')].map(element => ({ label: element.getAttribute('aria-label'), value: element.value, disabled: element.disabled, options: [...element.options].map(option => ({ value: option.value, text: option.text, disabled: option.disabled })) })),
        buttons: [...document.querySelectorAll('button')].map(element => ({ label: element.getAttribute('aria-label'), text: element.textContent, type: element.type, disabled: element.disabled })),
        images: [...document.images].map(element => ({ alt: element.alt, complete: element.complete, naturalWidth: element.naturalWidth })),
      }))
      const failureName = `${activeJourney}-failure.png`
      await activePage.screenshot({ path: resolve(output, failureName), fullPage: true, timeout: 5000 })
      report.screenshots.push(failureName)
    } catch (diagnosticError) {
      report.failure.diagnosticError = String(diagnosticError)
    }
  }
  throw error
} finally {
  persist(); await browser?.close(); await new Promise(done => server.close(done))
}
