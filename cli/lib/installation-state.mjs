/*
 * MyAPI distribution and self-hosting tooling.
 * Copyright (C) 2026 ForceMind
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published
 * by the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 */

/**
 * Pure, in-memory contracts for a future fresh-install implementation.
 *
 * This module intentionally performs no I/O. In particular, it does not
 * verify signatures, inspect files, acquire locks, install or remove files,
 * control services, or implement upgrade, switch, or rollback. A `verified`
 * release identity is only a fixture/caller assertion used to exercise the
 * state contract; it is not evidence of trust and must not be accepted from
 * untrusted input by a future executor.
 */

import { types } from 'node:util'

import {
  isPinnedHTTPSURL,
  parseCanonicalOCIReference,
} from './canonical-artifact-reference.mjs'

export const INSTALLATION_STATE_REVISION = 1

export const RELEASE_IDENTITY_STATUSES = Object.freeze([
  'structure_only',
  'verified',
])

export const FRESH_INSTALL_PROGRESS_STATES = Object.freeze([
  'planned',
  'preflighted',
  'artifact_verified',
  'staged',
  'service_started',
  'health_checked',
  'committed',
])

export const FRESH_INSTALL_TERMINAL_STATES = Object.freeze([
  'failed',
  'outcome_unknown',
])

export const FRESH_INSTALL_TERMINAL_REASON_CODES = Object.freeze({
  FAILED: 'effect_failed',
  OUTCOME_UNKNOWN: 'effect_outcome_unknown',
})

export const PRESERVED_INSTALLATION_ROOTS = Object.freeze([
  'config',
  'data',
  'logs',
  'backups',
  'cache',
  'state',
])

export const INSTALLATION_STATE_ERROR_CODES = Object.freeze({
  INVALID_OBJECT: 'invalid_object',
  INVALID_FIELDS: 'invalid_fields',
  INVALID_ARRAY: 'invalid_array',
  INVALID_STRING: 'invalid_string',
  INVALID_IDENTIFIER: 'invalid_identifier',
  INVALID_TIMESTAMP: 'invalid_timestamp',
  INVALID_VALUE: 'invalid_value',
  INVALID_RELEASE_IDENTITY: 'invalid_release_identity',
  INVALID_JOURNAL: 'invalid_journal',
  RELEASE_NOT_VERIFIED: 'release_not_verified',
  ILLEGAL_TRANSITION: 'illegal_transition',
  PENDING_EFFECT_EXISTS: 'pending_effect_exists',
  PENDING_EFFECT_MISSING: 'pending_effect_missing',
  PENDING_EFFECT_MISMATCH: 'pending_effect_mismatch',
  EFFECT_REPLAYED: 'effect_replayed',
  TERMINAL_JOURNAL: 'terminal_journal',
  RELEASE_IDENTITY_MISMATCH: 'release_identity_mismatch',
  UNSAFE_OWNED_PATH: 'unsafe_owned_path',
  PATH_OUTSIDE_OWNED_ROOTS: 'path_outside_owned_roots',
  PROTECTED_PATH: 'protected_path',
  DUPLICATE_PATH: 'duplicate_path',
  UNSUPPORTED_ENTRY_KIND: 'unsupported_entry_kind',
  CLEANUP_NOT_COMMITTED: 'cleanup_not_committed',
  CLEANUP_BINDING_MISMATCH: 'cleanup_binding_mismatch',
  CLEANUP_CANDIDATE_OVERLAP: 'cleanup_candidate_overlap',
  CLEANUP_BOUNDARY_CONFLICT: 'cleanup_boundary_conflict',
})

export class InstallationStateValidationError extends Error {
  constructor(code, message) {
    super(message)
    this.name = 'InstallationStateValidationError'
    this.code = code
  }
}

export class InstallationStateTransitionError extends Error {
  constructor(code, message) {
    super(message)
    this.name = 'InstallationStateTransitionError'
    this.code = code
  }
}

export class InstallationCleanupPlanError extends Error {
  constructor(code, message) {
    super(message)
    this.name = 'InstallationCleanupPlanError'
    this.code = code
  }
}

const {
  INVALID_OBJECT,
  INVALID_FIELDS,
  INVALID_ARRAY,
  INVALID_STRING,
  INVALID_IDENTIFIER,
  INVALID_TIMESTAMP,
  INVALID_VALUE,
  INVALID_RELEASE_IDENTITY,
  INVALID_JOURNAL,
  RELEASE_NOT_VERIFIED,
  ILLEGAL_TRANSITION,
  PENDING_EFFECT_EXISTS,
  PENDING_EFFECT_MISSING,
  PENDING_EFFECT_MISMATCH,
  EFFECT_REPLAYED,
  TERMINAL_JOURNAL,
  RELEASE_IDENTITY_MISMATCH,
  UNSAFE_OWNED_PATH,
  PATH_OUTSIDE_OWNED_ROOTS,
  PROTECTED_PATH,
  DUPLICATE_PATH,
  UNSUPPORTED_ENTRY_KIND,
  CLEANUP_NOT_COMMITTED,
  CLEANUP_BINDING_MISMATCH,
  CLEANUP_CANDIDATE_OVERLAP,
  CLEANUP_BOUNDARY_CONFLICT,
} = INSTALLATION_STATE_ERROR_CODES

const MAX_ID_LENGTH = 128
const MAX_SEMVER_LENGTH = 256
const MAX_REFERENCE_LENGTH = 2048
const MAX_OWNED_PATH_LENGTH = 1024
const MAX_PATH_SEGMENT_LENGTH = 128
const MAX_PATH_DEPTH = 32
const MAX_OWNED_ENTRIES = 4096
const MAX_BOUNDARY_PATHS = 1024

const IDENTIFIER_PATTERN = /^[A-Za-z0-9](?:[A-Za-z0-9._-]{0,127})?$/
const PORTABLE_ARTIFACT_ID_PATTERN = /^[a-z0-9](?:[a-z0-9._-]{0,127})?$/
const SHA256_PATTERN = /^[a-fA-F0-9]{64}$/
const OCI_DIGEST_PATTERN = /^sha256:[a-f0-9]{64}$/
const GIT_REVISION_PATTERN = /^(?:[a-fA-F0-9]{40}|[a-fA-F0-9]{64})$/
const SEMVER_PATTERN = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$/
const CANONICAL_TIMESTAMP_PATTERN = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/
const PORTABLE_PATH_SEGMENT_PATTERN = /^[A-Za-z0-9_@+-](?:[A-Za-z0-9._@+-]{0,127})?$/
const CONTROL_CHARACTER_PATTERN = /[\p{Cc}\p{Cf}]/u
const WINDOWS_RESERVED_FILE_BASENAMES = new Set([
  'con',
  'prn',
  'aux',
  'nul',
  'com1',
  'com2',
  'com3',
  'com4',
  'com5',
  'com6',
  'com7',
  'com8',
  'com9',
  'lpt1',
  'lpt2',
  'lpt3',
  'lpt4',
  'lpt5',
  'lpt6',
  'lpt7',
  'lpt8',
  'lpt9',
])

const RELEASE_EDITIONS = Object.freeze(['full', 'lite'])
const INSTALLATION_SHAPES = Object.freeze([
  'server-native',
  'server-container',
  'personal-native',
  'personal-container',
  'desktop',
])
const RELEASE_OPERATING_SYSTEMS = Object.freeze(['linux', 'darwin', 'win32'])
const RELEASE_ARCHITECTURES = Object.freeze(['amd64', 'arm64'])
const RELEASE_ARTIFACT_KINDS = Object.freeze(['oci', 'binary', 'desktop'])
const OWNED_ROOTS = Object.freeze(['releases', 'staging'])
const RELEASE_ARTIFACT_KEYS = Object.freeze([
  'id',
  'edition',
  'installation_shape',
  'os',
  'arch',
  'kind',
  'location',
  'sha256_or_digest',
])

function validationError(code, message) {
  throw new InstallationStateValidationError(code, message)
}

function transitionError(code, message) {
  throw new InstallationStateTransitionError(code, message)
}

function cleanupError(code, message) {
  throw new InstallationCleanupPlanError(code, message)
}

function assertDataObject(value, path, allowedKeys, fail = validationError) {
  if (
    value === null ||
    typeof value !== 'object' ||
    types.isProxy(value) ||
    Array.isArray(value) ||
    Object.getPrototypeOf(value) !== Object.prototype
  ) {
    fail(INVALID_OBJECT, `${path} must be a plain data object`)
  }

  const ownKeys = Reflect.ownKeys(value)
  if (ownKeys.some((key) => typeof key !== 'string')) {
    fail(INVALID_FIELDS, `${path} must not contain symbol fields`)
  }
  for (const key of ownKeys) {
    const descriptor = Object.getOwnPropertyDescriptor(value, key)
    if (!descriptor?.enumerable || !Object.hasOwn(descriptor, 'value')) {
      fail(INVALID_FIELDS, `${path}.${key} must be an enumerable data field`)
    }
  }

  const missing = allowedKeys.filter((key) => !Object.hasOwn(value, key))
  const unexpected = ownKeys.filter((key) => !allowedKeys.includes(key))
  if (missing.length > 0 || unexpected.length > 0) {
    const details = [
      missing.length > 0 ? `missing ${missing.join(', ')}` : '',
      unexpected.length > 0 ? `unsupported ${unexpected.join(', ')}` : '',
    ]
      .filter(Boolean)
      .join('; ')
    fail(INVALID_FIELDS, `${path} has invalid fields: ${details}`)
  }
}

function assertOptionalDataObject(value, path, allowedKeys, fail = validationError) {
  if (
    value === null ||
    typeof value !== 'object' ||
    types.isProxy(value) ||
    Array.isArray(value) ||
    Object.getPrototypeOf(value) !== Object.prototype
  ) {
    fail(INVALID_OBJECT, `${path} must be a plain data object`)
  }
  const ownKeys = Reflect.ownKeys(value)
  assertDataObject(value, path, ownKeys, fail)
  const unexpected = ownKeys.filter(
    (key) => typeof key !== 'string' || !allowedKeys.includes(key)
  )
  if (unexpected.length > 0) {
    fail(INVALID_FIELDS, `${path} contains unsupported fields`)
  }
}

function assertDataArray(value, path, minimum, maximum, fail = validationError) {
  if (
    types.isProxy(value) ||
    !Array.isArray(value) ||
    Object.getPrototypeOf(value) !== Array.prototype
  ) {
    fail(INVALID_ARRAY, `${path} must be a plain array`)
  }
  if (value.length < minimum || value.length > maximum) {
    fail(INVALID_ARRAY, `${path} must contain between ${minimum} and ${maximum} entries`)
  }

  const ownKeys = Reflect.ownKeys(value)
  for (const key of ownKeys) {
    if (key === 'length') continue
    if (typeof key !== 'string' || !/^(?:0|[1-9]\d*)$/.test(key) || Number(key) >= value.length) {
      fail(INVALID_ARRAY, `${path} must not contain non-index fields`)
    }
    const descriptor = Object.getOwnPropertyDescriptor(value, key)
    if (!descriptor?.enumerable || !Object.hasOwn(descriptor, 'value')) {
      fail(INVALID_ARRAY, `${path}[${key}] must be an enumerable data entry`)
    }
  }
  for (let index = 0; index < value.length; index += 1) {
    if (!Object.hasOwn(value, index)) {
      fail(INVALID_ARRAY, `${path} must not contain sparse entries`)
    }
  }
}

function assertOneOf(value, path, values, fail = validationError) {
  if (!values.includes(value)) {
    fail(INVALID_VALUE, `${path} must be one of ${values.join(', ')}`)
  }
}

function assertIdentifier(value, path, fail = validationError) {
  if (typeof value !== 'string' || !IDENTIFIER_PATTERN.test(value)) {
    fail(INVALID_IDENTIFIER, `${path} must be a portable identifier of at most ${MAX_ID_LENGTH} characters`)
  }
}

function assertCanonicalTimestamp(value, path, fail = validationError) {
  if (
    typeof value !== 'string' ||
    !CANONICAL_TIMESTAMP_PATTERN.test(value) ||
    Number.isNaN(Date.parse(value)) ||
    new Date(value).toISOString() !== value
  ) {
    fail(INVALID_TIMESTAMP, `${path} must be a canonical UTC timestamp with millisecond precision`)
  }
}

function assertTimeNotBefore(value, lowerBound, path, fail = validationError) {
  if (value < lowerBound) {
    fail(INVALID_TIMESTAMP, `${path} must not be earlier than ${lowerBound}`)
  }
}

function assertSemver(value, path) {
  if (
    typeof value !== 'string' ||
    value.length > MAX_SEMVER_LENGTH ||
    !SEMVER_PATTERN.test(value)
  ) {
    validationError(INVALID_RELEASE_IDENTITY, `${path} must be a bounded semantic version without a v prefix`)
  }
}

function assertSafeReference(value, path) {
  if (
    typeof value !== 'string' ||
    value.length === 0 ||
    value.length > MAX_REFERENCE_LENGTH ||
    !/^[\x21-\x7e]+$/.test(value)
  ) {
    validationError(INVALID_RELEASE_IDENTITY, `${path} must be a bounded visible ASCII reference`)
  }
}

function validateHTTPSLocation(value, path) {
  assertSafeReference(value, path)
  if (!isPinnedHTTPSURL(value)) {
    validationError(INVALID_RELEASE_IDENTITY, `${path} must be a canonical pinned HTTPS URL`)
  }
}

function snapshotArtifactIdentity(artifact) {
  assertDataObject(artifact, 'release_identity.artifact', RELEASE_ARTIFACT_KEYS)
  if (
    typeof artifact.id !== 'string' ||
    !PORTABLE_ARTIFACT_ID_PATTERN.test(artifact.id) ||
    artifact.id.endsWith('.') ||
    WINDOWS_RESERVED_FILE_BASENAMES.has(artifact.id.split('.', 1)[0])
  ) {
    validationError(INVALID_RELEASE_IDENTITY, 'release_identity.artifact.id must be a portable lowercase artifact identifier')
  }
  assertOneOf(artifact.edition, 'release_identity.artifact.edition', RELEASE_EDITIONS)
  assertOneOf(
    artifact.installation_shape,
    'release_identity.artifact.installation_shape',
    INSTALLATION_SHAPES
  )
  assertOneOf(artifact.os, 'release_identity.artifact.os', RELEASE_OPERATING_SYSTEMS)
  assertOneOf(artifact.arch, 'release_identity.artifact.arch', RELEASE_ARCHITECTURES)
  assertOneOf(artifact.kind, 'release_identity.artifact.kind', RELEASE_ARTIFACT_KINDS)

  const expectedKind = {
    'server-native': 'binary',
    'server-container': 'oci',
    'personal-native': 'binary',
    'personal-container': 'oci',
    desktop: 'desktop',
  }[artifact.installation_shape]
  if (artifact.kind !== expectedKind) {
    validationError(INVALID_RELEASE_IDENTITY, 'release_identity.artifact.kind does not match installation_shape')
  }
  if (artifact.edition === 'full' && !artifact.installation_shape.startsWith('server-')) {
    validationError(INVALID_RELEASE_IDENTITY, 'release_identity full artifacts must use a server installation shape')
  }
  if (artifact.installation_shape === 'desktop' && artifact.edition !== 'lite') {
    validationError(INVALID_RELEASE_IDENTITY, 'release_identity desktop artifacts must use the lite edition')
  }

  if (artifact.kind === 'oci') {
    if (typeof artifact.sha256_or_digest !== 'string' || !OCI_DIGEST_PATTERN.test(artifact.sha256_or_digest)) {
      validationError(INVALID_RELEASE_IDENTITY, 'release_identity OCI artifacts require a lowercase sha256 digest')
    }
    assertSafeReference(artifact.location, 'release_identity.artifact.location')
    const location = parseCanonicalOCIReference(artifact.location)
    if (location === null || location.digest !== artifact.sha256_or_digest) {
      validationError(INVALID_RELEASE_IDENTITY, 'release_identity OCI location must be pinned to its declared digest')
    }
  } else {
    if (typeof artifact.sha256_or_digest !== 'string' || !SHA256_PATTERN.test(artifact.sha256_or_digest)) {
      validationError(INVALID_RELEASE_IDENTITY, 'release_identity downloadable artifacts require a SHA-256 hash')
    }
    validateHTTPSLocation(artifact.location, 'release_identity.artifact.location')
  }

  return Object.freeze({
    id: artifact.id,
    edition: artifact.edition,
    installation_shape: artifact.installation_shape,
    os: artifact.os,
    arch: artifact.arch,
    kind: artifact.kind,
    location: artifact.location,
    sha256_or_digest: artifact.sha256_or_digest,
  })
}

/**
 * Validate and detach a release identity copied from an already structurally
 * validated schema-1 manifest and selected artifact. No signature material is
 * accepted or evaluated here. `verified` exists only for deterministic fixture
 * validation until the separate trust decision (D11) is resolved.
 */
export function validateReleaseIdentity(identity) {
  assertDataObject(identity, 'release_identity', [
    'manifest_schema_version',
    'product',
    'compatibility',
    'artifact',
    'verification_status',
  ])
  if (identity.manifest_schema_version !== 1) {
    validationError(INVALID_RELEASE_IDENTITY, 'release_identity.manifest_schema_version must be 1')
  }

  assertDataObject(identity.product, 'release_identity.product', [
    'name',
    'version',
    'source_sha',
    'build_revision',
  ])
  if (identity.product.name !== 'My API') {
    validationError(INVALID_RELEASE_IDENTITY, 'release_identity.product.name must be My API')
  }
  assertSemver(identity.product.version, 'release_identity.product.version')
  if (
    typeof identity.product.source_sha !== 'string' ||
    !GIT_REVISION_PATTERN.test(identity.product.source_sha)
  ) {
    validationError(INVALID_RELEASE_IDENTITY, 'release_identity.product.source_sha must be a Git revision hash')
  }
  if (
    typeof identity.product.build_revision !== 'string' ||
    !GIT_REVISION_PATTERN.test(identity.product.build_revision)
  ) {
    validationError(INVALID_RELEASE_IDENTITY, 'release_identity.product.build_revision must be a Git revision hash')
  }

  assertDataObject(identity.compatibility, 'release_identity.compatibility', [
    'config_schema',
    'data_schema',
  ])
  assertSemver(identity.compatibility.config_schema, 'release_identity.compatibility.config_schema')
  assertSemver(identity.compatibility.data_schema, 'release_identity.compatibility.data_schema')
  assertOneOf(
    identity.verification_status,
    'release_identity.verification_status',
    RELEASE_IDENTITY_STATUSES
  )

  return Object.freeze({
    manifest_schema_version: 1,
    product: Object.freeze({
      name: identity.product.name,
      version: identity.product.version,
      source_sha: identity.product.source_sha,
      build_revision: identity.product.build_revision,
    }),
    compatibility: Object.freeze({
      config_schema: identity.compatibility.config_schema,
      data_schema: identity.compatibility.data_schema,
    }),
    artifact: snapshotArtifactIdentity(identity.artifact),
    verification_status: identity.verification_status,
  })
}

function snapshotCompletedEffect(effect, path) {
  assertDataObject(effect, path, [
    'effect_id',
    'from_state',
    'to_state',
    'started_at',
    'completed_at',
  ])
  assertIdentifier(effect.effect_id, `${path}.effect_id`)
  assertOneOf(effect.from_state, `${path}.from_state`, FRESH_INSTALL_PROGRESS_STATES)
  assertOneOf(effect.to_state, `${path}.to_state`, FRESH_INSTALL_PROGRESS_STATES)
  assertCanonicalTimestamp(effect.started_at, `${path}.started_at`)
  assertCanonicalTimestamp(effect.completed_at, `${path}.completed_at`)
  assertTimeNotBefore(effect.completed_at, effect.started_at, `${path}.completed_at`)
  return Object.freeze({
    effect_id: effect.effect_id,
    from_state: effect.from_state,
    to_state: effect.to_state,
    started_at: effect.started_at,
    completed_at: effect.completed_at,
  })
}

function snapshotPendingEffect(effect, path) {
  assertDataObject(effect, path, [
    'effect_id',
    'from_state',
    'to_state',
    'started_at',
  ])
  assertIdentifier(effect.effect_id, `${path}.effect_id`)
  assertOneOf(effect.from_state, `${path}.from_state`, FRESH_INSTALL_PROGRESS_STATES)
  assertOneOf(effect.to_state, `${path}.to_state`, FRESH_INSTALL_PROGRESS_STATES)
  assertCanonicalTimestamp(effect.started_at, `${path}.started_at`)
  return Object.freeze({
    effect_id: effect.effect_id,
    from_state: effect.from_state,
    to_state: effect.to_state,
    started_at: effect.started_at,
  })
}

function snapshotTerminalOutcome(terminal) {
  assertDataObject(terminal, 'journal.terminal', [
    'outcome',
    'effect_id',
    'from_state',
    'to_state',
    'started_at',
    'recorded_at',
    'reason_code',
  ])
  assertOneOf(terminal.outcome, 'journal.terminal.outcome', FRESH_INSTALL_TERMINAL_STATES)
  assertIdentifier(terminal.effect_id, 'journal.terminal.effect_id')
  assertOneOf(terminal.from_state, 'journal.terminal.from_state', FRESH_INSTALL_PROGRESS_STATES)
  assertOneOf(terminal.to_state, 'journal.terminal.to_state', FRESH_INSTALL_PROGRESS_STATES)
  assertCanonicalTimestamp(terminal.started_at, 'journal.terminal.started_at')
  assertCanonicalTimestamp(terminal.recorded_at, 'journal.terminal.recorded_at')
  assertTimeNotBefore(terminal.recorded_at, terminal.started_at, 'journal.terminal.recorded_at')
  const expectedReasonCode = terminal.outcome === 'failed'
    ? FRESH_INSTALL_TERMINAL_REASON_CODES.FAILED
    : FRESH_INSTALL_TERMINAL_REASON_CODES.OUTCOME_UNKNOWN
  if (terminal.reason_code !== expectedReasonCode) {
    validationError(INVALID_JOURNAL, 'journal.terminal.reason_code does not match its terminal outcome')
  }
  return Object.freeze({
    outcome: terminal.outcome,
    effect_id: terminal.effect_id,
    from_state: terminal.from_state,
    to_state: terminal.to_state,
    started_at: terminal.started_at,
    recorded_at: terminal.recorded_at,
    reason_code: terminal.reason_code,
  })
}

function nextProgressState(state) {
  const index = FRESH_INSTALL_PROGRESS_STATES.indexOf(state)
  return index === -1 ? undefined : FRESH_INSTALL_PROGRESS_STATES[index + 1]
}

function effectIds(journal) {
  const ids = new Set(journal.completed_effects.map((effect) => effect.effect_id))
  if (journal.pending_effect !== null) ids.add(journal.pending_effect.effect_id)
  if (journal.terminal !== null) ids.add(journal.terminal.effect_id)
  return ids
}

function assertReleaseCanReach(identity, state) {
  const artifactVerifiedIndex = FRESH_INSTALL_PROGRESS_STATES.indexOf('artifact_verified')
  if (
    FRESH_INSTALL_PROGRESS_STATES.indexOf(state) >= artifactVerifiedIndex &&
    identity.verification_status !== 'verified'
  ) {
    validationError(INVALID_JOURNAL, 'a structure_only release cannot reach artifact_verified')
  }
}

/** Validate journal structure and all state/effect invariants, returning a detached snapshot. */
export function validateFreshInstallJournal(journal) {
  assertDataObject(journal, 'journal', [
    'revision',
    'operation',
    'operation_id',
    'created_at',
    'updated_at',
    'state',
    'release_identity',
    'pending_effect',
    'completed_effects',
    'terminal',
  ])
  if (journal.revision !== INSTALLATION_STATE_REVISION) {
    validationError(INVALID_JOURNAL, `journal.revision must be ${INSTALLATION_STATE_REVISION}`)
  }
  if (journal.operation !== 'fresh-install') {
    validationError(INVALID_JOURNAL, 'journal.operation must be fresh-install')
  }
  assertIdentifier(journal.operation_id, 'journal.operation_id')
  assertCanonicalTimestamp(journal.created_at, 'journal.created_at')
  assertCanonicalTimestamp(journal.updated_at, 'journal.updated_at')
  assertTimeNotBefore(journal.updated_at, journal.created_at, 'journal.updated_at')
  assertOneOf(
    journal.state,
    'journal.state',
    [...FRESH_INSTALL_PROGRESS_STATES, ...FRESH_INSTALL_TERMINAL_STATES]
  )

  const releaseIdentity = validateReleaseIdentity(journal.release_identity)
  assertDataArray(
    journal.completed_effects,
    'journal.completed_effects',
    0,
    FRESH_INSTALL_PROGRESS_STATES.length - 1
  )
  const completedEffects = journal.completed_effects.map((effect, index) =>
    snapshotCompletedEffect(effect, `journal.completed_effects[${index}]`)
  )
  const pendingEffect = journal.pending_effect === null
    ? null
    : snapshotPendingEffect(journal.pending_effect, 'journal.pending_effect')
  const terminal = journal.terminal === null
    ? null
    : snapshotTerminalOutcome(journal.terminal)

  let cursorState = FRESH_INSTALL_PROGRESS_STATES[0]
  let cursorTime = journal.created_at
  const usedEffectIds = new Set()
  for (const [index, effect] of completedEffects.entries()) {
    const expectedNext = nextProgressState(cursorState)
    if (effect.from_state !== cursorState || effect.to_state !== expectedNext) {
      validationError(INVALID_JOURNAL, `journal.completed_effects[${index}] is not the next legal transition`)
    }
    if (usedEffectIds.has(effect.effect_id)) {
      validationError(INVALID_JOURNAL, 'journal effect identifiers must be unique')
    }
    assertTimeNotBefore(effect.started_at, cursorTime, `journal.completed_effects[${index}].started_at`)
    usedEffectIds.add(effect.effect_id)
    cursorState = effect.to_state
    cursorTime = effect.completed_at
    assertReleaseCanReach(releaseIdentity, cursorState)
  }

  if (pendingEffect !== null && terminal !== null) {
    validationError(INVALID_JOURNAL, 'journal cannot have both pending_effect and terminal')
  }

  if (pendingEffect !== null) {
    const expectedNext = nextProgressState(cursorState)
    if (
      expectedNext === undefined ||
      pendingEffect.from_state !== cursorState ||
      pendingEffect.to_state !== expectedNext ||
      journal.state !== cursorState
    ) {
      validationError(INVALID_JOURNAL, 'journal.pending_effect is not the next legal transition')
    }
    if (usedEffectIds.has(pendingEffect.effect_id)) {
      validationError(INVALID_JOURNAL, 'journal.pending_effect reuses an effect identifier')
    }
    assertTimeNotBefore(pendingEffect.started_at, cursorTime, 'journal.pending_effect.started_at')
    if (journal.updated_at !== pendingEffect.started_at) {
      validationError(INVALID_JOURNAL, 'journal.updated_at must equal the pending effect start time')
    }
    assertReleaseCanReach(releaseIdentity, pendingEffect.to_state)
  } else if (terminal !== null) {
    const expectedNext = nextProgressState(cursorState)
    if (
      expectedNext === undefined ||
      terminal.from_state !== cursorState ||
      terminal.to_state !== expectedNext ||
      journal.state !== terminal.outcome
    ) {
      validationError(INVALID_JOURNAL, 'journal.terminal does not describe the interrupted next transition')
    }
    if (usedEffectIds.has(terminal.effect_id)) {
      validationError(INVALID_JOURNAL, 'journal.terminal reuses an effect identifier')
    }
    assertTimeNotBefore(terminal.started_at, cursorTime, 'journal.terminal.started_at')
    if (journal.updated_at !== terminal.recorded_at) {
      validationError(INVALID_JOURNAL, 'journal.updated_at must equal terminal.recorded_at')
    }
    assertReleaseCanReach(releaseIdentity, terminal.to_state)
  } else {
    if (journal.state !== cursorState) {
      validationError(INVALID_JOURNAL, 'journal.state does not match its completed transitions')
    }
    if (journal.updated_at !== cursorTime) {
      validationError(INVALID_JOURNAL, 'journal.updated_at must equal the last completed effect time')
    }
  }

  return Object.freeze({
    revision: INSTALLATION_STATE_REVISION,
    operation: 'fresh-install',
    operation_id: journal.operation_id,
    created_at: journal.created_at,
    updated_at: journal.updated_at,
    state: journal.state,
    release_identity: releaseIdentity,
    pending_effect: pendingEffect,
    completed_effects: Object.freeze(completedEffects),
    terminal,
  })
}

/** Create the initial planned state. All identifiers and times are caller supplied. */
export function createFreshInstallJournal(input) {
  assertDataObject(input, 'journal_input', [
    'operation_id',
    'created_at',
    'release_identity',
  ])
  assertIdentifier(input.operation_id, 'journal_input.operation_id')
  assertCanonicalTimestamp(input.created_at, 'journal_input.created_at')
  const releaseIdentity = validateReleaseIdentity(input.release_identity)

  return validateFreshInstallJournal({
    revision: INSTALLATION_STATE_REVISION,
    operation: 'fresh-install',
    operation_id: input.operation_id,
    created_at: input.created_at,
    updated_at: input.created_at,
    state: 'planned',
    release_identity: releaseIdentity,
    pending_effect: null,
    completed_effects: [],
    terminal: null,
  })
}

/** Persist-before-effect contract: declare exactly one legal pending transition. */
export function beginFreshInstallEffect(journal, command) {
  const snapshot = validateFreshInstallJournal(journal)
  assertDataObject(command, 'transition', ['effect_id', 'next_state', 'started_at'], transitionError)
  assertIdentifier(command.effect_id, 'transition.effect_id', transitionError)
  assertOneOf(command.next_state, 'transition.next_state', FRESH_INSTALL_PROGRESS_STATES, transitionError)
  assertCanonicalTimestamp(command.started_at, 'transition.started_at', transitionError)

  if (FRESH_INSTALL_TERMINAL_STATES.includes(snapshot.state) || snapshot.state === 'committed') {
    transitionError(TERMINAL_JOURNAL, 'a terminal fresh-install journal cannot start another effect')
  }
  if (snapshot.pending_effect !== null) {
    transitionError(PENDING_EFFECT_EXISTS, 'the journal already has a pending effect')
  }
  if (effectIds(snapshot).has(command.effect_id)) {
    transitionError(EFFECT_REPLAYED, 'effect_id has already been used by this journal')
  }
  const expectedNext = nextProgressState(snapshot.state)
  if (command.next_state !== expectedNext) {
    transitionError(ILLEGAL_TRANSITION, `${snapshot.state} can only advance to ${expectedNext}`)
  }
  if (
    command.next_state === 'artifact_verified' &&
    snapshot.release_identity.verification_status !== 'verified'
  ) {
    transitionError(RELEASE_NOT_VERIFIED, 'structure_only release identity cannot begin artifact_verified')
  }
  assertTimeNotBefore(command.started_at, snapshot.updated_at, 'transition.started_at', transitionError)

  return validateFreshInstallJournal({
    ...snapshot,
    updated_at: command.started_at,
    pending_effect: {
      effect_id: command.effect_id,
      from_state: snapshot.state,
      to_state: command.next_state,
      started_at: command.started_at,
    },
  })
}

/** Complete only the currently declared effect; mismatches and replays fail closed. */
export function completeFreshInstallEffect(journal, command) {
  const snapshot = validateFreshInstallJournal(journal)
  assertDataObject(command, 'transition', ['effect_id', 'completed_at'], transitionError)
  assertIdentifier(command.effect_id, 'transition.effect_id', transitionError)
  assertCanonicalTimestamp(command.completed_at, 'transition.completed_at', transitionError)

  if (snapshot.pending_effect === null) {
    if (effectIds(snapshot).has(command.effect_id)) {
      transitionError(EFFECT_REPLAYED, 'effect_id has already reached a recorded outcome')
    }
    if (FRESH_INSTALL_TERMINAL_STATES.includes(snapshot.state) || snapshot.state === 'committed') {
      transitionError(TERMINAL_JOURNAL, 'a terminal fresh-install journal cannot complete another effect')
    }
    transitionError(PENDING_EFFECT_MISSING, 'the journal has no pending effect')
  }
  if (snapshot.pending_effect.effect_id !== command.effect_id) {
    transitionError(PENDING_EFFECT_MISMATCH, 'effect_id does not match the pending effect')
  }
  assertTimeNotBefore(command.completed_at, snapshot.pending_effect.started_at, 'transition.completed_at', transitionError)

  return validateFreshInstallJournal({
    ...snapshot,
    updated_at: command.completed_at,
    state: snapshot.pending_effect.to_state,
    pending_effect: null,
    completed_effects: [
      ...snapshot.completed_effects,
      {
        ...snapshot.pending_effect,
        completed_at: command.completed_at,
      },
    ],
  })
}

function recordTerminalEffect(journal, command, outcome) {
  const snapshot = validateFreshInstallJournal(journal)
  assertDataObject(
    command,
    'terminal_transition',
    ['effect_id', 'recorded_at', 'reason_code'],
    transitionError
  )
  assertIdentifier(command.effect_id, 'terminal_transition.effect_id', transitionError)
  assertCanonicalTimestamp(command.recorded_at, 'terminal_transition.recorded_at', transitionError)
  const expectedReasonCode = outcome === 'failed'
    ? FRESH_INSTALL_TERMINAL_REASON_CODES.FAILED
    : FRESH_INSTALL_TERMINAL_REASON_CODES.OUTCOME_UNKNOWN
  if (command.reason_code !== expectedReasonCode) {
    transitionError(INVALID_VALUE, `terminal_transition.reason_code must be ${expectedReasonCode}`)
  }
  if (snapshot.pending_effect === null) {
    if (effectIds(snapshot).has(command.effect_id)) {
      transitionError(EFFECT_REPLAYED, 'effect_id has already reached a recorded outcome')
    }
    if (FRESH_INSTALL_TERMINAL_STATES.includes(snapshot.state) || snapshot.state === 'committed') {
      transitionError(TERMINAL_JOURNAL, 'a terminal fresh-install journal cannot record another outcome')
    }
    transitionError(PENDING_EFFECT_MISSING, 'the journal has no pending effect')
  }
  if (snapshot.pending_effect.effect_id !== command.effect_id) {
    transitionError(PENDING_EFFECT_MISMATCH, 'effect_id does not match the pending effect')
  }
  assertTimeNotBefore(command.recorded_at, snapshot.pending_effect.started_at, 'terminal_transition.recorded_at', transitionError)

  return validateFreshInstallJournal({
    ...snapshot,
    updated_at: command.recorded_at,
    state: outcome,
    pending_effect: null,
    terminal: {
      outcome,
      ...snapshot.pending_effect,
      recorded_at: command.recorded_at,
      reason_code: command.reason_code,
    },
  })
}

/** Record a known failure. This is terminal and never retries the effect. */
export function failFreshInstallEffect(journal, command) {
  return recordTerminalEffect(journal, command, 'failed')
}

/** Record an indeterminate external outcome. This is terminal and never retries. */
export function markFreshInstallOutcomeUnknown(journal, command) {
  return recordTerminalEffect(journal, command, 'outcome_unknown')
}

function sameReleaseIdentity(left, right) {
  return (
    left.manifest_schema_version === right.manifest_schema_version &&
    left.verification_status === right.verification_status &&
    left.product.name === right.product.name &&
    left.product.version === right.product.version &&
    left.product.source_sha === right.product.source_sha &&
    left.product.build_revision === right.product.build_revision &&
    left.compatibility.config_schema === right.compatibility.config_schema &&
    left.compatibility.data_schema === right.compatibility.data_schema &&
    RELEASE_ARTIFACT_KEYS.every((key) => left.artifact[key] === right.artifact[key])
  )
}

/** Validate a revision-1 fresh-install record and optionally bind it to an expected artifact. */
export function validateCommittedInstallationRecord(record, options = {}) {
  assertOptionalDataObject(options, 'record_options', ['expected_release_identity'])
  assertDataObject(record, 'installation_record', [
    'revision',
    'installation_id',
    'operation',
    'operation_id',
    'committed_at',
    'release_identity',
    'access',
  ])
  if (record.revision !== INSTALLATION_STATE_REVISION) {
    validationError(INVALID_VALUE, `installation_record.revision must be ${INSTALLATION_STATE_REVISION}`)
  }
  if (record.operation !== 'fresh-install') {
    validationError(INVALID_VALUE, 'installation_record.operation must be fresh-install')
  }
  assertIdentifier(record.installation_id, 'installation_record.installation_id')
  assertIdentifier(record.operation_id, 'installation_record.operation_id')
  assertCanonicalTimestamp(record.committed_at, 'installation_record.committed_at')
  const releaseIdentity = validateReleaseIdentity(record.release_identity)
  if (releaseIdentity.verification_status !== 'verified') {
    validationError(RELEASE_NOT_VERIFIED, 'a committed installation record requires a verified fixture identity')
  }

  assertDataObject(record.access, 'installation_record.access', [
    'desired_access_mode',
    'actual_listen',
    'externally_verified_reachability',
  ])
  if (
    record.access.desired_access_mode !== 'local' ||
    record.access.actual_listen !== 'not_observed' ||
    record.access.externally_verified_reachability !== 'not_checked'
  ) {
    validationError(
      INVALID_VALUE,
      'revision-1 installation records only support local/not_observed/not_checked access'
    )
  }

  if (Object.hasOwn(options, 'expected_release_identity')) {
    const expectedReleaseIdentity = validateReleaseIdentity(options.expected_release_identity)
    if (!sameReleaseIdentity(releaseIdentity, expectedReleaseIdentity)) {
      validationError(RELEASE_IDENTITY_MISMATCH, 'installation record does not match the expected release artifact')
    }
  }

  return Object.freeze({
    revision: INSTALLATION_STATE_REVISION,
    installation_id: record.installation_id,
    operation: 'fresh-install',
    operation_id: record.operation_id,
    committed_at: record.committed_at,
    release_identity: releaseIdentity,
    access: Object.freeze({
      desired_access_mode: 'local',
      actual_listen: 'not_observed',
      externally_verified_reachability: 'not_checked',
    }),
  })
}

/** Build a committed record only from a matching, fully committed journal. */
export function buildCommittedInstallationRecord(input) {
  assertDataObject(input, 'record_input', [
    'installation_id',
    'journal',
    'release_identity',
  ])
  assertIdentifier(input.installation_id, 'record_input.installation_id')
  const journal = validateFreshInstallJournal(input.journal)
  const releaseIdentity = validateReleaseIdentity(input.release_identity)
  if (journal.state !== 'committed' || journal.pending_effect !== null || journal.terminal !== null) {
    validationError(INVALID_JOURNAL, 'installation record requires a committed fresh-install journal')
  }
  if (!sameReleaseIdentity(journal.release_identity, releaseIdentity)) {
    validationError(RELEASE_IDENTITY_MISMATCH, 'journal and installation record release identities differ')
  }

  return validateCommittedInstallationRecord(
    {
      revision: INSTALLATION_STATE_REVISION,
      installation_id: input.installation_id,
      operation: 'fresh-install',
      operation_id: journal.operation_id,
      committed_at: journal.updated_at,
      release_identity: releaseIdentity,
      access: {
        desired_access_mode: 'local',
        actual_listen: 'not_observed',
        externally_verified_reachability: 'not_checked',
      },
    },
    { expected_release_identity: journal.release_identity }
  )
}

function caseFoldPath(value) {
  return value.toLowerCase()
}

function pathRoot(value) {
  return value.split('/', 1)[0].toLowerCase()
}

function validateManagedRelativePath(value, path, { ownedOnly = false, releaseOnly = false } = {}) {
  if (
    typeof value !== 'string' ||
    value.length === 0 ||
    value.length > MAX_OWNED_PATH_LENGTH ||
    value.trim() !== value ||
    CONTROL_CHARACTER_PATTERN.test(value) ||
    !/^[\x21-\x7e]+$/.test(value) ||
    value.includes('\\') ||
    value.startsWith('/') ||
    value.startsWith('//') ||
    /^[A-Za-z]:/.test(value) ||
    value.includes('//') ||
    value.endsWith('/')
  ) {
    cleanupError(UNSAFE_OWNED_PATH, `${path} must be a bounded lexical relative path using forward slashes`)
  }

  const segments = value.split('/')
  const minimumDepth = ownedOnly ? 2 : 1
  if (segments.length < minimumDepth || segments.length > MAX_PATH_DEPTH) {
    cleanupError(UNSAFE_OWNED_PATH, `${path} must contain a managed root and at most ${MAX_PATH_DEPTH} segments`)
  }
  for (const segment of segments) {
    if (
      segment === '.' ||
      segment === '..' ||
      segment.length > MAX_PATH_SEGMENT_LENGTH ||
      !PORTABLE_PATH_SEGMENT_PATTERN.test(segment) ||
      segment.endsWith('.') ||
      WINDOWS_RESERVED_FILE_BASENAMES.has(segment.split('.', 1)[0].toLowerCase())
    ) {
      cleanupError(UNSAFE_OWNED_PATH, `${path} contains an unsafe or non-portable segment`)
    }
  }

  const root = pathRoot(value)
  if (PRESERVED_INSTALLATION_ROOTS.includes(root)) {
    cleanupError(PROTECTED_PATH, `${path} is inside the protected ${root} root`)
  }
  if (ownedOnly && (!OWNED_ROOTS.includes(root) || segments[0] !== root)) {
    cleanupError(PATH_OUTSIDE_OWNED_ROOTS, `${path} must be inside canonical releases/ or staging/`)
  }
  if (releaseOnly && (root !== 'releases' || segments[0] !== 'releases')) {
    cleanupError(PROTECTED_PATH, `${path} must identify a canonical current releases/ path`)
  }
  return value
}

/**
 * Validate only the lexical shape and managed-root boundary. This does not
 * inspect the filesystem and cannot prove file type, ownership, or containment.
 */
export function validateOwnedRelativePath(value) {
  return validateManagedRelativePath(value, 'owned_path', { ownedOnly: true })
}

function assertUniquePaths(paths, path) {
  const folded = new Set()
  for (const value of paths) {
    const key = caseFoldPath(value)
    if (folded.has(key)) {
      cleanupError(DUPLICATE_PATH, `${path} contains a case-fold duplicate`)
    }
    folded.add(key)
  }
}

function createPathTrie() {
  return {
    children: new Map(),
    terminal: null,
    representative: null,
  }
}

function addPathToTrie(root, path) {
  let node = root
  if (node.representative === null) node.representative = path
  for (const segment of caseFoldPath(path).split('/')) {
    let child = node.children.get(segment)
    if (child === undefined) {
      child = createPathTrie()
      node.children.set(segment, child)
    }
    node = child
    if (node.representative === null) node.representative = path
  }
  node.terminal = path
}

function findPathTrieOverlap(root, path) {
  let node = root
  for (const segment of caseFoldPath(path).split('/')) {
    if (node.terminal !== null) return node.terminal
    node = node.children.get(segment)
    if (node === undefined) return undefined
  }
  return node.terminal ?? node.representative ?? undefined
}

function stableSortedPaths(values) {
  return values
    .map((value) => ({
      value,
      key: caseFoldPath(typeof value === 'string' ? value : value.path),
    }))
    .sort((left, right) => {
      if (left.key === right.key) return 0
      return left.key < right.key ? -1 : 1
    })
    .map(({ value }) => value)
}

/**
 * Produce a post-commit lexical cleanup plan only. The journal, committed
 * record, and ownership document must bind the same operation, installation,
 * and release. Every candidate must be an explicit regular file under
 * releases/ or staging/. A future executor must independently inspect and
 * revalidate file type, containment, ownership, current pointers, and locks
 * before any deletion.
 */
export function planOwnedFileCleanup(input) {
  assertDataObject(
    input,
    'cleanup_input',
    [
      'journal',
      'installation_record',
      'ownership_document',
      'current_paths',
      'protected_paths',
    ],
    cleanupError
  )
  assertDataObject(
    input.ownership_document,
    'cleanup_input.ownership_document',
    [
      'revision',
      'installation_id',
      'operation_id',
      'release_identity',
      'entries',
    ],
    cleanupError
  )
  if (input.ownership_document.revision !== 1) {
    cleanupError(INVALID_VALUE, 'cleanup_input.ownership_document.revision must be 1')
  }
  assertIdentifier(
    input.ownership_document.installation_id,
    'cleanup_input.ownership_document.installation_id',
    cleanupError
  )
  assertIdentifier(
    input.ownership_document.operation_id,
    'cleanup_input.ownership_document.operation_id',
    cleanupError
  )

  let journal
  let installationRecord
  let ownershipReleaseIdentity
  try {
    journal = validateFreshInstallJournal(input.journal)
    installationRecord = validateCommittedInstallationRecord(
      input.installation_record,
      { expected_release_identity: journal.release_identity }
    )
    ownershipReleaseIdentity = validateReleaseIdentity(
      input.ownership_document.release_identity
    )
  } catch (error) {
    if (error instanceof InstallationStateValidationError) {
      const code = error.code === RELEASE_IDENTITY_MISMATCH
        ? CLEANUP_BINDING_MISMATCH
        : error.code
      cleanupError(code, `cleanup authorization is invalid: ${error.message}`)
    }
    throw error
  }
  if (
    journal.state !== 'committed' ||
    journal.pending_effect !== null ||
    journal.terminal !== null
  ) {
    cleanupError(
      CLEANUP_NOT_COMMITTED,
      'cleanup candidates require a health-checked and committed fresh-install journal'
    )
  }
  if (
    installationRecord.operation_id !== journal.operation_id ||
    installationRecord.committed_at !== journal.updated_at ||
    input.ownership_document.installation_id !== installationRecord.installation_id ||
    input.ownership_document.operation_id !== journal.operation_id ||
    !sameReleaseIdentity(ownershipReleaseIdentity, installationRecord.release_identity)
  ) {
    cleanupError(
      CLEANUP_BINDING_MISMATCH,
      'journal, installation record, and ownership document must bind the same committed installation and release'
    )
  }
  assertDataArray(
    input.ownership_document.entries,
    'cleanup_input.ownership_document.entries',
    0,
    MAX_OWNED_ENTRIES,
    cleanupError
  )
  assertDataArray(input.current_paths, 'cleanup_input.current_paths', 0, MAX_BOUNDARY_PATHS, cleanupError)
  assertDataArray(input.protected_paths, 'cleanup_input.protected_paths', 0, MAX_BOUNDARY_PATHS, cleanupError)

  const suppliedCurrentPaths = input.current_paths.map((value, index) =>
    validateManagedRelativePath(value, `cleanup_input.current_paths[${index}]`, { releaseOnly: true })
  )
  const protectedPaths = input.protected_paths.map((value, index) =>
    validateManagedRelativePath(value, `cleanup_input.protected_paths[${index}]`)
  )
  assertUniquePaths(suppliedCurrentPaths, 'cleanup_input.current_paths')
  assertUniquePaths(protectedPaths, 'cleanup_input.protected_paths')
  const recordedCurrentPath = validateManagedRelativePath(
    `releases/${installationRecord.release_identity.product.version}/${installationRecord.release_identity.artifact.id}`,
    'installation_record derived current release path',
    { releaseOnly: true }
  )
  const currentPaths = suppliedCurrentPaths.some(
    (value) => caseFoldPath(value) === caseFoldPath(recordedCurrentPath)
  )
    ? suppliedCurrentPaths
    : [...suppliedCurrentPaths, recordedCurrentPath]
  if (currentPaths.length > MAX_BOUNDARY_PATHS) {
    cleanupError(
      INVALID_ARRAY,
      `cleanup_input.current_paths plus the recorded current release must not exceed ${MAX_BOUNDARY_PATHS} entries`
    )
  }
  assertUniquePaths(
    [...currentPaths, ...protectedPaths],
    'cleanup_input current/protected boundaries'
  )

  const candidates = input.ownership_document.entries.map((entry, index) => {
    const path = `cleanup_input.ownership_document.entries[${index}]`
    assertDataObject(entry, path, ['path', 'kind'], cleanupError)
    if (entry.kind !== 'regular_file') {
      cleanupError(
        UNSUPPORTED_ENTRY_KIND,
        `${path}.kind must be regular_file; symlink, reparse, directory, and unknown kinds are refused`
      )
    }
    return Object.freeze({
      path: validateManagedRelativePath(entry.path, `${path}.path`, { ownedOnly: true }),
      kind: 'regular_file',
    })
  })
  assertUniquePaths(candidates.map((candidate) => candidate.path), 'cleanup_input.ownership_document.entries')

  const candidateTrie = createPathTrie()
  for (const candidate of candidates) {
    const overlap = findPathTrieOverlap(candidateTrie, candidate.path)
    if (overlap !== undefined) {
      cleanupError(
        CLEANUP_CANDIDATE_OVERLAP,
        `cleanup candidates ${candidate.path} and ${overlap} overlap lexically`
      )
    }
    addPathToTrie(candidateTrie, candidate.path)
  }

  const boundaryTrie = createPathTrie()
  for (const boundary of [...currentPaths, ...protectedPaths]) {
    addPathToTrie(boundaryTrie, boundary)
  }
  for (const candidate of candidates) {
    const boundary = findPathTrieOverlap(boundaryTrie, candidate.path)
    if (boundary !== undefined) {
      cleanupError(
        CLEANUP_BOUNDARY_CONFLICT,
        `cleanup candidate ${candidate.path} overlaps protected/current boundary ${boundary}`
      )
    }
  }

  return Object.freeze({
    revision: 1,
    plan_type: 'post_commit_lexical_cleanup_only',
    installation_id: installationRecord.installation_id,
    operation_id: journal.operation_id,
    release_identity: ownershipReleaseIdentity,
    preservation_roots: PRESERVED_INSTALLATION_ROOTS,
    current_paths: Object.freeze(stableSortedPaths(currentPaths)),
    protected_paths: Object.freeze(stableSortedPaths(protectedPaths)),
    candidates: Object.freeze(stableSortedPaths(candidates)),
  })
}
