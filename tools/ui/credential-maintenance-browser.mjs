// Production-build Chromium qualification for Root-owned Key credential
// maintenance. All identities, credentials, pools, proofs and receipts are
// synthetic. This driver never contacts a live provider or credential store.
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
    `${repo}/.local-tests/credential-maintenance-browser`
)
const version = readFileSync(resolve(repo, 'VERSION'), 'utf8').trim()
const sourceHead = execFileSync('git', ['rev-parse', 'HEAD'], {
  cwd: repo,
  encoding: 'utf8',
}).trim()
const languages = { en: 'en', zh: 'zhCN', vi: 'vi' }
const translations = Object.fromEntries(
  Object.keys(languages).map((language) => [
    language,
    JSON.parse(
      readFileSync(
        resolve(repo, `web/src/i18n/locales/${language}.json`),
        'utf8'
      )
    ).translation,
  ])
)
const mime = {
  '.html': 'text/html',
  '.js': 'text/javascript',
  '.css': 'text/css',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.woff2': 'font/woff2',
}

assert(existsSync(resolve(build, 'index.html')), 'Build the actual frontend first')
mkdirSync(output, { recursive: true })

const server = createServer((request, response) => {
  const url = new URL(request.url || '/', 'http://localhost')
  let path
  try {
    path = resolve(build, `.${decodeURIComponent(url.pathname)}`)
  } catch {
    response.writeHead(400).end()
    return
  }
  if (!path.startsWith(`${build}${sep}`) && path !== build) {
    response.writeHead(403).end()
    return
  }
  if (!existsSync(path) || statSync(path).isDirectory()) {
    path = resolve(build, 'index.html')
  }
  response.writeHead(200, {
    'content-type': mime[extname(path)] || 'application/octet-stream',
    'cache-control': 'no-store',
  })
  createReadStream(path).pipe(response)
})

const scenarios = [
  {
    name: 'desktop-blocked-guidance',
    width: 1440,
    language: 'zh',
    theme: 'light',
    result: 'blocked',
  },
  {
    name: 'desktop-success-keyboard',
    width: 1440,
    language: 'en',
    theme: 'light',
    result: 'success',
  },
  {
    name: 'mobile-read-failure',
    width: 390,
    language: 'zh',
    theme: 'light',
    result: 'read-failure',
  },
  {
    name: 'narrow-dark-long-unknown',
    width: 320,
    language: 'vi',
    theme: 'dark',
    longText: true,
    result: 'unknown',
  },
]
const digest = 'a'.repeat(64)
const report = {
  schema: 1,
  synthetic: true,
  version,
  sourceHead,
  fixtureTime: new Date(FIXTURE_TIME).toISOString(),
  source:
    'Actual web/dist served locally with isolated synthetic maintenance, proof and receipt APIs.',
  scope:
    'Root-owned Key credential maintenance at 1440, 390 and 320 px, including dark theme, long copy, keyboard entry, read failure and uncertain commit readback. Rotation remains read-only.',
  journeys: [],
  errors: [],
  result: 'running',
}
const persist = () =>
  writeFileSync(
    resolve(output, 'credential-maintenance-qualification.json'),
    JSON.stringify(report, null, 2)
  )

let browser
try {
  await new Promise((done) => server.listen(0, '127.0.0.1', done))
  const origin = `http://127.0.0.1:${server.address().port}`
  const driver = process.env.MYAPI_PLAYWRIGHT_MODULE
  const { chromium } = await import(
    driver ? pathToFileURL(resolve(driver)).href : 'playwright'
  )
  browser = await chromium.launch({
    headless: true,
    args: ['--disable-dev-shm-usage'],
  })

  for (const scenario of scenarios) {
    const t = (key) => {
      const value = translations[scenario.language]?.[key]
      assert(value, `Missing ${scenario.language} translation: ${key}`)
      return value
    }
    const context = await browser.newContext({
      viewport: { width: scenario.width, height: 960 },
      reducedMotion: 'reduce',
      colorScheme: scenario.theme,
      timezoneId: 'Asia/Shanghai',
      serviceWorkers: 'block',
    })
    await context.addInitScript(
      ({ language, theme }) => {
        localStorage.setItem('i18nextLng', language)
        localStorage.setItem('theme', theme)
      },
      { language: languages[scenario.language], theme: scenario.theme }
    )
    const fixture = createUIFixture({
      role: 100,
      language: languages[scenario.language],
      personalPolicy: true,
      longText: scenario.longText,
    })
    const channelName = scenario.longText
      ? 'Synthetic Codex target account with a deliberately long descriptive name for narrow layout qualification'
      : 'Synthetic Codex target'
    const sourceWindows = [
      {
        pool_id: 11,
        window_slot: 'primary',
        revision: 3,
        share_bps: 2500,
        observed_at: FIXTURE_TIME / 1000 - 30,
        reset_at: FIXTURE_TIME / 1000 + 3600,
      },
      {
        pool_id: 13,
        window_slot: 'secondary',
        revision: 2,
        share_bps: 1000,
        observed_at: FIXTURE_TIME / 1000 - 30,
        reset_at: FIXTURE_TIME / 1000 + 86400,
      },
    ]
    const targetWindows = sourceWindows.map((window, index) => ({
      ...window,
      pool_id: 12 + index * 2,
      revision: 4,
      share_bps: 0,
      available_bps: 7500,
    }))
    const changes = {
      policy: {
        changed: true,
        before_revision: 2,
        after_revision: 3,
        source_channel_id: 7,
        target_channel_id: 8,
      },
      allocations: sourceWindows.map((window, index) => ({
        source_pool_id: window.pool_id,
        target_pool_id: targetWindows[index].pool_id,
        window_slot: window.window_slot,
        share_bps: window.share_bps,
        target_existing: false,
      })),
    }
    const maintenance = {
      token: {
        id: 21,
        name: scenario.longText
          ? 'Synthetic desktop Key with a deliberately long name for narrow layout qualification'
          : 'Synthetic desktop Key',
        masked_key: 'sk-…abcd',
        owner_user_id: 1,
      },
      permissions: { can_migrate: true, reason: '' },
      rotation: {
        commit_supported: false,
        status: 'blocked',
        blockers: [
          { code: 'rotation_commit_not_supported', state: 'blocked' },
          {
            code: 'token_secret_cache_locator_compatibility_unproven',
            state: 'blocked',
          },
          { code: 'global_inflight_registry_unavailable', state: 'blocked' },
        ],
      },
      migration: {
        status: 'ready',
        source: {
          channel_id: 7,
          channel_name: 'Synthetic current Codex account',
          subject_hint: 'abcd…wxyz',
          windows: sourceWindows,
        },
        candidates: [
          {
            channel_id: 8,
            channel_name: channelName,
            subject_hint: 'efgh…stuv',
            credential_mode: 'single',
            status: 'ready',
            blockers: [],
            windows: targetWindows,
          },
        ],
        blockers: [],
      },
    }
    let maintenanceReads = 0
    let previewWrites = 0
    let commitWrites = 0
    let receiptReads = 0
    let failurePending = scenario.result === 'read-failure'
    const violations = []
    if (scenario.result === 'blocked') {
      maintenance.migration.status = 'unknown'
      maintenance.migration.blockers = [
        { code: 'token_budget_conflicts_with_subscription_share', state: 'blocked' },
        { code: 'source_usage_history_unknown', state: 'unknown' },
        { code: 'subscription_baseline_not_ready', state: 'unknown' },
      ]
    }

    await context.route('**/*', async (route) => {
      try {
        const request = route.request()
        const url = new URL(request.url())
        if (url.origin !== origin) {
          violations.push(`External request: ${url.origin}`)
          return route.abort()
        }
        const path = url.pathname.replace(/\/$/, '')
        const method = request.method()
        if (path === '/api/token/21/maintenance' && method === 'GET') {
          maintenanceReads += 1
          if (failurePending) {
            failurePending = false
            return route.fulfill({
              status: 503,
              json: { success: false, message: 'Synthetic maintenance failure' },
            })
          }
          return route.fulfill({ json: { success: true, data: maintenance } })
        }
        if (
          path === '/api/token/21/account-migration/preview' &&
          method === 'POST'
        ) {
          previewWrites += 1
          const body = request.postDataJSON()
          assert.deepEqual(body, {
            target_channel_id: 8,
            target_shares: changes.allocations.map((allocation) => ({
              source_pool_id: allocation.source_pool_id,
              target_pool_id: allocation.target_pool_id,
              share_bps: allocation.share_bps,
            })),
          })
          return route.fulfill({
            json: {
              success: true,
              data: {
                status: 'ready',
                preview_revision: digest,
                expires_at: FIXTURE_TIME / 1000 + 300,
                blockers: [],
                changes,
              },
            },
          })
        }
        if (path === '/api/verify' && method === 'POST') {
          const body = request.postDataJSON()
          assert.equal(body.scope, 'token.account_migration')
          return route.fulfill({
            json: {
              success: true,
              data: {
                proof_token: 'synthetic-proof',
                expires_at: FIXTURE_TIME / 1000 + 60,
                method: '2fa',
                scope: 'token.account_migration',
              },
            },
          })
        }
        if (
          path === '/api/token/21/account-migration' &&
          method === 'POST'
        ) {
          commitWrites += 1
          assert.equal(request.headers()['x-security-proof'], 'synthetic-proof')
          const body = request.postDataJSON()
          assert.equal(body.confirmed, true)
          assert.equal(body.preview_revision, digest)
          assert.match(body.operation_id, /^[0-9a-f]{64}$/)
          if (scenario.result === 'unknown') {
            return route.fulfill({
              status: 503,
              json: { success: false, code: 'result_unknown', message: '' },
            })
          }
          return route.fulfill({
            json: {
              success: true,
              data: {
                operation_id: body.operation_id,
                state: 'committed',
                replayed: false,
                token_id: 21,
                source_subject_hint: 'abcd…wxyz',
                target_subject_hint: 'efgh…stuv',
                source_channel_id: 7,
                target_channel_id: 8,
                policy_before_revision: 2,
                policy_after_revision: 3,
                created_at: FIXTURE_TIME / 1000,
                changes,
              },
            },
          })
        }
        if (
          path.startsWith('/api/token/21/maintenance/operations/') &&
          method === 'GET'
        ) {
          receiptReads += 1
          return route.fulfill({
            status: 404,
            json: { success: false, code: 'not_found', message: '' },
          })
        }
        if (path === '/api/user/2fa/status' && method === 'GET') {
          return route.fulfill({
            json: { success: true, data: { enabled: true } },
          })
        }
        if (path === '/api/user/passkey' && method === 'GET') {
          return route.fulfill({
            json: { success: true, data: { enabled: false } },
          })
        }
        if (!path.startsWith('/api/') && !path.startsWith('/pg/')) {
          return route.continue()
        }
        const result = fixture.response(url, method)
        if (result.violation) violations.push(result.violation)
        assert(!result.violation, result.violation)
        return route.fulfill({ status: result.status, json: result.body })
      } catch (error) {
        const message = `${scenario.name}: ${error.message}`
        violations.push(message)
        report.errors.push(message)
        return route.abort('failed')
      }
    })

    const page = await context.newPage()
    page.setDefaultTimeout(15000)
    page.on('pageerror', (error) =>
      report.errors.push(`${scenario.name}: page error: ${error.message}`)
    )
    await page.clock.setFixedTime(new Date(FIXTURE_TIME))
    await page.goto(`${origin}/keys`, { waitUntil: 'networkidle' })
    const buildRevision = await page.locator('html').getAttribute('data-build-rev')
    assert(buildRevision?.startsWith(`rv.${version}.${process.env.GITHUB_SHA || sourceHead}.`))
    const visibleVersion = page.getByTestId(scenario.width < 768 ? 'console-mobile-build-version' : 'console-build-revision')
    assert.equal((await visibleVersion.textContent())?.trim(), version)
    const entry = page
      .getByRole('button', { name: t('Credential maintenance'), exact: true })
      .first()
    await entry.focus()
    assert(await entry.evaluate((element) => document.activeElement === element))
    await page.keyboard.press('Enter')
    const dialog = page.getByRole('dialog', {
      name: t('Credential maintenance'),
      exact: true,
    })
    await dialog.waitFor()
    if (scenario.theme === 'dark') {
      assert(
        await page
          .locator('html')
          .evaluate((element) => element.classList.contains('dark')),
        `${scenario.name}: dark theme is active`
      )
    }
    assert(
      await dialog.evaluate(
        (element) => element.scrollWidth <= element.clientWidth + 1
      ),
      `${scenario.name}: dialog has no horizontal overflow`
    )
    const screenshots = []
    const capture = async (suffix) => {
      await page.evaluate(async () => {
        await document.fonts.ready
        await Promise.all(document.getAnimations().map((animation) => animation.finished.catch(() => {})))
        await new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done)))
      })
      const file = `${scenario.name}-${suffix}.png`
      await page.screenshot({
        path: resolve(output, file),
        animations: 'disabled',
      })
      screenshots.push(file)
    }

    if (scenario.result === 'read-failure') {
      await dialog.getByText(t('Could not load credential maintenance')).waitFor()
      await capture('read-failure')
      const retry = dialog.getByRole('button', { name: t('Retry') })
      await retry.focus()
      await page.keyboard.press('Enter')
    }

    await dialog.getByText(maintenance.token.name, { exact: true }).waitFor()
    if (scenario.result === 'blocked') {
      await dialog.getByText(t('This Key still has an active budget mode'), { exact: true }).waitFor()
      await dialog.getByText(t('Request accounting is still unresolved'), { exact: true }).waitFor()
      const baseline = dialog.getByText(t('A trusted account baseline is not ready'), { exact: true })
      await baseline.waitFor()
      await baseline.scrollIntoViewIfNeeded()
      assert.equal(await dialog.getByRole('button', { name: t('Preview migration') }).count(), 0)
      assert.equal(await dialog.getByRole('button', { name: t('Verify and move account') }).count(), 0)
      await capture('blocked')
      assert.equal(previewWrites, 0)
      assert.equal(commitWrites, 0)
      assert.equal(receiptReads, 0)
      assert.equal(violations.length, 0, violations.join('\n'))
      report.journeys.push({ ...scenario, maintenanceReads, previewWrites, commitWrites, receiptReads, screenshots, violations })
      await context.close()
      persist()
      continue
    }
    await capture('loaded')
    const target = dialog.getByRole('combobox', {
      name: t('Target Codex account'),
    })
    await target.click()
    await page.getByText(`${channelName} · efgh…stuv`, { exact: true }).click()
    const previewButton = dialog.getByRole('button', {
      name: t('Preview migration'),
    })
    await previewButton.focus()
    await page.keyboard.press('Enter')
    await dialog.getByText(t('Confirm migration'), { exact: true }).waitFor()
    const confirmations = dialog.getByRole('checkbox')
    assert.equal(await confirmations.count(), 2)
    for (let index = 0; index < 2; index += 1) {
      await confirmations.nth(index).focus()
      await page.keyboard.press('Space')
    }
    const commit = dialog.getByRole('button', {
      name: t('Verify and move account'),
    })
    await commit.scrollIntoViewIfNeeded()
    if (scenario.width <= 390) {
      const navigation = await page.locator('nav.pc-mobile-nav').boundingBox()
      const action = await commit.boundingBox()
      assert(navigation && action && action.y >= 0 && action.y + action.height <= navigation.y,
        `${scenario.name}: migration action remains above mobile navigation`)
    }
    await capture('confirm-actions')
    await commit.focus()
    await page.keyboard.press('Enter')
    const verification = page.getByRole('dialog', {
      name: t('Verify account migration'),
    })
    await verification.getByPlaceholder(t('Enter verification code')).fill('123456')
    await page.keyboard.press('Enter')

    if (scenario.result === 'unknown') {
      await dialog.getByText(t('Migration result is unknown')).waitFor()
      await dialog.getByText(t('Migration result is unknown')).scrollIntoViewIfNeeded()
      await capture('unknown')
      assert.equal(commitWrites, 1)
      assert.equal(receiptReads, 1)
      const readAgain = dialog.getByRole('button', {
        name: t('Check receipt again'),
      })
      await readAgain.focus()
      const readback = page.waitForResponse(
        (response) =>
          response.request().method() === 'GET' &&
          new URL(response.url()).pathname.startsWith(
            '/api/token/21/maintenance/operations/'
          )
      )
      await page.keyboard.press('Enter')
      await readback
      assert.equal(commitWrites, 1, 'Unknown result never repeats commit POST')
      assert.equal(receiptReads, 2, 'Unknown result retries only receipt GET')
    } else {
      await dialog.getByText(t('Account migration committed')).waitFor()
      await dialog.getByText(t('Account migration committed')).scrollIntoViewIfNeeded()
      await capture('committed')
      assert.equal(commitWrites, 1)
      assert.equal(receiptReads, 0)
    }

    assert.equal(previewWrites, 1)
    assert.equal(violations.length, 0, violations.join('\n'))
    report.journeys.push({
      ...scenario,
      maintenanceReads,
      previewWrites,
      commitWrites,
      receiptReads,
      screenshots,
      violations,
    })
    await context.close()
    persist()
  }
  assert.equal(report.errors.length, 0, report.errors.join('\n'))
  report.result = 'passed'
} catch (error) {
  report.result = 'failed'
  report.errors.push(error.stack || String(error))
  throw error
} finally {
  persist()
  if (browser) await browser.close()
  await new Promise((done) => server.close(done))
}
