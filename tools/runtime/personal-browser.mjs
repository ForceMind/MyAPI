// Real browser -> isolated MyAPI -> bounded loopback provider acceptance.
// Never fulfill application routes, inject authentication, reveal a Key, record
// browser traces/storage, or serialize external errors/response bodies.
import { randomBytes } from 'node:crypto'
import { lstatSync, readFileSync, realpathSync } from 'node:fs'
import path from 'node:path'

const model = 'smoke-model'
const quota = 1000
const usage = 15
const stages = new Set(['launch', 'setup', 'setup-session', 'setup-open', 'setup-database',
  'setup-credentials', 'setup-mode', 'setup-review', 'setup-submit', 'setup-response', 'login', 'login-open', 'login-response', 'login-keys', 'document-backoff', 'writer-check', 'fixture-config', 'fixture-config-refresh', 'policy-view', 'policy-refresh',
  'key-create', 'key-create-open', 'key-create-ready', 'key-create-profile', 'key-create-cap',
  'key-create-quota', 'key-create-submit', 'key-create-payload', 'key-create-response', 'key-create-readback', 'key-selection', 'playground-send', 'ledger-check', 'usage-view', 'policy-confirm',
  'key-selection-refresh', 'key-selection-model', 'playground-response', 'playground-complete', 'playground-rejection',
  'usage-open', 'usage-list', 'usage-list-visible', 'usage-details-open', 'usage-details-request', 'usage-details-key',
  'strict-budget', 'reload-check', 'screenshot', 'browser-close'])
const codes = new Set(['SCOPE_REJECTED', 'AUTH_UNAVAILABLE', 'BROWSER_FAILED', 'BROWSER_CLOSE_FAILED', 'SETUP_FAILED',
  'HTTP_FAILED', 'LOGIN_FAILED', 'RUNTIME_ERROR', 'BUILD_MISMATCH', 'API_FAILED', 'STATE_MISMATCH',
  'KEY_MISMATCH', 'POLICY_MISMATCH', 'REQUEST_MISMATCH', 'RELAY_MISMATCH', 'LOG_MISMATCH',
  'USAGE_MISMATCH', 'UPSTREAM_MISMATCH', 'WRITER_MISMATCH', 'SCREENSHOT_REJECTED', 'TIMEOUT'])
const diagnosticCategories = new Set(['asset', 'auth', 'log', 'bootstrap'])
const browserErrorClasses = new Set(['Error', 'TypeError', 'RangeError', 'ReferenceError', 'SyntaxError', 'ChunkLoadError', 'ImportError', 'other'])

function diagnosticCategory(request, origin) {
  const url = new URL(request.url())
  if (url.origin !== origin) return undefined
  if (['/api/user/login', '/api/user/auth/refresh', '/api/user/logout', '/api/user/self'].includes(url.pathname)) return 'auth'
  if (url.pathname === '/api/log' || url.pathname.startsWith('/api/log/')) return 'log'
  if (['/api/status', '/api/setup'].includes(url.pathname)) return 'bootstrap'
  if (['script', 'stylesheet', 'font', 'image'].includes(request.resourceType?.())) return 'asset'
  return undefined
}

function browserErrorClass(name, message) {
  // Classify in memory only. Neither the text, error object, nor console JS
  // handles/location may cross the diagnostic boundary.
  const text = typeof message === 'string' ? message.slice(0, 4096) : ''
  if (name === 'ChunkLoadError' || /\bChunkLoadError\b|\bLoading (?:CSS )?chunk [\s\S]*? failed\b/i.test(text)) return 'ChunkLoadError'
  if (/\bFailed to fetch dynamically imported module\b|\bImporting a module script failed\b|\berror loading dynamically imported module\b/i.test(text)) return 'ImportError'
  if (browserErrorClasses.has(name)) return name
  return text.match(/\b(TypeError|RangeError|ReferenceError|SyntaxError|Error):/)?.[1] || 'other'
}

function requireThat(condition, code) {
  if (!condition) throw new Error(`SMOKE_PERSONAL_${code}`)
}

// Bound individual operations, never a whole business flow that could continue
// after a race loses. A timeout aborts owned fetches and actively closes contexts;
// withBrowser then attempts bounded browser shutdown before returning failure.
async function bounded(scope, operation, timeout = 20_000) {
  requireThat(!scope.abort.signal.aborted, 'TIMEOUT')
  let timer
  try {
    return await Promise.race([
      operation(),
      new Promise((_, reject) => {
        timer = setTimeout(() => reject(new Error('SMOKE_PERSONAL_TIMEOUT')), timeout)
      }),
    ])
  } catch (error) {
    if (error?.message === 'SMOKE_PERSONAL_TIMEOUT' || error?.name === 'TimeoutError') {
      scope.abort.abort()
      for (const context of scope.contexts) {
        // Do not let an unresponsive context delay the browser-close fallback.
        try { Promise.resolve(context.close()).catch(() => {}) } catch {}
      }
      throw new Error('SMOKE_PERSONAL_TIMEOUT')
    }
    throw error
  } finally {
    clearTimeout(timer)
  }
}

function validateOptions(options, withUpstream) {
  const { baseUrl, edition, sha, isolated, username, password, writer = 'legacy', artifactDir } = options
  let target
  let upstream
  try {
    target = new URL(baseUrl)
    if (withUpstream) upstream = new URL(options.upstreamBaseUrl)
  } catch { throw new Error('SMOKE_PERSONAL_SCOPE_REJECTED') }
  requireThat(isolated === true && target.protocol === 'http:' && target.hostname === '127.0.0.1' &&
    target.port && !target.username && !target.password && target.pathname === '/' && !target.search && !target.hash &&
    ['full', 'lan'].includes(edition) && /^[a-f0-9]{40}$/.test(sha || '') &&
    ['legacy', 'authoritative'].includes(writer), 'SCOPE_REJECTED')
  if (withUpstream) requireThat(upstream.origin === 'http://127.0.0.1:19090' && !upstream.username &&
    !upstream.password && upstream.pathname === '/' && !upstream.search && !upstream.hash, 'SCOPE_REJECTED')
  requireThat(typeof username === 'string' && username.length > 0 && typeof password === 'string' && password.length >= 8, 'AUTH_UNAVAILABLE')
  if (artifactDir !== undefined) {
    try {
      requireThat(path.isAbsolute(artifactDir) && lstatSync(artifactDir).isDirectory() &&
        !lstatSync(artifactDir).isSymbolicLink() && realpathSync(artifactDir) === path.resolve(artifactDir), 'SCREENSHOT_REJECTED')
    } catch { throw new Error('SMOKE_PERSONAL_SCREENSHOT_REJECTED') }
  }
  return { origin: target.origin, upstream: upstream?.origin, writer }
}

async function withBrowser(options, withUpstream, run) {
  const scope = validateOptions(options, withUpstream)
  scope.abort = new AbortController()
  scope.contexts = new Set()
  scope.backoffSeconds = 0
  let stage = 'launch'
  const progress = (event, code, httpStatus, retryAfter, diagnostic = {}) => {
    const record = { command: 'personal:browser', phase: withUpstream ? 'journey' : 'setup', event,
      stage, sha: options.sha, writer: scope.writer, ...(code ? { code } : {}),
      ...(Number.isInteger(httpStatus) && httpStatus >= 100 && httpStatus <= 599 ? { httpStatus } : {}),
      ...(Number.isSafeInteger(retryAfter) && retryAfter >= 0 ? { retryAfter } : {}),
      ...(diagnosticCategories.has(diagnostic.category) ? { category: diagnostic.category } : {}),
      ...(browserErrorClasses.has(diagnostic.errorClass) ? { errorClass: diagnostic.errorClass } : {}),
      ...(diagnostic.count === 16 ? { count: 16 } : {}) }
    // Emit only fixed classifications/numeric metadata, never raw diagnostics.
    try {
      if (typeof options.onProgress === 'function') Promise.resolve(options.onProgress(record)).catch(() => {})
      else console.error(JSON.stringify(record))
    } catch {} // A diagnostic sink must not prevent cleanup or replace a safe error.
  }
  Object.defineProperty(scope, 'stage', {
    get: () => stage,
    set(value) { stage = stages.has(value) ? value : 'launch'; progress('stage') },
  })
  scope.documentResponse = (httpStatus, retryAfter) => progress('document-response', undefined, httpStatus, retryAfter)
  scope.diagnostic = (event, detail) => progress(event, undefined, detail.httpStatus, detail.retryAfter, detail)
  let browser
  let failed = false
  scope.stage = 'launch'
  try {
    const module = typeof options.playwrightModule === 'string'
      ? await import(options.playwrightModule) : options.playwrightModule
    requireThat(typeof module?.chromium?.launch === 'function', 'BROWSER_FAILED')
    browser = await module.chromium.launch({ headless: true })
    return await run(browser, scope)
  } catch (error) {
    failed = true
    const suffix = String(error?.message || '').replace(/^SMOKE_PERSONAL_/, '')
    // No cause/stack from browser, HTTP client or assertion libraries may escape.
    const safeError = new Error(`SMOKE_PERSONAL_${codes.has(suffix) ? suffix : 'BROWSER_FAILED'}`)
    safeError.stage = stages.has(scope.stage) ? scope.stage : 'launch'
    if (suffix === 'HTTP_FAILED') {
      if (Number.isInteger(error.httpStatus) && error.httpStatus >= 100 && error.httpStatus <= 599) safeError.httpStatus = error.httpStatus
      if (Number.isSafeInteger(error.retryAfter) && error.retryAfter >= 0) safeError.retryAfter = error.retryAfter
    }
    throw safeError
  } finally {
    if (browser) {
      scope.abort.abort()
      scope.stage = 'browser-close'
      let timer
      try {
        await Promise.race([
          browser.close(),
          new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('cleanup deadline')), 10_000) }),
        ])
      } catch {
        // The close request is already active. Report immediately; process
        // cleanup remains unconfirmed and the isolated CI runner is the fallback.
        progress('cleanup-failed', 'SMOKE_PERSONAL_BROWSER_CLOSE_FAILED')
        // Preserve an earlier safe failure, but never return success when the
        // ephemeral browser (and its authentication state) could not be closed.
        if (!failed) {
          const safeError = new Error('SMOKE_PERSONAL_BROWSER_CLOSE_FAILED')
          safeError.stage = 'browser-close'
          throw safeError
        }
      } finally {
        clearTimeout(timer)
      }
    }
  }
}

async function newSession(browser, origin, language = 'en', width = 1280, scope = {}) {
  const context = await bounded(scope, () => browser.newContext({ locale: language === 'zh' ? 'zh-CN' : 'en-US',
    viewport: { width, height: 900 }, serviceWorkers: 'block' }))
  scope.contexts.add(context)
  // The only initialization is the user's ordinary language preference. No
  // synthetic API/auth state, selected Key or Playground config is injected.
  await bounded(scope, () => context.addInitScript(({ origin, language }) => {
    if (location.origin === origin) localStorage.setItem('i18nextLng', language === 'zh' ? 'zhCN' : language)
  }, { origin, language }))
  await bounded(scope, () => context.route('**/*', (route) => {
    let allowed = false
    try { allowed = new URL(route.request().url()).origin === origin } catch {}
    return allowed ? route.continue() : route.abort()
  }))
  const page = await bounded(scope, () => context.newPage())
  page.setDefaultTimeout(20_000)
  const translations = JSON.parse(readFileSync(new URL(`../../web/src/i18n/locales/${language}.json`, import.meta.url), 'utf8')).translation
  const session = { context, page, scope, language, width, origin, errors: 0, requests: 0, reveals: 0, secrets: [], keyReads: new Set(),
    label: (key) => translations[key] || key }
  const diagnostics = new Set()
  let diagnosticLimit = false
  const diagnose = (event, detail) => {
    const signature = JSON.stringify([event, detail])
    if (diagnostics.has(signature)) return
    if (diagnostics.size === 16) {
      if (!diagnosticLimit) {
        diagnosticLimit = true
        scope.diagnostic('diagnostic-limit', { count: 16 })
      }
      return
    }
    diagnostics.add(signature)
    scope.diagnostic(event, detail)
  }
  // Install before the first navigation, including lazy route assets. All
  // callbacks are best-effort observers: no body reads, retries or UI changes.
  page.on('response', response => {
    try {
      const category = diagnosticCategory(response.request(), origin)
      const httpStatus = response.status()
      if (!category || !Number.isInteger(httpStatus) || httpStatus < 400 || httpStatus > 599) return
      const raw = response.headers()['retry-after']
      const retryAfter = typeof raw === 'string' && /^\d+$/.test(raw) ? Number(raw) : undefined
      diagnose('diagnostic-response', { category, httpStatus,
        ...(Number.isSafeInteger(retryAfter) && retryAfter >= 0 ? { retryAfter } : {}) })
    } catch {} // Diagnostics must never replace the original business failure.
  })
  page.on('pageerror', error => {
    session.errors += 1 // Preserve the existing runtime-error acceptance check.
    try { diagnose('diagnostic-pageerror', { errorClass: browserErrorClass(error?.name, error?.message) }) } catch {}
  })
  page.on('console', message => {
    try {
      if (message.type() === 'error') diagnose('diagnostic-console', { errorClass: browserErrorClass(undefined, message.text()) })
    } catch {}
  })
  page.on('request', (request) => {
    const url = new URL(request.url())
    const pathname = url.pathname
    if (url.origin === origin && pathname === '/pg/keys' && request.method() === 'GET' &&
      url.searchParams.get('p') === '1' && url.searchParams.get('page_size') === '100') session.keyReads.add(request)
    if (pathname === '/pg/chat/completions' && request.method() === 'POST') session.requests += 1
    if (/^\/api\/token\/\d+\/key$/.test(pathname)) session.reveals += 1
  })
  page.on('requestfinished', request => session.keyReads.delete(request))
  page.on('requestfailed', request => {
    session.keyReads.delete(request)
    try {
      const category = diagnosticCategory(request, origin)
      if (category) diagnose('diagnostic-requestfailed', { category })
    } catch {}
  })
  return session
}

async function open(session, pathname, { forceLoad = false } = {}) {
  const target = session.origin + pathname
  // Reusing a page already at this exact URL avoids needless cache-disabled
  // document reloads. The final persistence check explicitly forces a load.
  if (!forceLoad && session.page.url() === target) return
  if (!forceLoad && session.user && ['/keys', '/playground'].includes(pathname)) {
    const { page, label } = session
    const mobile = session.width === 320
    const toggle = page.getByRole('button', { name: label('Toggle sidebar'), exact: true })
    if (mobile) {
      await toggle.waitFor()
      if (await toggle.getAttribute('aria-expanded') === 'false') await toggle.click()
    }
    await page.locator('[data-myapi-sidebar]').getByRole('link', {
      name: label(pathname === '/keys' ? 'API Keys' : 'Playground'), exact: true,
    }).and(page.locator(`a[href="${pathname}"]`)).click()
    await page.waitForURL(url => url.origin === session.origin && url.pathname === pathname)
    if (mobile) await toggle.and(page.locator('[aria-expanded="false"]')).waitFor()
    return
  }
  for (let attempt = 0; attempt < 2; attempt++) {
    const response = await session.page.goto(target, { waitUntil: 'domcontentloaded', timeout: 30_000 })
    const httpStatus = response?.status()
    if (httpStatus === 200) return
    const rawRetryAfter = response?.headers()['retry-after']
    // Our server's rate limiter returns integer delta-seconds. Do not guess a
    // missing/invalid value, follow arbitrary dates, or retain raw header text.
    const retryAfter = typeof rawRetryAfter === 'string' && /^\d+$/.test(rawRetryAfter)
      ? Number(rawRetryAfter) : undefined
    session.scope.documentResponse(httpStatus, retryAfter)
    if (attempt === 0 && httpStatus === 429 && response.request().method() === 'GET' &&
      Number.isSafeInteger(retryAfter) && retryAfter >= 0 &&
      retryAfter <= 180 - session.scope.backoffSeconds) {
      session.scope.backoffSeconds += retryAfter
      const previousStage = session.scope.stage
      session.scope.stage = 'document-backoff'
      await new Promise(resolve => setTimeout(resolve, retryAfter * 1000))
      session.scope.stage = previousStage
      continue
    }
    const error = new Error('SMOKE_PERSONAL_HTTP_FAILED')
    error.httpStatus = httpStatus
    error.retryAfter = retryAfter
    throw error
  }
}

function responseFor(session, pathname, method) {
  return session.page.waitForResponse((response) => {
    const url = new URL(response.url())
    return url.origin === session.origin && url.pathname === pathname && response.request().method() === method
  })
}

async function responseSuccess(session, response, code) {
  const payload = await bounded(session.scope, () => response.json())
  requireThat(response.status() >= 200 && response.status() < 300 && payload?.success === true, code)
  return payload.data
}

async function refreshReadback(session, pathname, button, code, parameters = {}) {
  // Policy Refresh is disabled during its initial query. Let that read settle
  // before observing the request caused by this explicit user action.
  await button.click({ trial: true })
  const matches = (request) => {
    const url = new URL(request.url())
    return url.origin === session.origin && url.pathname === pathname && request.method() === 'GET' &&
      Object.entries(parameters).every(([name, value]) => url.searchParams.get(name) === value)
  }
  // Correlate the newly requested read with its response. A cached dialog or a
  // previously in-flight read must not satisfy the explicit Refresh action.
  const requested = session.page.waitForRequest(matches)
  const received = session.page.waitForResponse(response => matches(response.request()) &&
    requested.then(request => response.request() === request))
  const [, response] = await Promise.all([requested, received, button.click()])
  return responseSuccess(session, response, code)
}

async function login(session, username, password, expectedRole, sha) {
  session.scope.stage = 'login-open'
  await open(session, '/sign-in')
  session.scope.stage = 'login'
  await session.page.locator('form input[name="username"]').fill(username)
  await session.page.locator('form input[name="password"][type="password"]').fill(password)
  const received = responseFor(session, '/api/user/login', 'POST')
  await session.page.locator('form button[type="submit"]').click()
  session.scope.stage = 'login-response'
  const bundle = await responseSuccess(session, await received, 'LOGIN_FAILED')
  requireThat(typeof bundle?.access_token === 'string' && bundle.access_token &&
    Number.isSafeInteger(bundle.user?.id) && bundle.user.id > 0 && bundle.user.role === expectedRole, 'LOGIN_FAILED')
  session.user = bundle.user
  session.token = bundle.access_token
  session.secrets.push(password, bundle.access_token)
  session.username = username
  await session.page.waitForURL((url) => url.pathname !== '/sign-in')
  session.scope.stage = 'login-keys'
  await open(session, '/keys')
  await session.page.getByRole('button', { name: session.label('My usage policy'), exact: true }).waitFor()
  const metadata = await bounded(session.scope, () => session.page.evaluate(() => ({
    global: window.__APP_BUILD__?.rev,
    html: document.documentElement.getAttribute('data-build-rev'),
    meta: document.querySelector('meta[name="build-id"]')?.getAttribute('content'),
  })))
  const version = readFileSync(new URL('../../VERSION', import.meta.url), 'utf8').trim()
  requireThat(typeof metadata.global === 'string' && metadata.global.startsWith(`rv.${version}.${sha}.`) &&
    metadata.global === metadata.html && metadata.global === metadata.meta, 'BUILD_MISMATCH')
}

async function capture(session, options, name, screenshots) {
  if (!options.artifactDir) return
  const previousStage = session.scope.stage
  session.scope.stage = 'screenshot'
  requireThat(/^[a-z0-9-]+$/.test(name) && !['/sign-in', '/setup'].includes(new URL(session.page.url()).pathname), 'SCREENSHOT_REJECTED')
  const safe = await bounded(session.scope, () => session.page.evaluate((secrets) => {
    const visible = document.body.innerText
    const values = [...document.querySelectorAll('input, textarea')].map((input) => input.value).join('\n')
    return secrets.every((secret) => !visible.includes(secret) && !values.includes(secret))
  }, session.secrets))
  requireThat(safe && session.reveals === 0, 'SCREENSHOT_REJECTED')
  const filename = `${name}-${session.language}-${session.width}.png`
  await session.page.screenshot({ path: path.join(options.artifactDir, filename), fullPage: true,
    mask: [session.page.locator('input'), session.page.getByText(/sk-[A-Za-z0-9*._-]+/),
      session.page.getByText(session.username, { exact: true })] })
  screenshots.push(filename)
  session.scope.stage = previousStage
}

async function policyView(session, expectedNoBalance, refreshRevision) {
  session.scope.stage = 'policy-view'
  const { page, label } = session
  await open(session, '/keys')
  await page.getByRole('button', { name: label('My usage policy'), exact: true }).click()
  const dialog = page.getByRole('dialog', { name: label('User usage policy'), exact: true })
  if (refreshRevision !== undefined) {
    session.scope.stage = 'policy-refresh'
    const policy = await refreshReadback(session, `/api/user/${session.user.id}/usage-policy`,
      dialog.getByRole('button', { name: label('Refresh'), exact: true }), 'POLICY_MISMATCH')
    requireThat(policy?.user_id === session.user.id && policy.revision === refreshRevision &&
      policy.no_balance === expectedNoBalance && policy.legacy_remaining_quota === 0, 'POLICY_MISMATCH')
  }
  const configuration = dialog.getByRole('region', { name: label('Current configuration'), exact: true })
  await configuration.getByText(label('Commercial funding disabled'), { exact: true }).waitFor()
  await configuration.getByText(label(expectedNoBalance ? 'Use Key limits without a user wallet' : 'Use the stored user allowance'), { exact: true }).waitFor()
  if (session.user.role !== 100) {
    await dialog.getByText(label('Only Root can change this policy.'), { exact: true }).waitFor()
    requireThat(await dialog.locator('form, input, [role="checkbox"]').count() === 0, 'POLICY_MISMATCH')
  }
  return dialog
}

async function closeDialog(session, dialog) {
  // A response can arrive before the UI mutation releases its close lock.
  await dialog.locator('[data-slot="dialog-close"]').waitFor({ state: 'visible' })
  await session.page.keyboard.press('Escape')
  await dialog.waitFor({ state: 'hidden' })
}

async function createKey(session, api, name, limit) {
  session.scope.stage = 'key-create-open'
  const { page, label } = session
  await open(session, '/keys')
  await page.getByRole('button', { name: label('Create API Key'), exact: true }).click()
  const dialog = page.getByRole('dialog', { name: label('Create API Key'), exact: true })
  session.scope.stage = 'key-create-ready'
  await dialog.locator('form[aria-busy="false"]').waitFor()
  await dialog.getByRole('textbox', { name: label('Name'), exact: true }).fill(name)
  session.scope.stage = 'key-create-profile'
  // The creation form intentionally starts with group='', which inherits the
  // user's route. Choose the fixture's explicit profile through the real UI.
  const profile = dialog.getByRole('combobox')
  await profile.click()
  await page.getByRole('option').filter({ hasText: `${label('Standard access')} (default)` }).click()
  await profile.getByText(`${label('Standard access')} (default)`, { exact: true }).waitFor()
  session.scope.stage = 'key-create-cap'
  const cap = dialog.getByRole('switch', { name: label('No ordinary quota cap'), exact: true })
  requireThat(await cap.getAttribute('aria-checked') === 'true', 'KEY_MISMATCH')
  await cap.click()
  await cap.and(page.locator('[aria-checked="false"]')).waitFor()
  requireThat(await cap.getAttribute('aria-checked') === 'false', 'KEY_MISMATCH')
  session.scope.stage = 'key-create-quota'
  await dialog.getByRole('spinbutton', { name: label('Key quota (internal units)'), exact: true }).fill(String(limit))
  await dialog.getByText(label('Internal quota is a price-converted allowance, not a count of actual input and output tokens.'), { exact: true }).waitFor()
  session.scope.stage = 'key-create-submit'
  const pending = responseFor(session, '/api/token/', 'POST')
  await dialog.getByRole('button', { name: label('Save changes'), exact: true }).click()
  const response = await pending
  session.scope.stage = 'key-create-payload'
  const sent = response.request().postDataJSON()
  requireThat(sent?.name === name && sent.remain_quota === limit && sent.unlimited_quota === false && sent.group === 'default', 'KEY_MISMATCH')
  session.scope.stage = 'key-create-response'
  await responseSuccess(session, response, 'KEY_MISMATCH')
  await dialog.waitFor({ state: 'hidden' })
  session.scope.stage = 'key-create-readback'
  const data = await api(session, `/api/token/search?keyword=${encodeURIComponent(name)}&p=1&page_size=10`)
  const keys = data.items?.filter((item) => item.name === name)
  requireThat(keys?.length === 1 && Number.isSafeInteger(keys[0].id) && keys[0].id > 0 &&
    keys[0].user_id === session.user.id && keys[0].remain_quota === limit && keys[0].used_quota === 0 &&
    keys[0].unlimited_quota === false && keys[0].group === 'default', 'KEY_MISMATCH')
  // Never resolve /key or retain the masked/full key property in driver state.
  return { id: keys[0].id, name, limit }
}

async function preparePlayground(session, key) {
  session.scope.stage = 'key-selection'
  const { page, label } = session
  await open(session, '/playground')
  const selected = page.getByRole('combobox', { name: label('API key'), exact: true })
  await selected.waitFor()
  // An explicit empty selection must prevent sending and cannot auto-select a
  // fallback. This also covers returning with an older remembered selection.
  await selected.selectOption('')
  await page.getByRole('textbox', { name: label('Message'), exact: true }).fill('synthetic personal browser request')
  requireThat(await page.getByRole('button', { name: label('Send'), exact: true }).isDisabled(), 'REQUEST_MISMATCH')
  session.scope.stage = 'key-selection-refresh'
  const refresh = page.getByRole('status').getByRole('button', { name: label('Refresh'), exact: true })
  let available
  // The enabled Key select already waits for an uncached initial query. With
  // cached data, however, a background GET can still be pending. The app's HTTP
  // deduplicator may reuse that promise when Refresh is clicked. Read its whole
  // real JSON body rather than requiring an additional network request event.
  const pendingKeys = [...session.keyReads]
  requireThat(pendingKeys.length <= 1, 'REQUEST_MISMATCH')
  if (pendingKeys.length === 1) {
    const existing = await bounded(session.scope, async () => {
      const response = await pendingKeys[0].response()
      requireThat(!session.scope.abort.signal.aborted, 'TIMEOUT')
      requireThat(response, 'REQUEST_MISMATCH')
      const payload = await response.json()
      requireThat(response.status() === 200 && payload?.success === true, 'REQUEST_MISMATCH')
      return payload.data
    })
    await bounded(session.scope, () => page.evaluate(() => document.readyState))
    if (Array.isArray(existing?.items) && existing.items.filter(item => item.id === key.id).length === 1) {
      await refresh.click()
      available = existing
    }
  }
  // If an older read lacked the new Key, it has now completed: explicitly
  // refresh and require the newly read list to contain this exact Key.
  if (!available) available = await refreshReadback(session, '/pg/keys', refresh,
    'REQUEST_MISMATCH', { p: '1', page_size: '100' })
  requireThat(Array.isArray(available?.items) && available.items.filter(item => item.id === key.id).length === 1, 'REQUEST_MISMATCH')
  session.scope.stage = 'key-selection-model'
  await selected.selectOption(String(key.id))
  requireThat(await selected.inputValue() === String(key.id), 'REQUEST_MISMATCH')
  // The same Key's models can legitimately remain in the fresh query cache.
  // Choose the real visible option; do not require a redundant models GET or
  // rely on the model-selection effect. This trigger also works at 320px.
  await page.locator('form button[role="combobox"]').click()
  const modelOption = page.getByRole('option', { name: model, exact: true })
  try { await modelOption.waitFor({ state: 'visible' }) } catch { throw new Error('SMOKE_PERSONAL_REQUEST_MISMATCH') }
  requireThat(await modelOption.count() === 1, 'REQUEST_MISMATCH')
  await modelOption.click()
  await page.getByRole('button', { name: label('Parameters'), exact: true }).click()
  const enabledName = label('Enable {{parameter}}').replace('{{parameter}}', label('Max Tokens'))
  const maxTokensEnabled = page.getByRole('switch', { name: enabledName, exact: true })
  if (await maxTokensEnabled.getAttribute('aria-checked') === 'false') await maxTokensEnabled.click()
  await maxTokensEnabled.and(page.locator('[aria-checked="true"]')).waitFor()
  requireThat(await maxTokensEnabled.getAttribute('aria-checked') === 'true', 'REQUEST_MISMATCH')
  await page.getByRole('spinbutton', { name: label('Max Tokens'), exact: true }).fill('8')
  await page.keyboard.press('Escape')
  await page.getByRole('spinbutton', { name: label('Max Tokens'), exact: true }).waitFor({ state: 'hidden' })
  await page.getByRole('button', { name: label('Send'), exact: true }).click({ trial: true })
}

async function sendPlayground(session, key, expectedCode) {
  session.scope.stage = 'playground-send'
  const assistants = session.page.locator('.is-assistant')
  const beforeAssistants = await bounded(session.scope, () => assistants.count())
  const pending = responseFor(session, '/pg/chat/completions', 'POST')
  const beforeRequests = session.requests
  await session.page.getByRole('button', { name: session.label('Send'), exact: true }).click()
  session.scope.stage = 'playground-response'
  const response = await pending
  const request = response.request()
  const sent = request.postDataJSON()
  requireThat(sent?.model === model && sent.max_tokens === 8 && sent.stream === true &&
    request.headers()['x-myapi-key-id'] === String(key.id), 'REQUEST_MISMATCH')
  const requestId = response.headers()['x-oneapi-request-id']
  requireThat(typeof requestId === 'string' && /^[A-Za-z0-9._:-]{1,160}$/.test(requestId), 'REQUEST_MISMATCH')
  if (expectedCode) {
    session.scope.stage = 'playground-rejection'
    const text = await bounded(session.scope, () => response.text())
    requireThat(text.length <= 128 * 1024, 'RELAY_MISMATCH')
    const payload = JSON.parse(text)
    requireThat(response.status() === (expectedCode === 'token_budget_unsupported_request' ? 400 : 403) &&
      payload?.error?.code === expectedCode, 'RELAY_MISMATCH')
    await assistants.last().getByRole('alert').filter({ hasText: session.label('Error') }).waitFor()
  } else {
    session.scope.stage = 'playground-complete'
    requireThat(response.status() === 200, 'RELAY_MISMATCH')
    // The actual frontend closes its SSE source after DONE. Network body/finished
    // is not its completion contract. Require the newly added assistant's full
    // text, normal terminal controls and no current-message error instead.
    await assistants.last().getByText('synthetic fixed response', { exact: true }).waitFor()
    await session.page.getByRole('button', { name: session.label('Stop'), exact: true }).waitFor({ state: 'hidden' })
    await session.page.getByRole('button', { name: session.label('Send'), exact: true }).waitFor({ state: 'visible' })
    requireThat(await bounded(session.scope, () => assistants.last().getByRole('alert').count()) === 0, 'RELAY_MISMATCH')
  }
  requireThat(await bounded(session.scope, () => assistants.count()) === beforeAssistants + 1, 'REQUEST_MISMATCH')
  // This read happens after the actual UI reaches a terminal state; no relay
  // replay is used to wait for asynchronous accounting.
  requireThat(session.requests === beforeRequests + 1, 'REQUEST_MISMATCH')
  return requestId
}

async function snapshot(api, session, key) {
  const user = await api(session, '/api/user/self')
  const token = await api(session, `/api/token/${key.id}`)
  requireThat(user.id === session.user.id && user.role === session.user.role && token.id === key.id &&
    token.user_id === user.id && token.unlimited_quota === false && user.quota === 0, 'STATE_MISMATCH')
  return { user, token }
}

async function verifyUsage(api, session, key, before, requestId, consumed) {
  session.scope.stage = 'ledger-check'
  let logs
  const deadline = Date.now() + 10_000
  for (let attempt = 0; attempt < 50; attempt++) {
    const remaining = deadline - Date.now()
    requireThat(remaining > 0, 'LOG_MISMATCH')
    logs = await api(session, `/api/log/self?type=2&request_id=${encodeURIComponent(requestId)}&p=1&page_size=100`, 'GET', undefined, false, remaining)
    requireThat(Array.isArray(logs.items) && Number.isSafeInteger(logs.total), 'LOG_MISMATCH')
    if (!consumed || logs.total !== 0 || logs.items.length !== 0) break
    requireThat(attempt < 49, 'LOG_MISMATCH')
    await new Promise((resolve) => setTimeout(resolve, Math.min(100, Math.max(0, deadline - Date.now()))))
  }
  requireThat(logs.total === (consumed ? 1 : 0) && logs.items.length === (consumed ? 1 : 0), 'LOG_MISMATCH')
  if (consumed) {
    const log = logs.items[0]
    const other = JSON.parse(log.other || '{}')
    requireThat(log.request_id === requestId && log.user_id === session.user.id && log.token_id === key.id &&
      log.token_name === key.name && log.model_name === model && log.quota === usage &&
      log.prompt_tokens === 10 && log.completion_tokens === 5 && other.billing_source === 'self_use' &&
      (session.user.role === 100 || !Object.hasOwn(other, 'admin_info')), 'LOG_MISMATCH')
  }
  const after = await snapshot(api, session, key)
  const delta = consumed ? usage : 0
  requireThat(after.user.quota === 0 && after.user.self_use_no_balance === before.user.self_use_no_balance &&
    after.user.used_quota === before.user.used_quota + delta &&
    after.user.request_count === before.user.request_count + (consumed ? 1 : 0) &&
    after.token.remain_quota === before.token.remain_quota - delta &&
    after.token.used_quota === before.token.used_quota + delta, 'USAGE_MISMATCH')
  return after
}

async function showPersistedUsage(session, key, requestId, options, screenshots) {
  session.scope.stage = 'usage-open'
  try {
    // The admin client GET redirects to Gin's /api/log/ route. The ordinary
    // owner's route is /api/log/self, without a trailing slash.
    const pathname = session.user.role === 100 ? '/api/log/' : '/api/log/self'
    const received = session.page.waitForResponse(response => {
      const url = new URL(response.url())
      return url.origin === session.origin && url.pathname === pathname && response.request().method() === 'GET' &&
        url.searchParams.get('request_id') === requestId && url.searchParams.get('token_name') === key.name
    })
    received.catch(() => {}) // Navigation failure still owns the original error.
    await open(session, `/usage-logs/common?token=${encodeURIComponent(key.name)}&requestId=${encodeURIComponent(requestId)}`)
    session.scope.stage = 'usage-list'
    const data = await responseSuccess(session, await received, 'LOG_MISMATCH')
    requireThat(data?.total === 1 && Array.isArray(data.items) && data.items.length === 1, 'LOG_MISMATCH')
    const log = data.items[0]
    requireThat(log.request_id === requestId && log.user_id === session.user.id && log.token_id === key.id &&
      log.token_name === key.name && log.model_name === model, 'LOG_MISMATCH')
    session.scope.stage = 'usage-list-visible'
    // Mobile cards reuse the actual desktop cells, but only one layout mounts.
    // Filter visibility to exclude tooltip copies; never choose an arbitrary
    // first history entry or suppress ambiguity among visible target entries.
    await session.page.getByText(key.name, { exact: true }).filter({ visible: true }).waitFor()
    await session.page.getByText(model, { exact: true }).filter({ visible: true }).waitFor()
    session.scope.stage = 'usage-details-open'
    const details = session.page.getByTitle(session.label('Click to view full details'), { exact: true }).filter({ visible: true })
    await details.waitFor()
    requireThat(await details.count() === 1, 'LOG_MISMATCH')
    await details.click()
    const dialog = session.page.getByRole('dialog')
    session.scope.stage = 'usage-details-request'
    await dialog.getByText(requestId, { exact: true }).waitFor()
    session.scope.stage = 'usage-details-key'
    await dialog.getByText(key.name, { exact: true }).waitFor()
  } catch (error) {
    const failedStage = session.scope.stage
    try {
      if (new URL(session.page.url()).pathname === '/usage-logs/common') {
        await capture(session, options, 'usage-failed', screenshots)
      }
    } catch {} // Failed/unsafe diagnostics must never replace the original error.
    session.scope.stage = failedStage
    throw error
  }
}

export async function browserSetup(options = {}) {
  return withBrowser(options, false, async (browser, scope) => {
    const { origin } = scope
    scope.stage = 'setup-session'
    const session = await newSession(browser, origin, 'en', 1280, scope)
    const { page, label } = session
    scope.stage = 'setup-open'
    await open(session, '/setup')
    scope.stage = 'setup-database'
    await page.getByRole('heading', { name: label('Database check'), exact: true }).waitFor()
    await page.getByRole('button', { name: label('Next'), exact: true }).click()
    scope.stage = 'setup-credentials'
    await page.getByRole('heading', { name: label('Administrator account'), exact: true }).waitFor()
    await page.locator('input[name="username"]').fill(options.username)
    await page.locator('input[name="password"]').fill(options.password)
    await page.locator('input[name="confirmPassword"]').fill(options.password)
    await page.getByRole('button', { name: label('Next'), exact: true }).click()
    scope.stage = 'setup-mode'
    await page.getByRole('heading', { name: label('Usage mode'), exact: true }).waitFor()
    // Base UI assigns the public usage-mode-* id to its clipped hidden input.
    // Its visible radio span has a generated id and inherits the label text.
    const mode = page.getByRole('radio', { name: options.edition === 'lan' ? /Personal use/ : /External operations/ })
    await mode.click()
    requireThat(await mode.getAttribute('aria-checked') === 'true', 'SETUP_FAILED')
    await page.getByRole('button', { name: label('Next'), exact: true }).click()
    scope.stage = 'setup-review'
    await page.getByRole('heading', { name: label('Review & initialize'), exact: true }).waitFor()
    const response = responseFor(session, '/api/setup', 'POST')
    scope.stage = 'setup-submit'
    await page.getByRole('button', { name: label('Initialize system'), exact: true }).click()
    scope.stage = 'setup-response'
    await responseSuccess(session, await response, 'SETUP_FAILED')
    requireThat(session.errors === 0, 'RUNTIME_ERROR')
    return { name: 'real initial setup wizard', ok: true, sha: options.sha }
  })
}

export async function probePersonalBrowserJourney(options = {}) {
  return withBrowser(options, true, async (browser, scope) => {
    const { origin, upstream, writer } = scope
    const screenshots = []
    const checks = []
    const fetchImpl = options.fetchImpl || globalThis.fetch
    const api = async (session, pathname, method = 'GET', body, allowFailure = false, timeout = 10_000) => bounded(scope, async () => {
      requireThat(pathname.startsWith('/api/') && !pathname.includes('://'), 'SCOPE_REJECTED')
      const response = await fetchImpl(origin + pathname, { method, redirect: 'error', signal: scope.abort.signal,
        headers: { Authorization: `Bearer ${session.token}`, 'Content-Type': 'application/json', Origin: origin },
        ...(body === undefined ? {} : { body: JSON.stringify(body) }) })
      const payload = await response.json()
      if (allowFailure) return { status: response.status, payload }
      requireThat(response.status >= 200 && response.status < 300 && payload?.success === true, 'API_FAILED')
      return payload.data
    }, timeout)
    const control = async () => bounded(scope, async () => {
      const response = await fetchImpl(upstream + '/__smoke__/control', { redirect: 'error', signal: scope.abort.signal })
      const value = await response.json()
      requireThat(response.status === 200 && Number.isSafeInteger(value.count) && value.count >= 0 &&
        value.request_ok === true, 'UPSTREAM_MISMATCH')
      return value.count
    }, 10_000)
    const root = await newSession(browser, origin, 'en', 1280, scope)
    await login(root, options.username, options.password, 100, options.sha)
    scope.stage = 'writer-check'
    const writerState = await api(root, '/api/quota-writer/status')
    requireThat(writerState.state?.mode === writer, 'WRITER_MISMATCH')
    scope.stage = 'fixture-config'
    const status = await api(root, '/api/status')
    requireThat(status.user_funding_mode === 'disabled' && status.enable_batch_update === false, 'STATE_MISMATCH')
    await api(root, '/api/option/', 'PUT', { key: 'general_setting.quota_display_type', value: 'TOKENS' })
    // The out-of-browser fixture PUT does not update this mounted client's
    // system-config store. Reload once before relying on internal-unit labels.
    scope.stage = 'fixture-config-refresh'
    await open(root, '/keys', { forceLoad: true })
    const rootPolicy = await api(root, `/api/user/${root.user.id}/usage-policy`)
    requireThat(rootPolicy.no_balance === true && rootPolicy.legacy_remaining_quota === 0, 'POLICY_MISMATCH')
    const rootDialog = await policyView(root, true)
    await capture(root, options, 'fresh-policy', screenshots)
    await closeDialog(root, rootDialog)
    const rootKey = await createKey(root, api, 'browser-root-finite', quota)
    await capture(root, options, 'fresh-key-list', screenshots)
    const rootBefore = await snapshot(api, root, rootKey)
    const startCount = await control()
    await preparePlayground(root, rootKey)
    await capture(root, options, 'fresh-selected-key', screenshots)
    const rootRequest = await sendPlayground(root, rootKey)
    await verifyUsage(api, root, rootKey, rootBefore, rootRequest, true)
    requireThat(await control() === startCount + 1, 'UPSTREAM_MISMATCH')
    await capture(root, options, 'fresh-success', screenshots)
    await showPersistedUsage(root, rootKey, rootRequest, options, screenshots)
    await capture(root, options, 'fresh-persisted-log', screenshots)
    await closeDialog(root, root.page.getByRole('dialog'))
    checks.push({ name: 'real English desktop fresh Root selected-Key streaming request and persisted usage', ok: true, quota: usage, requests: 1 })

    // An ordinary pre-existing-style user starts with the existing stored-wallet
    // policy. A true role-1 login, not Root impersonation, drives its entire UI.
    scope.stage = 'fixture-config'
    const ownerName = 'browserowner'
    const ownerPassword = randomBytes(9).toString('hex')
    await api(root, '/api/user/', 'POST', { username: ownerName, password: ownerPassword, display_name: 'Browser Owner', role: 1 })
    const owners = (await api(root, `/api/user/search?keyword=${ownerName}&p=1&page_size=10`)).items?.filter((user) => user.username === ownerName)
    requireThat(owners?.length === 1 && owners[0].role === 1 && Number.isSafeInteger(owners[0].id), 'STATE_MISMATCH')
    const owner = await newSession(browser, origin, 'zh', 320, scope)
    await login(owner, ownerName, ownerPassword, 1, options.sha)
    requireThat(owner.user.id === owners[0].id && owner.user.id !== root.user.id, 'STATE_MISMATCH')
    const oldPolicy = await api(owner, `/api/user/${owner.user.id}/usage-policy`)
    requireThat(oldPolicy.no_balance === false && oldPolicy.legacy_remaining_quota === 0 && oldPolicy.revision === 0, 'POLICY_MISMATCH')
    const ownerDialog = await policyView(owner, false)
    await capture(owner, options, 'existing-policy-before', screenshots)
    await closeDialog(owner, ownerDialog)
    const forbidden = await api(owner, `/api/user/${owner.user.id}/usage-policy`, 'PUT', {
      id: randomBytes(32).toString('hex'), expected_revision: oldPolicy.revision, no_balance: true, confirmed: true,
    }, true)
    requireThat(forbidden.status === 403 && forbidden.payload?.success === false, 'POLICY_MISMATCH')
    const crossWrite = await api(owner, `/api/user/${root.user.id}/usage-policy`, 'PUT', {
      id: randomBytes(32).toString('hex'), expected_revision: rootPolicy.revision, no_balance: false, confirmed: true,
    }, true)
    requireThat(crossWrite.status === 403 && crossWrite.payload?.success === false, 'POLICY_MISMATCH')
    const crossOwner = await api(owner, `/api/user/${root.user.id}/usage-policy`, 'GET', undefined, true)
    requireThat(crossOwner.status === 404 || crossOwner.status === 403 ||
      crossOwner.status === 200 && crossOwner.payload?.success === false, 'POLICY_MISMATCH')
    const unchangedPolicy = await api(owner, `/api/user/${owner.user.id}/usage-policy`)
    requireThat(unchangedPolicy.no_balance === false && unchangedPolicy.revision === oldPolicy.revision, 'POLICY_MISMATCH')
    const ownerKey = await createKey(owner, api, 'browser-owner-finite', quota)
    await capture(owner, options, 'existing-key-list', screenshots)
    const ownerBefore = await snapshot(api, owner, ownerKey)
    await preparePlayground(owner, ownerKey)
    await capture(owner, options, 'existing-selected-key', screenshots)
    const deniedRequest = await sendPlayground(owner, ownerKey, 'insufficient_user_quota')
    await verifyUsage(api, owner, ownerKey, ownerBefore, deniedRequest, false)
    requireThat(await control() === startCount + 1, 'UPSTREAM_MISMATCH')
    await capture(owner, options, 'existing-wallet-rejection', screenshots)

    // Root reviews this specific ordinary user, confirms, and submits through
    // the real policy dialog. API readback checks the audited revision change.
    scope.stage = 'policy-confirm'
    await open(root, `/users?filter=${ownerName}`)
    const row = root.page.getByRole('row').filter({ hasText: ownerName })
    await row.getByRole('button', { name: root.label('Open menu'), exact: true }).click()
    await root.page.getByRole('menuitem', { name: root.label('User usage policy'), exact: true }).click()
    const choice = root.page.getByRole('dialog', { name: root.label('User usage policy'), exact: true })
    await choice.getByRole('checkbox', { name: root.label('Use Key limits without a user wallet'), exact: true }).check()
    await choice.getByRole('checkbox', { name: root.label('I confirm all running instances support this policy and I understand the supported request paths.'), exact: true }).check()
    const saved = responseFor(root, `/api/user/${owner.user.id}/usage-policy`, 'PUT')
    await choice.getByRole('button', { name: root.label('Save'), exact: true }).click()
    const confirmed = await responseSuccess(root, await saved, 'POLICY_MISMATCH')
    requireThat(confirmed.user_id === owner.user.id && confirmed.no_balance === true &&
      confirmed.revision === oldPolicy.revision + 1 && confirmed.legacy_remaining_quota === 0, 'POLICY_MISMATCH')
    await closeDialog(root, choice)
    const afterChoice = await api(owner, `/api/user/${owner.user.id}/usage-policy`)
    requireThat(afterChoice.no_balance === true && afterChoice.revision === confirmed.revision && afterChoice.legacy_remaining_quota === 0, 'POLICY_MISMATCH')
    const activeDialog = await policyView(owner, true, confirmed.revision)
    await capture(owner, options, 'existing-policy-after', screenshots)
    await closeDialog(owner, activeDialog)
    const ownerEnabled = await snapshot(api, owner, ownerKey)
    requireThat(ownerEnabled.user.quota === ownerBefore.user.quota && ownerEnabled.user.used_quota === ownerBefore.user.used_quota &&
      ownerEnabled.user.request_count === ownerBefore.user.request_count, 'USAGE_MISMATCH')
    await preparePlayground(owner, ownerKey)
    const ownerRequest = await sendPlayground(owner, ownerKey)
    await verifyUsage(api, owner, ownerKey, ownerEnabled, ownerRequest, true)
    requireThat(await control() === startCount + 2, 'UPSTREAM_MISMATCH')
    await capture(owner, options, 'existing-success', screenshots)
    await showPersistedUsage(owner, ownerKey, ownerRequest, options, screenshots)
    await capture(owner, options, 'existing-persisted-log', screenshots)
    await closeDialog(owner, owner.page.getByRole('dialog'))
    checks.push({ name: 'real Chinese mobile ordinary owner rejects until explicit Root policy choice', ok: true, role: 1, policyChanges: 1, requests: 1, quota: usage })

    const lowKey = await createKey(owner, api, 'browser-owner-low', 1)
    const lowBefore = await snapshot(api, owner, lowKey)
    await preparePlayground(owner, lowKey)
    const lowRequest = await sendPlayground(owner, lowKey,
      writer === 'authoritative' ? 'insufficient_user_quota' : 'pre_consume_token_quota_failed')
    await verifyUsage(api, owner, lowKey, lowBefore, lowRequest, false)
    requireThat(await control() === startCount + 2, 'UPSTREAM_MISMATCH')
    await capture(owner, options, 'finite-quota-rejection', screenshots)
    checks.push({ name: 'finite ordinary internal quota rejection leaves wallet, Key and consume usage unchanged', ok: true, remainingQuota: 1, additionalUpstreamRequests: 0 })

    // Both strict dimensions are deliberately enabled on an unqualified
    // loopback/model/request combination. This does not isolate the host gate
    // or prove native-provider positives, strict settlement, or concurrency.
    const strictKey = await createKey(root, api, 'browser-root-strict-reject', quota)
    scope.stage = 'strict-budget'
    await open(root, `/keys?filter=${strictKey.name}`)
    await root.page.getByRole('button', { name: root.label('API Key usage budgets'), exact: true }).click()
    const budgetDialog = root.page.getByRole('dialog', { name: root.label('API Key usage budgets'), exact: true })
    await budgetDialog.getByRole('checkbox', { name: root.label('Enable strict Token budget'), exact: true }).check()
    await budgetDialog.getByRole('textbox', { name: root.label('Token total limit'), exact: true }).fill('1000')
    await budgetDialog.getByRole('checkbox', { name: root.label('Enable USD fee budget'), exact: true }).check()
    await budgetDialog.getByRole('textbox', { name: root.label('USD total limit'), exact: true }).fill('1')
    await budgetDialog.getByRole('checkbox', { name: root.label('I confirm all running instances support this budget and I understand its request restrictions.'), exact: true }).check()
    const budgetSaved = responseFor(root, `/api/token/${strictKey.id}/budget`, 'PUT')
    await budgetDialog.getByRole('button', { name: root.label('Save'), exact: true }).click()
    await responseSuccess(root, await budgetSaved, 'POLICY_MISMATCH')
    await closeDialog(root, budgetDialog)
    const strictBefore = await snapshot(api, root, strictKey)
    await preparePlayground(root, strictKey)
    const strictRequest = await sendPlayground(root, strictKey, 'token_budget_unsupported_request')
    await verifyUsage(api, root, strictKey, strictBefore, strictRequest, false)
    const budget = await api(root, `/api/token/${strictKey.id}/budget`)
    requireThat(budget.policy?.enabled === true && budget.policy.fee_enabled === true &&
      budget.policy.used === 0 && budget.policy.reserved === 0 && budget.policy.fee_used_usd === '0' &&
      budget.policy.fee_reserved_usd === '0' && budget.policy.pending_request_id === '' && budget.pending === null, 'USAGE_MISMATCH')
    requireThat(await control() === startCount + 2, 'UPSTREAM_MISMATCH')
    await capture(root, options, 'unqualified-strict-rejection', screenshots)
    checks.push({ name: 'unqualified loopback/model/request combination rejects strict Token and USD before dispatch or reservation', ok: true, additionalUpstreamRequests: 0, reserved: 0 })

    // Reload-and-read proves application-backed state, rather than a UI-only
    // result. DB restoration belongs to independent existing restore tests;
    // this personal application journey does not validate restart or recovery.
    scope.stage = 'reload-check'
    await open(owner, `/keys?filter=${ownerKey.name}`, { forceLoad: true })
    await owner.page.getByText(ownerKey.name, { exact: true }).filter({ visible: true }).waitFor()
    const persisted = await snapshot(api, owner, ownerKey)
    requireThat(persisted.token.remain_quota === quota - usage && persisted.token.used_quota === usage &&
      persisted.user.quota === 0 && persisted.user.role === 1 && persisted.user.used_quota === usage && persisted.user.request_count === 1, 'USAGE_MISMATCH')
    const finalWriter = await api(root, '/api/quota-writer/status')
    requireThat(finalWriter.state?.mode === writer, 'WRITER_MISMATCH')
    requireThat(root.errors === 0 && owner.errors === 0 && root.reveals === 0 && owner.reveals === 0 &&
      root.requests === 2 && owner.requests === 3, 'RUNTIME_ERROR')
    checks.push({ name: 'browser reload preserves finite Key and ordinary-owner counters without relay replay', ok: true, remainingQuota: quota - usage, usedQuota: usage })
    return { passed: true, sha: options.sha, writer, checks, screenshots }
  })
}
