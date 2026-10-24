// Production-build model-scope qualification with synthetic APIs only. This
// never reads a real Key secret, account, provider, route, quota, or billing state.
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
    `${repo}/.local-tests/advanced-model-scope-browser`
)
const version = readFileSync(resolve(repo, 'VERSION'), 'utf8').trim()
const sourceHead = execFileSync('git', ['rev-parse', 'HEAD'], {
  cwd: repo,
  encoding: 'utf8',
}).trim()
const languageCodes = {
  en: 'en',
  zh: 'zhCN',
  'zh-TW': 'zhTW',
  fr: 'fr',
  ru: 'ru',
  ja: 'ja',
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
const mime = {
  '.html': 'text/html',
  '.js': 'text/javascript',
  '.css': 'text/css',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.woff2': 'font/woff2',
}
const initialModes = ['unlimited-retained', 'restricted', 'blocked']
const modeByInitialScope = {
  'unlimited-retained': 'unlimited',
  restricted: 'restricted',
  blocked: 'blocked',
}
const modeBySaveAction = {
  'restricted-save': 'restricted',
  'unknown-save': 'restricted',
  'unlimited-save': 'unlimited',
  'blocked-save': 'blocked',
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

const baseScenarios = Object.keys(languageCodes).flatMap((language, index) =>
  [320, 1280].map((width, widthIndex) => ({
    language,
    width,
    theme: 'light',
    initial: initialModes[(index * 2 + widthIndex) % initialModes.length],
    catalog: 'populated',
    action: 'cancel',
  }))
)
const scenario = (language, width, changes) => {
  const target = baseScenarios.find(
    (item) => item.language === language && item.width === width
  )
  assert(target, `Missing scenario ${language}/${width}`)
  Object.assign(target, changes)
}
scenario('en', 1280, {
  initial: 'unlimited-retained',
  action: 'restricted-save',
})
scenario('en', 320, {
  initial: 'restricted',
  catalog: 'empty',
})
scenario('fr', 1280, {
  initial: 'restricted',
  catalog: 'failure',
  failFirstScopeRead: true,
})
scenario('ru', 1280, {
  initial: 'restricted',
  catalog: 'empty',
})
baseScenarios.push(
  {
    language: 'zh',
    width: 1440,
    theme: 'light',
    initial: 'restricted',
    catalog: 'populated',
    action: 'unlimited-save',
  },
  {
    language: 'zh',
    width: 390,
    theme: 'light',
    initial: 'restricted',
    catalog: 'populated',
    action: 'blocked-save',
  },
  {
    language: 'vi',
    width: 320,
    theme: 'dark',
    initial: 'unlimited-retained',
    catalog: 'populated',
    action: 'unknown-save',
  }
)

const report = {
  synthetic: true,
  version,
  sourceHead,
  workflowHead: process.env.GITHUB_SHA || null,
  fixtureTime: new Date(FIXTURE_TIME).toISOString(),
  source:
    'Actual web/dist from this checkout, served locally with createUIFixture and model-scope endpoint overrides.',
  scope:
    'Key model-scope UI and narrow API behavior. Excludes real Key secrets, providers, account migration, credential rotation, quota and billing writes.',
  journeys: [],
  errors: [],
  result: 'running',
}

let browser
let captureFailedCase
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

  for (const item of baseScenarios) {
    const t = (key) => {
      const value = translations[item.language][key]
      assert(value, `Missing ${item.language} translation: ${key}`)
      return value
    }
    const name = `advanced-model-scope-${item.language}-${item.width}-${item.theme}`
    const context = await browser.newContext({
      viewport: { width: item.width, height: 960 },
      reducedMotion: 'reduce',
      colorScheme: item.theme,
      timezoneId: 'Asia/Shanghai',
      serviceWorkers: 'block',
    })
    await context.addCookies([
      { name: 'vite-ui-theme', value: item.theme, url: origin },
    ])
    await context.addInitScript(
      ({ language, theme }) => {
        localStorage.setItem('i18nextLng', language)
        localStorage.setItem('theme', theme)
      },
      { language: languageCodes[item.language], theme: item.theme }
    )

    const fixture = createUIFixture({
      role: 100,
      language: languageCodes[item.language],
      personalPolicy: true,
      longText: item.language === 'fr',
    })
    const retainedModel =
      'retired/provider-model-with-a-name-that-remains-visible-after-catalog-removal'
    const initialScopes = {
      'unlimited-retained': {
        model_limits_enabled: false,
        models: [retainedModel],
      },
      restricted: {
        model_limits_enabled: true,
        models: [retainedModel],
      },
      blocked: { model_limits_enabled: true, models: [] },
    }
    const currentScope = { ...initialScopes[item.initial] }
    let revisionNumber = 10
    const revision = () => revisionNumber.toString(16).padStart(64, '0')
    const scopeView = () => ({
      token_id: 21,
      token_name:
        item.language === 'fr'
          ? 'Synthetic desktop application with a deliberately long descriptive name'
          : 'Synthetic desktop application',
      model_limits_enabled: currentScope.model_limits_enabled,
      models: [...currentScope.models],
      revision: revision(),
    })
    const keyView = () => ({
      id: 21,
      user_id: 1,
      name: 'synthetic-key',
      key: 'masked',
      status: 1,
      remain_quota: 1000,
      used_quota: 0,
      unlimited_quota: false,
      expired_time: -1,
      created_time: FIXTURE_TIME / 1000 - 600,
      accessed_time: FIXTURE_TIME / 1000,
      group: 'default',
      model_limits_enabled: currentScope.model_limits_enabled,
      model_limits: currentScope.models.join(','),
      allow_ips: '',
    })
    const catalogModels = ['synthetic-text-model', 'gpt-5-codex']
    const writes = []
    const unexpected = []
    const requests = [],
      responses = [],
      consoleErrors = []
    let stage = 'opening'
    let scopeReads = 0
    let catalogReads = 0
    let keySecretReads = 0
    let initialScopeFailurePending = item.failFirstScopeRead === true
    let unknownReadbackFailurePending = false

    await context.route('**/*', async (route) => {
      try {
        const request = route.request()
        const url = new URL(request.url())
        if (url.origin !== origin) {
          unexpected.push(`External request: ${url.origin}`)
          await route.abort()
          return
        }
        const path = url.pathname.replace(/\/$/, '')
        if (path.startsWith('/api/') || path.startsWith('/pg/'))
          requests.push({
            method: request.method(),
            path,
            params: Object.fromEntries(url.searchParams),
          })
        if (path === '/api/token/21/key') {
          keySecretReads += 1
          throw new Error('Model-scope journey attempted to read a Key secret')
        }
        if (
          (path === '/api/token' || path === '/api/token/search') &&
          request.method() === 'GET'
        ) {
          return route.fulfill({
            json: {
              success: true,
              data: {
                items: [keyView()],
                total: 1,
                page: 1,
                page_size: 10,
              },
            },
          })
        }
        if (path === '/api/token/21/model-limits') {
          if (request.method() === 'GET') {
            scopeReads += 1
            if (initialScopeFailurePending) {
              initialScopeFailurePending = false
              return route.fulfill({
                status: 503,
                json: {
                  success: false,
                  message: 'Synthetic model-scope read failure',
                },
              })
            }
            if (unknownReadbackFailurePending) {
              unknownReadbackFailurePending = false
              return route.fulfill({
                status: 503,
                json: {
                  success: false,
                  message: 'Synthetic model-scope readback failure',
                },
              })
            }
            return route.fulfill({
              headers: { 'cache-control': 'no-store' },
              json: { success: true, data: scopeView() },
            })
          }
          assert.equal(request.method(), 'PUT')
          const body = request.postDataJSON()
          assert.deepEqual(Object.keys(body).sort(), [
            'expected_revision',
            'model_limits_enabled',
            'models',
          ])
          assert.equal(body.expected_revision, revision())
          assert.equal(typeof body.model_limits_enabled, 'boolean')
          assert(Array.isArray(body.models))
          assert.equal('name' in body, false)
          assert.equal('remain_quota' in body, false)
          assert.equal('quota_version' in body, false)
          assert.equal('metadata_fingerprint' in body, false)
          writes.push(structuredClone(body))
          currentScope.model_limits_enabled = body.model_limits_enabled
          currentScope.models = [...body.models]
          revisionNumber += 1
          if (item.action === 'unknown-save') {
            unknownReadbackFailurePending = true
            return route.fulfill({
              status: 503,
              json: {
                success: false,
                message: 'Synthetic response lost after commit',
              },
            })
          }
          return route.fulfill({
            headers: { 'cache-control': 'no-store' },
            json: { success: true, data: scopeView() },
          })
        }
        if (path === '/api/user/models') {
          assert.equal(request.method(), 'GET')
          assert.equal(url.searchParams.get('group'), 'default')
          assert.deepEqual([...url.searchParams.keys()], ['group'])
          catalogReads += 1
          if (item.catalog === 'failure') {
            return route.fulfill({
              status: 503,
              json: {
                success: false,
                message: 'Synthetic model catalog failure',
              },
            })
          }
          return route.fulfill({
            json: {
              success: true,
              data: item.catalog === 'empty' ? [] : catalogModels,
            },
          })
        }
        if (!path.startsWith('/api/') && !path.startsWith('/pg/')) {
          return route.continue()
        }
        const result = fixture.response(url, request.method())
        if (result.violation) unexpected.push(result.violation)
        assert(!result.violation, result.violation)
        return route.fulfill({ status: result.status, json: result.body })
      } catch (error) {
        const message = `${name}: ${error.message}`
        unexpected.push(message)
        report.errors.push(message)
        await route.abort('failed')
      }
    })

    const page = await context.newPage()
    page.setDefaultTimeout(15000)
    page.on('console', (message) => {
      if (message.type() === 'error') consoleErrors.push(message.text())
    })
    page.on('response', (response) => {
      const url = new URL(response.url())
      if (
        url.origin === origin &&
        (url.pathname.startsWith('/api/') || url.pathname.startsWith('/pg/'))
      )
        responses.push({ path: url.pathname, status: response.status() })
    })
    captureFailedCase = async () => {
      report.failedCase = {
        name,
        ...item,
        stage,
        url: page.url(),
        scopeReads,
        catalogReads,
        keySecretReads,
        writes,
        currentScope: scopeView(),
        requests,
        responses,
        consoleErrors,
        unexpected,
      }
      try {
        const screenshot = `${name}-failed.png`
        await page.screenshot({
          path: resolve(output, screenshot),
          animations: 'disabled',
        })
        report.failedCase.screenshot = screenshot
      } catch (captureError) {
        report.failedCase.captureError = String(captureError)
      }
    }
    page.on('pageerror', (error) =>
      report.errors.push(`${name}: page error: ${error.message}`)
    )
    await page.clock.setFixedTime(new Date(FIXTURE_TIME))
    await page.goto(`${origin}/keys`, { waitUntil: 'networkidle' })
    if (item.theme === 'dark') {
      assert(
        await page
          .locator('html')
          .evaluate((element) => element.classList.contains('dark')),
        `${name}: dark-theme qualification renders the dark root state`
      )
    }

    const keyList = page.getByRole('main', {
      name: t('API Keys'),
      exact: true,
    })
    await keyList.waitFor({ state: 'visible' })
    // Key actions live in the Key inspector: open the Key, then Access.
    await keyList.locator('[data-inspect-id="21"]').first().click()
    await keyList.getByRole('tab', { name: t('Access'), exact: true }).click()
    const entry = keyList
      .getByRole('button', { name: t('Model scope'), exact: true })
      .last()
    await entry.waitFor({ state: 'visible' })
    await entry.focus()
    assert(
      await entry.evaluate((element) => document.activeElement === element),
      `${name}: independent model-scope entry receives keyboard focus`
    )
    await page.keyboard.press('Enter')

    let dialog = page.getByRole('dialog', {
      name: t('Model scope'),
      exact: true,
    })
    await dialog.waitFor({ state: 'visible' })
    let computedDialogColors = null
    if (item.theme === 'dark') {
      computedDialogColors = await dialog.evaluate((element) => {
        const style = getComputedStyle(element)
        const colorSummary = (value) => {
          const channels = (value.match(/[\d.]+/g) ?? [])
            .slice(0, 3)
            .map(Number)
          return {
            value,
            mean:
              channels.length === 3
                ? channels.reduce((sum, channel) => sum + channel, 0) / 3
                : null,
          }
        }
        return {
          background: colorSummary(style.backgroundColor),
          foreground: colorSummary(style.color),
        }
      })
      assert(
        computedDialogColors.background.mean !== null &&
          computedDialogColors.background.mean < 100 &&
          computedDialogColors.foreground.mean !== null &&
          computedDialogColors.foreground.mean > 180,
        `${name}: portal dialog uses readable dark computed colors ${JSON.stringify(
          computedDialogColors
        )}`
      )
    }
    const screenshots = []
    const capture = async (suffix) => {
      const image = `${name}-${suffix}.png`
      await page.screenshot({
        path: resolve(output, image),
        animations: 'disabled',
      })
      screenshots.push(image)
    }
    if (item.failFirstScopeRead) {
      await dialog
        .getByText(t('Could not load model scope'), { exact: true })
        .waitFor({ state: 'visible' })
      await capture('scope-error')
      const retry = dialog.getByRole('button', {
        name: t('Retry'),
        exact: true,
      })
      await retry.focus()
      assert(
        await retry.evaluate((element) => document.activeElement === element),
        `${name}: failed scope read exposes a keyboard-focusable retry`
      )
      await page.keyboard.press('Enter')
    }

    const radios = {
      unlimited: dialog.getByRole('radio', {
        name: t('No additional model limit'),
        exact: false,
      }),
      restricted: dialog.getByRole('radio', {
        name: t('Only selected models'),
        exact: false,
      }),
      blocked: dialog.getByRole('radio', {
        name: t('Block all models'),
        exact: false,
      }),
    }
    const expectedInitialMode = modeByInitialScope[item.initial]
    assert(expectedInitialMode, `${name}: initial scope maps to a UI mode`)
    await radios[expectedInitialMode].waitFor({ state: 'visible' })
    assert(
      await radios[expectedInitialMode].isChecked(),
      `${name}: ${item.initial} backend semantics select ${expectedInitialMode}`
    )
    if (item.initial !== 'blocked') {
      const retainedModelId = dialog.getByText(retainedModel, { exact: true })
      await retainedModelId.waitFor()
      assert.equal(
        await retainedModelId.textContent(),
        retainedModel,
        `${name}: the preserved model ID is rendered without shortening`
      )
      if (item.width <= 390) {
        const retainedGeometry = await retainedModelId.evaluate((element) => {
          const model = element.getBoundingClientRect()
          const row = element.parentElement?.getBoundingClientRect()
          return { modelWidth: model.width, rowWidth: row?.width ?? 0 }
        })
        assert(
          retainedGeometry.rowWidth > 0 &&
            retainedGeometry.modelWidth >= retainedGeometry.rowWidth - 24,
          `${name}: the preserved model ID has a full-width mobile row`
        )
      }
      await dialog
        .getByText(t('Preserved model ID'), { exact: true })
        .first()
        .waitFor()
    }
    if (item.catalog === 'failure') {
      await dialog
        .getByText(t('Could not load the current model catalog'), {
          exact: true,
        })
        .waitFor()
      assert(
        await dialog.getByText(retainedModel, { exact: true }).isVisible(),
        `${name}: a failed catalog keeps the stored unknown model visible`
      )
      await capture('catalog-error-preserved')
    }
    if (item.catalog === 'empty' && item.initial === 'restricted') {
      await dialog
        .getByText(t('No additional catalog models are available'), {
          exact: true,
        })
        .waitFor()
      assert(
        await dialog.getByText(retainedModel, { exact: true }).isVisible(),
        `${name}: an empty catalog keeps the stored unknown model visible`
      )
    }

    const geometry = await dialog.evaluate((element) => {
      const rect = element.getBoundingClientRect()
      return {
        left: rect.left,
        right: rect.right,
        viewport: innerWidth,
        clientWidth: element.clientWidth,
        scrollWidth: element.scrollWidth,
        pageScrollWidth: document.documentElement.scrollWidth,
      }
    })
    assert(
      geometry.left >= -1 &&
        geometry.right <= item.width + 1 &&
        geometry.scrollWidth <= geometry.clientWidth + 1 &&
        geometry.pageScrollWidth <= item.width + 1,
      `${name}: model-scope dialog fits without horizontal clipping`
    )

    await capture('open')
    if (item.width <= 390 && item.initial !== 'blocked') {
      await dialog
        .getByText(retainedModel, { exact: true })
        .scrollIntoViewIfNeeded()
      await capture('mobile-preserved-model')
    }

    let actionGeometry = null
    if (item.width <= 390) {
      const cancel = dialog.getByRole('button', {
        name: t('Cancel'),
        exact: true,
      })
      const save = dialog.getByRole('button', {
        name: t('Save model scope'),
        exact: true,
      })
      await cancel.scrollIntoViewIfNeeded()
      await save.scrollIntoViewIfNeeded()
      // The modal correctly makes the application shell aria-hidden. Locate the
      // still-rendered mobile navigation by its stable layout class so its real
      // geometry can continue to protect the dialog actions from overlap.
      const navigation = page.locator('nav.pc-mobile-nav')
      assert.equal(
        await navigation.count(),
        1,
        `${name}: mobile navigation layout anchor is unique`
      )
      const navigationBox = await navigation.boundingBox()
      assert(navigationBox, `${name}: mobile navigation has rendered geometry`)
      actionGeometry = []
      for (const action of [cancel, save]) {
        const box = await action.boundingBox()
        assert(
          box && box.width > 0 && box.height > 0,
          `${name}: action is visible`
        )
        assert(
          box.x >= 0 &&
            box.x + box.width <= item.width &&
            box.y >= 0 &&
            box.y + box.height <= navigationBox.y &&
            box.y + box.height <= 960,
          `${name}: action remains fully visible above mobile navigation ${JSON.stringify(
            {
              action: box,
              navigation: navigationBox,
              viewport: { width: item.width, height: 960 },
            }
          )}`
        )
        actionGeometry.push(box)
      }
      await capture('mobile-actions')
    }

    const save = dialog.getByRole('button', {
      name: t('Save model scope'),
      exact: true,
    })
    if (item.action === 'restricted-save' || item.action === 'unknown-save') {
      stage = 'selecting-model'
      await radios.restricted.click()
      const search = dialog.getByPlaceholder(t('Search current model catalog'))
      await search.fill('codex')
      const candidate = dialog.getByRole('button', {
        name: 'gpt-5-codex',
        exact: true,
      })
      await candidate.focus()
      assert(
        await candidate.evaluate(
          (element) => document.activeElement === element
        ),
        `${name}: a catalog result receives keyboard focus`
      )
      await page.keyboard.press('Enter')
      await dialog
        .getByRole('button', {
          name: t('Remove {{model}}').replace('{{model}}', 'gpt-5-codex'),
          exact: true,
        })
        .waitFor({ state: 'visible' })
      await candidate.waitFor({ state: 'hidden' })
      stage = 'submitting-model-scope'
      await save.click({ trial: true })
      assert(await save.isEnabled(), `${name}: selected scope is ready to save`)
      await save.focus()
      assert(
        await save.evaluate((element) => document.activeElement === element),
        `${name}: save receives keyboard focus`
      )
      await page.keyboard.press('Enter')
      if (item.action === 'unknown-save') {
        stage = 'awaiting-unknown-save-result'
        await dialog
          .getByText(t('The save result could not be confirmed'), {
            exact: true,
          })
          .waitFor({ state: 'visible' })
        await capture('unknown-readback')
        await radios.blocked.click()
        const confirmation = dialog.getByRole('checkbox', {
          name: t(
            'I understand that this Key will be unable to start model requests.'
          ),
          exact: true,
        })
        await confirmation.click()
        assert(
          await save.isDisabled(),
          `${name}: edits cannot bypass an unresolved readback lock`
        )
        assert.equal(
          writes.length,
          1,
          `${name}: unknown result is never replayed`
        )
        const reload = dialog.getByRole('button', {
          name: t('Reload current scope'),
          exact: true,
        })
        await reload.focus()
        await page.keyboard.press('Enter')
        await dialog
          .getByText('gpt-5-codex', { exact: true })
          .first()
          .waitFor({ state: 'visible' })
        await dialog
          .getByRole('button', { name: t('Cancel'), exact: true })
          .click()
        await dialog.waitFor({ state: 'hidden' })
        assert.equal(
          writes.length,
          1,
          `${name}: response loss and recovery never replay the PUT`
        )
        assert(scopeReads >= 3, `${name}: unknown result requires a fresh GET`)
      } else {
        await dialog.waitFor({ state: 'hidden' })
      }
      assert.equal(writes.length, 1, `${name}: one explicit model-scope write`)
      assert.deepEqual(writes[0], {
        expected_revision: 'a'.padStart(64, '0'),
        model_limits_enabled: true,
        models: [retainedModel, 'gpt-5-codex'],
      })
      assert(
        scopeReads >= 2,
        `${name}: every write is followed by a GET readback`
      )
    } else if (item.action === 'unlimited-save') {
      await radios.unlimited.click()
      await save.focus()
      await page.keyboard.press('Enter')
      await dialog.waitFor({ state: 'hidden' })
      assert.deepEqual(writes, [
        {
          expected_revision: 'a'.padStart(64, '0'),
          model_limits_enabled: false,
          models: [retainedModel],
        },
      ])
      assert(scopeReads >= 2, `${name}: unlimited save is read back`)
    } else if (item.action === 'blocked-save') {
      await radios.blocked.click()
      assert(
        await save.isDisabled(),
        `${name}: block-all requires confirmation`
      )
      await dialog
        .getByText(t('Confirm blocking all models'), { exact: true })
        .waitFor()
      await capture('block-confirmation')
      const confirmation = dialog.getByRole('checkbox', {
        name: t(
          'I understand that this Key will be unable to start model requests.'
        ),
        exact: true,
      })
      await confirmation.focus()
      await page.keyboard.press('Space')
      assert(await confirmation.isChecked())
      await save.click()
      await dialog.waitFor({ state: 'hidden' })
      assert.deepEqual(writes, [
        {
          expected_revision: 'a'.padStart(64, '0'),
          model_limits_enabled: true,
          models: [],
        },
      ])
      assert(scopeReads >= 2, `${name}: deny-all save is read back`)
    } else {
      const writesBeforeCancel = writes.length
      await dialog
        .getByRole('button', { name: t('Cancel'), exact: true })
        .click()
      await dialog.waitFor({ state: 'hidden' })
      assert.equal(
        writes.length,
        writesBeforeCancel,
        `${name}: cancel does not write`
      )
    }

    if (item.action !== 'cancel') {
      const refreshedEntry = keyList
        .getByRole('button', { name: t('Model scope'), exact: true })
        .last()
      await refreshedEntry.click()
      dialog = page.getByRole('dialog', {
        name: t('Model scope'),
        exact: true,
      })
      await dialog.waitFor({ state: 'visible' })
      const expectedSavedMode = modeBySaveAction[item.action]
      assert(expectedSavedMode, `${name}: save action maps to a UI mode`)
      assert(
        await dialog.locator(`#model-scope-${expectedSavedMode}`).isChecked(),
        `${name}: reopened dialog reflects the confirmed server scope`
      )
      await capture('readback')
      await dialog
        .getByRole('button', { name: t('Cancel'), exact: true })
        .click()
      assert.equal(
        writes.length,
        1,
        `${name}: readback and cancel do not replay PUT`
      )
    }

    const buildRevision = await page
      .locator('html')
      .getAttribute('data-build-rev')
    assert(
      buildRevision?.startsWith(`rv.${version}.`),
      `${name}: actual build matches candidate version`
    )
    if (process.env.GITHUB_SHA) {
      assert(
        buildRevision.includes(process.env.GITHUB_SHA),
        `${name}: actual build matches the CI checkout`
      )
    }
    assert.equal(keySecretReads, 0, `${name}: no Key secret read`)
    assert.deepEqual(unexpected, [])
    report.journeys.push({
      name,
      language: item.language,
      languageCode: languageCodes[item.language],
      width: item.width,
      theme: item.theme,
      initial: item.initial,
      catalog: item.catalog,
      action: item.action,
      buildRevision,
      scopeReads,
      catalogReads,
      writes: writes.length,
      keySecretReads,
      exactNarrowWrites: writes.every(
        (body) =>
          Object.keys(body).sort().join(',') ===
          'expected_revision,model_limits_enabled,models'
      ),
      geometry,
      actionGeometry,
      computedDialogColors,
      screenshots,
      passed: true,
    })
    await context.close()
    captureFailedCase = undefined
  }

  assert.equal(report.journeys.length, 17)
  assert.deepEqual(report.errors, [])
  assert(
    report.journeys.some(
      (journey) => journey.initial === 'unlimited-retained'
    ) &&
      report.journeys.some((journey) => journey.initial === 'restricted') &&
      report.journeys.some((journey) => journey.initial === 'blocked'),
    'All three model_limits_enabled/list semantics are qualified'
  )
  report.result = 'passed'
} catch (error) {
  await captureFailedCase?.()
  report.result = 'failed'
  report.failure = String(error.stack || error)
  throw error
} finally {
  writeFileSync(
    resolve(output, 'advanced-model-scope-qualification.json'),
    `${JSON.stringify(report, null, 2)}\n`
  )
  await browser?.close()
  await new Promise((done) => server.close(done))
}
