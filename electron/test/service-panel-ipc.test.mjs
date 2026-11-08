import assert from 'node:assert/strict'
import { EventEmitter } from 'node:events'
import { readFile } from 'node:fs/promises'
import test from 'node:test'
import vm from 'node:vm'
import ipc from '../service-panel-ipc.js'

function fixture() {
  const handlers = new Map()
  const panelURL = 'file:///synthetic/service-panel.html'
  let current
  const calls = []
  const actions = Object.fromEntries(Object.keys(ipc.SERVICE_PANEL_CHANNELS).map((name) => [name, (context, ...args) => { calls.push({ name, context, args }); return { accepted: true } }]))
  const window = () => {
    const mainFrame = { url: panelURL }
    return { isDestroyed: () => false, webContents: { mainFrame, isDestroyed: () => false, getURL: () => mainFrame.url } }
  }
  current = window()
  const registration = ipc.createServicePanelIPC({
    ipcMain: { handle(name, handler) { handlers.set(name, handler) }, removeHandler(name) { handlers.delete(name) } },
    getWindow: () => current, panelURL, actions,
  })
  const event = () => ({ sender: current.webContents, senderFrame: current.webContents.mainFrame })
  return { handlers, calls, actions, registration, event, window, get current() { return current }, set current(value) { current = value } }
}

test('only the exact current local main frame may invoke a fixed native panel action', async () => {
  const f = fixture()
  const invoke = f.handlers.get(ipc.SERVICE_PANEL_CHANNELS.snapshot)
  assert.deepEqual(await invoke(f.event()), { accepted: true })
  assert.equal(f.calls[0].context.isTrusted(), true)
  const trusted = f.event()
  for (const event of [
    { ...trusted, sender: { ...trusted.sender } },
    { ...trusted, senderFrame: { url: trusted.senderFrame.url } },
    { ...trusted, senderFrame: undefined },
  ]) {
    await assert.rejects(invoke(event), /Untrusted/)
  }
  f.current.webContents.mainFrame.url = 'https://untrusted.invalid/'
  await assert.rejects(invoke(trusted), /Untrusted/)
  f.current = f.window()
  await assert.rejects(invoke(trusted), /Untrusted/, 'an old panel cannot act on its replacement')
  f.current.isDestroyed = () => true
  await assert.rejects(invoke(f.event()), /Untrusted/)
  assert.equal(f.calls.length, 1)
})

test('panel methods reject arbitrary argument payloads and language or address coercion', async () => {
  const f = fixture()
  for (const [name, args] of [
    ['start', ['shell command']], ['stop', [7]], ['quit', [{}]],
    ['openKnownLogs', ['/private/arbitrary']], ['openConsole', ['https://other.invalid']], ['restartLAN', ['8.8.8.8']],
    ['setLanguage', ['../zh']], ['setLanguage', ['en', 'extra']],
    ['copyKnownAddress', ['']], ['copyKnownAddress', ['a'.repeat(2049)]], ['copyKnownAddress', [0]], ['copyKnownAddress', [-1]], ['copyKnownAddress', [0.5]], ['copyKnownAddress', [Infinity]],
    ['savePublicEntry', ['https://public.example.test']], ['savePublicEntry', ['https://public.example.test', -1]], ['savePublicEntry', ['a'.repeat(2049), 0]],
    ['clearPublicEntry', [null]], ['clearPublicEntry', [{}, 0]], ['clearPublicEntry', [null, NaN]],
    ['copyPublicBaseURL', ['a'.repeat(2052), 0]], ['copyPublicBaseURL', ['https://public.example.test/v1', '0']],
    ['checkPublicEntry', ['https://public.example.test']], ['checkPublicEntry', ['https://public.example.test', -1]], ['checkPublicEntry', ['https://public.example.test', 0, { rejectUnauthorized: false }]],
    ['checkPublicEntry', ['a'.repeat(2049), 0]], ['checkPublicEntry', [null, 0]], ['checkPublicEntry', ['https://public.example.test', Number.MAX_SAFE_INTEGER + 1]], ['checkPublicEntry', ['https://public.example.test', 0.5]], ['checkPublicEntry', ['https://public.example.test', NaN]], ['checkPublicEntry', ['https://public.example.test', Infinity]], ['checkPublicEntry', ['https://public.example.test', '0']],
    ['cancelPublicEntryCheck', []], ['cancelPublicEntryCheck', ['A'.repeat(32)]], ['cancelPublicEntryCheck', ['0'.repeat(31)]], ['cancelPublicEntryCheck', ['0'.repeat(32), 0]], ['cancelPublicEntryCheck', [0]],
    ['copyProxyTarget', [0]], ['copyProxyTarget', ['http://127.0.0.1:3000', 'extra']],
  ]) {
    await assert.rejects(f.handlers.get(ipc.SERVICE_PANEL_CHANNELS[name])(f.event(), ...args), /Invalid|Unexpected/)
  }
  assert.equal(f.calls.length, 0)
  await f.handlers.get(ipc.SERVICE_PANEL_CHANNELS.setLanguage)(f.event(), 'zh')
  await f.handlers.get(ipc.SERVICE_PANEL_CHANNELS.copyKnownAddress)(f.event(), 'http://127.0.0.1:3000/v1')
  await f.handlers.get(ipc.SERVICE_PANEL_CHANNELS.copyKnownAddress)(f.event(), 'a'.repeat(2048))
  assert.deepEqual(f.calls.map(({ name, args }) => ({ name, args })), [
    { name: 'setLanguage', args: ['zh'] }, { name: 'copyKnownAddress', args: ['http://127.0.0.1:3000/v1'] }, { name: 'copyKnownAddress', args: ['a'.repeat(2048)] },
  ])
  f.registration.dispose()
  assert.equal(f.handlers.size, 0)
})

test('a confirmation context rechecks panel navigation rather than retaining the initial trust decision', async () => {
  const f = fixture()
  let release
  const confirmation = new Promise((resolve) => { release = resolve })
  f.actions.stop = async (context) => { await confirmation; return { accepted: context.isTrusted() } }
  const result = f.handlers.get(ipc.SERVICE_PANEL_CHANNELS.stop)(f.event())
  f.current.webContents.mainFrame.url = 'file:///synthetic/replaced.html'
  release()
  assert.deepEqual(await result, { accepted: false })
})

test('the real preload exposes only named operations and a removable state subscription, never generic invoke', async () => {
  const source = await readFile(new URL('../service-panel-preload.js', import.meta.url), 'utf8')
  let bridge
  const renderer = new EventEmitter()
  const invoked = []
  renderer.invoke = (channel, ...args) => { invoked.push({ channel, args }); return Promise.resolve({ state: 'stopped' }) }
  vm.runInNewContext(source, { require(name) {
    assert.equal(name, 'electron')
    return { contextBridge: { exposeInMainWorld(name, value) { assert.equal(name, 'myapiService'); bridge = value } }, ipcRenderer: renderer }
  } })
  assert.deepEqual(Object.keys(bridge).sort(), [...Object.keys(ipc.SERVICE_PANEL_CHANNELS), 'onState'].sort())
  await bridge.stop(); await bridge.copyKnownAddress('http://127.0.0.1:3000/v1')
  await bridge.savePublicEntry('https://public.example.test', 0)
  await bridge.clearPublicEntry(null, 1)
  await bridge.copyPublicBaseURL('https://public.example.test/v1', 2)
  await bridge.copyProxyTarget('http://127.0.0.1:3000')
  await bridge.checkPublicEntry('https://public.example.test', 2)
  await bridge.cancelPublicEntryCheck('a'.repeat(32))
  assert.deepEqual(invoked, [
    { channel: ipc.SERVICE_PANEL_CHANNELS.stop, args: [] },
    { channel: ipc.SERVICE_PANEL_CHANNELS.copyKnownAddress, args: ['http://127.0.0.1:3000/v1'] },
    { channel: ipc.SERVICE_PANEL_CHANNELS.savePublicEntry, args: ['https://public.example.test', 0] },
    { channel: ipc.SERVICE_PANEL_CHANNELS.clearPublicEntry, args: [null, 1] },
    { channel: ipc.SERVICE_PANEL_CHANNELS.copyPublicBaseURL, args: ['https://public.example.test/v1', 2] },
    { channel: ipc.SERVICE_PANEL_CHANNELS.copyProxyTarget, args: ['http://127.0.0.1:3000'] },
    { channel: ipc.SERVICE_PANEL_CHANNELS.checkPublicEntry, args: ['https://public.example.test', 2] },
    { channel: ipc.SERVICE_PANEL_CHANNELS.cancelPublicEntryCheck, args: ['a'.repeat(32)] },
  ])
  const values = []
  const unsubscribe = bridge.onState((value) => values.push(value))
  const state = { state: 'stop-unconfirmed' }
  renderer.emit(ipc.SERVICE_PANEL_STATE_CHANNEL, { sender: 'not exposed' }, state)
  unsubscribe()
  renderer.emit(ipc.SERVICE_PANEL_STATE_CHANNEL, {}, { state: 'running' })
  assert.deepEqual(values, [state])
  assert.equal(renderer.listenerCount(ipc.SERVICE_PANEL_STATE_CHANNEL), 0)
})


test('public check methods retain captured frame trust and forward only fixed origin/revision or exact ID', async () => {
  const f = fixture()
  const start = f.handlers.get(ipc.SERVICE_PANEL_CHANNELS.checkPublicEntry)
  const cancel = f.handlers.get(ipc.SERVICE_PANEL_CHANNELS.cancelPublicEntryCheck)
  const trusted = f.event()
  await start(trusted, 'https://public.example.test', 0)
  await cancel(trusted, 'b'.repeat(32))
  assert.deepEqual(f.calls.map(({ name, args }) => ({ name, args })), [
    { name: 'checkPublicEntry', args: ['https://public.example.test', 0] },
    { name: 'cancelPublicEntryCheck', args: ['b'.repeat(32)] },
  ])
  assert.equal(f.calls[0].context.isTrusted(), true)
  f.current = f.window()
  assert.equal(f.calls[0].context.isTrusted(), false)
  assert.equal(f.calls[1].context.isTrusted(), false)
  await assert.rejects(start(trusted, 'https://public.example.test', 0), /Untrusted/)
  await assert.rejects(cancel(trusted, 'b'.repeat(32)), /Untrusted/)
  await assert.rejects(start({ ...f.event(), senderFrame: null }, 'https://public.example.test', 0), /Untrusted/)
  assert.equal(f.calls.length, 2)
})
