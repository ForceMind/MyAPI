// Production-build Chromium with isolated personal-workbench APIs. This is
// synthetic UI evidence only; it never contacts a provider or settles usage.
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
    `${repo}/.local-tests/personal-workbench-browser`
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
const browserLocales = {
  en: 'en-US',
  zh: 'zh-CN',
  'zh-TW': 'zh-TW',
  fr: 'fr-FR',
  ru: 'ru-RU',
  ja: 'ja-JP',
  vi: 'vi-VN',
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

for (const key of [
  'Personal workbench',
  'Request path',
  'Key and request limits',
  'Upstream account windows',
  'Your recent records',
  'Actions and reminders',
  'Select an API Key',
  'Choose a Key',
  'Load more Keys',
  'Refresh Keys',
  'Unable to load Keys',
  'No API Keys yet',
  'Unable to load this Key budget',
  'Choose a Key to inspect its limits',
  'Channel read permission is required to view upstream account windows.',
  'Upstream account windows are available to channel administrators.',
  'No upstream account windows were reported',
  'Unable to load upstream account windows',
  'No recent records',
  'Unable to load your recent records',
  'No-wallet policy active',
  'Internal Key quota',
  'Internal units',
  'Strict Token budget',
  'Token',
  'Strict USD budget',
  'Account window threshold',
  'Not configured',
  'Disabled',
  'Expired',
  'Out of internal quota',
  'Open Playground',
  'API key',
  'Select an available API key before sending',
  'Failed to load API keys',
  'The selected API key is unavailable. Choose another key.',
  'Retry',
]) {
  for (const language of Object.keys(languageCodes)) tr(key, language)
}

for (const language of Object.keys(languageCodes).filter(
  (candidate) => candidate !== 'en'
)) {
  assert.notEqual(
    tr('Personal workbench', language),
    tr('Personal workbench', 'en'),
    `${language} uses its real Personal workbench translation`
  )
  assert.notEqual(
    tr('Internal units', language),
    tr('Internal units', 'en'),
    `${language} uses its real internal-unit translation`
  )
}

assert(existsSync(resolve(build, 'index.html')), 'Build the production frontend first')
mkdirSync(output, { recursive: true })

const now = FIXTURE_TIME / 1000
const SECRET_SENTINEL = 'sk-synthetic-secret-must-never-render'
const safeKeys = [
  {
    id: 21,
    name: 'Synthetic internal quota Key',
    status: 1,
    group: 'default',
    remain_quota: 750000,
    unlimited_quota: false,
    expired_time: -1,
    strict_token_budget: false,
  },
  {
    id: 22,
    name: 'Synthetic Token and USD Key',
    status: 1,
    group: 'default',
    remain_quota: 500000,
    unlimited_quota: false,
    expired_time: -1,
    strict_token_budget: true,
  },
  {
    id: 23,
    name: 'Synthetic account threshold Key',
    status: 1,
    group: 'default',
    remain_quota: 400000,
    unlimited_quota: false,
    expired_time: -1,
    strict_token_budget: false,
  },
  {
    id: 24,
    name: 'Synthetic disabled Key',
    status: 2,
    group: 'default',
    remain_quota: 300000,
    unlimited_quota: false,
    expired_time: -1,
    strict_token_budget: false,
  },
  {
    id: 25,
    name: 'Synthetic expired Key',
    status: 1,
    group: 'default',
    remain_quota: 200000,
    unlimited_quota: false,
    expired_time: 1,
    strict_token_budget: false,
  },
  {
    id: 26,
    name: 'Synthetic exhausted Key',
    status: 1,
    group: 'default',
    remain_quota: 0,
    unlimited_quota: false,
    expired_time: -1,
    strict_token_budget: false,
  },
  ...Array.from({ length: 6 }, (_, index) => ({
    id: 27 + index,
    name: `Synthetic additional Key ${index + 1}`,
    status: 1,
    group: 'default',
    remain_quota: 100000,
    unlimited_quota: false,
    expired_time: -1,
    strict_token_budget: false,
  })),
]

function budget(tokenId, userId) {
  const accountThreshold = tokenId === 23
  const strict = tokenId === 22
  const requestId = strict ? 'synthetic-workbench-usage-unknown' : ''
  return {
    policy: {
      token_id: tokenId,
      user_id: userId,
      enabled: strict,
      account_threshold_enabled: accountThreshold,
      account_min_remaining_bps: accountThreshold ? 2500 : 1000,
      account_max_age_seconds: 300,
      fee_enabled: tokenId === 22,
      fee_limit_usd: tokenId === 22 ? '12.500000' : '0.000000',
      fee_used_usd: tokenId === 22 ? '2.250000' : '0.000000',
      fee_reserved_usd: tokenId === 22 ? '0.750000' : '0.000000',
      limit: strict ? 120000 : 0,
      used: strict ? 50000 : 0,
      reserved: strict ? 10000 : 0,
      pending_request_id: requestId,
      revision: 4,
    },
    pending: strict ? {
      request_id: requestId,
      token_id: tokenId,
      user_id: userId,
      state: 'usage_unknown',
      reserved: 10000,
      model_name: 'synthetic-responses-model',
      bound_source: 'openai_responses_input_tokens',
      input_tokens_bound: 4000,
      max_output_tokens: 6000,
      fee_enabled: true,
      fee_reserved_usd: '0.750000',
      actual_input: null,
      actual_output: null,
      actual_fee_usd: null,
    } : null,
    ...(strict ? { review: {
      request_id: requestId,
      token_id: tokenId,
      user_id: userId,
      state: 'usage_unknown',
      reserved_quota: 100,
      actual_quota: null,
      can_recover_text_dispatch: false,
      can_reconcile_usage: false,
    } } : {}),
  }
}

function statusData(base, mode = 'disabled') {
  const capabilityMode = mode === 'unknown' ? 'disabled' : mode
  const advertisedMode = mode === 'unknown' ? 'enabled' : capabilityMode
  return {
    ...base,
    user_funding_mode: advertisedMode,
    user_funding_capabilities: {
      mode: capabilityMode,
      epoch: 9,
      ready: true,
      can_top_up: mode === 'enabled',
      can_redeem: mode === 'enabled',
      can_transfer_affiliate_rewards: mode === 'enabled',
      can_purchase_subscription: mode === 'enabled',
      can_view_funding_history: true,
    },
  }
}

const recent = {
  requests: [
    { id: 501, created_at: now - 120, model_name: 'synthetic-personal-model' },
  ],
  errors: [
    { id: 502, created_at: now - 60, model_name: 'synthetic-pending-review' },
  ],
}

const report = {
  schema: 1,
  evidence:
    'Production-build Chromium with fixed, isolated API fixtures. This does not validate live providers, OAuth, 429 behavior, billing, settlement, deployment, or production.',
  sourceHead,
  workflowHead: process.env.GITHUB_SHA || null,
  version,
  fixtureTime: new Date(FIXTURE_TIME).toISOString(),
  result: 'running',
  journeys: [],
  screenshots: [],
  buildRevisions: [],
  errors: [],
}
const persist = () =>
  writeFileSync(
    resolve(output, 'personal-workbench-qualification.json'),
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
      `.${decodeURIComponent(new URL(request.url || '/', 'http://localhost').pathname)}`
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

let browser
let origin
const contexts = new Set()

function isReveal(path, method) {
  return method === 'POST' && /^\/api\/token\/\d+\/key\/?$/.test(path)
}

function scenarioResponse(fixture, scenario, url, method, reads) {
  const path = url.pathname.replace(/\/$/, '')
  const count = (reads.get(path) || 0) + 1
  reads.set(path, count)

  if (path === '/api/status') {
    const response = fixture.response(url, method)
    return {
      status: 200,
      body: {
        ...response.body,
        data: statusData(response.body.data, scenario.mode),
      },
    }
  }
  if (path === '/api/user/self') {
    if (scenario.failSelf && count === 1) {
      return {
        status: 503,
        body: { success: false, message: 'Synthetic self policy unavailable' },
      }
    }
    return {
      status: 200,
      body: {
        success: true,
        data: {
          ...fixture.user,
          self_use_no_balance: !scenario.storedAllowance,
          usage_policy_revision: 4,
        },
      },
    }
  }
  if (path === '/pg/keys') {
    if (
      url.searchParams.get('p') !== '1' ||
      url.searchParams.get('page_size') !== '100' ||
      [...url.searchParams.keys()].some((key) => !['p', 'page_size'].includes(key))
    ) {
      return { violation: `Unexpected safe Key query: ${url.search}` }
    }
    if (
      (scenario.failKeys && count === 1) ||
      (scenario.failPlaygroundKeys && count >= 2)
    ) {
      return {
        status: 503,
        body: { success: false, message: 'Synthetic Keys unavailable' },
      }
    }
    let items = scenario.empty ? [] : safeKeys
    if (scenario.handoffKeyUnavailable && count >= 2) {
      items = safeKeys.map((key) =>
        key.id === 23 ? { ...key, status: 2 } : key
      )
    }
    return {
      status: 200,
      body: {
        success: true,
        data: { items, total: items.length, page: 1, page_size: 100 },
      },
    }
  }
  const budgetMatch = path.match(/^\/api\/token\/(\d+)\/budget$/)
  if (budgetMatch) {
    const tokenId = Number(budgetMatch[1])
    if (url.search) return { violation: `Unexpected budget query: ${url.search}` }
    if (!safeKeys.some((key) => key.id === tokenId)) {
      return { violation: `Unknown synthetic Key budget: ${tokenId}` }
    }
    if (
      (scenario.failBudgetOnce && count === 1) ||
      (scenario.unknownBudget && tokenId === 22)
    ) {
      return {
        status: 503,
        body: { success: false, message: 'Synthetic budget unavailable' },
      }
    }
    return {
      status: 200,
      body: { success: true, data: budget(tokenId, fixture.user.id) },
    }
  }
  if (/^\/api\/token\/\d+$/.test(path) ||
      ((scenario.mode === 'disabled' || scenario.mode === 'unknown') && ['/api/token', '/api/token/search'].includes(path))) {
    return {
      violation:
        'Personal workbench requested a broad Key owner object instead of the safe list DTO',
    }
  }
  if (path === '/api/log/overview') {
    return { violation: 'Personal workbench requested the all-user log overview' }
  }
  if (path === '/api/log/self/overview') {
    if (scenario.failRecent && count === 1) {
      return {
        status: 503,
        body: { success: false, message: 'Synthetic recent records unavailable' },
      }
    }
    return {
      status: 200,
      body: { success: true, data: scenario.empty ? { requests: [], errors: [] } : recent },
    }
  }
  if (path === '/api/channel/quota/changes') {
    if (!scenario.channelRead) {
      return { violation: 'Session without channel.read requested account windows' }
    }
    if (scenario.failAccounts && count === 1) {
      return {
        status: 503,
        body: { success: false, message: 'Synthetic account windows unavailable' },
      }
    }
    if (scenario.empty) {
      return {
        status: 200,
        body: {
          success: true,
          data: {
            items: [],
            range: '24h',
            start: now - 86400,
            end: now,
            generated_at: now,
          },
        },
      }
    }
  }
  const response = fixture.response(url, method)
  return response || { violation: `Unconfigured API: ${method} ${path}${url.search}` }
}

async function createSession(scenario) {
  const fixture = createUIFixture({
    role: scenario.role,
    language: languageCodes[scenario.language],
    personalPolicy: true,
  })
  fixture.user.permissions = {
    ...fixture.user.permissions,
    admin_permissions:
      scenario.role === 100
        ? fixture.user.permissions.admin_permissions
        : { channel: { read: scenario.channelRead === true } },
  }
  const mobile = scenario.width === 320
  const reads = new Map()
  const audit = {
    requests: [],
    mutations: [],
    external: [],
    violations: [],
    pageErrors: [],
  }
  const context = await browser.newContext({
    viewport: { width: scenario.width, height: mobile ? 1100 : 900 },
    locale: browserLocales[scenario.language],
    timezoneId: 'UTC',
    reducedMotion: 'reduce',
    colorScheme: scenario.theme,
    serviceWorkers: 'block',
    hasTouch: mobile,
    isMobile: mobile,
  })
  contexts.add(context)
  await context.addCookies([
    { name: 'vite-ui-theme', value: scenario.theme, url: origin },
    { name: 'sidebar_state', value: 'true', url: origin },
  ])
  await context.addInitScript(
    ({ languageCode, savedPlaygroundKeyId, userId }) => {
      localStorage.setItem('i18nextLng', languageCode)
      if (savedPlaygroundKeyId) {
        localStorage.setItem(
          `playground_config:user:${userId}`,
          JSON.stringify({
            version: 1,
            data: { keyId: savedPlaygroundKeyId },
          })
        )
      }
    },
    {
      languageCode: languageCodes[scenario.language],
      savedPlaygroundKeyId: scenario.savedPlaygroundKeyId ?? null,
      userId: scenario.role === 1 ? 2 : 1,
    }
  )
  await context.route('**/*', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    if (url.origin !== origin) {
      if (url.protocol === 'data:' || url.protocol === 'blob:') {
        return route.continue()
      }
      audit.external.push(url.href)
      return route.abort('blockedbyclient')
    }
    if (!url.pathname.startsWith('/api/') && !url.pathname.startsWith('/pg/')) {
      if (
        url.pathname.startsWith('/v1/') ||
        (['fetch', 'xhr'].includes(request.resourceType()) &&
          request.method() !== 'GET')
      ) {
        audit.requests.push({
          method: request.method(),
          path: url.pathname,
          query: url.search,
          keyId: request.headers()['x-myapi-key-id'] || null,
        })
        audit.mutations.push(`${request.method()} ${url.pathname}`)
        audit.violations.push(
          `Unexpected same-origin service request: ${request.method()} ${url.pathname}`
        )
        return route.fulfill({ status: 501, json: { success: false } })
      }
      return route.continue()
    }
    const path = url.pathname.replace(/\/$/, '')
    const refresh =
      request.method() === 'POST' && path === '/api/user/auth/refresh'
    audit.requests.push({
      method: request.method(),
      path,
      query: url.search,
      keyId: request.headers()['x-myapi-key-id'] || null,
    })
    if (request.method() !== 'GET' && !refresh) {
      audit.mutations.push(`${request.method()} ${path}`)
      return route.fulfill({ status: 501, json: { success: false } })
    }
    if (isReveal(path, request.method())) {
      audit.violations.push(`Unexpected Key reveal: ${request.method()} ${path}`)
      return route.fulfill({ status: 501, json: { success: false } })
    }
    const response = scenarioResponse(
      fixture,
      scenario,
      url,
      request.method(),
      reads
    )
    if (response.violation) audit.violations.push(response.violation)
    return route.fulfill({
      status: response.status || 501,
      json:
        response.body || {
          success: false,
          message: response.violation || 'Unconfigured synthetic response',
        },
    })
  })
  await context.routeWebSocket('**/*', (socket) => {
    audit.violations.push(`Unexpected WebSocket: ${socket.url()}`)
    socket.close()
  })
  const page = await context.newPage()
  page.setDefaultTimeout(20000)
  await page.clock.setFixedTime(new Date(FIXTURE_TIME))
  page.on('pageerror', (error) => audit.pageErrors.push(error.message))
  return { page, context, reads, audit }
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

async function openWorkbench(page, scenario) {
  await page.goto(`${origin}/dashboard/overview`, { waitUntil: 'networkidle' })
  await page.locator('[data-myapi-shell="authenticated"]').waitFor()
  await page
    .getByRole('heading', {
      level: 1,
      name: tr('Personal workbench', scenario.language),
      exact: true,
    })
    .waitFor()
  await page.getByTestId('personal-workbench').waitFor()
  await settled(page)
  const revision = await page.locator('html').getAttribute('data-build-rev')
  assert(revision?.startsWith(`rv.${version}.`), 'Visible revision matches VERSION')
  if (process.env.GITHUB_SHA) {
    assert(
      revision.includes(process.env.GITHUB_SHA),
      'Visible revision matches the CI checkout'
    )
  }
  report.buildRevisions.push({ journey: scenario.name, revision })
  return revision
}

async function assertLayout(page, scenario) {
  const geometry = await page.evaluate(() => {
    const rect = (selector) => {
      const element = document.querySelector(selector)
      if (!element) return null
      const box = element.getBoundingClientRect()
      return {
        x: box.x,
        y: box.y,
        right: box.right,
        bottom: box.bottom,
        width: box.width,
        scrollWidth: element.scrollWidth,
        clientWidth: element.clientWidth,
      }
    }
    return {
      viewport: innerWidth,
      documentWidth: document.documentElement.scrollWidth,
      theme: document.documentElement.classList.contains('dark')
        ? 'dark'
        : 'light',
      request: rect('[data-testid="request-path-section"]'),
      actions: rect('[data-testid="workbench-actions"]'),
      sections: [
        'request-path-section',
        'key-budget-section',
        'account-windows-section',
        'recent-records-section',
        'workbench-actions',
      ].map((id) => rect(`[data-testid="${id}"]`)),
    }
  })
  assert.equal(geometry.theme, scenario.theme, `${scenario.name}: theme applied`)
  assert(
    geometry.documentWidth <= geometry.viewport + 1,
    `${scenario.name}: no page-wide horizontal overflow`
  )
  assert(
    geometry.sections.every(
      (section) =>
        section &&
        section.x >= -1 &&
        section.right <= geometry.viewport + 1 &&
        section.scrollWidth <= section.clientWidth + 1
    ),
    `${scenario.name}: every workbench section fits the viewport`
  )
  if (scenario.width === 320) {
    assert(
      Math.abs(geometry.request.x - geometry.actions.x) <= 2 &&
        geometry.actions.y >= geometry.request.y,
      `${scenario.name}: 320px uses one column`
    )
  } else {
    assert(
      geometry.actions.x > geometry.request.x,
      `${scenario.name}: desktop actions use the side column`
    )
  }
}

async function screenshot(page, scenario, suffix) {
  if (suffix === 'token-usd' || suffix === 'unknown-budget') {
    await page.getByTestId('key-budget-section').scrollIntoViewIfNeeded()
    await settled(page)
  } else if (suffix === 'account-threshold') {
    await page.getByTestId('workbench-actions').scrollIntoViewIfNeeded()
    await settled(page)
  }
  await assertLayout(page, scenario)
  const selectedKeyReadable = await page.getByTestId('request-flow-selected-key').evaluate(element => {
    const bounds = element.getBoundingClientRect()
    const range = document.createRange()
    range.selectNodeContents(element)
    const lines = [...range.getClientRects()].filter(line => line.width > 0 && line.height > 0)
    return lines.length > 0 && element.scrollWidth <= element.clientWidth + 1 &&
      element.scrollHeight <= element.clientHeight + 1 && getComputedStyle(element).textOverflow !== 'ellipsis' &&
      lines.every(line => line.left >= bounds.left - 1 && line.right <= bounds.right + 1 &&
        line.top >= bounds.top - 1 && line.bottom <= bounds.bottom + 1)
  })
  assert(selectedKeyReadable, 'Selected Key identity and the unselected prompt remain fully readable')
  const file = `${scenario.name}-${suffix}.png`
  await page.screenshot({ path: resolve(output, file), fullPage: true })
  report.screenshots.push({
    file,
    journey: scenario.name,
    sourceHead,
    buildRevision: report.buildRevisions.at(-1)?.revision || null,
    fixtureTime: new Date(FIXTURE_TIME).toISOString(),
    selectedKeyReadable,
  })
  persist()
}

function section(page, testId) {
  return page.getByTestId(testId)
}

function openPlaygroundButton(page, scenario) {
  return page.getByTestId('personal-workbench').getByRole('button', {
    name: tr('Open Playground', scenario.language),
    exact: true,
  })
}

async function assertCommonPersonal(page, scenario, audit) {
  assert.equal(
    await page.locator('main').count(),
    1,
    'The authenticated shell keeps one main landmark'
  )
  for (const [testId, title] of [
    ['request-path-section', 'Request path'],
    ['key-budget-section', 'Key and request limits'],
    ['account-windows-section', 'Upstream account windows'],
    ['recent-records-section', 'Your recent records'],
    ['workbench-actions', 'Actions and reminders'],
  ]) {
    await section(page, testId)
      .getByRole('heading', { name: tr(title, scenario.language), exact: true })
      .waitFor()
  }
  await section(page, 'workbench-actions')
    .getByText(tr('No-wallet policy active', scenario.language), { exact: true })
    .waitFor()
  assert.equal(
    await page.locator(`text=${SECRET_SENTINEL}`).count(),
    0,
    'No Key secret is rendered'
  )
  assert.equal(
    audit.requests.filter((request) => isReveal(request.path, request.method))
      .length,
    0,
    'No Key reveal request is made'
  )
}

async function selectNativeKey(page, scenario, value) {
  const select = page.getByRole('combobox', {
    name: tr('Select an API Key', scenario.language),
    exact: true,
  })
  await select.selectOption(String(value))
  assert.equal(await select.inputValue(), String(value))
  return select
}

function localizedNumber(value, language) {
  return new Intl.NumberFormat(browserLocales[language]).format(value)
}

async function assertPlaygroundSelection(page, scenario, expectedKeyId) {
  const keySelect = page.getByRole('combobox', {
    name: tr('API key', scenario.language),
    exact: true,
  })
  await keySelect.waitFor()
  await page.waitForFunction(
    ({ keyId, label }) =>
      [...document.querySelectorAll('select')].some(
        (select) =>
          select.getAttribute('aria-label') === label &&
          select.value === String(keyId)
      ),
    { keyId: expectedKeyId, label: tr('API key', scenario.language) }
  )
  assert.equal(
    await keySelect.inputValue(),
    String(expectedKeyId),
    `Playground selects the handed-off Key ${expectedKeyId}`
  )
  const identity = await keySelect.evaluate((select) => {
    const identityId = select.getAttribute('aria-describedby')
    return identityId
      ? document.getElementById(identityId)?.textContent?.trim()
      : null
  })
  assert(identity, 'The selected Playground Key exposes its full identity')
  assert.equal(
    identity,
    'Synthetic account threshold Key · default',
    'Playground renders the exact handed-off Key identity'
  )
}

async function storedPlaygroundKeyId(page, role) {
  return page.evaluate((userId) => {
    const value = localStorage.getItem(`playground_config:user:${userId}`)
    return value ? JSON.parse(value)?.data?.keyId : undefined
  }, role === 1 ? 2 : 1)
}

async function runPopulated(scenario) {
  const { page, context, reads, audit } = await createSession(scenario)
  const journey = {
    name: scenario.name,
    role: scenario.role,
    language: scenario.language,
    width: scenario.width,
    theme: scenario.theme,
    result: 'running',
  }
  try {
    journey.buildRevision = await openWorkbench(page, scenario)
    await assertCommonPersonal(page, scenario, audit)
    const keySelect = page.getByRole('combobox', {
      name: tr('Select an API Key', scenario.language),
      exact: true,
    })
    await keySelect.waitFor()
    assert.equal(await keySelect.inputValue(), '', 'No Key is selected automatically')
    const openPlayground = openPlaygroundButton(page, scenario)
    await openPlayground.waitFor()
    assert(
      await openPlayground.isDisabled(),
      'Open Playground stays disabled until an available Key is selected'
    )
    assert.equal(
      await page
        .getByTestId('personal-workbench')
        .locator('a[href="/playground"]')
        .count(),
      0,
      'The workbench exposes no link that can bypass the Key handoff'
    )
    await page.getByTestId('key-selection-empty').waitFor()
    if (scenario.role === 100 || scenario.role === 1) {
      await screenshot(page, scenario, 'unselected')
    }
    assert.equal(
      audit.requests.filter((request) =>
        /^\/api\/token\/\d+\/budget$/.test(request.path)
      ).length,
      0,
      'No budget is read before explicit Key selection'
    )
    assert.equal(await keySelect.locator('option').count(), 11)
    await section(page, 'request-path-section')
      .getByRole('button', {
        name: tr('Load more Keys', scenario.language),
        exact: true,
      })
      .click()
    assert.equal(await keySelect.locator('option').count(), 13)
    for (const [value, state] of [
      ['24', 'Disabled'],
      ['25', 'Expired'],
      ['26', 'Out of internal quota'],
    ]) {
      assert(
        (await keySelect.locator(`option[value="${value}"]`).innerText()).includes(
          tr(state, scenario.language)
        ),
        `Key ${value} exposes its actual availability state`
      )
    }
    assert.equal(
      reads.get('/pg/keys'),
      1,
      'Loading more locally does not refetch or reveal Keys'
    )

    await keySelect.focus()
    await keySelect.press('ArrowDown')
    assert.equal(await keySelect.inputValue(), '21', 'Keyboard selects the first Key')
    await section(page, 'key-budget-section')
      .getByText(localizedNumber(750000, scenario.language))
      .first()
      .waitFor()

    await selectNativeKey(page, scenario, 22)
    const budgetSection = section(page, 'key-budget-section')
    await budgetSection
      .getByText(localizedNumber(120000, scenario.language))
      .first()
      .waitFor()
    await budgetSection
      .getByText(/USD\s+12(?:[.,]50|\.500000)/)
      .first()
      .waitFor()
    for (const unit of [
      'Internal Key quota',
      'Internal units',
      'Strict Token budget',
      'Token',
      'Strict USD budget',
      'Account window threshold',
    ]) {
      await budgetSection.getByText(tr(unit, scenario.language), { exact: true }).first().waitFor()
    }
    const internalSummary = await budgetSection
      .getByText(tr('Internal Key quota', scenario.language), { exact: true })
      .locator('..')
      .locator('..')
      .innerText()
    assert(
      internalSummary.includes(localizedNumber(500000, scenario.language)) &&
        internalSummary.includes(tr('Internal units', scenario.language)) &&
        !internalSummary.includes('USD') &&
        !internalSummary.includes('%') &&
        !internalSummary.includes(tr('Token', scenario.language)),
      'Ordinary Key quota stays in internal units'
    )
    const tokenSummary = await budgetSection
      .getByText(tr('Strict Token budget', scenario.language), { exact: true })
      .locator('..')
      .locator('..')
      .innerText()
    assert(
      tokenSummary.includes(localizedNumber(120000, scenario.language)) &&
        tokenSummary.includes(tr('Token', scenario.language)),
      'Strict Token budget keeps its Token unit'
    )
    const usdSummary = await budgetSection
      .getByText(tr('Strict USD budget', scenario.language), { exact: true })
      .locator('..')
      .locator('..')
      .innerText()
    assert(
      usdSummary.includes('USD 12.500000'),
      'Strict fee budget keeps its exact USD amount and unit'
    )
    assert(
      audit.requests.some(
        (request) =>
          request.method === 'GET' && request.path === '/api/token/22/budget'
      ),
      'Explicit strict Key selection reads its budget'
    )
    await budgetSection.getByText(tr('Pending request review', scenario.language), { exact: true }).waitFor()
    await section(page, 'workbench-actions').getByText(tr('Pending request review', scenario.language), { exact: true }).waitFor()
    await screenshot(page, scenario, 'token-usd')

    await selectNativeKey(page, scenario, 23)
    await budgetSection.getByText(/25(?:[.,]00)?\s*%/).first().waitFor()
    assert.equal(
      await page.getByTestId('request-flow-selected-key').innerText(),
      'Synthetic account threshold Key',
      'The workbench keeps the exact Key 23 identity before handoff'
    )
    assert.equal(await budgetSection.getByText(tr('Not configured', scenario.language), { exact: true }).count(), 2, 'Account threshold does not enable Token or USD budgets')
    assert.equal(await budgetSection.getByText(tr('Pending request review', scenario.language), { exact: true }).count(), 0, 'Switching Keys removes the previous reservation reminder')
    await section(page, 'workbench-actions').getByText(tr('No pending request review for the selected Key.', scenario.language), { exact: true }).waitFor()
    const thresholdSummary = await budgetSection
      .getByText(tr('Account window threshold', scenario.language), {
        exact: true,
      })
      .locator('..')
      .locator('..')
      .innerText()
    assert(
      thresholdSummary.includes('25.00%') && !thresholdSummary.includes('USD'),
      'Account threshold stays a percentage'
    )
    assert(
      audit.requests.some(
        (request) =>
          request.method === 'GET' && request.path === '/api/token/23/budget'
      ),
      'Switching Keys reads the selected budget'
    )
    assert.equal(
      audit.requests.filter((request) => request.path === '/api/log/overview')
        .length,
      0,
      'Root also reads only the self log overview'
    )
    assert(
      audit.requests.some(
        (request) => request.path === '/api/log/self/overview'
      ),
      'Personal records come from the self endpoint'
    )
    assert(
      audit.requests.some((request) => request.path === '/api/user/self'),
      'The saved personal policy is freshly read'
    )
    if (scenario.channelRead) {
      assert(
        audit.requests.some(
          (request) => request.path === '/api/channel/quota/changes'
        ),
        'A permitted administrator reads account windows'
      )
    } else {
      assert.equal(
        audit.requests.filter(
          (request) => request.path === '/api/channel/quota/changes'
        ).length,
        0,
        'A session without channel.read never requests account windows'
      )
      const permissionCopy =
        scenario.role === 1
          ? 'Upstream account windows are available to channel administrators.'
          : 'Channel read permission is required to view upstream account windows.'
      await page
        .getByTestId('accounts-permission-state')
        .getByText(tr(permissionCopy, scenario.language), { exact: true })
        .waitFor()
    }

    await screenshot(page, scenario, 'account-threshold')
    const requestCount = audit.requests.length
    const handedOffModels = page.waitForResponse((response) => {
      const request = response.request()
      return (
        new URL(response.url()).pathname === '/pg/models' &&
        request.headers()['x-myapi-key-id'] === '23'
      )
    })
    assert.equal(
      await openPlayground.isDisabled(),
      false,
      'Open Playground becomes enabled for available Key 23'
    )
    await openPlayground.focus()
    await page.keyboard.press('Enter')
    await page.waitForURL(`${origin}/playground`)
    await assertPlaygroundSelection(page, scenario, 23)
    await handedOffModels
    assert.equal(
      await page.locator(`text=${SECRET_SENTINEL}`).count(),
      0,
      'Playground renders no Key secret after handoff'
    )
    assert.equal(
      reads.get('/pg/keys'),
      2,
      'Playground validates the handoff with one fresh safe-list read'
    )
    assert.equal(
      await storedPlaygroundKeyId(page, scenario.role),
      23,
      'The one-time handoff replaces the previously saved Key 22'
    )
    assert(
      audit.requests.some(
        (request) => request.path === '/pg/models' && request.keyId === '23'
      ),
      'Playground loads models for the handed-off Key 23'
    )
    assert.equal(
      audit.requests.filter(
        (request) => request.path === '/pg/models' && request.keyId === '22'
      ).length,
      0,
      'Playground never loads models for the older saved Key 22'
    )
    assert.equal(
      audit.requests.filter(
        (request) =>
          request.method === 'POST' &&
          request.path === '/pg/chat/completions'
      ).length,
      0,
      'Selecting the handed-off Key never sends a generation request'
    )
    assert.equal(
      audit.requests
        .slice(requestCount)
        .filter(
          (request) =>
            request.method !== 'GET' &&
            request.path !== '/api/user/auth/refresh'
        ).length,
      0,
      'The primary action only navigates; it does not send a request'
    )

    assert.deepEqual(audit.mutations, [], 'No business mutation is attempted')
    assert.deepEqual(audit.external, [], 'No external request is attempted')
    assert.deepEqual(audit.violations, [], 'No fixture contract is violated')
    assert.deepEqual(audit.pageErrors, [], 'No page runtime error is reported')
    journey.requestCount = audit.requests.length
    journey.reads = Object.fromEntries(reads)
    journey.result = 'passed'
  } catch (error) {
    journey.result = 'failed'
    journey.error = error instanceof Error ? error.stack : String(error)
    throw error
  } finally {
    report.journeys.push(journey)
    persist()
    await context.close()
    contexts.delete(context)
  }
}

async function runBlockedHandoff(scenario) {
  const { page, context, reads, audit } = await createSession(scenario)
  const journey = {
    name: scenario.name,
    role: scenario.role,
    language: scenario.language,
    width: scenario.width,
    theme: scenario.theme,
    result: 'running',
  }
  try {
    journey.buildRevision = await openWorkbench(page, scenario)
    await selectNativeKey(page, scenario, 23)
    await section(page, 'key-budget-section')
      .getByText('25.00%', { exact: true })
      .first()
      .waitFor()

    const requestCount = audit.requests.length
    const openPlayground = openPlaygroundButton(page, scenario)
    assert.equal(
      await page
        .getByTestId('personal-workbench')
        .locator('a[href="/playground"]')
        .count(),
      0,
      'The blocked-handoff setup exposes no Playground link shortcut'
    )
    assert.equal(await openPlayground.isDisabled(), false)
    await openPlayground.click()
    await page.waitForURL(`${origin}/playground`)

    const keySelect = page.getByRole('combobox', {
      name: tr('API key', scenario.language),
      exact: true,
    })
    await keySelect.waitFor()
    const notice = scenario.failPlaygroundKeys
      ? 'Failed to load API keys'
      : 'The selected API key is unavailable. Choose another key.'
    const status = page.getByRole('status')
    await status.waitFor()
    assert(
      (await status.innerText()).includes(tr(notice, scenario.language)),
      'The blocked handoff explains the exact localized failure state'
    )
    assert.equal(
      await keySelect.inputValue(),
      '',
      'An invalid handoff leaves the Playground Key selection empty'
    )
    assert.equal(
      await page.locator(`text=${SECRET_SENTINEL}`).count(),
      0,
      'A blocked handoff renders no Key secret'
    )
    assert.equal(
      await storedPlaygroundKeyId(page, scenario.role),
      null,
      'An invalid handoff clears the older saved Key instead of falling back'
    )
    assert.equal(
      reads.get('/pg/keys'),
      2,
      'A blocked handoff is decided by one fresh safe-list read'
    )
    assert.equal(
      audit.requests.filter((request) => request.path === '/pg/models').length,
      0,
      'An invalid handoff never loads models for the handed-off or saved Key'
    )
    assert.equal(
      audit.requests
        .slice(requestCount)
        .filter(
          (request) =>
            request.method !== 'GET' &&
            request.path !== '/api/user/auth/refresh'
        ).length,
      0,
      'An invalid handoff performs no business POST'
    )
    assert.equal(
      audit.requests.filter(
        (request) =>
          request.method === 'POST' &&
          request.path === '/pg/chat/completions'
      ).length,
      0,
      'An invalid handoff never sends a generation request'
    )
    assert.deepEqual(
      audit.mutations,
      [],
      'An invalid handoff performs no mutation'
    )
    assert.deepEqual(
      audit.external,
      [],
      'An invalid handoff performs no external request'
    )
    assert.deepEqual(
      audit.violations,
      [],
      'An invalid handoff honors fixture contracts'
    )
    assert.deepEqual(
      audit.pageErrors,
      [],
      'An invalid handoff reports no runtime error'
    )
    journey.requestCount = audit.requests.length
    journey.reads = Object.fromEntries(reads)
    journey.result = 'passed'
  } catch (error) {
    journey.result = 'failed'
    journey.error = error instanceof Error ? error.stack : String(error)
    throw error
  } finally {
    report.journeys.push(journey)
    persist()
    await context.close()
    contexts.delete(context)
  }
}

async function runRecovery(scenario) {
  const { page, context, reads, audit } = await createSession(scenario)
  const journey = {
    name: scenario.name,
    role: scenario.role,
    language: scenario.language,
    width: scenario.width,
    theme: scenario.theme,
    result: 'running',
  }
  try {
    journey.buildRevision = await openWorkbench(page, scenario)
    await page.getByTestId('keys-error-state').waitFor()
    await page.getByTestId('recent-error-state').waitFor()
    await page.getByTestId('accounts-error-state').waitFor()
    await screenshot(page, scenario, 'errors')

    await page
      .getByTestId('keys-error-state')
      .getByRole('button', { name: tr('Retry', scenario.language), exact: true })
      .click()
    await page
      .getByRole('combobox', {
        name: tr('Select an API Key', scenario.language),
        exact: true,
      })
      .waitFor()
    await page
      .getByTestId('recent-error-state')
      .getByRole('button', { name: tr('Retry', scenario.language), exact: true })
      .click()
    await section(page, 'recent-records-section')
      .getByText('synthetic-personal-model', { exact: true })
      .waitFor()
    await page
      .getByTestId('accounts-error-state')
      .getByRole('button', { name: tr('Retry', scenario.language), exact: true })
      .click()
    await page.getByTestId('accounts-error-state').waitFor({ state: 'hidden' })

    await selectNativeKey(page, scenario, 22)
    await page.getByTestId('budget-error-state').waitFor()
    await page
      .getByTestId('budget-error-state')
      .getByRole('button', { name: tr('Retry', scenario.language), exact: true })
      .click()
    await section(page, 'key-budget-section')
      .getByText(/USD\s+12(?:[.,]50|\.500000)/)
      .first()
      .waitFor()
    await screenshot(page, scenario, 'recovered')

    for (const path of [
      '/pg/keys',
      '/api/log/self/overview',
      '/api/channel/quota/changes',
      '/api/token/22/budget',
    ]) {
      assert.equal(reads.get(path), 2, `${path} is retried exactly once`)
    }
    assert.deepEqual(audit.mutations, [], 'Recovery performs no business mutation')
    assert.deepEqual(audit.external, [], 'Recovery performs no external request')
    assert.deepEqual(audit.violations, [], 'Recovery honors fixture contracts')
    assert.deepEqual(audit.pageErrors, [], 'Recovery reports no runtime error')
    journey.requestCount = audit.requests.length
    journey.reads = Object.fromEntries(reads)
    journey.result = 'passed'
  } catch (error) {
    journey.result = 'failed'
    journey.error = error instanceof Error ? error.stack : String(error)
    throw error
  } finally {
    report.journeys.push(journey)
    persist()
    await context.close()
    contexts.delete(context)
  }
}

async function runEmpty(scenario) {
  const { page, context, reads, audit } = await createSession(scenario)
  const journey = {
    name: scenario.name,
    role: scenario.role,
    language: scenario.language,
    width: scenario.width,
    theme: scenario.theme,
    result: 'running',
  }
  try {
    journey.buildRevision = await openWorkbench(page, scenario)
    await page.getByTestId('keys-empty-state').waitFor()
    await page.getByTestId('recent-empty-state').waitFor()
    await page.getByTestId('accounts-empty-state').waitFor()
    assert.equal(
      audit.requests.filter((request) =>
        /^\/api\/token\/\d+\/budget$/.test(request.path)
      ).length,
      0,
      'An empty Key list cannot trigger a budget read'
    )
    if (scenario.storedAllowance) {
      const policy = page.getByTestId('workbench-actions').getByText(tr('Stored user allowance remains in effect', scenario.language), { exact: true })
      await policy.scrollIntoViewIfNeeded()
      await settled(page)
      assert(await policy.evaluate(element => {
        const bounds = element.getBoundingClientRect()
        const region = element.closest('[role="region"]').getBoundingClientRect()
        return element.scrollWidth <= element.clientWidth + 1 && bounds.left >= region.left && bounds.right <= region.right
      }), 'Long translated policy remains readable inside its card')
    }
    await screenshot(page, scenario, scenario.storedAllowance ? 'long-policy' : 'empty')
    assert.deepEqual(audit.mutations, [], 'Empty state performs no mutation')
    assert.deepEqual(audit.external, [], 'Empty state performs no external request')
    assert.deepEqual(audit.violations, [], 'Empty state honors fixture contracts')
    assert.deepEqual(audit.pageErrors, [], 'Empty state reports no runtime error')
    journey.reads = Object.fromEntries(reads)
    journey.result = 'passed'
  } catch (error) {
    journey.result = 'failed'
    journey.error = error instanceof Error ? error.stack : String(error)
    throw error
  } finally {
    report.journeys.push(journey)
    persist()
    await context.close()
    contexts.delete(context)
  }
}

async function runUnknownBudget(scenario) {
  const { page, context, reads, audit } = await createSession(scenario)
  const journey = {
    name: scenario.name,
    role: scenario.role,
    language: scenario.language,
    width: scenario.width,
    theme: scenario.theme,
    result: 'running',
  }
  try {
    journey.buildRevision = await openWorkbench(page, scenario)
    await selectNativeKey(page, scenario, 22)
    const failure = page.getByTestId('budget-error-state')
    await failure.waitFor()
    await failure
      .getByText(tr('Unable to load this Key budget', scenario.language), {
        exact: true,
      })
      .waitFor()
    const text = await section(page, 'key-budget-section').innerText()
    assert.equal(await section(page, 'key-budget-section').getByText(tr('Unknown', scenario.language), { exact: true }).count(), 3, 'Unavailable budget explicitly marks Token, USD and threshold as unknown')
    assert(!text.includes('USD 0'), 'Unknown USD budget is not shown as zero')
    assert(!text.includes('0 Token'), 'Unknown Token budget is not shown as zero')
    assert(
      !text.includes(tr('Not configured', scenario.language)),
      'An unavailable budget is not presented as confirmed unconfigured state'
    )
    await screenshot(page, scenario, 'unknown-budget')
    assert.deepEqual(audit.mutations, [], 'Unknown budget performs no mutation')
    assert.deepEqual(audit.external, [], 'Unknown budget performs no external request')
    assert.deepEqual(audit.violations, [], 'Unknown budget honors fixture contracts')
    assert.deepEqual(audit.pageErrors, [], 'Unknown budget reports no runtime error')
    journey.reads = Object.fromEntries(reads)
    journey.result = 'passed'
  } catch (error) {
    journey.result = 'failed'
    journey.error = error instanceof Error ? error.stack : String(error)
    throw error
  } finally {
    report.journeys.push(journey)
    persist()
    await context.close()
    contexts.delete(context)
  }
}

async function runGate(scenario) {
  const { page, context, reads, audit } = await createSession(scenario)
  const journey = {
    name: scenario.name,
    mode: scenario.mode,
    role: scenario.role,
    language: scenario.language,
    width: scenario.width,
    theme: scenario.theme,
    result: 'running',
  }
  try {
    await page.goto(`${origin}/dashboard/overview`, { waitUntil: 'networkidle' })
    await page.locator('[data-myapi-shell="authenticated"]').waitFor()
    await page
      .getByRole('heading', {
        level: 1,
        name: tr('Overview', scenario.language),
        exact: true,
      })
      .waitFor()
    const revision = await page.locator('html').getAttribute('data-build-rev')
    assert(revision?.startsWith(`rv.${version}.`), 'Visible revision matches VERSION')
    if (scenario.mode === 'unknown') {
      await page.getByText(tr('Unable to confirm workspace mode', scenario.language), { exact: true }).waitFor()
      assert.equal(audit.requests.filter(request => ['/pg/keys', '/api/token', '/api/user/models', '/api/channel/quota/changes', '/api/log/self/overview', '/api/log/overview', '/api/data/self', '/api/data', '/api/perf-metrics/summary'].includes(request.path)).length, 0, 'An unconfirmed mode mounts neither workspace data plane')
    }
    if (process.env.GITHUB_SHA) {
      assert(
        revision.includes(process.env.GITHUB_SHA),
        'Visible revision matches the CI checkout'
      )
    }
    journey.buildRevision = revision
    report.buildRevisions.push({ journey: scenario.name, revision })
    assert.equal(
      await page.getByTestId('personal-workbench').count(),
      0,
      `${scenario.mode} never mounts the personal workspace`
    )
    assert.equal(
      audit.requests.filter((request) => request.path === '/pg/keys').length,
      0,
      `${scenario.mode} never boots personal Key reads`
    )
    assert.equal(
      audit.requests.filter((request) =>
        /^\/api\/token\/\d+\/budget$/.test(request.path)
      ).length,
      0,
      `${scenario.mode} never boots personal budget reads`
    )
    assert.deepEqual(audit.mutations, [], 'Legacy gate performs no business mutation')
    assert.deepEqual(audit.external, [], 'Legacy gate performs no external request')
    assert.deepEqual(audit.violations, [], 'Legacy gate honors fixture contracts')
    assert.deepEqual(audit.pageErrors, [], 'Legacy gate reports no runtime error')
    journey.reads = Object.fromEntries(reads)
    journey.result = 'passed'
  } catch (error) {
    journey.result = 'failed'
    journey.error = error instanceof Error ? error.stack : String(error)
    throw error
  } finally {
    report.journeys.push(journey)
    persist()
    await context.close()
    contexts.delete(context)
  }
}

try {
  await new Promise((done) => server.listen(0, '127.0.0.1', done))
  origin = `http://127.0.0.1:${server.address().port}`
  const { chromium } = await import(
    process.env.MYAPI_PLAYWRIGHT_MODULE
      ? pathToFileURL(process.env.MYAPI_PLAYWRIGHT_MODULE).href
      : 'playwright'
  )
  browser = await chromium.launch({
    headless: true,
    executablePath: process.env.MYAPI_CHROMIUM_PATH || undefined,
    args: ['--disable-dev-shm-usage'],
  })

  const populatedScenarios = [
    {
      name: 'personal-owner-en-320-dark',
      role: 1,
      language: 'en',
      width: 320,
      theme: 'dark',
      channelRead: false,
    },
    {
      name: 'personal-admin-no-channel-en-1280-dark',
      role: 10,
      language: 'en',
      width: 1280,
      theme: 'dark',
      channelRead: false,
    },
    {
      name: 'personal-owner-zh-320-dark',
      role: 1,
      language: 'zh',
      width: 320,
      theme: 'dark',
      channelRead: false,
    },
    {
      name: 'personal-root-zh-1280-light',
      role: 100,
      language: 'zh',
      width: 1280,
      theme: 'light',
      channelRead: true,
    },
    {
      name: 'personal-owner-zh-TW-320-light',
      role: 1,
      language: 'zh-TW',
      width: 320,
      theme: 'light',
      channelRead: false,
    },
    {
      name: 'personal-root-zh-TW-1280-dark',
      role: 100,
      language: 'zh-TW',
      width: 1280,
      theme: 'dark',
      channelRead: true,
    },
    {
      name: 'personal-owner-fr-320-dark',
      role: 1,
      language: 'fr',
      width: 320,
      theme: 'dark',
      channelRead: false,
    },
    {
      name: 'personal-root-fr-1280-light',
      role: 100,
      language: 'fr',
      width: 1280,
      theme: 'light',
      channelRead: true,
    },
    {
      name: 'personal-owner-ru-320-light',
      role: 1,
      language: 'ru',
      width: 320,
      theme: 'light',
      channelRead: false,
    },
    {
      name: 'personal-root-ru-1280-dark',
      role: 100,
      language: 'ru',
      width: 1280,
      theme: 'dark',
      channelRead: true,
    },
    {
      name: 'personal-owner-ja-320-dark',
      role: 1,
      language: 'ja',
      width: 320,
      theme: 'dark',
      channelRead: false,
    },
    {
      name: 'personal-root-ja-1280-light',
      role: 100,
      language: 'ja',
      width: 1280,
      theme: 'light',
      channelRead: true,
    },
    {
      name: 'personal-owner-vi-320-light',
      role: 1,
      language: 'vi',
      width: 320,
      theme: 'light',
      channelRead: false,
    },
    {
      name: 'personal-root-vi-1280-dark',
      role: 100,
      language: 'vi',
      width: 1280,
      theme: 'dark',
      channelRead: true,
    },
  ].map((scenario) => ({
    ...scenario,
    mode: 'disabled',
    savedPlaygroundKeyId: 22,
  }))
  const requiredPairs = new Set(
    Object.keys(languageCodes).flatMap((language) =>
      [320, 1280].map((width) => `${language}:${width}`)
    )
  )
  const populatedPairs = new Set(
    populatedScenarios.map(
      (scenario) => `${scenario.language}:${scenario.width}`
    )
  )
  assert.equal(populatedScenarios.length, 14)
  assert.equal(
    new Set(populatedScenarios.map((scenario) => scenario.name)).size,
    14
  )
  assert.deepEqual(
    [...populatedPairs].sort(),
    [...requiredPairs].sort(),
    'The bounded populated matrix covers every required language at 320px and 1280px exactly once'
  )
  assert(
    populatedScenarios.some(
      (scenario) => scenario.role === 10 && !scenario.channelRead
    ),
    'The populated matrix preserves the administrator permission-denied scenario'
  )
  report.coverage = {
    languages: [...Object.keys(languageCodes)],
    widths: [320, 1280],
    populatedPairs: [...populatedPairs].sort(),
    handoffCases: ['available', 'key-unavailable', 'safe-list-failed'],
  }
  for (const scenario of populatedScenarios) await runPopulated(scenario)

  await runBlockedHandoff({
    name: 'personal-handoff-key-unavailable-en-1280',
    role: 1,
    language: 'en',
    width: 1280,
    theme: 'light',
    mode: 'disabled',
    channelRead: false,
    savedPlaygroundKeyId: 22,
    handoffKeyUnavailable: true,
  })
  await runBlockedHandoff({
    name: 'personal-handoff-safe-list-failed-zh-320',
    role: 1,
    language: 'zh',
    width: 320,
    theme: 'dark',
    mode: 'disabled',
    channelRead: false,
    savedPlaygroundKeyId: 22,
    failPlaygroundKeys: true,
  })
  await runRecovery({
    name: 'personal-retry-zh-320-light',
    role: 100,
    language: 'zh',
    width: 320,
    theme: 'light',
    mode: 'disabled',
    channelRead: true,
    failKeys: true,
    failRecent: true,
    failAccounts: true,
    failBudgetOnce: true,
  })
  await runEmpty({
    name: 'personal-empty-en-1280-light',
    role: 100,
    language: 'en',
    width: 1280,
    theme: 'light',
    mode: 'disabled',
    channelRead: true,
    empty: true,
  })
  await runUnknownBudget({
    name: 'personal-unknown-budget-zh-320-dark',
    role: 1,
    language: 'zh',
    width: 320,
    theme: 'dark',
    mode: 'disabled',
    channelRead: false,
    unknownBudget: true,
  })
  await runEmpty({
    name: 'personal-long-policy-fr-320-dark',
    role: 100, language: 'fr', width: 320, theme: 'dark',
    mode: 'disabled', channelRead: true, empty: true, storedAllowance: true,
  })
  for (const mode of ['enabled', 'retirement', 'unknown']) {
    await runGate({
      name: `legacy-${mode}-gate`,
      role: 1,
      language: 'en',
      width: 1280,
      theme: 'light',
      mode,
      channelRead: false,
    })
  }

  assert.equal(report.journeys.length, 23)
  assert.equal(report.screenshots.length, 46)
  assert.equal(report.buildRevisions.length, 23)
  assert(report.buildRevisions.every((entry) => entry.revision))
  const populatedNames = new Set(
    populatedScenarios.map((scenario) => scenario.name)
  )
  assert.deepEqual(
    new Set(
      report.journeys
        .filter((journey) => populatedNames.has(journey.name))
        .map((journey) => `${journey.language}:${journey.width}`)
    ),
    requiredPairs,
    'The completed Chromium journeys cover the required locale-width pairs'
  )
  assert.deepEqual(
    new Set(
      report.screenshots
        .filter((entry) => populatedNames.has(entry.journey))
        .map((entry) => entry.journey)
    ),
    populatedNames,
    'Every locale-width populated journey produces real layout screenshots'
  )
  assert.deepEqual(
    new Set(
      report.journeys
        .filter((journey) => journey.name.startsWith('personal-handoff-'))
        .map((journey) => journey.name)
    ),
    new Set([
      'personal-handoff-key-unavailable-en-1280',
      'personal-handoff-safe-list-failed-zh-320',
    ]),
    'Both blocked handoff browser cases complete'
  )
  report.result = 'passed'
} catch (error) {
  report.result = 'failed'
  report.errors.push(error instanceof Error ? error.stack : String(error))
  throw error
} finally {
  for (const context of contexts) await context.close().catch(() => {})
  if (browser) await browser.close()
  if (server.listening) await new Promise((done) => server.close(done))
  persist()
  console.log(JSON.stringify(report))
}
