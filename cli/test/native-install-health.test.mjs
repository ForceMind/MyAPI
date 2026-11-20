import assert from 'node:assert/strict'
import { test } from 'node:test'
import { waitForService } from '../lib/native-install.mjs'

const paths = { binary: '/synthetic-native/current/my-api' }
const receipt = { version: '1.2.0', edition: 'lite' }
const installed = { dev: 7, ino: 41 }
const healthy = { success: true, data: { version: receipt.version, product_edition: receipt.edition } }
const listener = (pid = '123', address = '127.0.0.1:21952') =>
  `LISTEN 0 128 ${address} 0.0.0.0:* users:(("my-api",pid=${pid},fd=7))\n`

function healthFixture({ installedError, processStat, socket = listener(), status = healthy, changedPid = false } = {}) {
  let elapsed = 0
  const observed = { manager: 0, process: 0, sockets: 0, requests: 0, pauses: 0 }
  const manager = async () => {
    observed.manager++
    const pid = changedPid && observed.manager % 2 === 0 ? '456' : '123'
    return { stdout: `ActiveState=active\nMainPID=${pid}\n` }
  }
  const dependencies = {
    async stat(filename) {
      if (filename === paths.binary) {
        if (installedError) throw installedError
        return installed
      }
      assert.equal(filename, '/proc/123/exe')
      observed.process++
      return processStat ? processStat(observed.process) : installed
    },
    async exec() { observed.sockets++; return { stdout: socket } },
    async fetch() { observed.requests++; return { ok: true, async json() { return status } } },
    now: () => elapsed,
    async delay(milliseconds) { observed.pauses++; elapsed += milliseconds },
  }
  return { observed, wait: () => waitForService(manager, paths, '21952', receipt, dependencies) }
}

for (const code of ['EACCES', 'EPERM']) {
  test(`re-observes transient ${code} on proc identity before requiring complete health evidence`, async () => {
    const f = healthFixture({ processStat(attempt) {
      if (attempt === 1) throw Object.assign(new Error('synthetic proc access restriction'), { code })
      return installed
    } })
    assert.equal(await f.wait(), undefined, 'successful callers retain the original return contract')
    assert.ok(f.observed.process > 1)
    assert.ok(f.observed.pauses > 0)
    assert.equal(f.observed.sockets, 1)
    assert.equal(f.observed.requests, 1)
  })

  test(`permanent ${code} on proc identity refuses without trusting a socket or status response`, async () => {
    const f = healthFixture({ processStat() {
      throw Object.assign(new Error('synthetic private proc detail'), { code })
    } })
    await assert.rejects(f.wait(), error => {
      assert.equal(error.code, 'service_process_unreadable')
      assert.match(error.message, /无法读取服务进程身份/)
      assert.equal(error.message.includes('synthetic private proc detail'), false)
      return true
    })
    assert.ok(f.observed.process > 1)
    assert.equal(f.observed.sockets, 0)
    assert.equal(f.observed.requests, 0)
  })

  test(`${code} on the installed managed binary is rejected immediately, without startup retry`, async () => {
    const failure = Object.assign(new Error('synthetic installed-binary access failure'), { code })
    const f = healthFixture({ installedError: failure })
    await assert.rejects(f.wait(), error => error === failure)
    assert.deepEqual(f.observed, { manager: 0, process: 0, sockets: 0, requests: 0, pauses: 0 })
  })
}

for (const [name, executable] of [
  ['wrong inode', { dev: installed.dev, ino: installed.ino + 1 }],
  ['wrong device', { dev: installed.dev + 1, ino: installed.ino }],
]) {
  test(`${name} never passes health despite a valid-looking listener and API status`, async () => {
    const f = healthFixture({ processStat: () => executable })
    await assert.rejects(f.wait(), { code: 'health_timeout' })
    assert.equal(f.observed.sockets, 0)
    assert.equal(f.observed.requests, 0)
  })
}

for (const [name, socket] of [
  ['another listener PID', listener('456')],
  ['a wildcard listener address', listener('123', '0.0.0.0:21952')],
  ['another loopback port', listener('123', '127.0.0.1:21953')],
]) {
  test(`${name} cannot replace the same-PID exact loopback listener proof`, async () => {
    const f = healthFixture({ socket })
    await assert.rejects(f.wait(), { code: 'health_timeout' })
    assert.ok(f.observed.sockets > 0)
    assert.equal(f.observed.requests, 0)
  })
}

test('a different manager PID after a correct status response does not pass health', async () => {
  const f = healthFixture({ changedPid: true })
  await assert.rejects(f.wait(), { code: 'health_timeout' })
  assert.ok(f.observed.requests > 0)
})

for (const [name, status] of [
  ['wrong version', { success: true, data: { version: '1.2.1', product_edition: 'lite' } }],
  ['wrong edition', { success: true, data: { version: '1.2.0', product_edition: 'full' } }],
  ['unsuccessful status', { success: false, data: healthy.data }],
]) {
  test(`${name} cannot pass with otherwise matching process and listener evidence`, async () => {
    const f = healthFixture({ status })
    await assert.rejects(f.wait(), { code: 'health_timeout' })
    assert.ok(f.observed.requests > 0)
  })
}

test('other proc I/O failures retain immediate refusal instead of being retried as access races', async () => {
  const failure = Object.assign(new Error('synthetic proc I/O failure'), { code: 'EIO' })
  const f = healthFixture({ processStat() { throw failure } })
  await assert.rejects(f.wait(), error => error === failure)
  assert.equal(f.observed.pauses, 0)
  assert.equal(f.observed.requests, 0)
})
