import assert from 'node:assert/strict'
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { createServer } from 'node:http'
import { spawn } from 'node:child_process'
import { Readable, Writable } from 'node:stream'
import test from 'node:test'
import { createClient, loadConfiguration } from './client.mjs'
import { operations } from './catalog.mjs'
import { createProtocol, serve } from './mcp.mjs'

const secret = 'myapi_mgmt_synthetic_not_a_real_credential_12345'
const response = value => new Response(JSON.stringify(value), { headers: { 'Content-Type': 'application/json' } })
const initialize = { jsonrpc: '2.0', id: 1, method: 'initialize', params: { protocolVersion: '2025-11-25', capabilities: {}, clientInfo: { name: 'synthetic', version: '1' } } }

async function ready(client) {
  const handler = createProtocol(client)
  assert.equal((await handler(initialize)).result.protocolVersion, '2025-11-25')
  assert.equal(await handler({ jsonrpc: '2.0', method: 'notifications/initialized' }), null)
  return handler
}

test('credentials are configuration-only; HTTPS origin, ambiguity and protected file checks fail closed', t => {
  assert.deepEqual(loadConfiguration({ MYAPI_MANAGEMENT_URL: 'https://myapi.example', MYAPI_MANAGEMENT_KEY: secret }), { origin: 'https://myapi.example', key: secret })
  for (const origin of ['http://myapi.example', 'https://user:pass@myapi.example', 'https://myapi.example/path', 'https://myapi.example?key=secret', 'https://myapi.example/#fragment', 'file:///tmp']) {
    assert.throws(() => loadConfiguration({ MYAPI_MANAGEMENT_URL: origin, MYAPI_MANAGEMENT_KEY: secret }))
  }
  assert.throws(() => loadConfiguration({ MYAPI_MANAGEMENT_URL: 'https://myapi.example', MYAPI_MANAGEMENT_KEY: 'old_panel_pat' }))
  assert.throws(() => loadConfiguration({ MYAPI_MANAGEMENT_URL: 'https://myapi.example', MYAPI_MANAGEMENT_KEY: secret, MYAPI_MANAGEMENT_KEY_FILE: '/unused' }))
  const directory = mkdtempSync(path.join(tmpdir(), 'myapi-management-'))
  t.after(() => rmSync(directory, { recursive: true, force: true }))
  const credentialPath = path.join(directory, 'key')
  writeFileSync(credentialPath, `${secret}\n`, { mode: 0o600 })
  assert.equal(loadConfiguration({ MYAPI_MANAGEMENT_URL: 'https://myapi.example', MYAPI_MANAGEMENT_KEY_FILE: credentialPath }).key, secret)
  const unsafe = path.join(directory, 'unsafe')
  writeFileSync(unsafe, secret, { mode: 0o644 })
  if (process.platform !== 'win32') assert.throws(() => loadConfiguration({ MYAPI_MANAGEMENT_URL: 'https://myapi.example', MYAPI_MANAGEMENT_KEY_FILE: unsafe }))
})

test('safe fixed routes, exact arguments, missing/null/0 and credential redaction', async () => {
  const calls = []
  const client = createClient({ origin: 'https://myapi.example', key: secret }, async (url, options) => {
    calls.push({ url, options })
    return response({ success: true, data: { cache_zero: 0, cache_null: null, token: { key: 'provider-key', name: 'saved', access_token: 'provider-secret' }, note: `Echo ${secret}` } })
  })
  const result = await client.call('myapi_channels_get', { id: 3 })
  assert.equal(calls[0].url, 'https://myapi.example/api/channel/3')
  assert.equal(calls[0].options.headers.Authorization, `Bearer ${secret}`)
  assert.equal(calls[0].options.redirect, 'manual')
  assert.deepEqual(result.data, { cache_zero: 0, cache_null: null, token: { name: 'saved' }, note: 'Echo [REDACTED]' })
  assert.equal(Object.hasOwn(result.data, 'cache_missing'), false)
  for (const args of [{ id: '../key' }, { id: 0 }, { id: -1 }, { id: 1.2 }, { id: 2147483648 }, { id: 1, url: '/api/channel/test/1' }]) await assert.rejects(client.call('myapi_channels_get', args))
  await assert.rejects(client.call('myapi_request_anything', {}))
  await assert.rejects(client.call('myapi_logs_list', { result: 'success' }))
  await assert.rejects(client.call('myapi_logs_stats', { request_id: 'one-request' }))
  assert.equal(calls.length, 1, 'rejected calls must not reach HTTP')
})

test('mutations require exact target confirmation; unknown outcomes never replay or refresh', async () => {
  const calls = []
  const client = createClient({ origin: 'https://myapi.example', key: secret }, async (url, options) => {
    calls.push({ url, options })
    throw new Error(`transport must not reveal ${secret}`)
  })
  for (const args of [
    { id: 3, status: 2 },
    { id: 3, status: 2, confirmation: 'channel.status:4:2' },
    { id: 3, status: 2, confirmation: 'channel.status:3:1' },
    { id: 3, status: 3, confirmation: 'channel.status:3:3' },
  ]) await assert.rejects(client.call('myapi_channels_set_status', args))
  assert.equal(calls.length, 0)
  const result = await client.call('myapi_channels_set_status', { id: 3, status: 2, confirmation: 'channel.status:3:2' })
  assert.deepEqual(result, { success: false, code: 'REQUEST_UNCONFIRMED', outcome: 'unknown', replay: false })
  assert.equal(calls.length, 1)
  assert.equal(calls[0].url, 'https://myapi.example/api/channel/3/status')
  assert.equal(calls[0].options.method, 'POST')
  assert.equal(calls[0].options.headers['X-MyAPI-Confirm'], 'channel.status:3:2')
  assert.deepEqual(JSON.parse(calls[0].options.body), { status: 2 })
  assert.doesNotMatch(JSON.stringify(result), new RegExp(secret))
})

test('large upstream usage evidence remains an exact JSON number through MCP serialization', async () => {
  const client = createClient({ origin: 'https://myapi.example', key: secret }, async () => new Response('{"success":true,"data":{"cache_read_tokens":9007199254740993,"unknown":null,"zero":0}}'))
  const result = await client.call('myapi_logs_list')
  assert.equal(result.success, true)
  assert.equal(JSON.stringify(result), '{"success":true,"data":{"cache_read_tokens":9007199254740993,"unknown":null,"zero":0}}')
})

test('redirect, malformed/oversized responses and HTTP failures do not leak bodies or follow alternate targets', async () => {
  for (const fixture of [
    () => new Response(secret, { status: 302, headers: { Location: 'https://attacker.example' } }),
    () => new Response(secret, { status: 500 }),
    () => new Response(secret, { status: 403 }),
    () => new Response(secret),
    () => response({ success: false, message: secret, data: { key: secret } }),
    () => response({ success: true, data: 'x'.repeat(1024 * 1024) }),
  ]) {
    let calls = 0
    const client = createClient({ origin: 'https://myapi.example', key: secret }, async () => { calls++; return fixture() })
    const result = await client.call('myapi_channels_set_status', { id: 3, status: 2, confirmation: 'channel.status:3:2' })
    assert.equal(result.success, false)
    assert.equal(calls, 1)
    assert.doesNotMatch(JSON.stringify(result), new RegExp(secret))
  }
})

test('MCP lifecycle, current capability filtering and call errors use JSON-RPC without extra stdout', async () => {
  let available = ['channels.get']
  const handler = createProtocol({ capabilities: async () => ({ success: true, data: { operations: available } }), call: async () => ({ success: false, code: 'HTTP_403' }) })
  assert.equal((await handler({ jsonrpc: '2.0', id: 0, method: 'tools/list' })).error.code, -32002)
  assert.equal((await handler(initialize)).result.capabilities.tools.listChanged, undefined)
  await handler({ jsonrpc: '2.0', method: 'notifications/initialized' })
  assert.deepEqual((await handler({ jsonrpc: '2.0', id: 2, method: 'tools/list' })).result.tools.map(item => item.name), ['myapi_channels_get'])
  available = []
  assert.deepEqual((await handler({ jsonrpc: '2.0', id: 3, method: 'tools/list' })).result.tools, [])
  assert.equal((await handler({ jsonrpc: '2.0', id: 4, method: 'tools/call', params: { name: 'shell' } })).error.code, -32602)
  assert.equal((await handler({ jsonrpc: '2.0', id: 5, method: 'tools/call', params: { name: 'myapi_channels_get', arguments: { id: 3 } } })).result.isError, true)
  assert.equal((await handler({ jsonrpc: '2.0', method: 'unknown-notification' })), null)
  assert.equal((await handler([])).error.code, -32600)
})

test('line transport handles malformed JSON, split lines and bounded input', async () => {
  const handler = await ready({ capabilities: async () => ({ success: true, data: { operations: [] } }) })
  let text = ''
  const output = new Writable({ write(chunk, encoding, callback) { text += chunk; callback() } })
  await serve(Readable.from(['{bad}\n{"jsonrpc":"2.0",', '"id":9,"method":"ping"}\n']), output, handler)
  const lines = text.trim().split('\n').map(line => JSON.parse(line))
  assert.equal(lines.length, 2)
  assert.equal(lines[0].error.code, -32700)
  assert.deepEqual(lines[1], { jsonrpc: '2.0', id: 9, result: {} })
  await assert.rejects(serve(Readable.from(['x'.repeat(65537)]), output, handler), /MESSAGE_TOO_LARGE/)
})

test('real stdio process uses only the saved-data API and exits without exposing the key', async t => {
  const requests = []
  const server = createServer((request, reply) => {
    requests.push({ path: request.url, authorization: request.headers.authorization })
    reply.setHeader('Content-Type', 'application/json')
    if (request.url === '/api/management/capabilities') reply.end(JSON.stringify({ success: true, data: { operations: ['channels.get'] } }))
    else if (request.url === '/api/channel/3') reply.end(JSON.stringify({ success: true, data: { id: 3, name: 'Synthetic saved channel', cached_tokens: 0 } }))
    else { reply.statusCode = 500; reply.end(JSON.stringify({ success: false })) }
  })
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
  t.after(() => server.close())
  const child = spawn(process.execPath, [new URL('./mcp.mjs', import.meta.url).pathname], {
    env: { ...process.env, MYAPI_MANAGEMENT_URL: `http://127.0.0.1:${server.address().port}`, MYAPI_MANAGEMENT_KEY: secret, MYAPI_MANAGEMENT_KEY_FILE: '' },
    stdio: ['pipe', 'pipe', 'pipe'],
  })
  t.after(() => child.kill())
  let stdout = ''; let stderr = ''
  child.stdout.on('data', chunk => { stdout += chunk })
  child.stderr.on('data', chunk => { stderr += chunk })
  child.stdin.end([
    initialize,
    { jsonrpc: '2.0', method: 'notifications/initialized' },
    { jsonrpc: '2.0', id: 2, method: 'tools/list' },
    { jsonrpc: '2.0', id: 3, method: 'tools/call', params: { name: 'myapi_channels_get', arguments: { id: 3 } } },
  ].map(value => JSON.stringify(value)).join('\n') + '\n')
  const exit = await new Promise((resolve, reject) => { child.once('error', reject); child.once('exit', resolve) })
  assert.equal(exit, 0)
  assert.equal(stderr, '')
  const replies = stdout.trim().split('\n').map(line => JSON.parse(line))
  assert.deepEqual(replies.map(item => item.id), [1, 2, 3])
  assert.equal(replies[1].result.tools.length, 1)
  assert.deepEqual(JSON.parse(replies[2].result.content[0].text).data, { id: 3, name: 'Synthetic saved channel', cached_tokens: 0 })
  assert.deepEqual(requests.map(item => item.path), ['/api/management/capabilities', '/api/channel/3'])
  assert.ok(requests.every(item => item.authorization === `Bearer ${secret}`))
  assert.doesNotMatch(stdout + stderr, new RegExp(secret))
  assert.ok(operations.filter(item => !item.operationsService).every(item => !/test|fetch_models|update_balance|\/v1\//.test(item.path)))
  assert.deepEqual(new Set(operations.filter(item => item.operationsService).map(item => item.path)), new Set(['/v1/instances', '/v1/plans', '/v1/plans/:id', '/v1/plans/:id/execute']))
})

test('stdio size bound applies to each frame independently of chunk grouping', async () => {
  const handler = createProtocol({})
  const messages = [
    { jsonrpc: '2.0', id: 1, method: 'ping' },
    { jsonrpc: '2.0', id: 2, method: 'ping', params: { note: 'x'.repeat(30000) } },
  ]
  let text = ''
  const output = new Writable({ write(chunk, encoding, callback) { text += chunk; callback() } })
  await serve(Readable.from([' '.repeat(40000), messages.map(item => JSON.stringify(item)).join('\n') + '\n']), output, handler)
  assert.deepEqual(text.trim().split('\n').map(line => JSON.parse(line).id), [1, 2])
  await assert.rejects(serve(Readable.from([' '.repeat(65537) + '\n']), output, handler), /MESSAGE_TOO_LARGE/)
})

test('independent operations remain discoverable and readable while MyAPI is unavailable, with no approval tool', async () => {
  const calls = []
  const client = createClient({ origin: 'https://myapi.example', operationsOrigin: 'https://ops.example', key: secret }, async (url, options) => {
    calls.push({ url, options })
    if (url.startsWith('https://myapi.example/')) throw new Error('application stopped')
    if (url.endsWith('/capabilities')) return response({ success: true, data: { operations: ['operations.plan', 'operations.execute'] } })
    return response({ success: true, data: { id: 'a'.repeat(64), state: 'interrupted', backup_created: true, target_start_attempted: true } })
  })
  const handler = await ready(client)
  const listed = await handler({ jsonrpc: '2.0', id: 2, method: 'tools/list' })
  assert.deepEqual(listed.result.tools.map(item => item.name), ['myapi_operations_plan', 'myapi_operations_execute'])
  assert.ok(!operations.some(item => item.operation.includes('approve')))
  const result = await client.call('myapi_operations_plan', { id: 'a'.repeat(64) })
  assert.equal(result.data.state, 'interrupted')
  assert.equal(calls.at(-1).url, 'https://ops.example/v1/plans/' + 'a'.repeat(64))
  assert.equal(calls.at(-1).options.headers.Authorization, 'Bearer ' + secret)
})

test('upgrade tools require exact frozen identity and send at most one independent execute request', async () => {
  const calls = []
  const client = createClient({ origin: 'https://myapi.example', operationsOrigin: 'https://ops.example', key: secret }, async (url, options) => {
    calls.push({ url, options })
    throw new Error('connection lost after acceptance')
  })
  const id = 'a'.repeat(64), digest = 'b'.repeat(64)
  await assert.rejects(client.call('myapi_operations_execute', { id, digest, confirmation: `operations.execute:${id}:${'c'.repeat(64)}` }))
  await assert.rejects(client.call('myapi_operations_execute', { id: '../approve', digest, confirmation: 'approve' }))
  await assert.rejects(client.call('myapi_operations_prepare', { instance_id: 'primary', artifact: { run_id: '1', url: 'https://attacker.example' } }))
  assert.equal(calls.length, 0)
  const result = await client.call('myapi_operations_execute', { id, digest, confirmation: `operations.execute:${id}:${digest}` })
  assert.equal(calls.length, 1)
  assert.equal(calls[0].url, `https://ops.example/v1/plans/${id}/execute`)
  assert.deepEqual(JSON.parse(calls[0].options.body), { digest })
  assert.deepEqual(result, { success: false, code: 'REQUEST_UNCONFIRMED', outcome: 'unknown', replay: false })
  const noOperations = createClient({ origin: 'https://myapi.example', key: secret }, async () => { throw new Error('must not call') })
  assert.deepEqual(await noOperations.call('myapi_operations_plan', { id }), { success: false, code: 'OPERATIONS_NOT_CONFIGURED' })
})
