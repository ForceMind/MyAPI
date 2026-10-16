// Production-build Chromium qualification for Root-owned Key credential
// rotation. Every identity, proof, credential and receipt is synthetic.
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
    `${repo}/.local-tests/credential-rotation-browser`
)
const version = readFileSync(resolve(repo, 'VERSION'), 'utf8').trim()
const sourceHead = execFileSync('git', ['rev-parse', 'HEAD'], {
  cwd: repo,
  encoding: 'utf8',
}).trim()
const buildSource = process.env.GITHUB_SHA || sourceHead
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

assert(
  existsSync(resolve(build, 'index.html')),
  'Build the actual frontend first'
)
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
    name: 'desktop-ready-fresh-secret',
    width: 1440,
    language: 'en',
    theme: 'light',
    result: 'success',
  },
  {
    name: 'mobile-preview-retry',
    width: 390,
    language: 'zh',
    theme: 'light',
    result: 'preview-retry',
  },
  {
    name: 'narrow-dark-unknown-recovery',
    width: 320,
    language: 'vi',
    theme: 'dark',
    longText: true,
    result: 'unknown',
  },
  {
    name: 'desktop-blocked',
    width: 1440,
    language: 'zh',
    theme: 'light',
    result: 'blocked',
  },
]
const digest = 'a'.repeat(64)
const syntheticSecret = `sk-${'R'.repeat(48)}`
const report = {
  schema: 1,
  synthetic: true,
  version,
  sourceSha: buildSource,
  sourceHead,
  fixtureTime: new Date(FIXTURE_TIME).toISOString(),
  source:
    'Actual web/dist and checked-in locale files served locally with isolated synthetic maintenance, verification, rotation and receipt APIs.',
  scope:
    'Root-owned Key credential rotation at 1440, 390 and 320 px, including keyboard operation, preview retry, one-time reveal, dark long copy, unknown recovery and blocked qualification.',
  journeys: [],
  errors: [],
  result: 'running',
}
const persist = () =>
  writeFileSync(
    resolve(output, 'credential-rotation-qualification.json'),
    JSON.stringify(report, null, 2)
  )

let browser
try {
  await new Promise((done) => server.listen(0, '127.0.0.1', done))
  const origin = `http://127.0.0.1:${server.address().port}`
  const playwrightModule = process.env.MYAPI_PLAYWRIGHT_MODULE
  const { chromium } = await import(
    playwrightModule
      ? pathToFileURL(resolve(playwrightModule)).href
      : 'playwright'
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
      permissions: ['clipboard-read', 'clipboard-write'],
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
    const available = scenario.result !== 'blocked'
    const tokenName = scenario.longText
      ? 'Synthetic Root-owned Key with a deliberately long name for narrow credential rotation qualification'
      : 'Synthetic Root-owned Key'
    const maintenance = {
      token: {
        id: 21,
        name: tokenName,
        masked_key: 'sk-…RRRR',
        owner_user_id: 1,
      },
      permissions: {
        can_migrate: false,
        reason: 'Synthetic rotation-only fixture',
      },
      rotation: {
        commit_supported: available,
        status: available ? 'ready' : 'blocked',
        blockers: available
          ? []
          : [
              { code: 'legacy_settlement_mode', state: 'blocked' },
              { code: 'legacy_async_identity_unknown', state: 'unknown' },
            ],
      },
      migration: {
        status: 'blocked',
        source: null,
        candidates: [],
        blockers: [{ code: 'migration_target_unavailable', state: 'blocked' }],
      },
    }
    const counts = {
      read: 0,
      preview: 0,
      verify: 0,
      commit: 0,
      receipt: 0,
    }
    const violations = []
    const pageErrors = []
    const receiptOperationIds = []
    let previewFailurePending = scenario.result === 'preview-retry'
    let submittedOperationId = ''

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
          counts.read += 1
          return route.fulfill({
            json: { success: true, message: '', data: maintenance },
          })
        }
        if (
          path === '/api/token/21/credential-rotation/preview' &&
          method === 'POST'
        ) {
          counts.preview += 1
          const body = request.postDataJSON()
          assert(
            body &&
              Object.getPrototypeOf(body) === Object.prototype &&
              Object.keys(body).length === 0,
            'Rotation preview sends a strict empty JSON object'
          )
          if (previewFailurePending) {
            previewFailurePending = false
            return route.fulfill({
              status: 503,
              json: { success: false, message: 'Synthetic preview failure' },
            })
          }
          return route.fulfill({
            json: {
              success: true,
              message: '',
              data: {
                status: 'ready',
                preview_revision: digest,
                expires_at: FIXTURE_TIME / 1000 + 300,
                credential_version_before: 1,
                credential_version_after: 2,
                blockers: [],
                effects: {
                  token_id_preserved: true,
                  old_secret_invalidation: 'immediate',
                  accepted_settlement: 'continues',
                  writer_changed: false,
                },
              },
            },
          })
        }
        if (path === '/api/verify' && method === 'POST') {
          counts.verify += 1
          const body = request.postDataJSON()
          assert.equal(body.scope, 'token.credential_rotation')
          return route.fulfill({
            json: {
              success: true,
              data: {
                proof_token: 'synthetic-rotation-proof',
                expires_at: FIXTURE_TIME / 1000 + 60,
                method: '2fa',
                scope: 'token.credential_rotation',
              },
            },
          })
        }
        if (path === '/api/token/21/credential-rotation' && method === 'POST') {
          counts.commit += 1
          assert(
            request.headers()['x-security-proof'] ===
              'synthetic-rotation-proof',
            'Rotation commit carries the dedicated proof'
          )
          const body = request.postDataJSON()
          assert.equal(body.confirmed, true)
          assert.equal(body.preview_revision, digest)
          assert.match(body.operation_id, /^[0-9a-f]{64}$/)
          submittedOperationId = body.operation_id
          if (scenario.result === 'unknown') {
            return route.fulfill({
              status: 503,
              json: { success: false, code: 'result_unknown', message: '' },
            })
          }
          const receipt = {
            operation_id: body.operation_id,
            state: 'committed',
            token_id: 21,
            credential_version_before: 1,
            credential_version_after: 2,
            masked_key: 'sk-…RRRR',
            created_at: FIXTURE_TIME / 1000,
            old_secret_invalidated_at: FIXTURE_TIME / 1000,
            settlement_mode: 'authoritative_persistent',
            writer_changed: false,
          }
          return route.fulfill({
            json: {
              success: true,
              message: '',
              data: {
                receipt,
                replayed: false,
                reveal: { secret: syntheticSecret },
              },
            },
          })
        }
        if (
          path.startsWith('/api/token/21/credential-rotation/operations/') &&
          method === 'GET'
        ) {
          counts.receipt += 1
          const operationId = path.slice(path.lastIndexOf('/') + 1)
          receiptOperationIds.push(operationId)
          assert.equal(
            operationId,
            submittedOperationId,
            'Recovery must read the submitted rotation operation'
          )
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
    page.on('pageerror', (error) => {
      const message = error.message
      pageErrors.push(message)
      report.errors.push(`${scenario.name}: page error: ${message}`)
    })
    await page.clock.setFixedTime(new Date(FIXTURE_TIME))
    await page.goto(`${origin}/keys`, { waitUntil: 'networkidle' })
    const buildRevision = await page
      .locator('html')
      .getAttribute('data-build-rev')
    assert(buildRevision?.startsWith(`rv.${version}.${buildSource}.`))
    const visibleVersion = page.getByTestId(
      scenario.width < 768
        ? 'console-mobile-build-version'
        : 'console-build-revision'
    )
    assert.equal((await visibleVersion.textContent())?.trim(), version)
    if (scenario.theme === 'dark') {
      assert(
        await page
          .locator('html')
          .evaluate((element) => element.classList.contains('dark')),
        `${scenario.name}: dark theme is active`
      )
    }

    const screenshots = []
    const capture = async (suffix) => {
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
      const file = `${scenario.name}-${suffix}.png`
      await page.screenshot({
        path: resolve(output, file),
        animations: 'disabled',
        mask: [page.locator('input')],
        maskColor: '#111827',
      })
      screenshots.push(file)
    }
    const maintenanceEntry = page
      .getByRole('button', {
        name: t('Credential maintenance'),
        exact: true,
      })
      .first()
    const maintenanceRead = page.waitForResponse(
      (response) =>
        response.request().method() === 'GET' &&
        new URL(response.url()).pathname === '/api/token/21/maintenance'
    )
    await maintenanceEntry.focus()
    assert(
      await maintenanceEntry.evaluate(
        (element) => document.activeElement === element
      )
    )
    await page.keyboard.press('Enter')
    await maintenanceRead
    const maintenanceDialog = page.getByRole('dialog', {
      name: t('Credential maintenance'),
      exact: true,
    })
    await maintenanceDialog.getByText(tokenName, { exact: true }).waitFor()
    assert(
      await maintenanceDialog.evaluate(
        (element) => element.scrollWidth <= element.clientWidth + 1
      ),
      `${scenario.name}: maintenance dialog has no horizontal overflow`
    )

    if (scenario.result === 'blocked') {
      await maintenanceDialog
        .getByText(t('Rotation remains read-only'), { exact: true })
        .waitFor()
      assert.equal(
        await maintenanceDialog
          .getByRole('button', {
            name: t('Check and rotate Key'),
            exact: true,
          })
          .count(),
        0
      )
      assert.deepEqual(counts, {
        read: 1,
        preview: 0,
        verify: 0,
        commit: 0,
        receipt: 0,
      })
      await capture('blocked')
    } else {
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
      assert(
        await rotationDialog.evaluate(
          (element) => element.scrollWidth <= element.clientWidth + 1
        ),
        `${scenario.name}: rotation dialog has no horizontal overflow`
      )
      const checkEligibility = rotationDialog.getByRole('button', {
        name: t('Check rotation eligibility'),
        exact: true,
      })
      await checkEligibility.scrollIntoViewIfNeeded()
      if (scenario.width <= 390) {
        const navigation = await page.locator('nav.pc-mobile-nav').boundingBox()
        const action = await checkEligibility.boundingBox()
        assert(
          navigation &&
            action &&
            action.y >= 0 &&
            action.y + action.height <= navigation.y,
          `${scenario.name}: primary rotation action stays above mobile navigation`
        )
        assert(
          await checkEligibility.evaluate((element) => {
            const box = element.getBoundingClientRect()
            const hit = document.elementFromPoint(
              box.left + box.width / 2,
              box.top + box.height / 2
            )
            return hit === element || Boolean(hit && element.contains(hit))
          }),
          `${scenario.name}: primary rotation action is hit-testable`
        )
      }
      const firstPreviewResponse = page.waitForResponse(
        (response) =>
          response.request().method() === 'POST' &&
          new URL(response.url()).pathname ===
            '/api/token/21/credential-rotation/preview'
      )
      await checkEligibility.focus()
      await page.keyboard.press('Enter')
      await firstPreviewResponse

      if (scenario.result === 'preview-retry') {
        await rotationDialog
          .getByText(t('Could not check rotation eligibility'), { exact: true })
          .waitFor()
        await capture('preview-failed')
        const retry = rotationDialog.getByRole('button', {
          name: t('Check rotation eligibility'),
          exact: true,
        })
        const retryResponse = page.waitForResponse(
          (response) =>
            response.request().method() === 'POST' &&
            new URL(response.url()).pathname ===
              '/api/token/21/credential-rotation/preview'
        )
        await retry.click()
        await retryResponse
        await rotationDialog
          .getByText(t('Rotation is ready'), { exact: true })
          .waitFor()
        await capture('preview-retried-ready')
        assert.deepEqual(counts, {
          read: 1,
          preview: 2,
          verify: 0,
          commit: 0,
          receipt: 0,
        })
      } else {
        await rotationDialog
          .getByText(t('Rotation is ready'), { exact: true })
          .waitFor()
        const confirmations = rotationDialog.getByRole('checkbox')
        assert.equal(await confirmations.count(), 3)
        for (let index = 0; index < 3; index += 1) {
          await confirmations.nth(index).focus()
          await page.keyboard.press('Space')
        }
        const rotate = rotationDialog.getByRole('button', {
          name: t('Verify and rotate Key'),
          exact: true,
        })
        await rotate.scrollIntoViewIfNeeded()
        if (scenario.width <= 390) {
          const navigation = await page
            .locator('nav.pc-mobile-nav')
            .boundingBox()
          const action = await rotate.boundingBox()
          assert(
            navigation && action && action.y + action.height <= navigation.y,
            `${scenario.name}: confirmed rotation action stays above mobile navigation`
          )
          assert(
            await rotate.evaluate((element) => {
              const box = element.getBoundingClientRect()
              const hit = document.elementFromPoint(
                box.left + box.width / 2,
                box.top + box.height / 2
              )
              return hit === element || Boolean(hit && element.contains(hit))
            }),
            `${scenario.name}: confirmed rotation action is hit-testable`
          )
        }
        await capture('review-confirmed')
        await rotate.focus()
        await page.keyboard.press('Enter')
        const verificationDialog = page.getByRole('dialog', {
          name: t('Verify credential rotation'),
          exact: true,
        })
        await verificationDialog.waitFor()
        const code = verificationDialog.getByPlaceholder(
          t('Enter verification code')
        )
        await code.fill('123456')
        const verifyResponse = page.waitForResponse(
          (response) =>
            response.request().method() === 'POST' &&
            new URL(response.url()).pathname === '/api/verify'
        )
        const commitResponse = page.waitForResponse(
          (response) =>
            response.request().method() === 'POST' &&
            new URL(response.url()).pathname ===
              '/api/token/21/credential-rotation'
        )
        const receiptResponse =
          scenario.result === 'unknown'
            ? page.waitForResponse(
                (response) =>
                  response.request().method() === 'GET' &&
                  new URL(response.url()).pathname.startsWith(
                    '/api/token/21/credential-rotation/operations/'
                  )
              )
            : null
        const refreshedMaintenance =
          scenario.result === 'success'
            ? page.waitForResponse(
                (response) =>
                  response.request().method() === 'GET' &&
                  new URL(response.url()).pathname ===
                    '/api/token/21/maintenance'
              )
            : null
        await page.keyboard.press('Enter')
        await verifyResponse
        await commitResponse
        if (receiptResponse) await receiptResponse
        if (refreshedMaintenance) await refreshedMaintenance

        if (scenario.result === 'success') {
          await rotationDialog
            .getByText(t('Key value rotated'), { exact: true })
            .waitFor()
          const secretInput = rotationDialog.getByLabel(t('New Key value'))
          assert.equal(await secretInput.getAttribute('type'), 'text')
          assert(
            (await secretInput.inputValue()) === syntheticSecret,
            'The fresh response displays its complete synthetic value'
          )
          const hide = rotationDialog.getByRole('button', {
            name: t('Hide value'),
            exact: true,
          })
          await hide.click()
          assert.equal(await secretInput.getAttribute('type'), 'password')
          const show = rotationDialog.getByRole('button', {
            name: t('Show value'),
            exact: true,
          })
          await show.click()
          assert.equal(await secretInput.getAttribute('type'), 'text')
          const copy = rotationDialog.getByRole('button', {
            name: t('Copy value'),
            exact: true,
          })
          await copy.click()
          await rotationDialog
            .getByRole('button', { name: t('Copied'), exact: true })
            .waitFor()
          assert(
            await page.evaluate(
              async (expected) =>
                (await navigator.clipboard.readText()) === expected,
              syntheticSecret
            ),
            'Copy writes the complete synthetic credential value'
          )
          await capture('fresh-secret-masked')
          const saved = rotationDialog.getByRole('checkbox')
          assert.equal(await saved.count(), 1)
          await saved.focus()
          await page.keyboard.press('Space')
          const done = rotationDialog.getByRole('button', {
            name: t('Done'),
            exact: true,
          })
          await done.click()
          await rotationDialog.waitFor({ state: 'hidden' })
          assert.equal(
            await page.getByLabel(t('New Key value')).count(),
            0,
            'Closing removes the one-time secret from the rendered UI'
          )
          await maintenanceDialog
            .getByRole('button', {
              name: t('Check and rotate Key'),
              exact: true,
            })
            .click()
          await rotationDialog
            .getByRole('button', {
              name: t('Check rotation eligibility'),
              exact: true,
            })
            .waitFor()
          assert.equal(
            await rotationDialog.getByLabel(t('New Key value')).count(),
            0,
            'Reopening cannot restore the one-time secret'
          )
          await rotationDialog
            .getByRole('button', { name: t('Close'), exact: true })
            .first()
            .click()
          assert.deepEqual(counts, {
            read: 2,
            preview: 1,
            verify: 1,
            commit: 1,
            receipt: 0,
          })
        } else {
          await rotationDialog
            .getByText(t('Rotation result is unknown'), { exact: true })
            .waitFor()
          assert.equal(counts.commit, 1)
          assert.equal(counts.receipt, 1)
          await capture('unknown-receipt-missing')
          await rotationDialog
            .getByRole('button', { name: t('Close'), exact: true })
            .first()
            .click()
          await rotationDialog.waitFor({ state: 'hidden' })

          const reopenedReceipt = page.waitForResponse(
            (response) =>
              response.request().method() === 'GET' &&
              new URL(response.url()).pathname.startsWith(
                '/api/token/21/credential-rotation/operations/'
              )
          )
          await maintenanceDialog
            .getByRole('button', {
              name: t('Review rotation status'),
              exact: true,
            })
            .click()
          await reopenedReceipt
          await rotationDialog
            .getByText(t('Rotation result is unknown'), { exact: true })
            .waitFor()
          assert.equal(counts.commit, 1)
          assert.equal(counts.receipt, 2)
          await capture('unknown-reopened')

          await page.reload({ waitUntil: 'networkidle' })
          const reloadedMaintenance = page.waitForResponse(
            (response) =>
              response.request().method() === 'GET' &&
              new URL(response.url()).pathname === '/api/token/21/maintenance'
          )
          const reloadedReceipt = page.waitForResponse(
            (response) =>
              response.request().method() === 'GET' &&
              new URL(response.url()).pathname.startsWith(
                '/api/token/21/credential-rotation/operations/'
              )
          )
          const reloadedEntry = page
            .getByRole('button', {
              name: t('Credential maintenance'),
              exact: true,
            })
            .first()
          await reloadedEntry.click()
          await reloadedMaintenance
          await reloadedReceipt
          const reloadedMaintenanceDialog = page.getByRole('dialog', {
            name: t('Credential maintenance'),
            exact: true,
          })
          await reloadedMaintenanceDialog
            .getByRole('button', {
              name: t('Review rotation status'),
              exact: true,
            })
            .click()
          const reloadedRotationDialog = page.getByRole('dialog', {
            name: t('Credential rotation'),
            exact: true,
          })
          await reloadedRotationDialog
            .getByText(t('Rotation result is unknown'), { exact: true })
            .waitFor()
          assert.equal(counts.commit, 1)
          assert.equal(counts.preview, 1)
          assert.equal(counts.receipt, 3)
          assert(
            receiptOperationIds.every(
              (operationId) => operationId === submittedOperationId
            ),
            'Every recovery read uses the original operation'
          )
          await capture('unknown-reloaded')
          assert.deepEqual(counts, {
            read: 2,
            preview: 1,
            verify: 1,
            commit: 1,
            receipt: 3,
          })
        }
      }
    }

    assert.equal(violations.length, 0, violations.join('\n'))
    assert.equal(pageErrors.length, 0, pageErrors.join('\n'))
    report.journeys.push({
      name: scenario.name,
      language: scenario.language,
      width: scenario.width,
      theme: scenario.theme,
      result: scenario.result,
      synthetic: true,
      counts,
      unknownSameOperation:
        scenario.result === 'unknown'
          ? receiptOperationIds.length === 3 &&
            receiptOperationIds.every(
              (operationId) => operationId === submittedOperationId
            )
          : null,
      externalRequests: violations.filter((value) =>
        value.startsWith('External request:')
      ).length,
      pageErrors: pageErrors.length,
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
