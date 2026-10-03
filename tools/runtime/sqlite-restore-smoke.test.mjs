import assert from 'node:assert/strict'
import test from 'node:test'
import { mkdtempSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { verifySQLiteRestore, withRestoredSQLite } from './sqlite-restore-smoke.mjs'

const sha = 'a'.repeat(40)
const source = `myapi-smoke-full-${sha}`
const image = `myapi:smoke-full-${sha}`
const target = `myapi-smoke-restore-full-${sha}`
const json = (data) => new Response(JSON.stringify({ success: true, data }))

function fixture(t, overrides = {}) {
  const temp = mkdtempSync(path.join(os.tmpdir(), 'myapi-restore-test-'))
  t.after(() => rmSync(temp, { recursive: true, force: true }))
  const commands = []
  const env = { GITHUB_ACTIONS: 'true', MYAPI_ISOLATED_SMOKE: '1', MYAPI_SMOKE_RESTORE: '1', RUNNER_TEMP: temp, SMOKE_CONTAINER: source, SMOKE_IMAGE: image,
    SMOKE_DATA_DIR: path.join(temp, `myapi-smoke-data-full-${sha}`) }
  const runDocker = (args, commandEnv) => {
    commands.push(args)
    if (args[0] === 'inspect') {
      if (args[2].includes('Labels')) return overrides.foreign ? 'foreign' : sha
      if (args[2] === '{{.State.Running}}') return 'true'
      if (args[2] === '{{.Config.Image}}') return image
      if (args[2].includes('.Mounts')) return overrides.tmpfs ? 'tmpfs:' : `bind:${env.SMOKE_DATA_DIR}`
    }
    if (args[0] === 'ps') return overrides.existing ? target : ''
    if (args[0] === 'cp') {
      if (overrides.copyFailure) throw new Error('SMOKE_RESTORE_DOCKER_FAILED')
      assert.equal(args[1], `${source}:/data/.`)
      writeFileSync(path.join(args[2], 'my-api.db'), 'SQLite format 3\u0000synthetic database')
      writeFileSync(path.join(args[2], 'my-api.db-wal'), 'synthetic WAL')
    }
    if (args[0] === 'run') {
      assert.equal(args.includes('--privileged'), false)
      assert.ok(args.includes('127.0.0.1:18081:3000'))
      assert.equal(args.at(-1), image)
      assert.match(commandEnv.SESSION_SECRET, /^[a-f0-9]{64}$/)
      assert.equal(args.includes(commandEnv.SESSION_SECRET), false)
      const dir = args[args.indexOf('--mount') + 1].match(/^type=bind,src=(.*),dst=\/data$/)[1]
      assert.equal(statSync(path.dirname(dir)).mode & 0o777, 0o700)
      assert.equal(readFileSync(path.join(dir, 'my-api.db-wal'), 'utf8'), 'synthetic WAL')
      // The restored process can change its copy without changing the backup.
      writeFileSync(path.join(dir, 'my-api.db-wal'), 'restored WAL')
    }
    return ''
  }
  return { temp, commands, env, runDocker }
}

test('SQLite restore pauses only the owned fixture and copies DB plus WAL into a separate same-image instance', async (t) => {
  const f = fixture(t)
  const result = await withRestoredSQLite({ ...f, sha, edition: 'full', verify: async (url) => {
    assert.equal(url, 'http://127.0.0.1:18081')
    return 'verified'
  } })
  assert.equal(result, 'verified')
  assert.deepEqual(f.commands.filter((args) => ['pause', 'cp', 'unpause', 'run', 'rm'].includes(args[0])).map((args) => args[0]), ['pause', 'cp', 'unpause', 'run', 'rm'])
  assert.deepEqual(f.commands.at(-1), ['rm', '--force', target])
  assert.deepEqual(readdirSync(f.temp), [])
})

test('SQLite restore rejects foreign ownership, existing target and missing opt-in before mutation', async (t) => {
  for (const options of [{ foreign: true }, { existing: true }, { optOut: true }, { tmpfs: true }]) {
    const f = fixture(t, options)
    if (options.optOut) f.env.MYAPI_SMOKE_RESTORE = '0'
    await assert.rejects(withRestoredSQLite({ ...f, sha, edition: 'full', verify: async () => {} }), /SMOKE_RESTORE_SCOPE_REJECTED/)
    assert.equal(f.commands.some((args) => ['pause', 'cp', 'run', 'rm'].includes(args[0])), false)
  }
})

test('copy failure unpauses source without starting a restore; failed verification removes only the clone', async (t) => {
  const copy = fixture(t, { copyFailure: true })
  await assert.rejects(withRestoredSQLite({ ...copy, sha, edition: 'full', verify: async () => assert.fail() }), /DOCKER_FAILED/)
  assert.deepEqual(copy.commands.at(-1), ['unpause', source])
  assert.equal(copy.commands.some((args) => args[0] === 'run'), false)
  const verification = fixture(t)
  await assert.rejects(withRestoredSQLite({ ...verification, sha, edition: 'full', verify: async () => { throw new Error('verification failed') } }), /verification failed/)
  assert.deepEqual(verification.commands.at(-1), ['rm', '--force', target])
})

test('uncertain clone removal retains its private directory instead of unlinking a live mount', async (t) => {
  const f = fixture(t)
  const runDocker = (args, env) => {
    if (args[0] === 'rm') throw new Error('SMOKE_RESTORE_DOCKER_FAILED')
    return f.runDocker(args, env)
  }
  await assert.rejects(withRestoredSQLite({ ...f, runDocker, sha, edition: 'full', verify: async () => {} }), /DOCKER_FAILED/)
  const names = readdirSync(f.temp)
  assert.equal(names.length, 1)
  assert.equal(statSync(path.join(f.temp, names[0])).mode & 0o777, 0o700)
})

function apiFixture(tamper = '') {
  let relays = 0
  const fetchImpl = async (url, options = {}) => {
    const { origin, pathname } = new URL(url)
    assert.ok(['http://127.0.0.1:18080', 'http://127.0.0.1:18081'].includes(origin))
    assert.equal(options.redirect, 'error')
    const restored = origin.endsWith(':18081')
    if (pathname.startsWith('/v1/')) { relays += 1; assert.fail('restoration must not resend a relay') }
    switch (pathname) {
      case '/api/status': return json({ setup: true })
      case '/api/setup': return json({ status: true })
      case '/api/user/login':
        assert.deepEqual(JSON.parse(options.body), { username: 'smokeadmin', password: 'synthetic-password' })
        return json({ access_token: 'synthetic-session' })
      case '/api/user/self': return json({ id: 1, quota: 0, self_use_no_balance: true, usage_policy_revision: 1, used_quota: 15, request_count: 1 })
      case '/api/token/search': return json({ items: [{ id: 22, name: 'smoke-self-use-key' }] })
      case '/api/token/22': return json({ id: 22, remain_quota: restored && tamper === 'quota' ? 984 : 985, used_quota: 15 })
      case '/api/token/22/key': return json({ key: restored && tamper === 'key' ? 'different-synthetic-key' : 'synthetic-key' })
      case '/api/user/search': return json({ items: [{ id: 11, username: 'smokeuser', quota: 999985, used_quota: 15, request_count: 1 }] })
      case '/api/channel/search': return json({ items: [{ id: 13, name: 'smoke-openai-channel' }] })
      case '/api/channel/13': return json({ id: 13, used_quota: 30 })
      case '/api/log/': return json({ total: 2, items: [
        { id: 1, request_id: 'ordinary-request', user_id: 11, token_id: 12, quota: 15 },
        { id: 2, request_id: restored && tamper === 'log' ? 'different-request' : 'self-use-request', user_id: 1, token_id: 22, quota: 15 },
      ] })
      default: assert.fail('unexpected fixture API')
    }
  }
  return { fetchImpl, countRelays: () => relays }
}

test('restored snapshot requires same login, policy, Key secret, exact usage and log identities without exposing them', async () => {
  const f = apiFixture()
  const result = await verifySQLiteRestore({ baseUrl: 'http://127.0.0.1:18080', username: 'smokeadmin', password: 'synthetic-password', ...f,
    restore: (verify) => verify('http://127.0.0.1:18081') })
  assert.equal(result.ok, true)
  assert.equal(f.countRelays(), 0)
  for (const secret of ['synthetic-password', 'synthetic-key', 'synthetic-session', 'self-use-request']) assert.equal(JSON.stringify(result).includes(secret), false)
})

test('restore refuses changed quota, Key or log identity and never resends a billable request', async () => {
  for (const tamper of ['quota', 'key', 'log']) {
    const f = apiFixture(tamper)
    await assert.rejects(verifySQLiteRestore({ baseUrl: 'http://127.0.0.1:18080', username: 'smokeadmin', password: 'synthetic-password', ...f,
      restore: (verify) => verify('http://127.0.0.1:18081') }), /SMOKE_RESTORE_STATE_MISMATCH/)
    assert.equal(f.countRelays(), 0)
  }
})
