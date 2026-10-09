// Production-build chat-client acceptance with synthetic, intercepted APIs.
// No provider calls, real credentials, billing, or deployment acceptance.
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
const gif = Buffer.from('R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7', 'base64')
const pdf = Buffer.from('%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\n%%EOF\n')
// This buffer is rejected before FileReader; no oversized body reaches the API.
const oversized = Buffer.alloc(10 * 1024 * 1024 + 1)
const invalidFiles = [
  { name: 'unsupported-type', files: [{ name: 'notes.txt', mimeType: 'text/plain', buffer: Buffer.from('Synthetic unsupported text file') }], error: 'Only PNG, JPEG, WEBP, GIF and PDF files are supported' },
  { name: 'empty-file', files: [{ name: 'empty.png', mimeType: 'image/png', buffer: Buffer.alloc(0) }], error: 'Each file must be non-empty and no larger than 10 MiB' },
  { name: 'mismatched-signature', files: [{ name: 'spoofed.png', mimeType: 'image/png', buffer: Buffer.from('This is not a PNG') }], error: 'The file content does not match its type' },
  { name: 'invalid-pdf-filename', files: [{ name: 'notes.txt', mimeType: 'application/pdf', buffer: pdf }], error: 'PDF filenames must end in .pdf, fit within 255 bytes and contain no path separators' },
  { name: 'oversized-file', files: [{ name: 'oversized.png', mimeType: 'image/png', buffer: oversized }], error: 'Each file must be non-empty and no larger than 10 MiB' },
  { name: 'too-many-files', files: Array.from({ length: 5 }, (_, index) => ({ name: `extra-${index}.png`, mimeType: 'image/png', buffer: png })), error: 'You can attach up to 4 files' },
]
const report = { schema: 3, commit: process.env.GITHUB_SHA || execFileSync('git', ['rev-parse', 'HEAD'], { cwd: repo, encoding: 'utf8' }).trim(), result: 'running', evidence: 'Synthetic API UI qualification; no live provider or billing claim.', nativePickerLimit: 'Touch events open the native control; choosing a native OS option is driven by Playwright selectOption, not physical-device picker automation.', cancellationScope: 'Actual Stop click aborts a held streaming HTTP request before response headers; partial-stream cancellation is covered separately by lifecycle tests.', keyIdentity: [], attachmentValidation: [], attachmentRemoval: [], multipleImages: [], cancellations: [], usageInspection: [], strictText: [], journeys: [], screenshots: [], chatDispatches: [], requests: [], violations: [], pageErrors: [] }
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
    let effectiveOpacity = Number(getComputedStyle(identity).opacity)
    const dimmedAncestors = []
    for (let parent = identity.parentElement; parent; parent = parent.parentElement) {
      const style = getComputedStyle(parent)
      effectiveOpacity *= Number(style.opacity)
      if (Number(style.opacity) < 1) dimmedAncestors.push({ element: parent.dataset.slot || parent.tagName, opacity: Number(style.opacity) })
      if (['hidden', 'clip', 'auto', 'scroll'].includes(style.overflowX) || ['hidden', 'clip', 'auto', 'scroll'].includes(style.overflowY)) {
        const rect = parent.getBoundingClientRect()
        clips.push({ rect, x: style.overflowX !== 'visible', y: style.overflowY !== 'visible' })
      }
    }
    const style = getComputedStyle(identity)
    return {
      value: select.value,
      identityId,
      effectiveOpacity,
      dimmedAncestors,
      theme: document.documentElement.classList.contains('dark') ? 'dark' : 'light',
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
  const evidence = { journey, state, ...measured }
  report.keyIdentity.push(evidence)
  assert(measured, `${journey}: selected key must have a visible full identity`)
  assert(measured.effectiveOpacity >= 0.99, `${journey}/${state}: the identity must stay fully opaque, including its ancestors`)
  const identity = key.page().locator(`[id=${JSON.stringify(measured.identityId)}]`)
  evidence.contrast = await assertTextContrast(identity, `${journey}/${state} selected key identity`, measured.theme)
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
    let failNext = false, holdNext = false
    let expectedKeyId = '22'
    let releaseModels, releaseChat, reachCanceledResponse, releaseCanceledResponse, finishCanceledResponse
    const modelsReady = new Promise(resolve => { releaseModels = resolve })
    const chatReady = new Promise(resolve => { releaseChat = resolve })
    const canceledResponseReached = new Promise(resolve => { reachCanceledResponse = resolve })
    const canceledResponseReady = new Promise(resolve => { releaseCanceledResponse = resolve })
    const canceledResponseFinished = new Promise(resolve => { finishCanceledResponse = resolve })
    const primaryName = `${keyNames[language]} · primary-21`
    const secondaryName = `${keyNames[language]} · secondary-22`
    const strictName = `${keyNames[language]} · strict-24`
    const primaryIdentity = `${primaryName} · ${keyGroup}`
    const secondaryIdentity = `${secondaryName} · ${keyGroup}`
    const strictIdentity = `${strictName} · ${keyGroup}`
    await context.route('**/*', async route => {
      const request = route.request(), url = new URL(request.url())
      if (url.origin !== origin) { if (['data:', 'blob:'].includes(url.protocol)) return route.continue(); report.violations.push(`${name}: external request`); return route.abort() }
      if (url.pathname === '/pg/chat/completions') {
        assert.equal(request.method(), 'POST')
        assert.equal(request.headers()['x-myapi-key-id'], expectedKeyId, 'dispatch retains the explicitly selected key')
        const payload = request.postDataJSON()
        assert(!('group' in payload), 'body must not override key group')
        sent.push(payload)
        const dispatch = sent.length
        report.chatDispatches.push({ journey: name, dispatch, keyId: request.headers()['x-myapi-key-id'], model: payload.model, stream: payload.stream, held: holdNext, messages: payload.messages.map(message => ({ role: message.role, contentTypes: Array.isArray(message.content) ? message.content.map(part => part.type) : ['text'] })) })
        if (expectedKeyId === '24') {
          assert.equal(payload.model, 'gpt-6.1-sol')
          assert.equal(payload.stream, true)
          assert.equal(payload.service_tier, 'default')
          assert.deepEqual(payload.stream_options, { include_usage: true })
          assert(Number.isInteger(payload.max_completion_tokens) && payload.max_completion_tokens >= 1 && payload.max_completion_tokens <= 128000)
          for (const field of ['max_tokens', 'temperature', 'top_p', 'frequency_penalty', 'presence_penalty', 'seed']) assert(!(field in payload), `strict text envelope omits ${field}`)
          if (payload.messages.some(message => Array.isArray(message.content))) {
            assert.equal(payload.messages.at(-1).content.filter(part => part.type === 'image_url').length, 1, 'strict fixture receives the image instead of a silent text-only downgrade')
            return route.fulfill({ status: 400, json: { error: { code: 'token_budget_unsupported_request', message: 'Synthetic strict budget media rejection' } } })
          }
          assert.deepEqual(payload.messages, [{ role: 'user', content: 'Synthetic strict text' }], 'explicit retry replaces the rejected media turn')
          const chunk = { id: 'synthetic-strict-chat', object: 'chat.completion.chunk', model: payload.model, service_tier: 'default', choices: [{ index: 0, delta: { content: 'Synthetic strict text accepted' }, finish_reason: 'stop' }] }
          const usage = { id: 'synthetic-strict-chat', object: 'chat.completion.chunk', model: payload.model, service_tier: 'default', choices: [], usage: { prompt_tokens: 4, completion_tokens: 4, total_tokens: 8 } }
          return route.fulfill({ status: 200, contentType: 'text/event-stream', body: `data: ${JSON.stringify(chunk)}\n\ndata: ${JSON.stringify(usage)}\n\ndata: [DONE]\n\n` })
        }
        if (failNext) { failNext = false; return route.fulfill({ status: 400, json: { error: { code: 'playground_file_provider_unsupported', message: 'Synthetic unsupported PDF route' } } }) }
        if (holdNext) {
          holdNext = false
          assert.equal(payload.stream, true, 'cancellation exercises the production streaming transport')
          reachCanceledResponse(request)
          await canceledResponseReady
          const chunk = { id: 'synthetic-canceled-chat', object: 'chat.completion.chunk', model: payload.model, choices: [{ index: 0, delta: { content: 'Synthetic late canceled response must not appear' }, finish_reason: null }] }
          try {
            await route.fulfill({ status: 200, contentType: 'text/event-stream', body: `data: ${JSON.stringify(chunk)}\n\ndata: [DONE]\n\n` })
          } finally {
            finishCanceledResponse()
          }
          return
        }
        await chatReady
        const content = dispatch <= 3 ? 'Synthetic attachment received' : `Synthetic attachment received ${dispatch}`
        const chunk = { id: 'synthetic-chat', object: 'chat.completion.chunk', model: payload.model, choices: [{ index: 0, delta: { content }, finish_reason: null }] }
        return route.fulfill({ status: 200, contentType: 'text/event-stream', body: `data: ${JSON.stringify(chunk)}\n\ndata: [DONE]\n\n` })
      }
      if (url.pathname === '/pg/models') {
        assert(['21', '22', '24'].includes(request.headers()['x-myapi-key-id']), 'models must belong to an explicitly selected available key')
        await modelsReady
      }
      if (url.pathname.startsWith('/api/') || url.pathname.startsWith('/pg/')) {
        const result = fixture.response(url, request.method())
        if (url.pathname === '/pg/models' && request.headers()['x-myapi-key-id'] === '24' && !result.violation) {
          result.body.data = [{ id: 'gpt-6.1-sol', object: 'model', owned_by: 'openai' }]
        }
        if (url.pathname === '/pg/keys') {
          const base = result.body.data.items[0]
          result.body.data.items = [
            { ...base, name: primaryName, group: keyGroup },
            { ...base, id: 24, name: strictName, group: keyGroup, strict_token_budget: true },
            { ...base, id: 22, name: secondaryName, group: keyGroup },
            { ...base, id: 23, name: 'Synthetic expired key', expired_time: 1 },
          ]
          result.body.data.total = 4
        }
        if (url.pathname === '/api/log/self' && !result.violation) {
          // Deliberately synthetic attribution for UI inspection only. Actual
          // TokenId/quota attribution is proved by the backend relay tests.
          result.body.data.items = [{ id: 922, user_id: fixture.user.id, username: fixture.user.username, created_at: FIXTURE_TIME / 1000, type: 2, content: '', token_id: 22, token_name: secondaryName, model_name: 'synthetic-text-model', group: keyGroup, quota: 100, prompt_tokens: 12, completion_tokens: 8, use_time: 1, is_stream: true, request_id: 'synthetic-playground-key-22', other: '{}' }]
          result.body.data.total = 1
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
    await page.waitForFunction(() => document.querySelector('select')?.options.length === 5)
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
    const upload = page.getByLabel(label('Upload attachments'), { exact: true })
    const composer = page.locator('form').filter({ has: upload })
    const removeAttachment = filename => composer.getByRole('button', { name: label('Remove attachment {{name}}').replace('{{name}}', filename), exact: true })
    activeStep = 'reject invalid files without dispatch or partial draft changes'
    for (const invalid of invalidFiles) {
      activeStep = `reject invalid files: ${invalid.name}`
      await upload.setInputFiles(invalid.files)
      await page.getByRole('alert').filter({ hasText: label(invalid.error) }).waitFor()
      assert.equal(await composer.getByRole('img').count(), 0, `${invalid.name}: rejected files never enter the draft`)
      assert(await input.isEnabled(), `${invalid.name}: validation releases the composer`)
      assert(await send.isDisabled(), `${invalid.name}: rejected files cannot enable an empty send`)
      assert.equal(await upload.inputValue(), '', 'file picker resets after rejection')
      await input.press('Enter')
      assert.equal(sent.length, 0, `${invalid.name}: no request is dispatched`)
      report.attachmentValidation.push({ journey: name, case: invalid.name, message: label(invalid.error), noDispatch: true })
    }
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), 'localized invalid-file message must not overflow')
    const invalidName = `${name}-invalid-files.png`
    await page.screenshot({ path: resolve(output, invalidName), fullPage: true }); report.screenshots.push(invalidName)

    activeStep = 'remove the last image and select the same file again'
    await upload.setInputFiles({ name: 'sample.png', mimeType: 'image/png', buffer: png })
    await removeAttachment('sample.png').click()
    assert.equal(await composer.getByRole('img').count(), 0)
    assert(await send.isDisabled(), 'removing the last attachment disables an empty send')
    await input.press('Enter')
    assert.equal(sent.length, 0, 'removal and Enter never dispatch an empty prompt')
    report.attachmentRemoval.push({ journey: name, case: 'last-attachment', remaining: 0, noDispatch: true })
    const removedName = `${name}-removed-last.png`
    await page.screenshot({ path: resolve(output, removedName), fullPage: true }); report.screenshots.push(removedName)

    activeStep = 'preview and send image-only prompt'
    await upload.setInputFiles({ name: 'sample.png', mimeType: 'image/png', buffer: png })
    await page.getByRole('img', { name: 'sample.png', exact: true }).waitFor()
    await page.waitForFunction(() => [...document.images].some(image => image.alt === 'sample.png' && image.complete && image.naturalWidth === 1))
    await upload.setInputFiles([
      { name: 'not-partially-added.gif', mimeType: 'image/gif', buffer: gif },
      { name: 'spoofed.png', mimeType: 'image/png', buffer: Buffer.from('This is not a PNG') },
    ])
    await page.getByRole('alert').filter({ hasText: label('The file content does not match its type') }).waitFor()
    assert.equal(await composer.getByRole('img').count(), 1, 'invalid mixed batch leaves the existing image intact and adds no partial files')
    assert.equal(await composer.getByRole('img', { name: 'sample.png', exact: true }).getAttribute('src'), `data:image/png;base64,${png.toString('base64')}`)
    assert.equal(sent.length, 0)
    report.attachmentValidation.push({ journey: name, case: 'invalid-mixed-batch-preserves-existing', acceptedImages: 1, noDispatch: true })
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

    activeStep = 'Stop aborts a held streaming request and preserves its draft'
    await input.fill('Synthetic draft to cancel')
    await upload.setInputFiles({ name: 'cancel.png', mimeType: 'image/png', buffer: png })
    await removeAttachment('cancel.png').click({ trial: true })
    holdNext = true
    const canceledRequestReady = page.waitForRequest(request => new URL(request.url()).pathname === '/pg/chat/completions')
    await send.click()
    const canceledRequest = await canceledRequestReady
    assert.equal(await canceledResponseReached, canceledRequest, 'Stop targets the request actually held by the fixture')
    await stop.waitFor()
    assert.equal(sent.length, 4)
    assert(await input.isDisabled() && await key.isDisabled(), 'the held request locks its composer and identity')
    assert(await removeAttachment('cancel.png').isDisabled(), 'pending attachments cannot be removed')
    assert.equal(await send.count(), 0, 'the held request cannot be resubmitted')
    await assertKeyIdentity(key, secondaryIdentity, name, 'cancel-request-pending', width)
    const cancelPendingName = `${name}-cancel-pending.png`
    await page.screenshot({ path: resolve(output, cancelPendingName), fullPage: true }); report.screenshots.push(cancelPendingName)
    // Observe the browser's transport failure, not just a disappearing spinner.
    // Releasing the fixture after Stop also probes isolation from a late reply.
    const abortedRequestReady = page.waitForEvent('requestfailed', { predicate: request => request === canceledRequest })
    await stop.click()
    releaseCanceledResponse()
    const abortedRequest = await abortedRequestReady
    await canceledResponseFinished
    assert.match(abortedRequest.failure()?.errorText || '', /aborted/i, 'Stop must abort the real browser request')
    assert.equal(sent.length, 4, 'the Stop click must not submit a replacement request')
    await send.click({ trial: true })
    assert.equal(await stop.count(), 0)
    assert(await input.isEnabled() && await key.isEnabled(), 'Stop restores usable controls')
    assert.equal(await input.inputValue(), 'Synthetic draft to cancel', 'cancel does not clear an unsent draft')
    assert.equal(await composer.getByRole('img', { name: 'cancel.png', exact: true }).getAttribute('src'), `data:image/png;base64,${png.toString('base64')}`)
    assert.equal(sent.length, 4, 'Stop does not automatically replay the request')
    assert.equal(await page.getByText('Synthetic late canceled response must not appear', { exact: true }).count(), 0)
    await assertKeyIdentity(key, secondaryIdentity, name, 'request-canceled-draft-preserved', width)
    const canceledName = `${name}-canceled-draft.png`
    await page.screenshot({ path: resolve(output, canceledName), fullPage: true }); report.screenshots.push(canceledName)

    activeStep = 'edit a canceled draft and explicitly send a new text-only turn'
    await removeAttachment('cancel.png').click()
    await input.fill('Synthetic new text after cancellation')
    assert.equal(await composer.getByRole('img').count(), 0)
    assert.equal(sent.length, 4, 'editing/removing a canceled draft never dispatches')
    report.attachmentRemoval.push({ journey: name, case: 'canceled-draft', remaining: 0, noDispatch: true })
    await send.click()
    await page.getByText('Synthetic attachment received 5', { exact: true }).waitFor()
    await send.waitFor({ state: 'visible' })
    await page.waitForFunction(name => [...document.querySelectorAll('button')].some(button => button.getAttribute('aria-label') === name && button.disabled), label('Send'))
    assert.equal(sent.length, 5, 'only the new explicit send dispatches after Stop')
    assert.equal(sent[4].messages.at(-1).content, 'Synthetic new text after cancellation')
    assert(!JSON.stringify(sent[4]).includes('Synthetic draft to cancel'), 'new send replaces the canceled draft tail')
    assert.equal(sent[4].messages.filter(message => message.role === 'user').length, 3, 'the canceled user turn must not be duplicated')
    assert.equal(sent[4].messages.flatMap(message => Array.isArray(message.content) ? message.content : []).filter(part => part.type === 'image_url').length, 1, 'removing the canceled image preserves only the earlier successful image')
    assert.equal(await input.inputValue(), '')
    await assertKeyIdentity(key, secondaryIdentity, name, 'post-cancel-explicit-send-complete', width)
    report.cancellations.push({ journey: name, transport: 'streaming XHR held before response headers', browserFailure: abortedRequest.failure().errorText, draftPreserved: true, lateResponseIgnored: true, automaticReplay: false, replacementDispatch: 5 })

    activeStep = 'preview multiple images, remove one, and send exact remaining bytes'
    await upload.setInputFiles([
      { name: 'first.png', mimeType: 'image/png', buffer: png },
      { name: 'remove-me.png', mimeType: 'image/png', buffer: png },
      { name: 'second.gif', mimeType: 'image/gif', buffer: gif },
    ])
    await removeAttachment('remove-me.png').click({ trial: true })
    assert.equal(await composer.getByRole('img').count(), 3)
    await page.waitForFunction(() => ['first.png', 'remove-me.png', 'second.gif'].every(name => [...document.images].some(image => image.alt === name && image.complete && image.naturalWidth === 1)))
    await removeAttachment('remove-me.png').click()
    assert.equal(await composer.getByRole('img').count(), 2)
    assert.equal(await composer.getByRole('img', { name: 'remove-me.png', exact: true }).count(), 0)
    assert.equal(sent.length, 5, 'removal from multiple images does not dispatch')
    const previewBounds = await composer.getByRole('img').evaluateAll(images => images.map(image => {
      const rect = image.getBoundingClientRect()
      return { name: image.alt, width: rect.width, height: rect.height, inViewport: rect.left >= 0 && rect.right <= innerWidth + 1 }
    }))
    assert(previewBounds.every(image => image.width > 0 && image.height > 0 && image.inViewport), 'both image previews remain visible within the viewport')
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), 'multiple-image draft must not overflow')
    await assertKeyIdentity(key, secondaryIdentity, name, 'multiple-images-ready', width)
    const multipleName = `${name}-multiple-images.png`
    await page.screenshot({ path: resolve(output, multipleName), fullPage: true }); report.screenshots.push(multipleName)
    await send.click()
    await page.getByText('Synthetic attachment received 6', { exact: true }).waitFor()
    await send.waitFor({ state: 'visible' })
    await page.waitForFunction(name => [...document.querySelectorAll('button')].some(button => button.getAttribute('aria-label') === name && button.disabled), label('Send'))
    assert.equal(sent.length, 6)
    const multipleParts = sent[5].messages.at(-1).content
    assert.deepEqual(multipleParts, [
      { type: 'text', text: '' },
      { type: 'image_url', image_url: { url: `data:image/png;base64,${png.toString('base64')}` } },
      { type: 'image_url', image_url: { url: `data:image/gif;base64,${gif.toString('base64')}` } },
    ], 'multiple-image-only payload contains precisely the remaining files in selection order')
    assert.equal(await composer.getByRole('img').count(), 0, 'successful multi-image send clears only the composer attachments')
    report.attachmentRemoval.push({ journey: name, case: 'middle-of-multiple-images', remaining: 2, noDispatch: true })
    report.multipleImages.push({ journey: name, sentImages: 2, mimeTypes: ['image/png', 'image/gif'], exactBytesAndOrder: true, removedImageOmitted: true, previewBounds })

    activeStep = 'reload and reject unavailable attachment history'
    await page.waitForFunction(() => {
      const stored = localStorage.getItem('playground_messages:user:2') || '{}'
      return stored.includes('Synthetic attachment received 6') && JSON.parse(stored).data?.filter(message => message.missingAttachments).length === 3
    })
    await page.reload()
    await page.getByText(label('Attachments from this message are no longer available. Remove this message or start a new conversation before sending.')).first().waitFor()
    await input.fill('Do not silently drop prior attachments')
    await send.click()
    await page.getByText(label('Attachments from a previous session are unavailable. Remove those messages or start a new conversation.'), { exact: true }).waitFor()
    assert.equal(sent.length, 6, 'missing attachments must not dispatch a degraded prompt')
    const finalStorage = await page.evaluate(() => localStorage.getItem('playground_messages:user:2') || '')
    for (const bytes of [png, gif, pdf]) assert(!finalStorage.includes(bytes.toString('base64')), 'no image or PDF bytes persist after the full journey')

    activeStep = 'inspect the selected key in same-owner synthetic usage logs'
    const usageResponseReady = page.waitForResponse(response => {
      const url = new URL(response.url())
      return url.pathname === '/api/log/self' && url.searchParams.get('token_name') === secondaryName && response.status() === 200
    })
    await page.goto(`${origin}/usage-logs/common?token=${encodeURIComponent(secondaryName)}`)
    const usageResponse = await usageResponseReady
    const usage = (await usageResponse.json()).data.items[0]
    assert.equal(usage.user_id, fixture.user.id)
    assert.equal(usage.token_id, 22)
    assert.equal(usage.token_name, secondaryName)
    await page.getByText(secondaryName, { exact: true }).first().waitFor()
    await page.getByText('synthetic-text-model', { exact: true }).first().waitFor()
    await page.getByTitle(label('Click to view full details'), { exact: true }).click()
    const usageDetails = page.getByRole('dialog')
    await usageDetails.getByText(secondaryName, { exact: true }).waitFor()
    await usageDetails.getByText('synthetic-playground-key-22', { exact: true }).waitFor()
    await usageDetails.getByText(secondaryName, { exact: true }).scrollIntoViewIfNeeded()
    assert.equal(sent.length, 6, 'inspection and navigation cannot replay a canceled or restored chat')
    const usageName = `${name}-selected-key-usage.png`
    await page.screenshot({ path: resolve(output, usageName), fullPage: true }); report.screenshots.push(usageName)
    report.usageInspection.push({ journey: name, userId: usage.user_id, selectedKeyId: usage.token_id, displayedKeyName: secondaryName, filter: 'token_name', endpoint: '/api/log/self', syntheticOnly: true })

    activeStep = 'prepare an ordinary key preference before explicit strict-key selection'
    await page.goto(`${origin}/playground`)
    await page.getByRole('button', { name: label('Clear chat history'), exact: true }).click()
    await page.getByRole('alertdialog').getByRole('button', { name: label('Clear'), exact: true }).click()
    await page.getByRole('alertdialog').waitFor({ state: 'hidden' })
    assert.equal(await key.inputValue(), '22', 'clearing history never changes the selected key')
    const parameters = page.getByRole('button', { name: label('Parameters'), exact: true })
    const enabledParameter = parameter => page.getByRole('switch', { name: label('Enable {{parameter}}').replace('{{parameter}}', label(parameter)), exact: true })
    const maxTokens = page.getByRole('spinbutton', { name: label('Max Tokens'), exact: true })
    const seed = page.getByRole('spinbutton', { name: label('Seed'), exact: true })
    await parameters.click()
    assert.equal(await enabledParameter('Max Tokens').getAttribute('aria-checked'), 'false')
    assert(await maxTokens.isDisabled(), 'ordinary-key optional limit starts disabled')
    await enabledParameter('Seed').click()
    await seed.fill('42')
    assert.equal(await seed.inputValue(), '42')
    const ordinaryPreferences = await page.evaluate(() => ({
      enabled: JSON.parse(localStorage.getItem('playground_parameter_enabled:user:2') || 'null')?.data,
      config: JSON.parse(localStorage.getItem('playground_config:user:2') || 'null')?.data,
    }))
    assert.equal(ordinaryPreferences.enabled.seed, true)
    assert.equal(ordinaryPreferences.config.seed, 42)
    await page.keyboard.press('Escape')
    await maxTokens.waitFor({ state: 'hidden' })
    await input.fill('Synthetic strict text')
    await key.selectOption('')
    assert(await send.isDisabled(), 'a new strict journey still needs an explicit key')
    await input.press('Enter')
    assert.equal(sent.length, 6, 'clearing the key cannot dispatch or choose a fallback')
    expectedKeyId = '24'
    await key.selectOption('24')
    await send.click({ trial: true })
    assert.equal(await key.inputValue(), '24')
    await assertKeyIdentity(key, strictIdentity, name, 'strict-key-selected', width)

    activeStep = 'inspect required strict text limit and disabled sampling controls'
    await parameters.click()
    await maxTokens.waitFor()
    assert(await maxTokens.isEnabled(), 'strict Max Tokens value remains editable')
    assert.equal(await maxTokens.getAttribute('min'), '1')
    assert.equal(await maxTokens.getAttribute('max'), '128000')
    const strictLimit = Number(await maxTokens.inputValue())
    assert(Number.isInteger(strictLimit) && strictLimit >= 1 && strictLimit <= 128000)
    assert.equal(await enabledParameter('Max Tokens').getAttribute('aria-checked'), 'true')
    assert.equal(await enabledParameter('Max Tokens').getAttribute('aria-disabled'), 'true', 'required max-token switch cannot be turned off')
    for (const parameter of ['Temperature', 'Top P', 'Frequency Penalty', 'Presence Penalty', 'Seed']) {
      assert.equal(await enabledParameter(parameter).getAttribute('aria-checked'), 'false', `strict ${parameter} is not advertised as active`)
      assert.equal(await enabledParameter(parameter).getAttribute('aria-disabled'), 'true', `strict ${parameter} cannot be enabled`)
    }
    assert(await seed.isDisabled(), 'strict seed value is not editable')
    await maxTokens.scrollIntoViewIfNeeded()
    const strictParametersName = `${name}-strict-parameters.png`
    await page.screenshot({ path: resolve(output, strictParametersName), fullPage: true }); report.screenshots.push(strictParametersName)
    await page.keyboard.press('Escape')
    await maxTokens.waitFor({ state: 'hidden' })

    activeStep = 'strict budget media error preserves draft and selected key without fallback'
    await upload.setInputFiles({ name: 'strict-rejected.png', mimeType: 'image/png', buffer: png })
    await removeAttachment('strict-rejected.png').click({ trial: true })
    await send.click()
    await send.click({ trial: true })
    await page.getByText(label("This request does not meet the selected key's strict budget requirements. Use a compatible API client or choose another key."), { exact: true }).first().waitFor()
    assert.equal(sent.length, 7, 'the strict media fixture rejects one deliberate request without retry')
    assert.equal(await key.inputValue(), '24', 'strict rejection never falls back to an ordinary key')
    assert.equal(await input.inputValue(), 'Synthetic strict text')
    assert(await removeAttachment('strict-rejected.png').isEnabled(), 'strict rejection keeps the attachment actionable')
    await assertKeyIdentity(key, strictIdentity, name, 'strict-media-error-draft-preserved', width)
    const strictErrorName = `${name}-strict-media-error.png`
    await page.screenshot({ path: resolve(output, strictErrorName), fullPage: true }); report.screenshots.push(strictErrorName)

    activeStep = 'explicit text-only retry uses the qualified strict envelope'
    await removeAttachment('strict-rejected.png').click()
    assert.equal(sent.length, 7)
    await send.click()
    await page.getByText('Synthetic strict text accepted', { exact: true }).waitFor()
    await send.waitFor({ state: 'visible' })
    await page.waitForFunction(name => [...document.querySelectorAll('button')].some(button => button.getAttribute('aria-label') === name && button.disabled), label('Send'))
    assert.equal(sent.length, 8)
    assert.equal(sent[7].max_completion_tokens, strictLimit)
    assert.equal(await key.inputValue(), '24')
    assert.equal(await input.inputValue(), '')
    assert.equal(await composer.getByRole('img').count(), 0)
    await assertKeyIdentity(key, strictIdentity, name, 'strict-text-complete', width)
    const strictCompleteName = `${name}-strict-text-complete.png`
    await page.screenshot({ path: resolve(output, strictCompleteName), fullPage: true }); report.screenshots.push(strictCompleteName)

    activeStep = 'explicitly switching back restores ordinary controls and request preferences'
    expectedKeyId = '22'
    await key.selectOption('22')
    await input.fill('Synthetic ordinary preferences restored')
    await send.click({ trial: true })
    await parameters.click()
    await maxTokens.waitFor()
    assert.equal(await enabledParameter('Max Tokens').getAttribute('aria-checked'), 'false')
    assert.notEqual(await enabledParameter('Max Tokens').getAttribute('aria-disabled'), 'true')
    assert(await maxTokens.isDisabled())
    assert.equal(Number(await maxTokens.inputValue()), ordinaryPreferences.config.max_tokens)
    for (const parameter of ['Temperature', 'Top P', 'Frequency Penalty', 'Presence Penalty', 'Seed']) {
      assert.equal(await enabledParameter(parameter).getAttribute('aria-checked'), 'true', `ordinary ${parameter} preference is restored`)
      assert.notEqual(await enabledParameter(parameter).getAttribute('aria-disabled'), 'true')
    }
    assert.equal(await seed.inputValue(), '42')
    assert(await seed.isEnabled())
    assert.deepEqual(await page.evaluate(() => JSON.parse(localStorage.getItem('playground_parameter_enabled:user:2') || 'null')?.data), ordinaryPreferences.enabled, 'strict effective settings never overwrite saved parameter toggles')
    await page.keyboard.press('Escape')
    await maxTokens.waitFor({ state: 'hidden' })
    await assertKeyIdentity(key, secondaryIdentity, name, 'ordinary-key-preferences-restored', width)
    await send.click()
    await page.getByText('Synthetic attachment received 9', { exact: true }).waitFor()
    await send.waitFor({ state: 'visible' })
    await page.waitForFunction(name => [...document.querySelectorAll('button')].some(button => button.getAttribute('aria-label') === name && button.disabled), label('Send'))
    assert.equal(sent.length, 9, 'all strict and ordinary sends require deliberate user actions')
    assert.equal(sent[8].model, 'synthetic-text-model')
    for (const field of ['max_completion_tokens', 'max_tokens', 'stream_options', 'service_tier']) assert(!(field in sent[8]), `ordinary request does not inherit strict ${field}`)
    for (const field of ['temperature', 'top_p', 'frequency_penalty', 'presence_penalty', 'seed']) assert.equal(sent[8][field], ordinaryPreferences.config[field], `ordinary ${field} request preference is restored`)
    report.strictText.push({ journey: name, selectedKeyId: 24, model: 'gpt-6.1-sol', maxCompletionTokens: strictLimit, streamIncludeUsage: true, serviceTier: 'default', mediaRejected: true, noAutomaticFallback: true, ordinaryPreferencesRestored: true, syntheticOnly: true })
    report.journeys.push({ name, passed: true, explicitDispatches: sent.length, keyboardSelection: true, touchControl: width === 320, nativeOptionSelection: 'Playwright selectOption', sendStates: ['no-key', 'models-loading', 'ready', 'pending', 'complete', 'error', 'canceled', 'post-cancel-complete', 'strict-media-error', 'strict-text-complete', 'ordinary-preferences-restored'], invalidFileCases: invalidFiles.length + 1, multipleImages: true, removedAttachments: true, stoppedBrowserRequest: true, selectedKeyUsageInspection: 'synthetic owner-scoped log', strictText: true })
    await context.close(); activePage = null; persist()
  }
  assert.deepEqual(report.violations, [])
  assert.deepEqual(report.pageErrors, [])
  report.counts = { journeys: report.journeys.length, screenshots: report.screenshots.length, keyIdentityChecks: report.keyIdentity.length, invalidFileChecks: report.attachmentValidation.length, removalChecks: report.attachmentRemoval.length, multipleImageChecks: report.multipleImages.length, canceledRequests: report.cancellations.length, usageInspections: report.usageInspection.length, strictTextChecks: report.strictText.length, explicitDispatches: report.journeys.reduce((sum, journey) => sum + journey.explicitDispatches, 0) }
  assert.deepEqual(report.counts, { journeys: 8, screenshots: 96, keyIdentityChecks: 112, invalidFileChecks: 56, removalChecks: 24, multipleImageChecks: 8, canceledRequests: 8, usageInspections: 8, strictTextChecks: 8, explicitDispatches: 72 })
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
        textareas: [...document.querySelectorAll('textarea')].map(element => ({ label: element.getAttribute('aria-label'), value: element.value, disabled: element.disabled })),
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
