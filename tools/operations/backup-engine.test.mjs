import assert from 'node:assert/strict'
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import test from 'node:test'
import { BackupEngine } from './backup-engine.mjs'
import { OperationsEngine } from './engine.mjs'
import { JournalStore } from './store.mjs'
import { OperationsError } from './safety.mjs'

const sourceId = '1'.repeat(64)
const imageId = 'sha256:' + '2'.repeat(64)
const treeHash = '3'.repeat(64)
const manifestHash = '4'.repeat(64)

async function fixture(t, options = {}) {
  const root = await mkdtemp(path.join(os.tmpdir(), 'myapi-backup-engine-'))
  t.after(() => rm(root, { recursive: true, force: true }))
  for (const name of ['data', 'state', 'backups', 'restores']) await mkdir(path.join(root, name), { mode: 0o700 })
  await writeFile(path.join(root, 'data', 'my-api.db'), 'fixture-boundary; real SQLite is covered by the bundle suite')
  const instance = { id: 'primary', container: 'myapi-backup-fixture', data_dir: path.join(root, 'data'), sqlite_file: path.join(root, 'data', 'my-api.db'), health_url: 'http://127.0.0.1:18080/' }
  const config = { docker_socket: '/var/run/docker.sock', state_dir: path.join(root, 'state'), backups_dir: path.join(root, 'backups'), instances: [instance] }
  const source = { Id: sourceId, Image: imageId, Config: { Env: ['SQLITE_PATH=/data/my-api.db', 'SESSION_SECRET=synthetic-private-value'], WorkingDir: '/data' },
    HostConfig: { RestartPolicy: { Name: 'always', MaximumRetryCount: 0 }, NetworkMode: 'bridge', PortBindings: { '3000/tcp': [{ HostIp: '127.0.0.1', HostPort: '18080' }] } },
    Mounts: [{ Type: 'bind', Source: instance.data_dir, Destination: '/data', RW: true }],
    State: { Running: true, Pid: 4321, Status: 'running', ExitCode: 0, OOMKilled: false, Paused: false, Restarting: false },
    NetworkSettings: { Networks: { bridge: {} }, Ports: { '3000/tcp': [{ HostIp: '127.0.0.1', HostPort: '18080' }] } } }
  const calls = [], bundleCalls = [], writerCalls = []
  let now = 1000, scan = 0, currentTree = treeHash
  const binding = { source_process: { pid: 4321, starttime: '12345' }, fingerprint: 'fixture-visible-binding' }
  const docker = {
    async inspectContainer(id) { assert.ok([sourceId, instance.container].includes(id)); return structuredClone(source) },
    async inspectImage(id) { assert.equal(id, imageId); return { Id: imageId, Os: 'linux', Architecture: 'amd64', Config: { Labels: { 'io.myapi.edition': 'full' } } } },
    async listContainers() { return [{ Id: sourceId }] },
    async disableRestart(id) { calls.push('disable-restart'); assert.equal(id, sourceId); source.HostConfig.RestartPolicy = { Name: 'no', MaximumRetryCount: 0 } },
    async stopForBackup(id) {
      calls.push('stop-without-kill'); assert.equal(id, sourceId)
      if (options.stopUnknown) throw new OperationsError('DOCKER_FAILED', 503)
      source.State = { ...source.State, Running: false, Status: 'exited', Pid: 0 }
    },
    async start() { assert.fail('backup and restore must never start or replay a container') },
    async create() { assert.fail('backup and restore must never create a recovery container') },
  }
  const bundle = async input => {
    bundleCalls.push(structuredClone(input))
    if (options.bundleErrorAt === input.action) throw new OperationsError('HELPER_FAILED', 503)
    const common = { schema_version: 1, manifest_sha256: manifestHash, tree_sha256: currentTree, total_bytes: 1234 }
    if (input.action === 'stopped_scan') return common
    assert.equal(input.data_dir, instance.data_dir)
    assert.equal(input.sqlite_file, instance.sqlite_file)
    assert.equal(input.backups_dir, config.backups_dir)
    assert.equal(input.state_dir, config.state_dir)
    assert.match(input.identity_sha256, /^[a-f0-9]{64}$/)
    if (input.action === 'bundle_prepare') return { ...common, prepared: true, committed: false }
    assert.equal(input.expected_manifest_sha256, manifestHash)
    if (input.action === 'bundle_commit') { assert.equal(input.expected_tree_sha256, currentTree); return { ...common, committed: true, verified: true } }
    if (input.action === 'bundle_verify') return { ...common, verified: true, committed: true }
    if (input.action === 'bundle_restore') return { ...common, restored: true, verified: true }
    assert.fail('unexpected bundle action')
  }
  const writer = async input => {
    writerCalls.push(structuredClone(input))
    if (input.action === 'bind_running') return { schema_version: 1, action: input.action, status: 'observed', binding,
      evidence: { tree_entries: Object.hasOwn(options, 'treeEntries') ? options.treeEntries : 2 } }
    scan++
    if (options.writerAtScan === scan) throw new OperationsError('HELPER_FAILED', 503)
    return { schema_version: 1, action: input.action, status: 'observed', binding, evidence: { old_generation_gone: true, fds: 0, shared_maps: 0 } }
  }
  const engine = await new BackupEngine(config, { docker, bundle, writer, capacity: async () => {}, clock: () => now }).init()
  return { root, config, instance, source, calls, bundleCalls, writerCalls, engine, docker, setNow(value) { now = value }, setTree(value) { currentTree = value } }
}

test('a concrete plan exposes effects and confirmation without stopping or copying or printing credentials', async t => {
  const f = await fixture(t)
  const plan = await f.engine.prepareBackup('primary')
  assert.equal(plan.state, 'prepared')
  assert.deepEqual(f.calls, [])
  assert.deepEqual(f.bundleCalls, [])
  assert.deepEqual(plan.retention, { policy: 'keep-all', deletions: 0 })
  assert.equal(plan.restarts_instance, false)
  assert.equal(plan.overwrites_directory, false)
  assert.deepEqual(plan.original_restart_policy, { Name: 'always', MaximumRetryCount: 0 })
  assert.doesNotMatch(JSON.stringify(plan), /SESSION_SECRET|synthetic-private-value/)
  assert.equal(plan.destination, path.join(f.config.backups_dir, plan.id))
})

test('wrong digest, expiry and changed instance configuration cannot stop or copy the source', async t => {
  const f = await fixture(t)
  const plan = await f.engine.prepareBackup('primary')
  await assert.rejects(f.engine.execute(plan.id, '0'.repeat(64), 'backup', 'primary'), { code: 'PLAN_CONFIRMATION_INVALID' })
  f.setNow(plan.expires_at)
  await assert.rejects(f.engine.execute(plan.id, plan.digest, 'backup', 'primary'), { code: 'PLAN_CONFIRMATION_INVALID' })
  f.setNow(1001)
  f.source.Config.Env.push('PRESERVE_EXTRA_SWITCH=1')
  const rejected = await f.engine.execute(plan.id, plan.digest, 'backup', 'primary')
  assert.equal(rejected.state, 'failed_before_start')
  assert.equal(rejected.code, 'SOURCE_CONFIG_CHANGED')
  assert.deepEqual(f.calls, [])
  assert.deepEqual(f.bundleCalls, [])
})

test('missing or out-of-range entry evidence cannot become an invented zero or authorize stopping', async t => {
  for (const treeEntries of [undefined, null, 0, 10001]) {
    const f = await fixture(t, { treeEntries })
    await assert.rejects(f.engine.prepareBackup('primary'), { code: 'BACKUP_ENTRY_LIMIT_UNCONFIRMED' })
    assert.deepEqual(f.calls, [])
    assert.deepEqual(f.bundleCalls, [])
  }
})

test('unconfirmed backup preparation or commit stays unknown rather than reporting an absent backup', async t => {
  for (const bundleErrorAt of ['bundle_prepare', 'bundle_commit']) {
    const f = await fixture(t, { bundleErrorAt })
    const plan = await f.engine.prepareBackup('primary')
    const result = await f.engine.execute(plan.id, plan.digest, 'backup', 'primary')
    assert.equal(result.state, 'recovery_required')
    assert.equal(result.backup_created, null)
    assert.equal(f.engine.store.frozen.has('primary'), true)
    assert.equal(f.bundleCalls.filter(value => value.action === bundleErrorAt).length, 1)
  }
})

test('backup confirms a normal stop, rechecks writers before committing, and cannot replay', async t => {
  const f = await fixture(t)
  const plan = await f.engine.prepareBackup('primary')
  const result = await f.engine.execute(plan.id, plan.digest, 'backup', 'primary')
  assert.equal(result.state, 'succeeded')
  assert.equal(result.backup_created, true)
  assert.deepEqual(f.calls, ['disable-restart', 'stop-without-kill'])
  assert.deepEqual(f.bundleCalls.map(item => item.action), ['bundle_prepare', 'bundle_commit'])
  assert.equal(f.writerCalls.filter(item => item.action === 'stopped_scan').length, 2)
  assert.equal(f.source.State.Running, false)
  assert.equal(f.source.HostConfig.RestartPolicy.Name, 'no')
  assert.deepEqual(result.original_restart_policy, { Name: 'always', MaximumRetryCount: 0 }, 'disabling restart must not discard its original configuration')
  await assert.rejects(f.engine.execute(plan.id, plan.digest, 'backup', 'primary'), { code: 'PLAN_CONFIRMATION_INVALID' })
  assert.equal(f.bundleCalls.length, 2)
  const reopened = new JournalStore(f.config.state_dir)
  await reopened.init()
  assert.equal(reopened.current.size, 0)
  assert.equal(reopened.frozen.size, 0)
})

for (const [name, options, expectedActions] of [
  ['unknown stop', { stopUnknown: true }, []],
  ['writer after stop', { writerAtScan: 1 }, []],
  ['writer after preparing backup', { writerAtScan: 2 }, ['bundle_prepare']],
]) {
  test(`${name} freezes without a committed backup, restart or retry`, async t => {
    const f = await fixture(t, options)
    const plan = await f.engine.prepareBackup('primary')
    const result = await f.engine.execute(plan.id, plan.digest, 'backup', 'primary')
    assert.equal(result.state, 'recovery_required')
    assert.equal(result.backup_created, false)
    assert.deepEqual(f.bundleCalls.map(item => item.action), expectedActions)
    assert.equal(f.engine.store.frozen.has('primary'), true)
    const reopened = new JournalStore(f.config.state_dir)
    await reopened.init()
    assert.equal(reopened.frozen.has('primary'), true)
    await assert.rejects(f.engine.prepareBackup('primary'), { code: 'INSTANCE_FROZEN' })
    assert.deepEqual(f.calls, ['disable-restart', 'stop-without-kill'])
  })
}

test('isolated restore preserves the stopped source and refuses changed source accounting before a copy', async t => {
  const f = await fixture(t)
  const prepared = await f.engine.prepareBackup('primary')
  const backup = await f.engine.execute(prepared.id, prepared.digest, 'backup', 'primary')
  const target = path.join(f.root, 'restores', 'new-instance')
  // Production target permissions are root-only; this suite exercises the
  // orchestration using the current user's controlled fixture directory.
  if (process.getuid() !== 0) {
    f.engine.assertRestoreTarget = async candidate => {
      assert.equal(candidate, target)
      await assert.rejects(lstatMissing(candidate), { code: 'ENOENT' })
    }
  }
  const restore = await f.engine.prepareRestore('primary', backup.backup_id, target)
  f.setTree('5'.repeat(64))
  const changed = await f.engine.execute(restore.id, restore.digest, 'restore', 'primary', { backupId: backup.backup_id, target })
  assert.equal(changed.state, 'failed_before_start')
  assert.equal(changed.code, 'SOURCE_CHANGED_SINCE_BACKUP')
  assert.equal(f.bundleCalls.some(item => item.action === 'bundle_restore'), false)
  f.setTree(treeHash)
  const newPlan = await f.engine.prepareRestore('primary', backup.backup_id, target)
  const restored = await f.engine.execute(newPlan.id, newPlan.digest, 'restore', 'primary', { backupId: backup.backup_id, target })
  assert.equal(restored.state, 'succeeded')
  assert.equal(restored.restored, true)
  assert.equal(restored.starts_recovery_instance, false)
  assert.deepEqual(f.calls, ['disable-restart', 'stop-without-kill'])
  assert.equal(f.source.State.Running, false)
})

async function lstatMissing(filename) { return import('node:fs/promises').then(fs => fs.lstat(filename)) }

test('typed local backup journals neither alter the legacy upgrade tip nor enter remote upgrade operations', async t => {
  const f = await fixture(t)
  const legacy = { id: 'a'.repeat(64), instance_id: 'primary', state: 'succeeded', source_container_id: 'b'.repeat(64), target_container_id: sourceId }
  await f.engine.store.save(legacy)
  await f.engine.store.init()
  const plan = await f.engine.prepareBackup('primary')
  await f.engine.execute(plan.id, plan.digest, 'backup', 'primary')
  const reopened = new JournalStore(f.config.state_dir)
  await reopened.init()
  assert.equal(reopened.current.get('primary').id, legacy.id)
  assert.equal(reopened.frozen.has('primary'), false)
  const remote = new OperationsEngine(f.config, { docker: f.docker, github: {}, store: reopened })
  assert.throws(() => remote.get(plan.id), { code: 'PLAN_NOT_FOUND' })
  assert.deepEqual(remote.list({ owner_id: 1 }, 'primary'), [])
  const persisted = JSON.parse(await readFile(path.join(f.config.state_dir, plan.id + '.json'), 'utf8'))
  persisted.kind = 'unexpected'
  await writeFile(path.join(f.config.state_dir, plan.id + '.json'), JSON.stringify(persisted), { mode: 0o600 })
  await assert.rejects(new JournalStore(f.config.state_dir).init(), { code: 'JOURNAL_CORRUPT' })
})
