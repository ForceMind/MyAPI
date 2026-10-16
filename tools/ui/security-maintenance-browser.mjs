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
    failures: {
      // PasskeyCard also probes both methods for secure verification. Fail both
      // initial reads so the status cards deterministically expose Retry.
      passkey: scenario.failSecurityReads ? 2 : 0,
      twoFA: scenario.failSecurityReads ? 2 : 0,
      sessions: scenario.failSecurityReads ? 1 : 0,
    },
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
  }

  if (method === 'GET' && path === '/api/user/token') {
    state.counts.accessToken += 1
    return ok(syntheticAccessToken)
  }
  if (method === 'GET' && path === '/api/user/2fa/status') {
    if (state.failures.twoFA > 0) {
      state.failures.twoFA -= 1
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
    if (state.failures.passkey > 0) {
      state.failures.passkey -= 1
      return {
        status: 503,
        body: { success: false, message: 'Synthetic Passkey status unavailable' },
      }
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
    if (state.failures.sessions > 0) {
      state.failures.sessions -= 1
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
    state.mutations.push('POST /api/user/passkey/register/begin')
    return ok({
      flow_token: 'synthetic-passkey-flow',
      options: {
        publicKey: {
          challenge: 'AQIDBAUGBwgJCgsMDQ4PEA',
          rp: { id: '127.0.0.1', name: 'MyAPI synthetic fixture' },
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
    const body = request.postDataJSON()
    if (
      body?.flow_token !== 'synthetic-passkey-flow' ||
      typeof body?.credential?.id !== 'string' ||
      !body.credential.id
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
    state.twoFAEnabled = true
    state.backupCodesRemaining = syntheticBackupCodes.length
    state.mutations.push('POST /api/user/2fa/enable')
    return ok(authRotation(state, 'synthetic-two-fa-rotation'))
  }
  if (method === 'POST' && path === '/api/user/2fa/backup_codes') {
    assert.equal(request.postDataJSON()?.code, '123456')
    state.counts.backupCodes += 1
    state.mutations.push('POST /api/user/2fa/backup_codes')
    return ok({
      ...authRotation(state, 'synthetic-backup-code-rotation'),
      backup_codes: syntheticBackupCodes.map((code) => `${code}-NEW`),
    })
  }
  if (method === 'POST' && path === '/api/user/sessions/revoke-others') {
    state.counts.sessionRevokeOthers += 1
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
    const navigation = page.getByRole('navigation', {
      name: tr('Mobile console navigation', scenario.language),
    })
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
    .getByText(t('Password and account access'), { exact: true })
    .waitFor()
  await page
    .getByText(t('Passkey Login'), { exact: true })
    .waitFor()
  await page
    .getByText(t('Two-Factor Authentication'), { exact: true })
    .waitFor()
  await page
    .getByText(t('Login sessions'), { exact: true })
    .waitFor()
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
  const action = page
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
  await screenshot(page, scenario, 'password-dialog')
  await cancel.click()
  await dialog.waitFor({ state: 'detached' })
}

async function verifyPassword(page, scenario, state) {
  const t = (key) => tr(key, scenario.language)
  const action = page
    .getByRole('button')
    .filter({ has: page.getByText(t('Change Password'), { exact: true }) })
  await action.click()
  let dialog = page.getByRole('dialog', { name: t('Change Password') })
  await dialog.getByLabel(t('Current Password')).fill('old-password')
  await dialog.getByLabel(t('New Password')).fill('new-password')
  await dialog.getByLabel(t('Confirm New Password')).fill('mismatch-password')
  await dialog.getByRole('button', { name: t('Change Password') }).click()
  await page.getByText(t('Passwords do not match'), { exact: true }).waitFor()
  assert.equal(state.counts.password, 0, 'Client validation blocks the password request')
  await dialog.getByRole('button', { name: t('Cancel'), exact: true }).click()

  await action.click()
  dialog = page.getByRole('dialog', { name: t('Change Password') })
  for (const key of ['Current Password', 'New Password', 'Confirm New Password']) {
    assert.equal(await dialog.getByLabel(t(key)).inputValue(), '')
  }
  await dialog.getByLabel(t('Current Password')).fill('old-password')
  await dialog.getByLabel(t('New Password')).fill('new-password')
  await dialog.getByLabel(t('Confirm New Password')).fill('new-password')
  await dialog.getByLabel(t('Confirm New Password')).press('Enter')
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
  const tokenInput = dialog.getByLabel(t('Token'))
  await tokenInput.waitFor()
  assert.equal(await tokenInput.inputValue(), syntheticAccessToken)
  assert.equal(state.counts.accessToken, 1)
  await tokenInput.evaluate((element) => {
    element.type = 'password'
  })
  await screenshot(page, scenario, 'access-token-redacted')
  await dialog.getByRole('button', { name: t('Close'), exact: true }).click()
  await action.click()
  dialog = page.getByRole('dialog', { name: t('Access Token') })
  assert.equal(await dialog.getByLabel(t('Token')).count(), 0)
  assert.equal(state.counts.accessToken, 1, 'Reopening never regenerates')
  await dialog.getByRole('button', { name: t('Close'), exact: true }).click()
}

async function verifyPasskey(page, scenario, state) {
  const t = (key) => tr(key, scenario.language)
  const card = page
    .locator('[data-slot="card"]')
    .filter({
      has: page.getByText(t('Passkey Login'), { exact: true }),
    })
  const enable = card.getByRole('button', {
    name: t('Enable Passkey'),
    exact: true,
  })
  await enable.waitFor()
  assert.equal(await enable.isEnabled(), true, 'Virtual authenticator is detected')
  await enable.click()
  await card
    .getByRole('button', { name: t('Remove Passkey'), exact: true })
    .waitFor()
  assert.equal(state.counts.passkeyBegin, 1)
  assert.equal(state.counts.passkeyFinish, 1)
}

async function verifyTwoFA(page, scenario, state) {
  const t = (key) => tr(key, scenario.language)
  const card = page
    .locator('[data-slot="card"]')
    .filter({
      has: page.getByText(t('Two-Factor Authentication'), { exact: true }),
    })
  await card.getByRole('button', { name: t('Enable'), exact: true }).click()
  const setup = page.getByRole('dialog', {
    name: t('Setup Two-Factor Authentication'),
  })
  await setup.getByRole('button', { name: t('Next'), exact: true }).waitFor()
  await setup.getByRole('button', { name: t('Next'), exact: true }).click()
  await setup.getByRole('button', { name: t('Next'), exact: true }).click()
  await setup.getByLabel(t('Verification Code')).fill('123456')
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

  await card
    .getByRole('button', {
      name: t('Regenerate Backup Codes'),
      exact: true,
    })
    .click()
  const backupDialog = page.getByRole('dialog', {
    name: t('Regenerate Backup Codes'),
  })
  await backupDialog.getByLabel(t('Verification Code')).fill('123456')
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
}

async function verifySessions(page, scenario, state) {
  const t = (key) => tr(key, scenario.language)
  const card = page
    .locator('[data-slot="card"]')
    .filter({
      has: page.getByText(t('Login sessions'), { exact: true }),
    })
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
}

async function recoverSecurityReads(page, scenario) {
  const t = (key) => tr(key, scenario.language)
  const cards = [
    [t('Passkey Login'), t('Failed to load Passkey status')],
    [t('Two-Factor Authentication'), t('Unable to load settings')],
    [t('Login sessions'), t('Unable to load login sessions')],
  ]
  for (const [heading, errorTitle] of cards) {
    const card = page
      .locator('[data-slot="card"]')
      .filter({ has: page.getByText(heading, { exact: true }) })
    await card.getByText(errorTitle, { exact: true }).waitFor()
    await card.getByRole('button', { name: t('Retry'), exact: true }).click()
  }
  await page
    .getByText(t('Passkey Authentication'), { exact: true })
    .waitFor()
  await page.getByText(t('Two-Step Verification'), { exact: true }).waitFor()
  await page.getByText(t('Current'), { exact: true }).waitFor()
}

let browser
let origin
try {
  await new Promise((done) => server.listen(0, '127.0.0.1', done))
  origin = `http://127.0.0.1:${server.address().port}`
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
      if (scenario.name === 'zh-mobile-390') {
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
      await screenshot(page, errorScenario, 'failed-statuses')
      await recoverSecurityReads(page, errorScenario)
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
      await verifyRootRestriction(page, flowScenario, state)
      await verifyPassword(page, flowScenario, state)
      await verifyAccessToken(page, flowScenario, state)
      await verifyPasskey(page, flowScenario, state)
      await verifyTwoFA(page, flowScenario, state)
      await verifySessions(page, flowScenario, state)
      assert.equal(
        state.rotatedAuthorizationSeen,
        true,
        'A request after password rotation uses the rotated bearer token'
      )
      await screenshot(page, flowScenario, 'completed')
      assert.deepEqual(state.violations, [])
      assert.deepEqual(state.pageErrors, [])
      assert.deepEqual(state.external, [])
      report.journeys.push({
        name: flowScenario.name,
        result: 'passed',
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
        },
        twoFA: {
          setupCalls: state.counts.twoFASetup,
          enableCalls: state.counts.twoFAEnable,
          backupCodeCalls: state.counts.backupCodes,
          secretsAbsentAfterClose: true,
        },
        sessionRevokeOtherCalls: state.counts.sessionRevokeOthers,
        mutations: state.mutations,
      })
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
