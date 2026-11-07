import assert from 'node:assert/strict'
import { EventEmitter } from 'node:events'
import test from 'node:test'
import controller from '../service-controller.js'

function deferred() {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

function fixture(options = {}) {
  const children = []
  const changes = []
  const scheduled = new Map()
  let nextId = 0
  const timers = {
    setTimeout(callback) { const id = ++nextId; scheduled.set(id, callback); return id },
    clearTimeout(id) { scheduled.delete(id) },
  }
  const service = controller.createServiceController({
    timers,
    launch({ generation, signal }) {
      const ready = deferred()
      const child = new EventEmitter()
      const signals = []
      child.kill = (value) => { signals.push(value); options.kill?.(child, value); return true }
      children.push({ child, ready, generation, signal, signals })
      return { child, ready: ready.promise }
    },
    onChange(snapshot) { changes.push(snapshot); options.onChange?.(snapshot) },
  })
  return {
    service, children, changes, scheduled,
    fireNext() {
      const first = scheduled.entries().next().value
      assert.ok(first, 'a stop deadline must be scheduled')
      scheduled.delete(first[0]); first[1]()
      return first[1]
    },
  }
}

test('start owns one child, shares pending readiness and publishes running only after a successful probe', async () => {
  const f = fixture()
  const initial = f.service.identity()
  const first = f.service.start()
  assert.equal(f.service.start(), first)
  assert.equal(f.children.length, 1)
  assert.equal(f.service.snapshot().state, 'starting')
  assert.equal(f.service.snapshot().owned, true)
  assert.notEqual(f.service.identity(), initial)
  f.children[0].ready.resolve()
  assert.equal((await first).state, 'running')
  assert.equal((await f.service.start()).state, 'running')
  assert.equal(f.children.length, 1)
  assert.deepEqual(f.changes.map((value) => value.state), ['starting', 'running'])
  assert.deepEqual(Object.keys(f.service.snapshot()).sort(), ['generation', 'operation', 'owned', 'pending', 'reason', 'state'])
  assert.ok(Object.isFrozen(f.service.snapshot()))
})

test('stop cancels pending readiness, settles start and waits for actual close before permitting another generation', async () => {
  const f = fixture()
  const start = f.service.start()
  const old = f.children[0]
  const stop = f.service.stop()
  assert.equal(f.service.stop(), stop)
  assert.equal(old.signal.aborted, true)
  assert.equal((await start).state, 'stopping')
  assert.deepEqual(old.signals, ['SIGTERM'])
  assert.equal((await f.service.start()).state, 'stopping')
  assert.equal(f.children.length, 1)
  old.ready.resolve()
  await Promise.resolve()
  assert.equal(f.service.snapshot().state, 'stopping', 'an old probe must not report running')
  old.child.emit('close', 0)
  assert.equal((await stop).state, 'stopped')
  assert.equal(f.scheduled.size, 0)
  const next = f.service.start()
  assert.equal(f.children[1].generation, old.generation + 1)
  f.children[1].ready.resolve()
  assert.equal((await next).state, 'running')
})

test('successful kill return values never prove stop; TERM/KILL deadlines retain the unconfirmed child and block new spawn', async () => {
  const f = fixture()
  const start = f.service.start()
  f.children[0].ready.resolve(); await start
  const child = f.children[0]
  const stop = f.service.stop()
  assert.equal(f.service.snapshot().state, 'stopping')
  const oldGrace = f.fireNext()
  assert.deepEqual(child.signals, ['SIGTERM', 'SIGKILL'])
  assert.equal(f.service.snapshot().owned, true)
  const oldForce = f.fireNext()
  assert.equal((await stop).state, 'stop-unconfirmed')
  assert.equal(f.service.snapshot().reason, 'stop-timeout')
  assert.equal(f.service.snapshot().owned, true)
  assert.equal((await f.service.start()).state, 'stop-unconfirmed')
  assert.equal(f.children.length, 1)
  assert.equal(f.scheduled.size, 0)
  child.child.emit('close', null, 'SIGKILL')
  assert.equal(f.service.snapshot().state, 'stopped')
  assert.equal(f.service.snapshot().owned, false)
  const next = f.service.start()
  oldGrace(); oldForce()
  assert.deepEqual(f.children[1].signals, [], 'expired callbacks cannot target the new child')
  f.children[1].ready.resolve(); await next
})

test('close listeners exist before signals, including a child that closes synchronously during TERM', async () => {
  const f = fixture({ kill(child) { child.emit('close', 0) } })
  const start = f.service.start()
  f.children[0].ready.resolve(); await start
  assert.equal((await f.service.stop()).state, 'stopped')
  assert.equal(f.service.snapshot().owned, false)
  assert.equal(f.scheduled.size, 0)
  assert.deepEqual(f.children[0].signals, ['SIGTERM'])
})

test('readiness failure keeps ownership and disallows duplicate launch until the failed child really closes', async () => {
  const f = fixture()
  const start = f.service.start()
  f.children[0].ready.reject(new Error('private probe detail'))
  assert.equal((await start).state, 'failed')
  assert.equal(f.service.snapshot().reason, 'start-failed')
  assert.equal(f.service.snapshot().owned, true)
  await f.service.start()
  assert.equal(f.children.length, 1)
  const stop = f.service.stop()
  f.children[0].child.emit('close', 0)
  assert.equal((await stop).state, 'stopped')
  assert.ok(!JSON.stringify(f.changes).includes('private probe detail'))
})

test('a launched child without a readiness promise stays owned and unverified until a controlled stop closes it', async () => {
  const child = new EventEmitter()
  const signals = []
  let launches = 0
  child.kill = (signal) => { signals.push(signal); return true }
  const service = controller.createServiceController({ launch() { launches += 1; return { child } } })
  const result = await service.start()
  assert.equal(result.state, 'failed')
  assert.equal(result.reason, 'start-unverified')
  assert.equal(result.owned, true)
  await service.start()
  assert.equal(launches, 1)
  const stop = service.stop()
  assert.deepEqual(signals, ['SIGTERM'])
  child.emit('close', 0)
  assert.equal((await stop).state, 'stopped')
  assert.equal(service.snapshot().owned, false)
})

test('signal errors do not release ownership or claim successful shutdown', async () => {
  const f = fixture({ kill() { throw new Error('signal refused') } })
  const start = f.service.start()
  f.children[0].ready.resolve(); await start
  const stop = f.service.stop()
  f.fireNext(); f.fireNext()
  assert.equal((await stop).state, 'stop-unconfirmed')
  assert.equal(f.service.snapshot().owned, true)
  assert.equal(f.children.length, 1)
  f.children[0].child.emit('close', 0)
})

test('an early process error settles start, and a rejected stale probe cannot rewrite a later generation', async () => {
  const f = fixture()
  const start = f.service.start()
  const old = f.children[0]
  old.child.emit('error', new Error('spawn error'))
  assert.equal((await start).reason, 'process-error')
  old.child.emit('close', 1)
  const next = f.service.start()
  f.children[1].ready.resolve(); await next
  old.ready.reject(new Error('late failure'))
  old.child.emit('close', 1)
  await Promise.resolve()
  assert.equal(f.service.snapshot().state, 'running')
  assert.equal(f.service.snapshot().generation, 2)
})

test('external development services are never launched, stopped or adopted', async () => {
  let launched = false
  const service = controller.createServiceController({ external: true, launch() { launched = true } })
  assert.equal((await service.start()).state, 'external')
  assert.equal((await service.stop()).owned, false)
  assert.equal(launched, false)
  assert.equal(service.identity().child, null)
})

test('a launcher error and presentation callback error cannot fabricate running or interrupt shutdown', async () => {
  const service = controller.createServiceController({ launch() { throw new Error('launch failed') } })
  assert.equal((await service.start()).reason, 'launch-failed')
  assert.equal(service.snapshot().owned, false)
  const f = fixture({ onChange() { throw new Error('UI callback failed') } })
  const start = f.service.start(); f.children[0].ready.resolve(); await start
  const stop = f.service.stop(); f.children[0].child.emit('close', 0)
  assert.equal((await stop).state, 'stopped')
})
