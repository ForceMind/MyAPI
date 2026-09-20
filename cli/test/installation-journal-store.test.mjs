import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { lstat, mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import test from 'node:test'

import {
  acquireInstallationLock,
  beginPersistedFreshInstallEffect,
  completePersistedFreshInstallEffect,
  failPersistedFreshInstallEffect,
  InstallationJournalStoreError,
  readFreshInstallJournal,
  releaseInstallationLock,
  markPersistedFreshInstallOutcomeUnknown,
  stageVerifiedLocalArtifact,
  stagePersistedVerifiedLocalArtifact,
  writeFreshInstallJournal,
} from '../lib/installation-journal-store.mjs'
import {
  beginFreshInstallEffect,
  completeFreshInstallEffect,
  createFreshInstallJournal,
} from '../lib/installation-state.mjs'

const hash = createHash('sha256').update('artifact').digest('hex')
const sourceSHA = 'a'.repeat(40)

function releaseIdentity() {
  return {
    manifest_schema_version: 1,
    product: { name: 'My API', version: '0.2.0-beta.1', source_sha: sourceSHA, build_revision: sourceSHA },
    compatibility: { config_schema: '1.0.0', data_schema: '1.0.0' },
    artifact: {
      id: 'myapi-lite-linux-amd64', edition: 'lite', installation_shape: 'personal-native', os: 'linux', arch: 'amd64', kind: 'binary',
      location: 'https://releases.example.test/myapi-lite-linux-amd64', sha256_or_digest: hash,
    },
    verification_status: 'verified',
  }
}

function releaseIdentityForBytes(bytes) {
  const identity = releaseIdentity()
  identity.artifact.sha256_or_digest = createHash('sha256').update(bytes).digest('hex')
  return identity
}

function journal() {
  return createFreshInstallJournal({
    operation_id: 'install-1', created_at: '2026-09-20T00:00:00.000Z', release_identity: releaseIdentity(),
  })
}

async function managedRoot(t) {
  const root = await mkdtemp(path.join(os.tmpdir(), 'myapi-journal-test-'))
  t.after(async () => rm(root, { recursive: true, force: true }))
  return root
}

test('persists and reloads a validated fresh-install journal inside state', async (t) => {
  const root = await managedRoot(t)
  const written = await writeFreshInstallJournal(root, journal())
  const loaded = await readFreshInstallJournal(root)
  assert.deepEqual(loaded, written)
})

test('serializes installation ownership and rejects a non-owner release', async (t) => {
  const root = await managedRoot(t)
  const owner = await acquireInstallationLock(root, 'install-1', new Date('2026-09-20T00:00:00.000Z'))
  assert.equal(owner.operation_id, 'install-1')
  await assert.rejects(
    acquireInstallationLock(root, 'install-2'),
    (error) => error instanceof InstallationJournalStoreError && error.code === 'lock_exists',
  )
  await assert.rejects(
    releaseInstallationLock(root, 'install-2'),
    (error) => error instanceof InstallationJournalStoreError && error.code === 'lock_owner_mismatch',
  )
  await releaseInstallationLock(root, 'install-1')
  const next = await acquireInstallationLock(root, 'install-2')
  assert.equal(next.operation_id, 'install-2')
})

test('persists a declared effect before an installer can perform it', async (t) => {
  const root = await managedRoot(t)
  await writeFreshInstallJournal(root, journal())
  await acquireInstallationLock(root, 'install-1')

  const pending = await beginPersistedFreshInstallEffect(
    root,
    'install-1',
    'preflight',
    'preflighted',
    new Date('2026-09-20T00:00:01.000Z'),
  )

  assert.equal(pending.state, 'planned')
  assert.equal(pending.pending_effect.effect_id, 'preflight')
  assert.deepEqual(await readFreshInstallJournal(root), pending)
})

test('completes only the persisted pending effect held by the installation owner', async (t) => {
  const root = await managedRoot(t)
  await writeFreshInstallJournal(root, journal())
  await acquireInstallationLock(root, 'install-1')
  await beginPersistedFreshInstallEffect(
    root,
    'install-1',
    'preflight',
    'preflighted',
    new Date('2026-09-20T00:00:01.000Z'),
  )

  const completed = await completePersistedFreshInstallEffect(
    root,
    'install-1',
    'preflight',
    new Date('2026-09-20T00:00:02.000Z'),
  )

  assert.equal(completed.state, 'preflighted')
  assert.equal(completed.pending_effect, null)
  assert.equal(completed.completed_effects.at(-1).effect_id, 'preflight')
  assert.deepEqual(await readFreshInstallJournal(root), completed)
})

test('persists a known preflight failure without retrying the declared effect', async (t) => {
  const root = await managedRoot(t)
  await writeFreshInstallJournal(root, journal())
  await acquireInstallationLock(root, 'install-1')
  await beginPersistedFreshInstallEffect(
    root,
    'install-1',
    'preflight',
    'preflighted',
    new Date('2026-09-20T00:00:01.000Z'),
  )

  const failed = await failPersistedFreshInstallEffect(
    root,
    'install-1',
    'preflight',
    new Date('2026-09-20T00:00:02.000Z'),
  )

  assert.equal(failed.state, 'failed')
  assert.equal(failed.terminal.reason_code, 'effect_failed')
  assert.deepEqual(await readFreshInstallJournal(root), failed)
})

test('persists an unknown outcome instead of retrying the declared effect', async (t) => {
  const root = await managedRoot(t)
  await writeFreshInstallJournal(root, journal())
  await acquireInstallationLock(root, 'install-1')
  await beginPersistedFreshInstallEffect(
    root,
    'install-1',
    'preflight',
    'preflighted',
    new Date('2026-09-20T00:00:01.000Z'),
  )

  const unknown = await markPersistedFreshInstallOutcomeUnknown(
    root,
    'install-1',
    'preflight',
    new Date('2026-09-20T00:00:02.000Z'),
  )

  assert.equal(unknown.state, 'outcome_unknown')
  assert.equal(unknown.terminal.reason_code, 'effect_outcome_unknown')
  assert.deepEqual(await readFreshInstallJournal(root), unknown)
})

test('fails closed on a malformed persisted journal without reading outside state', async (t) => {
  const root = await managedRoot(t)
  const state = path.join(root, 'state')
  await writeFile(path.join(root, 'outside.json'), 'outside', 'utf8')
  await mkdir(state, { recursive: true })
  await writeFile(path.join(state, 'fresh-install-journal.json'), '{not-json', 'utf8')
  await assert.rejects(
    readFreshInstallJournal(root),
    (error) => error instanceof InstallationJournalStoreError && error.code === 'invalid_journal',
  )
})

function stagedJournal(identity) {
  let value = createFreshInstallJournal({
    operation_id: 'install-1', created_at: '2026-09-20T00:00:00.000Z', release_identity: identity,
  })
  for (const [effectID, nextState, startedAt, completedAt] of [
    ['preflight', 'preflighted', '2026-09-20T00:00:01.000Z', '2026-09-20T00:00:02.000Z'],
    ['verify', 'artifact_verified', '2026-09-20T00:00:03.000Z', '2026-09-20T00:00:04.000Z'],
  ]) {
    value = beginFreshInstallEffect(value, { effect_id: effectID, next_state: nextState, started_at: startedAt })
    value = completeFreshInstallEffect(value, { effect_id: effectID, completed_at: completedAt })
  }
  return beginFreshInstallEffect(value, {
    effect_id: 'stage', next_state: 'staged', started_at: '2026-09-20T00:00:05.000Z',
  })
}

test('stages only a hash-matching local artifact while the journal lock owns a declared stage effect', async (t) => {
  const root = await managedRoot(t)
  const source = path.join(root, 'source.bin')
  const bytes = Buffer.from('verified artifact bytes')
  await writeFile(source, bytes)
  const identity = releaseIdentityForBytes(bytes)
  await acquireInstallationLock(root, 'install-1')

  const staged = await stageVerifiedLocalArtifact(root, stagedJournal(identity), 'install-1', source)

  assert.equal(staged.sha256, identity.artifact.sha256_or_digest)
  assert.equal(staged.path, 'staging/install-1/myapi-lite-linux-amd64')
})

test('rejects a hash mismatch and leaves no staged artifact', async (t) => {
  const root = await managedRoot(t)
  const source = path.join(root, 'source.bin')
  await writeFile(source, 'untrusted bytes')
  const identity = releaseIdentity()
  await acquireInstallationLock(root, 'install-1')

  await assert.rejects(
    stageVerifiedLocalArtifact(root, stagedJournal(identity), 'install-1', source),
    (error) => error instanceof InstallationJournalStoreError && error.code === 'artifact_hash_mismatch',
  )
  await assert.rejects(
    lstat(path.join(root, 'staging', 'install-1', identity.artifact.id)),
    (error) => error && error.code === 'ENOENT',
  )
})

test('durably records a completed staging effect only after the verified artifact is copied', async (t) => {
  const root = await managedRoot(t)
  const source = path.join(root, 'source.bin')
  const bytes = Buffer.from('durable verified artifact')
  const identity = releaseIdentityForBytes(bytes)
  await writeFile(source, bytes)
  const pending = stagedJournal(identity)
  await writeFreshInstallJournal(root, pending)
  await acquireInstallationLock(root, 'install-1')

  const result = await stagePersistedVerifiedLocalArtifact(
    root,
    'install-1',
    source,
    new Date('2026-09-20T00:00:06.000Z'),
  )

  assert.equal(result.artifact.path, 'staging/install-1/myapi-lite-linux-amd64')
  assert.equal(result.journal.state, 'staged')
  assert.equal(result.journal.pending_effect, null)
  assert.equal(result.journal.completed_effects.at(-1).effect_id, 'stage')
  assert.deepEqual(await readFreshInstallJournal(root), result.journal)
})
