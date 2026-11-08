import assert from 'node:assert/strict'
import test from 'node:test'
import { selectBackupRetention } from './backup-retention.mjs'

const asOf = 2_000_000_000
const day = 86400
const id = value => value.toString(16).padStart(64, '0')

function archive(value, createdUnixSeconds, extra = {}) {
  return { id: id(value), instanceId: 'primary', manifestSha256: id(100 + value), treeSha256: id(200 + value), identitySha256: id(300 + value),
    rootDev: '2049', rootIno: (9007199254740993n + BigInt(value)).toString(), createdUnixSeconds, logicalBytes: value * 100, ...extra }
}

function input(extra = {}) {
  return { instanceId: 'primary', asOf, triggerId: id(5), protectedIds: [],
    archives: [archive(1, asOf - 3 * day), archive(2, asOf - 2 * day), archive(3, asOf - day), archive(4, asOf - 100), archive(5, asOf - 10)], ...extra }
}

function freeze(value) {
  if (value && typeof value === 'object') {
    for (const child of Object.values(value)) freeze(child)
    Object.freeze(value)
  }
  return value
}

test('count and days form a union with exact retained and candidate observations', () => {
  const result = selectBackupRetention(input({ keepCount: 4, keepDays: 1 }))
  assert.deepEqual(result.retainedIds, [5, 4, 3, 2].map(id))
  assert.deepEqual(result.candidateIds, [id(1)])
  assert.deepEqual(result.candidates, [{ ...archive(1, asOf - 3 * day), reasons: ['outside-retention-union'] }])
  assert.ok(result.retained.find(value => value.id === id(2)).reasons.includes('newest-count'))
  assert.equal(result.retained.find(value => value.id === id(2)).reasons.includes('within-keep-days'), false)
  assert.equal(result.cutoffUnixSeconds, asOf - day)
  assert.equal(result.candidateLogicalBytes, 100)
  assert.equal(result.spaceBasis, 'logical-file-bytes-not-reclaimable-disk-space')
})

test('count-only, days-only and omitted policy keep their distinct meaning', () => {
  const cases = [
    [{ keepCount: 1 }, [5], [4, 3, 2, 1]],
    [{ keepDays: 1 }, [5, 4, 3], [2, 1]],
    [{ keepCount: 10000 }, [5, 4, 3, 2, 1], []],
    [{}, [5, 4, 3, 2, 1], []],
  ]
  for (const [policy, retained, candidates] of cases) {
    const result = selectBackupRetention(input(policy))
    assert.deepEqual(result.retainedIds, retained.map(id))
    assert.deepEqual(result.candidateIds, candidates.map(id))
  }
  const all = selectBackupRetention(input())
  assert.deepEqual(all.policy, { mode: 'keep-all', keepCount: null, keepDays: null })
  assert.equal(all.cutoffUnixSeconds, null)
  assert.equal(all.candidateLogicalBytes, 0)
  assert.equal(all.retained.every(value => value.reasons.includes('default-keep-all')), true)
  assert.equal(all.deletionAuthorized, false)
  assert.equal(all.requiresApplyRevalidation, true)
  assert.equal(all.observationsVerifiedBySelector, false)
})

test('all referenced or active-use IDs remain protected beyond the count window', () => {
  const result = selectBackupRetention(input({ keepCount: 1, protectedIds: [id(2), id(1)] }))
  assert.deepEqual(result.retainedIds, [5, 2, 1].map(id))
  assert.deepEqual(result.candidateIds, [4, 3].map(id))
  assert.deepEqual(result.protectedIds, [1, 2].map(id))
  assert.equal(result.retained.filter(value => value.reasons.includes('protected-by-engine')).length, 2)
  assert.ok(result.retained.find(value => value.id === id(5)).reasons.includes('trigger-backup'))
})

test('the exact days cutoff is inclusive and an old latest trigger still survives', () => {
  const result = selectBackupRetention(input({ keepDays: 1 }))
  assert.ok(result.retained.find(value => value.id === id(3)).reasons.includes('within-keep-days'))
  const old = selectBackupRetention(input({ keepDays: 1, triggerId: id(2), archives: [archive(1, asOf - 4 * day), archive(2, asOf - 3 * day)] }))
  assert.deepEqual(old.retainedIds, [id(2)])
  assert.deepEqual(old.candidateIds, [id(1)])
  assert.deepEqual(old.retained[0].reasons, ['latest-valid-backup', 'trigger-backup'])
})

test('all latest timestamp ties survive, trigger comes first and other ties have stable ID order', () => {
  const result = selectBackupRetention(input({ keepCount: 1, archives: [archive(5, asOf), archive(3, asOf), archive(1, asOf - 1), archive(4, asOf)] }))
  assert.deepEqual(result.latestIds, [5, 3, 4].map(id))
  assert.deepEqual(result.retainedIds, [5, 3, 4].map(id))
  assert.deepEqual(result.candidateIds, [id(1)])
  assert.equal(result.retained.every(value => value.reasons.includes('latest-timestamp-tie')), true)
  assert.ok(result.retained[0].reasons.includes('trigger-backup'))
})

test('a missing or older trigger and future archive clocks are rejected, never guessed', () => {
  assert.throws(() => selectBackupRetention(input({ triggerId: id(9) })), { code: 'RETENTION_TRIGGER_INVALID' })
  assert.throws(() => selectBackupRetention(input({ triggerId: id(1) })), { code: 'RETENTION_TRIGGER_NOT_LATEST' })
  assert.throws(() => selectBackupRetention(input({ archives: [archive(5, asOf + 1)] })), { code: 'RETENTION_FUTURE_ARCHIVE' })
  for (const value of [0, -1, NaN, Infinity, Number.MAX_SAFE_INTEGER + 1, '2000000000']) {
    assert.throws(() => selectBackupRetention(input({ asOf: value })), { code: 'RETENTION_TIME_INVALID' })
  }
})

test('policy bounds and explicit unknown values cannot silently become keep-all or zero', () => {
  for (const key of ['keepCount', 'keepDays']) {
    for (const value of [0, -1, 1.5, NaN, Infinity, null, undefined, '1', true]) {
      assert.throws(() => selectBackupRetention(input({ [key]: value })), { code: 'RETENTION_POLICY_INVALID' })
    }
  }
  assert.throws(() => selectBackupRetention(input({ keepCount: 10001 })), { code: 'RETENTION_POLICY_INVALID' })
  assert.throws(() => selectBackupRetention(input({ keepDays: 36501 })), { code: 'RETENTION_POLICY_INVALID' })
  assert.deepEqual(selectBackupRetention(input({ keepDays: 36500 })).candidateIds, [])
})

test('missing fields, unsafe numbers, duplicates and cross-instance observations fail closed', () => {
  const base = archive(5, asOf)
  const missing = { ...base }
  delete missing.logicalBytes
  for (const record of [missing, { ...base, logicalBytes: null }, { ...base, logicalBytes: NaN }, { ...base, logicalBytes: Infinity },
    { ...base, logicalBytes: Number.MAX_SAFE_INTEGER + 1 }, { ...base, logicalBytes: -1 }, { ...base, createdUnixSeconds: 0 },
    { ...base, createdUnixSeconds: '1' }, { ...base, createdUnixSeconds: Number.MAX_SAFE_INTEGER + 1 }, { ...base, manifestSha256: 'unknown' }]) {
    assert.throws(() => selectBackupRetention(input({ archives: [record] })))
  }
  assert.throws(() => selectBackupRetention(input({ archives: [base, { ...base }] })), { code: 'RETENTION_DUPLICATE_ID' })
  assert.throws(() => selectBackupRetention(input({ archives: [archive(4, asOf - 1, { rootIno: base.rootIno }), base] })), { code: 'RETENTION_DUPLICATE_ROOT' })
  assert.throws(() => selectBackupRetention(input({ archives: [{ ...base, instanceId: 'secondary' }] })), { code: 'RETENTION_INSTANCE_MISMATCH' })
  assert.throws(() => selectBackupRetention(input({ archives: [] })), { code: 'RETENTION_LIST_INVALID' })
  assert.throws(() => selectBackupRetention(input({ protectedIds: [id(1), id(1)] })), { code: 'RETENTION_DUPLICATE_PROTECTED_ID' })
  assert.throws(() => selectBackupRetention(input({ protectedIds: [id(9)] })), { code: 'RETENTION_PROTECTED_ID_UNOBSERVED' })
})

test('uint64 device and inode strings retain exact precision and reject ambiguous encodings', () => {
  const largest = '18446744073709551615'
  const result = selectBackupRetention(input({ archives: [archive(5, asOf, { rootDev: '0', rootIno: largest, logicalBytes: 0 })] }))
  assert.equal(result.retained[0].rootIno, largest)
  assert.equal(result.retained[0].rootDev, '0')
  assert.equal(result.retained[0].logicalBytes, 0)
  for (const value of ['18446744073709551616', '00', '01', '-1', '+1', '1.0', '1e3', ' 1', 1, null]) {
    assert.throws(() => selectBackupRetention(input({ archives: [archive(5, asOf, { rootIno: value })] })), { code: 'RETENTION_ARCHIVE_INVALID' })
  }
  assert.throws(() => selectBackupRetention(input({ archives: [archive(5, asOf, { rootIno: '0' })] })), { code: 'RETENTION_ARCHIVE_INVALID' })
})

test('observation order and protected order do not mutate input or change the selection', () => {
  const original = input({ keepCount: 2, keepDays: 1, protectedIds: [id(2), id(1)] })
  const before = structuredClone(original)
  freeze(original)
  const result = selectBackupRetention(original)
  assert.deepEqual(original, before)
  const reordered = { ...before, archives: [...before.archives].reverse(), protectedIds: [...before.protectedIds].reverse() }
  assert.deepEqual(selectBackupRetention(reordered), result)
  result.retained[0].logicalBytes = 0
  result.retained[0].reasons.push('local-consumer-edit')
  assert.deepEqual(original, before, 'output must not share mutable archive objects with the observation input')
})

test('observations, policy, cutoff, trigger and protections are bound to distinct stable digests', () => {
  const original = input({ keepCount: 1 })
  const result = selectBackupRetention(original)
  assert.match(result.observationDigest, /^[a-f0-9]{64}$/)
  assert.match(result.selectionDigest, /^[a-f0-9]{64}$/)
  const changed = structuredClone(original)
  changed.archives[0].treeSha256 = id(999)
  assert.notEqual(selectBackupRetention(changed).observationDigest, result.observationDigest)
  assert.notEqual(selectBackupRetention(changed).selectionDigest, result.selectionDigest)
  for (const extra of [{ keepCount: 2 }, { keepDays: 1 }, { asOf: asOf + 1 }, { protectedIds: [id(1)] }]) {
    const next = selectBackupRetention({ ...original, ...extra })
    assert.equal(next.observationDigest, result.observationDigest)
    assert.notEqual(next.selectionDigest, result.selectionDigest)
  }
  const tied = input({ keepCount: 1, archives: [archive(4, asOf), archive(5, asOf)] })
  const left = selectBackupRetention(tied), right = selectBackupRetention({ ...tied, triggerId: id(4) })
  assert.equal(left.observationDigest, right.observationDigest)
  assert.notEqual(left.selectionDigest, right.selectionDigest)
})

test('logical candidate-byte addition cannot overflow or claim released filesystem space', () => {
  const archives = [archive(1, asOf - 3, { logicalBytes: Number.MAX_SAFE_INTEGER }), archive(2, asOf - 2, { logicalBytes: 1 }), archive(5, asOf)]
  assert.throws(() => selectBackupRetention(input({ archives, keepCount: 1 })), { code: 'RETENTION_LOGICAL_BYTES_OVERFLOW' })
  assert.equal(selectBackupRetention(input()).deletionAuthorized, false)
})

test('qualification booleans, proc paths, accessors and sparse inputs are never accepted as observations', () => {
  for (const key of ['verified', 'caller-verified', 'caller_verified', 'proc_root', 'path', 'state', 'succeeded']) {
    assert.throws(() => selectBackupRetention(input({ [key]: true })), { code: 'RETENTION_RECORD_INVALID' })
    assert.throws(() => selectBackupRetention(input({ archives: [archive(5, asOf, { [key]: true })] })), { code: 'RETENTION_RECORD_INVALID' })
  }
  let called = false
  const getter = input()
  Object.defineProperty(getter, 'archives', { enumerable: true, get() { called = true; return [] } })
  assert.throws(() => selectBackupRetention(getter), { code: 'RETENTION_RECORD_INVALID' })
  assert.equal(called, false)
  assert.throws(() => selectBackupRetention(input({ archives: new Array(1) })), { code: 'RETENTION_LIST_INVALID' })
  assert.throws(() => selectBackupRetention(new Proxy(input(), {})), { code: 'RETENTION_RECORD_INVALID' })
})
