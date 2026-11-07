import assert from 'node:assert/strict'
import { fork } from 'node:child_process'
import { once } from 'node:events'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'
import controller from '../service-controller.js'

test('Actions synthetic owned child is stopped only after its actual close, without terminating an unrelated child', { skip: process.env.GITHUB_ACTIONS !== 'true', timeout: 15000 }, async () => {
  const directory = await mkdtemp(path.join(process.env.RUNNER_TEMP || tmpdir(), 'myapi-desktop-child-'))
  const children = []
  const fixture = fileURLToPath(new URL('./fixtures/service-child.mjs', import.meta.url))
  const launch = () => {
    const child = fork(fixture, [], { cwd: directory, env: { MYAPI_DESKTOP_CHILD_FIXTURE: '1', SystemRoot: process.env.SystemRoot, PATH: process.env.PATH }, stdio: ['ignore', 'ignore', 'ignore', 'ipc'] })
    children.push(child)
    const ready = once(child, 'message').then(([message]) => { assert.deepEqual(message, { ready: true }) })
    return { child, ready }
  }
  try {
    const unrelated = launch()
    await unrelated.ready
    const service = controller.createServiceController({ launch })
    assert.equal((await service.start()).state, 'running')
    const owned = children[1]
    const closed = once(owned, 'close')
    const stopped = await service.stop()
    await closed
    assert.equal(stopped.state, 'stopped')
    assert.equal(stopped.owned, false)
    assert.notEqual(owned.exitCode === null && owned.signalCode === null, true)
    assert.equal(unrelated.child.exitCode, null)
    assert.equal(unrelated.child.signalCode, null)
    const unrelatedClosed = once(unrelated.child, 'close')
    unrelated.child.send('exit')
    await unrelatedClosed
  } finally {
    for (const child of children) {
      if (child.exitCode === null && child.signalCode === null) {
        const closed = once(child, 'close')
        child.kill('SIGKILL')
        await closed
      }
    }
    await rm(directory, { recursive: true, force: true })
  }
})
