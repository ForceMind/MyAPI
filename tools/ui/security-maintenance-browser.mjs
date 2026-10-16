// Production-build Chromium qualification for profile security maintenance.
// Every API response and mutation is synthetic and isolated from real accounts.
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import {
  createReadStream,
  existsSync,
  mkdirSync,
  readFileSync,
  statSync,
  writeFileSync,
} from 'node:fs'
import { createServer } from 'node:http'
import { extname, resolve, sep } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

import { createUIFixture, FIXTURE_TIME } from './browser-fixtures.mjs'

const repo = fileURLToPath(new URL('../../', import.meta.url))
const build = resolve(repo, 'web/dist')
const output = resolve(
  process.env.MYAPI_BROWSER_ARTIFACTS ||
    `${repo}/.local-tests/advanced-security`
)
const version = readFileSync(resolve(repo, 'VERSION'), 'utf8').trim()
const head = execFileSync('git', ['rev-parse', 'HEAD'], {
  cwd: repo,
  encoding: 'utf8',
}).trim()
const languageCodes = {
  en: 'en',
  zh: 'zhCN',
  'zh-TW': 'zhTW',
  fr: 'fr',
  ja: 'ja',
  ru: 'ru',
  vi: 'vi',
}
const translations = Object.fromEntries(
  Object.keys(languageCodes).map((language) => [
    language,
    JSON.parse(
      readFileSync(
        resolve(repo, `web/src/i18n/locales/${language}.json`),
        'utf8'
      )
    ).translation,
  ])
)
const tr = (key, language) => {
  const value = translations[language]?.[key]
  assert(
    typeof value === 'string' && value.trim(),
    `Missing ${language} translation: ${key}`
  )
  return value
}

const requiredKeys = [
  'Password and account access',
  'Review each capability before changing access to this account.',
  'Regenerate the console access token only when you intend to replace it',
  'Root accounts cannot be deleted',
]
for (const key of requiredKeys) {
  for (const language of Object.keys(languageCodes)) tr(key, language)
}

assert(existsSync(resolve(build, 'index.html')), 'Build frontend first')
mkdirSync(output, { recursive: true })

const report = {
  schema: 1,
  evidence:
    'Production-build Chromium with isolated synthetic security APIs and an optional Chromium virtual authenticator.',
  scope:
    'Profile password, bindings presentation, Passkey, 2FA, backup codes, login sessions, Root deletion restriction, and one-time console Access Token presentation. Excludes production, live accounts, real credentials, real hardware authenticators, credential migration, and rotation policy changes.',
  mutationPolicy:
    'Only the explicit synthetic actions listed per journey are accepted; every other mutation is a qualification failure.',
  webauthnBoundary:
    'Chromium CDP virtual authenticator only. It exercises navigator.credentials and browser WebAuthn serialization but is not real hardware or biometric evidence.',
  version,
  head,
  workflowHead: process.env.GITHUB_SHA || null,
  fixtureTime: new Date(FIXTURE_TIME).toISOString(),
  result: 'running',
  journeys: [],
  screenshots: [],
  errors: [],
}
const persist = () =>
  writeFileSync(
    resolve(output, 'advanced-security-qualification.json'),
    JSON.stringify(report, null, 2)
  )

const mime = {
  '.html': 'text/html',
  '.js': 'text/javascript',
  '.css': 'text/css',
  '.json': 'application/json',
  '.png': 'image/png',
  '.svg': 'image/svg+xml',
  '.ico': 'image/x-icon',
  '.woff2': 'font/woff2',
}
const server = createServer((request, response) => {
  let file
  try {
    file = resolve(
      build,
      `.${decodeURIComponent(
        new URL(request.url || '/', 'http://localhost').pathname
      )}`
    )
  } catch {
    response.writeHead(400).end()
    return
  }
  if (!file.startsWith(`${build}${sep}`) && file !== build) {
    response.writeHead(403).end()
    return
  }
  if (!existsSync(file) || statSync(file).isDirectory()) {
    file = resolve(build, 'index.html')
  }
  response.writeHead(200, {
    'content-type': mime[extname(file)] || 'application/octet-stream',
    'cache-control': 'no-store',
  })
  createReadStream(file).pipe(response)
})

const ok = (data) => ({ status: 200, body: { success: true, data } })
const syntheticAccessToken =
  'synthetic-console-access-token-never-a-real-credential'
const syntheticTwoFASecret = 'SYNTHETICNOTAREALSECRET'
const syntheticBackupCodes = [
  'SYNTH-001',
  'SYNTH-002',
  'SYNTH-003',
  'SYNTH-004',
]

function securityState(fixture, scenario, origin) {
  const refresh = fixture.response(
    new URL('/api/user/auth/refresh', origin),
    'POST'
  )
  assert.equal(refresh.status, 200)
  const auth = refresh.body.data
  assert.equal(auth.user.role, 100, 'Root fixture role is preserved')
  assert.equal(
    auth.session.sid,
    'synthetic-ui-100',
    'createUIFixture session SID is preserved'
  )
  return {
    auth,
    rpID: new URL(origin).hostname,
    stage: 'initialized',
    twoFAEnabled: false,
    backupCodesRemaining: 0,
    passkeyEnabled: false,
    sessions: [
      auth.session,
      {
        ...auth.session,
        sid: 'synthetic-other-session',
        current: false,
        login_method: 'password',
        ip: '192.0.2.20',
        user_agent: 'Synthetic Firefox on Linux',
        created_at: auth.session.created_at - 3600,
        last_active_at: auth.session.last_active_at - 300,
      },
    ],
    // Keep the initial read phase unavailable until every status card has
    // rendered its own error. Several security components probe the same
    // endpoints concurrently, so a fixed request budget is not deterministic.
    failSecurityReads: scenario.failSecurityReads === true,
    counts: {
      accessToken: 0,
      password: 0,
      passkeyBegin: 0,
      passkeyFinish: 0,
      twoFASetup: 0,
      twoFAEnable: 0,
      backupCodes: 0,
      sessionRevokeOthers: 0,
    },
    mutations: [],
    violations: [],
    pageErrors: [],
    external: [],
    rotatedAuthorizationSeen: false,
    passkeyRotationSeen: false,
    twoFARotationSeen: false,
    backupCodeRotationSeen: false,
  }
}

function authRotation(state, accessToken) {
  return {
    access_token: accessToken,
    token_type: state.auth.token_type,
    access_expires_at: state.auth.access_expires_at,
    session: {
      ...state.auth.session,
      last_active_at: state.auth.session.last_active_at + 1,
    },
  }
}

async function responseFor(request, fixture, state) {
  const url = new URL(request.url())
  const path = url.pathname.replace(/\/$/, '')
  const method = request.method()
  const authorization = request.headers().authorization
  if (authorization === 'Bearer synthetic-password-rotation') {
    state.rotatedAuthorizationSeen = true
  } else if (authorization === 'Bearer synthetic-passkey-rotation') {
    state.passkeyRotationSeen = true
  } else if (authorization === 'Bearer synthetic-two-fa-rotation') {
    state.twoFARotationSeen = true
  } else if (authorization === 'Bearer synthetic-backup-code-rotation') {
    state.backupCodeRotationSeen = true
  }

  if (method === 'GET' && path === '/api/user/token') {
    state.counts.accessToken += 1
    return ok(syntheticAccessToken)
  }
  if (method === 'GET' && path === '/api/user/2fa/status') {
    if (state.failSecurityReads) {
      return {
        status: 503,
        body: { success: false, message: 'Synthetic 2FA status unavailable' },
      }
    }
    return ok({
      enabled: state.twoFAEnabled,
      locked: false,
      backup_codes_remaining: state.backupCodesRemaining,
    })
  }
  if (method === 'GET' && path === '/api/user/passkey') {
    if (state.failSecurityReads) {
      return {
        status: 503,
        body: { success: false, message: 'Synthetic Passkey status unavailable' },
      }
    }
    if (state.passkeyEnabled && state.stage.startsWith('passkey')) {
      state.stage = 'passkey-status-enabled'
    }
    return ok({
      enabled: state.passkeyEnabled,
      last_used_at: state.passkeyEnabled
        ? new Date(FIXTURE_TIME).toISOString()
        : null,
      backup_eligible: true,
      backup_state: state.passkeyEnabled,
    })
  }
  if (method === 'GET' && path === '/api/user/sessions') {
    if (state.failSecurityReads) {
      return {
        status: 503,
        body: { success: false, message: 'Synthetic sessions unavailable' },
      }
    }
    return ok(state.sessions)
  }

  if (method === 'PUT' && path === '/api/user/self') {
    const body = request.postDataJSON()
    if (
      typeof body?.original_password !== 'string' ||
      typeof body?.password !== 'string'
    ) {
      return { violation: 'Password mutation omitted required fields' }
    }
    state.counts.password += 1
    state.mutations.push('PUT /api/user/self password')
    return ok(authRotation(state, 'synthetic-password-rotation'))
  }
  if (method === 'POST' && path === '/api/user/passkey/register/begin') {
    state.counts.passkeyBegin += 1
    state.stage = 'passkey-credential-create'
    state.mutations.push('POST /api/user/passkey/register/begin')
    return ok({
      flow_token: 'synthetic-passkey-flow',
      options: {
        publicKey: {
          challenge: 'AQIDBAUGBwgJCgsMDQ4PEA',
          rp: { id: state.rpID, name: 'MyAPI synthetic fixture' },
          user: {
            id: 'c3ludGhldGljLXVzZXItMQ',
            name: state.auth.user.username,
            displayName: state.auth.user.display_name,
          },
          pubKeyCredParams: [
            { type: 'public-key', alg: -7 },
            { type: 'public-key', alg: -257 },
          ],
          timeout: 30_000,
          attestation: 'none',
          authenticatorSelection: {
            residentKey: 'preferred',
            userVerification: 'required',
          },
          excludeCredentials: [],
        },
      },
    })
  }
  if (method === 'POST' && path === '/api/user/passkey/register/finish') {
    state.stage = 'passkey-finish'
    const body = request.postDataJSON()
    const credential = body?.credential
    if (
      body?.flow_token !== 'synthetic-passkey-flow' ||
      typeof credential?.id !== 'string' ||
      !credential.id ||
      typeof credential.rawId !== 'string' ||
      !credential.rawId ||
      credential.type !== 'public-key' ||
      typeof credential.response?.attestationObject !== 'string' ||
      !credential.response.attestationObject ||
      typeof credential.response?.clientDataJSON !== 'string' ||
      !credential.response.clientDataJSON
    ) {
      return { violation: 'Virtual WebAuthn credential was not serialized' }
    }
    state.counts.passkeyFinish += 1
    state.passkeyEnabled = true
    state.mutations.push('POST /api/user/passkey/register/finish')
    return ok(authRotation(state, 'synthetic-passkey-rotation'))
  }
  if (method === 'POST' && path === '/api/user/2fa/setup') {
    state.counts.twoFASetup += 1
    state.stage = 'two-fa-setup-response'
    state.mutations.push('POST /api/user/2fa/setup')
    return ok({
      secret: syntheticTwoFASecret,
      qr_code_data: `otpauth://totp/MyAPI:synthetic?secret=${syntheticTwoFASecret}&issuer=MyAPI`,
      backup_codes: syntheticBackupCodes,
    })
  }
  if (method === 'POST' && path === '/api/user/2fa/enable') {
    assert.equal(request.postDataJSON()?.code, '123456')
    state.counts.twoFAEnable += 1
    state.stage = 'two-fa-enable-response'
    state.twoFAEnabled = true
    state.backupCodesRemaining = syntheticBackupCodes.length
    state.mutations.push('POST /api/user/2fa/enable')
    return ok(authRotation(state, 'synthetic-two-fa-rotation'))
  }
  if (method === 'POST' && path === '/api/user/2fa/backup_codes') {
    assert.equal(request.postDataJSON()?.code, '123456')
    state.counts.backupCodes += 1
    state.stage = 'two-fa-backup-response'
    state.mutations.push('POST /api/user/2fa/backup_codes')
    return ok({
      ...authRotation(state, 'synthetic-backup-code-rotation'),
      backup_codes: syntheticBackupCodes.map((code) => `${code}-NEW`),
    })
  }
  if (method === 'POST' && path === '/api/user/sessions/revoke-others') {
    state.counts.sessionRevokeOthers += 1
    state.stage = 'sessions-revoke-response'
    state.sessions = state.sessions.filter((session) => session.current)
    state.mutations.push('POST /api/user/sessions/revoke-others')
    return ok(null)
  }

  const response = fixture.response(url, method)
  return response
}

async function settled(page) {
  await page.evaluate(async () => {
    await document.fonts.ready
    await Promise.all(
      document
        .getAnimations()
        .filter(
          (animation) =>
            animation.effect?.getComputedTiming().iterations !== Infinity
        )
        .map((animation) => animation.finished.catch(() => {}))
    )
    await new Promise((done) =>
      requestAnimationFrame(() => requestAnimationFrame(done))
    )
  })
}

function exactText(value) {
  return new RegExp(`^${value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}$`)
}

function securityCard(page, title) {
  const panel = page.locator('#personal-security-settings')
  const cardTitle = page
    .locator('[data-slot="card-title"]')
    .filter({ hasText: exactText(title) })
  return panel.locator('[data-slot="card"]').filter({ has: cardTitle })
}

async function assertGeometry(page, scenario, label) {
  assert.equal(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth + 1
    ),
    true,
    `${label}: no page-wide horizontal overflow`
  )
  const dialog = page.locator('[data-slot="dialog-content"]:visible')
  if (await dialog.count()) {
    assert.equal(
      await dialog.last().evaluate((element) => {
        const bounds = element.getBoundingClientRect()
        return (
          element.scrollWidth <= element.clientWidth + 1 &&
          bounds.left >= -1 &&
          bounds.right <= window.innerWidth + 1
        )
      }),
      true,
      `${label}: dialog fits the viewport`
    )
  }
  if (scenario.width < 768) {
    // A modal marks the application shell aria-hidden. Geometry still needs to
    // cover the fixed navigation underneath it, so use its stable DOM label.
    const navigation = page.locator(
      `[aria-label=${JSON.stringify(tr('Mobile console navigation', scenario.language))}]`
    )
    assert.equal(await navigation.count(), 1)
    const bounds = await navigation.boundingBox()
    assert(
      bounds && bounds.x >= 0 && bounds.x + bounds.width <= scenario.width + 1,
      `${label}: mobile navigation remains inside the viewport`
    )
  }
}

async function screenshot(page, scenario, stateName) {
  await settled(page)
  await assertGeometry(page, scenario, stateName)
  const file = `advanced-security-${scenario.name}-${stateName}.png`
  await page.screenshot({
    path: resolve(output, file),
    animations: 'disabled',
    fullPage: true,
  })
  report.screenshots.push({
    file,
    language: scenario.language,
    width: scenario.width,
    theme: scenario.theme,
    state: stateName,
  })
  persist()
}

async function openSecurity(page, scenario) {
  const t = (key) => tr(key, scenario.language)
  await page.goto(`${origin}/profile`, { waitUntil: 'networkidle' })
  await page.getByRole('heading', { name: t('Identity'), exact: true }).waitFor()
  const revision = await page.locator('html').getAttribute('data-build-rev')
  assert(
    revision?.startsWith(
      `rv.${version}.${process.env.GITHUB_SHA || head}.`
    ),
    'Visible build revision matches the qualified source'
  )
  const trigger = page.getByRole('button', {
    name: new RegExp(t('Manage login methods')),
  })
  await trigger.focus()
  await page.keyboard.press('Enter')
  assert.equal(await trigger.getAttribute('aria-expanded'), 'true')
  await page
    .locator('#personal-security-settings')
    .waitFor({ state: 'visible' })
  const cards = [
    securityCard(page, t('Password and account access')),
    securityCard(page, t('Passkey Login')),
    securityCard(page, t('Two-Factor Authentication')),
    securityCard(page, t('Login sessions')),
  ]
  for (const card of cards) {
    await card.waitFor({ state: 'visible' })
    assert.equal(await card.count(), 1)
  }
  // The application scrolls its main content inside a viewport-height shell.
  // Put the expanded security region in that real viewport before evidence.
  await cards[0].scrollIntoViewIfNeeded()
  const securityBounds = await cards[0].boundingBox()
  assert(
    securityBounds &&
      securityBounds.y < scenario.height &&
      securityBounds.y + securityBounds.height > 0,
    'Expanded security controls are visible in the screenshot viewport'
  )
  return revision
}

async function verifyRootRestriction(page, scenario, state) {
  const t = (key) => tr(key, scenario.language)
  const deleteAction = page
    .getByRole('button')
    .filter({ has: page.getByText(t('Delete Account'), { exact: true }) })
  assert.equal(await deleteAction.isDisabled(), true)
  await page.getByText(t('Root accounts cannot be deleted'), { exact: true }).waitFor()
  assert.equal(
    await page.getByText(t('Current Balance'), { exact: true }).count(),
    0,
    'Personal profile hides balance'
  )
  assert.equal(
    await page.getByText(t('Recharge'), { exact: true }).count(),
    0,
    'Personal profile hides recharge'
  )
  assert.equal(
    await page
      .getByRole('button', { name: t('API Keys'), exact: true })
      .count(),
    0,
    'The console credential is named Access Token, not API Keys'
  )
  assert.equal(state.counts.accessToken, 0, 'Opening security never generates an Access Token')
}

async function verifyMobilePasswordFooter(page, scenario) {
  const t = (key) => tr(key, scenario.language)
  const action = securityCard(page, t('Password and account access'))
    .getByRole('button')
    .filter({ has: page.getByText(t('Change Password'), { exact: true }) })
  await action.click()
  const dialog = page.getByRole('dialog', { name: t('Change Password') })
  const cancel = dialog.getByRole('button', { name: t('Cancel'), exact: true })
  const submit = dialog.getByRole('button', {
    name: t('Change Password'),
    exact: true,
  })
  for (const button of [cancel, submit]) {
    const bounds = await button.boundingBox()
    assert(
      bounds &&
        bounds.x >= 0 &&
        bounds.x + bounds.width <= scenario.width + 1 &&
        bounds.y >= 0 &&
        bounds.y + bounds.height <= scenario.height + 1,
      'Mobile security dialog footer action remains usable in the viewport'
    )
  }
  if (scenario.theme === 'dark') {
    const colors = await dialog.evaluate((element) => {
      const parse = (value) => {
        const channels = value.match(/[\d.]+/g)?.slice(0, 3).map(Number) || []
        return channels.length === 3
          ? channels.reduce((sum, channel) => sum + channel, 0) / 3
          : Number.NaN
      }
      const style = getComputedStyle(element)
      return {
        backgroundMean: parse(style.backgroundColor),
        foregroundMean: parse(style.color),
      }
    })
    assert(
      Number.isFinite(colors.backgroundMean) && colors.backgroundMean < 100,
      'Dark password dialog has a dark computed background'
    )
    assert(
      Number.isFinite(colors.foregroundMean) && colors.foregroundMean > 180,
      'Dark password dialog has readable computed foreground text'
    )
  }
  await screenshot(page, scenario, 'password-dialog')
  await cancel.click()
  await dialog.waitFor({ state: 'detached' })
}

async function verifyPassword(page, scenario, state) {
  const t = (key) => tr(key, scenario.language)
  const action = securityCard(page, t('Password and account access'))
    .getByRole('button')
    .filter({ has: page.getByText(t('Change Password'), { exact: true }) })
  await action.click()
  let dialog = page.getByRole('dialog', { name: t('Change Password') })
  await dialog.getByLabel(t('Current Password'), { exact: true }).fill('old-password')
  await dialog.getByLabel(t('New Password'), { exact: true }).fill('new-password')
  await dialog.getByLabel(t('Confirm New Password'), { exact: true }).fill('mismatch-password')
  await dialog.getByRole('button', { name: t('Change Password') }).click()
  await page.getByText(t('Passwords do not match'), { exact: true }).waitFor()
  assert.equal(state.counts.password, 0, 'Client validation blocks the password request')
  await dialog.getByRole('button', { name: t('Cancel'), exact: true }).click()
  await dialog.waitFor({ state: 'detached' })

  await action.click()
  dialog = page.getByRole('dialog', { name: t('Change Password') })
  for (const key of ['Current Password', 'New Password', 'Confirm New Password']) {
    assert.equal(await dialog.getByLabel(t(key), { exact: true }).inputValue(), '')
  }
  await dialog.getByLabel(t('Current Password'), { exact: true }).fill('old-password')
  await dialog.getByLabel(t('New Password'), { exact: true }).fill('new-password')
  await dialog.getByLabel(t('Confirm New Password'), { exact: true }).fill('new-password')
  await dialog.getByLabel(t('Confirm New Password'), { exact: true }).press('Enter')
  await dialog.waitFor({ state: 'detached' })
  assert.equal(state.counts.password, 1)
}

async function verifyAccessToken(page, scenario, state) {
  const t = (key) => tr(key, scenario.language)
  const action = page
    .getByRole('button')
    .filter({ has: page.getByText(t('Access Token'), { exact: true }) })
  await action.click()
  let dialog = page.getByRole('dialog', { name: t('Access Token') })
  await dialog
    .getByText(t('Access tokens are shown only once'), { exact: true })
    .waitFor()
  assert.equal(state.counts.accessToken, 0)
  await dialog.getByRole('button', { name: t('Regenerate'), exact: true }).click()
  const confirmation = page.getByRole('alertdialog', {
    name: t('Regenerate access token?'),
  })
  await confirmation.waitFor()
  assert.equal(state.counts.accessToken, 0, 'The first confirmation does not mutate')
  await confirmation
    .getByRole('button', { name: t('Regenerate token'), exact: true })
    .click()
  const tokenInput = dialog.getByLabel(t('Token'), { exact: true })
  await tokenInput.waitFor()
  assert.equal(await tokenInput.inputValue(), syntheticAccessToken)
  assert.equal(state.counts.accessToken, 1)
  await tokenInput.evaluate((element) => {
    element.type = 'password'
  })
  await screenshot(page, scenario, 'access-token-redacted')
  const footerClose = dialog
    .locator('button:not([data-slot="dialog-close"])')
    .filter({ has: page.getByText(t('Close'), { exact: true }) })
  await footerClose.click()
  await dialog.waitFor({ state: 'detached' })
  await action.click()
  dialog = page.getByRole('dialog', { name: t('Access Token') })
  assert.equal(
    await dialog.getByLabel(t('Token'), { exact: true }).count(),
    0
  )
  assert.equal(state.counts.accessToken, 1, 'Reopening never regenerates')
  await dialog
    .locator('button:not([data-slot="dialog-close"])')
    .filter({ has: page.getByText(t('Close'), { exact: true }) })
    .click()
  await dialog.waitFor({ state: 'detached' })
}

async function verifyPasskey(page, scenario, state) {
  const t = (key) => tr(key, scenario.language)
  state.stage = 'passkey-start'
  const card = securityCard(page, t('Passkey Login'))
  const enable = card.getByRole('button', {
    name: t('Enable Passkey'),
    exact: true,
  })
  await enable.waitFor()
  assert.equal(await enable.isEnabled(), true, 'Virtual authenticator is detected')
  state.stage = 'passkey-register'
  await enable.click()
  const remove = card.getByRole('button', {
    name: t('Remove Passkey'),
    exact: true,
  })
  await remove.waitFor()
  const removeBounds = await remove.boundingBox()
  assert(
    removeBounds && removeBounds.height >= 44,
    'Remove Passkey keeps a full-height touch target'
  )
  assert.equal(state.counts.passkeyBegin, 1)
  assert.equal(state.counts.passkeyFinish, 1)
  assert.equal(
    state.passkeyRotationSeen,
    true,
    'The Passkey status read uses the rotated bearer token'
  )
  state.stage = 'passkey-complete'
  return removeBounds.height
}

async function verifyTwoFA(page, scenario, state) {
  const t = (key) => tr(key, scenario.language)
  state.stage = 'two-fa-setup'
  const card = securityCard(page, t('Two-Factor Authentication'))
  await card.getByRole('button', { name: t('Enable'), exact: true }).click()
  const setup = page.getByRole('dialog', {
    name: t('Setup Two-Factor Authentication'),
  })
  await setup.getByRole('button', { name: t('Next'), exact: true }).waitFor()
  await setup.getByRole('button', { name: t('Next'), exact: true }).click()
  await setup.getByRole('button', { name: t('Next'), exact: true }).click()
  await setup
    .getByLabel(t('Verification Code'), { exact: true })
    .fill('123456')
  await setup.getByRole('button', { name: t('Enable 2FA'), exact: true }).click()
  await setup.waitFor({ state: 'detached' })
  await card
    .getByRole('button', {
      name: t('Regenerate Backup Codes'),
      exact: true,
    })
    .waitFor()
  assert.equal(state.counts.twoFASetup, 1)
  assert.equal(state.counts.twoFAEnable, 1)
  assert.equal(
    state.twoFARotationSeen,
    true,
    'The 2FA status read uses the rotated bearer token'
  )

  state.stage = 'two-fa-backup-codes'
  await card
    .getByRole('button', {
      name: t('Regenerate Backup Codes'),
      exact: true,
    })
    .click()
  const backupDialog = page.getByRole('dialog', {
    name: t('Regenerate Backup Codes'),
  })
  await backupDialog
    .getByLabel(t('Verification Code'), { exact: true })
    .fill('123456')
  await backupDialog
    .getByRole('button', { name: t('Generate New Codes'), exact: true })
    .click()
  await backupDialog.getByRole('button', { name: t('Done'), exact: true }).waitFor()
  assert.equal(state.counts.backupCodes, 1)
  await backupDialog.getByRole('button', { name: t('Done'), exact: true }).click()
  await backupDialog.waitFor({ state: 'detached' })
  for (const code of syntheticBackupCodes) {
    assert.equal(await page.getByText(`${code}-NEW`, { exact: true }).count(), 0)
  }
  state.stage = 'two-fa-complete'
}

async function verifySessions(page, scenario, state) {
  const t = (key) => tr(key, scenario.language)
  state.stage = 'sessions-revoke-others'
  const card = securityCard(page, t('Login sessions'))
  const action = card.getByRole('button', {
    name: t('Sign out other sessions'),
    exact: true,
  })
  await action.click()
  const confirmation = page.getByRole('alertdialog', {
    name: t('Sign out other sessions?'),
  })
  await confirmation
    .getByRole('button', { name: t('Sign out others'), exact: true })
    .click()
  await confirmation.waitFor({ state: 'detached' })
  assert.equal(state.counts.sessionRevokeOthers, 1)
  await page.waitForFunction(
    (label) => {
      const button = [...document.querySelectorAll('button')].find(
        (element) => element.textContent?.trim() === label
      )
      return button?.hasAttribute('disabled')
    },
    t('Sign out other sessions')
  )
  assert.equal(
    state.backupCodeRotationSeen,
    true,
    'The session request uses the bearer token rotated with backup codes'
  )
  state.stage = 'sessions-complete'
}

async function verifyAndRecoverSecurityReads(page, scenario, state) {
  const t = (key) => tr(key, scenario.language)
  const cards = [
    [t('Passkey Login'), t('Failed to load Passkey status'), 'passkey-failed'],
    [
      t('Two-Factor Authentication'),
      t('Unable to load settings'),
      'two-fa-failed',
    ],
    [t('Login sessions'), t('Unable to load login sessions'), 'sessions-failed'],
  ]
  for (const [heading, errorTitle] of cards) {
    const card = securityCard(page, heading)
    await card.getByText(errorTitle, { exact: true }).waitFor()
  }
  await screenshot(page, scenario, 'failed-statuses')
  for (const [heading, , suffix] of cards) {
    const file = `advanced-security-${scenario.name}-${suffix}.png`
    await securityCard(page, heading).screenshot({
      path: resolve(output, file),
      animations: 'disabled',
    })
    report.screenshots.push({
      file,
      language: scenario.language,
      width: scenario.width,
      theme: scenario.theme,
      state: `error-retry-${suffix}`,
    })
  }
  persist()
  state.failSecurityReads = false
  for (const [heading] of cards) {
    const card = securityCard(page, heading)
    await card.getByRole('button', { name: t('Retry'), exact: true }).click()
  }
  const passkeyCard = securityCard(page, t('Passkey Login'))
  await passkeyCard
    .locator('p.font-medium')
    .filter({ hasText: exactText(t('Passkey Authentication')) })
    .waitFor()
  const twoFACard = securityCard(page, t('Two-Factor Authentication'))
  await twoFACard
    .locator('p.font-medium')
    .filter({ hasText: exactText(t('Two-Step Verification')) })
    .waitFor()
  await securityCard(page, t('Login sessions'))
    .getByText(t('Current'), { exact: true })
    .waitFor()
}

let browser
let origin
try {
  await new Promise((done) => server.listen(0, '127.0.0.1', done))
  origin = `http://localhost:${server.address().port}`
  const { chromium } = await import(
    process.env.MYAPI_PLAYWRIGHT_MODULE
      ? pathToFileURL(process.env.MYAPI_PLAYWRIGHT_MODULE).href
      : 'playwright'
  )
  browser = await chromium.launch({ headless: true })

  const visualScenarios = Object.keys(languageCodes).flatMap((language) => [
    {
      name: `${language}-desktop-1280`,
      language,
      width: 1280,
      height: 960,
      theme: 'light',
    },
    {
      name: `${language}-mobile-320`,
      language,
      width: 320,
      height: 960,
      theme: 'light',
    },
  ])
  visualScenarios.push(
    {
      name: 'zh-desktop-1440',
      language: 'zh',
      width: 1440,
      height: 960,
      theme: 'light',
    },
    {
      name: 'zh-mobile-390',
      language: 'zh',
      width: 390,
      height: 960,
      theme: 'light',
    },
    {
      name: 'vi-mobile-320-dark',
      language: 'vi',
      width: 320,
      height: 960,
      theme: 'dark',
    },
    {
      name: 'en-desktop-1280-dark',
      language: 'en',
      width: 1280,
      height: 960,
      theme: 'dark',
    }
  )

  for (const scenario of visualScenarios) {
    const fixture = createUIFixture({
      role: 100,
      language: languageCodes[scenario.language],
      longText: scenario.language === 'fr',
    })
    const state = securityState(fixture, scenario, origin)
    const context = await browser.newContext({
      viewport: { width: scenario.width, height: scenario.height },
      locale:
        scenario.language === 'zh'
          ? 'zh-CN'
          : scenario.language === 'zh-TW'
            ? 'zh-TW'
            : scenario.language,
      timezoneId: 'UTC',
      reducedMotion: 'reduce',
      colorScheme: scenario.theme,
      serviceWorkers: 'block',
      hasTouch: scenario.width < 768,
      isMobile: scenario.width < 768,
    })
    await context.addCookies([
      { name: 'vite-ui-theme', value: scenario.theme, url: origin },
      { name: 'sidebar_state', value: 'true', url: origin },
    ])
    await context.addInitScript(
      (language) => localStorage.setItem('i18nextLng', language),
      languageCodes[scenario.language]
    )
    await context.route('**/*', async (route) => {
      const request = route.request()
      const url = new URL(request.url())
      if (url.origin !== origin) {
        if (url.protocol === 'data:' || url.protocol === 'blob:') {
          return route.continue()
        }
        state.external.push(url.href)
        return route.abort('blockedbyclient')
      }
      if (!url.pathname.startsWith('/api/') && !url.pathname.startsWith('/pg/')) {
        return route.continue()
      }
      const response = await responseFor(request, fixture, state)
      if (response.violation) state.violations.push(response.violation)
      await route.fulfill({
        status: response.status || 501,
        json:
          response.body || {
            success: false,
            message: response.violation || 'Unconfigured synthetic response',
          },
      })
    })
    await context.routeWebSocket('**/*', (socket) => {
      state.violations.push(`Unexpected WebSocket: ${socket.url()}`)
      socket.close()
    })
    const page = await context.newPage()
    page.setDefaultTimeout(25_000)
    await page.clock.setFixedTime(new Date(FIXTURE_TIME))
    page.on('pageerror', (error) => state.pageErrors.push(error.message))
    try {
      const revision = await openSecurity(page, scenario)
      await verifyRootRestriction(page, scenario, state)
      await screenshot(page, scenario, 'panel')
      if (
        scenario.name === 'zh-mobile-390' ||
        scenario.name === 'vi-mobile-320-dark'
      ) {
        await verifyMobilePasswordFooter(page, scenario)
      }
      assert.deepEqual(state.violations, [])
      assert.deepEqual(state.pageErrors, [])
      assert.deepEqual(state.external, [])
      report.journeys.push({
        name: scenario.name,
        language: scenario.language,
        width: scenario.width,
        theme: scenario.theme,
        role: state.auth.user.role,
        sid: state.auth.session.sid,
        revision,
        result: 'passed',
      })
    } finally {
      await context.close()
    }
  }

  const errorScenario = {
    name: 'zh-mobile-390-error-retry',
    language: 'zh',
    width: 390,
    height: 960,
    theme: 'light',
    failSecurityReads: true,
  }
  {
    const fixture = createUIFixture({ role: 100, language: 'zhCN' })
    const state = securityState(fixture, errorScenario, origin)
    const context = await browser.newContext({
      viewport: { width: errorScenario.width, height: errorScenario.height },
      locale: 'zh-CN',
      timezoneId: 'UTC',
      reducedMotion: 'reduce',
      colorScheme: 'light',
      serviceWorkers: 'block',
      hasTouch: true,
      isMobile: true,
    })
    await context.addCookies([
      { name: 'vite-ui-theme', value: 'light', url: origin },
      { name: 'sidebar_state', value: 'true', url: origin },
    ])
    await context.addInitScript(() => localStorage.setItem('i18nextLng', 'zhCN'))
    await context.route('**/*', async (route) => {
      const request = route.request()
      const url = new URL(request.url())
      if (url.origin !== origin) return route.abort('blockedbyclient')
      if (!url.pathname.startsWith('/api/') && !url.pathname.startsWith('/pg/')) {
        return route.continue()
      }
      const response = await responseFor(request, fixture, state)
      if (response.violation) state.violations.push(response.violation)
      await route.fulfill({
        status: response.status || 501,
        json: response.body || { success: false, message: response.violation },
      })
    })
    const page = await context.newPage()
    page.setDefaultTimeout(25_000)
    await page.clock.setFixedTime(new Date(FIXTURE_TIME))
    page.on('pageerror', (error) => state.pageErrors.push(error.message))
    try {
      const revision = await openSecurity(page, errorScenario)
      await verifyAndRecoverSecurityReads(page, errorScenario, state)
      await screenshot(page, errorScenario, 'recovered')
      assert.deepEqual(state.violations, [])
      assert.deepEqual(state.pageErrors, [])
      report.journeys.push({
        name: errorScenario.name,
        result: 'passed',
        revision,
        recovered: ['passkey', 'twoFA', 'sessions'],
      })
    } finally {
      await context.close()
    }
  }

  const flowScenario = {
    name: 'en-desktop-1280-complete-flow',
    language: 'en',
    width: 1280,
    height: 960,
    theme: 'light',
  }
  {
    const fixture = createUIFixture({ role: 100, language: 'en' })
    const state = securityState(fixture, flowScenario, origin)
    const context = await browser.newContext({
      viewport: { width: flowScenario.width, height: flowScenario.height },
      locale: 'en-US',
      timezoneId: 'UTC',
      reducedMotion: 'reduce',
      colorScheme: 'light',
      serviceWorkers: 'block',
    })
    await context.grantPermissions(
      ['clipboard-read', 'clipboard-write'],
      { origin }
    )
    await context.addCookies([
      { name: 'vite-ui-theme', value: 'light', url: origin },
      { name: 'sidebar_state', value: 'true', url: origin },
    ])
    await context.addInitScript(() => localStorage.setItem('i18nextLng', 'en'))
    await context.route('**/*', async (route) => {
      const request = route.request()
      const url = new URL(request.url())
      if (url.origin !== origin) {
        if (url.protocol === 'data:' || url.protocol === 'blob:') return route.continue()
        state.external.push(url.href)
        return route.abort('blockedbyclient')
      }
      if (!url.pathname.startsWith('/api/') && !url.pathname.startsWith('/pg/')) {
        return route.continue()
      }
      const response = await responseFor(request, fixture, state)
      if (response.violation) state.violations.push(response.violation)
      await route.fulfill({
        status: response.status || 501,
        json:
          response.body || {
            success: false,
            message: response.violation || 'Unconfigured synthetic response',
          },
      })
    })
    await context.routeWebSocket('**/*', (socket) => {
      state.violations.push(`Unexpected WebSocket: ${socket.url()}`)
      socket.close()
    })
    const page = await context.newPage()
    page.setDefaultTimeout(25_000)
    await page.clock.setFixedTime(new Date(FIXTURE_TIME))
    page.on('pageerror', (error) => state.pageErrors.push(error.message))
    const cdp = await context.newCDPSession(page)
    await cdp.send('WebAuthn.enable')
    const virtualAuthenticator = await cdp.send(
      'WebAuthn.addVirtualAuthenticator',
      {
        options: {
          protocol: 'ctap2',
          transport: 'internal',
          hasResidentKey: true,
          hasUserVerification: true,
          isUserVerified: true,
          automaticPresenceSimulation: true,
        },
      }
    )
    try {
      const revision = await openSecurity(page, flowScenario)
      state.stage = 'root-restriction'
      await verifyRootRestriction(page, flowScenario, state)
      state.stage = 'password'
      await verifyPassword(page, flowScenario, state)
      state.stage = 'access-token'
      await verifyAccessToken(page, flowScenario, state)
      const passkeyRemoveHeight = await verifyPasskey(page, flowScenario, state)
      await verifyTwoFA(page, flowScenario, state)
      await verifySessions(page, flowScenario, state)
      assert.equal(
        state.rotatedAuthorizationSeen,
        true,
        'A request after password rotation uses the rotated bearer token'
      )
      state.stage = 'complete'
      await screenshot(page, flowScenario, 'completed')
      assert.deepEqual(state.violations, [])
      assert.deepEqual(state.pageErrors, [])
      assert.deepEqual(state.external, [])
      report.journeys.push({
        name: flowScenario.name,
        result: 'passed',
        stage: state.stage,
        role: state.auth.user.role,
        sid: state.auth.session.sid,
        revision,
        passwordValidationAndKeyboardSubmit: true,
        authRotationObservedOnSubsequentRequest: state.rotatedAuthorizationSeen,
        accessToken: {
          calls: state.counts.accessToken,
          doubleConfirmation: true,
          screenshotRedacted: true,
          clearedOnClose: true,
        },
        passkey: {
          boundary: 'Chromium CDP virtual authenticator; not real hardware',
          beginCalls: state.counts.passkeyBegin,
          finishCalls: state.counts.passkeyFinish,
          removeTargetHeight: passkeyRemoveHeight,
          authRotationObservedOnStatusRead: state.passkeyRotationSeen,
        },
        twoFA: {
          setupCalls: state.counts.twoFASetup,
          enableCalls: state.counts.twoFAEnable,
          backupCodeCalls: state.counts.backupCodes,
          enableAuthRotationObserved: state.twoFARotationSeen,
          backupAuthRotationObserved: state.backupCodeRotationSeen,
          secretsAbsentAfterClose: true,
        },
        sessionRevokeOtherCalls: state.counts.sessionRevokeOthers,
        mutations: state.mutations,
      })
    } catch (error) {
      if (state.stage.startsWith('passkey')) {
        try {
          const card = securityCard(
            page,
            tr('Passkey Login', flowScenario.language)
          )
          if ((await card.count()) === 1 && (await card.isVisible())) {
            const file = `advanced-security-${flowScenario.name}-passkey-failed.png`
            await card.screenshot({
              path: resolve(output, file),
              animations: 'disabled',
            })
            report.screenshots.push({
              file,
              language: flowScenario.language,
              width: flowScenario.width,
              theme: flowScenario.theme,
              state: 'passkey-failed',
            })
          }
        } catch {
          // Preserve the original journey failure when evidence capture fails.
        }
      }
      report.journeys.push({
        name: flowScenario.name,
        result: 'failed',
        stage: state.stage,
        passkey: {
          beginCalls: state.counts.passkeyBegin,
          finishCalls: state.counts.passkeyFinish,
          authRotationObserved: state.passkeyRotationSeen,
        },
        twoFA: {
          setupCalls: state.counts.twoFASetup,
          enableCalls: state.counts.twoFAEnable,
          backupCodeCalls: state.counts.backupCodes,
          enableAuthRotationObserved: state.twoFARotationSeen,
          backupAuthRotationObserved: state.backupCodeRotationSeen,
        },
        sessionRevokeOtherCalls: state.counts.sessionRevokeOthers,
      })
      throw error
    } finally {
      await cdp.send('WebAuthn.removeVirtualAuthenticator', {
        authenticatorId: virtualAuthenticator.authenticatorId,
      })
      await cdp.send('WebAuthn.disable')
      await context.close()
    }
  }

  report.result = 'passed'
} catch (error) {
  report.result = 'failed'
  report.errors.push(error.stack || String(error))
  throw error
} finally {
  persist()
  if (browser) await browser.close()
  await new Promise((done) => server.close(done))
  console.log(JSON.stringify(report))
}
