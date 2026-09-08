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
 * A bounded, detached representation of untrusted Release Manifest bytes.
 *
 * This module deliberately performs no I/O, parsing, fetching, signature
 * verification, provenance verification, installation, or state transition.
 * Capturing bytes is not evidence that they are trusted or verified. A future
 * trust boundary must validate this material before it is parsed or used.
 */
import { Buffer } from 'node:buffer'
import { createHash } from 'node:crypto'
import { types } from 'node:util'

export const PENDING_RELEASE_MANIFEST_EVIDENCE_REVISION = 1
export const PENDING_RELEASE_MANIFEST_EVIDENCE_KIND = 'untrusted_release_manifest_bytes'
export const PENDING_RELEASE_MANIFEST_MEDIA_TYPE = 'application/json'
export const PENDING_RELEASE_MANIFEST_ENCODING = 'base64'

const MIN_MANIFEST_BYTES = 1
const MAX_MANIFEST_BYTES = 4 * 1024 * 1024
const SHA256_PATTERN = /^[a-f0-9]{64}$/
const CANONICAL_BASE64_PATTERN = /^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/
const EVIDENCE_KEYS = Object.freeze([
  'revision',
  'kind',
  'media_type',
  'encoding',
  'byte_length',
  'content_sha256',
  'raw_base64',
])
const TYPED_ARRAY_PROTOTYPE = Object.getPrototypeOf(Uint8Array.prototype)
const TYPED_ARRAY_BUFFER_GETTER = Object.getOwnPropertyDescriptor(TYPED_ARRAY_PROTOTYPE, 'buffer').get
const TYPED_ARRAY_BYTE_LENGTH_GETTER = Object.getOwnPropertyDescriptor(TYPED_ARRAY_PROTOTYPE, 'byteLength').get

export class PendingReleaseManifestEvidenceError extends Error {
  constructor(code, message) {
    super(message)
    this.name = 'PendingReleaseManifestEvidenceError'
    this.code = code
  }
}

function evidenceError(code, message) {
  throw new PendingReleaseManifestEvidenceError(code, message)
}

function expectedBase64Length(byteLength) {
  return Math.ceil(byteLength / 3) * 4
}

function isPlainDataObject(value) {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) return false
  const prototype = Object.getPrototypeOf(value)
  return prototype === Object.prototype || prototype === null
}

function captureArrayBufferBackedUint8Array(rawBytes) {
  if (types.isProxy(rawBytes) || !types.isUint8Array(rawBytes)) {
    evidenceError(
      'invalid_raw_bytes',
      'release manifest bytes must be an ArrayBuffer-backed Uint8Array'
    )
  }

  const backingBuffer = Reflect.apply(TYPED_ARRAY_BUFFER_GETTER, rawBytes, [])
  const byteLength = Reflect.apply(TYPED_ARRAY_BYTE_LENGTH_GETTER, rawBytes, [])
  if (!types.isArrayBuffer(backingBuffer) || types.isSharedArrayBuffer(backingBuffer)) {
    evidenceError(
      'invalid_raw_bytes',
      'release manifest bytes must be an ArrayBuffer-backed Uint8Array'
    )
  }
  if (!Number.isSafeInteger(byteLength) || byteLength < MIN_MANIFEST_BYTES || byteLength > MAX_MANIFEST_BYTES) {
    evidenceError('invalid_byte_length', 'release manifest byte length is outside the allowed bound')
  }
  return byteLength
}

function copyBytes(rawBytes, byteLength) {
  const copied = new Uint8Array(byteLength)
  Reflect.apply(Uint8Array.prototype.set, copied, [rawBytes])
  return copied
}

function digestBytes(bytes) {
  return createHash('sha256').update(bytes).digest('hex')
}

function snapshotEvidence(values) {
  return Object.freeze({
    revision: values.revision,
    kind: values.kind,
    media_type: values.media_type,
    encoding: values.encoding,
    byte_length: values.byte_length,
    content_sha256: values.content_sha256,
    raw_base64: values.raw_base64,
  })
}

function readEvidenceDataFields(evidence) {
  if (types.isProxy(evidence)) {
    evidenceError('invalid_evidence', 'release manifest evidence must be a plain data object')
  }
  if (!isPlainDataObject(evidence)) {
    evidenceError('invalid_evidence', 'release manifest evidence must be a plain data object')
  }

  const ownKeys = Reflect.ownKeys(evidence)
  if (ownKeys.length !== EVIDENCE_KEYS.length || ownKeys.some((key) => !EVIDENCE_KEYS.includes(key))) {
    evidenceError('invalid_fields', 'release manifest evidence has unsupported or missing fields')
  }

  const values = {}
  for (const key of EVIDENCE_KEYS) {
    const descriptor = Object.getOwnPropertyDescriptor(evidence, key)
    if (!descriptor || !Object.hasOwn(descriptor, 'value') || !descriptor.enumerable) {
      evidenceError('invalid_fields', 'release manifest evidence fields must be enumerable data properties')
    }
    values[key] = descriptor.value
  }
  return values
}

function assertEvidenceHeader(values) {
  if (values.revision !== PENDING_RELEASE_MANIFEST_EVIDENCE_REVISION) {
    evidenceError('unsupported_revision', 'release manifest evidence revision is unsupported')
  }
  if (values.kind !== PENDING_RELEASE_MANIFEST_EVIDENCE_KIND) {
    evidenceError('invalid_kind', 'release manifest evidence kind is invalid')
  }
  if (values.media_type !== PENDING_RELEASE_MANIFEST_MEDIA_TYPE) {
    evidenceError('invalid_media_type', 'release manifest evidence media type is invalid')
  }
  if (values.encoding !== PENDING_RELEASE_MANIFEST_ENCODING) {
    evidenceError('invalid_encoding', 'release manifest evidence encoding is invalid')
  }
  if (
    !Number.isSafeInteger(values.byte_length) ||
    values.byte_length < MIN_MANIFEST_BYTES ||
    values.byte_length > MAX_MANIFEST_BYTES
  ) {
    evidenceError('invalid_byte_length', 'release manifest byte length is outside the allowed bound')
  }
  if (typeof values.content_sha256 !== 'string' || !SHA256_PATTERN.test(values.content_sha256)) {
    evidenceError('invalid_content_sha256', 'release manifest content digest is invalid')
  }
}

function materializeValidatedEvidence(values) {
  if (
    typeof values.raw_base64 !== 'string' ||
    values.raw_base64.length !== expectedBase64Length(values.byte_length) ||
    !CANONICAL_BASE64_PATTERN.test(values.raw_base64)
  ) {
    evidenceError('invalid_raw_base64', 'release manifest bytes are not canonical base64')
  }

  const decoded = new Uint8Array(Buffer.from(values.raw_base64, PENDING_RELEASE_MANIFEST_ENCODING))
  if (
    decoded.byteLength !== values.byte_length ||
    Buffer.from(decoded).toString(PENDING_RELEASE_MANIFEST_ENCODING) !== values.raw_base64
  ) {
    evidenceError('invalid_raw_base64', 'release manifest bytes are not canonical base64')
  }
  if (digestBytes(decoded) !== values.content_sha256) {
    evidenceError('content_digest_mismatch', 'release manifest content digest does not match its bytes')
  }
  return decoded
}

/**
 * Copy a bounded ArrayBuffer-backed byte view into a detached evidence record.
 */
export function capturePendingReleaseManifestBytes(rawBytes) {
  const byteLength = captureArrayBufferBackedUint8Array(rawBytes)
  const copied = copyBytes(rawBytes, byteLength)
  return snapshotEvidence({
    revision: PENDING_RELEASE_MANIFEST_EVIDENCE_REVISION,
    kind: PENDING_RELEASE_MANIFEST_EVIDENCE_KIND,
    media_type: PENDING_RELEASE_MANIFEST_MEDIA_TYPE,
    encoding: PENDING_RELEASE_MANIFEST_ENCODING,
    byte_length: copied.byteLength,
    content_sha256: digestBytes(copied),
    raw_base64: Buffer.from(copied).toString(PENDING_RELEASE_MANIFEST_ENCODING),
  })
}

/**
 * Validate, detach, and freeze untrusted byte evidence. This does not confer
 * trust or verification status on the represented Release Manifest.
 */
export function validatePendingReleaseManifestEvidence(evidence) {
  const values = readEvidenceDataFields(evidence)
  assertEvidenceHeader(values)
  materializeValidatedEvidence(values)
  return snapshotEvidence(values)
}

/**
 * Return a fresh byte copy after fully validating the evidence record.
 */
export function materializePendingReleaseManifestBytes(evidence) {
  const values = readEvidenceDataFields(evidence)
  assertEvidenceHeader(values)
  const decoded = materializeValidatedEvidence(values)
  return copyBytes(decoded, decoded.byteLength)
}
