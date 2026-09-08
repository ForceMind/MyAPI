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
 * The initial manifest format described in docs/RELEASE_MANIFEST.md.
 *
 * This module deliberately does not fetch, parse, write, or verify a
 * signature. It validates data which was already obtained from a trusted
 * manifest verification boundary, snapshots its schema-1 fields, then makes
 * a fail-closed local choice. Structural validation is never a substitute
 * for canonical-byte signature verification and trust-root policy.
 */
import { types } from 'node:util'

import {
  isPinnedHTTPSURL,
  parseCanonicalOCIReference,
} from './canonical-artifact-reference.mjs'

export const RELEASE_MANIFEST_SCHEMA_VERSION = 1

export const RELEASE_EDITIONS = Object.freeze(['full', 'lite'])
export const INSTALLATION_SHAPES = Object.freeze([
  'server-native',
  'server-container',
  'personal-native',
  'personal-container',
  'desktop',
])
export const RELEASE_OPERATING_SYSTEMS = Object.freeze(['linux', 'darwin', 'win32'])
export const RELEASE_ARCHITECTURES = Object.freeze(['amd64', 'arm64'])
export const RELEASE_ARTIFACT_KINDS = Object.freeze(['oci', 'binary', 'desktop'])
export const RELEASE_SELECTION_OPERATIONS = Object.freeze(['fresh-install', 'upgrade', 'switch', 'rollback'])

const SHA256_PATTERN = /^[a-fA-F0-9]{64}$/
const GIT_REVISION_PATTERN = /^(?:[a-fA-F0-9]{40}|[a-fA-F0-9]{64})$/
const OCI_DIGEST_PATTERN = /^sha256:[a-f0-9]{64}$/
const PORTABLE_ARTIFACT_ID_PATTERN = /^[a-z0-9](?:[a-z0-9._-]{0,127})?$/
const SEMVER_PATTERN = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$/
const MAX_RELEASE_REFERENCE_LENGTH = 2048
const MAX_SEMVER_LENGTH = 256
const MAX_ARTIFACTS = 256
const MAX_SUPPORTED_FROM = 128
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

export class ReleaseManifestValidationError extends Error {
  constructor(code, message) {
    super(message)
    this.name = 'ReleaseManifestValidationError'
    this.code = code
  }
}

export class ReleaseManifestCompatibilityError extends Error {
  constructor(code, message) {
    super(message)
    this.name = 'ReleaseManifestCompatibilityError'
    this.code = code
  }
}

export class ReleaseManifestSelectionError extends Error {
  constructor(code, message) {
    super(message)
    this.name = 'ReleaseManifestSelectionError'
    this.code = code
  }
}

function validationError(code, message) {
  throw new ReleaseManifestValidationError(code, message)
}

function compatibilityError(code, message) {
  throw new ReleaseManifestCompatibilityError(code, message)
}

function selectionError(code, message) {
  throw new ReleaseManifestSelectionError(code, message)
}

function assertDataObject(value, path, keys, fail = validationError, options = {}) {
  const { requireAll = true, invalidObjectCode = 'invalid_object', invalidFieldsCode = 'invalid_fields' } = options
  if (
    value === null ||
    typeof value !== 'object' ||
    types.isProxy(value) ||
    Array.isArray(value)
  ) {
    fail(invalidObjectCode, `${path} must be a plain data object`)
  }
  const prototype = Object.getPrototypeOf(value)
  if (prototype !== Object.prototype && prototype !== null) {
    fail(invalidObjectCode, `${path} must be a plain data object`)
  }

  const actualKeys = Reflect.ownKeys(value)
  if (actualKeys.some((key) => typeof key !== 'string')) {
    fail(invalidFieldsCode, `${path} must not contain symbol fields`)
  }
  for (const key of actualKeys) {
    const descriptor = Object.getOwnPropertyDescriptor(value, key)
    if (!descriptor?.enumerable || !Object.hasOwn(descriptor, 'value')) {
      fail(invalidFieldsCode, `${path}.${key} must be an enumerable data field`)
    }
  }

  const unexpected = actualKeys.filter((key) => !keys.includes(key))
  const missing = requireAll ? keys.filter((key) => !Object.hasOwn(value, key)) : []
  if (missing.length > 0 || unexpected.length > 0) {
    const details = [
      missing.length > 0 ? `missing ${missing.join(', ')}` : '',
      unexpected.length > 0 ? `${unexpected.length} unsupported field(s)` : '',
    ]
      .filter(Boolean)
      .join('; ')
    fail(invalidFieldsCode, `${path} has invalid fields: ${details}`)
  }
}

function assertDataArray(value, path, minimum, maximum, code) {
  if (
    types.isProxy(value) ||
    !Array.isArray(value) ||
    Object.getPrototypeOf(value) !== Array.prototype
  ) {
    validationError(code, `${path} must be a plain data array`)
  }
  if (value.length < minimum || value.length > maximum) {
    validationError(code, `${path} contains an unsupported number of entries`)
  }

  const ownKeys = Reflect.ownKeys(value)
  for (const key of ownKeys) {
    if (key === 'length') continue
    if (
      typeof key !== 'string' ||
      !/^(?:0|[1-9]\d*)$/.test(key) ||
      Number(key) >= value.length
    ) {
      validationError(code, `${path} must not contain non-index fields`)
    }
    const descriptor = Object.getOwnPropertyDescriptor(value, key)
    if (!descriptor?.enumerable || !Object.hasOwn(descriptor, 'value')) {
      validationError(code, `${path}[${key}] must be an enumerable data entry`)
    }
  }
  for (let index = 0; index < value.length; index += 1) {
    if (!Object.hasOwn(value, index)) {
      validationError(code, `${path} must not contain sparse entries`)
    }
  }
}

function assertNonEmptyString(value, path) {
  if (typeof value !== 'string' || value.trim() === '') {
    validationError('invalid_string', `${path} must be a non-empty string`)
  }
}

function assertSafeOpaqueReference(value, path) {
  assertNonEmptyString(value, path)
  if (
    value.length > MAX_RELEASE_REFERENCE_LENGTH ||
    !/^[\x21-\x7e]+$/.test(value)
  ) {
    validationError('invalid_reference', `${path} must be a bounded visible ASCII reference`)
  }
}

function assertPinnedHttpsURLShape(value, path) {
  assertSafeOpaqueReference(value, path)
  if (!isPinnedHTTPSURL(value)) {
    validationError('invalid_location', `${path} must be a canonical pinned HTTPS URL`)
  }
}

function assertPortableArtifactID(value, path) {
  assertNonEmptyString(value, path)
  if (!PORTABLE_ARTIFACT_ID_PATTERN.test(value) || value.endsWith('.')) {
    validationError('invalid_artifact_id', `${path} contains unsupported characters`)
  }
  const baseName = value.split('.', 1)[0]
  if (WINDOWS_RESERVED_FILE_BASENAMES.has(baseName)) {
    validationError('invalid_artifact_id', `${path} is reserved on Windows`)
  }
}

function assertOneOf(value, path, values) {
  if (!values.includes(value)) {
    validationError('invalid_value', `${path} must be one of ${values.join(', ')}`)
  }
}

function assertSelectionOneOf(value, path, values) {
  if (!values.includes(value)) {
    selectionError('invalid_target', `${path} must be one of ${values.join(', ')}`)
  }
}

function assertSemver(value, path) {
  if (
    typeof value !== 'string' ||
    value.length > MAX_SEMVER_LENGTH ||
    !SEMVER_PATTERN.test(value)
  ) {
    validationError('invalid_version', `${path} must be a semantic version without a v prefix`)
  }
}

function parseSemver(value) {
  const match = SEMVER_PATTERN.exec(value)
  if (!match) return null
  return {
    major: match[1],
    minor: match[2],
    patch: match[3],
    prerelease: match[4] ? match[4].split('.') : [],
  }
}

function compareNumericIdentifiers(left, right) {
  if (left.length !== right.length) return left.length > right.length ? 1 : -1
  if (left === right) return 0
  return left > right ? 1 : -1
}

/**
 * Compare valid semantic versions. Build metadata is intentionally ignored,
 * following SemVer precedence rules.
 */
export function compareSemver(left, right) {
  const leftVersion = parseSemver(left)
  const rightVersion = parseSemver(right)
  if (!leftVersion || !rightVersion) {
    throw new TypeError('compareSemver requires valid semantic versions without v prefixes')
  }

  for (const key of ['major', 'minor', 'patch']) {
    if (leftVersion[key] !== rightVersion[key]) {
      return compareNumericIdentifiers(leftVersion[key], rightVersion[key])
    }
  }
  if (leftVersion.prerelease.length === 0 || rightVersion.prerelease.length === 0) {
    if (leftVersion.prerelease.length === rightVersion.prerelease.length) return 0
    return leftVersion.prerelease.length === 0 ? 1 : -1
  }
  const length = Math.max(leftVersion.prerelease.length, rightVersion.prerelease.length)
  for (let index = 0; index < length; index += 1) {
    const leftIdentifier = leftVersion.prerelease[index]
    const rightIdentifier = rightVersion.prerelease[index]
    if (leftIdentifier === undefined) return -1
    if (rightIdentifier === undefined) return 1
    if (leftIdentifier === rightIdentifier) continue
    const leftNumeric = /^\d+$/.test(leftIdentifier)
    const rightNumeric = /^\d+$/.test(rightIdentifier)
    if (leftNumeric && rightNumeric) {
      return compareNumericIdentifiers(leftIdentifier, rightIdentifier)
    }
    if (leftNumeric !== rightNumeric) return leftNumeric ? -1 : 1
    return leftIdentifier > rightIdentifier ? 1 : -1
  }
  return 0
}

function validateLocation(artifact, path) {
  if (artifact.kind === 'oci') {
    assertSafeOpaqueReference(artifact.location, `${path}.location`)
    const location = parseCanonicalOCIReference(artifact.location)
    if (!location) {
      validationError('invalid_location', `${path}.location must be a canonical immutable OCI reference`)
    }
    if (location.digest !== artifact.sha256_or_digest) {
      validationError('digest_mismatch', `${path}.location digest must match sha256_or_digest`)
    }
    return
  }

  assertPinnedHttpsURLShape(artifact.location, `${path}.location`)
}

function validateArtifact(artifact, index) {
  const path = `artifacts[${index}]`
  assertDataObject(artifact, path, [
    'id',
    'edition',
    'installation_shape',
    'os',
    'arch',
    'kind',
    'location',
    'sha256_or_digest',
    'signature',
    'sbom_or_provenance',
  ])
  assertPortableArtifactID(artifact.id, `${path}.id`)
  assertOneOf(artifact.edition, `${path}.edition`, RELEASE_EDITIONS)
  assertOneOf(artifact.installation_shape, `${path}.installation_shape`, INSTALLATION_SHAPES)
  assertOneOf(artifact.os, `${path}.os`, RELEASE_OPERATING_SYSTEMS)
  assertOneOf(artifact.arch, `${path}.arch`, RELEASE_ARCHITECTURES)
  assertOneOf(artifact.kind, `${path}.kind`, RELEASE_ARTIFACT_KINDS)

  const allowedKind = {
    'server-native': 'binary',
    'server-container': 'oci',
    'personal-native': 'binary',
    'personal-container': 'oci',
    desktop: 'desktop',
  }[artifact.installation_shape]
  if (artifact.kind !== allowedKind) {
    validationError('invalid_artifact_kind', `${path}.kind does not match installation_shape`)
  }
  if (artifact.edition === 'full' && !artifact.installation_shape.startsWith('server-')) {
    validationError('invalid_artifact_shape', `${path}.edition full only supports server installation shapes`)
  }
  if (artifact.installation_shape === 'desktop' && artifact.edition !== 'lite') {
    validationError('invalid_artifact_shape', `${path}.edition desktop must be lite`)
  }

  assertNonEmptyString(artifact.sha256_or_digest, `${path}.sha256_or_digest`)
  const validHash = SHA256_PATTERN.test(artifact.sha256_or_digest)
  const validDigest = OCI_DIGEST_PATTERN.test(artifact.sha256_or_digest)
  if (!validHash && !validDigest) {
    validationError('invalid_sha256', `${path}.sha256_or_digest must be a SHA-256 hash or sha256 digest`)
  }
  if (artifact.kind === 'oci' && !validDigest) {
    validationError('invalid_sha256', `${path}.sha256_or_digest must be an OCI sha256 digest`)
  }
  if (artifact.kind !== 'oci' && !validHash) {
    validationError('invalid_sha256', `${path}.sha256_or_digest must be a SHA-256 hash`)
  }
  validateLocation(artifact, path)
  assertSafeOpaqueReference(artifact.signature, `${path}.signature`)
  assertPinnedHttpsURLShape(artifact.sbom_or_provenance, `${path}.sbom_or_provenance`)
}

function snapshotManifest(manifest) {
  return Object.freeze({
    schema_version: manifest.schema_version,
    product: Object.freeze({ ...manifest.product }),
    bootstrap: Object.freeze({ ...manifest.bootstrap }),
    compatibility: Object.freeze({
      ...manifest.compatibility,
      supported_from: Object.freeze([...manifest.compatibility.supported_from]),
    }),
    artifacts: Object.freeze(manifest.artifacts.map((artifact) => Object.freeze({ ...artifact }))),
  })
}

/**
 * Validate the complete schema-1 release manifest. The input is never
 * modified. Signature cryptography remains intentionally outside this pure
 * structural validator.
 */
export function validateReleaseManifest(manifest) {
  assertDataObject(manifest, 'manifest', [
    'schema_version',
    'product',
    'bootstrap',
    'compatibility',
    'artifacts',
  ])
  if (manifest.schema_version !== RELEASE_MANIFEST_SCHEMA_VERSION) {
    validationError(
      'unsupported_schema_version',
      `manifest.schema_version must be ${RELEASE_MANIFEST_SCHEMA_VERSION}`
    )
  }

  assertDataObject(manifest.product, 'manifest.product', [
    'name',
    'version',
    'source_sha',
    'build_revision',
  ])
  if (manifest.product.name !== 'My API') {
    validationError('invalid_product', 'manifest.product.name must be My API')
  }
  assertSemver(manifest.product.version, 'manifest.product.version')
  if (typeof manifest.product.source_sha !== 'string' || !GIT_REVISION_PATTERN.test(manifest.product.source_sha)) {
    validationError('invalid_source_sha', 'manifest.product.source_sha must be a 40 or 64 character Git revision hash')
  }
  if (typeof manifest.product.build_revision !== 'string' || !GIT_REVISION_PATTERN.test(manifest.product.build_revision)) {
    validationError('invalid_build_revision', 'manifest.product.build_revision must be a Git revision hash')
  }

  assertDataObject(manifest.bootstrap, 'manifest.bootstrap', ['min_version', 'max_version'])
  assertSemver(manifest.bootstrap.min_version, 'manifest.bootstrap.min_version')
  assertSemver(manifest.bootstrap.max_version, 'manifest.bootstrap.max_version')
  if (compareSemver(manifest.bootstrap.min_version, manifest.bootstrap.max_version) > 0) {
    validationError('invalid_bootstrap_range', 'manifest.bootstrap.min_version must not exceed max_version')
  }

  assertDataObject(manifest.compatibility, 'manifest.compatibility', [
    'config_schema',
    'data_schema',
    'supported_from',
    'rollback',
  ])
  assertSemver(manifest.compatibility.config_schema, 'manifest.compatibility.config_schema')
  assertSemver(manifest.compatibility.data_schema, 'manifest.compatibility.data_schema')
  assertDataArray(
    manifest.compatibility.supported_from,
    'manifest.compatibility.supported_from',
    1,
    MAX_SUPPORTED_FROM,
    'invalid_supported_from'
  )
  const supportedVersions = new Set()
  for (const [index, version] of manifest.compatibility.supported_from.entries()) {
    assertSemver(version, `manifest.compatibility.supported_from[${index}]`)
    if (compareSemver(version, manifest.product.version) > 0) {
      validationError('invalid_supported_from', 'manifest.compatibility.supported_from cannot include a future version')
    }
    if (supportedVersions.has(version)) {
      validationError('invalid_supported_from', 'manifest.compatibility.supported_from must not contain duplicates')
    }
    supportedVersions.add(version)
  }
  assertOneOf(manifest.compatibility.rollback, 'manifest.compatibility.rollback', [
    'code_only',
    'database_restore',
    'required_manual',
  ])

  assertDataArray(manifest.artifacts, 'manifest.artifacts', 1, MAX_ARTIFACTS, 'invalid_artifacts')
  const artifactIds = new Set()
  for (const [index, artifact] of manifest.artifacts.entries()) {
    validateArtifact(artifact, index)
    if (artifactIds.has(artifact.id)) {
      validationError('duplicate_artifact_id', `manifest.artifacts contains duplicate id ${artifact.id}`)
    }
    artifactIds.add(artifact.id)
  }

  return snapshotManifest(manifest)
}

/**
 * Verify that a local bootstrap can consume a structurally valid manifest for
 * a fresh installation. Schema 1 deliberately cannot authorize managed
 * upgrade, switch, or rollback: it has no signed transition matrix covering
 * the installed edition, shape, database engine, and persistent data state.
 */
export function assertReleaseManifestCompatibility(manifest, options = {}) {
  const validatedManifest = validateReleaseManifest(manifest)
  assertDataObject(
    options,
    'compatibility options',
    ['bootstrapVersion'],
    compatibilityError,
    {
      requireAll: false,
      invalidObjectCode: 'invalid_options',
      invalidFieldsCode: 'invalid_options',
    }
  )
  const { bootstrapVersion } = options
  if (
    typeof bootstrapVersion !== 'string' ||
    bootstrapVersion.length > MAX_SEMVER_LENGTH ||
    !SEMVER_PATTERN.test(bootstrapVersion)
  ) {
    compatibilityError('invalid_bootstrap_version', 'bootstrapVersion must be a semantic version without a v prefix')
  }
  if (
    compareSemver(bootstrapVersion, validatedManifest.bootstrap.min_version) < 0 ||
    compareSemver(bootstrapVersion, validatedManifest.bootstrap.max_version) > 0
  ) {
    compatibilityError('bootstrap_incompatible', 'local bootstrap version is outside the manifest-supported range')
  }

  return validatedManifest
}

/**
 * Choose exactly one artifact for an explicit fresh-install target. Managed
 * operations fail closed until a later manifest schema carries a verified
 * transition matrix and a durable installation record contract.
 */
export function selectReleaseArtifact(manifest, target) {
  const required = ['operation', 'edition', 'installationShape', 'os', 'arch', 'bootstrapVersion']
  const allowedTargetKeys = [...required, 'kind']
  assertDataObject(
    target,
    'artifact target',
    allowedTargetKeys,
    selectionError,
    {
      requireAll: false,
      invalidObjectCode: 'invalid_target',
      invalidFieldsCode: 'invalid_target',
    }
  )
  if (required.some((key) => !Object.hasOwn(target, key))) {
    selectionError('invalid_target', 'artifact target requires operation, edition, installationShape, os, arch, and bootstrapVersion')
  }
  assertSelectionOneOf(target.operation, 'target.operation', RELEASE_SELECTION_OPERATIONS)
  if (target.operation !== 'fresh-install') {
    selectionError('managed_operation_unsupported', 'schema 1 release selection supports fresh-install only')
  }
  const validatedManifest = assertReleaseManifestCompatibility(manifest, {
    bootstrapVersion: target.bootstrapVersion,
  })
  assertSelectionOneOf(target.edition, 'target.edition', RELEASE_EDITIONS)
  assertSelectionOneOf(target.installationShape, 'target.installationShape', INSTALLATION_SHAPES)
  assertSelectionOneOf(target.os, 'target.os', RELEASE_OPERATING_SYSTEMS)
  assertSelectionOneOf(target.arch, 'target.arch', RELEASE_ARCHITECTURES)
  if (target.kind !== undefined) assertSelectionOneOf(target.kind, 'target.kind', RELEASE_ARTIFACT_KINDS)

  const matches = validatedManifest.artifacts.filter(
    (artifact) =>
      artifact.edition === target.edition &&
      artifact.installation_shape === target.installationShape &&
      artifact.os === target.os &&
      artifact.arch === target.arch &&
      (target.kind === undefined || artifact.kind === target.kind)
  )
  if (matches.length === 0) {
    selectionError('artifact_not_found', 'manifest has no artifact matching the requested target')
  }
  if (matches.length > 1) {
    selectionError('artifact_ambiguous', 'manifest has multiple artifacts matching the requested target')
  }
  return matches[0]
}
