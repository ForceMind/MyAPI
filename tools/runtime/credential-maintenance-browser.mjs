// Production frontend -> real Go credential-maintenance runtime journey.
// Authentication is a signed isolated-test session. Maintenance, verification,
// migration and receipt routes are never fulfilled by the browser fixture.
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
const reportName = 'credential-maintenance-runtime.json'
const inputMask = `
  input, textarea {
    -webkit-text-security: disc !important;
    color: transparent !important;
    caret-color: transparent !important;
  }
`

function fail(code) {
  throw new Error(`CREDENTIAL_MAINTENANCE_RUNTIME_${code}`)
}

function requireThat(condition, code) {
  if (!condition) fail(code)
}

function readConfig() {
  requireThat(process.argv.length === 3, 'CONFIG')
  let value
  try {
    value = JSON.parse(readFileSync(process.argv[2], 'utf8'))
  } catch {
    fail('CONFIG')
  }
  requireThat(value && typeof value === 'object' && !Array.isArray(value), 'CONFIG')
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
  requireThat(['en', 'zh'].includes(value.language), 'CONFIG')
  requireThat([1440, 390].includes(value.width), 'CONFIG')
  requireThat(typeof value.access_token === 'string' && value.access_token.length > 20, 'AUTH')
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
    typeof value.totp_secret === 'string' && /^[A-Z2-7]+=*$/.test(value.totp_secret),
    'AUTH'
  )
  requireThat(Number.isSafeInteger(value.token_id) && value.token_id > 0, 'CONFIG')
  requireThat(
    Number.isSafeInteger(value.target_channel_id) && value.target_channel_id > 0,
    'CONFIG'
  )
  requireThat(
    typeof value.target_channel_name === 'string' && value.target_channel_name.length > 0,
    'CONFIG'
  )
  requireThat(typeof value.version === 'string' && value.version.trim().length > 0, 'BUILD')
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
  const digest = createHmac('sha1', decodeBase32(secret)).update(message).digest()
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
  requireThat(payload && payload.success === true && payload.data, 'API_RESPONSE')
  return payload.data
}

function safeFailure(error, config, generatedCode) {
  let message = typeof error?.message === 'string' ? error.message : 'runtime failure'
  for (const secret of [
    config.access_token,
    config.totp_secret,
    config.session.sid,
    generatedCode,
  ]) {
    if (secret) message = message.split(secret).join('[redacted]')
  }
  message = message
    .replace(/Bearer\s+[A-Za-z0-9._~-]+/gi, 'Bearer [redacted]')
    .replace(/\b\d{6}\b/g, '[redacted-code]')
    .slice(0, 240)
  return message
}

const config = readConfig()
const translations = JSON.parse(
  readFileSync(path.join(repo, 'web', 'src', 'i18n', 'locales', `${config.language}.json`), 'utf8')
).translation
const t = (key) => {
  requireThat(typeof translations[key] === 'string' && translations[key].length > 0, 'TRANSLATION')
  return translations[key]
}
const fixture = createUIFixture({
  role: 100,
  language: config.language === 'zh' ? 'zhCN' : 'en',
  personalPolicy: true,
})
const buildSource = {
  version: config.version,
  sourceHead: config.source_head,
}
const metrics = {
  maintenance: { requests: 0, statuses: [] },
  preview: { requests: 0, statuses: [] },
  verification: { requests: 0, statuses: [] },
  commit: { requests: 0, statuses: [] },
  receipt: { requests: 0, statuses: [] },
  rotationUnsupported: { requests: 0, statuses: [] },
  syntheticConsole: { requests: 0, statuses: [] },
  externalAborted: 0,
  pageErrors: 0,
}
const report = {
  schema: 1,
  result: 'running',
  realMaintenanceAPI: true,
  otherConsoleAPIsSynthetic: true,
  syntheticDatabase: true,
  evidence: {
    maintenance:
      'Maintenance and verification routes are configured to reach real Go. Metrics record which calls actually occurred.',
    console:
      'Only unrelated console bootstrap, list and status reads used createUIFixture.',
  },
  language: config.language,
  width: config.width,
  buildSource,
  metrics,
  checks: {},
  screenshots: [],
}
const reportPath = path.join(config.output_dir, reportName)
const persist = () => writeFileSync(reportPath, JSON.stringify(report, null, 2), { mode: 0o600 })
const screenshotName = (suffix) =>
  `credential-maintenance-runtime-${config.language}-${config.width}-${suffix}.png`

let stage = 'launch'
let browser
let context
let page
let generatedCode = ''

function realKind(pathname, method) {
  const base = `/api/token/${config.token_id}`
  if (pathname === `${base}/maintenance` && method === 'GET') return 'maintenance'
  if (pathname === `${base}/account-migration/preview` && method === 'POST') return 'preview'
  if (pathname === `${base}/account-migration` && method === 'POST') return 'commit'
  if (
    pathname.startsWith(`${base}/maintenance/operations/`) &&
    method === 'GET'
  ) return 'receipt'
  if (pathname === '/api/verify' && method === 'POST') return 'verification'
  if (pathname === `${base}/credential-rotation` && method === 'POST') {
    return 'rotationUnsupported'
  }
  return null
}

async function capture(suffix) {
  stage = `screenshot-${suffix}`
  await page.addStyleTag({ content: inputMask })
  await page.evaluate(async () => {
    await document.fonts.ready
    await Promise.all(document.getAnimations().map((animation) => animation.finished.catch(() => {})))
    await new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done)))
  })
  const file = screenshotName(suffix)
  await page.screenshot({
    path: path.join(config.output_dir, file),
    animations: 'disabled',
    fullPage: true,
  })
  report.screenshots.push(file)
  persist()
}

try {
  persist()
  stage = 'load-playwright'
  const playwrightPath = process.env.MYAPI_PLAYWRIGHT_MODULE
  const module = await import(
    playwrightPath ? pathToFileURL(path.resolve(playwrightPath)).href : 'playwright'
  )
  requireThat(typeof module?.chromium?.launch === 'function', 'BROWSER')
  browser = await module.chromium.launch({
    headless: true,
    args: ['--disable-dev-shm-usage'],
  })
  stage = 'create-context'
  context = await browser.newContext({
    viewport: { width: config.width, height: config.width === 390 ? 844 : 960 },
    locale: config.language === 'zh' ? 'zh-CN' : 'en-US',
    timezoneId: 'Asia/Shanghai',
    reducedMotion: 'reduce',
    serviceWorkers: 'block',
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
    const kind = realKind(pathname, method)
    if (kind) {
      const authorization = request.headers().authorization
      requireThat(authorization === `Bearer ${config.access_token}`, 'REAL_AUTH')
      if (kind === 'commit') {
        const proof = request.headers()['x-security-proof']
        requireThat(typeof proof === 'string' && proof.length > 20, 'REAL_PROOF')
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
              user_agent: 'Synthetic isolated runtime Chromium',
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
      const kind = realKind(pathname, response.request().method())
      if (kind) metrics[kind].statuses.push(response.status())
      else if (pathname.startsWith('/api/') || pathname.startsWith('/pg/')) {
        metrics.syntheticConsole.statuses.push(response.status())
      }
    } catch {}
  })

  stage = 'open-keys'
  await page.goto(`${config.origin}/keys`, { waitUntil: 'domcontentloaded' })
  const visibleVersion = page.getByTestId(config.width < 768 ? 'console-mobile-build-version' : 'console-build-revision')
  await visibleVersion.waitFor({ state: 'visible' })
  const buildRevision = await page.locator('html').getAttribute('data-build-rev')
  requireThat(buildRevision?.startsWith(`rv.${config.version}.${config.source_head}.`), 'BUILD_REVISION')
  requireThat((await visibleVersion.textContent())?.trim() === config.version, 'VISIBLE_VERSION')
  report.checks.exactProductionBuild = true
  const entry = page
    .getByRole('button', { name: t('Credential maintenance'), exact: true })
    .first()
  await entry.waitFor()
  await entry.focus()
  requireThat(await entry.evaluate((element) => document.activeElement === element), 'KEYBOARD')
  const initialMaintenanceResponse = page.waitForResponse(
    (response) =>
      response.request().method() === 'GET' &&
      new URL(response.url()).pathname === `/api/token/${config.token_id}/maintenance`
  )
  stage = 'open-maintenance'
  await page.keyboard.press('Enter')
  const initialResponse = await initialMaintenanceResponse
  requireThat(initialResponse.status() === 200, 'MAINTENANCE_STATUS')
  const initialMaintenance = successData(await initialResponse.json())
  requireThat(
    initialMaintenance.token?.id === config.token_id &&
      initialMaintenance.token?.owner_user_id === config.user.id &&
      initialMaintenance.rotation?.commit_supported === false &&
      initialMaintenance.rotation?.status === 'blocked' &&
      initialMaintenance.migration?.status === 'ready' &&
      initialMaintenance.migration?.source?.channel_id !== config.target_channel_id,
    'MAINTENANCE_CONTRACT'
  )
  const targetCandidate = initialMaintenance.migration.candidates?.find(
    (candidate) => candidate.channel_id === config.target_channel_id
  )
  requireThat(
    targetCandidate?.channel_name === config.target_channel_name &&
      targetCandidate?.status === 'ready' &&
      Array.isArray(targetCandidate.windows) &&
      targetCandidate.windows.length === initialMaintenance.migration.source.windows.length,
    'TARGET_CONTRACT'
  )
  const dialog = page.getByRole('dialog', {
    name: t('Credential maintenance'),
    exact: true,
  })
  await dialog.waitFor()
  await dialog.getByText(t('Credential rotation'), { exact: true }).waitFor()
  const rotationCard = dialog.locator('[data-slot="card"]').filter({
    has: page.getByText(t('Credential rotation'), { exact: true }),
  })
  requireThat((await rotationCard.count()) === 1, 'ROTATION_CARD')
  requireThat((await rotationCard.getByRole('button').count()) === 0, 'ROTATION_ACTION')
  report.checks.rootOwnedKey = true
  report.checks.rotationReadOnly = true
  await capture('loaded')

  stage = 'select-target'
  const target = dialog.getByRole('combobox', { name: t('Target Codex account') })
  await target.click()
  const option = page.getByRole('option').filter({ hasText: config.target_channel_name })
  requireThat((await option.count()) === 1, 'TARGET_SELECTION')
  await option.click()
  requireThat((await target.textContent())?.includes(config.target_channel_name), 'TARGET_SELECTION')
  report.checks.targetSelected = true

  const previewResponsePromise = page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' &&
      new URL(response.url()).pathname ===
        `/api/token/${config.token_id}/account-migration/preview`
  )
  stage = 'preview'
  const previewButton = dialog.getByRole('button', { name: t('Preview migration') })
  await previewButton.focus()
  await page.keyboard.press('Enter')
  const previewResponse = await previewResponsePromise
  requireThat(previewResponse.status() === 200, 'PREVIEW_STATUS')
  const preview = successData(await previewResponse.json())
  requireThat(
    preview.status === 'ready' &&
      /^[0-9a-f]{64}$/.test(preview.preview_revision || '') &&
      preview.changes?.policy?.target_channel_id === config.target_channel_id &&
      preview.changes?.allocations?.length === initialMaintenance.migration.source.windows.length,
    'PREVIEW_CONTRACT'
  )
  for (const source of initialMaintenance.migration.source.windows) {
    const allocation = preview.changes.allocations.find(
      (item) => item.source_pool_id === source.pool_id
    )
    requireThat(
      allocation?.share_bps === source.share_bps &&
        targetCandidate.windows.some(
          (window) =>
            window.pool_id === allocation.target_pool_id &&
            window.window_slot === source.window_slot
        ),
      'SHARE_CONTRACT'
    )
  }
  await dialog.getByText(t('Confirm migration'), { exact: true }).waitFor()
  report.checks.previewReady = true

  stage = 'confirm'
  const confirmations = dialog.getByRole('checkbox')
  requireThat((await confirmations.count()) === 2, 'CONFIRMATIONS')
  for (let index = 0; index < 2; index += 1) {
    await confirmations.nth(index).focus()
    await page.keyboard.press('Space')
    requireThat(await confirmations.nth(index).isChecked(), 'CONFIRMATIONS')
  }
  report.checks.twoConfirmations = true
  const verifyAndMove = dialog.getByRole('button', {
    name: t('Verify and move account'),
  })
  await verifyAndMove.scrollIntoViewIfNeeded()
  const actionVisible = await verifyAndMove.evaluate((element) => {
    const box = element.getBoundingClientRect()
    const hit = document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2)
    const nav = document.querySelector('nav.pc-mobile-nav')?.getBoundingClientRect()
    return box.top >= 0 && box.bottom <= innerHeight &&
      (!nav || nav.width === 0 || box.bottom <= nav.top) &&
      Boolean(hit && element.contains(hit))
  })
  requireThat(actionVisible, 'CONFIRM_ACTION_VISIBLE')
  report.checks.confirmActionVisible = true
  await capture('confirmed')

  stage = 'open-verification'
  await verifyAndMove.focus()
  await page.keyboard.press('Enter')
  const verificationDialog = page.getByRole('dialog', {
    name: t('Verify account migration'),
  })
  const codeInput = verificationDialog.getByPlaceholder(t('Enter verification code'))
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
        `/api/token/${config.token_id}/account-migration`
  )
  stage = 'verify-and-commit'
  await page.keyboard.press('Enter')
  const proofResponse = await proofResponsePromise
  requireThat(proofResponse.status() === 200, 'VERIFY_STATUS')
  const proof = successData(await proofResponse.json())
  requireThat(
    proof.method === '2fa' &&
      proof.scope === 'token.account_migration' &&
      typeof proof.proof_token === 'string' &&
      proof.proof_token.length > 20,
    'VERIFY_CONTRACT'
  )
  const commitResponse = await commitResponsePromise
  requireThat(commitResponse.status() === 200, 'COMMIT_STATUS')
  const receipt = successData(await commitResponse.json())
  requireThat(
    receipt.state === 'committed' &&
      receipt.token_id === config.token_id &&
      receipt.target_channel_id === config.target_channel_id &&
      /^[0-9a-f]{64}$/.test(receipt.operation_id || '') &&
      receipt.replayed === false,
    'COMMIT_CONTRACT'
  )
  requireThat(metrics.commit.requests === 1, 'DUPLICATE_COMMIT')
  await dialog.getByText(t('Account migration committed'), { exact: true }).waitFor()
  await dialog.getByText(t('Account migration committed'), { exact: true }).scrollIntoViewIfNeeded()
  report.checks.realVerification = true
  report.checks.commitOnce = true
  await capture('committed')

  stage = 'receipt-readback'
  const receiptReadback = await page.evaluate(
    async ({ url, accessToken }) => {
      const response = await fetch(url, {
        method: 'GET',
        headers: { Authorization: `Bearer ${accessToken}` },
        cache: 'no-store',
      })
      const body = await response.json().catch(() => null)
      return { status: response.status, body }
    },
    {
      url: `${config.origin}/api/token/${config.token_id}/maintenance/operations/${receipt.operation_id}`,
      accessToken: config.access_token,
    }
  )
  requireThat(receiptReadback.status === 200, 'RECEIPT_STATUS')
  const readReceipt = successData(receiptReadback.body)
  requireThat(
    readReceipt.state === 'committed' &&
      readReceipt.token_id === config.token_id &&
      readReceipt.operation_id === receipt.operation_id &&
      readReceipt.target_channel_id === config.target_channel_id &&
      readReceipt.replayed === false,
    'RECEIPT_CONTRACT'
  )
  report.checks.realReceipt = true

  stage = 'rotation-unsupported'
  const rotationStatus = await page.evaluate(
    async ({ url, accessToken }) => {
      const response = await fetch(url, {
        method: 'POST',
        headers: { Authorization: `Bearer ${accessToken}` },
        cache: 'no-store',
      })
      return response.status
    },
    {
      url: `${config.origin}/api/token/${config.token_id}/credential-rotation`,
      accessToken: config.access_token,
    }
  )
  requireThat(rotationStatus === 404, 'ROTATION_STATUS')
  report.checks.rotationPostMissing = true

  stage = 'reopen'
  await dialog.getByRole('button', { name: t('Done'), exact: true }).click()
  await dialog.waitFor({ state: 'hidden' })
  const reopenedResponsePromise = page.waitForResponse(
    (response) =>
      response.request().method() === 'GET' &&
      new URL(response.url()).pathname === `/api/token/${config.token_id}/maintenance`
  )
  await entry.click()
  const reopenedResponse = await reopenedResponsePromise
  requireThat(reopenedResponse.status() === 200, 'REOPEN_STATUS')
  const reopened = successData(await reopenedResponse.json())
  requireThat(
    reopened.token?.id === config.token_id &&
      reopened.token?.owner_user_id === config.user.id &&
      reopened.migration?.source?.channel_id === config.target_channel_id,
    'REOPEN_CONTRACT'
  )
  await dialog.waitFor()
  await dialog
    .getByText(config.target_channel_name, { exact: true })
    .first()
    .waitFor()
  report.checks.reopenedAtTarget = true
  await capture('reopened')

  stage = 'final-checks'
  requireThat(metrics.maintenance.requests === 2, 'MAINTENANCE_COUNT')
  requireThat(metrics.preview.requests === 1, 'PREVIEW_COUNT')
  requireThat(metrics.verification.requests === 1, 'VERIFY_COUNT')
  requireThat(metrics.commit.requests === 1, 'COMMIT_COUNT')
  requireThat(metrics.receipt.requests === 1, 'RECEIPT_COUNT')
  requireThat(metrics.rotationUnsupported.requests === 1, 'ROTATION_COUNT')
  requireThat(metrics.externalAborted === 0, 'NETWORK_GUARD')
  requireThat(metrics.pageErrors === 0, 'PAGE_ERROR')
  requireThat(
    metrics.maintenance.statuses.length === 2 &&
      metrics.maintenance.statuses.every((status) => status === 200) &&
      metrics.preview.statuses.length === 1 &&
      metrics.preview.statuses[0] === 200 &&
      metrics.verification.statuses.length === 1 &&
      metrics.verification.statuses[0] === 200 &&
      metrics.commit.statuses.length === 1 &&
      metrics.commit.statuses[0] === 200 &&
      metrics.receipt.statuses.length === 1 &&
      metrics.receipt.statuses[0] === 200 &&
      metrics.rotationUnsupported.statuses.length === 1 &&
      metrics.rotationUnsupported.statuses[0] === 404,
    'STATUS_COUNTS'
  )
  report.result = 'passed'
  report.evidence.maintenance =
    'The four credential-maintenance routes and /api/verify reached the real Go router and passed their asserted contracts.'
  report.completedStage = stage
  persist()
} catch (error) {
  report.result = 'failed'
  report.failure = {
    stage,
    message: safeFailure(error, config, generatedCode),
  }
  if (page) {
    try {
      await page.addStyleTag({ content: inputMask })
      const file = screenshotName('failure')
      await page.screenshot({
        path: path.join(config.output_dir, file),
        animations: 'disabled',
        fullPage: true,
      })
      report.screenshots.push(file)
    } catch {}
  }
  persist()
  process.exitCode = 1
  console.error(JSON.stringify({
    command: 'credential-maintenance-runtime',
    result: 'failed',
    stage,
  }))
} finally {
  generatedCode = ''
  if (context) {
    try {
      await context.close()
    } catch {
      report.result = 'failed'
      report.failure = {
        stage: 'close-context',
        message: 'CREDENTIAL_MAINTENANCE_RUNTIME_BROWSER_CLOSE',
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
        message: 'CREDENTIAL_MAINTENANCE_RUNTIME_BROWSER_CLOSE',
      }
      process.exitCode = 1
    }
  }
  persist()
}
