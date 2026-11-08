import assert from 'node:assert/strict'
import path from 'node:path'
import test from 'node:test'
import { assessPreservedClone, assessmentTransportTarget, hardeningScope, runContainerHardening, validateRootManifest } from './container-hardening.mjs'

function environment() {
  return { GITHUB_ACTIONS: 'true', GITHUB_EVENT_NAME: 'workflow_dispatch', GITHUB_REF: 'refs/heads/dev',
    GITHUB_SHA: 'a'.repeat(40), GITHUB_RUN_ID: '12345', GITHUB_RUN_ATTEMPT: '2', RUNNER_TEMP: '/tmp/myapi-b6-contract',
    MYAPI_B6_ASSESSMENT: '1', MYAPI_SMOKE_API_ONLY: '1', MYAPI_SMOKE_BROWSER: '0', MYAPI_SMOKE_PERSONAL: '0',
    SMOKE_APP_IMAGE_ID: `sha256:${'b'.repeat(64)}`, SMOKE_BUN_IMAGE_ID: `sha256:${'c'.repeat(64)}` }
}

function manifest(dataMode = 0o755) {
  return [
    { path: '.', type: 'directory', uid: 0, gid: 0, mode: 0o700, size: null, sha256: null },
    ...['data', 'logs', 'config'].map(name => ({ path: name, type: 'directory', uid: 0, gid: 0, mode: name === 'data' ? dataMode : 0o700, size: null, sha256: null })),
    { path: 'data/my-api.db', type: 'file', uid: 0, gid: 0, mode: 0o600, size: 4096, sha256: 'a'.repeat(64) },
    { path: 'logs/myapi-synthetic.log', type: 'file', uid: 0, gid: 0, mode: 0o644, size: 80, sha256: 'b'.repeat(64) },
    { path: 'config/my-api.env', type: 'file', uid: 0, gid: 0, mode: 0o600, size: 128, sha256: 'c'.repeat(64) },
  ]
}

function lifecycle(overrides = {}) {
  const events = []
  const original = manifest()
  const callbacks = {
    original,
    snapshot() { events.push('snapshot'); return structuredClone(original) },
    upstreamCount() { events.push('upstream-count'); return 2 },
    startNonroot() { events.push('nonroot-start') },
    outcome() { events.push('nonroot-outcome'); return { status: 'exited', running: false, exitCode: 1, startExitCode: 0, oomKilled: false, uid: 10001, gid: 10001, processStarted: true, reason: 'permission-denied-startup' } },
    removeNonroot() { events.push('nonroot-remove') },
    startRoot() { events.push('root-start') },
    verifyRoot() { events.push('business-verified') },
    stopRoot() { events.push('root-stop'); return { status: 'exited', running: false, exitCode: 0, oomKilled: false } },
    ...overrides,
  }
  return { events, callbacks }
}

test('manual dev scope derives only owned names and keeps archive size distinct or unknown', () => {
  const env = environment()
  const scope = hardeningScope({ ...env, SMOKE_CONTAINER: 'production', MYAPI_SMOKE_BASE_URL: 'https://production.invalid' })
  assert.equal(scope.root, path.join(env.RUNNER_TEMP, `myapi-b6-${env.GITHUB_SHA}-12345-2`))
  assert.equal(scope.names.source, `myapi-b6-source-${env.GITHUB_SHA}-12345-2`)
  assert.equal(scope.appImage, env.SMOKE_APP_IMAGE_ID)
  assert.equal(scope.archiveBytes, null, 'no archive was created by this assessment')
  assert.equal(hardeningScope({ ...env, SMOKE_BUNDLE_TAR_BYTES: '123456' }).archiveBytes, 123456)
  assert.equal(Object.values(scope.names).includes('production'), false)
})

test('isolated transport maps only fixed fixture origins and preserves each request path and query', () => {
  assert.deepEqual(assessmentTransportTarget('http://127.0.0.1:18080/api/status'), { role: 'fake', url: 'http://127.0.0.1:3000/api/status' })
  assert.deepEqual(assessmentTransportTarget('http://127.0.0.1:18081/api/log/?p=1'), { role: 'reader', url: 'http://127.0.0.1:3000/api/log/?p=1' })
  assert.deepEqual(assessmentTransportTarget('http://127.0.0.1:19090/__smoke__/control'), { role: 'fake', url: 'http://127.0.0.1:19090/__smoke__/control' })
  for (const url of ['https://provider.invalid/v1/chat/completions', 'http://127.0.0.1:3000/api/status',
    'http://localhost:18080/api/status', 'http://user:secret@127.0.0.1:18080/api/status',
    'http://127.0.0.1:18080/api/status#fragment', 'http://127.0.0.1:18080/other']) {
    assert.throws(() => assessmentTransportTarget(url), /B6_TRANSPORT_SCOPE_REJECTED/)
  }
})

test('missing gates, foreign branches, mutable identities and unsafe paths fail before any runner action', async () => {
  for (const changes of [{ GITHUB_ACTIONS: 'false' }, { GITHUB_EVENT_NAME: 'push' }, { GITHUB_REF: 'refs/heads/main' },
    { MYAPI_B6_ASSESSMENT: '0' }, { MYAPI_SMOKE_API_ONLY: '0' }, { MYAPI_SMOKE_BROWSER: '1' }, { MYAPI_SMOKE_PERSONAL: '1' },
    { GITHUB_SHA: 'abc' }, { GITHUB_RUN_ATTEMPT: '0' }, { RUNNER_TEMP: '/' }, { RUNNER_TEMP: 'relative' },
    { RUNNER_TEMP: '/tmp/../production' }, { RUNNER_TEMP: '/tmp/data,dst=/foreign' },
    { SMOKE_APP_IMAGE_ID: 'image:latest' }, { SMOKE_BUN_IMAGE_ID: environment().SMOKE_APP_IMAGE_ID },
    { SMOKE_BUNDLE_TAR_BYTES: 'unknown' }, { SMOKE_BUNDLE_TAR_BYTES: '9007199254740992' }]) {
    await assert.rejects(runContainerHardening({ ...environment(), ...changes }), /B6_/)
  }
})

test('a runner-owned or incomplete fixture cannot be substituted for the root-owned old volume', () => {
  for (const mutate of [
    rows => { rows[0].uid = 1001 },
    rows => { rows[0].mode = 0o755 },
    rows => { rows.find(row => row.path === 'data/my-api.db').sha256 = 'not-a-hash' },
    rows => { rows.find(row => row.path === 'data/my-api.db').type = 'symlink' },
    rows => { rows.find(row => row.path === 'data/my-api.db').path = 'data/../foreign.db' },
    rows => { rows.pop() },
    rows => { rows.push({ ...rows.find(row => row.path === 'data/my-api.db') }) },
  ]) {
    const rows = manifest()
    mutate(rows)
    assert.throws(() => validateRootManifest(rows), /B6_/)
  }
})

test('verified refusal preserves metadata and business state but never authorizes a USER switch', async () => {
  const local = lifecycle()
  const before = structuredClone(local.callbacks.original)
  const report = await assessPreservedClone(local.callbacks)
  assert.deepEqual(local.callbacks.original, before)
  assert.equal(report.assessment_completed, true)
  assert.equal(report.compatibility, false)
  assert.equal(report.default_user_changed, false)
  assert.equal(report.default_user_change_authorized, false)
  assert.equal(report.verdict, 'nonroot-default-not-safe-for-tested-root-volume')
  assert.equal(report.tested_data_mode, '0755')
  assert.equal(report.rejection_stage, 'application')
  assert.equal(report.process_started, true)
  assert.equal(report.refusal_bytes_owner_mode_preserved, true)
  assert.equal(report.original_root_business_state_verified, true)
  assert.match(report.immutable_old_volume_manifest_sha256, /^[a-f0-9]{64}$/)
  assert.equal(report.refusal_manifest_sha256, report.immutable_old_volume_manifest_sha256)
  assert.deepEqual(report.old_volume_manifest, validateRootManifest(before))
  assert.deepEqual(report.fake_upstream_requests, { before_nonroot: 2, after_root_recovery: 2, additional: 0 })
  assert.deepEqual(local.events, ['snapshot', 'upstream-count', 'nonroot-start', 'nonroot-outcome', 'snapshot', 'nonroot-remove', 'root-start', 'business-verified', 'root-stop', 'upstream-count'])
})

test('a metadata-changing copy is rejected before attempting the nonroot process', async () => {
  const changed = manifest()
  changed.find(row => row.path === 'data/my-api.db').mode = 0o644
  const local = lifecycle({ snapshot: () => changed })
  await assert.rejects(assessPreservedClone(local.callbacks), /B6_CLONE_NOT_PRESERVED/)
  assert.deepEqual(local.events, [])
})

test('any byte or permission change during refusal freezes recovery instead of hiding it with root startup', async () => {
  for (const property of ['sha256', 'mode', 'size', 'uid', 'gid']) {
    const local = lifecycle()
    let reads = 0
    local.callbacks.snapshot = () => {
      const rows = manifest()
      if (reads++ === 1) rows.find(row => row.path === 'data/my-api.db')[property] = { sha256: 'd'.repeat(64), mode: 0o644, size: 8192, uid: 1001, gid: 1001 }[property]
      return rows
    }
    await assert.rejects(assessPreservedClone(local.callbacks), property === 'uid' || property === 'gid' ? /B6_ROOT_OWNERSHIP_MISSING/ : /B6_REFUSAL_CHANGED_FIXTURE/)
    assert.equal(local.events.includes('root-start'), false)
    assert.equal(local.events.includes('business-verified'), false)
  }
})

test('running, wrong UID, OOM or unclassified failures are never treated as the expected refusal', async () => {
  for (const changes of [{ status: 'running', running: true }, { uid: 0 }, { gid: 0 }, { oomKilled: true },
    { exitCode: 137 }, { startExitCode: 1 }, { reason: 'unknown' }]) {
    const local = lifecycle()
    const expected = local.callbacks.outcome()
    local.events.length = 0
    local.callbacks.outcome = () => ({ ...expected, ...changes })
    await assert.rejects(assessPreservedClone(local.callbacks), /B6_NONROOT_OUTCOME_UNVERIFIED/)
    assert.equal(local.events.includes('root-start'), false)
  }
})

test('missing evidence, failed business verification and unconfirmed final stop cannot produce a completed report', async () => {
  const missing = lifecycle({ verifyRoot: undefined })
  await assert.rejects(assessPreservedClone(missing.callbacks), /B6_EVIDENCE_CALLBACK_MISSING/)
  assert.deepEqual(missing.events, [])
  const failed = lifecycle({ verifyRoot: () => { throw new Error('B6_EXACT_STATE_MISMATCH') } })
  await assert.rejects(assessPreservedClone(failed.callbacks), /B6_EXACT_STATE_MISMATCH/)
  const uncertain = lifecycle({ stopRoot: () => ({ status: 'running', running: true, exitCode: 0, oomKilled: false }) })
  await assert.rejects(assessPreservedClone(uncertain.callbacks), /B6_STOP_UNCONFIRMED/)
})

test('root recovery must not replay a third synthetic upstream request', async () => {
  let reads = 0
  const local = lifecycle({ upstreamCount: () => ++reads === 1 ? 2 : 3 })
  await assert.rejects(assessPreservedClone(local.callbacks), /B6_UPSTREAM_REPLAY_DETECTED/)
})

test('the separate root 0700 layout records a proved OCI refusal without inventing a process UID', async () => {
  const original = manifest(0o700)
  const refused = { status: 'created', running: false, pid: 0, exitCode: 0, startExitCode: 1,
    oomKilled: false, uid: null, gid: null, processStarted: false, reason: 'oci-workdir-permission-denied' }
  const local = lifecycle({ original, dataMode: 0o700, rejectionStage: 'oci_workdir', snapshot: () => structuredClone(original), outcome: () => refused })
  const report = await assessPreservedClone(local.callbacks)
  assert.equal(report.tested_data_mode, '0700')
  assert.equal(report.rejection_stage, 'oci_workdir')
  assert.equal(report.process_started, false)
  assert.equal(report.nonroot_process_uid, null)
  assert.equal(report.nonroot_process_gid, null)
  assert.equal(report.docker_start_exit_code, 1)
  assert.equal(report.compatibility, false)
  assert.deepEqual(report.old_volume_manifest, validateRootManifest(original, { dataMode: 0o700 }))
  assert.deepEqual(report.fake_upstream_requests, { before_nonroot: 2, after_root_recovery: 2, additional: 0 })
  for (const change of [{ uid: 0 }, { processStarted: true }, { pid: 123 }, { startExitCode: 0 }, { reason: 'unknown' }]) {
    const invalid = lifecycle({ original, dataMode: 0o700, rejectionStage: 'oci_workdir', snapshot: () => structuredClone(original), outcome: () => ({ ...refused, ...change }) })
    await assert.rejects(assessPreservedClone(invalid.callbacks), /B6_OCI_REFUSAL_UNVERIFIED/)
    assert.equal(invalid.events.includes('root-start'), false)
  }
})
