/*
 * Strict SemVer release-channel helpers.
 * Copyright (C) 2026 ForceMind
 */
import { fileURLToPath } from 'node:url'

const versionPattern = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*))?$/

export function parseSemVer(version) {
  if (typeof version !== 'string') return null
  const match = versionPattern.exec(version)
  if (!match) return null
  return {
    version,
    major: match[1],
    minor: match[2],
    patch: match[3],
    prerelease: match[4]?.split('.') ?? [],
  }
}

export function parseReleaseTag(tag) {
  if (typeof tag !== 'string' || !tag.startsWith('v')) return null
  return parseSemVer(tag.slice(1))
}

export function compareSemVer(left, right) {
  const a = typeof left === 'string' ? parseSemVer(left) : left
  const b = typeof right === 'string' ? parseSemVer(right) : right
  if (!a || !b) throw new Error('compareSemVer requires valid SemVer values')
  for (const field of ['major', 'minor', 'patch']) {
    if (a[field] !== b[field]) {
      if (a[field].length !== b[field].length) {
        return a[field].length > b[field].length ? 1 : -1
      }
      return a[field] > b[field] ? 1 : -1
    }
  }
  if (a.prerelease.length === 0 || b.prerelease.length === 0) {
    if (a.prerelease.length === b.prerelease.length) return 0
    return a.prerelease.length === 0 ? 1 : -1
  }
  const length = Math.max(a.prerelease.length, b.prerelease.length)
  for (let index = 0; index < length; index++) {
    const aPart = a.prerelease[index]
    const bPart = b.prerelease[index]
    if (aPart === undefined) return -1
    if (bPart === undefined) return 1
    if (aPart === bPart) continue
    const aNumber = /^\d+$/.test(aPart)
    const bNumber = /^\d+$/.test(bPart)
    if (aNumber && bNumber) {
      if (aPart.length !== bPart.length) return aPart.length > bPart.length ? 1 : -1
      return aPart > bPart ? 1 : -1
    }
    if (aNumber !== bNumber) return aNumber ? -1 : 1
    return aPart > bPart ? 1 : -1
  }
  return 0
}

export function canAdvanceStableLatest(candidateVersion, currentVersion) {
  const candidate = parseSemVer(candidateVersion)
  if (!candidate || candidate.prerelease.length > 0) return false
  if (!currentVersion) return true
  const current = parseSemVer(currentVersion)
  return Boolean(current && compareSemVer(candidate, current) > 0)
}

export function stableLatestAction(candidateVersion, currentVersion) {
  if (!currentVersion) return 'advance'
  const comparison = compareSemVer(candidateVersion, currentVersion)
  if (comparison > 0) return 'advance'
  if (comparison === 0) return 'idempotent'
  return 'reject'
}

export function latestStableTag(tags) {
  let latest = null
  for (const tag of tags) {
    const parsed = parseReleaseTag(tag)
    if (!parsed || parsed.prerelease.length > 0) continue
    if (!latest || compareSemVer(parsed, latest) > 0) latest = parsed
  }
  return latest ? `v${latest.version}` : ''
}

export function flattenPages(value) {
  if (!Array.isArray(value)) return []
  return value.flatMap((entry) => (Array.isArray(entry) ? flattenPages(entry) : [entry]))
}

function readJsonFromStdin() {
  let input = ''
  process.stdin.setEncoding('utf8')
  process.stdin.on('data', (chunk) => {
    input += chunk
  })
  return new Promise((resolve) => {
    process.stdin.on('end', () => resolve(JSON.parse(input)))
  })
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const [command, ...values] = process.argv.slice(2)
  if (command === 'validate-tag') {
    if (!parseReleaseTag(values[0])) process.exitCode = 1
  } else if (command === 'can-advance-stable-latest') {
    if (!canAdvanceStableLatest(values[0], values[1])) process.exitCode = 1
  } else if (command === 'stable-latest-action') {
    console.log(stableLatestAction(values[0], values[1]))
  } else if (
    command === 'latest-stable-release-tag' ||
    command === 'latest-stable-container-tag'
  ) {
    const documents = await readJsonFromStdin()
    const entries = flattenPages(documents)
    const tags =
      command === 'latest-stable-release-tag'
        ? entries
            .filter((entry) => !entry.draft && !entry.prerelease)
            .map((entry) => entry.tag_name)
        : entries.flatMap((entry) =>
            entry.metadata?.container?.tags?.includes('latest')
              ? entry.metadata.container.tags
              : []
          )
    console.log(latestStableTag(tags))
  } else {
    process.exitCode = 2
  }
}
