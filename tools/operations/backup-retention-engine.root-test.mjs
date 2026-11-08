import assert from 'node:assert/strict'
import { lstat, mkdir, mkdtemp, readFile, readdir, rm, writeFile } from 'node:fs/promises'
import path from 'node:path'
import test from 'node:test'
import { BackupEngine } from './backup-engine.mjs'
import { JournalStore } from './store.mjs'
import { digest, OperationsError } from './safety.mjs'

assert.equal(process.platform, 'linux')
assert.equal(process.arch, 'x64')
assert.equal(process.getuid(), 0, 'retention owner/protectedRead boundaries require sudo; no skip')

const id = number => number.toString(16).padStart(64, '0')
const now = 2_000_000_000

// Real root-owned filesystem and JournalStore; helpers and Docker are explicit
// fixed synthetic responses. No real deletion or Docker qualification is claimed.
async function fixture(t) {
  const root = await mkdtemp('/tmp/myapi-retention-root-')
  t.after(() => rm(root, { recursive: true, force: true }))
  for (const name of ['state', 'backups', 'data']) await mkdir(path.join(root, name), { mode: 0o700 })
  const source = path.join(root, 'data', 'source-marker')
  await writeFile(source, 'SYNTHETIC_SOURCE_NEVER_REWRITTEN', { mode: 0o600 })
  const sourceBefore = await lstat(source, { bigint: true })
  const config = { docker_socket: '/var/run/docker.sock', state_dir: path.join(root, 'state'), backups_dir: path.join(root, 'backups'),
    instances: [{ id: 'primary', container: 'synthetic-source', data_dir: path.join(root, 'data'), sqlite_file: path.join(root, 'data', 'my-api.db') }] }
  const helperCalls = [], writerCalls = [], dockerCalls = [], observations = new Map(), writerModes = new Map(), badVerify = new Set()
  let containers = [], deletionMode = 'success', onPrune = null
  const deny = async () => { assert.fail('pruning must never inspect, start, stop, restart or write the source') }
  const docker = {
    async listContainers() { dockerCalls.push('list'); return containers.map(value => ({ Id: value.Id })) },
    async inspectContainer(value) { dockerCalls.push('inspect'); assert.ok(containers.some(item => item.Id === value)); return structuredClone(containers.find(item => item.Id === value)) },
    stopForBackup: deny, disableRestart: deny, start: deny, create: deny, inspectImage: deny,
  }
  const bundle = async input => {
    helperCalls.push(structuredClone(input))
    assert.ok(['bundle_verify', 'bundle_prune'].includes(input.action))
    const observation = observations.get(input.bundle_id)
    assert.ok(observation, 'helper must receive only an explicit fixture archive ID')
    assert.equal(input.data_dir, config.instances[0].data_dir)
    assert.equal(input.expected_manifest_sha256, observation.manifest_sha256)
    if (input.action === 'bundle_verify') {
      if (badVerify.has(input.bundle_id)) throw new OperationsError('BACKUP_VERIFICATION_FAILED', 409)
      return { verified: true, ...observation }
    }
    if (onPrune) await onPrune(input)
    if (deletionMode === 'throw') throw new OperationsError('HELPER_FAILED', 503)
    if (deletionMode === 'unknown') return { deleted: null, bundle_id: input.bundle_id }
    return { deleted: true, bundle_id: input.bundle_id, manifest_sha256: observation.manifest_sha256, logical_bytes: observation.total_bytes }
  }
  const writer = async input => {
    writerCalls.push(structuredClone(input))
    assert.equal(input.action, 'archive_scan')
    const selected = path.basename(input.archive_dir)
    assert.ok(observations.has(selected))
    const mode = writerModes.get(selected)
    if (mode === 'throw') throw new OperationsError('HELPER_FAILED', 503)
    if (mode === 'unknown' || mode === 'blocked') return { schema_version: 1, action: 'archive_scan', status: mode, code: 'SYNTHETIC_USAGE_NOT_CLEAR' }
    const root = await lstat(input.archive_dir, { bigint: true })
    return { schema_version: 1, action: 'archive_scan', status: 'observed', deletion_authorized: false,
      binding: { archive_root: { dev: root.dev.toString(), ino: root.ino.toString() } }, evidence: { manifest_verified: false } }
  }
  const engine = await new BackupEngine(config, { docker, bundle, writer, capacity: async () => {}, clock: () => now }).init()
  for (const number of [1, 2, 3]) {
    const directory = path.join(config.backups_dir, id(number))
    await mkdir(path.join(directory, 'data'), { recursive: true, mode: 0o700 })
    const manifest = { created_unix_seconds: now - (3 - number) * 86400, entries: [{ path: 'payload' }] }
    const raw = JSON.stringify(manifest)
    await writeFile(path.join(directory, 'manifest.json'), raw, { mode: 0o600 })
    await writeFile(path.join(directory, 'COMMITTED'), 'SYNTHETIC_COMMIT', { mode: 0o600 })
    await writeFile(path.join(directory, 'data', 'payload'), 'SYNTHETIC_ARCHIVE_' + number, { mode: 0o600 })
    const observation = { manifest_sha256: digest(raw), tree_sha256: id(100 + number), identity_sha256: id(200 + number), total_bytes: 100 * number }
    observations.set(id(number), observation)
    const intent = { id: id(number), kind: 'backup', instance_id: 'primary', registry_digest: engine.registryDigest(config.instances[0]),
      source_container_id: id(90), source_image_id: 'sha256:' + id(91) }
    await engine.store.save({ id: id(number), kind: 'backup', instance_id: 'primary', state: 'succeeded', created_at: manifest.created_unix_seconds, intent, digest: digest(intent),
      source_stopped_digest: id(92), backup_created: true, bundle: observation })
  }
  async function restoreReference(state, backupId = id(1)) {
    const restoreId = id(40)
    const intent = { id: restoreId, kind: 'restore', instance_id: 'primary', registry_digest: engine.registryDigest(config.instances[0]), backup_id: backupId }
    await engine.store.save({ id: restoreId, kind: 'restore', instance_id: 'primary', state, intent, digest: digest(intent) })
  }
  async function unchangedSource() {
    const after = await lstat(source, { bigint: true })
    for (const key of ['dev', 'ino', 'uid', 'gid', 'mode', 'size', 'mtimeNs', 'ctimeNs']) assert.equal(after[key], sourceBefore[key], key)
    assert.equal(await readFile(source, 'utf8'), 'SYNTHETIC_SOURCE_NEVER_REWRITTEN')
  }
  return { root, config, source, engine, helperCalls, writerCalls, dockerCalls, badVerify, writerModes, restoreReference, unchangedSource,
    setContainers(value) { containers = value }, setDeletionMode(value) { deletionMode = value }, onPrune(value) { onPrune = value } }
}

async function lockAbsent(f) {
  await assert.rejects(lstat(path.join(f.config.state_dir, 'instance-primary.lock')), { code: 'ENOENT' })
}

test('catalog retains old directories, corrupt backups and every restore-reference state', async t => {
  for (const state of ['prepared', 'queued', 'failed_before_start', 'recovery_required', 'interrupted', 'succeeded', 'legacy-unknown-state']) {
    await t.test(state, async t => {
      const f = await fixture(t)
      await f.restoreReference(state)
      f.badVerify.add(id(2))
      await mkdir(path.join(f.config.backups_dir, 'legacy-without-journal'), { mode: 0o700 })
      const catalog = await f.engine.retentionCatalog('primary')
      assert.deepEqual(catalog.archives.map(value => value.id).sort(), [id(1), id(3)])
      assert.deepEqual(catalog.protectedIds, [id(1)])
      assert.equal(catalog.references[0].state, state)
      assert.equal(catalog.references[0].backup_id, id(1))
      assert.deepEqual(catalog.retainedUnknown.map(value => value.reason).sort(), ['BACKUP_VERIFICATION_FAILED', 'unrecognized-or-uncommitted'].sort())
      const plan = await f.engine.preparePrune('primary', id(3), { keepCount: 1 })
      assert.equal(plan.state, 'no_changes')
      assert.deepEqual(plan.selection.candidateIds, [])
      assert.equal(f.helperCalls.some(value => value.action === 'bundle_prune'), false)
      assert.ok((await readdir(f.config.backups_dir)).includes('legacy-without-journal'))
      await f.unchangedSource()
    })
  }
})

test('unknown, blocked and failed process observations protect an archive without invented zero usage', async t => {
  for (const mode of ['unknown', 'blocked', 'throw']) {
    await t.test(mode, async t => {
      const f = await fixture(t)
      f.writerModes.set(id(1), mode)
      f.writerModes.set(id(2), mode)
      const catalog = await f.engine.retentionCatalog('primary')
      assert.deepEqual(catalog.protectedIds.sort(), [id(1), id(2)])
      assert.equal(catalog.archives.length, 3)
      const result = await f.engine.preparePrune('primary', id(3), { keepCount: 1 })
      assert.equal(result.state, 'no_changes')
      assert.deepEqual(result.selection.candidateIds, [])
      assert.equal(Object.hasOwn(result, 'id'), false)
      assert.equal(f.helperCalls.some(value => value.action === 'bundle_prune'), false)
      await f.unchangedSource()
    })
  }
})

test('readonly stopped-container mounts protect archives in the real catalog', async t => {
  const f = await fixture(t)
  f.setContainers([{ Id: id(70), State: { Running: false, Pid: 0, Status: 'exited' },
    Mounts: [{ Type: 'bind', Source: path.join(f.config.backups_dir, id(1)), RW: false }] }])
  const catalog = await f.engine.retentionCatalog('primary')
  assert.deepEqual(catalog.protectedIds, [id(1)])
  const plan = await f.engine.preparePrune('primary', id(3), { keepCount: 1 })
  assert.deepEqual(plan.selection.candidateIds, [id(2)])
  await f.unchangedSource()
})

test('successful orchestration sends only confirmed IDs and consumes the trigger once without source effects', async t => {
  const f = await fixture(t)
  const legacy = { id: id(80), instance_id: 'primary', state: 'succeeded', source_container_id: id(81), target_container_id: id(82) }
  await f.engine.store.save(legacy)
  const plan = await f.engine.preparePrune('primary', id(3), { keepCount: 1 })
  assert.deepEqual(plan.selection.candidateIds, [id(2), id(1)])
  const result = await f.engine.executePrune(plan.id, plan.digest, 'primary', id(3), { keepCount: 1 })
  assert.equal(result.state, 'succeeded')
  assert.equal(result.code, null)
  assert.equal(result.source_changed, false)
  assert.equal(result.automatic_retry, false)
  assert.deepEqual(result.results, [{ id: id(2), state: 'deleted', logical_bytes: 200 }, { id: id(1), state: 'deleted', logical_bytes: 100 }])
  const calls = f.helperCalls.filter(value => value.action === 'bundle_prune')
  assert.deepEqual(calls.map(value => value.bundle_id), [id(2), id(1)])
  for (const call of calls) {
    const selected = plan.selection.candidates.find(value => value.id === call.bundle_id)
    assert.equal(call.expected_manifest_sha256, selected.manifestSha256)
    assert.equal(call.expected_root.dev, selected.rootDev)
    assert.equal(call.expected_root.ino, selected.rootIno)
    assert.equal(call.expected_root.uid, 0)
    assert.equal(call.expected_root.mode, '0700')
  }
  assert.equal(f.engine.store.plans.get(id(3)).prune_consumed_by, plan.id)
  await assert.rejects(f.engine.executePrune(plan.id, plan.digest, 'primary', id(3), { keepCount: 1 }), { code: 'PLAN_CONFIRMATION_INVALID' })
  await assert.rejects(f.engine.preparePrune('primary', id(3), { keepCount: 1 }), { code: 'RETENTION_TRIGGER_UNAVAILABLE' })
  assert.equal(f.helperCalls.filter(value => value.action === 'bundle_prune').length, 2)
  const reopened = new JournalStore(f.config.state_dir)
  await reopened.init()
  assert.equal(reopened.current.get('primary').id, legacy.id)
  assert.equal(reopened.frozen.has('primary'), false)
  assert.equal(reopened.plans.get(id(1)).state, 'pruned')
  assert.equal(reopened.plans.get(id(2)).state, 'pruned')
  assert.equal(reopened.plans.get(id(3)).state, 'succeeded')
  // The fixed helper response deliberately does not delete real fixture files.
  assert.ok((await readdir(f.config.backups_dir)).includes(id(1)))
  await lockAbsent(f)
  await f.unchangedSource()
})

test('a failed or unconfirmed delete is never retried and preserves lock, unconfirmed result and recovery freeze', async t => {
  for (const mode of ['throw', 'unknown']) {
    await t.test(mode, async t => {
      const f = await fixture(t)
      f.setDeletionMode(mode)
      const plan = await f.engine.preparePrune('primary', id(3), { keepCount: 1 })
      const result = await f.engine.executePrune(plan.id, plan.digest, 'primary', id(3), { keepCount: 1 })
      assert.equal(result.state, 'recovery_required')
      assert.equal(result.code, mode === 'throw' ? 'HELPER_FAILED' : 'RETENTION_DELETE_UNCONFIRMED')
      assert.deepEqual(result.results, [{ id: id(2), state: 'unconfirmed' }])
      assert.equal(result.automatic_retry, false)
      assert.equal(f.helperCalls.filter(value => value.action === 'bundle_prune').length, 1)
      assert.equal(f.engine.store.frozen.has('primary'), true)
      assert.equal(f.engine.store.plans.get(id(3)).prune_consumed_by, plan.id)
      assert.equal(f.engine.store.plans.get(id(2)).state, 'succeeded')
      assert.equal(f.engine.store.plans.get(id(2)).backup_created, true)
      assert.ok((await lstat(path.join(f.config.state_dir, 'instance-primary.lock'))).isFile())
      await assert.rejects(f.engine.executePrune(plan.id, plan.digest, 'primary', id(3), { keepCount: 1 }), { code: 'PLAN_CONFIRMATION_INVALID' })
      const reopened = new JournalStore(f.config.state_dir)
      await reopened.init()
      assert.equal(reopened.frozen.has('primary'), true)
      assert.equal(reopened.plans.get(plan.id).state, 'recovery_required')
      assert.equal(reopened.current.size, 0)
      assert.equal(f.helperCalls.filter(value => value.action === 'bundle_prune').length, 1)
      await f.unchangedSource()
    })
  }
})

test('late restore references stop the next candidate after one confirmed helper effect', async t => {
  const f = await fixture(t)
  f.onPrune(async input => {
    assert.equal(input.bundle_id, id(2))
    await f.restoreReference('failed_before_start', id(1))
  })
  const plan = await f.engine.preparePrune('primary', id(3), { keepCount: 1 })
  const result = await f.engine.executePrune(plan.id, plan.digest, 'primary', id(3), { keepCount: 1 })
  assert.equal(result.state, 'recovery_required')
  assert.equal(result.code, 'RETENTION_REFERENCE_CHANGED')
  assert.deepEqual(result.results, [{ id: id(2), state: 'deleted', logical_bytes: 200 }])
  assert.deepEqual(f.helperCalls.filter(value => value.action === 'bundle_prune').map(value => value.bundle_id), [id(2)])
  assert.equal(f.engine.store.frozen.has('primary'), true)
  assert.equal(f.engine.store.plans.get(id(1)).state, 'succeeded')
  await f.unchangedSource()
})

test('actual archive directory changes and new restore journals invalidate confirmation before any delete', async t => {
  for (const change of ['new-directory', 'changed-manifest', 'new-restore']) {
    await t.test(change, async t => {
      const f = await fixture(t)
      const plan = await f.engine.preparePrune('primary', id(3), { keepCount: 1 })
      if (change === 'new-directory') await mkdir(path.join(f.config.backups_dir, 'new-unknown-directory'), { mode: 0o700 })
      if (change === 'changed-manifest') await writeFile(path.join(f.config.backups_dir, id(1), 'manifest.json'), '{"changed":true}', { mode: 0o600 })
      if (change === 'new-restore') await f.restoreReference('failed_before_start')
      const result = await f.engine.executePrune(plan.id, plan.digest, 'primary', id(3), { keepCount: 1 })
      assert.equal(result.state, 'failed_before_start')
      assert.equal(result.code, 'RETENTION_CATALOG_CHANGED')
      assert.deepEqual(result.results, [])
      assert.equal(f.helperCalls.some(value => value.action === 'bundle_prune'), false)
      assert.equal(f.engine.store.plans.get(id(3)).prune_consumed_by, undefined)
      await lockAbsent(f)
      await f.unchangedSource()
    })
  }
})
