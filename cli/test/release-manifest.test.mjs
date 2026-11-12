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
  assertReleaseManifestCompatibility,
  compareSemver,
  ReleaseManifestCompatibilityError,
  ReleaseManifestSelectionError,
  ReleaseManifestValidationError,
  selectReleaseArtifact,
  selectNativeReleaseTransition,
  validateReleaseManifest,
} from '../lib/release-manifest.mjs'

const sha256 = 'a'.repeat(64)
const digest = `sha256:${'b'.repeat(64)}`

function manifest(overrides = {}) {
  const value = {
    schema_version: 1,
    product: {
      name: 'My API',
      version: '1.2.0',
      source_sha: 'a'.repeat(40),
      build_revision: '1234567890abcdef1234567890abcdef12345678',
    },
    bootstrap: {
      min_version: '1.0.0',
      max_version: '1.9.0',
    },
    compatibility: {
      config_schema: '2.0.0',
      data_schema: '3.0.0',
      supported_from: ['1.1.0', '1.2.0'],
      rollback: 'database_restore',
    },
    artifacts: [
      {
        id: 'myapi-lite-linux-amd64',
        edition: 'lite',
        installation_shape: 'personal-native',
        os: 'linux',
        arch: 'amd64',
        kind: 'binary',
        location: 'https://releases.example.test/myapi-lite-linux-amd64',
        sha256_or_digest: sha256,
        signature: 'sigstore-bundle',
        sbom_or_provenance: 'https://releases.example.test/myapi-lite-linux-amd64.intoto.jsonl',
      },
      {
        id: 'myapi-full-linux-amd64',
        edition: 'full',
        installation_shape: 'server-container',
        os: 'linux',
        arch: 'amd64',
        kind: 'oci',
        location: `ghcr.io/forcemind/myapi@${digest}`,
        sha256_or_digest: digest,
        signature: 'cosign-bundle',
        sbom_or_provenance: 'https://releases.example.test/myapi-full-linux-amd64.intoto.jsonl',
      },
    ],
  }
  return {
    ...value,
    ...overrides,
  }
}

function assertError(ErrorType, code, action) {
  assert.throws(action, (error) => error instanceof ErrorType && error.code === code)
}

function nativeTransitionFixture() {
  const older = manifest()
  const newer = manifest()
  newer.schema_version = 2
  newer.product.version = '1.3.0'
  newer.product.source_sha = 'c'.repeat(40)
  newer.artifacts[0].id = 'myapi-lite-linux-amd64-next'
  newer.artifacts[0].sha256_or_digest = 'd'.repeat(64)
  newer.compatibility.rollback = 'code_only'
  newer.native_transitions = [{
    from: { version: older.product.version, source_sha: older.product.source_sha,
      artifact_id: older.artifacts[0].id, sha256: older.artifacts[0].sha256_or_digest },
    to_artifact_id: newer.artifacts[0].id,
    storage: 'sqlite',
    allow_code_rollback: true,
  }]
  function identity(release) {
    return { version: release.product.version, sourceSha: release.product.source_sha,
      artifact: release.artifacts[0].id, sha256: release.artifacts[0].sha256_or_digest,
      edition: 'lite', scope: 'user', arch: 'amd64' }
  }
  return { older, newer, identity, request: { operation: 'upgrade', storage: 'sqlite', bootstrapVersion: '1.5.0' } }
}

test('plans native SQLite upgrade and explicitly authorized code-only rollback for the same schemas', () => {
  const { older, newer, identity, request } = nativeTransitionFixture()
  const plan = selectNativeReleaseTransition(older, newer, identity(older), request)
  const rollback = selectNativeReleaseTransition(newer, older, identity(newer), { ...request, operation: 'rollback' })
  assert.equal(plan.configSchema, '2.0.0')
  assert.equal(plan.dataSchema, '3.0.0')
  assert.equal(plan.current.sha256, older.artifacts[0].sha256_or_digest)
  assert.equal(plan.target.sha256, newer.artifacts[0].sha256_or_digest)
  assert.deepEqual(rollback.current, plan.target)
  assert.deepEqual(rollback.target, plan.current)
  assert.equal(rollback.operation, 'rollback')
  assert.ok(Object.isFrozen(plan) && Object.isFrozen(plan.current) && Object.isFrozen(plan.target))
})

test('schema 1 supported_from alone never authorizes a native transition', () => {
  const { older, newer, identity, request } = nativeTransitionFixture()
  newer.schema_version = 1
  delete newer.native_transitions
  assertError(ReleaseManifestCompatibilityError, 'native_transition_missing', () =>
    selectNativeReleaseTransition(older, newer, identity(older), request))
  assertError(ReleaseManifestCompatibilityError, 'native_transition_missing', () =>
    selectNativeReleaseTransition(newer, older, identity(newer), { ...request, operation: 'rollback' }))
})

test('rejects an installed identity or signed edge that does not match the exact binary', () => {
  const { older, newer, identity, request } = nativeTransitionFixture()
  for (const [key, value] of Object.entries({
    sha256: 'f'.repeat(64), sourceSha: 'e'.repeat(40), artifact: 'other-binary', version: '1.1.0',
  })) {
    assertError(ReleaseManifestCompatibilityError, 'installed_identity_mismatch', () =>
      selectNativeReleaseTransition(older, newer, { ...identity(older), [key]: value }, request))
  }
  newer.native_transitions[0].from.sha256 = 'f'.repeat(64)
  assertError(ReleaseManifestCompatibilityError, 'native_transition_missing', () =>
    selectNativeReleaseTransition(older, newer, identity(older), request))
})

test('rejects incompatible config and data schemas, non-SQLite and reversed version direction', () => {
  const { older, newer, identity, request } = nativeTransitionFixture()
  assertError(ReleaseManifestCompatibilityError, 'native_storage_unsupported', () =>
    selectNativeReleaseTransition(older, newer, identity(older), { ...request, storage: 'postgres' }))
  assertError(ReleaseManifestCompatibilityError, 'native_transition_direction_mismatch', () =>
    selectNativeReleaseTransition(newer, older, identity(newer), request))
  for (const key of ['config_schema', 'data_schema']) {
    const modified = { ...newer, compatibility: { ...newer.compatibility, [key]: '4.0.0' } }
    assertError(ReleaseManifestCompatibilityError, 'native_schema_mismatch', () =>
      selectNativeReleaseTransition(older, modified, identity(older), request))
    assertError(ReleaseManifestCompatibilityError, 'native_schema_mismatch', () =>
      selectNativeReleaseTransition(modified, older, identity(modified), { ...request, operation: 'rollback' }))
  }
})

test('rollback requires both code_only policy and exact transition authorization', () => {
  const { older, newer, identity, request } = nativeTransitionFixture()
  const rollback = { ...request, operation: 'rollback' }
  newer.native_transitions[0].allow_code_rollback = false
  assertError(ReleaseManifestCompatibilityError, 'native_rollback_forbidden', () =>
    selectNativeReleaseTransition(newer, older, identity(newer), rollback))
  newer.native_transitions[0].allow_code_rollback = true
  newer.compatibility.rollback = 'database_restore'
  assertError(ReleaseManifestCompatibilityError, 'native_rollback_forbidden', () =>
    selectNativeReleaseTransition(newer, older, identity(newer), rollback))
})

test('rejects a malformed or duplicate native transition even when another edge matches', () => {
  const { older, newer, identity, request } = nativeTransitionFixture()
  newer.native_transitions.push(structuredClone(newer.native_transitions[0]))
  assertError(ReleaseManifestValidationError, 'duplicate_native_transition', () =>
    selectNativeReleaseTransition(older, newer, identity(older), request))
  newer.native_transitions[1].to_artifact_id = newer.artifacts[1].id
  assertError(ReleaseManifestValidationError, 'invalid_native_transition', () =>
    selectNativeReleaseTransition(older, newer, identity(older), request))
})

test('rejects proxies and accessors without executing caller code', () => {
  assertError(ReleaseManifestValidationError, 'invalid_object', () =>
    validateReleaseManifest(new Proxy(manifest(), {}))
  )

  const revoked = Proxy.revocable(manifest(), {})
  revoked.revoke()
  assertError(ReleaseManifestValidationError, 'invalid_object', () =>
    validateReleaseManifest(revoked.proxy)
  )

  for (const trapName of ['get', 'getPrototypeOf', 'ownKeys', 'getOwnPropertyDescriptor']) {
    let trapExecuted = false
    const throwingProxy = new Proxy(manifest(), {
      [trapName]() {
        trapExecuted = true
        throw new Error(`${trapName} trap must not execute`)
      },
    })
    assertError(ReleaseManifestValidationError, 'invalid_object', () =>
      validateReleaseManifest(throwingProxy)
    )
    assert.equal(trapExecuted, false)
  }

  const accessorManifest = manifest()
  let getterExecuted = false
  Object.defineProperty(accessorManifest, 'product', {
    configurable: true,
    enumerable: true,
    get() {
      getterExecuted = true
      throw new Error('manifest getter must not execute')
    },
  })
  assertError(ReleaseManifestValidationError, 'invalid_fields', () =>
    validateReleaseManifest(accessorManifest)
  )
  assert.equal(getterExecuted, false)

  const setterManifest = manifest()
  let setterExecuted = false
  Object.defineProperty(setterManifest, 'bootstrap', {
    configurable: true,
    enumerable: true,
    set() {
      setterExecuted = true
    },
  })
  assertError(ReleaseManifestValidationError, 'invalid_fields', () =>
    validateReleaseManifest(setterManifest)
  )
  assert.equal(setterExecuted, false)

  const target = {
    operation: 'fresh-install',
    edition: 'lite',
    installationShape: 'personal-native',
    os: 'linux',
    arch: 'amd64',
    bootstrapVersion: '1.5.0',
  }
  assertError(ReleaseManifestSelectionError, 'invalid_target', () =>
    selectReleaseArtifact(manifest(), new Proxy(target, {}))
  )
  assertError(ReleaseManifestCompatibilityError, 'invalid_options', () =>
    assertReleaseManifestCompatibility(manifest(), new Proxy({ bootstrapVersion: '1.5.0' }, {}))
  )
})

test('rejects custom objects and hostile data arrays', () => {
  const customManifest = manifest()
  Object.setPrototypeOf(customManifest, { custom: true })
  assertError(ReleaseManifestValidationError, 'invalid_object', () =>
    validateReleaseManifest(customManifest)
  )

  const symbolManifest = manifest()
  symbolManifest[Symbol('extra')] = true
  assertError(ReleaseManifestValidationError, 'invalid_fields', () =>
    validateReleaseManifest(symbolManifest)
  )

  const nonEnumerableManifest = manifest()
  Object.defineProperty(nonEnumerableManifest, 'hidden', { value: true })
  assertError(ReleaseManifestValidationError, 'invalid_fields', () =>
    validateReleaseManifest(nonEnumerableManifest)
  )

  const sparseSupportedFrom = manifest()
  sparseSupportedFrom.compatibility.supported_from = new Array(1)
  assertError(ReleaseManifestValidationError, 'invalid_supported_from', () =>
    validateReleaseManifest(sparseSupportedFrom)
  )

  const customArrayManifest = manifest()
  Object.setPrototypeOf(
    customArrayManifest.compatibility.supported_from,
    Object.create(Array.prototype)
  )
  assertError(ReleaseManifestValidationError, 'invalid_supported_from', () =>
    validateReleaseManifest(customArrayManifest)
  )

  const extraArrayField = manifest()
  extraArrayField.compatibility.supported_from.extra = true
  assertError(ReleaseManifestValidationError, 'invalid_supported_from', () =>
    validateReleaseManifest(extraArrayField)
  )

  const symbolArrayField = manifest()
  symbolArrayField.compatibility.supported_from[Symbol('extra')] = true
  assertError(ReleaseManifestValidationError, 'invalid_supported_from', () =>
    validateReleaseManifest(symbolArrayField)
  )

  const nonEnumerableArrayField = manifest()
  Object.defineProperty(nonEnumerableArrayField.compatibility.supported_from, 'hidden', {
    value: true,
  })
  assertError(ReleaseManifestValidationError, 'invalid_supported_from', () =>
    validateReleaseManifest(nonEnumerableArrayField)
  )

  const accessorArray = manifest()
  let arrayGetterExecuted = false
  Object.defineProperty(accessorArray.compatibility.supported_from, 0, {
    configurable: true,
    enumerable: true,
    get() {
      arrayGetterExecuted = true
      throw new Error('array getter must not execute')
    },
  })
  assertError(ReleaseManifestValidationError, 'invalid_supported_from', () =>
    validateReleaseManifest(accessorArray)
  )
  assert.equal(arrayGetterExecuted, false)

  let proxyTrapExecuted = false
  const proxiedArtifacts = manifest()
  proxiedArtifacts.artifacts = new Proxy(proxiedArtifacts.artifacts, {
    ownKeys() {
      proxyTrapExecuted = true
      throw new Error('array proxy trap must not execute')
    },
  })
  assertError(ReleaseManifestValidationError, 'invalid_artifacts', () =>
    validateReleaseManifest(proxiedArtifacts)
  )
  assert.equal(proxyTrapExecuted, false)
})

test('validates a complete schema-1 manifest and selects its unique artifact', () => {
  const releaseManifest = manifest()
  const snapshot = validateReleaseManifest(releaseManifest)
  assert.notEqual(snapshot, releaseManifest)
  assert.ok(Object.isFrozen(snapshot))
  assert.ok(Object.isFrozen(snapshot.product))
  assert.ok(Object.isFrozen(snapshot.bootstrap))
  assert.ok(Object.isFrozen(snapshot.compatibility))
  assert.ok(Object.isFrozen(snapshot.compatibility.supported_from))
  assert.ok(Object.isFrozen(snapshot.artifacts))
  assert.ok(Object.isFrozen(snapshot.artifacts[0]))

  const artifact = selectReleaseArtifact(releaseManifest, {
    operation: 'fresh-install',
    edition: 'lite',
    installationShape: 'personal-native',
    os: 'linux',
    arch: 'amd64',
    bootstrapVersion: '1.5.0',
  })

  releaseManifest.artifacts[0].id = 'tampered-after-selection'
  releaseManifest.compatibility.supported_from.push('1.3.0')
  assert.equal(snapshot.artifacts[0].id, 'myapi-lite-linux-amd64')
  assert.deepEqual(snapshot.compatibility.supported_from, ['1.1.0', '1.2.0'])
  assert.equal(artifact.id, 'myapi-lite-linux-amd64')
  assert.ok(Object.isFrozen(artifact))
})

test('rejects unsupported schema versions, malformed hashes, and invalid platform fields', () => {
  assertError(ReleaseManifestValidationError, 'unsupported_schema_version', () =>
    validateReleaseManifest(manifest({ schema_version: 2 }))
  )

  const malformedHash = manifest()
  malformedHash.artifacts[0].sha256_or_digest = 'not-a-sha256'
  assertError(ReleaseManifestValidationError, 'invalid_sha256', () =>
    validateReleaseManifest(malformedHash)
  )

  const invalidPlatform = manifest()
  invalidPlatform.artifacts[0].arch = 'x64'
  assertError(ReleaseManifestValidationError, 'invalid_value', () =>
    validateReleaseManifest(invalidPlatform)
  )
})

test('rejects malformed portable artifact identities and OCI location/digest mismatches', () => {
  const invalidId = manifest()
  invalidId.artifacts[0].id = '../not-owned'
  assertError(ReleaseManifestValidationError, 'invalid_artifact_id', () =>
    validateReleaseManifest(invalidId)
  )

  const caseAmbiguousId = manifest()
  caseAmbiguousId.artifacts[0].id = 'MyAPI-lite-linux-amd64'
  assertError(ReleaseManifestValidationError, 'invalid_artifact_id', () =>
    validateReleaseManifest(caseAmbiguousId)
  )

  const windowsReservedId = manifest()
  windowsReservedId.artifacts[0].id = 'con.release'
  assertError(ReleaseManifestValidationError, 'invalid_artifact_id', () =>
    validateReleaseManifest(windowsReservedId)
  )

  const mismatchedDigest = manifest()
  mismatchedDigest.artifacts[1].location = `ghcr.io/forcemind/myapi@sha256:${'c'.repeat(64)}`
  assertError(ReleaseManifestValidationError, 'digest_mismatch', () =>
    validateReleaseManifest(mismatchedDigest)
  )

  const ambiguousOCIRegistry = manifest()
  ambiguousOCIRegistry.artifacts[1].location = `myapi/full@${digest}`
  assertError(ReleaseManifestValidationError, 'invalid_location', () =>
    validateReleaseManifest(ambiguousOCIRegistry)
  )

  const unsafeOCIPath = manifest()
  unsafeOCIPath.artifacts[1].location = `ghcr.io/forcemind/../myapi@${digest}`
  assertError(ReleaseManifestValidationError, 'invalid_location', () =>
    validateReleaseManifest(unsafeOCIPath)
  )

  const invalidOCIPort = manifest()
  invalidOCIPort.artifacts[1].location = `ghcr.io:65536/forcemind/myapi@${digest}`
  assertError(ReleaseManifestValidationError, 'invalid_location', () =>
    validateReleaseManifest(invalidOCIPort)
  )

  for (const invalidRegistry of [
    `ghcr..io/forcemind/myapi@${digest}`,
    `ghcr.-io/forcemind/myapi@${digest}`,
    `ghcr.io:0/forcemind/myapi@${digest}`,
    `ghcr.io:00080/forcemind/myapi@${digest}`,
    `GHCR.IO/forcemind/myapi@${digest}`,
    `256.1.1.1/forcemind/myapi@${digest}`,
    `[::1]/forcemind/myapi@${digest}`,
  ]) {
    const malformedRegistry = manifest()
    malformedRegistry.artifacts[1].location = invalidRegistry
    assertError(ReleaseManifestValidationError, 'invalid_location', () =>
      validateReleaseManifest(malformedRegistry)
    )
  }

  const mutableDownloadLocation = manifest()
  mutableDownloadLocation.artifacts[0].location += '?version=latest'
  assertError(ReleaseManifestValidationError, 'invalid_location', () =>
    validateReleaseManifest(mutableDownloadLocation)
  )

  const latestDownloadLocation = manifest()
  latestDownloadLocation.artifacts[0].location = 'https://releases.example.test/latest'
  assertError(ReleaseManifestValidationError, 'invalid_location', () =>
    validateReleaseManifest(latestDownloadLocation)
  )

  for (const encodedUnsafePath of [
    'https://releases.example.test/%6c%61%74%65%73%74/pkg',
    'https://releases.example.test/%2e%2e/pkg',
    'https://releases.example.test/%252e%252e/pkg',
    'https://releases.example.test/a/../pkg',
    'https://releases.example.test/pkg%2Fother',
    'https://releases.example.test/pkg%5Cother',
    'https://releases.example.test/pkg\\other',
    'https://releases.example.test/pkg%00other',
    'https://releases.example.test/pkg%C2%85other',
    'https://releases.example.test/pkg?',
    'https://releases.example.test/pkg#',
  ]) {
    const unsafeDownloadPath = manifest()
    unsafeDownloadPath.artifacts[0].location = encodedUnsafePath
    assertError(ReleaseManifestValidationError, 'invalid_location', () =>
      validateReleaseManifest(unsafeDownloadPath)
    )
  }

  const mutableProvenance = manifest()
  mutableProvenance.artifacts[0].sbom_or_provenance += '#unverified'
  assertError(ReleaseManifestValidationError, 'invalid_location', () =>
    validateReleaseManifest(mutableProvenance)
  )

  const unsafeSignature = manifest()
  unsafeSignature.artifacts[0].signature = 'bundle\nnext-header'
  assertError(ReleaseManifestValidationError, 'invalid_reference', () =>
    validateReleaseManifest(unsafeSignature)
  )

  const invalidSourceRevision = manifest()
  invalidSourceRevision.product.source_sha = 'a'.repeat(63)
  assertError(ReleaseManifestValidationError, 'invalid_source_sha', () =>
    validateReleaseManifest(invalidSourceRevision)
  )
})

test('bounds schema-1 collection and semantic-version fields', () => {
  const longVersion = manifest()
  longVersion.product.version = `1.2.3-${'a'.repeat(251)}`
  assertError(ReleaseManifestValidationError, 'invalid_version', () =>
    validateReleaseManifest(longVersion)
  )

  const tooManySupportedVersions = manifest()
  tooManySupportedVersions.compatibility.supported_from = Array.from(
    { length: 129 },
    (_, index) => `1.0.${index}`
  )
  assertError(ReleaseManifestValidationError, 'invalid_supported_from', () =>
    validateReleaseManifest(tooManySupportedVersions)
  )

  const tooManyArtifacts = manifest()
  for (let index = 0; index < 255; index += 1) {
    tooManyArtifacts.artifacts.push({
      ...tooManyArtifacts.artifacts[0],
      id: `myapi-lite-linux-amd64-${index}`,
    })
  }
  assertError(ReleaseManifestValidationError, 'invalid_artifacts', () =>
    validateReleaseManifest(tooManyArtifacts)
  )

  const maxBoundManifest = manifest()
  maxBoundManifest.artifacts[0].id = 'a'.repeat(128)
  maxBoundManifest.compatibility.supported_from = Array.from(
    { length: 128 },
    (_, index) => `1.0.${index}`
  )
  for (let index = 0; index < 254; index += 1) {
    maxBoundManifest.artifacts.push({
      ...maxBoundManifest.artifacts[0],
      id: `lite-${index}`,
    })
  }
  assert.equal(validateReleaseManifest(maxBoundManifest).artifacts.length, 256)
})

test('compares arbitrarily large semantic version components without numeric precision loss', () => {
  assert.equal(
    compareSemver('999999999999999999999999.0.0', '1000000000000000000000000.0.0'),
    -1
  )
  assert.equal(
    compareSemver('1.0.0-999999999999999999999999', '1.0.0-1000000000000000000000000'),
    -1
  )
})

test('fails closed for every managed operation until a verified transition matrix exists', () => {
  const managedTarget = {
    operation: 'upgrade',
    edition: 'lite',
    installationShape: 'personal-native',
    os: 'linux',
    arch: 'amd64',
    bootstrapVersion: '1.5.0',
  }
  for (const operation of ['upgrade', 'switch', 'rollback']) {
    assertError(ReleaseManifestSelectionError, 'managed_operation_unsupported', () =>
      selectReleaseArtifact(manifest(), { ...managedTarget, operation })
    )
  }

  const unsafeFreshTarget = {
    ...managedTarget,
    operation: 'fresh-install',
    currentInstallation: {
      version: '1.1.0',
      configSchema: '2.0.0',
      dataSchema: '3.0.0',
    },
  }
  assertError(ReleaseManifestSelectionError, 'invalid_target', () =>
    selectReleaseArtifact(manifest(), unsafeFreshTarget)
  )

  assertError(ReleaseManifestCompatibilityError, 'invalid_options', () =>
    assertReleaseManifestCompatibility(manifest(), {
      bootstrapVersion: '1.5.0',
      currentInstallation: unsafeFreshTarget.currentInstallation,
    })
  )
})

test('reports all malformed selection fields as selection errors', () => {
  const target = {
    operation: 'fresh-install',
    edition: 'lite',
    installationShape: 'personal-native',
    os: 'linux',
    arch: 'amd64',
    bootstrapVersion: '1.5.0',
  }
  for (const [field, value] of [
    ['operation', 'repair'],
    ['edition', 'enterprise'],
    ['installationShape', 'bare-metal'],
    ['os', 'freebsd'],
    ['arch', 'x64'],
    ['kind', 'archive'],
  ]) {
    assertError(ReleaseManifestSelectionError, 'invalid_target', () =>
      selectReleaseArtifact(manifest(), { ...target, [field]: value })
    )
  }
})

test('rejects a bootstrap version outside the declared compatibility range', () => {
  assertError(ReleaseManifestCompatibilityError, 'bootstrap_incompatible', () =>
    selectReleaseArtifact(manifest(), {
      operation: 'fresh-install',
      edition: 'lite',
      installationShape: 'personal-native',
      os: 'linux',
      arch: 'amd64',
      bootstrapVersion: '0.9.9',
    })
  )
})

test('allows canonical OCI registry port boundaries', () => {
  for (const port of ['1', '65535']) {
    const explicitPort = manifest()
    explicitPort.artifacts[1].location = `registry.example.test:${port}/forcemind/myapi@${digest}`
    assert.equal(validateReleaseManifest(explicitPort).artifacts[1].location, explicitPort.artifacts[1].location)
  }
})

test('bounds an untrusted bootstrap version before semantic comparison', () => {
  assertError(ReleaseManifestCompatibilityError, 'invalid_bootstrap_version', () =>
    selectReleaseArtifact(manifest(), {
      operation: 'fresh-install',
      edition: 'lite',
      installationShape: 'personal-native',
      os: 'linux',
      arch: 'amd64',
      bootstrapVersion: `1.0.0-${'a'.repeat(251)}`,
    })
  )

  assertError(ReleaseManifestCompatibilityError, 'invalid_bootstrap_version', () =>
    selectReleaseArtifact(manifest(), {
      operation: 'fresh-install',
      edition: 'lite',
      installationShape: 'personal-native',
      os: 'linux',
      arch: 'amd64',
      bootstrapVersion: 'not-a-version',
    })
  )
})

test('fails closed for no matching or ambiguous artifacts', () => {
  const target = {
    operation: 'fresh-install',
    edition: 'lite',
    installationShape: 'personal-native',
    os: 'darwin',
    arch: 'arm64',
    bootstrapVersion: '1.5.0',
  }
  assertError(ReleaseManifestSelectionError, 'artifact_not_found', () =>
    selectReleaseArtifact(manifest(), target)
  )

  const ambiguous = manifest()
  ambiguous.artifacts.push({ ...ambiguous.artifacts[0], id: 'myapi-lite-linux-amd64-alt' })
  assertError(ReleaseManifestSelectionError, 'artifact_ambiguous', () =>
    selectReleaseArtifact(ambiguous, {
      ...target,
      os: 'linux',
      arch: 'amd64',
    })
  )
})
