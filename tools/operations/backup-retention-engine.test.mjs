import assert from 'node:assert/strict'
import { lstat, link, mkdir, mkdtemp, readFile, rm, symlink, writeFile } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import test from 'node:test'
import { BackupEngine } from './backup-engine.mjs'
import { JournalStore } from './store.mjs'
import { digest } from './safety.mjs'

const id = number => number.toString(16).padStart(64, '0')
const now = 2_000_000_000

// This suite injects an explicitly synthetic, already-observed catalog. It
// tests orchestration, not root ownership, process visibility or deletion.
async function fixture(t) {
  const root = await mkdtemp(path.join(os.tmpdir(), 'myapi-retention-engine-'))
  t.after(() => rm(root, { recursive: true, force: true }))
  for (const name of ['state', 'backups', 'data']) await mkdir(path.join(root, name), { mode: 0o700 })
  const source = path.join(root, 'data', 'source-marker')
  await writeFile(source, 'SYNTHETIC_UNCHANGED_SOURCE', { mode: 0o600 })
  const config = { docker_socket: '/var/run/docker.sock', state_dir: path.join(root, 'state'), backups_dir: path.join(root, 'backups'),
    instances: [{ id: 'primary', container: 'synthetic-source', data_dir: path.join(root, 'data'), sqlite_file: path.join(root, 'data', 'my-api.db') }] }
  const effects = []
  let time = now
  const deny = async () => { assert.fail('retention orchestration must not invoke source Docker or helper effects') }
  const engine = await new BackupEngine(config, { docker: { inspectContainer: deny, listContainers: deny, stopForBackup: deny, start: deny, disableRestart: deny },
    bundle: deny, writer: deny, capacity: async () => {}, clock: () => time }).init()
  const archives = [1, 2, 3].map(number => ({ id: id(number), instanceId: 'primary', manifestSha256: id(100 + number), treeSha256: id(200 + number),
    identitySha256: id(300 + number), rootDev: '2049', rootIno: String(9000 + number), createdUnixSeconds: now - (3 - number) * 86400, logicalBytes: number * 100 }))
  for (const archive of archives) {
    const intent = { id: archive.id, kind: 'backup', instance_id: 'primary', registry_digest: engine.registryDigest(config.instances[0]) }
    await engine.store.save({ id: archive.id, kind: 'backup', instance_id: 'primary', state: 'succeeded', created_at: archive.createdUnixSeconds, intent, digest: digest(intent), backup_created: true })
  }
  const catalog = { archives, protectedIds: [], retainedUnknown: [], references: [], registry_digest: engine.registryDigest(config.instances[0]), backup_journals: [] }
  engine.retentionCatalog = async () => { effects.push('synthetic-catalog-read'); return structuredClone(catalog) }
  return { root, source, config, engine, catalog, effects, setTime(value) { time = value } }
}

async function noLock(fixture) {
  await assert.rejects(lstat(path.join(fixture.config.state_dir, 'instance-primary.lock')), { code: 'ENOENT' })
  assert.equal(await readFile(fixture.source, 'utf8'), 'SYNTHETIC_UNCHANGED_SOURCE')
}

test('keep-all and a fully retained catalog create no executable deletion plan', async t => {
  const f = await fixture(t)
  for (const policy of [{}, { keepCount: 3 }]) {
    const before = f.engine.store.plans.size
    const result = await f.engine.preparePrune('primary', id(3), policy)
    assert.equal(result.state, 'no_changes')
    assert.equal(result.deletion_authorized, false)
    assert.equal(result.selection.deletionAuthorized, false)
    assert.deepEqual(result.selection.candidateIds, [])
    assert.equal(Object.hasOwn(result, 'id'), false)
    assert.equal(f.engine.store.plans.size, before)
  }
  await noLock(f)
})

test('a prepared plan binds the exact policy, trigger and catalog without authorizing deletion', async t => {
  const f = await fixture(t)
  f.catalog.protectedIds = [id(1)]
  f.catalog.retainedUnknown = [{ name_sha256: id(90), reason: 'unrecognized-or-uncommitted' }]
  const plan = await f.engine.preparePrune('primary', id(3), { keepCount: 1 })
  assert.equal(plan.kind, 'backup-prune')
  assert.equal(plan.state, 'prepared')
  assert.deepEqual(plan.selection.candidateIds, [id(2)])
  assert.deepEqual(plan.selection.protectedIds, [id(1)])
  assert.deepEqual(plan.selection.policy, { mode: 'union', keepCount: 1, keepDays: null })
  assert.deepEqual(plan.retained_unknown, f.catalog.retainedUnknown)
  assert.equal(plan.deletion_authorized, false)
  const stored = f.engine.store.plans.get(plan.id)
  assert.equal(stored.intent.trigger_id, id(3))
  assert.equal(stored.intent.catalog_digest, digest(f.catalog))
  assert.equal(plan.digest, digest(stored.intent))
  assert.equal(f.engine.store.plans.get(id(3)).prune_consumed_by, undefined)
  assert.deepEqual(f.effects, ['synthetic-catalog-read'])
  await noLock(f)
})

test('wrong digest, policy, trigger, instance, expiry or registry cannot confirm a prune', async t => {
  for (const mismatch of ['digest', 'count', 'days', 'trigger', 'instance', 'expiry', 'registry']) {
    await t.test(mismatch, async t => {
      const f = await fixture(t)
      const plan = await f.engine.preparePrune('primary', id(3), { keepCount: 1 })
      let suppliedDigest = plan.digest, instance = 'primary', trigger = id(3), policy = { keepCount: 1 }
      if (mismatch === 'digest') suppliedDigest = id(999)
      if (mismatch === 'count') policy = { keepCount: 2 }
      if (mismatch === 'days') policy = { keepCount: 1, keepDays: 1 }
      if (mismatch === 'trigger') trigger = id(2)
      if (mismatch === 'instance') instance = 'secondary'
      if (mismatch === 'expiry') f.setTime(plan.expires_at)
      if (mismatch === 'registry') f.config.instances[0].container = 'changed-source'
      await assert.rejects(f.engine.executePrune(plan.id, suppliedDigest, instance, trigger, policy),
        { code: mismatch === 'registry' ? 'PLAN_INTEGRITY_FAILED' : 'PLAN_CONFIRMATION_INVALID' })
      assert.equal(f.engine.store.plans.get(plan.id).state, 'prepared')
      assert.equal(f.effects.length, 1)
      await noLock(f)
    })
  }
})

test('new backups, restore references and unknown directory changes invalidate the observed catalog', async t => {
  for (const change of ['backup', 'restore-prepared', 'restore-failed_before_start', 'restore-succeeded', 'directory']) {
    await t.test(change, async t => {
      const f = await fixture(t)
      const plan = await f.engine.preparePrune('primary', id(3), { keepCount: 1 })
      if (change === 'backup') f.catalog.archives.push({ ...f.catalog.archives[2], id: id(4), rootIno: '9004', createdUnixSeconds: now })
      else if (change === 'directory') f.catalog.retainedUnknown.push({ name_sha256: id(91), reason: 'unrecognized-or-uncommitted' })
      else f.catalog.references.push({ id: id(40), backup_id: id(1), state: change.slice(8), digest: id(41) })
      const result = await f.engine.executePrune(plan.id, plan.digest, 'primary', id(3), { keepCount: 1 })
      assert.equal(result.state, 'failed_before_start')
      assert.equal(result.code, 'RETENTION_CATALOG_CHANGED')
      assert.deepEqual(result.results, [])
      assert.equal(result.source_changed, false)
      assert.equal(result.automatic_retry, false)
      assert.equal(f.engine.store.plans.get(id(3)).prune_consumed_by, undefined)
      await noLock(f)
    })
  }
})

test('archive mount inspection protects readonly and stopped containers, inode aliases and ancestor aliases', async t => {
  const f = await fixture(t)
  const archive = path.join(f.config.backups_dir, id(1))
  await mkdir(path.join(archive, 'data'), { recursive: true, mode: 0o700 })
  for (const filename of ['manifest.json', 'COMMITTED', 'data/payload']) await writeFile(path.join(archive, filename), 'synthetic', { mode: 0o600 })
  const fileAlias = path.join(f.root, 'payload-alias')
  await link(path.join(archive, 'data', 'payload'), fileAlias)
  const ancestorAlias = path.join(f.root, 'backup-parent-alias')
  await symlink(f.config.backups_dir, ancestorAlias)
  const unrelated = path.join(f.root, 'unrelated')
  await mkdir(unrelated, { mode: 0o700 })
  const manifest = { entries: [{ path: 'payload' }] }
  for (const [source, rw, running] of [[archive, false, false], [fileAlias, false, true], [ancestorAlias, true, false]]) {
    assert.equal(await f.engine.archiveMounted(archive, manifest, [{ Id: id(70), State: { Running: running }, Mounts: [{ Type: 'bind', Source: source, RW: rw }] }]), true)
  }
  assert.equal(await f.engine.archiveMounted(archive, manifest, [{ Id: id(70), Mounts: [{ Type: 'bind', Source: unrelated, RW: false }] }]), false)
  await assert.rejects(f.engine.archiveMounted(archive, manifest, [{ Id: id(70), Mounts: [{ Type: 'unknown', Source: archive }] }]), { code: 'RETENTION_MOUNT_STATE_UNKNOWN' })
  await noLock(f)
})

test('a persisted pruning journal freezes the instance on restart without entering the upgrade chain', async t => {
  const f = await fixture(t)
  const legacy = { id: id(80), instance_id: 'primary', state: 'succeeded', source_container_id: id(81), target_container_id: id(82) }
  await f.engine.store.save(legacy)
  const plan = await f.engine.preparePrune('primary', id(3), { keepCount: 1 })
  const stored = f.engine.store.plans.get(plan.id)
  stored.state = 'pruning'
  await f.engine.store.save(stored)
  await f.engine.store.lock('primary')
  const reopened = new JournalStore(f.config.state_dir)
  await reopened.init()
  assert.equal(reopened.plans.get(plan.id).state, 'interrupted')
  assert.equal(reopened.plans.get(plan.id).code, 'EXECUTION_INTERRUPTED')
  assert.equal(reopened.frozen.has('primary'), true)
  assert.equal(reopened.current.get('primary').id, legacy.id)
  assert.equal(reopened.current.get('primary').kind, undefined)
  assert.equal(await readFile(f.source, 'utf8'), 'SYNTHETIC_UNCHANGED_SOURCE')
})

test('an older or unknown-time journal cannot serve as the fresh backup trigger', async t => {
  for (const changed of ['older', 'unknown-trigger-time', 'unknown-other-time']) {
    const f = await fixture(t)
    let trigger = id(3)
    if (changed === 'older') trigger = id(2)
    if (changed === 'unknown-trigger-time') delete f.engine.store.plans.get(id(3)).created_at
    if (changed === 'unknown-other-time') delete f.engine.store.plans.get(id(1)).created_at
    await assert.rejects(f.engine.preparePrune('primary', trigger, { keepCount: 1 }), { code: 'RETENTION_TRIGGER_NOT_LATEST' })
    assert.deepEqual(f.effects, [])
    await noLock(f)
  }
})
