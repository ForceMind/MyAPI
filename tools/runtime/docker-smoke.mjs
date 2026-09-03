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

export async function probeFreshSQLite({ baseUrl, edition, sha, isolated = false, fetchImpl = globalThis.fetch }) {
  if (!isolated) throw new Error('ISOLATED_SMOKE_OPT_IN_REQUIRED')
  const origin = validateSmokeTarget(baseUrl, edition, sha)
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
  return { command: 'docker:smoke', sha, edition, database: 'fresh-sqlite',
    passed: probe.passed, checks: [
      { name: 'fresh SQLite initialization', ok: true },
      { name: 'anonymous self rejected', ok: true, status: anonymous.status },
      { name: 'runtime self-use mode', ok: true },
      ...probe.checks,
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
    if (!metadata.global?.startsWith(`rv.${version}.${sha}.`) || metadata.global !== metadata.html ||
        metadata.global !== metadata.meta || errors > 0) throw new Error('SMOKE_FRONTEND_BUILD_MISMATCH')
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
    const report = await probeFreshSQLite({ baseUrl, sha, edition, isolated: true })
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
      'SMOKE_BROWSER_MODULE_REQUIRED', 'SMOKE_FRONTEND_HTTP_FAILED',
      'SMOKE_FRONTEND_BUILD_MISMATCH', 'SMOKE_FRONTEND_FORM_UNAVAILABLE', 'ISOLATED_CI_SMOKE_ONLY'])
    console.error(JSON.stringify({ command: 'docker:smoke', passed: false,
      code: safeCodes.has(error?.message) ? error.message : 'SMOKE_UNEXPECTED_FAILURE' }))
    process.exitCode = 1
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) await main()
