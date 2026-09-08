/*
 * MyAPI distribution and self-hosting tooling.
 * Copyright (C) 2026 ForceMind
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published
 * by the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 */

// These lexical predicates deliberately perform no I/O, fetching, signature
// verification, registry lookup, or filesystem resolution. Both manifest and
// durable installation-state validation use them so a non-secret record never
// accepts an artifact reference the release manifest would reject.
const OCI_REFERENCE_PATTERN = /^([^/@]+)\/([^@]+)@sha256:([a-f0-9]{64})$/
const OCI_REPOSITORY_COMPONENT_PATTERN = /^[a-z0-9]+(?:[._-][a-z0-9]+)*$/
const OCI_DNS_LABEL_PATTERN = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/
const CONTROL_CHARACTER_PATTERN = /[\p{Cc}\p{Cf}]/u

function isCanonicalIPv4(value) {
  const parts = value.split('.')
  return (
    parts.length === 4 &&
    parts.every((part) => /^(?:0|[1-9]\d{0,2})$/.test(part) && Number(part) <= 255)
  )
}

function isCanonicalOCIRegistry(registry) {
  const separator = registry.lastIndexOf(':')
  const host = separator === -1 ? registry : registry.slice(0, separator)
  const port = separator === -1 ? undefined : registry.slice(separator + 1)
  if (!host || host.includes(':') || registry !== registry.toLowerCase()) return false
  if (port !== undefined && (!/^[1-9]\d{0,4}$/.test(port) || Number(port) > 65535)) {
    return false
  }
  if (host === 'localhost' || isCanonicalIPv4(host)) return true
  if (/^\d+(?:\.\d+){3}$/.test(host) || host.length > 253 || !host.includes('.')) {
    return false
  }
  return host.split('.').every((label) => OCI_DNS_LABEL_PATTERN.test(label))
}

/**
 * Return a detached canonical OCI reference, or null when it is not a
 * credential-free registry/repository digest reference suitable for durable
 * non-secret installation state.
 */
export function parseCanonicalOCIReference(value) {
  if (typeof value !== 'string') return null
  const match = OCI_REFERENCE_PATTERN.exec(value)
  if (!match) return null
  const [, registry, repository, digest] = match
  if (!isCanonicalOCIRegistry(registry)) return null
  if (!repository.split('/').every((component) => OCI_REPOSITORY_COMPONENT_PATTERN.test(component))) {
    return null
  }
  return Object.freeze({
    registry,
    repository,
    digest: `sha256:${digest}`,
  })
}

/**
 * Check an immutable HTTPS download reference without accepting URL userinfo,
 * queries, fragments, traversal, ambiguous separators, or a mutable `latest`
 * segment. Callers remain responsible for their own length/ASCII diagnostics.
 */
export function isPinnedHTTPSURL(value) {
  if (typeof value !== 'string') return false
  if (
    value.includes('?') ||
    value.includes('#') ||
    value.includes('\\') ||
    /%(?:2e|2f|5c|25)/i.test(value) ||
    /(?:^|\/)\.{1,2}(?=\/|$)/.test(value)
  ) {
    return false
  }
  let location
  try {
    location = new URL(value)
  } catch {
    return false
  }
  if (
    location.protocol !== 'https:' ||
    !location.hostname ||
    location.username ||
    location.password ||
    location.search ||
    location.hash
  ) {
    return false
  }
  const pathSegments = location.pathname.split('/')
  for (let index = 0; index < pathSegments.length; index += 1) {
    const rawSegment = pathSegments[index]
    if (rawSegment === '' && index !== 0 && index !== pathSegments.length - 1) {
      return false
    }
    let segment
    try {
      segment = decodeURIComponent(rawSegment)
    } catch {
      return false
    }
    if (
      segment.includes('%') ||
      segment.includes('/') ||
      segment.includes('\\') ||
      segment.includes('?') ||
      segment.includes('#') ||
      segment === '.' ||
      segment === '..' ||
      CONTROL_CHARACTER_PATTERN.test(segment) ||
      segment.toLowerCase() === 'latest'
    ) {
      return false
    }
  }
  return true
}
