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
 * Pure compatibility parsing for a Legacy Docker deployment.
 *
 * This module deliberately does not read environment files, inspect sockets,
 * resolve names, access Docker, or change any deployment setting. It only
 * classifies an explicitly supplied Legacy configuration. In particular, an
 * unspecified or broadly bound legacy listener is never evidence of public
 * reachability.
 */

import { isIP } from 'node:net'
import { types } from 'node:util'

const ALLOWED_INPUT_KEYS = Object.freeze([
  'edition',
  'bindAddress',
  'allowLan',
  'reverseProxyConfigured',
])

const CONTROL_CHARACTER_PATTERN = /[\p{Cc}\p{Cf}]/u
const IPV4_LOOPBACK_PATTERN = /^127(?:\.\d{1,3}){3}$/
const IPV4_PRIVATE_PATTERN = /^(?:10(?:\.\d{1,3}){3}|192\.168(?:\.\d{1,3}){2}|172\.(?:1[6-9]|2\d|3[0-1])(?:\.\d{1,3}){2})$/

export class LegacyInstallationProfileError extends Error {
  constructor(code, message) {
    super(message)
    this.name = 'LegacyInstallationProfileError'
    this.code = code
  }
}

function profileError(code, message) {
  throw new LegacyInstallationProfileError(code, message)
}

function isPlainDataObject(value) {
  if (value === null || typeof value !== 'object') return false
  if (types.isProxy(value)) return false
  return !Array.isArray(value) && Object.getPrototypeOf(value) === Object.prototype
}

function assertInputObject(input) {
  if (!isPlainDataObject(input)) {
    profileError('invalid_input', 'legacy installation profile input must be a plain data object')
  }

  const keys = Reflect.ownKeys(input)
  if (keys.some((key) => typeof key !== 'string')) {
    profileError('invalid_fields', 'legacy installation profile input must not contain symbol fields')
  }
  for (const key of keys) {
    const descriptor = Object.getOwnPropertyDescriptor(input, key)
    if (!descriptor?.enumerable || !Object.hasOwn(descriptor, 'value')) {
      profileError('invalid_fields', `legacy installation profile input.${key} must be an enumerable data field`)
    }
    if (!ALLOWED_INPUT_KEYS.includes(key)) {
      profileError('invalid_fields', `legacy installation profile input has unsupported field ${key}`)
    }
  }
}

function assertBoundedText(value, field) {
  if (
    typeof value !== 'string' ||
    value.length === 0 ||
    value.length > 255 ||
    value.trim() !== value ||
    CONTROL_CHARACTER_PATTERN.test(value)
  ) {
    profileError('invalid_value', `legacy installation profile input.${field} is invalid`)
  }
}

function validateOptionalBoolean(input, field) {
  if (!Object.hasOwn(input, field)) return undefined
  if (typeof input[field] !== 'boolean') {
    profileError('invalid_value', `legacy installation profile input.${field} must be boolean`)
  }
  return input[field]
}

function classifyAddress(address) {
  if (address === 'localhost') return 'loopback'
  if (address === '0.0.0.0' || address === '::') return 'broad'
  const addressFamily = isIP(address)
  if (addressFamily === 0) return 'ambiguous'
  if (address === '::1' || IPV4_LOOPBACK_PATTERN.test(address)) return 'loopback'
  if (IPV4_PRIVATE_PATTERN.test(address)) return 'private'
  if (addressFamily === 6 && /^f[cd][0-9a-f:]*$/i.test(address)) return 'private'
  if (addressFamily !== 0) return 'public'
  return 'ambiguous'
}

function snapshotProfile(values) {
  return Object.freeze({
    product_edition: values.product_edition,
    installation_shape: 'legacy-server-container',
    desired_access_mode: values.desired_access_mode,
    legacy_identity: values.legacy_identity,
    migration_status: values.migration_status,
    warnings: Object.freeze([...values.warnings]),
  })
}

/**
 * Convert an explicit Legacy Docker configuration to the target three-axis
 * product profile. `public` is intentionally never an output: this parser has
 * no external-reachability evidence and cannot authorize an access expansion.
 */
export function resolveLegacyInstallationProfile(input) {
  assertInputObject(input)

  const warnings = []
  let productEdition = null
  let legacyIdentity = null
  if (Object.hasOwn(input, 'edition')) {
    assertBoundedText(input.edition, 'edition')
    if (input.edition === 'full') {
      productEdition = 'full'
      legacyIdentity = 'full'
    } else if (input.edition === 'lan') {
      productEdition = 'lite'
      legacyIdentity = 'lan'
    } else {
      profileError('invalid_value', 'legacy installation profile input.edition must be full or lan')
    }
  } else {
    warnings.push('Legacy edition is missing; product edition requires manual confirmation.')
  }

  const allowLan = validateOptionalBoolean(input, 'allowLan')
  const reverseProxyConfigured = validateOptionalBoolean(input, 'reverseProxyConfigured')
  let addressKind = 'missing'
  if (Object.hasOwn(input, 'bindAddress')) {
    assertBoundedText(input.bindAddress, 'bindAddress')
    addressKind = classifyAddress(input.bindAddress)
  }

  let desiredAccessMode = 'needs_manual'
  if (reverseProxyConfigured === true) {
    warnings.push('Reverse-proxy configuration requires manual access-mode confirmation.')
  } else if (addressKind === 'loopback' && allowLan === false) {
    desiredAccessMode = 'local'
  } else if (addressKind === 'private' && allowLan === true) {
    desiredAccessMode = 'lan'
  } else if (addressKind === 'broad' || addressKind === 'public') {
    warnings.push('Broad or public listener binding is not proof of public reachability.')
  } else {
    warnings.push('Legacy listener configuration is incomplete or ambiguous; access mode requires manual confirmation.')
  }

  if (desiredAccessMode === 'needs_manual') {
    return snapshotProfile({
      product_edition: productEdition,
      desired_access_mode: desiredAccessMode,
      legacy_identity: legacyIdentity,
      migration_status: 'needs_manual',
      warnings,
    })
  }

  return snapshotProfile({
    product_edition: productEdition,
    desired_access_mode: desiredAccessMode,
    legacy_identity: legacyIdentity,
    migration_status: productEdition === null ? 'needs_manual' : 'compatible',
    warnings,
  })
}
