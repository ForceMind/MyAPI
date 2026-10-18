// Compiled personal-console maintenance with isolated synthetic APIs only.
// Same-user SID replacement and role/user changes are covered by transport/UI tests;
// these browser journeys protect normal actions and navigation during a write.
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
  process.env.MYAPI_BROWSER_ARTIFACTS || '.local-tests/maintenance-session'
)
const version = readFileSync(resolve(repo, 'VERSION'), 'utf8').trim()
const head = execFileSync('git', ['rev-parse', 'HEAD'], {
  cwd: repo,
  encoding: 'utf8',
}).trim()
const report = {
  head,
  version,
  workflowHead: process.env.GITHUB_SHA || null,
  scope:
    'Actual production frontend with synthetic APIs; single deletion, budget policy and Key assigned access. No real credentials, database, providers, financial recovery or deployment.',
  passed: false,
  journeys: [],
  errors: [],
}
const persist = () =>
  writeFileSync(
    resolve(output, 'maintenance-session-qualification.json'),
    `${JSON.stringify(report, null, 2)}\n`
  )
const mime = {
  '.html': 'text/html',
  '.js': 'text/javascript',
  '.css': 'text/css',
  '.png': 'image/png',
  '.svg': 'image/svg+xml',
  '.woff2': 'font/woff2',
}
const server = createServer((request, response) => {
  let file
  try {
    file = resolve(
      build,
      `.${decodeURIComponent(new URL(request.url, 'http://localhost').pathname)}`
    )
  } catch {
    return response.writeHead(400).end()
  }
  if (!file.startsWith(`${build}${sep}`) && file !== build) {
    return response.writeHead(403).end()
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

assert(
  existsSync(resolve(build, 'index.html')),
  'Build the actual frontend first'
)
mkdirSync(output, { recursive: true })
let browser
try {
  await new Promise((done) => server.listen(0, '127.0.0.1', done))
  const origin = `http://127.0.0.1:${server.address().port}`
  const modulePath = process.env.MYAPI_PLAYWRIGHT_MODULE
  assert(modulePath, 'Use the isolated Actions Playwright installation')
  const { chromium } = await import(pathToFileURL(modulePath).href)
  browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })
  for (const { language, width, height } of [
    { language: 'en', width: 1440, height: 1000 },
    { language: 'zh', width: 390, height: 900 },
  ]) {
    const texts = JSON.parse(
      readFileSync(
        resolve(repo, `web/src/i18n/locales/${language}.json`),
        'utf8'
      )
    ).translation
    const t = (key) => texts[key] || key
    for (const action of ['delete', 'budget', 'policy']) {
      const actionSpec = {
        delete: {
          path: '/api/token/21',
          method: 'DELETE',
          title: 'Are you sure?',
        },
        budget: {
          path: '/api/token/21/budget',
          method: 'PUT',
          title: 'API Key usage budgets',
        },
        policy: {
          path: '/api/access-policy/token/21',
          method: 'PUT',
          title: 'Assigned access',
        },
      }[action]
      for (const outcome of ['success', 'navigate-away']) {
        const name = `maintenance-${action}-${outcome}-${language}-${width}`
        const context = await browser.newContext({
          viewport: { width, height },
          locale: language === 'zh' ? 'zh-CN' : 'en-US',
          serviceWorkers: 'block',
          reducedMotion: 'reduce',
          colorScheme: 'light',
          timezoneId: 'Asia/Shanghai',
        })
        await context.addCookies([
          { name: 'vite-ui-theme', value: 'light', url: origin },
        ])
        await context.addInitScript(
          ({ language }) => {
            localStorage.setItem('i18nextLng', language)
            localStorage.setItem('theme', 'light')
          },
          { language: language === 'zh' ? 'zhCN' : 'en' }
        )
        const fixture = createUIFixture({
          language: language === 'zh' ? 'zhCN' : 'en',
          personalPolicy: true,
        })
        const budget = {
          policy: {
            token_id: 21,
            user_id: 1,
            enabled: false,
            account_threshold_enabled: false,
            account_min_remaining_bps: 2000,
            account_max_age_seconds: 300,
            fee_enabled: false,
            fee_limit_usd: '0',
            fee_used_usd: '0',
            fee_reserved_usd: '0',
            limit: 100,
            used: 0,
            reserved: 0,
            pending_request_id: '',
            revision: 0,
          },
          pending: null,
        }
        const policy = {
          subject: 'token',
          subject_id: 21,
          owner_user_id: 1,
          revision: 0,
          assigned: false,
          enabled: true,
          public_models: null,
          upstream_models: null,
          channel_ids: null,
        }
        const writes = []
        const unexpected = []
        const pageErrors = []
        let refreshes = 0
        let writeDisposition = 'not-submitted'
        let markStarted
        const writeStarted = new Promise((done) => {
          markStarted = done
        })
        let releaseWrite
        const writeGate = new Promise((done) => {
          releaseWrite = done
        })
        let markHandled
        const handled = new Promise((done) => {
          markHandled = done
        })
        let secretReads = 0
        await context.route('**/*', async (route) => {
          const request = route.request()
          const url = new URL(request.url())
          try {
            assert.equal(url.origin, origin, 'No external traffic')
            const path = url.pathname.replace(/\/$/, '')
            const method = request.method()
            if (path === '/api/user/auth/refresh') refreshes += 1
            if (path === '/api/token/21/key' && method === 'POST') {
              secretReads += 1
              return await route.fulfill({
                json: {
                  success: true,
                  data: { key: 'synthetic-maintenance-only' },
                },
              })
            }
            if (
              path === '/api/token/21/subscription-share' &&
              method === 'GET'
            ) {
              return await route.fulfill({
                json: { success: true, data: { token_id: 21, pools: [] } },
              })
            }
            if (path === '/api/token/21/access' && method === 'GET') {
              return await route.fulfill({
                json: {
                  success: true,
                  data: {
                    token_id: 21,
                    assigned: false,
                    user_revision: 0,
                    token_revision: 0,
                    models: [],
                  },
                },
              })
            }
            if (
              method === 'GET' &&
              (path === '/api/token/21/budget' ||
                path === '/api/access-policy/token/21')
            ) {
              return await route.fulfill({
                json: {
                  success: true,
                  data: path.endsWith('/budget') ? budget : policy,
                },
              })
            }
            if (path === actionSpec.path && method === actionSpec.method) {
              writes.push({ method, path })
              markStarted()
              assert.equal(writes.length, 1, 'One explicit submission only')
              if (outcome === 'navigate-away') {
                await writeGate
                if (action !== 'delete') {
                  assert(
                    request.failure(),
                    'Navigation canceled the scoped request'
                  )
                  writeDisposition = 'navigation-aborted'
                  markHandled()
                  return
                }
                try {
                  await route.fulfill({
                    status: 401,
                    json: {
                      success: false,
                      message: 'Synthetic expired session',
                    },
                  })
                  writeDisposition = '401-delivered'
                } finally {
                  markHandled()
                }
                return
              }
              if (action === 'delete') fixture.state.keys = 'empty'
              if (action === 'budget') {
                const body = request.postDataJSON()
                assert.equal(body.expected_revision, 0)
                assert.equal(body.confirmed, true)
                assert.match(body.id, /^[a-f0-9]{64}$/)
                Object.assign(budget.policy, {
                  enabled: body.enabled,
                  limit: body.limit,
                  revision: 1,
                })
              }
              if (action === 'policy') {
                const body = request.postDataJSON()
                assert.equal(body.expected_revision, 0)
                Object.assign(policy, body, { assigned: true, revision: 1 })
              }
              let data = null
              if (action === 'budget') data = budget
              if (action === 'policy') data = policy
              writeDisposition = '200-delivered'
              return await route.fulfill({ json: { success: true, data } })
            }
            if (!path.startsWith('/api/') && !path.startsWith('/pg/')) {
              return await route.continue()
            }
            const result = fixture.response(url, method)
            assert(!result.violation, result.violation)
            return await route.fulfill({
              status: result.status,
              json: result.body,
            })
          } catch (error) {
            const message = `${name}: ${error.message}`
            unexpected.push(message)
            report.errors.push(message)
            markHandled()
            await route.abort('failed').catch(() => undefined)
          }
        })
        const page = await context.newPage()
        page.setDefaultTimeout(15000)
        page.on('pageerror', (error) => {
          pageErrors.push(error.message)
          report.errors.push(`${name}: ${error.message}`)
        })
        await page.clock.setFixedTime(new Date(FIXTURE_TIME))
        const screenshots = []
        const capture = async (state) => {
          const file = `${name}-${state}.png`
          assert.equal(
            await page.evaluate(
              () => document.documentElement.scrollWidth > innerWidth + 1
            ),
            false,
            'Page fits horizontally'
          )
          await page.screenshot({
            path: resolve(output, file),
            animations: 'disabled',
          })
          screenshots.push(file)
        }
        // This creates a real browser history entry so Back unmounts the form.
        await page.goto(`${origin}/profile`, { waitUntil: 'networkidle' })
        await page
          .getByRole('heading', { name: t('Identity'), exact: true })
          .waitFor()
        const nav = page.getByRole('navigation', {
          name: t(
            width < 768
              ? 'Mobile console navigation'
              : 'Personal console navigation'
          ),
          exact: true,
        })
        await nav
          .getByRole('link', { name: t('API Keys'), exact: true })
          .click()
        const main = page.getByRole('main', {
          name: t('API Keys'),
          exact: true,
        })
        await main.getByText('synthetic-key', { exact: true }).waitFor()
        const revision = await page
          .locator('html')
          .getAttribute('data-build-rev')
        assert(
          revision?.startsWith(
            `rv.${version}.${process.env.GITHUB_SHA || head}.`
          ),
          'Exact candidate build'
        )
        await main.getByText(t('Advanced options'), { exact: true }).click()
        if (action === 'budget') {
          await main
            .getByRole('button', {
              name: t('API Key usage budgets'),
              exact: true,
            })
            .click()
        } else {
          await main
            .getByRole('button', { name: t('Open menu'), exact: true })
            .click()
          await page
            .getByRole('menuitem', {
              name: t(action === 'delete' ? 'Delete' : 'Assigned access'),
              exact: true,
            })
            .click()
        }
        const dialog = page.getByRole(
          action === 'delete' ? 'alertdialog' : 'dialog',
          {
            name: t(actionSpec.title),
            exact: true,
          }
        )
        await dialog.waitFor({ state: 'visible' })
        let submit
        if (action === 'budget') {
          await dialog
            .getByRole('checkbox', {
              name: t(
                'I confirm all running instances support this budget and I understand its request restrictions.'
              ),
              exact: true,
            })
            .check()
          submit = dialog.getByRole('button', { name: t('Save'), exact: true })
        } else {
          submit = dialog.getByRole('button', {
            name: t(action === 'delete' ? 'Delete' : 'Save assignment'),
            exact: true,
          })
        }
        await submit.scrollIntoViewIfNeeded()
        await capture('review')
        const initialRefreshes = refreshes
        const cancelled =
          outcome === 'navigate-away' && action !== 'delete'
            ? page.waitForEvent('requestfailed', {
                predicate: (request) =>
                  request.method() === actionSpec.method &&
                  new URL(request.url()).pathname.replace(/\/$/, '') ===
                    actionSpec.path,
              })
            : null
        const started = page.waitForRequest(
          (request) =>
            request.method() === actionSpec.method &&
            new URL(request.url()).pathname.replace(/\/$/, '') ===
              actionSpec.path
        )
        const pendingResponse =
          outcome === 'navigate-away'
            ? null
            : page.waitForResponse(
                (response) =>
                  response.request().method() === actionSpec.method &&
                  new URL(response.url()).pathname.replace(/\/$/, '') ===
                    actionSpec.path
              )
        await submit.click()
        await Promise.all([started, writeStarted])
        if (outcome === 'navigate-away') {
          // The request adapter is deliberately held until the page unmounts.
          assert.equal(writes.length, 1)
          await page.goBack({ waitUntil: 'domcontentloaded' })
          await page
            .getByRole('heading', { name: t('Identity'), exact: true })
            .waitFor()
          await capture('navigated')
          if (cancelled) await cancelled
          releaseWrite()
          let deadline
          try {
            await Promise.race([
              handled,
              new Promise((_, reject) => {
                deadline = setTimeout(
                  () =>
                    reject(new Error('Held synthetic write did not settle')),
                  15000
                )
              }),
            ])
          } finally {
            clearTimeout(deadline)
          }
          await page.waitForLoadState('networkidle')
          assert.equal(
            refreshes,
            initialRefreshes,
            'Old 401 cannot refresh authentication'
          )
          assert.equal(writes.length, 1, 'Old 401 cannot replay the write')
          assert.equal(
            writeDisposition,
            action === 'delete' ? '401-delivered' : 'navigation-aborted'
          )
          assert.equal(
            await page.locator('[data-sonner-toast]').count(),
            0,
            'No stale success/error notification'
          )
          await dialog.waitFor({ state: 'hidden' })
          assert.equal(await page.getByRole('dialog').count(), 0)
          assert.equal(await page.getByRole('alertdialog').count(), 0)
          await capture('settled')
        } else {
          assert.equal((await pendingResponse).status(), 200)
          if (action === 'delete') {
            await dialog.waitFor({ state: 'hidden' })
            await main
              .getByText(t('No API keys found'), { exact: true })
              .waitFor()
          } else if (action === 'policy') {
            await dialog
              .getByText(t('Assignment saved.'), { exact: true })
              .waitFor()
          } else {
            assert.equal(budget.policy.revision, 1)
            await dialog
              .getByRole('checkbox', {
                name: t(
                  'I confirm all running instances support this budget and I understand its request restrictions.'
                ),
                exact: true,
              })
              .waitFor()
            assert.equal(
              await dialog
                .getByRole('checkbox', {
                  name: t(
                    'I confirm all running instances support this budget and I understand its request restrictions.'
                  ),
                  exact: true,
                })
                .isChecked(),
              false
            )
          }
          await capture('success')
        }
        assert.deepEqual(unexpected, [])
        assert.deepEqual(pageErrors, [])
        assert.equal(
          secretReads,
          action === 'budget' ? 0 : 1,
          'Only the existing action-menu reveal may read the synthetic Key'
        )
        assert.equal(
          await page
            .getByText('sk-synthetic-maintenance-only', { exact: false })
            .count(),
          0,
          'Menu-fetched synthetic secret is never displayed'
        )
        report.journeys.push({
          name,
          language,
          width,
          action,
          outcome,
          revision,
          writes: writes.length,
          initialRefreshes,
          finalRefreshes: refreshes,
          writeDisposition,
          menuTriggeredSecretReads: secretReads,
          screenshots,
          passed: true,
        })
        persist()
        await context.close()
      }
    }
  }
  assert.equal(report.journeys.length, 12)
  assert.deepEqual(report.errors, [])
  report.passed = true
} catch (error) {
  report.failure = String(error.stack || error)
  throw error
} finally {
  persist()
  await browser?.close()
  await new Promise((done) => server.close(done))
}
