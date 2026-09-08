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
import { Buffer } from 'node:buffer'
import { createHash } from 'node:crypto'
import { test } from 'node:test'
import {
  capturePendingReleaseManifestBytes,
  materializePendingReleaseManifestBytes,
  PendingReleaseManifestEvidenceError,
  validatePendingReleaseManifestEvidence,
} from '../lib/release-manifest-evidence.mjs'

function assertError(code, action) {
  assert.throws(action, (error) => error instanceof PendingReleaseManifestEvidenceError && error.code === code)
}

function digest(bytes) {
  return createHash('sha256').update(bytes).digest('hex')
}

function evidenceFor(bytes) {
  return capturePendingReleaseManifestBytes(bytes)
}

test('captures an offset Uint8Array view as detached, bounded untrusted evidence', () => {
  const source = Uint8Array.from([0, 123, 34, 97, 34, 58, 49, 125, 0])
  const view = new Uint8Array(source.buffer, 1, source.byteLength - 2)
  const evidence = evidenceFor(view)

  source.fill(0)
  assert.ok(Object.isFrozen(evidence))
  assert.equal(evidence.byte_length, 7)
  assert.equal(evidence.content_sha256, digest(Uint8Array.from([123, 34, 97, 34, 58, 49, 125])))
  assert.equal(evidence.raw_base64, 'eyJhIjoxfQ==')
  assert.deepEqual([...materializePendingReleaseManifestBytes(evidence)], [123, 34, 97, 34, 58, 49, 125])
})

test('validates a frozen detached snapshot and materializes a new copy every time', () => {
  const evidence = validatePendingReleaseManifestEvidence(evidenceFor(Buffer.from('{"version":1}')))
  const first = materializePendingReleaseManifestBytes(evidence)
  const second = materializePendingReleaseManifestBytes(evidence)

  assert.ok(Object.isFrozen(evidence))
  assert.notEqual(first, second)
  first[0] = 0
  assert.notEqual(first[0], second[0])
})

test('accepts only non-empty ArrayBuffer-backed Uint8Array bytes within the 4 MiB bound', () => {
  assertError('invalid_raw_bytes', () => capturePendingReleaseManifestBytes(new DataView(new ArrayBuffer(1))))
  assertError('invalid_byte_length', () => capturePendingReleaseManifestBytes(new Uint8Array(0)))
  const maximum = capturePendingReleaseManifestBytes(new Uint8Array(4 * 1024 * 1024))
  assert.equal(maximum.byte_length, 4 * 1024 * 1024)
  assertError('invalid_byte_length', () => capturePendingReleaseManifestBytes(new Uint8Array(4 * 1024 * 1024 + 1)))

  if (typeof SharedArrayBuffer === 'function') {
    assertError('invalid_raw_bytes', () =>
      capturePendingReleaseManifestBytes(new Uint8Array(new SharedArrayBuffer(1)))
    )
  }
})

test('uses genuine TypedArray internal slots instead of own shadow properties', () => {
  const shadowed = Uint8Array.from([0, 1, 2, 3])
  let shadowGetterRead = false
  Object.defineProperty(shadowed, 'buffer', {
    get() {
      shadowGetterRead = true
      throw new Error('must not execute')
    },
  })
  let shadowByteLengthGetterRead = false
  Object.defineProperty(shadowed, 'byteLength', {
    get() {
      shadowByteLengthGetterRead = true
      throw new Error('must not execute')
    },
  })
  const shadowedEvidence = capturePendingReleaseManifestBytes(shadowed)
  assert.equal(shadowGetterRead, false)
  assert.equal(shadowByteLengthGetterRead, false)
  assert.equal(shadowedEvidence.byte_length, 4)

  const fake = Object.create(Uint8Array.prototype)
  let fakeGetterRead = false
  Object.defineProperty(fake, 'buffer', {
    get() {
      fakeGetterRead = true
      throw new Error('must not execute')
    },
  })
  assertError('invalid_raw_bytes', () => capturePendingReleaseManifestBytes(fake))
  assert.equal(fakeGetterRead, false)

  const buffer = Buffer.from([0, 1, 2, 3, 4])
  const offsetBuffer = buffer.subarray(1, 4)
  assert.deepEqual([...materializePendingReleaseManifestBytes(evidenceFor(offsetBuffer))], [1, 2, 3])
})

test('rejects SharedArrayBuffer views even when their own buffer field is shadowed', () => {
  if (typeof SharedArrayBuffer !== 'function') return

  const view = new Uint8Array(new SharedArrayBuffer(1))
  Object.defineProperty(view, 'buffer', { value: new ArrayBuffer(1) })
  assertError('invalid_raw_bytes', () => capturePendingReleaseManifestBytes(view))
})

test('rejects evidence with altered header fields, digest, length, or non-canonical base64', () => {
  const evidence = evidenceFor(Uint8Array.from([0, 1, 2, 3]))
  for (const [key, value, code] of [
    ['revision', 2, 'unsupported_revision'],
    ['kind', 'verified_release_manifest_bytes', 'invalid_kind'],
    ['media_type', 'text/plain', 'invalid_media_type'],
    ['encoding', 'base64url', 'invalid_encoding'],
    ['byte_length', 3, 'invalid_raw_base64'],
    ['content_sha256', 'A'.repeat(64), 'invalid_content_sha256'],
    ['raw_base64', evidence.raw_base64.replace(/=$/, ''), 'invalid_raw_base64'],
    ['raw_base64', 'AAECAw=A', 'invalid_raw_base64'],
  ]) {
    assertError(code, () => validatePendingReleaseManifestEvidence({ ...evidence, [key]: value }))
  }

  assertError('content_digest_mismatch', () =>
    validatePendingReleaseManifestEvidence({ ...evidence, content_sha256: '0'.repeat(64) })
  )
})

test('rejects extra, missing, inherited, accessor, non-enumerable, and symbol fields', () => {
  const evidence = evidenceFor(Uint8Array.from([1, 2, 3]))
  assertError('invalid_fields', () => validatePendingReleaseManifestEvidence({ ...evidence, extra: true }))

  const missing = { ...evidence }
  delete missing.raw_base64
  assertError('invalid_fields', () => validatePendingReleaseManifestEvidence(missing))

  const inherited = Object.create(evidence)
  assertError('invalid_evidence', () => validatePendingReleaseManifestEvidence(inherited))

  const accessor = { ...evidence }
  Object.defineProperty(accessor, 'raw_base64', {
    enumerable: true,
    get() {
      throw new Error('must not execute')
    },
  })
  assertError('invalid_fields', () => validatePendingReleaseManifestEvidence(accessor))

  const nonEnumerable = { ...evidence }
  Object.defineProperty(nonEnumerable, 'raw_base64', {
    configurable: true,
    enumerable: false,
    value: evidence.raw_base64,
  })
  assertError('invalid_fields', () => validatePendingReleaseManifestEvidence(nonEnumerable))

  const withSymbol = { ...evidence, [Symbol('extra')]: true }
  assertError('invalid_fields', () => validatePendingReleaseManifestEvidence(withSymbol))
})

test('rejects evidence proxies before invoking any proxy trap', () => {
  let trapTouched = false
  const proxy = new Proxy(
    {},
    {
      get() {
        trapTouched = true
        throw new Error('must not execute')
      },
      getPrototypeOf() {
        trapTouched = true
        throw new Error('must not execute')
      },
      ownKeys() {
        trapTouched = true
        throw new Error('must not execute')
      },
      getOwnPropertyDescriptor() {
        trapTouched = true
        throw new Error('must not execute')
      },
    }
  )
  assertError('invalid_evidence', () => validatePendingReleaseManifestEvidence(proxy))
  assert.equal(trapTouched, false)
})

test('rejects non-plain objects and never treats fixtures as verified trust evidence', () => {
  const evidence = evidenceFor(Uint8Array.from([1]))
  assertError('invalid_evidence', () => validatePendingReleaseManifestEvidence(new (class Evidence {})()))
  assert.equal(evidence.kind, 'untrusted_release_manifest_bytes')
  assert.equal(Object.hasOwn(evidence, 'verified'), false)
})
