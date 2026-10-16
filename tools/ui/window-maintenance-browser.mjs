// Production-build qualification for advanced subscription-window maintenance.
// Every API response is synthetic. This never contacts a live backend,
// provider account, credential store, or subscription.
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
    `${repo}/.local-tests/advanced-window-browser`
)
const version = readFileSync(resolve(repo, 'VERSION'), 'utf8').trim()
const sourceHead = execFileSync('git', ['rev-parse', 'HEAD'], {
  cwd: repo,
  encoding: 'utf8',
}).trim()
const languages = {
  en: 'en',
  zh: 'zhCN',
  'zh-TW': 'zhTW',
  fr: 'fr',
  ru: 'ru',
  ja: 'ja',
  vi: 'vi',
}
const mime = {
  '.html': 'text/html',
  '.js': 'text/javascript',
  '.css': 'text/css',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.woff2': 'font/woff2',
}
const currentConnection =
  'Synthetic current connection · primary entitlement window · 当前连接'
const replacementConnection =
  'Synthetic replacement sample connection · same provider slot · 候选连接'
const laterReplacementConnection =
  'Synthetic later replacement sample · server-qualified only · 后续候选'

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

const now = FIXTURE_TIME / 1000
const basePool = {
  pool_id: 7,
  snapshot_id: 73,
  revision: 4,
  channel_id: 9,
  channel_name: currentConnection,
  window_source: 'codex_wham_usage_primary',
  plan_type: 'plus',
  window_slot: 'primary',
  window_type: 'five_hour',
  window_seconds: 18_000,
  reset_at: now + 3_600,
  observed_at: now - 30,
  max_age_seconds: 300,
  sample_age_seconds: 30,
  state: 'discontinuity',
  reason: 'observation_discontinuity',
  allocated_bps: 7_000,
  share_bps: 2_000,
  estimated_used_micro: 5_000_000,
  unattributed_micro: 1_000_000,
  pending_attempts: 2,
  unsampled_attempts: 1,
  uncertain_attempts: 3,
  approximate: true,
  baseline_time: now - 600,
  replacement_required: false,
  rebaseline_required: true,
}
const initialReplacement = {
  ...basePool,
  snapshot_id: 71,
  channel_id: 11,
  channel_name: replacementConnection,
  window_type: 'weekly',
  window_seconds: 604_800,
  reset_at: now + 86_400,
  state: 'candidate',
  reason: 'fresh_qualified_sample',
  estimated_used_micro: 0,
  unattributed_micro: 0,
  pending_attempts: 0,
  unsampled_attempts: 0,
  uncertain_attempts: 0,
  baseline_time: 0,
  replacement_required: true,
  rebaseline_required: false,
}

function poolsForPhase(phase) {
  if (phase === 'initial') return [initialReplacement, basePool]
  const replacementGeneration = {
    ...initialReplacement,
    snapshot_id: 74,
    revision: 5,
    state: 'discontinuity',
    reason: 'observation_discontinuity',
    estimated_used_micro: 1_250_000,
    unattributed_micro: 250_000,
    pending_attempts: 1,
    unsampled_attempts: 1,
    uncertain_attempts: 2,
    baseline_time: now - 120,
    replacement_required: false,
    rebaseline_required: true,
  }
  if (phase === 'after-replacement') return [replacementGeneration]
  const recovered = {
    ...replacementGeneration,
    revision: 6,
    state: 'active',
    reason: 'eligible',
    rebaseline_required: false,
  }
  if (phase === 'closed') {
    return [
      {
        ...recovered,
        revision: 7,
        state: 'closed',
        reason: 'pool_closed',
      },
    ]
  }
  return [
    {
      ...recovered,
      snapshot_id: 75,
      channel_id: 13,
      channel_name: laterReplacementConnection,
      window_type: 'monthly',
      window_seconds: 2_592_000,
      reset_at: now + 172_800,
      state: 'candidate',
      reason: 'fresh_qualified_sample',
      estimated_used_micro: 0,
      unattributed_micro: 0,
      pending_attempts: 0,
      unsampled_attempts: 0,
      uncertain_attempts: 0,
      baseline_time: 0,
      replacement_required: true,
    },
    recovered,
  ]
}

function translation(language) {
  const translations = JSON.parse(
    readFileSync(
      resolve(repo, `web/src/i18n/locales/${language}.json`),
      'utf8'
    )
  ).translation
  return (key) => {
    assert(translations[key], `Missing ${language} key: ${key}`)
    return translations[key]
  }
}

async function assertFits(locator, width, label) {
  const geometry = await locator.evaluate((element) => {
    const rect = element.getBoundingClientRect()
    return {
      left: rect.left,
      right: rect.right,
      viewport: innerWidth,
      scrollWidth: element.scrollWidth,
      clientWidth: element.clientWidth,
      pageScrollWidth: document.documentElement.scrollWidth,
    }
  })
  assert(
    geometry.left >= -1 &&
      geometry.right <= width + 1 &&
      geometry.scrollWidth <= geometry.clientWidth + 1 &&
      geometry.pageScrollWidth <= width + 1,
    `${label}: content fits without horizontal clipping`
  )
  return geometry
}

async function assertActionVisible(page, action, width, height, label, t) {
  await action.scrollIntoViewIfNeeded()
  const geometry = await action.evaluate((element) => {
    const rect = element.getBoundingClientRect()
    const top = document.elementFromPoint(
      rect.left + rect.width / 2,
      rect.top + rect.height / 2
    )
    return {
      x: rect.x,
      y: rect.y,
      width: rect.width,
      height: rect.height,
      unobscured: top === element || element.contains(top),
    }
  })
  assert(
    geometry.width > 0 &&
      geometry.height > 0 &&
      geometry.x >= 0 &&
      geometry.x + geometry.width <= width &&
      geometry.y >= 0 &&
      geometry.y + geometry.height <= height &&
      geometry.unobscured,
    `${label}: action is visible and unobscured`
  )
  if (width <= 390) {
    const navigation = page.getByRole('navigation', {
      name: t('Mobile console navigation'),
      exact: true,
    })
    const navigationBox = await navigation.boundingBox()
    assert(navigationBox, `${label}: mobile navigation is visible`)
    assert(
      geometry.y + geometry.height <= navigationBox.y + 1,
      `${label}: action stays above mobile navigation`
    )
  }
  return geometry
}

const report = {
  synthetic: true,
  liveBackend: false,
  scope:
    'Compiled frontend with synthetic API contracts; no live account, credential, provider, or production data is read or changed.',
  version,
  sourceHead,
  fixtureTime: new Date(FIXTURE_TIME).toISOString(),
  journeys: [],
  errors: [],
}

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

  const viewports = Object.keys(languages).flatMap((language) => [
    {
      language,
      width: 1280,
      height: 1000,
      theme: language === 'en' ? 'dark' : 'light',
      emphasis: false,
    },
    {
      language,
      width: 320,
      height: 900,
      theme: language === 'vi' ? 'dark' : 'light',
      emphasis: language === 'vi',
    },
  ])
  viewports.push(
    {
      language: 'zh',
      width: 1440,
      height: 1000,
      theme: 'light',
      emphasis: true,
    },
    {
      language: 'zh',
      width: 390,
      height: 900,
      theme: 'light',
      emphasis: true,
    }
  )

  for (const scenario of viewports) {
    const { language, width, height, theme } = scenario
    const languageCode = languages[language]
    const t = translation(language)
    const name = `advanced-window-${language}-${width}`
    const functional =
      (language === 'en' && width === 1280) ||
      (language === 'vi' && width === 320)
    const context = await browser.newContext({
      viewport: { width, height },
      reducedMotion: 'reduce',
      colorScheme: theme,
      timezoneId: 'Asia/Shanghai',
      serviceWorkers: 'block',
    })
    const fixture = createUIFixture({
      role: 100,
      language: languageCode,
      personalPolicy: true,
      longText: true,
    })
    const calls = { gets: 0, puts: [], closes: [], authRefreshes: 0 }
    const unexpected = []
    let phase = 'initial'
    let failNextPut = false
    await context.addCookies([
      { name: 'vite-ui-theme', value: theme, url: origin },
    ])
    await context.addInitScript((code) => {
      localStorage.setItem('i18nextLng', code)
    }, languageCode)
    await context.route('**/*', async (route) => {
      try {
        const request = route.request()
        const url = new URL(request.url())
        if (url.origin !== origin) {
          unexpected.push(`external:${url.origin}`)
          return route.abort()
        }
        if (url.pathname === '/api/user/auth/refresh') {
          calls.authRefreshes++
        }
        if (url.pathname === '/api/token/21/subscription-share') {
          if (request.method() === 'GET') {
            calls.gets++
            return route.fulfill({
              json: {
                success: true,
                data: { token_id: 21, pools: poolsForPhase(phase) },
              },
            })
          }
          assert.equal(request.method(), 'PUT')
          const body = request.postDataJSON()
          assert.match(body.id, /^[a-f0-9]{64}$/)
          assert.equal(body.pool_id, 7)
          assert.equal(body.share_bps, 2000)
          assert.equal(body.confirmed_approximate, true)
          assert.equal(body.replace_window, true)
          if (failNextPut) {
            failNextPut = false
            assert.equal(phase, 'after-recovery')
            assert.equal(body.snapshot_id, 75)
            assert.equal(body.expected_revision, 6)
            calls.puts.push({ phase, outcome: 'unknown-401', body })
            return route.fulfill({
              status: 401,
              json: { success: false, message: 'Synthetic expired session' },
            })
          }
          if (phase === 'initial') {
            assert.equal(body.snapshot_id, 71)
            assert.equal(body.expected_revision, 4)
            calls.puts.push({ phase, outcome: 'accepted', body })
            phase = 'after-replacement'
          } else {
            assert.equal(phase, 'after-replacement')
            assert.equal(body.snapshot_id, 74)
            assert.equal(body.expected_revision, 5)
            calls.puts.push({ phase, outcome: 'accepted', body })
            phase = 'after-recovery'
          }
          return route.fulfill({
            json: {
              success: true,
              data: { token_id: 21, pools: poolsForPhase(phase) },
            },
          })
        }
        if (url.pathname === '/api/subscription-share/pools/7/close') {
          assert.equal(request.method(), 'POST')
          assert.equal(phase, 'after-recovery')
          const body = request.postDataJSON()
          assert.match(body.id, /^[a-f0-9]{64}$/)
          assert.equal(body.expected_revision, 6)
          assert.equal(body.confirmed_close, true)
          calls.closes.push(body)
          phase = 'closed'
          return route.fulfill({ json: { success: true, data: null } })
        }
        if (!url.pathname.startsWith('/api/') && !url.pathname.startsWith('/pg/')) {
          return route.continue()
        }
        const result = fixture.response(url, request.method())
        if (result.violation) unexpected.push(result.violation)
        assert(!result.violation, result.violation)
        return route.fulfill({ status: result.status, json: result.body })
      } catch (error) {
        const message = `${name}: synthetic API contract failed: ${error.message}`
        unexpected.push(message)
        report.errors.push(message)
        return route.abort('failed')
      }
    })

    const page = await context.newPage()
    page.setDefaultTimeout(15_000)
    page.on('pageerror', (error) => report.errors.push(`${name}: ${error.message}`))
    page.on('console', (message) => {
      if (message.type() === 'error') {
        report.errors.push(`${name}: ${message.text()}`)
      }
    })
    await page.clock.setFixedTime(new Date(FIXTURE_TIME))

    const screenshots = []
    const actionGeometry = []
    const openEditor = async (navigate = false) => {
      if (navigate) {
        await page.goto(`${origin}/keys`, { waitUntil: 'networkidle' })
        const keyList = page.getByRole('main', {
          name: t('API Keys'),
          exact: true,
        })
        await keyList.waitFor({ state: 'visible' })
        await assertFits(keyList, width, `${name}: Key list`)
      }
      await page
        .getByRole('button', { name: t('Window share'), exact: true })
        .last()
        .click()
      const editor = page.getByRole('main', {
        name: t('Adjust window share'),
        exact: true,
      })
      await editor.waitFor({ state: 'visible' })
      return editor
    }
    const selectWindow = async (editor, accountId, selection) => {
      await editor.locator('#personal-share-account').selectOption(accountId)
      await editor.locator('#personal-share-window').selectOption(selection)
    }
    const confirmWindowChange = async (editor, actionName) => {
      const action = editor.getByRole('button', {
        name: actionName,
        exact: true,
      })
      const windowConfirmation = editor.locator(
        '#personal-share-window-confirmation'
      )
      const approximationConfirmation = editor.locator(
        '#personal-share-confirmation'
      )
      assert(await action.isDisabled(), `${name}: both confirmations start required`)
      await windowConfirmation.click()
      assert(
        await action.isDisabled(),
        `${name}: one confirmation cannot enable the action`
      )
      await approximationConfirmation.click()
      assert(await action.isEnabled(), `${name}: both confirmations enable the action`)
      return action
    }
    const screenshot = async (suffix) => {
      const file = `${name}-${suffix}.png`
      await page.screenshot({
        path: resolve(output, file),
        fullPage: true,
        animations: 'disabled',
      })
      screenshots.push(file)
    }

    let editor = await openEditor(true)
    const revision = await page.locator('html').getAttribute('data-build-rev')
    assert(revision?.startsWith(`rv.${version}.`), 'Actual build matches candidate version')
    if (process.env.GITHUB_SHA) {
      assert(revision.includes(process.env.GITHUB_SHA), 'Actual build matches CI checkout')
    }
    const resolvedTheme = await page.locator('html').evaluate((element) =>
      element.classList.contains('dark') ? 'dark' : 'light'
    )
    assert.equal(
      resolvedTheme,
      theme,
      `${name}: stored ${theme} theme is applied to the document root`
    )

    await selectWindow(editor, '11', '7:71')
    const replacementReview = editor.getByRole('region', {
      name: t('Window replacement review'),
      exact: true,
    })
    await replacementReview.waitFor({ state: 'visible' })
    await replacementReview
      .getByText(currentConnection, { exact: false })
      .waitFor({ state: 'visible' })
    await replacementReview
      .getByText(replacementConnection, { exact: false })
      .waitFor({ state: 'visible' })
    assert(
      (await replacementReview.innerText()).includes(
        t(
          'Replacement is allowed only when the server verifies the same subscription account and native window slot. Channel names and IDs identify sample connections; they do not permit a cross-account move.'
        )
      ),
      `${name}: replacement boundary is visible`
    )
    assert(
      (await replacementReview.innerText()).includes(
        t(
          'Replacement starts a new tracking generation. Existing Key shares and prior window history stay with the pool. Current estimated usage does not carry into the new generation, and requests wait for a fresh baseline.'
        )
      ),
      `${name}: replacement generation semantics are visible`
    )
    await assertFits(editor, width, `${name}: replacement editor`)
    const replacementAction = editor.getByRole('button', {
      name: t('Replace window'),
      exact: true,
    })
    assert(await replacementAction.isDisabled(), `${name}: replacement starts disabled`)
    actionGeometry.push(
      await assertActionVisible(
        page,
        replacementAction,
        width,
        height,
        `${name}: replacement`,
        t
      )
    )
    await screenshot('replacement')

    await selectWindow(editor, '9', '7:73')
    const recoveryReview = editor.getByRole('region', {
      name: t('Observation recovery review'),
      exact: true,
    })
    await recoveryReview.waitFor({ state: 'visible' })
    const currentRecoveryWindow = recoveryReview.getByRole('article', {
      name: t('Current managed window'),
      exact: true,
    })
    const currentRecoverySnapshot = currentRecoveryWindow
      .getByText(t('Snapshot'), { exact: true })
      .locator('..')
    assert(
      (await currentRecoverySnapshot.innerText()).includes(t('Unknown')),
      `${name}: unavailable prior recovery sample is not invented`
    )
    const recoverySample = recoveryReview.getByRole('article', {
      name: t('Recovery sample'),
      exact: true,
    })
    await recoverySample.getByText('#73', { exact: true }).waitFor({
      state: 'visible',
    })
    const recoveryConnection = recoverySample
      .getByText(t('Sample connection'), { exact: true })
      .locator('..')
    assert(
      (await recoveryConnection.innerText()).includes(t('Unknown')),
      `${name}: unexposed recovery connection is not inferred`
    )
    assert(
      (await recoveryReview.innerText()).includes(
        t(
          'Recovery keeps the current window descriptor, tracking generation, and estimated usage. Unresolved requests remain uncertain while observation resumes from this fresh sample.'
        )
      ),
      `${name}: recovery preservation semantics are visible`
    )
    const recoveryAction = editor.getByRole('button', {
      name: t('Resume observation'),
      exact: true,
    })
    assert(await recoveryAction.isDisabled(), `${name}: recovery starts disabled`)
    actionGeometry.push(
      await assertActionVisible(
        page,
        recoveryAction,
        width,
        height,
        `${name}: recovery`,
        t
      )
    )
    await screenshot('recovery')

    if (functional) {
      await selectWindow(editor, '11', '7:71')
      await confirmWindowChange(editor, t('Replace window'))
      const replacementPutCount = calls.puts.length
      await editor.locator('#personal-share-percentage').focus()
      await page.keyboard.press('Enter')
      await editor.waitFor({ state: 'hidden' })
      assert.equal(calls.puts.length, replacementPutCount + 1)
      assert.equal(calls.puts.at(-1).phase, 'initial')
      assert.equal(calls.puts.at(-1).body.expected_revision, 4)
      assert.equal(calls.puts.at(-1).body.snapshot_id, 71)

      editor = await openEditor()
      await selectWindow(editor, '11', '7:74')
      const recovery = await confirmWindowChange(
        editor,
        t('Resume observation')
      )
      const recoveryPutCount = calls.puts.length
      await recovery.focus()
      await page.keyboard.press('Enter')
      await editor.waitFor({ state: 'hidden' })
      assert.equal(calls.puts.length, recoveryPutCount + 1)
      assert.equal(calls.puts.at(-1).phase, 'after-replacement')
      assert.equal(calls.puts.at(-1).body.expected_revision, 5)
      assert.equal(calls.puts.at(-1).body.snapshot_id, 74)

      editor = await openEditor()
      await selectWindow(editor, '13', '7:75')
      const unknownAction = await confirmWindowChange(
        editor,
        t('Replace window')
      )
      failNextPut = true
      const unknownPutCount = calls.puts.length
      await unknownAction.focus()
      await page.keyboard.press('Enter')
      const unknownNotice = await editor.getByRole('alert', {
        name: t('The save result is unknown or out of date'),
        exact: true,
      })
      await unknownNotice.waitFor({ state: 'visible' })
      await page.waitForFunction(
        () =>
          document.activeElement?.getAttribute('aria-labelledby') ===
          'personal-share-save-state-title'
      )
      assert(await unknownNotice.evaluate((element) => element === document.activeElement))
      assert.equal(calls.puts.length, unknownPutCount + 1)
      assert.equal(calls.puts.at(-1).outcome, 'unknown-401')
      assert.equal(calls.authRefreshes, 0, 'A stale 401 cannot refresh or replay')
      await screenshot('unknown-result')
      const readsBeforeReload = calls.gets
      await page.keyboard.press('Tab')
      const reload = editor.getByRole('button', {
        name: t('Reload current state'),
        exact: true,
      })
      assert(await reload.evaluate((element) => element === document.activeElement))
      await page.keyboard.press('Enter')
      await editor
        .getByText(
          t(
            'Current server state reloaded. Review the window and confirmations before taking another action.'
          ),
          { exact: true }
        )
        .waitFor({ state: 'visible' })
      assert(calls.gets > readsBeforeReload, 'Unknown result requires an explicit reread')
      assert.equal(calls.puts.length, unknownPutCount + 1, 'Unknown write is never repeated')
      assert.equal(calls.authRefreshes, 0)

      await editor
        .getByRole('button', {
          name: t('Advanced window controls'),
          exact: true,
        })
        .click()
      const advanced = page.getByRole('dialog', {
        name: t('Subscription shares'),
        exact: true,
      })
      const closeSection = advanced.getByRole('region', {
        name: t('Close managed pool'),
        exact: true,
      })
      await closeSection.waitFor({ state: 'visible' })
      assert.equal(
        await closeSection.getByRole('button', { name: t('Close pool') }).count(),
        1,
        'Pool closing has one independent action'
      )
      const putsBeforeClose = calls.puts.length
      await closeSection
        .getByRole('button', { name: t('Close pool'), exact: true })
        .click()
      const closeDialog = page.getByRole('alertdialog', {
        name: t('Close this subscription pool?'),
        exact: true,
      })
      const closeAction = closeDialog.getByRole('button', {
        name: t('Close pool'),
        exact: true,
      })
      assert(await closeAction.isDisabled(), 'Closing has its own confirmation')
      await closeDialog
        .getByRole('checkbox', {
          name: t('I understand this pool cannot be enabled again in this version.'),
          exact: true,
        })
        .click()
      await closeAction.click()
      await closeDialog.waitFor({ state: 'hidden' })
      await advanced
        .getByText(t('Closed — history retained'), { exact: true })
        .waitFor({ state: 'visible' })
      assert.equal(calls.closes.length, 1)
      assert.equal(calls.closes[0].expected_revision, 6)
      assert.equal(calls.puts.length, putsBeforeClose)
      await screenshot('independent-close')
    }

    assert.deepEqual(unexpected, [])
    report.journeys.push({
      name,
      language,
      width,
      height,
      theme: resolvedTheme,
      emphasis: scenario.emphasis,
      functional,
      buildRevision: revision,
      phase,
      calls,
      actionGeometry,
      screenshots,
      passed: true,
    })
    await context.close()
  }

  const ownerLanguage = 'en'
  const ownerCode = languages[ownerLanguage]
  const ownerT = translation(ownerLanguage)
  const ownerName = 'advanced-window-en-320-owner-read-only'
  const ownerContext = await browser.newContext({
    viewport: { width: 320, height: 900 },
    reducedMotion: 'reduce',
    colorScheme: 'light',
    timezoneId: 'Asia/Shanghai',
    serviceWorkers: 'block',
  })
  const ownerFixture = createUIFixture({
    role: 1,
    language: ownerCode,
    personalPolicy: true,
    longText: true,
  })
  const ownerUnexpected = []
  await ownerContext.addCookies([
    { name: 'vite-ui-theme', value: 'light', url: origin },
  ])
  await ownerContext.addInitScript((code) => {
    localStorage.setItem('i18nextLng', code)
  }, ownerCode)
  await ownerContext.route('**/*', async (route) => {
    try {
      const request = route.request()
      const url = new URL(request.url())
      if (url.origin !== origin) {
        ownerUnexpected.push(`external:${url.origin}`)
        return route.abort()
      }
      if (url.pathname === '/api/token/21/subscription-share') {
        assert.equal(request.method(), 'GET', 'Owner subscription share is read-only')
        return route.fulfill({
          json: {
            success: true,
            data: {
              token_id: 21,
              pools: [
                {
                  ...basePool,
                  channel_id: undefined,
                  channel_name: '',
                  state: 'active',
                  reason: 'eligible',
                  rebaseline_required: false,
                },
              ],
            },
          },
        })
      }
      if (!url.pathname.startsWith('/api/') && !url.pathname.startsWith('/pg/')) {
        return route.continue()
      }
      const result = ownerFixture.response(url, request.method())
      if (result.violation) ownerUnexpected.push(result.violation)
      assert(!result.violation, result.violation)
      return route.fulfill({ status: result.status, json: result.body })
    } catch (error) {
      const message = `${ownerName}: synthetic API contract failed: ${error.message}`
      ownerUnexpected.push(message)
      report.errors.push(message)
      return route.abort('failed')
    }
  })
  const ownerPage = await ownerContext.newPage()
  ownerPage.setDefaultTimeout(15_000)
  ownerPage.on('pageerror', (error) =>
    report.errors.push(`${ownerName}: ${error.message}`)
  )
  ownerPage.on('console', (message) => {
    if (message.type() === 'error') {
      report.errors.push(`${ownerName}: ${message.text()}`)
    }
  })
  await ownerPage.clock.setFixedTime(new Date(FIXTURE_TIME))
  await ownerPage.goto(`${origin}/keys`, { waitUntil: 'networkidle' })
  const ownerTheme = await ownerPage.locator('html').evaluate((element) =>
    element.classList.contains('dark') ? 'dark' : 'light'
  )
  assert.equal(ownerTheme, 'light', `${ownerName}: stored light theme is applied`)
  await ownerPage
    .getByRole('button', { name: ownerT('Window share'), exact: true })
    .last()
    .click()
  const ownerDialog = ownerPage.getByRole('dialog', {
    name: ownerT('Subscription shares'),
    exact: true,
  })
  await ownerDialog.waitFor({ state: 'visible' })
  await ownerDialog
    .getByText(ownerT('Read-only'), { exact: true })
    .waitFor({ state: 'visible' })
  assert.equal(
    await ownerDialog.getByRole('button', { name: ownerT('Close pool') }).count(),
    0
  )
  assert.equal(await ownerDialog.getByRole('radio').count(), 0)
  assert(!(await ownerDialog.innerText()).includes(currentConnection))
  await assertFits(ownerDialog, 320, ownerName)
  const ownerScreenshot = `${ownerName}.png`
  await ownerPage.screenshot({
    path: resolve(output, ownerScreenshot),
    fullPage: true,
    animations: 'disabled',
  })
  assert.deepEqual(ownerUnexpected, [])
  report.journeys.push({
    name: ownerName,
    language: ownerLanguage,
    width: 320,
    theme: ownerTheme,
    role: 'owner',
    screenshots: [ownerScreenshot],
    passed: true,
  })
  await ownerContext.close()

  assert.equal(report.journeys.length, 17)
  assert.deepEqual(report.errors, [])
  report.passed = true
} catch (error) {
  report.passed = false
  report.failure = String(error.stack || error)
  throw error
} finally {
  writeFileSync(
    resolve(output, 'advanced-window-qualification.json'),
    `${JSON.stringify(report, null, 2)}\n`
  )
  await browser?.close()
  if (server.listening) await new Promise((done) => server.close(done))
}
