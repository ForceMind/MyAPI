/*
 * MyAPI distribution and self-hosting tooling.
 * Copyright (C) 2026 ForceMind
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published
 * by the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 */
import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  beginFreshInstallEffect,
  buildCommittedInstallationRecord,
  completeFreshInstallEffect,
  createFreshInstallJournal,
  failFreshInstallEffect,
  FRESH_INSTALL_PROGRESS_STATES,
  InstallationCleanupPlanError,
  InstallationStateTransitionError,
  InstallationStateValidationError,
  markFreshInstallOutcomeUnknown,
  planOwnedFileCleanup,
  PRESERVED_INSTALLATION_ROOTS,
  validateCommittedInstallationRecord,
  validateFreshInstallJournal,
  validateOwnedRelativePath,
  validateReleaseIdentity,
} from '../lib/installation-state.mjs'

const sha256 = 'a'.repeat(64)

function releaseIdentity(verificationStatus = 'structure_only') {
  return {
    manifest_schema_version: 1,
    product: {
      name: 'My API',
      version: '1.2.0',
      source_sha: 'b'.repeat(40),
      build_revision: '1234567890abcdef1234567890abcdef12345678',
    },
    compatibility: {
      config_schema: '2.0.0',
      data_schema: '3.0.0',
    },
    artifact: {
      id: 'myapi-lite-linux-amd64',
      edition: 'lite',
      installation_shape: 'personal-native',
      os: 'linux',
      arch: 'amd64',
      kind: 'binary',
      location: 'https://releases.example.test/myapi-lite-linux-amd64',
      sha256_or_digest: sha256,
    },
    verification_status: verificationStatus,
  }
}

function at(second) {
  return `2026-09-06T00:00:${String(second).padStart(2, '0')}.000Z`
}

function assertError(ErrorType, code, action) {
  assert.throws(action, (error) => error instanceof ErrorType && error.code === code)
}

function createJournal(identity = releaseIdentity('verified')) {
  return createFreshInstallJournal({
    operation_id: 'install-operation-1',
    created_at: at(0),
    release_identity: identity,
  })
}

function commitJournal(identity = releaseIdentity('verified')) {
  let journal = createJournal(identity)
  for (let index = 1; index < FRESH_INSTALL_PROGRESS_STATES.length; index += 1) {
    const effectId = `effect-${index}`
    journal = beginFreshInstallEffect(journal, {
      effect_id: effectId,
      next_state: FRESH_INSTALL_PROGRESS_STATES[index],
      started_at: at(index * 2 - 1),
    })
    journal = completeFreshInstallEffect(journal, {
      effect_id: effectId,
      completed_at: at(index * 2),
    })
  }
  return journal
}

function cleanupInput(entries, identity = releaseIdentity('verified')) {
  const journal = commitJournal(identity)
  const installationRecord = buildCommittedInstallationRecord({
    installation_id: 'installation-1',
    journal,
    release_identity: identity,
  })
  return {
    journal,
    installation_record: installationRecord,
    ownership_document: {
      revision: 1,
      installation_id: 'installation-1',
      operation_id: 'install-operation-1',
      release_identity: identity,
      entries,
    },
    current_paths: [],
    protected_paths: [],
  }
}

test('rejects proxies and accessors without executing caller code', () => {
  assertError(InstallationStateValidationError, 'invalid_object', () =>
    validateReleaseIdentity(new Proxy(releaseIdentity(), {}))
  )

  const revoked = Proxy.revocable(releaseIdentity(), {})
  revoked.revoke()
  assertError(InstallationStateValidationError, 'invalid_object', () =>
    validateReleaseIdentity(revoked.proxy)
  )

  for (const trapName of ['get', 'getPrototypeOf', 'ownKeys', 'getOwnPropertyDescriptor']) {
    let trapExecuted = false
    const throwingProxy = new Proxy(releaseIdentity(), {
      [trapName]() {
        trapExecuted = true
        throw new Error(`${trapName} trap must not execute`)
      },
    })
    assertError(InstallationStateValidationError, 'invalid_object', () =>
      validateReleaseIdentity(throwingProxy)
    )
    assert.equal(trapExecuted, false)
  }

  const accessorIdentity = releaseIdentity()
  let getterExecuted = false
  Object.defineProperty(accessorIdentity, 'product', {
    configurable: true,
    enumerable: true,
    get() {
      getterExecuted = true
      throw new Error('release identity getter must not execute')
    },
  })
  assertError(InstallationStateValidationError, 'invalid_fields', () =>
    validateReleaseIdentity(accessorIdentity)
  )
  assert.equal(getterExecuted, false)

  const setterIdentity = releaseIdentity()
  let setterExecuted = false
  Object.defineProperty(setterIdentity, 'artifact', {
    configurable: true,
    enumerable: true,
    set() {
      setterExecuted = true
    },
  })
  assertError(InstallationStateValidationError, 'invalid_fields', () =>
    validateReleaseIdentity(setterIdentity)
  )
  assert.equal(setterExecuted, false)

  const journal = createJournal()
  assertError(InstallationStateValidationError, 'invalid_object', () =>
    validateFreshInstallJournal(new Proxy(journal, {}))
  )
  assertError(InstallationStateTransitionError, 'invalid_object', () =>
    beginFreshInstallEffect(journal, new Proxy({
      effect_id: 'proxy-effect',
      next_state: 'preflighted',
      started_at: at(1),
    }, {}))
  )

  const committed = commitJournal()
  const record = buildCommittedInstallationRecord({
    installation_id: 'installation-1',
    journal: committed,
    release_identity: releaseIdentity('verified'),
  })
  assertError(InstallationStateValidationError, 'invalid_object', () =>
    validateCommittedInstallationRecord(new Proxy(record, {}))
  )
  assertError(InstallationCleanupPlanError, 'invalid_object', () =>
    planOwnedFileCleanup(new Proxy(cleanupInput([]), {}))
  )
})

test('rejects custom objects and hostile installation arrays', () => {
  const customIdentity = releaseIdentity()
  Object.setPrototypeOf(customIdentity, { custom: true })
  assertError(InstallationStateValidationError, 'invalid_object', () =>
    validateReleaseIdentity(customIdentity)
  )

  const symbolIdentity = releaseIdentity()
  symbolIdentity[Symbol('extra')] = true
  assertError(InstallationStateValidationError, 'invalid_fields', () =>
    validateReleaseIdentity(symbolIdentity)
  )

  const nonEnumerableIdentity = releaseIdentity()
  Object.defineProperty(nonEnumerableIdentity, 'hidden', { value: true })
  assertError(InstallationStateValidationError, 'invalid_fields', () =>
    validateReleaseIdentity(nonEnumerableIdentity)
  )

  const sparseJournal = structuredClone(createJournal())
  sparseJournal.completed_effects = new Array(1)
  assertError(InstallationStateValidationError, 'invalid_array', () =>
    validateFreshInstallJournal(sparseJournal)
  )

  const customArrayJournal = structuredClone(createJournal())
  Object.setPrototypeOf(
    customArrayJournal.completed_effects,
    Object.create(Array.prototype)
  )
  assertError(InstallationStateValidationError, 'invalid_array', () =>
    validateFreshInstallJournal(customArrayJournal)
  )

  const extraArrayFieldJournal = structuredClone(createJournal())
  extraArrayFieldJournal.completed_effects.extra = true
  assertError(InstallationStateValidationError, 'invalid_array', () =>
    validateFreshInstallJournal(extraArrayFieldJournal)
  )

  const symbolArrayFieldJournal = structuredClone(createJournal())
  symbolArrayFieldJournal.completed_effects[Symbol('extra')] = true
  assertError(InstallationStateValidationError, 'invalid_array', () =>
    validateFreshInstallJournal(symbolArrayFieldJournal)
  )

  const nonEnumerableArrayFieldJournal = structuredClone(createJournal())
  Object.defineProperty(nonEnumerableArrayFieldJournal.completed_effects, 'hidden', {
    value: true,
  })
  assertError(InstallationStateValidationError, 'invalid_array', () =>
    validateFreshInstallJournal(nonEnumerableArrayFieldJournal)
  )

  const accessorArrayJournal = structuredClone(createJournal())
  let arrayGetterExecuted = false
  accessorArrayJournal.completed_effects = [null]
  Object.defineProperty(accessorArrayJournal.completed_effects, 0, {
    configurable: true,
    enumerable: true,
    get() {
      arrayGetterExecuted = true
      throw new Error('journal array getter must not execute')
    },
  })
  assertError(InstallationStateValidationError, 'invalid_array', () =>
    validateFreshInstallJournal(accessorArrayJournal)
  )
  assert.equal(arrayGetterExecuted, false)

  let proxyTrapExecuted = false
  const proxiedArrayJournal = structuredClone(createJournal())
  proxiedArrayJournal.completed_effects = new Proxy([], {
    ownKeys() {
      proxyTrapExecuted = true
      throw new Error('array proxy trap must not execute')
    },
  })
  assertError(InstallationStateValidationError, 'invalid_array', () =>
    validateFreshInstallJournal(proxiedArrayJournal)
  )
  assert.equal(proxyTrapExecuted, false)

  const proxiedOwnership = cleanupInput([])
  proxiedOwnership.ownership_document = new Proxy(
    proxiedOwnership.ownership_document,
    {}
  )
  assertError(InstallationCleanupPlanError, 'invalid_object', () =>
    planOwnedFileCleanup(proxiedOwnership)
  )
})

test('release identity validation detaches and deeply freezes bounded input', () => {
  const input = releaseIdentity()
  const before = structuredClone(input)
  const snapshot = validateReleaseIdentity(input)

  assert.notEqual(snapshot, input)
  assert.deepEqual(input, before)
  assert.ok(Object.isFrozen(snapshot))
  assert.ok(Object.isFrozen(snapshot.product))
  assert.ok(Object.isFrozen(snapshot.compatibility))
  assert.ok(Object.isFrozen(snapshot.artifact))
  assert.equal(snapshot.verification_status, 'structure_only')

  input.product.version = '9.9.9'
  input.artifact.id = 'tampered-after-validation'
  assert.equal(snapshot.product.version, '1.2.0')
  assert.equal(snapshot.artifact.id, 'myapi-lite-linux-amd64')
})

test('release identity accepts only supported Full/Lite/Desktop artifact shapes', () => {
  const digest = `sha256:${sha256}`
  for (const expected of [
    { edition: 'full', installation_shape: 'server-native', kind: 'binary', location: 'https://releases.example.test/myapi-full-native', sha256_or_digest: sha256 },
    { edition: 'full', installation_shape: 'server-container', kind: 'oci', location: `ghcr.io/forcemind/myapi@${digest}`, sha256_or_digest: digest },
    { edition: 'lite', installation_shape: 'server-native', kind: 'binary', location: 'https://releases.example.test/myapi-lite-server', sha256_or_digest: sha256 },
    { edition: 'lite', installation_shape: 'server-container', kind: 'oci', location: `ghcr.io/forcemind/myapi-lite@${digest}`, sha256_or_digest: digest },
    { edition: 'lite', installation_shape: 'personal-native', kind: 'binary', location: 'https://releases.example.test/myapi-lite-personal', sha256_or_digest: sha256 },
    { edition: 'lite', installation_shape: 'personal-container', kind: 'oci', location: `ghcr.io/forcemind/myapi-lite@${digest}`, sha256_or_digest: digest },
    { edition: 'lite', installation_shape: 'desktop', kind: 'desktop', location: 'https://releases.example.test/myapi-lite-desktop', sha256_or_digest: sha256 },
  ]) {
    const identity = releaseIdentity()
    Object.assign(identity.artifact, expected)
    assert.equal(validateReleaseIdentity(identity).artifact.installation_shape, expected.installation_shape)
  }

  for (const expected of [
    { edition: 'full', installation_shape: 'personal-native', kind: 'binary' },
    { edition: 'full', installation_shape: 'desktop', kind: 'desktop' },
  ]) {
    const identity = releaseIdentity()
    Object.assign(identity.artifact, expected)
    assertError(InstallationStateValidationError, 'invalid_release_identity', () =>
      validateReleaseIdentity(identity)
    )
  }
})

test('release identity refuses ambiguous OCI and mutable HTTPS references', () => {
  const digest = `sha256:${sha256}`
  for (const location of [
    `user:password@ghcr.io/forcemind/myapi@${digest}`,
    `ghcr.io/forcemind/myapi@${digest}?mutable=true`,
    `ghcr.io/forcemind/myapi@${digest}#fragment`,
    `ghcr.io/forcemind/myapi@@${digest}`,
    `https://ghcr.io/forcemind/myapi@${digest}`,
    `ghcr.io/forcemind/myapi;echo@${digest}`,
  ]) {
    const identity = releaseIdentity()
    Object.assign(identity.artifact, {
      id: 'myapi-lite-container',
      installation_shape: 'personal-container',
      kind: 'oci',
      location,
      sha256_or_digest: digest,
    })
    assertError(InstallationStateValidationError, 'invalid_release_identity', () =>
      validateReleaseIdentity(identity)
    )
  }

  for (const location of [
    'https://releases.example.test/latest',
    'https://releases.example.test/%6c%61%74%65%73%74/pkg',
    'https://releases.example.test/a//b',
    'https://releases.example.test/%C2%85/pkg',
  ]) {
    const identity = releaseIdentity()
    identity.artifact.location = location
    assertError(InstallationStateValidationError, 'invalid_release_identity', () =>
      validateReleaseIdentity(identity)
    )
  }
})

test('release identity rejects prototype objects, unknown fields, malformed values, and bounds', () => {
  const inherited = Object.create(releaseIdentity())
  assertError(InstallationStateValidationError, 'invalid_object', () =>
    validateReleaseIdentity(inherited)
  )

  const unknown = releaseIdentity()
  unknown.signature_evidence = 'not-accepted-here'
  assertError(InstallationStateValidationError, 'invalid_fields', () =>
    validateReleaseIdentity(unknown)
  )

  const tooLong = releaseIdentity()
  tooLong.artifact.id = 'a'.repeat(129)
  assertError(InstallationStateValidationError, 'invalid_release_identity', () =>
    validateReleaseIdentity(tooLong)
  )

  const malformedLocation = releaseIdentity()
  malformedLocation.artifact.location = 'https://releases.example.test/../mutable'
  assertError(InstallationStateValidationError, 'invalid_release_identity', () =>
    validateReleaseIdentity(malformedLocation)
  )

  const unsupportedStatus = releaseIdentity('trusted-by-claim')
  assertError(InstallationStateValidationError, 'invalid_value', () =>
    validateReleaseIdentity(unsupportedStatus)
  )
})

test('structure-only identity fails closed before artifact_verified', () => {
  const planned = createJournal(releaseIdentity('structure_only'))
  const pendingPreflight = beginFreshInstallEffect(planned, {
    effect_id: 'preflight-effect',
    next_state: 'preflighted',
    started_at: at(1),
  })
  const preflighted = completeFreshInstallEffect(pendingPreflight, {
    effect_id: 'preflight-effect',
    completed_at: at(2),
  })

  assertError(InstallationStateTransitionError, 'release_not_verified', () =>
    beginFreshInstallEffect(preflighted, {
      effect_id: 'verify-effect',
      next_state: 'artifact_verified',
      started_at: at(3),
    })
  )
  assert.equal(preflighted.state, 'preflighted')
  assert.equal(preflighted.pending_effect, null)
  assert.equal(preflighted.release_identity.verification_status, 'structure_only')
})

test('fresh-install state advances through the one deterministic legal sequence', () => {
  const identity = releaseIdentity('verified')
  const planned = createJournal(identity)
  const committed = commitJournal(identity)

  assert.equal(planned.state, 'planned')
  assert.equal(planned.completed_effects.length, 0)
  assert.equal(committed.state, 'committed')
  assert.equal(committed.pending_effect, null)
  assert.equal(committed.terminal, null)
  assert.equal(committed.updated_at, at(12))
  assert.equal(committed.completed_effects.length, 6)
  assert.deepEqual(
    committed.completed_effects.map(({ from_state, to_state }) => [from_state, to_state]),
    [
      ['planned', 'preflighted'],
      ['preflighted', 'artifact_verified'],
      ['artifact_verified', 'staged'],
      ['staged', 'service_started'],
      ['service_started', 'health_checked'],
      ['health_checked', 'committed'],
    ]
  )
  assert.ok(Object.isFrozen(committed))
  assert.ok(Object.isFrozen(committed.completed_effects))
  assert.ok(committed.completed_effects.every(Object.isFrozen))
  assert.deepEqual(validateFreshInstallJournal(committed), committed)

  assertError(InstallationStateTransitionError, 'illegal_transition', () =>
    beginFreshInstallEffect(planned, {
      effect_id: 'skip-effect',
      next_state: 'staged',
      started_at: at(1),
    })
  )
  assertError(InstallationStateTransitionError, 'terminal_journal', () =>
    beginFreshInstallEffect(committed, {
      effect_id: 'post-commit-effect',
      next_state: 'planned',
      started_at: at(13),
    })
  )
})

test('pending effect identity prevents mismatched completion and replay', () => {
  const planned = createJournal()
  const pending = beginFreshInstallEffect(planned, {
    effect_id: 'effect-one',
    next_state: 'preflighted',
    started_at: at(1),
  })

  assertError(InstallationStateTransitionError, 'pending_effect_mismatch', () =>
    completeFreshInstallEffect(pending, {
      effect_id: 'effect-two',
      completed_at: at(2),
    })
  )

  const completed = completeFreshInstallEffect(pending, {
    effect_id: 'effect-one',
    completed_at: at(2),
  })
  assertError(InstallationStateTransitionError, 'effect_replayed', () =>
    completeFreshInstallEffect(completed, {
      effect_id: 'effect-one',
      completed_at: at(3),
    })
  )
  assertError(InstallationStateTransitionError, 'effect_replayed', () =>
    beginFreshInstallEffect(completed, {
      effect_id: 'effect-one',
      next_state: 'artifact_verified',
      started_at: at(3),
    })
  )
})

test('known failure and outcome_unknown are fail-safe terminal outcomes', () => {
  const pendingFailure = beginFreshInstallEffect(createJournal(), {
    effect_id: 'failure-effect',
    next_state: 'preflighted',
    started_at: at(1),
  })
  assertError(InstallationStateTransitionError, 'invalid_fields', () =>
    failFreshInstallEffect(pendingFailure, {
      effect_id: 'failure-effect',
      recorded_at: at(2),
      reason_code: 'effect_failed',
      message: 'raw stderr is not accepted',
    })
  )
  assertError(InstallationStateTransitionError, 'invalid_value', () =>
    failFreshInstallEffect(pendingFailure, {
      effect_id: 'failure-effect',
      recorded_at: at(2),
      reason_code: 'arbitrary_secret_token',
    })
  )
  const failed = failFreshInstallEffect(pendingFailure, {
    effect_id: 'failure-effect',
    recorded_at: at(2),
    reason_code: 'effect_failed',
  })
  assert.equal(failed.state, 'failed')
  assert.equal(failed.terminal.outcome, 'failed')
  assert.equal(failed.pending_effect, null)

  const pendingUnknown = beginFreshInstallEffect(createJournal(), {
    effect_id: 'unknown-effect',
    next_state: 'preflighted',
    started_at: at(1),
  })
  const unknown = markFreshInstallOutcomeUnknown(pendingUnknown, {
    effect_id: 'unknown-effect',
    recorded_at: at(2),
    reason_code: 'effect_outcome_unknown',
  })
  assert.equal(unknown.state, 'outcome_unknown')
  assert.equal(unknown.terminal.effect_id, 'unknown-effect')
  assertError(InstallationStateTransitionError, 'terminal_journal', () =>
    beginFreshInstallEffect(unknown, {
      effect_id: 'retry-effect',
      next_state: 'preflighted',
      started_at: at(3),
    })
  )
  assertError(InstallationStateTransitionError, 'effect_replayed', () =>
    completeFreshInstallEffect(unknown, {
      effect_id: 'unknown-effect',
      completed_at: at(3),
    })
  )
})

test('committed record is revision 1, artifact-bound, deeply frozen, and local-only', () => {
  const identity = releaseIdentity('verified')
  const journal = commitJournal(identity)
  const record = buildCommittedInstallationRecord({
    installation_id: 'installation-1',
    journal,
    release_identity: identity,
  })

  assert.equal(record.revision, 1)
  assert.equal(record.committed_at, at(12))
  assert.deepEqual(record.access, {
    desired_access_mode: 'local',
    actual_listen: 'not_observed',
    externally_verified_reachability: 'not_checked',
  })
  assert.ok(Object.isFrozen(record))
  assert.ok(Object.isFrozen(record.access))
  assert.ok(Object.isFrozen(record.release_identity.artifact))
  assert.deepEqual(validateCommittedInstallationRecord(record), record)
  assertError(InstallationStateValidationError, 'invalid_object', () =>
    validateCommittedInstallationRecord(record, {
      expected_release_identity: undefined,
    })
  )

  const otherIdentity = releaseIdentity('verified')
  otherIdentity.artifact.id = 'myapi-lite-linux-amd64-other'
  assertError(InstallationStateValidationError, 'release_identity_mismatch', () =>
    validateCommittedInstallationRecord(record, {
      expected_release_identity: otherIdentity,
    })
  )
  assertError(InstallationStateValidationError, 'release_identity_mismatch', () =>
    buildCommittedInstallationRecord({
      installation_id: 'installation-1',
      journal,
      release_identity: otherIdentity,
    })
  )

  const publicRecord = structuredClone(record)
  publicRecord.access.desired_access_mode = 'public'
  assertError(InstallationStateValidationError, 'invalid_value', () =>
    validateCommittedInstallationRecord(publicRecord)
  )

  const structureOnlyRecord = structuredClone(record)
  structureOnlyRecord.release_identity.verification_status = 'structure_only'
  assertError(InstallationStateValidationError, 'release_not_verified', () =>
    validateCommittedInstallationRecord(structureOnlyRecord)
  )
})

test('lexical owned paths reject traversal, absolute forms, controls, and protected roots', () => {
  assert.equal(
    validateOwnedRelativePath('releases/1.2.0/myapi-lite/bin/myapi'),
    'releases/1.2.0/myapi-lite/bin/myapi'
  )
  assert.equal(validateOwnedRelativePath('staging/install-1/package.bin'), 'staging/install-1/package.bin')

  for (const unsafe of [
    '../outside',
    'releases/../data/secret',
    '/releases/1.2.0/file',
    'C:/releases/1.2.0/file',
    '\\\\server\\share\\file',
    'releases\\1.2.0\\file',
    'releases/file\0name',
    'releases/file\nname',
    'releases//file',
    'releases/file/',
  ]) {
    assertError(InstallationCleanupPlanError, 'unsafe_owned_path', () =>
      validateOwnedRelativePath(unsafe)
    )
  }

  for (const protectedPath of [
    'config/settings.json',
    'data/myapi.db',
    'logs/server.log',
    'backups/install/backup.tar',
    'cache/shared.bin',
    'state/installation.json',
  ]) {
    assertError(InstallationCleanupPlanError, 'protected_path', () =>
      validateOwnedRelativePath(protectedPath)
    )
  }
})

test('cleanup plan rejects case-fold duplicates, non-files, unknown fields, and prototypes', () => {
  assertError(InstallationCleanupPlanError, 'duplicate_path', () =>
    planOwnedFileCleanup(cleanupInput([
      { path: 'staging/install-1/Payload.bin', kind: 'regular_file' },
      { path: 'staging/install-1/payload.bin', kind: 'regular_file' },
    ]))
  )
  assertError(InstallationCleanupPlanError, 'cleanup_candidate_overlap', () =>
    planOwnedFileCleanup(cleanupInput([
      { path: 'staging/install-1/payload.bin', kind: 'regular_file' },
      { path: 'staging/install-1/payload.bin/child', kind: 'regular_file' },
    ]))
  )

  for (const kind of ['symlink', 'reparse_point', 'directory', 'unknown']) {
    assertError(InstallationCleanupPlanError, 'unsupported_entry_kind', () =>
      planOwnedFileCleanup(cleanupInput([
        { path: 'staging/install-1/payload.bin', kind },
      ]))
    )
  }

  const unknownField = cleanupInput([
    { path: 'staging/install-1/payload.bin', kind: 'regular_file', target: '/tmp/payload' },
  ])
  assertError(InstallationCleanupPlanError, 'invalid_fields', () =>
    planOwnedFileCleanup(unknownField)
  )

  const prototypeDocument = cleanupInput([])
  prototypeDocument.ownership_document = Object.create(prototypeDocument.ownership_document)
  assertError(InstallationCleanupPlanError, 'invalid_object', () =>
    planOwnedFileCleanup(prototypeDocument)
  )
})

test('cleanup plan refuses current/protected overlap and returns stable explicit file candidates', () => {
  const recordedCurrentConflict = cleanupInput([
    { path: 'releases/1.2.0/myapi-lite-linux-amd64/bin/myapi', kind: 'regular_file' },
  ])
  assertError(InstallationCleanupPlanError, 'cleanup_boundary_conflict', () =>
    planOwnedFileCleanup(recordedCurrentConflict)
  )

  const currentConflict = cleanupInput([
    { path: 'releases/1.2.0/myapi-lite/bin/myapi', kind: 'regular_file' },
  ])
  currentConflict.current_paths = ['releases/1.2.0/myapi-lite']
  assertError(InstallationCleanupPlanError, 'cleanup_boundary_conflict', () =>
    planOwnedFileCleanup(currentConflict)
  )

  const protectedConflict = cleanupInput([
    { path: 'staging/install-1/keep/payload.bin', kind: 'regular_file' },
  ])
  protectedConflict.protected_paths = ['staging/install-1/keep']
  assertError(InstallationCleanupPlanError, 'cleanup_boundary_conflict', () =>
    planOwnedFileCleanup(protectedConflict)
  )

  const entries = [
    { path: 'staging/install-1/package.bin', kind: 'regular_file' },
    { path: 'releases/1.1.0/old/bin/myapi', kind: 'regular_file' },
  ]
  const first = cleanupInput(entries)
  first.current_paths = ['releases/1.2.0/current']
  first.protected_paths = ['releases/1.0.0/recovery']
  const second = cleanupInput([...entries].reverse())
  second.current_paths = [...first.current_paths]
  second.protected_paths = [...first.protected_paths]

  const firstPlan = planOwnedFileCleanup(first)
  const secondPlan = planOwnedFileCleanup(second)
  assert.deepEqual(firstPlan, secondPlan)
  assert.equal(firstPlan.plan_type, 'post_commit_lexical_cleanup_only')
  assert.equal(firstPlan.installation_id, 'installation-1')
  assert.equal(firstPlan.operation_id, 'install-operation-1')
  assert.deepEqual(firstPlan.release_identity, validateReleaseIdentity(releaseIdentity('verified')))
  assert.deepEqual(firstPlan.preservation_roots, PRESERVED_INSTALLATION_ROOTS)
  assert.deepEqual(firstPlan.candidates, [
    { path: 'releases/1.1.0/old/bin/myapi', kind: 'regular_file' },
    { path: 'staging/install-1/package.bin', kind: 'regular_file' },
  ])
  assert.ok(Object.isFrozen(firstPlan))
  assert.ok(Object.isFrozen(firstPlan.current_paths))
  assert.ok(Object.isFrozen(firstPlan.protected_paths))
  assert.ok(Object.isFrozen(firstPlan.candidates))
  assert.ok(firstPlan.candidates.every(Object.isFrozen))
})

test('cleanup plan requires post-health commit and matching ownership authority', () => {
  const entry = {
    path: 'staging/install-1/package.bin',
    kind: 'regular_file',
  }
  const beforeCommit = cleanupInput([entry])
  beforeCommit.journal = createJournal(releaseIdentity('verified'))
  assertError(InstallationCleanupPlanError, 'cleanup_not_committed', () =>
    planOwnedFileCleanup(beforeCommit)
  )

  const unknownOutcome = cleanupInput([entry])
  const pending = beginFreshInstallEffect(createJournal(releaseIdentity('verified')), {
    effect_id: 'unknown-before-cleanup',
    next_state: 'preflighted',
    started_at: at(1),
  })
  unknownOutcome.journal = markFreshInstallOutcomeUnknown(pending, {
    effect_id: 'unknown-before-cleanup',
    recorded_at: at(2),
    reason_code: 'effect_outcome_unknown',
  })
  assertError(InstallationCleanupPlanError, 'cleanup_not_committed', () =>
    planOwnedFileCleanup(unknownOutcome)
  )

  const wrongInstallation = cleanupInput([entry])
  wrongInstallation.ownership_document.installation_id = 'installation-2'
  assertError(InstallationCleanupPlanError, 'cleanup_binding_mismatch', () =>
    planOwnedFileCleanup(wrongInstallation)
  )

  const wrongArtifact = cleanupInput([entry])
  wrongArtifact.ownership_document.release_identity.artifact.id = 'myapi-lite-linux-amd64-other'
  assertError(InstallationCleanupPlanError, 'cleanup_binding_mismatch', () =>
    planOwnedFileCleanup(wrongArtifact)
  )

  const journalRecordMismatch = cleanupInput([entry])
  const otherJournalIdentity = releaseIdentity('verified')
  otherJournalIdentity.artifact.id = 'myapi-lite-linux-amd64-other'
  journalRecordMismatch.journal = commitJournal(otherJournalIdentity)
  assertError(InstallationCleanupPlanError, 'cleanup_binding_mismatch', () =>
    planOwnedFileCleanup(journalRecordMismatch)
  )

  const longVersionIdentity = releaseIdentity('verified')
  longVersionIdentity.product.version = `1.2.0-${'a'.repeat(150)}`
  assertError(InstallationCleanupPlanError, 'unsafe_owned_path', () =>
    planOwnedFileCleanup(cleanupInput([], longVersionIdentity))
  )

  const tooManyCurrentBoundaries = cleanupInput([])
  tooManyCurrentBoundaries.current_paths = Array.from(
    { length: 1024 },
    (_, index) => `releases/retained-${index}`
  )
  assertError(InstallationCleanupPlanError, 'invalid_array', () =>
    planOwnedFileCleanup(tooManyCurrentBoundaries)
  )
})
