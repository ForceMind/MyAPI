// Production-build Chromium with isolated personal-console APIs: the single
// overview, the Key inspector's Playground handoff and the account windows.
// This is synthetic UI evidence only; it never contacts a provider or settles
// usage. (The separate personal workbench was merged into the overview.)
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
import { assertTextContrast } from './contrast.mjs'

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
  "API key",
  "Account window used",
  "Account windows",
  "Approximate",
  "Channel read permission is required to view upstream account windows.",
  "Choose a Key",
  "Choose a Key to observe its native window share.",
  "Create an API Key for your client",
  "Enabled connections",
  "Error",
  "Existing channel logins are kept. Open Channels to inspect the connection.",
  "Failed to load API keys",
  "Key share settings",
  "Loading upstream accounts",
  "Local Key estimate",
  "Manage Key shares",
  "Manage account connection",
  "Most used account windows",
  "Needs attention",
  "Next steps",
  "No requests in the last 24 hours",
  "No timestamp",
  "No upstream account windows were reported",
  "Observed {{time}}",
  "Open Playground",
  "Overview",
  "Pending requests",
  "Recent requests",
  "Refresh account windows",
  "Request failed",
  "Request successful",
  "Retry",
  "Sample age",
  "Select an API Key",
  "Select an available API key before sending",
  "Select upstream account",
  "Shares are approximate and do not reserve upstream capacity. Concurrent requests and sampling delays can exceed the share.",
  "Snapshot available",
  "Snapshot unavailable",
  "Stale sample",
  "Table view",
  "The selected API key is unavailable. Choose another key.",
  "This Key allocation",
  "This Key has no share for the selected account.",
  "This window needs new upstream evidence before it can be used.",
  "Total allocated",
  "Unable to confirm channel permissions",
  "Unable to load Key shares",
  "Unable to load Keys",
  "Unable to load recent requests",
  "Unable to load upstream account windows",
  "Unallocated",
  "Unattributed growth",
  "Unknown",
  "Unsampled requests",
  "Upstream account usage is not exact per-Key attribution. Windows are observed separately.",
  "Upstream accounts",
  "Usage not provided",
  "Window observations are unavailable; existing estimates are preserved.",
  "Window resets require a trustworthy upstream sample.",
  "{{count}} requests completed without a usage report",
  "{{count}} Tokens",
  "{{count}} percentage points",
  "{{count}} seconds"
]) {
  for (const language of Object.keys(languageCodes)) tr(key, language)
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

// The Keys page's list and Key inspector read the masked Key object; no
// secret is part of it.
function keyListItem(key, fixture) {
  return { ...key, user_id: fixture.user.id, key: 'masked', used_quota: 0, created_time: now - 3600, accessed_time: now - 60, model_limits_enabled: false, model_limits: '', allow_ips: '' }
}

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
  contrast: [],
  diagnostics: [],
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

function poolUsageFixture(scenario) {
  const usage = {
    pool_id: 7, snapshot_id: 9, revision: 1, channel_id: 1, channel_name: 'Synthetic subscription account',
    window_slot: 'primary', window_type: 'five_hour', window_seconds: 18000, plan_type: 'pro', generation: 1,
    state: 'active', reason: 'eligible', observed_at: now - 60, reset_at: now + 3600, max_age_seconds: 300, sample_age_seconds: 60,
    allocated_bps: 6000, approximate: true,
    baseline_time: now - 300, baseline_used_micro: 43000000,
    initial_baseline_time: now - 600, initial_baseline_used_micro: 35000000,
    observed_used_micro: 55000000, estimated_used_micro: 15000000, unattributed_micro: 40000000, unattributed_growth_micro: 5000000,
    decomposition_state: 'available', pending_attempts: 2, unsampled_attempts: 2, uncertain_attempts: 1,
    allocations: [
      { token_id: 21, token_name: safeKeys[0].name, token_state: 'enabled', share_bps: 1000, estimated_used_micro: 5000000, pending_attempts: 2, unsampled_attempts: 1, uncertain_attempts: 0 },
      { token_id: 22, token_name: safeKeys[1].name, token_state: 'paused', share_bps: 3000, estimated_used_micro: 7000000, pending_attempts: 0, unsampled_attempts: 1, uncertain_attempts: 0 },
      { token_id: 23, token_name: safeKeys[2].name, token_state: 'enabled', share_bps: 2000, estimated_used_micro: 3000000, pending_attempts: 0, unsampled_attempts: 0, uncertain_attempts: 1 },
    ],
  }
  if (scenario.poolUsageMode === 'baseline_pending') {
    Object.assign(usage, { state: 'baseline_pending', decomposition_state: 'baseline_pending', baseline_time: 0, baseline_used_micro: null, initial_baseline_time: null, initial_baseline_used_micro: null, observed_used_micro: null, estimated_used_micro: null, unattributed_micro: null, unattributed_growth_micro: null })
    for (const allocation of usage.allocations) allocation.estimated_used_micro = null
  }
  if (scenario.poolUsageMode === 'baseline_unknown') Object.assign(usage, { decomposition_state: 'baseline_unknown', initial_baseline_time: null, initial_baseline_used_micro: null, unattributed_growth_micro: null })
  if (scenario.poolUsageMode === 'inconsistent') {
    Object.assign(usage, { state: 'discontinuity', decomposition_state: 'inconsistent', estimated_used_micro: null, pending_attempts: 3 })
    usage.allocations[0].estimated_used_micro = null
  }
  if (scenario.poolUsageMode === 'zero-share') {
    usage.allocated_bps = 3000
    Object.assign(usage.allocations[1], { token_name: null, token_state: 'deleted', share_bps: 0 })
  }
  if (scenario.poolSession) usage.allocations[0].token_name = 'Synthetic new session Key'
  if (scenario.stale) Object.assign(usage, { state: 'stale', observed_at: now - 600, sample_age_seconds: 600 })
  return usage
}

function nativeWindowStatesFixture() {
  const primary = { metric_type: 'codex_rate_limit', unit: 'percent', window_type: 'five_hour', window_seconds: 18000, source: 'codex_wham_usage_primary', status: 'success', observed_at: now - 60 }
  const secondary = { ...primary, window_type: 'weekly', window_seconds: 604800, source: 'codex_wham_usage_secondary' }
  return [
    { ...primary, channel_id: 7, series_id: '7a'.padStart(64, '0'), name: 'Synthetic failed subscription account with a long descriptive identity', status: 'error', current_available: null },
    { ...secondary, channel_id: 7, series_id: '7b'.padStart(64, '0'), name: 'Synthetic failed subscription account with a long descriptive identity', status: 'unsupported', current_available: null },
    { ...primary, channel_id: 8, series_id: '8a'.padStart(64, '0'), name: 'Synthetic zero and stale account', current_available: 100 },
    { ...secondary, channel_id: 8, series_id: '8b'.padStart(64, '0'), name: 'Synthetic zero and stale account', current_available: 32, observed_at: now - 600 },
    { ...primary, channel_id: 9, series_id: '9a'.padStart(64, '0'), name: 'Synthetic incomplete evidence account', status: 'unavailable', current_available: null },
    { ...secondary, channel_id: 9, series_id: '9b'.padStart(64, '0'), name: 'Synthetic incomplete evidence account', current_available: 45, observed_at: undefined },
  ]
}

function scenarioResponse(fixture, scenario, url, method, reads, pagePath) {
  const path = url.pathname.replace(/\/$/, '')
  const count = (reads.get(path) || 0) + 1
  reads.set(path, count)

  if (scenario.windowStates && path === '/api/channel') {
    assert(scenario.role === 100 && scenario.channelRead && method === 'GET', 'Only the authorized metadata read is configured')
    const template = fixture.response(url, method).body.data.items[0]
    const accounts = [...new Map(nativeWindowStatesFixture().map((item) => [item.channel_id, item])).values()]
    const items = accounts.map((item) => ({ ...template, id: item.channel_id, name: item.name, type: 57, key: '', status: 1 }))
    items.push({ ...template, id: 10, name: 'Synthetic standard API channel', type: 1, key: '', status: 1 })
    return { status: 200, body: { success: true, data: { items, total: items.length, page: 1, page_size: Number(url.searchParams.get('page_size') || 10) } } }
  }

  if (scenario.windowStates && path === '/api/channel/7/quota/history') {
    assert(scenario.role === 100 && scenario.channelRead && method === 'GET' && pagePath === '/channels', 'Only the authorized channel page reads its saved history')
    return { status: 200, body: { success: true, data: { channel_id: 7, start: now - 86400, end: now, limit: 5000, points: [], current: null, source_complete: true, points_complete: true, complete: true } } }
  }

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
  if (path === '/api/user/auth/refresh' && scenario.poolSession) {
    const response = fixture.response(url, method)
    response.body.data.session = { ...response.body.data.session, sid: 'synthetic-ui-root-new-session' }
    return response
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
  const legacyDashboardList = method === 'GET' && pagePath === '/dashboard/overview' && ['enabled', 'retirement'].includes(scenario.mode)
  // Only the Keys page (its list and the Key inspector) reads Key objects;
  // the overview counts Keys from the safe /pg/keys list.
  const keysPage = method === 'GET' && pagePath === '/keys'
  const keyObject = path.match(/^\/api\/token\/(\d+)$/)
  if (keysPage && keyObject) {
    const key = safeKeys.find((item) => item.id === Number(keyObject[1]))
    if (!key || url.search) return { violation: `Unexpected Key read: ${path}${url.search}` }
    return { status: 200, body: { success: true, data: keyListItem(key, fixture) } }
  }
  if (keysPage && ['/api/token', '/api/token/search'].includes(path)) {
    const items = scenario.empty ? [] : safeKeys.map((key) => keyListItem(key, fixture))
    return { status: 200, body: { success: true, data: { items, total: items.length, page: 1, page_size: 20 } } }
  }
  if (keyObject ||
      (['/api/token', '/api/token/search'].includes(path) && !legacyDashboardList)) {
    return {
      violation:
        'The overview requested a broad Key owner object instead of the safe list DTO',
    }
  }
  if (path === '/api/log/overview') {
    return { violation: 'The overview requested the all-user log overview' }
  }
  // Administrators read every user's records, like the call records page;
  // ordinary users read only their own.
  if (path === '/api/log/self' || (path === '/api/log' && scenario.role >= 10)) {
    const sample = url.searchParams.get('page_size') !== '1'
    if (sample) scenario.sampleReads = (scenario.sampleReads || 0) + 1
    if (scenario.failRecent && sample && scenario.sampleReads === 1) return { status: 503, body: { success: false, message: 'Synthetic recent calls unavailable' } }
    const rows = scenario.empty || url.searchParams.has('token_id') ? [] : [
      { id: 501, user_id: fixture.user.id, created_at: now - 120, type: 5, content: '', token_name: 'Synthetic desktop app', model_name: 'synthetic-personal-model', prompt_tokens: 0, completion_tokens: 0, use_time: 2, other: JSON.stringify({ request_outcome: 'completed', usage_outcome: 'unavailable', settlement_status: 'pending_review' }) },
      { id: 502, user_id: fixture.user.id, created_at: now - 60, type: 5, content: 'Synthetic upstream 429', token_name: 'Synthetic desktop app', model_name: 'synthetic-error-model', prompt_tokens: 0, completion_tokens: 0, use_time: 1, other: JSON.stringify({ request_outcome: 'failed', usage_outcome: 'unavailable' }) },
    ]
    const type = url.searchParams.get('type')
    const items = type ? rows.filter((row) => String(row.type) === type) : rows
    return { status: 200, body: { success: true, data: { items: sample ? items : [], total: items.length, page: 1, page_size: Number(url.searchParams.get('page_size') || 10) } } }
  }
  const shareMatch = path.match(/^\/api\/token\/(\d+)\/subscription-share$/)
  if (shareMatch && Number(shareMatch[1]) === 24) return { status: 200, body: { success: true, data: { token_id: 24, pools: [{ pool_id: 0, snapshot_id: 9, revision: 0, channel_id: 1, channel_name: 'Synthetic subscription account', plan_type: 'pro', window_slot: 'primary', window_type: 'five_hour', window_seconds: 18000, reset_at: now + 3600, observed_at: now - 60, max_age_seconds: 300, sample_age_seconds: 60, state: 'candidate', allocated_bps: 0, share_bps: 0, estimated_used_micro: 0, unattributed_micro: 0, pending_attempts: 0, unsampled_attempts: 0, approximate: true, baseline_time: 0 }] } } }
  if (shareMatch) {
    const pool = { pool_id: 7, snapshot_id: 9, revision: 1, channel_id: 1, channel_name: 'Synthetic subscription account', plan_type: 'pro', window_slot: 'primary', window_type: 'five_hour', window_seconds: 18000, reset_at: now + 3600, observed_at: now - 60, max_age_seconds: 300, sample_age_seconds: 60, state: 'active', allocated_bps: 6000, share_bps: 2000, estimated_used_micro: 5000000, unattributed_micro: 5000000, pending_attempts: 2, unsampled_attempts: 1, approximate: true, baseline_time: now - 600 }
    if (scenario.role !== 100) { delete pool.channel_id; pool.channel_name = '' }
    return { status: 200, body: { success: true, data: { token_id: Number(shareMatch[1]), pools: [pool] } } }
  }
  const poolUsageMatch = path.match(/^\/api\/subscription-share\/pools\/(\d+)\/usage$/)
  if (poolUsageMatch) {
    if (scenario.role !== 100 || !scenario.channelRead || poolUsageMatch[1] !== '7' || url.search) return { violation: 'Unexpected root pool usage request' }
    if (scenario.poolUsageMode === 'error') return { status: 503, body: { success: false, message: 'Synthetic pool usage unavailable' } }
    return { status: 200, body: { success: true, data: poolUsageFixture(scenario) } }
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
    if (scenario.windowStates) return { status: 200, body: { success: true, data: { items: nativeWindowStatesFixture(), source_complete: true, items_complete: true, generated_at: now } } }
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
  if (path === '/api/channel/quota/changes') return { status: 200, body: { success: true, data: { items: [
    { channel_id: 1, series_id: 'a'.repeat(64), name: 'Synthetic subscription account', account_label: 'Synthetic subscription account', metric_type: 'codex_rate_limit', window_type: 'five_hour', source: 'codex_wham_usage_primary', window_seconds: 18000, unit: 'percent', status: 'success', current_available: 45, observed_at: now - 60 },
    { channel_id: 1, series_id: 'b'.repeat(64), name: 'Synthetic subscription account', account_label: 'Synthetic subscription account', metric_type: 'codex_rate_limit', window_type: 'weekly', source: 'codex_wham_usage_secondary', window_seconds: 604800, unit: 'percent', status: 'success', current_available: 32, observed_at: scenario.stale ? now - 600 : now - 60 },
  ], generated_at: now } } }
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
  const mobile = scenario.width < 768
  const reads = new Map()
  const audit = {
    requests: [],
    mutations: [],
    external: [],
    violations: [],
    pageErrors: [],
    pageErrorKinds: [],
    responses: [],
    requestFailures: [],
    fixtureResponses: [],
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
      reads,
      new URL(page.url()).pathname
    )
    if (response.violation) audit.violations.push(response.violation)
    if (scenario.poolUsageMode === 'hang' && /^\/api\/subscription-share\/pools\/7\/usage$/.test(path) && response.status === 200) {
      audit.fixtureResponses.push({ method: request.method(), path, status: 'pending-until-client-timeout' })
      await new Promise((done) => {
        const cleanup = () => { page.off('requestfailed', cancelled); context.off('close', closed) }
        const cancelled = (failed) => { if (failed === request) { cleanup(); done() } }
        const closed = () => { cleanup(); done() }
        page.on('requestfailed', cancelled)
        context.on('close', closed)
      })
      // The real Axios deadline cancels this intentionally pending read. The
      // browser may already have discarded the interception when it aborts.
      await route.abort('timedout').catch(() => {})
      return
    }
    audit.fixtureResponses.push({ method: request.method(), path, status: response.status || 501 })
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
  page.on('pageerror', (error) => {
    audit.pageErrors.push(error.message)
    audit.pageErrorKinds.push(['TypeError', 'SyntaxError', 'ReferenceError', 'RangeError', 'Error'].includes(error.name) ? error.name : 'other')
  })
  page.on('response', (response) => {
    const url = new URL(response.url())
    if (url.origin === origin && (url.pathname.startsWith('/api/') || url.pathname.startsWith('/pg/'))) {
      audit.responses.push({ method: response.request().method(), path: url.pathname.replace(/\/$/, ''), status: response.status() })
    }
  })
  page.on('requestfailed', (request) => {
    const url = new URL(request.url())
    if (url.origin !== origin || (!url.pathname.startsWith('/api/') && !url.pathname.startsWith('/pg/'))) return
    const reason = request.failure()?.errorText || ''
    audit.requestFailures.push({ method: request.method(), path: url.pathname.replace(/\/$/, ''), kind: /aborted/i.test(reason) ? 'aborted' : /blocked/i.test(reason) ? 'blocked' : 'other' })
  })
  return { page, context, reads, audit, fixture }
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

async function openOverview(page, scenario) {
  await page.goto(`${origin}/dashboard/overview`, { waitUntil: 'networkidle' })
  await page.locator('[data-myapi-shell="authenticated"], [data-myapi-shell="personal"]').waitFor()
  await page.getByTestId('overview').waitFor()
  await page.getByRole('heading', { level: 1, name: tr('Overview', scenario.language), exact: true }).waitFor()
  await settled(page)
  const revision = await page.locator('html').getAttribute('data-build-rev')
  assert(revision?.startsWith(`rv.${version}.`), 'Visible revision matches VERSION')
  if (process.env.GITHUB_SHA) assert(revision.includes(process.env.GITHUB_SHA), 'Revision matches CI checkout')
  report.buildRevisions.push({ journey: scenario.name, revision })
  return revision
}

async function screenshot(page, scenario, suffix) {
  await settled(page)
  const geometry = await page.evaluate(() => {
    const sections = []
    let hiddenSections = 0
    for (const element of document.querySelectorAll('[data-testid="overview"], [data-testid="personal-upstream"], [data-slot="card"]')) {
      const box = element.getBoundingClientRect()
      // Hidden mounted cards have no rendered geometry. Include visible cards
      // below the fold as well: their content must also fit when scrolled to.
      if (box.width <= 0 || box.height <= 0 || !element.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true })) {
        hiddenSections++
        continue
      }
      const section = { tag: element.tagName, testId: element.getAttribute('data-testid'), slot: element.getAttribute('data-slot'), left: box.left, right: box.right, top: box.top, bottom: box.bottom, scroll: element.scrollWidth, client: element.clientWidth, overflowingChildren: [] }
      if (box.left < -1 || box.right > innerWidth + 1 || element.scrollWidth > element.clientWidth + 1) {
        for (const child of element.querySelectorAll('*')) {
          const childBox = child.getBoundingClientRect()
          if (childBox.width <= 0 || childBox.height <= 0 || !child.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true })) continue
          if (childBox.left >= box.left - 1 && childBox.right <= box.right + 1 && child.scrollWidth <= child.clientWidth + 1) continue
          const style = getComputedStyle(child)
          section.overflowingChildren.push({ tag: child.tagName, slot: child.getAttribute('data-slot'), testId: child.getAttribute('data-testid'), left: childBox.left, right: childBox.right, scroll: child.scrollWidth, client: child.clientWidth, display: style.display, overflowX: style.overflowX, whiteSpace: style.whiteSpace, minWidth: style.minWidth })
          if (section.overflowingChildren.length === 20) break
        }
      }
      sections.push(section)
    }
    return { viewport: innerWidth, document: document.documentElement.scrollWidth, hiddenSections, sections }
  })
  if (geometry.sections.length === 0 || geometry.document > geometry.viewport + 1 || geometry.sections.some((section) => section.left < -1 || section.right > geometry.viewport + 1 || section.scroll > section.client + 1)) {
    const prefix = `${scenario.name}-${suffix}-overflow`
    const image = `${prefix}.png`
    const data = `${prefix}.json`
    writeFileSync(resolve(output, data), JSON.stringify({ sourceHead, journey: scenario.name, suffix, geometry }, null, 2))
    await page.screenshot({ path: resolve(output, image), fullPage: true, animations: 'disabled' })
    report.diagnostics.push({ journey: scenario.name, image, geometry: data })
    persist()
  }
  assert(geometry.document <= geometry.viewport + 1, `${scenario.name}: no horizontal page overflow`)
  assert(geometry.sections.length > 0, `${scenario.name}: visible content geometry exists`)
  assert(geometry.sections.every((section) => section.left >= -1 && section.right <= geometry.viewport + 1 && section.scroll <= section.client + 1), `${scenario.name}: cards remain inside the viewport`)
  const file = `${scenario.name}-${suffix}.png`
  await page.screenshot({ path: resolve(output, file), fullPage: true })
  report.screenshots.push({ file, journey: scenario.name, sourceHead, buildRevision: report.buildRevisions.at(-1)?.revision, fixtureTime: new Date(FIXTURE_TIME).toISOString() })
  for (const semantic of ['success', 'warning', 'destructive']) {
    const candidates = page.locator(`.text-${semantic}:not(svg)`)
    const index = await candidates.evaluateAll((elements) => elements.findIndex((element) => {
      const box = element.getBoundingClientRect()
      return box.width > 0 && box.height > 0 && element.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true }) && element.textContent.trim().length > 0
    }))
    if (index < 0) continue
    const result = await assertTextContrast(candidates.nth(index), semantic, scenario.theme)
    report.contrast.push({ journey: scenario.name, snapshot: suffix, ...result })
  }
  persist()
}

// The Playground handoff starts from the Key inspector on the Keys page.
async function openKeyInspector(page, value) {
  await page.goto(`${origin}/keys?inspect=${value}`, { waitUntil: 'networkidle' })
  const inspector = page.locator('[data-inspector]')
  const key = safeKeys.find((item) => item.id === value)
  await inspector.getByRole('heading', { level: 2, name: key.name, exact: true }).waitFor()
  return inspector
}

async function screenshotPoolUsage(page, scenario, suffix) {
  await page.getByTestId('pool-usage-summary').evaluate((element) => element.scrollIntoView({ block: 'start', behavior: 'instant' }))
  await screenshot(page, scenario, `${suffix}-pool-summary`)
  const rows = page.getByTestId('pool-key-allocations').locator('li')
  assert.equal(await rows.count(), 3, 'The complete pool Key list is present')
  for (let index = 0; index < 3; index++) {
    const row = rows.nth(index)
    await row.evaluate((element) => element.scrollIntoView({ block: 'start', behavior: 'instant' }))
    const bounds = await row.boundingBox()
    const content = await page.locator('.pc-route-content').boundingBox()
    assert(bounds && content && bounds.y >= content.y - 1 && bounds.y + bounds.height <= content.y + content.height + 1, 'Every complete Key row is reachable within the scrolling content')
    await screenshot(page, scenario, `${suffix}-pool-key-${index + 1}`)
  }
  const manage = page.getByTestId('pool-key-allocations').getByText(tr('Manage Key shares', scenario.language), { exact: true })
  await manage.scrollIntoViewIfNeeded()
  const button = await manage.boundingBox()
  const content = await page.locator('.pc-route-content').boundingBox()
  assert(button && content && button.y >= content.y - 1 && button.y + button.height <= content.y + content.height + 1, 'Pool management remains reachable above the mobile navigation')
  await screenshot(page, scenario, `${suffix}-pool-actions`)
}

async function runJourney(scenario, work) {
  const session = await createSession(scenario)
  const journey = { name: scenario.name, language: scenario.language, width: scenario.width, role: scenario.role, result: 'running' }
  try {
    journey.buildRevision = await openOverview(session.page, scenario)
    await work(session)
    assert.deepEqual(session.audit.mutations, [], 'No business mutation or generation request')
    assert.deepEqual(session.audit.external, [], 'No external network request')
    assert.deepEqual(session.audit.violations, [], 'All reads respect API and permission contracts')
    assert.deepEqual(session.audit.pageErrors, [], 'No browser runtime error')
    assert.equal(await session.page.getByText(SECRET_SENTINEL).count(), 0, 'No Key secret is rendered')
    journey.reads = Object.fromEntries(session.reads)
    journey.result = 'passed'
  } catch (error) {
    journey.result = 'failed'
    journey.error = error.stack || String(error)
    journey.reads = Object.fromEntries(session.reads)
    const prefix = `${scenario.name}-failed-page`
    const evidence = {
      sourceHead, journey: scenario.name,
      reads: journey.reads,
      responses: session.audit.responses,
      fixtureResponses: session.audit.fixtureResponses,
      requestFailures: session.audit.requestFailures,
      pageErrors: { count: session.audit.pageErrors.length, kinds: session.audit.pageErrorKinds },
      violations: session.audit.violations.length,
      mutations: session.audit.mutations.length,
      externalRequests: session.audit.external.length,
    }
    try {
      const url = new URL(session.page.url())
      evidence.finalUrl = url.origin === origin ? `${url.origin}${url.pathname}` : 'outside-scope'
      evidence.states = {}
      const states = {
        loading: session.page.getByRole('status', { name: tr('Loading upstream accounts', scenario.language), exact: true }),
        permissionError: session.page.getByText(tr('Unable to confirm channel permissions', scenario.language), { exact: true }),
        permissionDenied: session.page.getByText(tr('Channel read permission is required to view upstream account windows.', scenario.language), { exact: true }),
        accountsError: session.page.getByTestId('accounts-error-state'),
        empty: session.page.getByTestId('accounts-empty-state'),
        upstream: session.page.getByTestId('personal-upstream'),
        overview: session.page.getByTestId('overview'),
      }
      for (const [name, locator] of Object.entries(states)) evidence.states[name] = await locator.first().isVisible()
      evidence.render = await session.page.evaluate(() => ({
        readyState: document.readyState,
        hidden: document.hidden,
        viewport: innerWidth,
        documentWidth: document.documentElement.scrollWidth,
        windows: [...document.querySelectorAll('[data-testid="native-window-card"]')].map((element) => {
          const box = element.getBoundingClientRect()
          return { visible: element.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true }), left: box.left, right: box.right, top: box.top, bottom: box.bottom }
        }),
      }))
      await session.page.screenshot({ path: resolve(output, `${prefix}.png`), fullPage: true, animations: 'disabled', mask: [session.page.locator('input, textarea'), session.page.getByText(SECRET_SENTINEL)] })
      evidence.image = `${prefix}.png`
    } catch {
      evidence.captureFailed = true
    }
    writeFileSync(resolve(output, `${prefix}.json`), JSON.stringify(evidence, null, 2))
    report.diagnostics.push({ journey: scenario.name, image: evidence.image || null, evidence: `${prefix}.json` })
    persist()
    throw error
  }
  finally { report.journeys.push(journey); persist(); await session.context.close(); contexts.delete(session.context) }
}

async function populated(scenario) {
  await runJourney(scenario, async ({ page, audit, reads, fixture }) => {
    const recentSection = page.getByRole('region', { name: tr('Recent requests', scenario.language), exact: true })
    await recentSection.getByText('synthetic-personal-model', { exact: false }).first().waitFor()
    await recentSection.getByText(tr('Request successful', scenario.language), { exact: true }).first().waitFor()
    await recentSection.getByText(tr('Request failed', scenario.language), { exact: true }).first().waitFor()
    assert.equal(await page.getByTestId('overview').getByText(/\b0 Tokens\b/).count(), 0, 'Missing usage is never zero Tokens')
    // The completed request without usage is listed for attention, not zeroed.
    await page.getByRole('region', { name: tr('Needs attention', scenario.language), exact: true })
      .getByText(tr('{{count}} requests completed without a usage report', scenario.language).replace('{{count}}', '1'), { exact: true }).waitFor()
    const body = await page.getByTestId('overview').innerText()
    assert(!/USD\s+\d|\$\s*\d/.test(body), 'Personal overview shows no monetary balance')
    await screenshot(page, scenario, 'overview')
    if (scenario.channelRead) {
      await page.goto(`${origin}/upstream-accounts`, { waitUntil: 'networkidle' })
      await page.getByTestId('personal-upstream').waitFor()
      // The page root also renders while fresh-self permission and its dependent
      // account request are loading. Wait for both actual windows, not that root.
      const windows = page.getByTestId('native-window-card')
      await windows.filter({ has: page.getByText('55%', { exact: true }) }).waitFor({ state: 'visible' })
      await windows.filter({ has: page.getByText('68%', { exact: true }) }).waitFor({ state: 'visible' })
      assert.equal(await windows.count(), 2, 'Native windows stay separate')
      await page.getByRole('combobox', { name: tr('Select an API Key', scenario.language), exact: true }).selectOption('23')
      const selectedKeyName = page.getByTestId('upstream-share-selected-key')
      await selectedKeyName.waitFor({ state: 'visible' })
      assert.equal(await selectedKeyName.innerText(), safeKeys.find((key) => key.id === 23).name, 'The complete selected Key name remains visible')
      assert.equal(await page.getByRole('combobox', { name: tr('Select an API Key', scenario.language), exact: true }).getAttribute('aria-describedby'), await selectedKeyName.getAttribute('id'), 'The Key picker describes its exact selected identity')
      assert(await selectedKeyName.evaluate((element) => element.scrollWidth <= element.clientWidth + 1), 'The selected Key name wraps without horizontal clipping')
      if (scenario.role === 100) {
      await page.getByTestId('pool-key-23').waitFor({ state: 'visible' })
      const points = (value) => tr('{{value}} percentage points', scenario.language).replace('{{value}}', new Intl.NumberFormat(browserLocales[scenario.language], { maximumFractionDigits: 6 }).format(value))
      assert.equal(await page.getByTestId('pool-estimated-total').innerText(), points(15), 'The local estimate is the complete Key sum')
      assert.equal(await page.getByTestId('pool-initial-baseline').innerText(), points(35), 'The immutable initial cycle observation stays separate')
      assert.equal(await page.getByTestId('pool-unattributed-growth').innerText(), points(5), 'Growth excludes the initial baseline')
      assert.equal(await page.getByTestId('pool-recovery-reference').innerText(), points(43), 'A later reference is not substituted for the initial cycle observation')
      assert.deepEqual(await page.getByTestId('pool-decomposition-bar').locator('span').evaluateAll((elements) => elements.map((element) => element.style.width)), ['15%', '35%', '5%'], 'Only mutually exclusive usage components form the bar')
      for (const key of safeKeys.slice(0, 3)) await page.getByTestId(`pool-key-${key.id}`).getByText(key.name, { exact: true }).waitFor()
      assert.equal(await page.getByTestId('pool-key-21').getByTestId('allocation-used-within-share').innerText(), '50%', 'Estimated usage is relative to the actual allocated share')
      await screenshot(page, scenario, scenario.stale ? 'upstream-stale' : 'upstream')
      await screenshotPoolUsage(page, scenario, 'upstream')
      if (scenario.language === 'en' && scenario.width === 320 && scenario.theme === 'light') {
        const poolPath = '/api/subscription-share/pools/7/usage'
        const section = page.getByTestId('pool-usage-7')
        const refresh = section.getByRole('button', { name: tr('Refresh', scenario.language), exact: true })
        for (const mode of ['baseline_pending', 'baseline_unknown', 'inconsistent', 'zero-share', 'hang', 'error']) {
          scenario.poolUsageMode = mode
          await refresh.click()
          if (mode === 'error' || mode === 'hang') {
            await page.getByTestId('pool-usage-error').waitFor({ state: 'visible', timeout: 20000 })
            assert.equal(await page.getByTestId('pool-decomposition-bar').count(), 0, 'An API failure does not retain a misleading successful chart')
          } else if (mode === 'zero-share') {
            await page.getByTestId('pool-key-22').getByText(tr('Deleted', scenario.language), { exact: true }).waitFor()
            assert.equal(await page.getByTestId('pool-key-22').getByTestId('allocation-estimated-usage').innerText(), points(7), 'Deleted zero-share Keys retain prior estimates')
            assert.equal(await page.getByTestId('pool-key-22').getByTestId('allocation-used-within-share').innerText(), tr('Unknown', scenario.language), 'Zero share never divides by zero')
          } else {
            const stateText = { baseline_pending: 'Waiting for a trustworthy baseline', baseline_unknown: 'The first observation for this cycle is unavailable.', inconsistent: 'The window evidence is inconsistent. Values are shown separately.' }[mode]
            await page.getByTestId('pool-decomposition-unavailable').getByText(tr(stateText, scenario.language), { exact: true }).waitFor()
            assert.equal(await page.getByTestId('pool-decomposition-bar').count(), 0, 'Incomplete evidence never becomes a complete decomposition')
            if (mode !== 'baseline_unknown') assert.equal(await page.getByTestId('pool-estimated-total').innerText(), tr('Unknown', scenario.language), 'Missing usage never becomes zero')
            if (mode === 'inconsistent') await page.getByTestId('pool-preserved-estimates').waitFor()
          }
          await screenshot(page, scenario, `pool-${mode}`)
          if (mode !== 'error' && mode !== 'hang') await screenshotPoolUsage(page, scenario, `pool-${mode}`)
          if (mode === 'hang') {
            assert(audit.requestFailures.some((request) => request.path === poolPath && request.kind === 'aborted'), 'The client deadline aborts the pending read')
            scenario.poolUsageMode = 'active'
            const retry = page.getByTestId('pool-usage-error').getByRole('button', { name: tr('Retry', scenario.language), exact: true })
            await retry.focus()
            assert(await retry.evaluate((element) => document.activeElement === element), 'Pool timeout recovery is keyboard focused')
            await page.keyboard.press('Enter')
            await page.getByTestId('pool-decomposition-bar').waitFor()
            await screenshot(page, scenario, 'pool-timeout-recovered')
            await screenshotPoolUsage(page, scenario, 'pool-timeout-recovered')
          }
        }
        scenario.poolUsageMode = 'active'
        const retry = page.getByTestId('pool-usage-error').getByRole('button', { name: tr('Retry', scenario.language), exact: true })
        await retry.focus()
        assert(await retry.evaluate((element) => document.activeElement === element), 'Pool API recovery is keyboard focused')
        await page.keyboard.press('Enter')
        await page.getByTestId('pool-decomposition-bar').waitFor()
        await screenshot(page, scenario, 'pool-recovered')
        await screenshotPoolUsage(page, scenario, 'pool-recovered')
        const picker = page.getByRole('combobox', { name: tr('Select an API Key', scenario.language), exact: true })
        const beforeCandidate = reads.get(poolPath)
        await picker.selectOption('24')
        await page.getByText(tr('This Key has no share for the selected account.', scenario.language), { exact: true }).waitFor()
        assert.equal(reads.get(poolPath), beforeCandidate, 'An unconfigured candidate never requests pool usage')
        scenario.poolSession = true
        await page.goto(`${origin}/upstream-accounts`, { waitUntil: 'networkidle' })
        await page.getByTestId('native-window-card').filter({ has: page.getByText('55%', { exact: true }) }).waitFor()
        const newPicker = page.getByRole('combobox', { name: tr('Select an API Key', scenario.language), exact: true })
        assert.equal(await newPicker.inputValue(), '', 'A new session does not inherit a prior selected Key')
        assert.equal(reads.get(poolPath), beforeCandidate, 'A new session reads no pool before an explicit Key selection')
        await newPicker.selectOption('23')
        await page.getByTestId('pool-key-21').getByText('Synthetic new session Key', { exact: true }).waitFor()
        assert.equal(await page.getByTestId('pool-key-21').getByText(safeKeys[0].name, { exact: true }).count(), 0, 'Old session allocation names do not populate the new view')
        await screenshot(page, scenario, 'pool-session-isolated')
        await screenshotPoolUsage(page, scenario, 'session-isolated')
        const managementRead = page.waitForResponse((response) => new URL(response.url()).pathname === poolPath && response.request().method() === 'GET' && response.status() === 200)
        const manage = page.getByTestId('pool-key-allocations').getByText(tr('Manage Key shares', scenario.language), { exact: true })
        const managementUrl = new URL(await manage.getAttribute('href'), origin)
        const expectedContext = { connection: { connection: 1 }, pool: 7 }
        assert.equal(managementUrl.pathname, '/keys', 'Pool management targets the associated Key workspace')
        assert.deepEqual(JSON.parse(managementUrl.searchParams.get('connectionContext')), expectedContext, 'Pool management preserves saved connection and pool identity')
        await manage.focus()
        assert(await manage.evaluate((element) => document.activeElement === element), 'Pool management is keyboard focused')
        await page.keyboard.press('Enter')
        await page.waitForURL((url) => url.pathname === '/keys' && url.searchParams.has('connectionContext'))
        await managementRead
        await page.getByRole('heading', { name: `${tr('Associated Key shares', scenario.language)} · #1`, exact: true }).waitFor()
        await page.getByTestId('pool-key-21').getByText('Synthetic new session Key', { exact: true }).waitFor()
        assert.deepEqual(JSON.parse(new URL(page.url()).searchParams.get('connectionContext')), expectedContext, 'Keyboard navigation retains the scoped relationship')
        assert.equal(audit.requests.filter((request) => request.path === '/api/token' || request.path === '/api/token/search').length, 0, 'Associated navigation never reads a broad Key management list')
        await screenshot(page, scenario, 'pool-manage-keyboard')
      }
      } else {
        await page.getByText(tr('Root access is required to match Key shares to this account.', scenario.language), { exact: true }).waitFor()
        assert.equal(await page.getByTestId('pool-usage-7').count(), 0, 'Native window permission does not reveal Root usage')
        await screenshot(page, scenario, 'upstream-without-root-usage')
      }
      await openOverview(page, scenario)
    } else assert.equal(audit.requests.filter((request) => request.path === '/api/channel/quota/changes').length, 0, 'No channel read without permission')
    for (const keyId of [24, 25, 26]) {
      const inspector = await openKeyInspector(page, keyId)
      assert(await inspector.getByRole('button', { name: tr('Open Playground', scenario.language), exact: true }).isDisabled(), 'Unavailable Key cannot open Playground')
    }
    const inspector = await openKeyInspector(page, 23)
    const modelResponse = page.waitForResponse((response) => new URL(response.url()).pathname === '/pg/models' && response.request().headers()['x-myapi-key-id'] === '23')
    const open = inspector.getByRole('button', { name: tr('Open Playground', scenario.language), exact: true })
    await open.focus(); await page.keyboard.press('Enter')
    await page.waitForURL(`${origin}/playground`)
    await assertPlaygroundSelection(page, scenario, 23)
    await modelResponse
    assert.equal(await storedPlaygroundKeyId(page, scenario.role), 23, 'Handoff replaces previously saved Key 22')
    assert.equal(audit.requests.filter((request) => request.path === '/pg/models' && request.keyId === '22').length, 0, 'No fallback to an older wider Key')
    assert.equal(audit.requests.filter((request) => request.path === '/api/log/overview').length, 0, 'The overview never reads the all-user log overview')
    // Administrators see every user's requests, as on the call records page;
    // ordinary users only ever read their own.
    if (scenario.role < 10) assert.equal(audit.requests.filter((request) => request.path === '/api/log').length, 0, 'Ordinary users read only their own records')
    assert.equal(audit.requests.filter((request) => /^\/api\/token\/\d+\/budget$/.test(request.path)).length, 0, 'Overview does not load monetary budget forms')
    if (scenario.role !== 100) assert.equal(audit.requests.filter((request) => /^\/api\/subscription-share\/pools\//.test(request.path)).length, 0, 'Non-Root sessions never request full pool usage')
    if (scenario.role === 10) {
      // This second permission state uses a new authenticated document whose
      // API fixture grants channel.read; no application auth state is injected.
      fixture.user.permissions.admin_permissions.channel.read = true
      scenario.channelRead = true
      await page.goto(`${origin}/upstream-accounts`, { waitUntil: 'networkidle' })
      await page.getByTestId('native-window-card').filter({ has: page.getByText('55%', { exact: true }) }).waitFor()
      const windows = page.getByTestId('native-window-card')
      await windows.filter({ has: page.getByText('68%', { exact: true }) }).waitFor()
      assert.equal(await windows.count(), 2, 'An authorized admin can view native account windows')
      await page.getByRole('combobox', { name: tr('Select an API Key', scenario.language), exact: true }).selectOption('23')
      await page.getByTestId('upstream-share-selected-key').waitFor()
      await page.getByText(tr('Root access is required to match Key shares to this account.', scenario.language), { exact: true }).waitFor()
      assert.equal(audit.requests.filter((request) => /^\/api\/subscription-share\/pools\//.test(request.path)).length, 0, 'channel.read never grants Root pool usage permission')
      assert.equal(await page.getByTestId('pool-usage-7').count(), 0, 'Admin sees no Root pool chart')
      await screenshot(page, scenario, 'admin-channel-reader')
    }
  })
}

async function blockedHandoff(scenario) {
  await runJourney(scenario, async ({ page, reads, audit }) => {
    const inspector = await openKeyInspector(page, 23)
    await inspector.getByRole('button', { name: tr('Open Playground', scenario.language), exact: true }).click()
    await page.waitForURL(`${origin}/playground`)
    const keySelect = page.getByRole('combobox', { name: tr('API key', scenario.language), exact: true })
    await keySelect.waitFor()
    const notice = scenario.failPlaygroundKeys ? 'Failed to load API keys' : 'The selected API key is unavailable. Choose another key.'
    await page.getByRole('status').getByText(tr(notice, scenario.language), { exact: false }).waitFor()
    assert.equal(await keySelect.inputValue(), '', 'Failed handoff leaves no selected Key')
    assert.equal(await storedPlaygroundKeyId(page, scenario.role), null, 'Failed handoff clears older saved Key')
    assert.equal(reads.get('/pg/keys'), 2, 'Playground validates against one fresh safe list')
    assert.equal(audit.requests.filter((request) => request.path === '/pg/models').length, 0, 'Failed handoff loads no models')
  })
}

async function inspectWindowStates(scenario) {
  await runJourney(scenario, async ({ page, reads }) => {
    const searchControl = page.locator('header[data-myapi-header]').getByRole('button', { name: tr('Search', scenario.language), exact: true })
    const search = await searchControl.boundingBox()
    if (scenario.width < 1024) {
      assert(search && search.width === search.height && search.width >= 44, 'The mobile console search is a compact square control with an accessible target')
    } else {
      // Design 5.1: the PC header carries a wide global search bar with its Ctrl/Cmd K shortcut.
      assert(search && search.width > search.height * 3 && search.height >= 36, 'The desktop console search is a wide bar with a pointer-sized target')
      assert.match(await searchControl.getAttribute('aria-keyshortcuts') ?? '', /^(Control|Meta)\+K$/, 'The desktop console search announces its keyboard shortcut')
    }
    // The overview's health strip carries the connection totals; the
    // connection list itself lives on the Connections page (design 8.1).
    const health = page.locator('[aria-labelledby="overview-health-title"]')
    await health.getByText(tr('Enabled connections', scenario.language), { exact: true }).waitFor()
    await health.getByText('4 / 4', { exact: true }).waitFor()
    assert.equal(reads.get('/api/channel'), 2, 'The overview reads only the configured and the enabled connection totals')
    await screenshot(page, scenario, 'overview-connections')
    const savedHistory = page.waitForResponse((response) => new URL(response.url()).pathname === '/api/channel/7/quota/history' && response.request().method() === 'GET' && response.status() === 200)
    // Design 8.1: "Add connection" is a page action beside the overview title.
    await page.locator('main#content').getByRole('link', { name: tr('Add connection', scenario.language), exact: true }).first().click()
    await page.waitForURL('**/connections')
    const inventory = page.getByTestId('connections-inventory')
    await inventory.waitFor()
    await inventory.locator('[data-connection-id="10"]').filter({ hasText: 'Synthetic standard API channel' }).waitFor()
    await page.getByRole('link', { name: tr('Table view', scenario.language), exact: true }).click()
    await page.waitForURL('**/channels')
    await page.waitForFunction((name) => [...document.querySelectorAll('main *')].some((element) => element.textContent?.trim() === name && element.checkVisibility()), 'Synthetic standard API channel')
    const history = await (await savedHistory).json()
    assert.equal(history.data.channel_id, 7, 'The channel page loads the selected synthetic account history')
    assert.equal(reads.get('/api/channel/7/quota/history'), 1, 'The existing history chart performs one saved-data read before returning')
    await page.goBack()
    await page.getByTestId('connections-inventory').waitFor()
    await page.goBack()
    await page.getByTestId('overview').waitFor()
    await page.goto(`${origin}/upstream-accounts`, { waitUntil: 'networkidle' })
    await page.getByText(tr('Usage sample failed', scenario.language), { exact: true }).waitFor()
    await page.getByText(tr('Window reporting unsupported', scenario.language), { exact: true }).waitFor()
    const snapshotReads = reads.get('/api/channel/quota/changes')
    const windows = page.getByTestId('native-window-card')
    assert.equal(await windows.count(), 2, 'The failed and unsupported account windows remain separate')
    assert.equal(await windows.getByRole('link', { name: tr('View channel history', scenario.language), exact: true }).first().getAttribute('href'), '/channels?quotaChannelId=7', 'History locates the original channel identity')
    await screenshot(page, scenario, 'failed-and-unsupported')
    // One account list for every width (no native select on phones).
    const selectAccount = async (name) => {
      const account = page.getByRole('complementary', { name: tr('Select upstream account', scenario.language), exact: true }).getByRole('button', { name, exact: true })
      await account.click()
      assert.equal(await account.getAttribute('aria-pressed'), 'true')
      await page.getByRole('heading', { name, exact: true }).waitFor()
    }
    await selectAccount('Synthetic zero and stale account')
    await windows.getByText('0%', { exact: true }).waitFor()
    await windows.getByText(tr('Stale sample', scenario.language), { exact: true }).waitFor()
    await windows.getByText(tr('Last reported usage', scenario.language), { exact: true }).waitFor()
    await screenshot(page, scenario, 'zero-and-stale')
    await selectAccount('Synthetic incomplete evidence account')
    await windows.getByText(tr('No reliable usage sample', scenario.language), { exact: true }).waitFor()
    await windows.getByText(tr('Sample time unverified', scenario.language), { exact: true }).waitFor()
    assert.equal(await windows.getByText('0%', { exact: true }).count(), 0, 'Missing evidence is never replaced with zero')
    assert.equal(reads.get('/api/channel/quota/changes'), snapshotReads, 'Switching account presentations does not duplicate snapshot reads')
    await screenshot(page, scenario, 'incomplete-evidence')
  })
}

try {
  await new Promise((done) => server.listen(0, '127.0.0.1', done))
  origin = `http://127.0.0.1:${server.address().port}`
  const { chromium } = await import(process.env.MYAPI_PLAYWRIGHT_MODULE ? pathToFileURL(process.env.MYAPI_PLAYWRIGHT_MODULE).href : 'playwright')
  browser = await chromium.launch({ headless: true, executablePath: process.env.MYAPI_CHROMIUM_PATH || undefined, args: ['--disable-dev-shm-usage'] })
  const base = { role: 100, mode: 'disabled', theme: 'light', channelRead: true, savedPlaygroundKeyId: 22 }
  const matrix = Object.keys(languageCodes).flatMap((language) => [320, 1280].map((width) => ({ ...base, name: `personal-${language}-${width}`, language, width })))
  matrix.push(...[390, 1440].map((width) => ({ ...base, name: `personal-zh-${width}`, language: 'zh', width, stale: width === 390 })))
  matrix.push({ ...base, name: 'personal-zh-320-dark', language: 'zh', width: 320, theme: 'dark' }, { ...base, name: 'personal-en-1280-dark', language: 'en', width: 1280, theme: 'dark' })
  report.coverage = { languages: Object.keys(languageCodes), widths: [320, 390, 1280, 1440], handoffCases: ['available', 'key-unavailable', 'safe-list-failed'], poolUsageCases: ['available', 'baseline_pending', 'baseline_unknown', 'inconsistent', 'zero-share', 'timeout', 'error', 'recovered', 'candidate-not-read', 'session-isolated', 'non-root-not-read', 'admin-channel-read-without-root'] }
  report.coverage.widths = [320, 390, 768, 1024, 1280, 1440]
  report.coverage.nativeWindowStates = ['error', 'unsupported', 'available-zero', 'stale', 'unavailable', 'unverified-time']
  for (const item of [{ language: 'fr', width: 320 }, { language: 'zh', width: 768 }, { language: 'en', width: 1024 }]) {
    await inspectWindowStates({ ...base, ...item, name: `platform-p2-${item.language}-${item.width}`, windowStates: true })
  }
  for (const scenario of matrix) await populated(scenario)
  for (const role of [1, 10]) await populated({ ...base, name: `personal-permission-${role}`, role, channelRead: false, language: 'en', width: 320 })
  await blockedHandoff({ ...base, name: 'personal-handoff-key-unavailable-en-1280', role: 1, channelRead: false, language: 'en', width: 1280, handoffKeyUnavailable: true })
  await blockedHandoff({ ...base, name: 'personal-handoff-safe-list-failed-zh-320', role: 1, channelRead: false, language: 'zh', width: 320, failPlaygroundKeys: true })
  const recovery = { ...base, name: 'personal-retry-zh-320', language: 'zh', width: 320, failKeys: true, failRecent: true, failAccounts: true }
  await runJourney(recovery, async ({ page, reads }) => {
    // The overview lists no Keys (design 8.1), so the failed Key count has
    // no section of its own to retry; recent requests and windows do.
    const recentSection = page.getByRole('region', { name: tr('Recent requests', recovery.language), exact: true })
    const recentError = recentSection.getByRole('alert').filter({ hasText: tr('Unable to load recent requests', recovery.language) })
    await recentError.waitFor()
    await page.getByTestId('accounts-error-state').waitFor()
    await screenshot(page, recovery, 'errors')
    await recentError.getByRole('button', { name: tr('Retry', recovery.language), exact: true }).click()
    await recentError.waitFor({ state: 'hidden' })
    await recentSection.getByText('synthetic-personal-model', { exact: false }).first().waitFor()
    await page.getByTestId('accounts-error-state').getByRole('button', { name: tr('Retry', recovery.language), exact: true }).click()
    await page.getByTestId('accounts-error-state').waitFor({ state: 'hidden' })
    await screenshot(page, recovery, 'recovered')
    assert.equal(recovery.sampleReads, 2, 'The failed request sample retries once')
    assert.equal(reads.get('/api/channel/quota/changes'), 2, 'Failed account windows retry once')
  })
  const empty = { ...base, name: 'personal-empty-en-1280', language: 'en', width: 1280, empty: true }
  await runJourney(empty, async ({ page }) => {
    await page.getByRole('region', { name: tr('Recent requests', empty.language), exact: true })
      .getByText(tr('No requests in the last 24 hours', empty.language), { exact: true }).waitFor()
    await page.getByTestId('accounts-empty-state').waitFor()
    // No Keys yet: the next step points at creating one.
    await page.getByRole('link', { name: tr('Create an API Key for your client', empty.language), exact: true }).waitFor()
    await screenshot(page, empty, 'empty')
  })
  for (const mode of ['enabled', 'retirement', 'unknown']) {
    const scenario = { ...base, name: `legacy-${mode}-gate`, role: 1, channelRead: false, language: 'en', width: 1280, mode }
    const session = await createSession(scenario)
    try {
      await session.page.goto(`${origin}/dashboard/overview`, { waitUntil: 'networkidle' })
      await session.page.locator('[data-myapi-shell="authenticated"], [data-myapi-shell="personal"]').waitFor()
      // Every funding mode mounts the same overview. Funds appear only once
      // the mode is confirmed as commercial; an unknown mode reads none.
      await session.page.getByTestId('overview').waitFor()
      const funds = session.audit.requests.filter((request) => ['/api/data/self', '/api/data', '/api/perf-metrics/summary', '/api/user/topup/self', '/api/subscription/self'].includes(request.path))
      if (mode === 'unknown') assert.equal(funds.length, 0, 'Unknown mode reads no funds or analytics data')
      assert.equal(session.audit.requests.filter((request) => /^\/api\/token\/\d+\/subscription-share$/.test(request.path)).length, 0, 'The overview does not request personal share pools')
      assert.equal(session.audit.requests.filter((request) => /^\/api\/subscription-share\/pools\//.test(request.path)).length, 0, 'The overview does not request full pool usage')
      assert.equal(session.audit.requests.filter((request) => ['/api/log/self/overview', '/api/log/overview', '/api/log'].includes(request.path)).length, 0, 'An ordinary user reads only their own records')
      assert.equal(session.audit.requests.filter((request) => request.path === '/api/channel/quota/changes').length, 0, 'No account windows without channel read')
      assert.deepEqual(session.audit.mutations, [], 'Gate has no business mutation')
      assert.deepEqual(session.audit.external, [], 'Gate has no external calls')
      assert.deepEqual(session.audit.violations, [], 'Gate respects fixture contracts')
      assert.deepEqual(session.audit.pageErrors, [], 'Gate has no runtime errors')
      report.journeys.push({ name: scenario.name, language: 'en', width: 1280, result: 'passed', reads: Object.fromEntries(session.reads) })
    } finally { await session.context.close(); contexts.delete(session.context); persist() }
  }
  report.result = 'passed'
} catch (error) { report.result = 'failed'; report.errors.push(error.stack || String(error)); throw error }
finally {
  for (const context of contexts) await context.close().catch(() => {})
  if (browser) await browser.close()
  if (server.listening) await new Promise((done) => server.close(done))
  persist()
  console.log(JSON.stringify(report))
}
