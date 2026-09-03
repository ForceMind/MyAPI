#!/usr/bin/env node

/*
 * Read-only authenticated runtime probe for MyAPI deployments.
 * Copyright (C) 2026 ForceMind
 *
 * Credentials are accepted only through process memory. The probe never
 * prints, stores, or returns access tokens, cookies, request bodies, or
 * upstream response bodies.
 */
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const DEFAULT_TIMEOUT_MS = 10_000
const MAX_TIMEOUT_MS = 60_000

function trimBaseUrl(value) {
  const parsed = new URL(String(value || '').trim())
  if (!/^https?:$/.test(parsed.protocol)) throw new Error('base URL must use http or https')
  parsed.hash = ''
  parsed.search = ''
  return parsed.toString().replace(/\/$/, '')
}

function boundedTimeout(value) {
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed >= 1_000 && parsed <= MAX_TIMEOUT_MS
    ? Math.floor(parsed)
    : DEFAULT_TIMEOUT_MS
}

function check(name, ok, status, code = '') {
  const value = { name, ok: Boolean(ok), status: status ?? null }
  if (code) value.code = code
  return value
}

async function request(fetchImpl, url, options, timeoutMs, parseJson = false) {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), boundedTimeout(timeoutMs))
  try {
    const response = await fetchImpl(url, { ...options, signal: controller.signal })
    if (parseJson) {
      const body = await response.json().catch(() => null)
      return { status: response.status, body }
    }
    await response.arrayBuffer()
    return { status: response.status }
  } catch (error) {
    return { status: null, error: error?.name === 'AbortError' ? 'TIMEOUT' : 'NETWORK_ERROR' }
  } finally {
    clearTimeout(timer)
  }
}

function successfulResponse(response) {
  return response.status >= 200 && response.status < 300 && response.body?.success === true
}

/**
 * Run read-only authenticated deployment checks. The returned report contains
 * only status codes and stable error classifications, never credentials.
 */
export async function runRuntimeProbe({
  baseUrl,
  username,
  password,
  timeoutMs = DEFAULT_TIMEOUT_MS,
  fetchImpl = globalThis.fetch,
} = {}) {
  const checks = []
  let normalizedBaseUrl
  try {
    normalizedBaseUrl = trimBaseUrl(baseUrl)
  } catch {
    return { command: 'runtime:probe', passed: false, checks: [check('base URL', false, null, 'INVALID_BASE_URL')] }
  }
  if (!String(username || '') || !String(password || '')) {
    return { command: 'runtime:probe', base_url: normalizedBaseUrl, passed: false, checks: [check('credentials supplied', false, null, 'MISSING_CREDENTIALS')] }
  }
  if (typeof fetchImpl !== 'function') {
    return { command: 'runtime:probe', base_url: normalizedBaseUrl, passed: false, checks: [check('fetch available', false, null, 'FETCH_UNAVAILABLE')] }
  }

  const status = await request(fetchImpl, `${normalizedBaseUrl}/api/status`, { headers: { Accept: 'application/json' } }, timeoutMs, true)
  const statusConfirmed = successfulResponse(status)
  checks.push(check('server status', statusConfirmed, status.status, status.error || (!statusConfirmed && status.status >= 200 && status.status < 300 ? 'STATUS_UNCONFIRMED' : '')))

  const login = await request(fetchImpl, `${normalizedBaseUrl}/api/user/login`, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  }, timeoutMs, true)
  const token = login.body?.data?.access_token
  const loggedIn = successfulResponse(login) && typeof token === 'string' && token.length > 0
  let loginCode = ''
  if (!loggedIn) {
    loginCode = login.error || 'LOGIN_TOKEN_UNAVAILABLE'
    if (login.status === 401) loginCode = 'AUTH_FAILED'
    else if (login.status >= 200 && login.status < 300 && login.body?.success !== true) loginCode = 'LOGIN_UNCONFIRMED'
  }
  checks.push(check('password login', loggedIn, login.status, loginCode))
  if (!loggedIn) return { command: 'runtime:probe', base_url: normalizedBaseUrl, passed: false, checks }

  const headers = { Accept: 'application/json', Authorization: `Bearer ${token}` }
  for (const [name, pathname] of [
    ['user self', '/api/user/self'],
    ['channel quota changes', '/api/channel/quota/changes?range=24h'],
    ['admin logs', '/api/log/?p=1&size=10'],
  ]) {
    const response = await request(fetchImpl, `${normalizedBaseUrl}${pathname}`, { headers }, timeoutMs, true)
    const confirmed = successfulResponse(response)
    checks.push(check(name, confirmed, response.status, response.error || (!confirmed && response.status >= 200 && response.status < 300 ? 'API_UNCONFIRMED' : '')))
  }
  return { command: 'runtime:probe', base_url: normalizedBaseUrl, passed: checks.every((item) => item.ok), checks }
}

function parseArgs(argv) {
  const args = {
    baseUrl: process.env.MYAPI_PROBE_URL,
    usernameEnv: 'MYAPI_PROBE_USERNAME',
    passwordEnv: 'MYAPI_PROBE_PASSWORD',
    timeoutMs: process.env.MYAPI_PROBE_TIMEOUT_MS,
  }
  for (let index = 0; index < argv.length; index += 1) {
    const value = argv[index]
    if (value === '--base-url') args.baseUrl = argv[++index]
    else if (value === '--username-env') args.usernameEnv = argv[++index]
    else if (value === '--password-env') args.passwordEnv = argv[++index]
    else if (value === '--timeout-ms') args.timeoutMs = argv[++index]
    else if (value === '--help') args.help = true
    else throw new Error(`unknown argument: ${value}`)
  }
  return args
}

async function main() {
  let args
  try {
    args = parseArgs(process.argv.slice(2))
  } catch (error) {
    console.error(`runtime:probe: ${error.message}`)
    process.exitCode = 2
    return
  }
  if (args.help) {
    console.log('Usage: MYAPI_PROBE_URL=... MYAPI_PROBE_USERNAME=... MYAPI_PROBE_PASSWORD=... node tools/runtime/auth-probe.mjs')
    return
  }
  if (!args.baseUrl) {
    console.error('runtime:probe: MYAPI_PROBE_URL or --base-url is required')
    process.exitCode = 2
    return
  }
  const report = await runRuntimeProbe({
    baseUrl: args.baseUrl,
    username: process.env[args.usernameEnv] || '',
    password: process.env[args.passwordEnv] || '',
    timeoutMs: args.timeoutMs,
  })
  console.log(JSON.stringify(report, null, 2))
  if (!report.passed) process.exitCode = 1
}

const invokedPath = process.argv[1] ? path.resolve(process.argv[1]) : ''
if (invokedPath === path.resolve(fileURLToPath(import.meta.url))) void main()
