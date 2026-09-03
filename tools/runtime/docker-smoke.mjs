#!/usr/bin/env node

// Initializes only a fresh, explicitly selected loopback SQLite test instance.
// No credentials, cookies or API response bodies are returned in the report.
import { randomBytes } from 'node:crypto'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { runRuntimeProbe } from './auth-probe.mjs'

export function validateSmokeTarget(baseUrl, edition, sha) {
  let target
  try { target = new URL(baseUrl) } catch { throw new Error('INVALID_ISOLATED_SMOKE_TARGET') }
  if (target.protocol !== 'http:' || target.hostname !== '127.0.0.1' || !target.port || Number(target.port) <= 0 ||
      target.username || target.password || target.pathname !== '/' || target.search || target.hash ||
      !['full', 'lan'].includes(edition) || !/^[a-f0-9]{40}$/.test(sha || '')) {
    throw new Error('INVALID_ISOLATED_SMOKE_TARGET')
  }
  return target.origin
}

const fixture = Object.freeze({
  userName: 'smokeuser',
  userDisplayName: 'Smoke User',
  userPasswordBytes: 9,
  tokenName: 'smoke-restricted-key',
  channelName: 'smoke-openai-channel',
  model: 'smoke-model',
  walletQuota: 1_000_000,
  usageQuota: 15,
})

function validateFixtureUpstreamTarget(value) {
  let target
  try { target = new URL(value) } catch { throw new Error('INVALID_SMOKE_UPSTREAM_TARGET') }
  if (target.protocol !== 'http:' || target.hostname !== '127.0.0.1' || target.port !== '19090' ||
      target.username || target.password || target.pathname !== '/' || target.search || target.hash) {
    throw new Error('INVALID_SMOKE_UPSTREAM_TARGET')
  }
  return target.origin
}

function fixtureSuccess(response) {
  return response.status >= 200 && response.status < 300 && response.body?.success === true
}

function fixturePageItems(response) {
  return Array.isArray(response.body?.data?.items) ? response.body.data.items : []
}

function findFixtureItem(items, predicate) {
  const matches = items.filter(predicate)
  return matches.length === 1 ? matches[0] : null
}

/**
 * Exercise the smallest real billed relay path against an isolated fake
 * OpenAI upstream. Credentials and response bodies remain in process memory.
 */
export async function probeRelayFixture({
  baseUrl,
  edition,
  sha,
  username,
  password,
  isolated = false,
  fetchImpl = globalThis.fetch,
  upstreamBaseUrl,
  fullContentExpected = false,
} = {}) {
  if (!isolated) throw new Error('ISOLATED_SMOKE_OPT_IN_REQUIRED')
  const origin = validateSmokeTarget(baseUrl, edition, sha)
  const upstreamOrigin = validateFixtureUpstreamTarget(upstreamBaseUrl)
  const controlUrl = upstreamOrigin + '/__smoke__/control'
  if (typeof fetchImpl !== 'function' || !username || !password) throw new Error('SMOKE_FIXTURE_AUTH_UNAVAILABLE')

  const localFetch = (pathname, options = {}) => fetchImpl(origin + pathname, {
    ...options,
    redirect: 'error',
    signal: AbortSignal.timeout(10_000),
  })
  const json = async (pathname, options = {}) => {
    const response = await localFetch(pathname, options)
    const body = await response.json().catch(() => null)
    return { status: response.status, body, headers: response.headers }
  }
  const expectSuccess = async (code, pathname, options = {}) => {
    const response = await json(pathname, options)
    if (!fixtureSuccess(response)) throw new Error(code)
    return response
  }
  const readControl = async () => {
    const response = await fetchImpl(controlUrl, { redirect: 'error', signal: AbortSignal.timeout(10_000) })
    return { status: response.status, body: await response.json().catch(() => null) }
  }
  const beforeControl = await readControl()
  if (beforeControl.status !== 200 || beforeControl.body?.count !== 0 || beforeControl.body?.path_ok !== false ||
      beforeControl.body?.model_ok !== false || beforeControl.body?.max_tokens_ok !== false || beforeControl.body?.stream_ok !== false ||
      beforeControl.body?.bearer_ok !== false || beforeControl.body?.request_ok !== false) {
    throw new Error('SMOKE_FIXTURE_UPSTREAM_NOT_FRESH')
  }
  const rootLogin = await expectSuccess('SMOKE_FIXTURE_ROOT_LOGIN_FAILED', '/api/user/login', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password }),
  })
  const rootToken = rootLogin.body.data?.access_token
  if (typeof rootToken !== 'string' || rootToken.length === 0) throw new Error('SMOKE_FIXTURE_ROOT_LOGIN_FAILED')
  const rootHeaders = { Authorization: `Bearer ${rootToken}`, 'Content-Type': 'application/json' }

  const status = await expectSuccess('SMOKE_FIXTURE_BATCH_MODE_UNSAFE', '/api/status')
  if (status.body.data?.enable_batch_update !== false) throw new Error('SMOKE_FIXTURE_BATCH_MODE_UNSAFE')

  for (const [key, value] of [
    ['ModelRatio', JSON.stringify({ [fixture.model]: 1 })],
    ['CompletionRatio', JSON.stringify({ [fixture.model]: 1 })],
    ['GroupRatio', JSON.stringify({ default: 1 })],
    ['GroupGroupRatio', '{}'],
    ['LogConsumeEnabled', 'true'],
  ]) {
    await expectSuccess('SMOKE_FIXTURE_RATIO_CONFIG_FAILED', '/api/option/', {
      method: 'PUT', headers: rootHeaders, body: JSON.stringify({ key, value }),
    })
  }

  const fixtureUserPassword = randomBytes(fixture.userPasswordBytes).toString('hex')
  await expectSuccess('SMOKE_FIXTURE_USER_CREATE_FAILED', '/api/user/', {
    method: 'POST', headers: rootHeaders, body: JSON.stringify({
      username: fixture.userName,
      password: fixtureUserPassword,
      display_name: fixture.userDisplayName,
      role: 1,
    }),
  })

  const userSearch = await expectSuccess('SMOKE_FIXTURE_USER_LOOKUP_FAILED', `/api/user/search?keyword=${fixture.userName}&p=1&page_size=10`, { headers: rootHeaders })
  const user = findFixtureItem(fixturePageItems(userSearch), (item) => item?.username === fixture.userName && item?.role === 1)
  if (!Number.isInteger(user?.id) || user.id <= 0) throw new Error('SMOKE_FIXTURE_USER_LOOKUP_FAILED')
  await expectSuccess('SMOKE_FIXTURE_WALLET_OVERRIDE_FAILED', '/api/user/manage', {
    method: 'POST', headers: rootHeaders,
    body: JSON.stringify({ id: user.id, action: 'add_quota', mode: 'override', value: fixture.walletQuota }),
  })
  const userLogin = await expectSuccess('SMOKE_FIXTURE_USER_LOGIN_FAILED', '/api/user/login', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: fixture.userName, password: fixtureUserPassword }),
  })
  const userToken = userLogin.body.data?.access_token
  if (typeof userToken !== 'string' || userToken.length === 0) throw new Error('SMOKE_FIXTURE_USER_LOGIN_FAILED')
  const userHeaders = { Authorization: `Bearer ${userToken}`, 'Content-Type': 'application/json' }

  await expectSuccess('SMOKE_FIXTURE_KEY_CREATE_FAILED', '/api/token/', {
    method: 'POST', headers: userHeaders, body: JSON.stringify({
      name: fixture.tokenName,
      remain_quota: fixture.walletQuota,
      unlimited_quota: false,
      model_limits_enabled: true,
      model_limits: fixture.model,
      group: 'default',
      expired_time: -1,
    }),
  })
  const tokenList = await expectSuccess('SMOKE_FIXTURE_KEY_LOOKUP_FAILED', `/api/token/search?keyword=${fixture.tokenName}&p=1&page_size=10`, { headers: userHeaders })
  const apiKey = findFixtureItem(fixturePageItems(tokenList), (item) => item?.name === fixture.tokenName)
  if (!Number.isInteger(apiKey?.id) || apiKey.id <= 0) throw new Error('SMOKE_FIXTURE_KEY_LOOKUP_FAILED')
  const keyResult = await expectSuccess('SMOKE_FIXTURE_KEY_RETRIEVE_FAILED', `/api/token/${apiKey.id}/key`, {
    method: 'POST', headers: userHeaders,
  })
  const apiKeyValue = keyResult.body.data?.key
  if (typeof apiKeyValue !== 'string' || apiKeyValue.length === 0) throw new Error('SMOKE_FIXTURE_KEY_RETRIEVE_FAILED')

  await expectSuccess('SMOKE_FIXTURE_CHANNEL_CREATE_FAILED', '/api/channel/', {
    method: 'POST', headers: rootHeaders, body: JSON.stringify({
      mode: 'single',
      channel: {
        type: 1,
        key: 'synthetic-upstream-key',
        status: 1,
        name: fixture.channelName,
        models: fixture.model,
        group: 'default',
        base_url: upstreamOrigin,
      },
    }),
  })
  const channelList = await expectSuccess('SMOKE_FIXTURE_CHANNEL_LOOKUP_FAILED', `/api/channel/search?keyword=${fixture.channelName}&p=1&page_size=10`, { headers: rootHeaders })
  const channel = findFixtureItem(channelList.body.data?.items || [], (item) => item?.name === fixture.channelName && item?.type === 1)
  if (!Number.isInteger(channel?.id) || channel.id <= 0) throw new Error('SMOKE_FIXTURE_CHANNEL_LOOKUP_FAILED')

  const relay = await json('/v1/chat/completions', {
    method: 'POST', headers: { Authorization: `Bearer ${apiKeyValue}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ model: fixture.model, messages: [{ role: 'user', content: 'synthetic smoke request' }], api_key: 'synthetic-request-log-value', max_tokens: 8, stream: false }),
  })
  if (relay.status !== 200 || relay.body?.usage?.prompt_tokens !== 10 || relay.body?.usage?.completion_tokens !== 5 || relay.body?.usage?.total_tokens !== 15) {
    throw new Error('SMOKE_FIXTURE_RELAY_FAILED')
  }
  const requestId = relay.headers.get('x-oneapi-request-id')
  if (typeof requestId !== 'string' || requestId.length === 0) throw new Error('SMOKE_FIXTURE_REQUEST_ID_MISSING')

  const userState = await expectSuccess('SMOKE_FIXTURE_USAGE_MISMATCH', `/api/user/${user.id}`, { headers: rootHeaders })
  const tokenState = await expectSuccess('SMOKE_FIXTURE_USAGE_MISMATCH', `/api/token/${apiKey.id}`, { headers: userHeaders })
  const channelState = await expectSuccess('SMOKE_FIXTURE_USAGE_MISMATCH', `/api/channel/${channel.id}`, { headers: rootHeaders })
  const usedChannel = channelState.body.data
  if (userState.body.data?.quota !== fixture.walletQuota - fixture.usageQuota ||
      userState.body.data?.used_quota !== fixture.usageQuota || userState.body.data?.request_count !== 1 ||
      tokenState.body.data?.remain_quota !== fixture.walletQuota - fixture.usageQuota ||
      tokenState.body.data?.used_quota !== fixture.usageQuota || usedChannel?.used_quota !== fixture.usageQuota) {
    throw new Error('SMOKE_FIXTURE_USAGE_MISMATCH')
  }

  const logs = await expectSuccess('SMOKE_FIXTURE_CONSUME_LOG_MISMATCH', `/api/log/self?type=2&request_id=${encodeURIComponent(requestId)}&p=1&page_size=100`, { headers: userHeaders })
  const consumeLogs = fixturePageItems(logs)
  const consumeLog = consumeLogs.length === 1 ? consumeLogs[0] : null
  let other
  try { other = JSON.parse(consumeLog?.other || '{}') } catch { throw new Error('SMOKE_FIXTURE_CONSUME_LOG_MISMATCH') }
  if (logs.body.data?.total !== 1 || !consumeLog || consumeLog.user_id !== user.id || consumeLog.token_id !== apiKey.id ||
      consumeLog.channel !== channel.id || consumeLog.request_id !== requestId || consumeLog.quota !== fixture.usageQuota ||
      consumeLog.prompt_tokens !== 10 || consumeLog.completion_tokens !== 5 || Object.hasOwn(other, 'admin_info')) {
    throw new Error('SMOKE_FIXTURE_CONSUME_LOG_MISMATCH')
  }
  const rootLogs = await expectSuccess('SMOKE_FIXTURE_CONSUME_LOG_MISMATCH', `/api/log/?type=2&request_id=${encodeURIComponent(requestId)}&p=1&page_size=100`, { headers: rootHeaders })
  if (rootLogs.body.data?.total !== 1 || fixturePageItems(rootLogs).length !== 1 || fixturePageItems(rootLogs)[0]?.request_id !== requestId) {
    throw new Error('SMOKE_FIXTURE_CONSUME_LOG_MISMATCH')
  }

  const fullContent = await json(`/api/full-content-logs/${encodeURIComponent(requestId)}`, { headers: userHeaders })
  if (fullContent.status !== 403) throw new Error('SMOKE_FIXTURE_FULL_CONTENT_ACCESS_MISMATCH')
  let fullContentCheck = { name: 'full-content response-body handling', ok: true, covered: false, reason: 'FULL_CONTENT_LOG_NOT_ENABLED' }
  if (fullContentExpected) {
    const rootFullContent = await expectSuccess('SMOKE_FIXTURE_FULL_CONTENT_REDACTION_MISMATCH',
      `/api/full-content-logs/${encodeURIComponent(requestId)}`, { headers: rootHeaders })
    const detail = rootFullContent.body.data
    const headerValue = (headers, key) => {
      const entry = Object.entries(headers || {}).find(([name]) => name.toLowerCase() === key)
      return entry?.[1]
    }
    const requestAuthorization = headerValue(detail?.request_headers, 'authorization')
    const responseApiKey = headerValue(detail?.response_headers, 'x-api-key')
    if (!Array.isArray(requestAuthorization) || requestAuthorization.some((value) => value !== '[REDACTED]') ||
        !Array.isArray(responseApiKey) || responseApiKey.some((value) => value !== '[REDACTED]') ||
        !String(detail?.request_body || '').includes('[REDACTED]') || String(detail?.request_body || '').includes('synthetic-request-log-value') ||
        !String(detail?.response_body || '').includes('synthetic fixed response') || String(detail?.response_body || '').includes('[REDACTED]')) {
      throw new Error('SMOKE_FIXTURE_FULL_CONTENT_REDACTION_MISMATCH')
    }
    fullContentCheck = { name: 'full-content request/header redaction and fixed response-body contract', ok: true, covered: true }
  }

  const anonymous = await json('/v1/chat/completions', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ model: fixture.model, messages: [], stream: false }),
  })
  if (![401, 403].includes(anonymous.status)) throw new Error('SMOKE_FIXTURE_ANONYMOUS_RELAY_NOT_REJECTED')
  const control = await readControl()
  if (control.status !== 200 || control.body?.count !== 1 || control.body?.path_ok !== true ||
      control.body?.model_ok !== true || control.body?.max_tokens_ok !== true || control.body?.stream_ok !== true ||
      control.body?.bearer_ok !== true || control.body?.request_ok !== true) {
    throw new Error('SMOKE_FIXTURE_UPSTREAM_MISMATCH')
  }

  return { passed: true, checks: [
    { name: 'synthetic ordinary user wallet, key and channel usage', ok: true },
    { name: 'single associated consume log hides admin metadata', ok: true },
    { name: 'ordinary user full-content log access rejected', ok: true, status: fullContent.status },
    fullContentCheck,
    { name: 'anonymous relay rejected without upstream request', ok: true, status: anonymous.status },
    { name: 'synthetic upstream request contract', ok: true },
  ] }
}

export async function probeFreshSQLite({
  baseUrl, edition, sha, isolated = false, relayFixture = false, fullContentExpected = false,
  upstreamBaseUrl, fetchImpl = globalThis.fetch,
}) {
  if (!isolated) throw new Error('ISOLATED_SMOKE_OPT_IN_REQUIRED')
  const origin = validateSmokeTarget(baseUrl, edition, sha)
  // Fail before setup writes when the billed relay fixture has no literal
  // loopback upstream. The downstream probe validates it again defensively.
  if (relayFixture) validateFixtureUpstreamTarget(upstreamBaseUrl)
  const localFetch = (url, options = {}) => {
    if (new URL(url).origin !== origin) throw new Error('SMOKE_ORIGIN_MISMATCH')
    return fetchImpl(url, { ...options, redirect: 'error' })
  }
  const json = async (pathname, options = {}) => {
    const response = await localFetch(origin + pathname, { ...options, signal: AbortSignal.timeout(10_000) })
    const body = await response.json().catch(() => null)
    return { status: response.status, body }
  }
  const setup = await json('/api/setup')
  if (setup.status !== 200 || setup.body?.success !== true || setup.body?.data?.status !== false ||
      setup.body?.data?.root_init !== false || setup.body?.data?.database_type !== 'sqlite') {
    throw new Error('SMOKE_REQUIRES_FRESH_SQLITE')
  }

  const username = 'smokeadmin'
  const password = randomBytes(24).toString('hex')
  const created = await json('/api/setup', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password, confirmPassword: password,
      SelfUseModeEnabled: edition === 'lan', DemoSiteEnabled: false }),
  })
  if (created.status !== 200 || created.body?.success !== true) throw new Error('SMOKE_SETUP_FAILED')
  const anonymous = await localFetch(origin + '/api/user/self', { signal: AbortSignal.timeout(10_000) })
  await anonymous.arrayBuffer()
  if (![401, 403].includes(anonymous.status)) throw new Error('SMOKE_ANONYMOUS_ACCESS_NOT_REJECTED')
  const status = await json('/api/status')
  if (status.status !== 200 || status.body?.success !== true || status.body?.data?.setup !== true ||
      status.body?.data?.self_use_mode_enabled !== (edition === 'lan')) {
    throw new Error('SMOKE_RUNTIME_MODE_MISMATCH')
  }
  const probe = await runRuntimeProbe({ baseUrl: origin, username, password, fetchImpl: localFetch })
  if (!probe.passed) {
    return { command: 'docker:smoke', sha, edition, database: 'fresh-sqlite', passed: false, checks: [
      { name: 'fresh SQLite initialization', ok: true },
      { name: 'anonymous self rejected', ok: true, status: anonymous.status },
      { name: 'runtime self-use mode', ok: true },
      ...probe.checks,
    ] }
  }
  const relay = relayFixture
    ? await probeRelayFixture({ baseUrl: origin, edition, sha, username, password, isolated, fetchImpl, fullContentExpected, upstreamBaseUrl })
    : null
  return { command: 'docker:smoke', sha, edition, database: 'fresh-sqlite',
    passed: probe.passed && (relay?.passed ?? true), checks: [
      { name: 'fresh SQLite initialization', ok: true },
      { name: 'anonymous self rejected', ok: true, status: anonymous.status },
      { name: 'runtime self-use mode', ok: true },
      ...probe.checks,
      ...(relay?.checks || []),
    ] }
}

async function probeFrontend(baseUrl, sha, version, modulePath) {
  if (!modulePath) throw new Error('SMOKE_BROWSER_MODULE_REQUIRED')
  const { chromium } = await import(modulePath)
  const browser = await chromium.launch({ headless: true })
  try {
    const page = await browser.newPage({ locale: 'en-US' })
    page.setDefaultTimeout(20_000)
    const origin = new URL(baseUrl).origin
    let errors = 0
    page.on('pageerror', () => { errors += 1 })
    await page.route('**/*', (route) => new URL(route.request().url()).origin === origin ? route.continue() : route.abort())
    const response = await page.goto(origin + '/sign-in', { waitUntil: 'domcontentloaded', timeout: 30_000 })
    if (response?.status() !== 200) throw new Error('SMOKE_FRONTEND_HTTP_FAILED')
    // A loading/error boundary can contain text and metadata too. Require the
    // real, editable sign-in form shared by both editions before accepting it.
    const username = page.locator('form input[name="username"]')
    const password = page.locator('form input[name="password"][type="password"]')
    const submit = page.locator('form button[type="submit"]')
    await username.waitFor({ state: 'visible' })
    await password.waitFor({ state: 'visible' })
    await submit.waitFor({ state: 'visible' })
    if (!await username.isEditable() || !await password.isEditable() || !await submit.isEnabled()) {
      throw new Error('SMOKE_FRONTEND_FORM_UNAVAILABLE')
    }
    const metadata = await page.evaluate(() => ({
      global: window.__APP_BUILD__?.rev,
      html: document.documentElement.getAttribute('data-build-rev'),
      meta: document.querySelector('meta[name="build-id"]')?.getAttribute('content'),
    }))
    if (errors > 0) throw new Error('SMOKE_FRONTEND_RUNTIME_ERROR')
    if (!metadata.global?.startsWith(`rv.${version}.${sha}.`) || metadata.global !== metadata.html ||
        metadata.global !== metadata.meta) throw new Error('SMOKE_FRONTEND_BUILD_MISMATCH')
    return { name: 'real frontend sign-in form and revision', ok: true, revision: metadata.global }
  } finally {
    await browser.close()
  }
}

async function main() {
  try {
    if (process.env.GITHUB_ACTIONS !== 'true' || process.env.MYAPI_ISOLATED_SMOKE !== '1') {
      throw new Error('ISOLATED_CI_SMOKE_ONLY')
    }
    const baseUrl = process.env.MYAPI_SMOKE_BASE_URL
    const sha = process.env.GITHUB_SHA
    const edition = process.env.MYAPI_SMOKE_EDITION
    const upstreamBaseUrl = process.env.MYAPI_FAKE_UPSTREAM_URL
    const report = await probeFreshSQLite({ baseUrl, sha, edition, isolated: true, relayFixture: true,
      fullContentExpected: process.env.MYAPI_SMOKE_FULL_CONTENT === '1', upstreamBaseUrl })
    if (report.passed) report.checks.push(await probeFrontend(baseUrl, sha,
      readFileSync(new URL('../../VERSION', import.meta.url), 'utf8').trim(), process.env.MYAPI_PLAYWRIGHT_MODULE))
    console.log(JSON.stringify(report, null, 2))
    if (!report.passed) process.exitCode = 1
  } catch (error) {
    // Only our fixed error codes may be printed; external exception text may
    // contain URLs, request bodies or credentials.
    const safeCodes = new Set(['INVALID_ISOLATED_SMOKE_TARGET', 'ISOLATED_SMOKE_OPT_IN_REQUIRED',
      'SMOKE_ORIGIN_MISMATCH', 'SMOKE_REQUIRES_FRESH_SQLITE', 'SMOKE_SETUP_FAILED',
      'SMOKE_ANONYMOUS_ACCESS_NOT_REJECTED', 'SMOKE_RUNTIME_MODE_MISMATCH',
      'INVALID_SMOKE_UPSTREAM_TARGET', 'SMOKE_FIXTURE_AUTH_UNAVAILABLE',
      'SMOKE_FIXTURE_ROOT_LOGIN_FAILED', 'SMOKE_FIXTURE_BATCH_MODE_UNSAFE',
      'SMOKE_FIXTURE_RATIO_CONFIG_FAILED', 'SMOKE_FIXTURE_USER_CREATE_FAILED',
      'SMOKE_FIXTURE_USER_LOOKUP_FAILED', 'SMOKE_FIXTURE_WALLET_OVERRIDE_FAILED',
      'SMOKE_FIXTURE_USER_LOGIN_FAILED', 'SMOKE_FIXTURE_KEY_CREATE_FAILED',
      'SMOKE_FIXTURE_KEY_LOOKUP_FAILED', 'SMOKE_FIXTURE_KEY_RETRIEVE_FAILED',
      'SMOKE_FIXTURE_CHANNEL_CREATE_FAILED', 'SMOKE_FIXTURE_CHANNEL_LOOKUP_FAILED',
      'SMOKE_FIXTURE_RELAY_FAILED', 'SMOKE_FIXTURE_REQUEST_ID_MISSING',
      'SMOKE_FIXTURE_USAGE_MISMATCH', 'SMOKE_FIXTURE_CONSUME_LOG_MISMATCH',
      'SMOKE_FIXTURE_FULL_CONTENT_ACCESS_MISMATCH', 'SMOKE_FIXTURE_ANONYMOUS_RELAY_NOT_REJECTED',
      'SMOKE_FIXTURE_FULL_CONTENT_REDACTION_MISMATCH',
      'SMOKE_FIXTURE_UPSTREAM_NOT_FRESH',
      'SMOKE_FIXTURE_UPSTREAM_MISMATCH',
      'SMOKE_BROWSER_MODULE_REQUIRED', 'SMOKE_FRONTEND_HTTP_FAILED',
      'SMOKE_FRONTEND_BUILD_MISMATCH', 'SMOKE_FRONTEND_RUNTIME_ERROR',
      'SMOKE_FRONTEND_FORM_UNAVAILABLE', 'ISOLATED_CI_SMOKE_ONLY'])
    console.error(JSON.stringify({ command: 'docker:smoke', passed: false,
      code: safeCodes.has(error?.message) ? error.message : 'SMOKE_UNEXPECTED_FAILURE' }))
    process.exitCode = 1
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) await main()
