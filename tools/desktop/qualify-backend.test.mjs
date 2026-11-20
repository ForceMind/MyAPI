import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { lstat, mkdtemp, readFile, readdir, realpath, rm, symlink, writeFile } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import test from 'node:test'
import { binaryDigest, childEnvironment, createOwnedFixture, createSyntheticUpstream, qualificationScope,
  readStoppedAccounting, removeOwnedFixture, verifyDashboardRows, verifySyntheticAccounting } from './qualify-backend.mjs'
import { observeNativeShutdown } from '../../electron/test/fixtures/native-host.mjs'

const env = {
  MYAPI_DESKTOP_BACKEND_QUALIFICATION: '1', GITHUB_ACTIONS: 'true', GITHUB_EVENT_NAME: 'workflow_dispatch',
  GITHUB_REF: 'refs/heads/dev', GITHUB_REPOSITORY: 'ForceMind/MyAPI', GITHUB_SHA: '1'.repeat(40),
  GITHUB_RUN_ID: '123', GITHUB_RUN_ATTEMPT: '1', MYAPI_DESKTOP_BINARY_SHA256: '2'.repeat(64), RUNNER_TEMP: os.tmpdir(),
}

test('qualification accepts only the two native targets and exact manual dev identity without caller arguments', () => {
  for (const [platform, arch] of [['darwin', 'arm64'], ['win32', 'x64']]) {
    const result = qualificationScope(env, [], platform, arch)
    assert.equal(result.source_sha, env.GITHUB_SHA)
    assert.equal(result.binary_sha256, env.MYAPI_DESKTOP_BINARY_SHA256)
    assert.equal(result.platform, platform)
    assert.equal(result.architecture, arch)
  }
  const rejected = [
    { GITHUB_ACTIONS: 'false' }, { GITHUB_EVENT_NAME: 'push' }, { GITHUB_REF: 'refs/heads/main' },
    { GITHUB_REPOSITORY: 'unrelated/project' }, { GITHUB_SHA: 'short' }, { GITHUB_RUN_ID: '0' },
    { GITHUB_RUN_ATTEMPT: '-1' }, { MYAPI_DESKTOP_BACKEND_QUALIFICATION: '0' },
    { MYAPI_DESKTOP_BINARY_SHA256: '' }, { RUNNER_TEMP: 'relative' },
  ]
  for (const change of rejected) assert.throws(() => qualificationScope({ ...env, ...change }, [], 'darwin', 'arm64'), /^Error: DESKTOP_(?:SCOPE|IDENTITY)_REJECTED$/)
  assert.throws(() => qualificationScope(env, ['--binary', '/somewhere'], 'darwin', 'arm64'), /DESKTOP_SCOPE_REJECTED/)
  for (const [platform, arch] of [['linux', 'x64'], ['darwin', 'x64'], ['win32', 'arm64']]) {
    assert.throws(() => qualificationScope(env, [], platform, arch), /DESKTOP_PLATFORM_REJECTED/)
  }
})

test('the child environment does not inherit provider credentials, existing data locations or development mode', () => {
  const selected = childEnvironment({ PATH: '/tool-path', SystemRoot: 'C:\\Windows', HOME: '/personal',
    SESSION_SECRET: 'private', SQL_DSN: 'private-database', SQLITE_PATH: '/existing.db',
    MYAPI_BIND_ADDRESS: '0.0.0.0', MYAPI_EDITION: 'full', NODE_ENV: 'development', HTTP_PROXY: 'private-proxy',
    OPENAI_API_KEY: 'private-key', AWS_SHARED_CREDENTIALS_FILE: '/private-credentials', NODE_OPTIONS: '--inspect' })
  assert.deepEqual(selected, { PATH: '/tool-path', SystemRoot: 'C:\\Windows' })
})

test('a bound binary is read only and requires its exact hash and a regular file', async t => {
  const root = await mkdtemp(path.join(os.tmpdir(), 'desktop-binary-contract-'))
  t.after(() => rm(root, { recursive: true, force: true }))
  const filename = path.join(root, 'synthetic executable 非ASCII')
  const bytes = Buffer.from('synthetic data; this contract never executes it')
  const expected = createHash('sha256').update(bytes).digest('hex')
  await writeFile(filename, bytes)
  assert.equal(await binaryDigest(filename, expected), expected)
  assert.deepEqual(await readFile(filename), bytes)
  await assert.rejects(binaryDigest(filename, '0'.repeat(64)), /DESKTOP_BINARY_CHANGED/)
  await assert.rejects(binaryDigest(root, expected), /DESKTOP_BINARY_TYPE_REJECTED/)
  await assert.rejects(binaryDigest(path.join(root, 'missing'), expected), { code: 'ENOENT' })
  // Windows junctions are directory links and do not require developer-mode
  // permission to create a symbolic file link; neither is a regular binary.
  const alias = path.join(root, 'alias')
  await symlink(process.platform === 'win32' ? root : filename, alias, process.platform === 'win32' ? 'junction' : 'file')
  await assert.rejects(binaryDigest(alias, expected), /DESKTOP_BINARY_TYPE_REJECTED/)
})

test('fixture cleanup requires its exact private ownership record and never removes a neighbouring directory', async t => {
  const parent = await mkdtemp(path.join(os.tmpdir(), 'desktop-owned-contract-'))
  t.after(() => rm(parent, { recursive: true, force: true }))
  const neighbour = path.join(parent, 'keep-me')
  await writeFile(neighbour, 'neighbour')
  const scope = qualificationScope({ ...env, RUNNER_TEMP: parent }, [], 'darwin', 'arm64')
  const fixture = await createOwnedFixture(parent, scope)
  const marker = path.join(fixture.root, 'ownership.json')
  const original = await readFile(marker)
  await writeFile(marker, JSON.stringify({ ...fixture.ownership, run_id: '456' }))
  await assert.rejects(removeOwnedFixture(fixture), /DESKTOP_CLEANUP_OWNERSHIP_UNKNOWN/)
  assert.deepEqual(await readFile(neighbour, 'utf8'), 'neighbour')
  await writeFile(marker, original)
  await removeOwnedFixture(fixture)
  assert.deepEqual(await readdir(parent), ['keep-me'])
})

test('shutdown observation forwards only the owned stdin frame and records actual signals independently of close', () => {
  const frames = [], signals = []
  const record = { requested_signals: [], stdin_shutdown_count: 0, closed: false, exit_code: null }
  const child = { kill(signal) { signals.push(signal); return true }, stdin: { end(frame) { frames.push(frame); return this } } }
  observeNativeShutdown(child, record)
  assert.throws(() => child.stdin.end('private invalid command'), /^Error: NATIVE_HOST_STDIN_FRAME_REJECTED$/)
  assert.deepEqual(frames, [])
  assert.equal(record.stdin_shutdown_count, 0)
  child.stdin.end('shutdown\n')
  assert.deepEqual(frames, ['shutdown\n'])
  assert.equal(record.stdin_shutdown_count, 1)
  assert.deepEqual(record.requested_signals, [])
  assert.equal(record.closed, false)
  assert.equal(record.exit_code, null)
  child.kill('SIGTERM')
  assert.deepEqual(signals, ['SIGTERM'])
  assert.deepEqual(record.requested_signals, ['SIGTERM'])
  // A duplicate remains observable; it cannot masquerade as exactly one frame.
  child.stdin.end(Buffer.from('shutdown\n'))
  assert.equal(record.stdin_shutdown_count, 2)
})

const accountingIdentity = { userId: 7, tokenId: 11, channelId: 13, requestId: 'synthetic-local-request' }
const accountingFixture = () => ({
  user: { id: 7, quota: 0, self_use_no_balance: true, used_quota: 15, request_count: 1 },
  token: { id: 11, user_id: 7, remain_quota: 985, used_quota: 15, unlimited_quota: false },
  logs: { total: 1, items: [{ user_id: 7, token_id: 11, channel: 13, request_id: 'synthetic-local-request',
    type: 2, quota: 15, prompt_tokens: 10, completion_tokens: 5, other: '{"billing_source":"self_use"}' }] },
})

test('self-use evidence requires one exact user, finite key and associated consume log', () => {
  assert.doesNotThrow(() => verifySyntheticAccounting(accountingFixture(), accountingIdentity))
  for (const [field, value] of [['id', 8], ['quota', 15], ['self_use_no_balance', false], ['used_quota', 0], ['request_count', 2]]) {
    const fixture = accountingFixture()
    fixture.user[field] = value
    assert.throws(() => verifySyntheticAccounting(fixture, accountingIdentity), /DESKTOP_ACCOUNTING_USER_MISMATCH/)
  }
  for (const [field, value] of [['id', 12], ['user_id', 8], ['remain_quota', 1000], ['used_quota', 0], ['unlimited_quota', true]]) {
    const fixture = accountingFixture()
    fixture.token[field] = value
    assert.throws(() => verifySyntheticAccounting(fixture, accountingIdentity), /DESKTOP_ACCOUNTING_KEY_MISMATCH/)
  }
  for (const [field, value] of [['user_id', 8], ['token_id', 12], ['channel', 14], ['request_id', 'other-request'],
    ['type', 1], ['quota', 0], ['prompt_tokens', 9], ['completion_tokens', 6], ['other', '{"billing_source":"wallet"}']]) {
    const fixture = accountingFixture()
    fixture.logs.items[0][field] = value
    assert.throws(() => verifySyntheticAccounting(fixture, accountingIdentity), /DESKTOP_ACCOUNTING_LOG_MISMATCH/)
  }
  const duplicate = accountingFixture()
  duplicate.logs.items.push({ ...duplicate.logs.items[0] })
  duplicate.logs.total = 2
  assert.throws(() => verifySyntheticAccounting(duplicate, accountingIdentity), /DESKTOP_ACCOUNTING_LOG_COUNT_MISMATCH/)
})

test('pending dashboard data is empty and the stopped flush is one exact usage row', () => {
  const row = { user_id: 7, model_name: 'desktop-synthetic-model', count: 1, quota: 15, token_used: 15 }
  assert.doesNotThrow(() => verifyDashboardRows([], 7, 0))
  assert.doesNotThrow(() => verifyDashboardRows([row], 7, 1))
  assert.throws(() => verifyDashboardRows([row], 7, 0), /DESKTOP_DASHBOARD_COUNT_MISMATCH/)
  assert.throws(() => verifyDashboardRows([], 7, 1), /DESKTOP_DASHBOARD_COUNT_MISMATCH/)
  assert.throws(() => verifyDashboardRows([row, row], 7, 1), /DESKTOP_DASHBOARD_COUNT_MISMATCH/)
  for (const [field, value] of [['user_id', 8], ['model_name', 'other-model'], ['count', 2], ['quota', 0], ['token_used', 0]]) {
    assert.throws(() => verifyDashboardRows([{ ...row, [field]: value }], 7, 1), /DESKTOP_DASHBOARD_USAGE_MISMATCH/)
  }
})

test('stopped SQLite readback verifies ledger and aggregate separately without changing database bytes or identity', async t => {
  const { DatabaseSync } = await import('node:sqlite')
  const root = await realpath(await mkdtemp(path.join(os.tmpdir(), 'desktop-accounting-contract-')))
  t.after(() => rm(root, { recursive: true, force: true }))
  const database = path.join(root, 'synthetic.db')
  const db = new DatabaseSync(database)
  try {
    db.exec(`CREATE TABLE users (id INTEGER,quota INTEGER,used_quota INTEGER,request_count INTEGER,self_use_no_balance INTEGER);
      CREATE TABLE tokens (id INTEGER,user_id INTEGER,remain_quota INTEGER,used_quota INTEGER,unlimited_quota INTEGER);
      CREATE TABLE logs (user_id INTEGER,token_id INTEGER,channel_id INTEGER,request_id TEXT,type INTEGER,quota INTEGER,prompt_tokens INTEGER,completion_tokens INTEGER,other TEXT);
      CREATE TABLE quota_data (user_id INTEGER,model_name TEXT,count INTEGER,quota INTEGER,token_used INTEGER);
      INSERT INTO users VALUES (7,0,15,1,1);
      INSERT INTO tokens VALUES (11,7,985,15,0);
      INSERT INTO logs VALUES (7,11,13,'synthetic-local-request',2,15,10,5,'{"billing_source":"self_use"}');
      INSERT INTO quota_data VALUES (7,'desktop-synthetic-model',1,15,15);`)
  } finally { db.close() }
  const before = await readFile(database), identity = await lstat(database, { bigint: true })
  assert.deepEqual(await readStoppedAccounting(database, accountingIdentity), { ledgerVerified: true, dashboardVerified: true })
  assert.deepEqual(await readFile(database), before)
  const after = await lstat(database, { bigint: true })
  assert.deepEqual([after.dev, after.ino, after.size, after.mtimeNs], [identity.dev, identity.ino, identity.size, identity.mtimeNs])
  await assert.rejects(readStoppedAccounting(database, { ...accountingIdentity, userId: 8 }), /DESKTOP_ACCOUNTING_USER_MISMATCH/)
  // A persisted ledger alone cannot prove that the cached dashboard was flushed.
  const changed = new DatabaseSync(database)
  try { changed.exec('DELETE FROM quota_data') } finally { changed.close() }
  await assert.rejects(readStoppedAccounting(database, accountingIdentity), /DESKTOP_DASHBOARD_COUNT_MISMATCH/)
})

test('owned loopback upstream counts each request and rejects a changed synthetic contract', async t => {
  const upstream = await createSyntheticUpstream()
  t.after(() => upstream.close())
  assert.match(upstream.origin, /^http:\/\/127\.0\.0\.1:[1-9][0-9]*$/)
  assert.deepEqual(upstream.stats(), { count: 0, violation: false })
  const request = { model: 'desktop-synthetic-model', stream: false, max_tokens: 8,
    messages: [{ role: 'user', content: 'synthetic desktop qualification' }] }
  const send = body => fetch(upstream.origin + '/v1/chat/completions', { method: 'POST', redirect: 'error',
    signal: AbortSignal.timeout(5000), headers: { 'Content-Type': 'application/json', Authorization: 'Bearer synthetic-desktop-upstream-key' },
    body: JSON.stringify(body) })
  const response = await send(request)
  assert.equal(response.status, 200)
  assert.deepEqual((await response.json()).usage, { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 })
  assert.deepEqual(upstream.stats(), { count: 1, violation: false })
  const rejected = await send({ ...request, model: 'unapproved-model' })
  assert.equal(rejected.status, 400)
  await rejected.arrayBuffer()
  assert.deepEqual(upstream.stats(), { count: 2, violation: true })
  assert.equal(JSON.stringify(upstream.stats()), '{"count":2,"violation":true}')
})
