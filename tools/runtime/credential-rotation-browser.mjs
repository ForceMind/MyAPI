// Production frontend -> real Go credential-rotation runtime journey.
// Authentication is an isolated signed session. Maintenance, rotation,
// verification, receipt and Key authentication routes are never fulfilled by
// the browser fixture.
import { createHmac } from 'node:crypto'
import {
  lstatSync,
  mkdirSync,
  readFileSync,
  realpathSync,
  writeFileSync,
} from 'node:fs'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

import { createUIFixture } from '../ui/browser-fixtures.mjs'

const repo = fileURLToPath(new URL('../../', import.meta.url))
const sensitiveInputSelector =
  'input[id^="rotated-key-value-"], input[inputmode="numeric"], input[type="password"], textarea'
const visibleSensitiveInputSelector =
  'input[id^="rotated-key-value-"]:visible, input[inputmode="numeric"]:visible, input[type="password"]:visible, textarea:visible'
const inputMask = `
  ${sensitiveInputSelector} {
    -webkit-text-security: disc !important;
    color: transparent !important;
    caret-color: transparent !important;
  }
`

function fail(code) {
  throw new Error(`CREDENTIAL_ROTATION_RUNTIME_${code}`)
}

function requireThat(condition, code) {
  if (!condition) fail(code)
}

function readConfig() {
  requireThat(process.argv.length === 3, 'CONFIG')
  const configPath = process.argv[2]
  let value
  try {
    const info = lstatSync(configPath)
    requireThat(
      info.isFile() && !info.isSymbolicLink() && (info.mode & 0o777) === 0o600,
      'CONFIG_PRIVATE'
    )
    value = JSON.parse(readFileSync(configPath, 'utf8'))
  } catch {
    fail('CONFIG')
  }
  requireThat(
    value && typeof value === 'object' && !Array.isArray(value),
    'CONFIG'
  )
  let target
  try {
    target = new URL(value.origin)
  } catch {
    fail('SCOPE')
  }
  requireThat(
    target.protocol === 'http:' &&
      target.hostname === '127.0.0.1' &&
      Boolean(target.port) &&
      !target.username &&
      !target.password &&
      target.pathname === '/' &&
      !target.search &&
      !target.hash &&
      target.origin === value.origin,
    'SCOPE'
  )
  requireThat(
    (value.language === 'en' && value.width === 1440) ||
      (value.language === 'zh' && value.width === 390),
    'VIEWPORT'
  )
  requireThat(
    typeof value.access_token === 'string' && value.access_token.length > 20,
    'AUTH'
  )
  requireThat(
    value.session &&
      typeof value.session.sid === 'string' &&
      value.session.sid.length > 0 &&
      value.session.current === true &&
      value.session.login_method === 'password' &&
      Number.isSafeInteger(value.session.expires_at),
    'AUTH'
  )
  requireThat(
    value.user &&
      value.user.id === 1 &&
      typeof value.user.username === 'string' &&
      value.user.username.length > 0 &&
      value.user.role === 100 &&
      value.user.status === 1 &&
      value.user.group === 'default' &&
      value.user.auth_version === 1,
    'AUTH'
  )
  requireThat(
    typeof value.totp_secret === 'string' &&
      /^[A-Z2-7]+=*$/.test(value.totp_secret),
    'AUTH'
  )
  requireThat(
    typeof value.synthetic_old_key === 'string' &&
      /^(?:sk-)?[A-Za-z0-9_-]{48}$/.test(value.synthetic_old_key),
    'AUTH'
  )
  requireThat(
    Number.isSafeInteger(value.token_id) && value.token_id > 0,
    'CONFIG'
  )
  requireThat(
    typeof value.version === 'string' && value.version.trim().length > 0,
    'BUILD'
  )
  requireThat(/^[0-9a-f]{40}$/.test(value.source_head || ''), 'BUILD')
  try {
    requireThat(path.isAbsolute(value.output_dir), 'OUTPUT')
    mkdirSync(value.output_dir, { recursive: true, mode: 0o700 })
    const info = lstatSync(value.output_dir)
    requireThat(
      info.isDirectory() &&
        !info.isSymbolicLink() &&
        realpathSync(value.output_dir) === path.resolve(value.output_dir),
      'OUTPUT'
    )
  } catch {
    fail('OUTPUT')
  }
  return { ...value, origin: target.origin }
}

function decodeBase32(value) {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  let bits = ''
  for (const character of value.replace(/=+$/, '')) {
    const index = alphabet.indexOf(character)
    requireThat(index >= 0, 'AUTH')
    bits += index.toString(2).padStart(5, '0')
  }
  const bytes = []
  for (let offset = 0; offset + 8 <= bits.length; offset += 8) {
    bytes.push(Number.parseInt(bits.slice(offset, offset + 8), 2))
  }
  return Buffer.from(bytes)
}

function currentTOTP(secret) {
  const counter = Math.floor(Date.now() / 30_000)
  const message = Buffer.alloc(8)
  message.writeBigUInt64BE(BigInt(counter))
  const digest = createHmac('sha1', decodeBase32(secret))
    .update(message)
    .digest()
  const offset = digest[digest.length - 1] & 0x0f
  const value =
    (((digest[offset] & 0x7f) << 24) |
      (digest[offset + 1] << 16) |
      (digest[offset + 2] << 8) |
      digest[offset + 3]) %
    1_000_000
  return String(value).padStart(6, '0')
}

function successData(payload) {
  requireThat(
    payload && payload.success === true && payload.data,
    'API_RESPONSE'
  )
  return payload.data
}

function hasSecretField(value) {
  if (!value || typeof value !== 'object') return false
  if (Array.isArray(value)) return value.some(hasSecretField)
  return Object.entries(value).some(
    ([key, child]) =>
      ['secret', 'new_secret', 'raw_secret', 'key'].includes(
        key.toLowerCase()
      ) || hasSecretField(child)
  )
}

function redactSensitiveText(value, secrets) {
  let message = String(value)
  for (const secret of secrets) {
    if (secret) message = message.split(secret).join('[redacted]')
  }
  return message
    .replace(/Bearer\s+[A-Za-z0-9._~-]+/gi, 'Bearer [redacted]')
    .replace(/sk-[A-Za-z0-9_-]{48}/g, 'sk-[redacted]')
    .replace(/\b\d{6}\b/g, '[redacted-code]')
}

function safeFailure(error, secrets) {
  return redactSensitiveText(
    typeof error?.stack === 'string'
      ? error.stack
      : typeof error?.message === 'string'
        ? error.message
        : 'runtime failure',
    secrets
  ).slice(0, 240)
}

const config = readConfig()
const translations = JSON.parse(
  readFileSync(
    path.join(repo, 'web', 'src', 'i18n', 'locales', `${config.language}.json`),
    'utf8'
  )
).translation
const t = (key) => {
  requireThat(
    typeof translations[key] === 'string' && translations[key].length > 0,
    'TRANSLATION'
  )
  return translations[key]
}
const fixture = createUIFixture({
  role: 100,
  language: config.language === 'zh' ? 'zhCN' : 'en',
  personalPolicy: true,
})
const metrics = {
  maintenance: { requests: 0, statuses: [] },
  preview: { requests: 0, statuses: [] },
  verification: { requests: 0, statuses: [] },
  freshCommit: { requests: 0, statuses: [] },
  receipt: { requests: 0, statuses: [] },
  replay: { requests: 0, statuses: [] },
  oldKeyAuth: { requests: 0, statuses: [] },
  newKeyAuth: { requests: 0, statuses: [] },
  syntheticConsole: { requests: 0, statuses: [] },
  externalAborted: 0,
  pageErrors: 0,
}
const report = {
  schema: 1,
  result: 'running',
  realMaintenanceAPI: true,
  realRotationAPI: true,
  realVerificationAPI: true,
  realReceiptAPI: true,
  realKeyAuthenticationAPI: true,
  otherConsoleAPIsSynthetic: true,
  syntheticDatabase: true,
  language: config.language,
  width: config.width,
  buildSource: {
    version: config.version,
    sourceHead: config.source_head,
  },
  evidence: {
    real: 'Maintenance, rotation preview, verification, fresh commit, receipt, replay and Key authentication reached the real isolated Go server.',
    console:
      'Only unrelated console bootstrap, list and status APIs used createUIFixture.',
  },
  metrics,
  checks: {},
  screenshots: [],
}
const reportPath = path.join(
  config.output_dir,
  `credential-rotation-runtime-${config.language}-${config.width}.json`
)
const persist = () =>
  writeFileSync(
    reportPath,
    redactSensitiveText(JSON.stringify(report, null, 2), [
      config.access_token,
      config.totp_secret,
      config.synthetic_old_key,
      config.session.sid,
      generatedCode,
      freshSecret,
      capturedProof,
      capturedCommitBody,
    ]),
    { mode: 0o600 }
  )
const screenshotName = (suffix) =>
  `credential-rotation-runtime-${config.language}-${config.width}-${suffix}.png`

let stage = 'launch'
let browser
let context
let page
let generatedCode = ''
let freshSecret = ''
let capturedCommitBody = ''
let capturedProof = ''

function browserRealKind(pathname, method) {
  const base = `/api/token/${config.token_id}`
  if (pathname === `${base}/maintenance` && method === 'GET') {
    return 'maintenance'
  }
  if (pathname === `${base}/credential-rotation/preview` && method === 'POST') {
    return 'preview'
  }
  if (pathname === `${base}/credential-rotation` && method === 'POST') {
    return 'freshCommit'
  }
  if (pathname === '/api/verify' && method === 'POST') return 'verification'
  return null
}

async function capture(suffix) {
  stage = `screenshot-${suffix}`
  await page.addStyleTag({ content: inputMask })
  await page.evaluate(async () => {
    await document.fonts.ready
    await Promise.all(
      document
        .getAnimations()
        .map((animation) => animation.finished.catch(() => {}))
    )
    await new Promise((done) =>
      requestAnimationFrame(() => requestAnimationFrame(done))
    )
  })
  const file = screenshotName(suffix)
  await page.screenshot({
    path: path.join(config.output_dir, file),
    animations: 'disabled',
    fullPage: true,
    mask: [page.locator(visibleSensitiveInputSelector)],
    maskColor: '#111827',
  })
  report.screenshots.push(file)
  persist()
}

async function realFetch(kind, pathname, options = {}) {
  requireThat(pathname.startsWith('/'), 'REAL_SCOPE')
  const url = new URL(pathname, config.origin)
  requireThat(url.origin === config.origin, 'REAL_SCOPE')
  metrics[kind].requests += 1
  const response = await fetch(url, {
    cache: 'no-store',
    redirect: 'error',
    ...options,
  })
  metrics[kind].statuses.push(response.status)
  const body = await response.json().catch(() => null)
  return { status: response.status, body }
}

try {
  persist()
  stage = 'load-playwright'
  const playwrightPath = process.env.MYAPI_PLAYWRIGHT_MODULE
  const module = await import(
    playwrightPath
      ? pathToFileURL(path.resolve(playwrightPath)).href
      : 'playwright'
  )
  requireThat(typeof module?.chromium?.launch === 'function', 'BROWSER')
  browser = await module.chromium.launch({
    headless: true,
    args: ['--disable-dev-shm-usage'],
  })
  stage = 'create-context'
  context = await browser.newContext({
    viewport: {
      width: config.width,
      height: config.width === 390 ? 844 : 960,
    },
    locale: config.language === 'zh' ? 'zh-CN' : 'en-US',
    timezoneId: 'Asia/Shanghai',
    reducedMotion: 'reduce',
    serviceWorkers: 'block',
    permissions: ['clipboard-read', 'clipboard-write'],
  })
  await context.addInitScript(
    ({ origin, language }) => {
      if (location.origin !== origin) return
      localStorage.setItem('i18nextLng', language === 'zh' ? 'zhCN' : 'en')
      localStorage.setItem('theme', 'light')
    },
    { origin: config.origin, language: config.language }
  )
  await context.route('**/*', async (route) => {
    const request = route.request()
    let url
    try {
      url = new URL(request.url())
    } catch {
      metrics.externalAborted += 1
      return route.abort('blockedbyclient')
    }
    if (url.origin !== config.origin) {
      metrics.externalAborted += 1
      return route.abort('blockedbyclient')
    }
    const pathname = url.pathname.replace(/\/$/, '') || '/'
    const method = request.method()
    const kind = browserRealKind(pathname, method)
    if (kind) {
      requireThat(
        request.headers().authorization === `Bearer ${config.access_token}`,
        'REAL_AUTH'
      )
      if (kind === 'freshCommit') {
        requireThat(
          capturedCommitBody === '' && capturedProof === '',
          'COMMIT_ONCE'
        )
        capturedCommitBody = request.postData() || ''
        capturedProof = request.headers()['x-security-proof'] || ''
        requireThat(capturedCommitBody.length > 0, 'COMMIT_BODY')
        requireThat(capturedProof.length > 20, 'REAL_PROOF')
      }
      metrics[kind].requests += 1
      return route.continue()
    }
    if (pathname === '/api/user/auth/refresh' && method === 'POST') {
      metrics.syntheticConsole.requests += 1
      const now = Math.floor(Date.now() / 1000)
      return route.fulfill({
        status: 200,
        json: {
          success: true,
          data: {
            access_token: config.access_token,
            token_type: 'Bearer',
            access_expires_at: config.session.expires_at,
            user: {
              ...fixture.user,
              ...config.user,
              language: config.language === 'zh' ? 'zhCN' : 'en',
            },
            session: {
              ip: '127.0.0.1',
              user_agent: 'Synthetic isolated rotation runtime Chromium',
              created_at: now - 60,
              last_active_at: now,
              ...config.session,
            },
          },
        },
      })
    }
    if (pathname === '/api/user/2fa/status' && method === 'GET') {
      metrics.syntheticConsole.requests += 1
      return route.fulfill({
        status: 200,
        json: { success: true, data: { enabled: true } },
      })
    }
    if (pathname === '/api/user/passkey' && method === 'GET') {
      metrics.syntheticConsole.requests += 1
      return route.fulfill({
        status: 200,
        json: { success: true, data: { enabled: false } },
      })
    }
    if (pathname.startsWith('/api/') || pathname.startsWith('/pg/')) {
      const result = fixture.response(url, method)
      requireThat(!result.violation, 'UNCONFIGURED_CONSOLE_API')
      metrics.syntheticConsole.requests += 1
      return route.fulfill({ status: result.status, json: result.body })
    }
    return route.continue()
  })
  page = await context.newPage()
  page.setDefaultTimeout(20_000)
  page.on('pageerror', () => {
    metrics.pageErrors += 1
  })
  page.on('response', (response) => {
    try {
      const url = new URL(response.url())
      if (url.origin !== config.origin) return
      const pathname = url.pathname.replace(/\/$/, '') || '/'
      const kind = browserRealKind(pathname, response.request().method())
      if (kind) metrics[kind].statuses.push(response.status())
      else if (pathname.startsWith('/api/') || pathname.startsWith('/pg/')) {
        metrics.syntheticConsole.statuses.push(response.status())
      }
    } catch {}
  })

  stage = 'open-keys'
  await page.goto(`${config.origin}/keys`, { waitUntil: 'domcontentloaded' })
  // The console shows its build number at the foot of the account menu on
  // every width; the mobile top bar no longer carries it.
  await page
    .getByRole('button', { name: t('Account menu'), exact: true })
    .click()
  const visibleVersion = page.getByTestId('console-build-revision')
  await visibleVersion.waitFor({ state: 'visible' })
  const buildRevision = await page
    .locator('html')
    .getAttribute('data-build-rev')
  requireThat(
    buildRevision?.startsWith(`rv.${config.version}.${config.source_head}.`),
    'BUILD_REVISION'
  )
  requireThat(
    (await visibleVersion.textContent())?.trim() === config.version,
    'VISIBLE_VERSION'
  )
  await page.keyboard.press('Escape')
  await visibleVersion.waitFor({ state: 'hidden' })
  report.checks.exactProductionBuild = true

  const maintenanceEntry = page
    .getByRole('button', {
      name: t('Credential maintenance'),
      exact: true,
    })
    .first()
  await maintenanceEntry.waitFor()
  const maintenanceResponsePromise = page.waitForResponse(
    (response) =>
      response.request().method() === 'GET' &&
      new URL(response.url()).pathname ===
        `/api/token/${config.token_id}/maintenance`
  )
  await maintenanceEntry.focus()
  requireThat(
    await maintenanceEntry.evaluate(
      (element) => document.activeElement === element
    ),
    'KEYBOARD'
  )
  stage = 'open-maintenance'
  await page.keyboard.press('Enter')
  const maintenanceResponse = await maintenanceResponsePromise
  requireThat(maintenanceResponse.status() === 200, 'MAINTENANCE_STATUS')
  const maintenance = successData(await maintenanceResponse.json())
  requireThat(
    maintenance.token?.id === config.token_id &&
      maintenance.token?.owner_user_id === config.user.id &&
      maintenance.rotation?.status === 'ready' &&
      maintenance.rotation?.commit_supported === true,
    'MAINTENANCE_CONTRACT'
  )
  const maintenanceDialog = page.getByRole('dialog', {
    name: t('Credential maintenance'),
    exact: true,
  })
  await maintenanceDialog.waitFor()
  const openRotation = maintenanceDialog.getByRole('button', {
    name: t('Check and rotate Key'),
    exact: true,
  })
  await openRotation.scrollIntoViewIfNeeded()
  await openRotation.click()
  const rotationDialog = page.getByRole('dialog', {
    name: t('Credential rotation'),
    exact: true,
  })
  await rotationDialog.waitFor()
  report.checks.rootOwnedReadyKey = true

  stage = 'preview'
  const previewButton = rotationDialog.getByRole('button', {
    name: t('Check rotation eligibility'),
    exact: true,
  })
  const previewResponsePromise = page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' &&
      new URL(response.url()).pathname ===
        `/api/token/${config.token_id}/credential-rotation/preview`
  )
  await previewButton.focus()
  await page.keyboard.press('Enter')
  const previewResponse = await previewResponsePromise
  requireThat(previewResponse.status() === 200, 'PREVIEW_STATUS')
  const preview = successData(await previewResponse.json())
  requireThat(
    preview.status === 'ready' &&
      /^[0-9a-f]{64}$/.test(preview.preview_revision || '') &&
      Number.isSafeInteger(preview.credential_version_before) &&
      preview.credential_version_before > 0 &&
      preview.credential_version_after ===
        preview.credential_version_before + 1 &&
      preview.effects?.token_id_preserved === true &&
      preview.effects?.old_secret_invalidation === 'immediate' &&
      preview.effects?.accepted_settlement === 'continues' &&
      preview.effects?.writer_changed === false,
    'PREVIEW_CONTRACT'
  )
  await rotationDialog
    .getByText(t('Rotation is ready'), { exact: true })
    .waitFor()
  report.checks.previewReady = true

  stage = 'confirm'
  const confirmations = rotationDialog.getByRole('checkbox')
  requireThat((await confirmations.count()) === 3, 'CONFIRMATIONS')
  for (let index = 0; index < 3; index += 1) {
    await confirmations.nth(index).focus()
    await page.keyboard.press('Space')
    requireThat(await confirmations.nth(index).isChecked(), 'CONFIRMATIONS')
  }
  const verifyAndRotate = rotationDialog.getByRole('button', {
    name: t('Verify and rotate Key'),
    exact: true,
  })
  await verifyAndRotate.scrollIntoViewIfNeeded()
  const actionVisible = await verifyAndRotate.evaluate((element) => {
    const box = element.getBoundingClientRect()
    const hit = document.elementFromPoint(
      box.x + box.width / 2,
      box.y + box.height / 2
    )
    const nav = document
      .querySelector('nav.pc-mobile-nav')
      ?.getBoundingClientRect()
    return (
      box.top >= 0 &&
      box.bottom <= innerHeight &&
      (!nav || nav.width === 0 || box.bottom <= nav.top) &&
      Boolean(hit && element.contains(hit))
    )
  })
  requireThat(actionVisible, 'CONFIRM_ACTION_VISIBLE')
  report.checks.threeConfirmations = true
  report.checks.confirmActionVisible = true
  await capture('review-confirmed')

  stage = 'verify-and-commit'
  await verifyAndRotate.focus()
  await page.keyboard.press('Enter')
  const verificationDialog = page.getByRole('dialog', {
    name: t('Verify credential rotation'),
    exact: true,
  })
  const codeInput = verificationDialog.getByPlaceholder(
    t('Enter verification code')
  )
  await codeInput.waitFor()
  generatedCode = currentTOTP(config.totp_secret)
  await codeInput.fill(generatedCode)
  const proofResponsePromise = page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' &&
      new URL(response.url()).pathname === '/api/verify'
  )
  const commitResponsePromise = page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' &&
      new URL(response.url()).pathname ===
        `/api/token/${config.token_id}/credential-rotation`
  )
  const maintenanceRefreshPromise = page.waitForResponse(
    (response) =>
      response.request().method() === 'GET' &&
      new URL(response.url()).pathname ===
        `/api/token/${config.token_id}/maintenance`
  )
  await page.keyboard.press('Enter')
  const proofResponse = await proofResponsePromise
  requireThat(proofResponse.status() === 200, 'VERIFY_STATUS')
  const proof = successData(await proofResponse.json())
  requireThat(
    proof.method === '2fa' &&
      proof.scope === 'token.credential_rotation' &&
      typeof proof.proof_token === 'string' &&
      proof.proof_token.length > 20,
    'VERIFY_CONTRACT'
  )
  const commitResponse = await commitResponsePromise
  requireThat(commitResponse.status() === 200, 'COMMIT_STATUS')
  const commit = successData(await commitResponse.json())
  requireThat(
    commit.replayed === false &&
      commit.receipt?.state === 'committed' &&
      commit.receipt?.token_id === config.token_id &&
      /^[0-9a-f]{64}$/.test(commit.receipt?.operation_id || '') &&
      commit.receipt?.credential_version_before ===
        preview.credential_version_before &&
      commit.receipt?.credential_version_after ===
        preview.credential_version_after &&
      commit.receipt?.settlement_mode === 'authoritative_persistent' &&
      commit.receipt?.writer_changed === false &&
      typeof commit.reveal?.secret === 'string' &&
      /^sk-[A-Za-z0-9_-]{48}$/.test(commit.reveal.secret),
    'FRESH_COMMIT_CONTRACT'
  )
  freshSecret = commit.reveal.secret
  const maintenanceRefresh = await maintenanceRefreshPromise
  requireThat(maintenanceRefresh.status() === 200, 'MAINTENANCE_REFRESH')
  requireThat(capturedCommitBody.length > 0, 'COMMIT_BODY')
  let capturedCommand
  try {
    capturedCommand = JSON.parse(capturedCommitBody)
  } catch {
    fail('COMMIT_BODY')
  }
  requireThat(
    Object.keys(capturedCommand).sort().join(',') ===
      'confirmed,operation_id,preview_revision' &&
      capturedCommand.confirmed === true &&
      capturedCommand.operation_id === commit.receipt.operation_id &&
      capturedCommand.preview_revision === preview.preview_revision,
    'COMMIT_BODY'
  )
  requireThat(capturedProof === proof.proof_token, 'PROOF_IDENTITY')
  requireThat(metrics.freshCommit.requests === 1, 'FRESH_COMMIT_COUNT')
  await rotationDialog
    .getByText(t('Key value rotated'), { exact: true })
    .waitFor()
  const secretInput = rotationDialog.getByLabel(t('New Key value'), {
    exact: true,
  })
  requireThat(
    (await secretInput.inputValue()) === freshSecret,
    'FRESH_REVEAL_VALUE'
  )
  report.checks.realVerification = true
  report.checks.freshCommitOnce = true
  report.checks.freshRevealOnce = true
  await capture('fresh-secret-masked')

  stage = 'key-authentication'
  const oldAuth = await realFetch('oldKeyAuth', '/api/rotation-test/auth', {
    method: 'GET',
    headers: {
      Authorization: `Bearer ${config.synthetic_old_key.startsWith('sk-') ? config.synthetic_old_key : `sk-${config.synthetic_old_key}`}`,
    },
  })
  requireThat(oldAuth.status === 401, 'OLD_KEY_AUTH')
  const newAuth = await realFetch('newKeyAuth', '/api/rotation-test/auth', {
    method: 'GET',
    headers: { Authorization: `Bearer ${freshSecret}` },
  })
  requireThat(newAuth.status === 200, 'NEW_KEY_AUTH')
  const newAuthData =
    newAuth.body?.success === true ? successData(newAuth.body) : newAuth.body
  requireThat(newAuthData.token_id === config.token_id, 'NEW_KEY_TOKEN_ID')
  report.checks.oldKeyRejected = true
  report.checks.newKeySameTokenID = true

  stage = 'receipt-readback'
  const receiptReadback = await realFetch(
    'receipt',
    `/api/token/${config.token_id}/credential-rotation/operations/${commit.receipt.operation_id}`,
    {
      method: 'GET',
      headers: { Authorization: `Bearer ${config.access_token}` },
    }
  )
  requireThat(receiptReadback.status === 200, 'RECEIPT_STATUS')
  const receipt = successData(receiptReadback.body)
  requireThat(
    receipt.operation_id === commit.receipt.operation_id &&
      receipt.state === 'committed' &&
      receipt.token_id === config.token_id &&
      receipt.credential_version_before ===
        commit.receipt.credential_version_before &&
      receipt.credential_version_after ===
        commit.receipt.credential_version_after &&
      receipt.masked_key === commit.receipt.masked_key &&
      receipt.settlement_mode === 'authoritative_persistent' &&
      receipt.writer_changed === false &&
      !hasSecretField(receipt) &&
      !JSON.stringify(receipt).includes(freshSecret),
    'RECEIPT_CONTRACT'
  )
  report.checks.receiptHasNoSecret = true

  stage = 'idempotent-replay'
  const replay = await realFetch(
    'replay',
    `/api/token/${config.token_id}/credential-rotation`,
    {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${config.access_token}`,
        Origin: config.origin,
        'Content-Type': 'application/json',
        'X-Security-Proof': capturedProof,
      },
      body: capturedCommitBody,
    }
  )
  requireThat(replay.status === 200, 'REPLAY_STATUS')
  const replayData = successData(replay.body)
  requireThat(
    replayData.replayed === true &&
      replayData.reveal === null &&
      replayData.receipt?.operation_id === commit.receipt.operation_id &&
      replayData.receipt?.token_id === config.token_id &&
      replayData.receipt?.credential_version_before ===
        commit.receipt.credential_version_before &&
      replayData.receipt?.credential_version_after ===
        commit.receipt.credential_version_after &&
      replayData.receipt?.masked_key === commit.receipt.masked_key &&
      !hasSecretField(replayData.receipt) &&
      !JSON.stringify(replayData).includes(freshSecret),
    'REPLAY_CONTRACT'
  )
  report.checks.sameOperationReplay = true
  report.checks.replayHasNoSecret = true
  report.checks.noSecondCredentialChange = true

  stage = 'close-and-clear'
  const saved = rotationDialog.getByRole('checkbox')
  requireThat((await saved.count()) === 1, 'SAVED_CONFIRMATION')
  await saved.focus()
  await page.keyboard.press('Space')
  requireThat(await saved.isChecked(), 'SAVED_CONFIRMATION')
  await rotationDialog
    .getByRole('button', { name: t('Done'), exact: true })
    .click()
  await rotationDialog.waitFor({ state: 'hidden' })
  const clearedSecretInputs = page.getByLabel(t('New Key value'), {
    exact: true,
  })
  requireThat(
    (await clearedSecretInputs.count()) === 0 ||
      (await clearedSecretInputs.evaluateAll((elements) =>
        elements.every((element) => element.value === '')
      )),
    'SECRET_CLEARED'
  )
  report.checks.closeClearsSecret = true
  await capture('closed-secret-cleared')
  freshSecret = ''

  stage = 'final-checks'
  requireThat(metrics.maintenance.requests === 2, 'MAINTENANCE_COUNT')
  requireThat(metrics.preview.requests === 1, 'PREVIEW_COUNT')
  requireThat(metrics.verification.requests === 1, 'VERIFY_COUNT')
  requireThat(metrics.freshCommit.requests === 1, 'FRESH_COMMIT_COUNT')
  requireThat(metrics.receipt.requests === 1, 'RECEIPT_COUNT')
  requireThat(metrics.replay.requests === 1, 'REPLAY_COUNT')
  requireThat(metrics.oldKeyAuth.requests === 1, 'OLD_AUTH_COUNT')
  requireThat(metrics.newKeyAuth.requests === 1, 'NEW_AUTH_COUNT')
  requireThat(metrics.externalAborted === 0, 'NETWORK_GUARD')
  requireThat(metrics.pageErrors === 0, 'PAGE_ERROR')
  requireThat(
    metrics.maintenance.statuses.length === 2 &&
      metrics.maintenance.statuses.every((status) => status === 200) &&
      metrics.preview.statuses.length === 1 &&
      metrics.preview.statuses[0] === 200 &&
      metrics.verification.statuses.length === 1 &&
      metrics.verification.statuses[0] === 200 &&
      metrics.freshCommit.statuses.length === 1 &&
      metrics.freshCommit.statuses[0] === 200 &&
      metrics.receipt.statuses.length === 1 &&
      metrics.receipt.statuses[0] === 200 &&
      metrics.replay.statuses.length === 1 &&
      metrics.replay.statuses[0] === 200 &&
      metrics.oldKeyAuth.statuses.length === 1 &&
      metrics.oldKeyAuth.statuses[0] === 401 &&
      metrics.newKeyAuth.statuses.length === 1 &&
      metrics.newKeyAuth.statuses[0] === 200,
    'STATUS_COUNTS'
  )
  report.result = 'passed'
  report.completedStage = stage
  persist()
} catch (error) {
  report.result = 'failed'
  report.failure = {
    stage,
    message: safeFailure(error, [
      config.access_token,
      config.totp_secret,
      config.synthetic_old_key,
      config.session.sid,
      generatedCode,
      freshSecret,
      capturedProof,
      capturedCommitBody,
    ]),
  }
  if (page) {
    try {
      await page.addStyleTag({ content: inputMask })
      const file = screenshotName('failure')
      await page.screenshot({
        path: path.join(config.output_dir, file),
        animations: 'disabled',
        fullPage: true,
        mask: [page.locator(visibleSensitiveInputSelector)],
        maskColor: '#111827',
      })
      report.screenshots.push(file)
    } catch {}
  }
  persist()
  process.exitCode = 1
  console.error(
    JSON.stringify({
      command: 'credential-rotation-runtime',
      result: 'failed',
      stage,
    })
  )
} finally {
  generatedCode = ''
  freshSecret = ''
  capturedProof = ''
  capturedCommitBody = ''
  if (context) {
    try {
      await context.close()
    } catch {
      report.result = 'failed'
      report.failure = {
        stage: 'close-context',
        message: 'CREDENTIAL_ROTATION_RUNTIME_BROWSER_CLOSE',
      }
      process.exitCode = 1
    }
  }
  if (browser) {
    try {
      await browser.close()
    } catch {
      report.result = 'failed'
      report.failure = {
        stage: 'close-browser',
        message: 'CREDENTIAL_ROTATION_RUNTIME_BROWSER_CLOSE',
      }
      process.exitCode = 1
    }
  }
  persist()
}
