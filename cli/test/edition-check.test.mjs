import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, realpathSync, rmSync, statSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, test } from 'node:test'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const cli = path.join(root, 'cli/myapi.mjs')
const containerId = '012345abcdef'.padEnd(64, '0')
const imageId = `sha256:${'a'.repeat(64)}`
const temporary = []
afterEach(() => temporary.splice(0).forEach((directory) => rmSync(directory, { recursive: true, force: true })))

function report(overrides = {}) {
  return {
    schema_version: 1, check: 'edition-compatibility', read_only: true, source_edition: 'full', target_edition: 'lite', state: 'compatible', scope: 'configuration', login_continuity: 'not-verified', runtime_login_verified: false,
    facts: {
      schema_version: 1, read_only: true, schema: 'existing', source: 'configured-primary', database: 'sqlite', complete: true, login_continuity: 'not-verified',
      top_ups: { total: 0, pending: 0, terminal: 0, unknown: 0 }, subscription_orders: { total: 0, pending: 0, terminal: 0, unknown: 0 },
      authentication: { password_login_enabled: true, password_login_source: 'startup-default', passkey_enabled: true, passkey_source: 'startup-default', enabled_users: 1, enabled_root_users: 1, password_candidates: 1, passkey_candidates: 0, local_candidates: 1, root_local_candidates: 1, missing_local_candidates: 0, unknown_users: 0, unknown_credential_records: 0 },
    }, blockers: [], unknowns: [], warnings: ['local-login-not-verified'], ...overrides,
  }
}

function fixture(options = {}) {
  const directory = mkdtempSync(path.join(tmpdir(), 'myapi-edition-check-'))
  temporary.push(directory)
  const project = path.join(directory, 'project')
  const deploy = path.join(project, 'deploy')
  const bin = path.join(directory, 'bin')
  mkdirSync(path.join(deploy, 'data'), { recursive: true })
  mkdirSync(path.join(deploy, 'logs'))
  mkdirSync(bin)
  for (const name of ['docker-compose.yml', '.env.example', 'install.sh']) copyFileSync(path.join(root, 'deploy', name), path.join(deploy, name))
  copyFileSync(path.join(root, 'VERSION'), path.join(project, 'VERSION'))
  const configured = options.configured || 'full'
  const restricted = ['lite', 'lan'].includes(configured.trim().toLowerCase())
  const envFile = path.join(deploy, '.env')
  writeFileSync(envFile, `MYAPI_EDITION=${configured}\nMYAPI_IMAGE=ghcr.io/forcemind/${restricted ? 'myapi-lan' : 'myapi'}:previous\nSESSION_SECRET=${'a'.repeat(64)}\nBATCH_UPDATE_ENABLED=${options.batch ?? 'false'}\nMYAPI_ACCOUNTING_CONFIG_VERSION=1\nMYAPI_SESSION_COOKIE_SECURE=${!restricted}\nMYAPI_PUBLIC_URL=${restricted ? 'http://127.0.0.1:3000' : 'https://api.example.test'}\nMYAPI_DATA_DIR=./data\nMYAPI_LOGS_DIR=./logs\n`, { mode: 0o600 })
  const dataFile = path.join(deploy, 'data/one-api.db')
  writeFileSync(dataFile, 'synthetic-existing-database-do-not-replace')
  const metadata = {
    id: containerId, image_id: imageId, image_reference: 'ghcr.io/forcemind/myapi:original', running: String(options.running ?? true), edition: `MYAPI_EDITION=${options.actual ?? 'full'}`,
    data: `bind|${realpathSync(path.join(deploy, 'data'))}|true`, logs: `bind|${realpathSync(path.join(deploy, 'logs'))}|true`, working_dir: '/data', custom_storage: '', ...options.metadata,
  }
  const state = { ids: options.absent ? [] : [containerId], batch: options.actualBatch ?? options.batch ?? 'false', metadata: Object.entries(metadata).map(([key, value]) => `${key}=${value}`).join('\n'), backend: JSON.stringify(options.backend || report()), backendStatus: options.backendStatus ?? 0, backendStderr: 'PRIVATE-INSPECTOR-STDERR' }
  const stateFile = path.join(directory, 'docker-state.json')
  const log = path.join(directory, 'docker-calls.jsonl')
  writeFileSync(stateFile, JSON.stringify(state))
  writeFileSync(path.join(bin, 'docker'), `#!/usr/bin/env node
const fs = require('node:fs');
const args = process.argv.slice(2);
const state = JSON.parse(fs.readFileSync(process.env.MYAPI_EDITION_FIXTURE_STATE, 'utf8'));
fs.appendFileSync(process.env.MYAPI_EDITION_FIXTURE_LOG, JSON.stringify(args) + '\\n');
if (args[0] === 'container' && args[1] === 'ls') { process.stdout.write(state.ids.join('\\n')); process.exit(0); }
if (args[0] === 'container' && args[1] === 'inspect') { process.stdout.write(args[3].startsWith('id=') ? state.metadata + '\\n' : state.batch + '\\n'); process.exit(0); }
if (args[0] === 'exec') { process.stdout.write(state.backend); process.stderr.write(state.backendStderr); process.exit(state.backendStatus); }
if (args[0] !== 'compose') { process.stderr.write('unexpected fake Docker action'); process.exit(98); }
`, { mode: 0o700 })
  const env = { ...process.env, PATH: `${bin}${path.delimiter}${process.env.PATH || ''}`, MYAPI_EDITION_FIXTURE_STATE: stateFile, MYAPI_EDITION_FIXTURE_LOG: log }
  const run = (...args) => spawnSync(process.execPath, [cli, ...args, '--project-dir', project], { env, cwd: root, encoding: 'utf8' })
  return { project, envFile, dataFile, log, env, state, update() { writeFileSync(stateFile, JSON.stringify(state)) }, run,
    doctor(target = 'lite') { return run('doctor', '--target-edition', target, '--json') },
    install() { return spawnSync('bash', [path.join(deploy, 'install.sh')], { env, cwd: project, encoding: 'utf8' }) },
    calls() { return existsSync(log) ? readFileSync(log, 'utf8').trim().split('\n').filter(Boolean).map((line) => JSON.parse(line)) : [] },
  }
}

function snapshotTree(directory) {
  const entries = {}
  for (const item of readdirSync(directory, { withFileTypes: true })) {
    const target = path.join(directory, item.name)
    entries[item.name] = item.isDirectory() ? snapshotTree(target) : { content: readFileSync(target, 'base64'), mode: statSync(target).mode }
  }
  return entries
}

test('target doctor inspects the actual container, executes only its original backend flag and reports configuration without claiming a verified login', () => {
  const f = fixture({ configured: 'lite', actual: 'full' })
  const before = snapshotTree(f.project)
  const result = f.doctor('Lan')
  assert.equal(result.status, 0, result.stderr)
  const output = JSON.parse(result.stdout)
  assert.equal(output.source_edition, 'full', 'an edited target file cannot describe the running source')
  assert.equal(output.target_edition, 'lite')
  assert.equal(output.read_only, true)
  assert.equal(output.scope, 'configuration')
  assert.equal(output.login_continuity, 'not-verified')
  assert.equal(output.runtime_login_verified, false)
  assert.deepEqual(snapshotTree(f.project), before)
  assert.deepEqual(f.calls().at(-1), ['exec', containerId, '/my-api', '--edition-check=lite', '--json'])
  assert.ok(f.calls().every((args) => args[0] === 'container' || args[0] === 'exec'))
  assert.ok(!`${result.stdout}${result.stderr}`.includes('PRIVATE-INSPECTOR-STDERR'))
})

test('a matching original true batch choice permits readonly configuration facts without changing or certifying cache accounting', () => {
  const f = fixture({ batch: 'true', actualBatch: 'true' })
  const before = snapshotTree(f.project)
  const result = f.doctor()
  assert.equal(result.status, 0)
  const output = JSON.parse(result.stdout)
  assert.equal(output.scope, 'configuration')
  assert.equal(output.runtime_login_verified, false)
  assert.equal(Object.hasOwn(output, 'accounting_verified'), false)
  assert.equal(Object.hasOwn(output, 'cache_verified'), false)
  assert.deepEqual(snapshotTree(f.project), before)
  assert.deepEqual(f.calls().at(-1), ['exec', containerId, '/my-api', '--edition-check=lite', '--json'])
  assert.ok(f.calls().every((args) => args[0] === 'container' || args[0] === 'exec'))
})

for (const options of [{ batch: '' }, { batch: 'TRUE' }, { batch: 'true', actualBatch: 'false' }]) {
  test(`unknown or mismatched batch configuration remains unverifiable without any action: ${JSON.stringify(options)}`, () => {
    const f = fixture(options)
    const before = snapshotTree(f.project)
    const result = f.doctor()
    assert.equal(result.status, 2)
    assert.equal(JSON.parse(result.stdout).state, 'unverifiable')
    assert.equal(f.calls().some((args) => args[0] === 'exec'), false)
    assert.deepEqual(snapshotTree(f.project), before)
  })
}

test('blocked and unverifiable backend verdicts keep their exact 1/2 exits and null facts, never fabricated zero counts', () => {
  for (const [state, status, blockers, unknowns] of [['blocked', 1, ['pending-top-ups'], []], ['unverifiable', 2, [], ['database-unavailable']]]) {
    const f = fixture({ backend: report({ state, facts: null, blockers, unknowns }), backendStatus: status })
    const result = f.doctor()
    assert.equal(result.status, status)
    const output = JSON.parse(result.stdout)
    assert.equal(output.state, state)
    assert.equal(output.facts, null)
    assert.deepEqual(output.blockers, blockers)
    assert.deepEqual(output.unknowns, unknowns)
  }
})

test('a real fresh-primary report retains nullable facts while anonymous-memory freshness is not accepted as the running primary', () => {
  for (const source of ['configured-primary', 'new-anonymous-memory']) {
    const facts = { ...report().facts, schema: 'fresh', source, top_ups: null, subscription_orders: null, authentication: null }
    const f = fixture({ backend: report({ facts }) })
    const result = f.doctor()
    const output = JSON.parse(result.stdout)
    assert.equal(result.status, source === 'configured-primary' ? 0 : 2)
    if (source === 'configured-primary') {
      assert.equal(output.facts.schema, 'fresh')
      assert.equal(output.facts.top_ups, null)
      assert.equal(output.facts.authentication, null)
    } else assert.equal(output.facts, null)
    assert.equal(output.runtime_login_verified, false)
  }
})

test('unknown database is only a fixed unverifiable value and Full no-reduction reports need no fabricated facts', () => {
  const unknown = fixture({ backendStatus: 2, backend: report({ state: 'unverifiable', facts: { ...report().facts, schema: 'unavailable', database: 'unknown', complete: false, top_ups: null, subscription_orders: null, authentication: null }, unknowns: ['database-unavailable'] }) })
  const result = unknown.doctor()
  assert.equal(result.status, 2)
  assert.equal(JSON.parse(result.stdout).facts.database, 'unknown')
  const full = fixture({ backend: report({ target_edition: 'full', facts: null, warnings: ['local-login-not-verified', 'full-edition-no-feature-reduction'] }) })
  const noReduction = full.doctor('full')
  assert.equal(noReduction.status, 0)
  assert.equal(JSON.parse(noReduction.stdout).facts, null)
  assert.deepEqual(full.calls().at(-1), ['exec', containerId, '/my-api', '--edition-check=full', '--json'])
})

test('unknown additional backend fields are stripped at every reported level without exposing their private values', () => {
  const facts = report().facts
  const f = fixture({ backend: report({ private: 'PRIVATE-HEADER', facts: { ...facts, private: 'PRIVATE-FACTS', authentication: { ...facts.authentication, credential: 'PRIVATE-CREDENTIAL' } } }) })
  const result = f.doctor()
  assert.equal(result.status, 0)
  assert.ok(!result.stdout.includes('PRIVATE-'))
  assert.equal(JSON.parse(result.stdout).facts.authentication.password_candidates, 1)
})

for (const mismatch of ['source', 'target', 'exit', 'unsafe-count', 'wrong-type', 'database-binding', 'initialization-scope', 'unknown-code', 'verified-login']) {
  test(`malformed or mismatched backend ${mismatch} is unverifiable and sanitized`, () => {
    const backend = report()
    let backendStatus = 0
    if (mismatch === 'source') backend.source_edition = 'lite'
    if (mismatch === 'target') backend.target_edition = 'full'
    if (mismatch === 'exit') backendStatus = 1
    if (mismatch === 'unsafe-count') backend.facts.top_ups.total = Number.MAX_SAFE_INTEGER + 1
    if (mismatch === 'wrong-type') backend.facts.authentication.enabled_users = '1'
    if (mismatch === 'database-binding') backend.facts.database = 'mysql'
    if (mismatch === 'initialization-scope') backend.scope = 'initialization'
    if (mismatch === 'unknown-code') backend.unknowns = ['PRIVATE-UNSUPPORTED-CODE']
    if (mismatch === 'verified-login') backend.runtime_login_verified = true
    const f = fixture({ backend, backendStatus })
    const result = f.doctor()
    const output = JSON.parse(result.stdout)
    assert.equal(result.status, 2)
    assert.equal(output.state, 'unverifiable')
    assert.equal(output.facts, null)
    assert.deepEqual(output.unknowns, ['schema-unverifiable'])
    assert.ok(!`${result.stdout}${result.stderr}`.includes('PRIVATE-'))
  })
}

test('stopped containers are not execed for doctor and old unsupported binaries remain unknown without a pull or start', () => {
  for (const options of [{ running: false }, { backendStatus: 64 }]) {
    const f = fixture(options)
    const before = snapshotTree(f.project)
    const result = f.doctor()
    assert.equal(result.status, 2)
    assert.equal(JSON.parse(result.stdout).source_edition, 'full')
    if (options.running === false) assert.equal(f.calls().some((args) => args[0] === 'exec'), false)
    assert.ok(f.calls().every((args) => args[0] === 'container' || args[0] === 'exec'))
    assert.deepEqual(snapshotTree(f.project), before)
  }
})

for (const metadata of [
  { edition: 'MYAPI_EDITION=pro' }, { edition: '' }, { image_id: 'unknown-image' }, { data: 'bind|/other-data|true' },
  { logs: 'volume|other-logs|true' }, { custom_storage: 'custom' }, { working_dir: '/other' },
]) {
  test(`unverified actual metadata or primary binding blocks before writes and never falls back to the target file: ${JSON.stringify(metadata)}`, () => {
    const f = fixture({ metadata })
    const before = snapshotTree(f.project)
    assert.equal(f.doctor().status, 2)
    assert.equal(f.run('up').status, 1)
    assert.equal(f.run('upgrade', '--version', 'v0.2.0').status, 1)
    assert.equal(f.install().status, 1)
    assert.deepEqual(snapshotTree(f.project), before)
    assert.equal(existsSync(path.join(f.project, 'backups')), false)
    assert.ok(f.calls().every((args) => args[0] === 'container'))
  })
}

for (const [configured, actual] of [['lite', 'full'], ['full', 'lite']]) {
  test(`ordinary ${actual} to ${configured} requests cannot recreate the source through up, upgrade or installer`, () => {
    const f = fixture({ configured, actual })
    const before = snapshotTree(f.project)
    for (const result of [f.run('up'), f.run('upgrade', '--version', 'v0.2.0'), f.install()]) {
      assert.equal(result.status, 1)
      assert.match(result.stderr, /ordinary upgrades cannot change/i)
      assert.match(result.stderr, /doctor --target-edition/)
    }
    assert.deepEqual(snapshotTree(f.project), before)
    assert.equal(existsSync(path.join(f.project, 'backups')), false)
    assert.ok(f.calls().every((args) => args[0] === 'container'))
  })
}

for (const [configured, actual] of [['full', 'FULL'], ['full', '   '], ['lite', 'lite'], ['lan', 'lite'], ['lite', ' Lan ']]) {
  test(`same-edition ${actual}/${configured} start and upgrade preserve normal paths even for a stopped old binary without the new checker`, () => {
    const f = fixture({ configured, actual, running: false, backendStatus: 64 })
    assert.equal(f.run('up').status, 0)
    assert.equal(f.install().status, 0)
    const before = readFileSync(f.envFile, 'utf8')
    assert.equal(f.run('upgrade', '--version', 'v0.2.0').status, 0)
    assert.ok(f.calls().some((args) => args[0] === 'compose' && args.includes('up')))
    assert.equal(f.calls().some((args) => args[0] === 'exec'), false)
    assert.match(readFileSync(f.envFile, 'utf8'), new RegExp(`^MYAPI_EDITION=${configured}$`, 'm'))
    const backups = readdirSync(path.join(f.project, 'backups'))
    assert.equal(backups.length, 1)
    assert.equal(readFileSync(path.join(f.project, 'backups', backups[0]), 'utf8'), before)
    assert.equal(readFileSync(f.dataFile, 'utf8'), 'synthetic-existing-database-do-not-replace')
  })
}

test('missing source and invalid requested editions are explicit unknown reports and cannot borrow another container or create state', () => {
  const f = fixture({ absent: true })
  const before = snapshotTree(f.project)
  const unavailable = f.doctor()
  assert.equal(unavailable.status, 2)
  assert.equal(JSON.parse(unavailable.stdout).source_edition, null)
  assert.equal(f.calls().some((args) => args[0] === 'exec'), false)
  const previousCalls = f.calls().length
  const invalid = f.doctor('pro')
  assert.equal(invalid.status, 2)
  assert.equal(JSON.parse(invalid.stdout).target_edition, null)
  assert.deepEqual(JSON.parse(invalid.stdout).unknowns, ['invalid-request'])
  assert.equal(f.calls().length, previousCalls)
  assert.deepEqual(snapshotTree(f.project), before)
})
