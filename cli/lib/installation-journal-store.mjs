import { randomBytes } from 'node:crypto'
import {
  copyFile,
  lstat,
  mkdir,
  readFile,
  rename,
  rm,
  writeFile,
} from 'node:fs/promises'
import path from 'node:path'

import {
  beginFreshInstallEffect,
  completeFreshInstallEffect,
  failFreshInstallEffect,
  markFreshInstallOutcomeUnknown,
  validateFreshInstallJournal,
} from './installation-state.mjs'

const JOURNAL_FILE = 'fresh-install-journal.json'
const LOCK_DIRECTORY = 'installation.lock'
const LOCK_FILE = 'owner.json'

export class InstallationJournalStoreError extends Error {
  constructor(code, message) {
    super(message)
    this.code = code
  }
}

function fail(code, message) {
  throw new InstallationJournalStoreError(code, message)
}

function assertManagedRoot(rootDir) {
  if (typeof rootDir !== 'string' || rootDir.length === 0) {
    fail('invalid_root', 'managed root must be an absolute path')
  }
  const resolved = path.resolve(rootDir)
  if (resolved !== rootDir || resolved === path.parse(resolved).root) {
    fail('invalid_root', 'managed root must be a non-root absolute path')
  }
  return resolved
}

async function ensureDirectory(directory, label) {
  await mkdir(directory, { recursive: true, mode: 0o700 })
  const info = await lstat(directory)
  if (!info.isDirectory() || info.isSymbolicLink()) {
    fail('unsafe_path', `${label} must be a real directory`)
  }
}

async function stateDirectory(rootDir) {
  const root = assertManagedRoot(rootDir)
  await ensureDirectory(root, 'managed root')
  const state = path.join(root, 'state')
  await ensureDirectory(state, 'managed state directory')
  return state
}

function canonicalJSON(value) {
  return `${JSON.stringify(value, null, 2)}\n`
}

function validateOperationID(operationID) {
  if (typeof operationID !== 'string' || !/^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/.test(operationID)) {
    fail('invalid_operation', 'operation id must be a portable identifier')
  }
}

function canonicalTimestamp(now) {
  const value = now instanceof Date ? now : new Date(now)
  if (Number.isNaN(value.getTime())) fail('invalid_time', 'timestamp is invalid')
  return value.toISOString()
}

async function assertInstallationLockOwner(state, operationID) {
  let raw
  try {
    raw = await readFile(path.join(state, LOCK_DIRECTORY, LOCK_FILE), 'utf8')
  } catch (error) {
    if (error && error.code === 'ENOENT') fail('lock_missing', 'installation lock is missing')
    throw error
  }
  let owner
  try {
    owner = JSON.parse(raw)
  } catch {
    fail('invalid_lock', 'installation lock owner is invalid')
  }
  if (owner?.operation_id !== operationID) {
    fail('lock_owner_mismatch', 'operation does not own the installation lock')
  }
}

async function sha256File(filePath) {
  const { createHash } = await import('node:crypto')
  const { createReadStream } = await import('node:fs')
  return new Promise((resolve, reject) => {
    const hash = createHash('sha256')
    const stream = createReadStream(filePath, { flags: 'r' })
    stream.on('error', reject)
    stream.on('data', (chunk) => hash.update(chunk))
    stream.on('end', () => resolve(hash.digest('hex')))
  })
}

function assertStageableJournal(journal, operationID) {
  const snapshot = validateFreshInstallJournal(journal)
  if (snapshot.operation_id !== operationID) {
    fail('operation_mismatch', 'journal operation does not match the lock owner')
  }
  if (snapshot.release_identity.verification_status !== 'verified') {
    fail('release_unverified', 'only a verified release identity may be staged')
  }
  if (snapshot.state !== 'artifact_verified' || snapshot.pending_effect?.to_state !== 'staged') {
    fail('invalid_state', 'artifact staging requires a declared artifact_verified to staged effect')
  }
  if (snapshot.release_identity.artifact.kind === 'oci') {
    fail('unsupported_artifact', 'local artifact staging supports downloadable artifacts only')
  }
  return snapshot
}

export async function readFreshInstallJournal(rootDir) {
  const state = await stateDirectory(rootDir)
  const journalPath = path.join(state, JOURNAL_FILE)
  let content
  try {
    content = await readFile(journalPath, 'utf8')
  } catch (error) {
    if (error && error.code === 'ENOENT') return null
    throw error
  }
  let parsed
  try {
    parsed = JSON.parse(content)
  } catch {
    fail('invalid_journal', 'stored installation journal is not valid JSON')
  }
  try {
    return validateFreshInstallJournal(parsed)
  } catch {
    fail('invalid_journal', 'stored installation journal violates the state contract')
  }
}

// writeFreshInstallJournal persists a fully validated detached journal before
// any future install effect. It never creates or changes files outside state/.
export async function writeFreshInstallJournal(rootDir, journal) {
  const snapshot = validateFreshInstallJournal(journal)
  const state = await stateDirectory(rootDir)
  const target = path.join(state, JOURNAL_FILE)
  const temporary = path.join(state, `.${JOURNAL_FILE}.${randomBytes(12).toString('hex')}.tmp`)
  try {
    await writeFile(temporary, canonicalJSON(snapshot), { encoding: 'utf8', mode: 0o600, flag: 'wx' })
    await rename(temporary, target)
  } catch (error) {
    await rm(temporary, { force: true }).catch(() => {})
    throw error
  }
  return snapshot
}

// acquireInstallationLock serializes journal owners. It intentionally never
// steals an existing lock: stale/interrupted work must be inspected through
// the journal and resolved by an explicit recovery path.
export async function acquireInstallationLock(rootDir, operationID, now = new Date()) {
  validateOperationID(operationID)
  const state = await stateDirectory(rootDir)
  const lockDirectory = path.join(state, LOCK_DIRECTORY)
  try {
    await mkdir(lockDirectory, { mode: 0o700 })
  } catch (error) {
    if (error && error.code === 'EEXIST') {
      fail('lock_exists', 'another installation operation owns the managed root')
    }
    throw error
  }
  try {
    const info = await lstat(lockDirectory)
    if (!info.isDirectory() || info.isSymbolicLink()) {
      fail('unsafe_path', 'installation lock must be a real directory')
    }
    const owner = Object.freeze({ operation_id: operationID, acquired_at: canonicalTimestamp(now) })
    await writeFile(path.join(lockDirectory, LOCK_FILE), canonicalJSON(owner), {
      encoding: 'utf8', mode: 0o600, flag: 'wx',
    })
    return owner
  } catch (error) {
    await rm(lockDirectory, { recursive: true, force: true }).catch(() => {})
    throw error
  }
}

export async function releaseInstallationLock(rootDir, operationID) {
  validateOperationID(operationID)
  const state = await stateDirectory(rootDir)
  const lockDirectory = path.join(state, LOCK_DIRECTORY)
  let raw
  try {
    raw = await readFile(path.join(lockDirectory, LOCK_FILE), 'utf8')
  } catch (error) {
    if (error && error.code === 'ENOENT') fail('lock_missing', 'installation lock is missing')
    throw error
  }
  let owner
  try {
    owner = JSON.parse(raw)
  } catch {
    fail('invalid_lock', 'installation lock owner is invalid')
  }
  if (owner?.operation_id !== operationID) {
    fail('lock_owner_mismatch', 'operation does not own the installation lock')
  }
  const info = await lstat(lockDirectory)
  if (!info.isDirectory() || info.isSymbolicLink()) fail('unsafe_path', 'installation lock must be a real directory')
  await rm(lockDirectory, { recursive: true, force: false })
}

// beginPersistedFreshInstallEffect is the persist-before-effect boundary for
// future installation executors. The next effect is derived from the journal
// currently stored under the lock, never from a caller-held snapshot. Nothing
// beyond state/ changes here; callers must run their actual effect only after
// this function returns its pending record.
export async function beginPersistedFreshInstallEffect(rootDir, operationID, effectID, nextState, startedAt = new Date()) {
  validateOperationID(operationID)
  const state = await stateDirectory(rootDir)
  await assertInstallationLockOwner(state, operationID)
  const persisted = await readFreshInstallJournal(rootDir)
  if (persisted === null) {
    fail('journal_missing', 'a persisted fresh-install journal is required before an installation effect')
  }
  if (persisted.operation_id !== operationID) {
    fail('operation_mismatch', 'journal operation does not match the lock owner')
  }
  const pending = beginFreshInstallEffect(persisted, {
    effect_id: effectID,
    next_state: nextState,
    started_at: canonicalTimestamp(startedAt),
  })
  await writeFreshInstallJournal(rootDir, pending)
  return pending
}

// completePersistedFreshInstallEffect is the matching persist-after-effect
// boundary. It re-reads the owned journal while the lock is held so a stale
// caller cannot complete a different or already-recovered effect.
export async function completePersistedFreshInstallEffect(rootDir, operationID, effectID, completedAt = new Date()) {
  validateOperationID(operationID)
  const state = await stateDirectory(rootDir)
  await assertInstallationLockOwner(state, operationID)
  const persisted = await readFreshInstallJournal(rootDir)
  if (persisted === null) {
    fail('journal_missing', 'a persisted fresh-install journal is required before completing an installation effect')
  }
  if (persisted.operation_id !== operationID) {
    fail('operation_mismatch', 'journal operation does not match the lock owner')
  }
  const completed = completeFreshInstallEffect(persisted, {
    effect_id: effectID,
    completed_at: canonicalTimestamp(completedAt),
  })
  await writeFreshInstallJournal(rootDir, completed)
  return completed
}

async function recordPersistedTerminalEffect(rootDir, operationID, effectID, recordedAt, terminalTransition) {
  validateOperationID(operationID)
  const state = await stateDirectory(rootDir)
  await assertInstallationLockOwner(state, operationID)
  const persisted = await readFreshInstallJournal(rootDir)
  if (persisted === null) {
    fail('journal_missing', 'a persisted fresh-install journal is required before recording an installation outcome')
  }
  if (persisted.operation_id !== operationID) {
    fail('operation_mismatch', 'journal operation does not match the lock owner')
  }
  const terminal = terminalTransition(persisted, {
    effect_id: effectID,
    recorded_at: canonicalTimestamp(recordedAt),
    reason_code: terminalTransition === failFreshInstallEffect ? 'effect_failed' : 'effect_outcome_unknown',
  })
  await writeFreshInstallJournal(rootDir, terminal)
  return terminal
}

// failPersistedFreshInstallEffect records a known failure under the lock. It
// never retries or erases the pending effect's provenance.
export async function failPersistedFreshInstallEffect(rootDir, operationID, effectID, recordedAt = new Date()) {
  return recordPersistedTerminalEffect(rootDir, operationID, effectID, recordedAt, failFreshInstallEffect)
}

// markPersistedFreshInstallOutcomeUnknown records an indeterminate external
// outcome. Recovery must be an explicit later operation, never an automatic
// replay that could duplicate an external side effect.
export async function markPersistedFreshInstallOutcomeUnknown(rootDir, operationID, effectID, recordedAt = new Date()) {
  return recordPersistedTerminalEffect(rootDir, operationID, effectID, recordedAt, markFreshInstallOutcomeUnknown)
}

// stageVerifiedLocalArtifact copies one caller-provided local file into the
// operation's owned staging directory. It requires both the persisted lock and
// a pending staging effect, and hashes the copied result after the copy so a
// source-file race cannot make an unverified payload appear staged.
export async function stageVerifiedLocalArtifact(rootDir, journal, operationID, sourcePath) {
  validateOperationID(operationID)
  if (typeof sourcePath !== 'string' || path.resolve(sourcePath) !== sourcePath) {
    fail('invalid_source', 'artifact source must be an absolute path')
  }
  const snapshot = assertStageableJournal(journal, operationID)
  const state = await stateDirectory(rootDir)
  await assertInstallationLockOwner(state, operationID)
  const sourceInfo = await lstat(sourcePath)
  if (!sourceInfo.isFile() || sourceInfo.isSymbolicLink()) {
    fail('invalid_source', 'artifact source must be a regular non-symbolic file')
  }
  const managedRoot = path.dirname(state)
  const stagingRoot = path.join(managedRoot, 'staging')
  const staging = path.join(stagingRoot, operationID)
  await ensureDirectory(stagingRoot, 'managed staging directory')
  await ensureDirectory(staging, 'operation staging directory')
  const artifactPath = path.join(staging, snapshot.release_identity.artifact.id)
  const temporary = `${artifactPath}.${randomBytes(12).toString('hex')}.tmp`
  try {
    await copyFile(sourcePath, temporary, 1)
    const digest = await sha256File(temporary)
    if (digest !== snapshot.release_identity.artifact.sha256_or_digest) {
      fail('artifact_hash_mismatch', 'staged artifact hash does not match the verified identity')
    }
    await rename(temporary, artifactPath)
  } catch (error) {
    await rm(temporary, { force: true }).catch(() => {})
    throw error
  }
  return Object.freeze({
    path: path.relative(managedRoot, artifactPath).split(path.sep).join('/'),
    sha256: snapshot.release_identity.artifact.sha256_or_digest,
    artifact_id: snapshot.release_identity.artifact.id,
  })
}

// stagePersistedVerifiedLocalArtifact is the durable staging boundary used by
// a future installer. It takes the journal from disk while the operation lock
// is held, stages only the exact verified artifact declared by its pending
// effect, then persists the completed transition. A crash after the copy but
// before the final journal write leaves the declared effect pending, which is
// deliberately recoverable and never reported as staged.
//
// This function still does not obtain a manifest, validate a signature, start
// a service, switch a release, or remove any live installation files.
export async function stagePersistedVerifiedLocalArtifact(rootDir, operationID, sourcePath, completedAt = new Date()) {
  validateOperationID(operationID)
  const state = await stateDirectory(rootDir)
  await assertInstallationLockOwner(state, operationID)
  const persisted = await readFreshInstallJournal(rootDir)
  if (persisted === null) {
    fail('journal_missing', 'a persisted fresh-install journal is required before artifact staging')
  }
  const snapshot = assertStageableJournal(persisted, operationID)
  const staged = await stageVerifiedLocalArtifact(rootDir, snapshot, operationID, sourcePath)
  const completed = await completePersistedFreshInstallEffect(
    rootDir,
    operationID,
    snapshot.pending_effect.effect_id,
    completedAt,
  )
  return Object.freeze({ journal: completed, artifact: staged })
}
