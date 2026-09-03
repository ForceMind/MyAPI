import assert from 'node:assert/strict'
import { test } from 'node:test'

import { runRuntimeProbe } from './auth-probe.mjs'

function mockFetch({ loginStatus = 200, loginSuccess = true, token = 'synthetic-token' } = {}) {
  const calls = []
  const fetchImpl = async (url, options = {}) => {
    calls.push({ url, options })
    if (url.endsWith('/api/user/login')) {
      const body = loginStatus === 200
        ? { success: loginSuccess, data: { access_token: token } }
        : { success: false, message: 'invalid credentials' }
      return new Response(JSON.stringify(body), { status: loginStatus, headers: { 'content-type': 'application/json' } })
    }
    return new Response(JSON.stringify({ success: true, data: {} }), { status: 200, headers: { 'content-type': 'application/json' } })
  }
  return { calls, fetchImpl }
}

function mockUnsuccessfulApiFetch() {
  return async (url) => {
    if (url.endsWith('/api/user/login')) {
      return new Response(JSON.stringify({ success: true, data: { access_token: 'synthetic-token' } }), { status: 200 })
    }
    return new Response(JSON.stringify({ success: false, message: 'synthetic upstream failure' }), { status: 200 })
  }
}

test('authenticated runtime probe checks status, identity, quota, and logs without exposing tokens', async () => {
  const { calls, fetchImpl } = mockFetch()
  const report = await runRuntimeProbe({
    baseUrl: 'http://127.0.0.1:3311/',
    username: 'probe-user',
    password: 'probe-password',
    fetchImpl,
  })

  assert.equal(report.passed, true)
  assert.ok(report.checks.every((item) => item.ok && !item.code))
  assert.deepEqual(report.checks.map((item) => item.name), [
    'server status',
    'password login',
    'user self',
    'channel quota changes',
    'admin logs',
  ])
  assert.equal(calls.length, 5)
  assert.equal(calls.at(-1).options.headers.Authorization, 'Bearer synthetic-token')
  assert.doesNotMatch(JSON.stringify(report), /synthetic-token|probe-password/)
})

test('probe fails closed when credentials are missing without making a request', async () => {
  let called = false
  const report = await runRuntimeProbe({
    baseUrl: 'http://127.0.0.1:3311',
    username: '',
    password: '',
    fetchImpl: async () => {
      called = true
      return new Response('{}', { status: 200 })
    },
  })

  assert.equal(report.passed, false)
  assert.equal(report.checks[0].code, 'MISSING_CREDENTIALS')
  assert.equal(called, false)
})

test('probe reports authentication failure without returning upstream response text', async () => {
  const { fetchImpl } = mockFetch({ loginStatus: 401 })
  const report = await runRuntimeProbe({
    baseUrl: 'https://myapi.example.test',
    username: 'probe-user',
    password: 'probe-password',
    fetchImpl,
  })

  assert.equal(report.passed, false)
  assert.equal(report.checks[1].code, 'AUTH_FAILED')
  assert.doesNotMatch(JSON.stringify(report), /invalid credentials|probe-password/)
})

test('probe rejects HTTP 200 responses that report success=false', async () => {
  const report = await runRuntimeProbe({
    baseUrl: 'https://myapi.example.test',
    username: 'probe-user',
    password: 'probe-password',
    fetchImpl: mockUnsuccessfulApiFetch(),
  })

  assert.equal(report.passed, false)
  assert.equal(report.checks[0].code, 'STATUS_UNCONFIRMED')
  assert.equal(report.checks[2].code, 'API_UNCONFIRMED')
  assert.doesNotMatch(JSON.stringify(report), /synthetic upstream failure|synthetic-token|probe-password/)
})

test('probe rejects a logically failed login even when its body contains a token', async () => {
  const { calls, fetchImpl } = mockFetch({ loginSuccess: false })
  const report = await runRuntimeProbe({
    baseUrl: 'http://127.0.0.1:3311', username: 'probe-user', password: 'probe-password', fetchImpl,
  })

  assert.equal(report.passed, false)
  assert.equal(report.checks[1].code, 'LOGIN_UNCONFIRMED')
  assert.equal(calls.length, 2, 'do not use a token from a failed login')
  assert.equal(report.checks.length, 2)
  assert.doesNotMatch(JSON.stringify(report), /synthetic-token|probe-password/)
})
