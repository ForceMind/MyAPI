import { types } from 'node:util'
import { check, digest } from './safety.mjs'

const archiveFields = ['id', 'instanceId', 'manifestSha256', 'treeSha256', 'identitySha256', 'rootDev', 'rootIno', 'createdUnixSeconds', 'logicalBytes']

function retentionRecord(value, required, optional = []) {
  check(value && typeof value === 'object' && !types.isProxy(value) && !Array.isArray(value), 'RETENTION_RECORD_INVALID')
  check([Object.prototype, null].includes(Object.getPrototypeOf(value)), 'RETENTION_RECORD_INVALID')
  const properties = Object.getOwnPropertyDescriptors(value)
  const keys = Reflect.ownKeys(properties)
  check(required.every(key => Object.hasOwn(properties, key)) &&
    keys.every(key => typeof key === 'string' && [...required, ...optional].includes(key) &&
      Object.hasOwn(properties[key], 'value') && properties[key].enumerable), 'RETENTION_RECORD_INVALID')
  return Object.fromEntries([...required, ...optional.filter(key => Object.hasOwn(properties, key))].map(key => [key, properties[key].value]))
}

function retentionList(value, minimum) {
  check(Array.isArray(value) && !types.isProxy(value) && Object.getPrototypeOf(value) === Array.prototype, 'RETENTION_LIST_INVALID')
  check(value.length >= minimum && value.length <= 10000, 'RETENTION_LIST_INVALID')
  const properties = Object.getOwnPropertyDescriptors(value)
  check(Reflect.ownKeys(properties).length === value.length + 1 &&
    Array.from({ length: value.length }, (_unused, index) => properties[index]).every(property =>
      property && Object.hasOwn(property, 'value') && property.enumerable), 'RETENTION_LIST_INVALID')
  return Array.from({ length: value.length }, (_unused, index) => properties[index].value)
}

function decimal64(value, positive) {
  return typeof value === 'string' && /^(0|[1-9][0-9]{0,19})$/.test(value) &&
    BigInt(value) <= 18446744073709551615n && (!positive || value !== '0')
}

function compareIds(left, right) {
  return left < right ? -1 : left > right ? 1 : 0
}

/**
 * Select only from the engine's complete, currently observed archive set.
 * This function performs no IO and does not verify journals, COMMITTED files,
 * process usage, mounts, source ownership or an unconsumed trigger. The engine
 * must establish those facts and revalidate the exact plan before any effect.
 */
export function selectBackupRetention(value) {
  const input = retentionRecord(value, ['instanceId', 'archives', 'protectedIds', 'triggerId', 'asOf'], ['keepCount', 'keepDays'])
  check(typeof input.instanceId === 'string' && /^[a-z0-9][a-z0-9-]{0,47}$/.test(input.instanceId), 'RETENTION_INSTANCE_INVALID')
  check(Number.isSafeInteger(input.asOf) && input.asOf > 0, 'RETENTION_TIME_INVALID')
  for (const [key, maximum] of [['keepCount', 10000], ['keepDays', 36500]]) {
    if (Object.hasOwn(input, key)) check(Number.isSafeInteger(input[key]) && input[key] >= 1 && input[key] <= maximum, 'RETENTION_POLICY_INVALID')
  }
  const ids = new Set(), roots = new Set()
  const archives = retentionList(input.archives, 1).map(item => {
    const archive = retentionRecord(item, archiveFields)
    check(['id', 'manifestSha256', 'treeSha256', 'identitySha256'].every(key => typeof archive[key] === 'string' && /^[a-f0-9]{64}$/.test(archive[key])), 'RETENTION_ARCHIVE_INVALID')
    check(archive.instanceId === input.instanceId, 'RETENTION_INSTANCE_MISMATCH')
    check(decimal64(archive.rootDev, false) && decimal64(archive.rootIno, true), 'RETENTION_ARCHIVE_INVALID')
    check(Number.isSafeInteger(archive.createdUnixSeconds) && archive.createdUnixSeconds > 0 &&
      Number.isSafeInteger(archive.logicalBytes) && archive.logicalBytes >= 0, 'RETENTION_ARCHIVE_INVALID')
    check(archive.createdUnixSeconds <= input.asOf, 'RETENTION_FUTURE_ARCHIVE')
    check(!ids.has(archive.id), 'RETENTION_DUPLICATE_ID')
    const root = `${archive.rootDev}:${archive.rootIno}`
    check(!roots.has(root), 'RETENTION_DUPLICATE_ROOT')
    ids.add(archive.id)
    roots.add(root)
    return archive
  })
  const protectedIds = retentionList(input.protectedIds, 0)
  const protectedSet = new Set()
  for (const id of protectedIds) {
    check(typeof id === 'string' && /^[a-f0-9]{64}$/.test(id), 'RETENTION_PROTECTED_INVALID')
    check(!protectedSet.has(id), 'RETENTION_DUPLICATE_PROTECTED_ID')
    check(ids.has(id), 'RETENTION_PROTECTED_ID_UNOBSERVED')
    protectedSet.add(id)
  }
  check(typeof input.triggerId === 'string' && /^[a-f0-9]{64}$/.test(input.triggerId) && ids.has(input.triggerId), 'RETENTION_TRIGGER_INVALID')
  const latestTime = archives.reduce((latest, archive) => Math.max(latest, archive.createdUnixSeconds), 0)
  check(archives.find(archive => archive.id === input.triggerId).createdUnixSeconds === latestTime, 'RETENTION_TRIGGER_NOT_LATEST')
  const ordered = [...archives].sort((left, right) => right.createdUnixSeconds - left.createdUnixSeconds ||
    Number(right.id === input.triggerId) - Number(left.id === input.triggerId) || compareIds(left.id, right.id))
  const keepCount = input.keepCount ?? null, keepDays = input.keepDays ?? null
  const mode = keepCount === null && keepDays === null ? 'keep-all' : 'union'
  const cutoffUnixSeconds = keepDays === null ? null : input.asOf - keepDays * 86400
  check(cutoffUnixSeconds === null || Number.isSafeInteger(cutoffUnixSeconds), 'RETENTION_TIME_INVALID')
  const countIds = new Set(keepCount === null ? [] : ordered.slice(0, keepCount).map(archive => archive.id))
  const latestIds = ordered.filter(archive => archive.createdUnixSeconds === latestTime).map(archive => archive.id)
  const retained = [], candidates = []
  let candidateLogicalBytes = 0
  for (const archive of ordered) {
    const reasons = []
    if (mode === 'keep-all') reasons.push('default-keep-all')
    if (countIds.has(archive.id)) reasons.push('newest-count')
    if (cutoffUnixSeconds !== null && archive.createdUnixSeconds >= cutoffUnixSeconds) reasons.push('within-keep-days')
    if (archive.createdUnixSeconds === latestTime) reasons.push(latestIds.length === 1 ? 'latest-valid-backup' : 'latest-timestamp-tie')
    if (archive.id === input.triggerId) reasons.push('trigger-backup')
    if (protectedSet.has(archive.id)) reasons.push('protected-by-engine')
    if (reasons.length) {
      retained.push({ ...archive, reasons })
    } else {
      candidateLogicalBytes += archive.logicalBytes
      check(Number.isSafeInteger(candidateLogicalBytes), 'RETENTION_LOGICAL_BYTES_OVERFLOW')
      candidates.push({ ...archive, reasons: ['outside-retention-union'] })
    }
  }
  const observationDigest = digest({ instanceId: input.instanceId, archives: [...archives].sort((left, right) => compareIds(left.id, right.id)) })
  const selection = {
    schemaVersion: 1, instanceId: input.instanceId, policy: { mode, keepCount, keepDays },
    asOf: input.asOf, cutoffUnixSeconds, triggerId: input.triggerId, latestIds,
    protectedIds: [...protectedSet].sort(compareIds), observationDigest,
    retained, candidates, retainedIds: retained.map(archive => archive.id), candidateIds: candidates.map(archive => archive.id),
    candidateLogicalBytes, spaceBasis: 'logical-file-bytes-not-reclaimable-disk-space',
    deletionAuthorized: false, requiresApplyRevalidation: true, observationsVerifiedBySelector: false,
  }
  return { ...selection, selectionDigest: digest(selection) }
}
