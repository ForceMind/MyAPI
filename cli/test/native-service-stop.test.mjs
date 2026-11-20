import assert from 'node:assert/strict'
import { EventEmitter } from 'node:events'
import { PassThrough } from 'node:stream'
import { test } from 'node:test'
import { nativeServiceStopTimeout, openNativeServiceReference, prepareNativeServiceStop, verifyNativeServiceDrain } from '../lib/native-service-stop.mjs'

const timestamp = '2026/11/20 - 12:00:00'
const signal = `[SYS] ${timestamp} | received signal: terminated, shutting down... \n`
const exited = `[SYS] ${timestamp} | server exited \n`

function evidence(overrides = {}) {
  const before = { MainPID: '123', InvocationID: 'a'.repeat(32), ExecMainStartTimestampMonotonic: '1234567' }
  return {
    before,
    after: { ActiveState: 'inactive', SubState: 'dead', MainPID: '0', ControlPID: '0', Result: 'success',
      ExecMainCode: '1', ExecMainStatus: '0', ExecMainPID: before.MainPID,
      ExecMainStartTimestampMonotonic: before.ExecMainStartTimestampMonotonic, InvocationID: before.InvocationID },
    appendedLog: signal + exited,
    ...overrides,
  }
}

test('the captured generation with ordered fresh SYS shutdown entries confirms drain after normal exit', () => {
  assert.deepEqual(verifyNativeServiceDrain(evidence()), { drained: true })
})

test('ordinary saved quota completion between the fixed shutdown markers preserves drain evidence', () => {
  const appendedLog = signal + `[SYS] ${timestamp} | 保存数据看板数据成功，共保存1条数据 \n` + exited
  assert.deepEqual(verifyNativeServiceDrain(evidence({ appendedLog })), { drained: true })
})

test('the real access-log format may finish an in-flight HTTP request while the service drains', () => {
  const appendedLog = signal + `[GIN] ${timestamp} | relay | synthetic-request | 200 |          10ms |       127.0.0.1 |    POST /v1/chat/completions\n` + exited
  assert.deepEqual(verifyNativeServiceDrain(evidence({ appendedLog })), { drained: true })
})

test('service-local calendar timestamps are independent of CLI timezone and wall-clock comparisons', () => {
  for (const localTime of ['2026/11/19 - 23:00:00', '2026/11/21 - 01:00:00', '2026/03/08 - 02:30:00']) {
    const appendedLog = (signal + exited).replaceAll(timestamp, localTime)
    assert.deepEqual(verifyNativeServiceDrain(evidence({ appendedLog })), { drained: true })
  }
})

test('ordinary failed requests and business ERR logs do not imply HTTP or quota drain failure', () => {
  const appendedLog = signal +
    `[GIN] ${timestamp} | web | synthetic-request | 404 |          10ms |       127.0.0.1 |     GET /error\n` +
    `[ERR] ${timestamp} | synthetic-request | provider request failed: synthetic private detail \n` +
    `[SYS] ${timestamp} | unrelated provider request failed \n` + exited
  assert.deepEqual(verifyNativeServiceDrain(evidence({ appendedLog })), { drained: true })
})

test('a manager-cleared InvocationID is accepted only with the retained main PID and start timestamp', () => {
  const proof = evidence()
  proof.after.InvocationID = ''
  assert.deepEqual(verifyNativeServiceDrain(proof), { drained: true })
  proof.after.ExecMainStartTimestampMonotonic = ''
  assert.throws(() => verifyNativeServiceDrain(proof), { code: 'native_stop_unconfirmed' })
})

const invalidLogs = [
  ['empty append', ''],
  ['only old log bytes and no new marker', `[SYS] ${timestamp} | a regular old entry \n`],
  ['signal without completion', signal],
  ['completion without signal', exited],
  ['completion precedes signal', exited + signal],
  ['duplicate signal', signal + signal + exited],
  ['duplicate completion', signal + exited + exited],
  ['unterminated completion', signal + exited.trimEnd()],
  ['wrong signal', signal.replace('terminated', 'interrupt') + exited],
  ['embedded marker in request text', `[INFO] ${timestamp} | SYSTEM | received signal: terminated, shutting down... \n` + exited],
  ['wrong SYS prefix', signal.replace('[SYS]', '[INFO]') + exited],
  ['unprefixed signal', 'received signal: terminated, shutting down...\n' + exited],
  ['multiline payload injection', `[INFO] ${timestamp} | SYSTEM | response body \npayload\n` + signal + exited],
  ['trailing payload on completion', signal + exited.replace('server exited ', 'server exited payload ')],
  ['HTTP forced shutdown with exit zero', signal + `[SYS] ${timestamp} | server forced to shutdown: context deadline exceeded \n` + exited],
  ['quota persistence failed', signal + `[SYS] ${timestamp} | failed to persist quota balance batch drain: synthetic private detail \n` + exited],
  ['flush error', signal + `[SYS] ${timestamp} | increaseQuotaData error: synthetic private detail \n` + exited],
  ['quota drain error after exit marker', signal + exited + `[SYS] ${timestamp} | failed to apply quota balance batch drain: synthetic private detail \n`],
  ['CloseDB failure after exit marker', signal + exited + `[FATAL] ${timestamp} | failed to close database: synthetic private detail \n`],
  ['recovered panic during drain', signal + `[SYS] ${timestamp} | panic detected: synthetic private detail \n` + exited],
  ['fatal entry', signal + `[FATAL] ${timestamp} | synthetic private detail \n` + exited],
  ['invalid timestamp', signal.replace(timestamp, '9999/99/99 - 99:99:99') + exited],
  ['normalized invalid calendar day', signal.replace(timestamp, '2026/11/31 - 12:00:00') + exited],
]

for (const [name, appendedLog] of invalidLogs) {
  test(`${name} cannot establish graceful drain or expose captured log contents`, () => {
    assert.throws(() => verifyNativeServiceDrain(evidence({ appendedLog })), error => {
      assert.equal(error.code, 'native_stop_unconfirmed')
      assert.match(error.message, /无法确认服务正常排空/)
      assert.doesNotMatch(error.message, /synthetic private detail|response body/)
      return true
    })
  })
}

function referenceFixture(options = {}) {
  const child = new EventEmitter()
  child.stdin = new PassThrough()
  child.stdout = new PassThrough()
  const commands = []
  const launches = []
  let ended = false
  const finish = (code = 0) => {
    if (ended) return
    ended = true
    child.emit('close', code, null)
  }
  child.stdin.on('data', bytes => {
    const command = bytes.toString('utf8')
    commands.push(command)
    queueMicrotask(() => {
      if (command === 'check\n') {
        child.stdout.write(options.checkReply ?? 'held\n')
      } else if (command === 'release\n') {
        child.stdout.write(options.releaseReply ?? 'released\n')
        finish(options.exitCode ?? 0)
      }
    })
  })
  child.stdin.on('finish', () => queueMicrotask(() => finish()))
  return {
    commands, launches, child,
    spawn(command, args, config) {
      launches.push({ command, args, config })
      if (options.startup !== null) queueMicrotask(() => {
        child.stdout.write(options.startup ?? 'held\n')
        if (options.earlyExit) finish()
      })
      return child
    },
  }
}

test('a fixed private reference is checked and explicitly released once after proof, with no inherited credentials', async () => {
  const fixture = referenceFixture()
  const reference = await openNativeServiceReference({ scope: 'system', uid: 0 }, fixture.spawn)
  assert.equal(fixture.launches.length, 1)
  assert.equal(fixture.launches[0].command, '/usr/bin/python3')
  const { args, config } = fixture.launches[0]
  assert.deepEqual(args.slice(0, 2), ['-I', '-B'])
  assert.match(args[2], /native-service-ref\.py$/)
  assert.equal(args[3], 'system')
  assert.deepEqual(config.env, { PATH: '/usr/sbin:/usr/bin:/sbin:/bin', LC_ALL: 'C' })
  assert.deepEqual(config.stdio, ['pipe', 'pipe', 'ignore'])
  await reference.check()
  await reference.close()
  await reference.close()
  assert.deepEqual(fixture.commands, ['check\n', 'release\n'])
  await assert.rejects(reference.check(), { code: 'native_stop_unconfirmed' })
})

test('a user reference is connected only to the intended user runtime bus', async () => {
  const fixture = referenceFixture()
  const reference = await openNativeServiceReference({ scope: 'user', uid: 1000, runtime: '/run/user/1000' }, fixture.spawn)
  assert.equal(fixture.launches[0].args[3], 'user')
  assert.equal(fixture.launches[0].config.env.XDG_RUNTIME_DIR, '/run/user/1000')
  assert.equal(fixture.launches[0].config.env.DBUS_SESSION_BUS_ADDRESS, 'unix:path=/run/user/1000/bus')
  await reference.close()
})

for (const [name, options] of [
  ['unrecognized native output', { startup: 'synthetic private detail\n' }],
  ['excessive native output', { startup: 'x'.repeat(1025) }],
  ['early successful exit', { earlyExit: true }],
]) {
  test(`${name} cannot acknowledge a held reference or expose the helper output`, async () => {
    const fixture = referenceFixture(options)
    await assert.rejects(openNativeServiceReference({ scope: 'system', uid: 0 }, fixture.spawn), error => {
      assert.equal(error.code, 'native_stop_unconfirmed')
      assert.doesNotMatch(error.message, /synthetic private detail/)
      return true
    })
    assert.equal(fixture.child.stdin.writableEnded, true)
    assert.deepEqual(fixture.commands, [])
  })
}

test('a lost sender reference fails the check and never reports a successful release', async () => {
  const fixture = referenceFixture({ checkReply: 'unavailable\n' })
  const reference = await openNativeServiceReference({ scope: 'system', uid: 0 }, fixture.spawn)
  await assert.rejects(reference.check(), { code: 'native_stop_unconfirmed' })
  await assert.rejects(reference.close(), { code: 'native_stop_unconfirmed' })
  assert.deepEqual(fixture.commands, ['check\n'])
})

for (const [name, options] of [
  ['nonzero helper exit', { exitCode: 2 }],
  ['EOF without release receipt', { releaseReply: '' }],
  ['unexpected release reply', { releaseReply: 'held\n' }],
]) {
  test(`${name} cannot turn an unconfirmed cleanup into drained success`, async () => {
    const fixture = referenceFixture(options)
    const reference = await openNativeServiceReference({ scope: 'system', uid: 0 }, fixture.spawn)
    await assert.rejects(reference.close(), { code: 'native_stop_unconfirmed' })
    assert.deepEqual(fixture.commands, ['release\n'])
  })
}

test('a missing reference handshake times out safely and closes stdin without issuing a service command', async t => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const fixture = referenceFixture({ startup: null })
  const pending = openNativeServiceReference({ scope: 'system', uid: 0 }, fixture.spawn)
  t.mock.timers.tick(7000)
  await assert.rejects(pending, { code: 'native_stop_unconfirmed' })
  assert.equal(fixture.child.stdin.writableEnded, true)
  assert.deepEqual(fixture.commands, [])
})

test('a reference cannot use an arbitrary scope or another user runtime', async () => {
  const fixture = referenceFixture()
  for (const context of [{ scope: 'external', uid: 0 }, { scope: 'user', uid: 1000, runtime: '/run/user/2000' }]) {
    await assert.rejects(openNativeServiceReference(context, fixture.spawn), { code: 'native_stop_unconfirmed' })
  }
  assert.deepEqual(fixture.launches, [])
})

test('explicit long SSE shutdown windows include cleanup time rather than using a fixed 180-second stop deadline', () => {
  assert.equal(nativeServiceStopTimeout('1800'), 1_920_000)
  assert.equal(nativeServiceStopTimeout('+600'), 720_000)
  assert.equal(nativeServiceStopTimeout(''), 240_000)
})

test('unknown, dotenv-supplied or unrepresentable durations wait for the actual systemd stop result', () => {
  for (const raw of [undefined, 'invalid', '-1', '0', '9223372036854775807']) {
    assert.equal(nativeServiceStopTimeout(raw), 0)
  }
})

const invalidStates = [
  ['still active', 'ActiveState', 'active'],
  ['stop not complete', 'SubState', 'stop-sigterm'],
  ['main process remains', 'MainPID', '123'],
  ['control process remains', 'ControlPID', '222'],
  ['systemd timeout', 'Result', 'timeout'],
  ['signal termination', 'ExecMainCode', '2'],
  ['nonzero exit', 'ExecMainStatus', '1'],
  ['SIGKILL exit', 'ExecMainStatus', '9'],
  ['another retained PID', 'ExecMainPID', '124'],
  ['another invocation', 'InvocationID', 'b'.repeat(32)],
  ['another process generation', 'ExecMainStartTimestampMonotonic', '1234568'],
  ['missing exit status', 'ExecMainStatus', undefined],
]

for (const [name, key, value] of invalidStates) {
  test(`${name} remains unknown even with both apparent shutdown markers`, () => {
    const proof = evidence()
    proof.after[key] = value
    assert.throws(() => verifyNativeServiceDrain(proof), { code: 'native_stop_unconfirmed' })
  })
}

test('unbounded evidence is refused instead of treating a clipped prefix as complete', () => {
  assert.throws(() => verifyNativeServiceDrain(evidence({ appendedLog: signal + ' '.repeat(4 * 1024 * 1024) + '\n' + exited })),
    { code: 'native_stop_unconfirmed' })
})

test('a missing main-generation identity cannot be replaced by an apparent successful stop', () => {
  for (const before of [{}, { ...evidence().before, MainPID: '0' }, { ...evidence().before, InvocationID: '' }]) {
    assert.throws(() => verifyNativeServiceDrain(evidence({ before })), { code: 'native_stop_unconfirmed' })
  }
})

test('a mismatched service scope or name is refused before any manager action', async () => {
  const calls = []
  const context = { scope: 'user', uid: 1000, manager: async args => { calls.push(args); return { stdout: '' } } }
  for (const service of [
    { name: 'my-api.service', scope: 'system', pid: 123 },
    { name: 'external.service', scope: 'user', pid: 123 },
    { name: 'my-api.service', scope: 'user', pid: 0 },
  ]) {
    await assert.rejects(prepareNativeServiceStop(context, service), { code: 'native_stop_unconfirmed' })
  }
  assert.deepEqual(calls, [])
})

test('unknown systemd state never issues stop or discloses manager output', async () => {
  const calls = []
  const context = { scope: 'user', uid: 1000, paths: { unit: '/synthetic/my-api.service' }, unit: 'expected',
    manager: async args => { calls.push(args); throw new Error('synthetic private detail') } }
  await assert.rejects(prepareNativeServiceStop(context, { name: 'my-api.service', scope: 'user', pid: 123 }), error => {
    assert.equal(error.code, 'native_stop_unconfirmed')
    assert.doesNotMatch(error.message, /synthetic private detail/)
    return true
  })
  assert.equal(calls.length, 1)
  assert.equal(calls[0][0], 'show')
})

for (const [name, override] of [
  ['an external unit', { FragmentPath: '/external/my-api.service' }],
  ['a drop-in override', { DropInPaths: '/synthetic/my-api.service.d/override.conf' }],
  ['unloaded changes', { NeedDaemonReload: 'yes' }],
  ['a replacement main process', { MainPID: '124' }],
  ['a service already stopping', { SubState: 'stop-sigterm' }],
]) {
  test(`${name} prevents capture before any stop action or filesystem adoption`, async () => {
    const calls = []
    const state = { LoadState: 'loaded', ActiveState: 'active', SubState: 'running', MainPID: '123',
      ExecMainPID: '123', ControlPID: '0', FragmentPath: '/synthetic/my-api.service', DropInPaths: '',
      NeedDaemonReload: 'no', InvocationID: 'a'.repeat(32), ExecMainStartTimestampMonotonic: '1234567', ...override }
    const context = { scope: 'user', uid: 1000, paths: { unit: '/synthetic/my-api.service' }, unit: 'expected', manager: async args => {
      calls.push(args)
      return { stdout: Object.entries(state).map(([key, value]) => `${key}=${value}`).join('\n') + '\n' }
    } }
    await assert.rejects(prepareNativeServiceStop(context, { name: 'my-api.service', scope: 'user', pid: 123 }),
      { code: 'native_stop_unconfirmed' })
    assert.equal(calls.length, 1)
    assert.equal(calls[0][0], 'show')
  })
}
