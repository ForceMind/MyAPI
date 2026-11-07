import assert from 'node:assert/strict'
import { EventEmitter } from 'node:events'
import { readFile } from 'node:fs/promises'
import { createRequire } from 'node:module'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import test from 'node:test'
import vm from 'node:vm'
import service from '../service-controller.js'
import ipc from '../service-panel-ipc.js'
import diagnostics from '../service-diagnostics.js'
import publicConfig from '../public-access-config.js'

const require = createRequire(import.meta.url)
const directory = fileURLToPath(new URL('..', import.meta.url))
// Main receives the host's real node:path implementation. Only test-owned
// locations use native separators; explicit database/env values stay verbatim.
const SYNTHETIC_ROOT = path.normalize('/synthetic')
const RESOURCES_DIRECTORY = path.join(SYNTHETIC_ROOT, 'resources')
const USER_DATA_DIRECTORY = path.join(SYNTHETIC_ROOT, 'userData')
const DATA_DIRECTORY = path.join(USER_DATA_DIRECTORY, 'data')
const LOGS_DIRECTORY = path.join(USER_DATA_DIRECTORY, 'logs')
const DATABASE_PATH = path.join(DATA_DIRECTORY, 'my-api.db')
const LANGUAGE_PATH = path.join(USER_DATA_DIRECTORY, 'myapi-desktop-language.json')
const DIAGNOSTIC_PATH = path.join(LOGS_DIRECTORY, 'desktop-service-diagnostic.log')
const PUBLIC_ENTRY_PATH = path.join(USER_DATA_DIRECTORY, 'myapi-desktop-public-access.json')
const CANONICAL_BINARIES = new Set([
  path.join(RESOURCES_DIRECTORY, 'bin', 'my-api'),
  path.join(RESOURCES_DIRECTORY, 'bin', 'my-api.exe'),
])
function deferred() {
  let resolve
  const promise = new Promise((yes) => { resolve = yes })
  return { promise, resolve }
}

async function boot(options = {}) {
  const source = await readFile(new URL('../main.js', import.meta.url), 'utf8')
  const files = new Map()
  if (options.publicRecordText !== undefined) files.set(PUBLIC_ENTRY_PATH, options.publicRecordText)
  const fileOptions = new Map()
  const descriptors = new Map()
  let nextDescriptor = 10
  const children = [], windows = [], probes = [], dialogs = [], answers = [], menus = [], copied = [], openedPaths = [], removedPaths = []
  const timers = new Map()
  const handlers = new Map()
  const updates = new EventEmitter()
  let nextTimer = 0
  let quits = 0
  let exits = 0
  let lastState
  let privateCandidates = options.candidates || ['192.168.1.20']
  let publicReadFailure = false
  const clock = {
    setTimeout(callback, delay) { const id = ++nextTimer; timers.set(id, { callback, delay }); return id },
    clearTimeout(id) { timers.delete(id) },
  }
  const app = new EventEmitter()
  const ready = deferred()
  app.whenReady = () => ready.promise
  app.isReady = () => true
  app.requestSingleInstanceLock = () => true
  app.getLocale = () => options.locale || 'en-US'
  app.getVersion = () => '0.2.0-synthetic'
  app.getPath = (name) => path.join(SYNTHETIC_ROOT, name)
  app.dock = { visible: true, show() { this.visible = true }, hide() { this.visible = false } }
  app.relaunch = (value) => { app.relaunchArguments = value }
  app.quit = () => {
    quits += 1
    const event = { prevented: false, preventDefault() { this.prevented = true } }
    app.emit('before-quit', event)
    if (!event.prevented) exits += 1
  }
  class Window extends EventEmitter {
    constructor(config) {
      super(); this.config = config; this.visible = false; this.destroyed = false
      const contents = new EventEmitter()
      contents.mainFrame = { url: '' }
      contents.isDestroyed = () => this.destroyed
      contents.getURL = () => contents.mainFrame.url
      contents.setWindowOpenHandler = (handler) => { contents.openHandler = handler }
      contents.send = (channel, state) => { assert.equal(channel, ipc.SERVICE_PANEL_STATE_CHANNEL); lastState = state; updates.emit('state', state) }
      this.webContents = contents; windows.push(this)
    }
    static getAllWindows() { return windows.filter((window) => !window.destroyed) }
    isDestroyed() { return this.destroyed }
    isVisible() { return this.visible }
    isMinimized() { return false }
    show() { this.visible = true; this.emit('show') }
    hide() { this.visible = false; this.emit('hide') }
    focus() { this.focused = true }
    restore() { this.visible = true }
    loadFile(file) { this.loadedFile = file; this.webContents.mainFrame.url = pathToFileURL(file).href; return Promise.resolve().then(() => { this.webContents.emit('did-finish-load') }) }
    loadURL(url) { this.loadedURL = url; this.webContents.mainFrame.url = url; return Promise.resolve() }
    close() { const event = { prevented: false, preventDefault() { this.prevented = true } }; this.emit('close', event); if (!event.prevented) this.destroy() }
    destroy() { this.destroyed = true; this.visible = false; this.emit('closed') }
  }
  class Tray extends EventEmitter {
    isDestroyed() { return false }
    setContextMenu(menu) { menus.push(menu) }
    setToolTip(value) { this.tooltip = value }
  }
  const fs = {
    existsSync(file) { return files.has(file) || CANONICAL_BINARIES.has(file) },
    lstatSync(file) {
      if (!files.has(file)) throw Object.assign(new Error('Synthetic missing file'), { code: 'ENOENT' })
      return { isFile: () => file !== PUBLIC_ENTRY_PATH || !options.publicSymlink, isSymbolicLink: () => file === PUBLIC_ENTRY_PATH && options.publicSymlink === true, dev: 1, ino: 1, size: Buffer.byteLength(files.get(file)), mtimeMs: 1, mode: file === PUBLIC_ENTRY_PATH ? (options.publicMode ?? 0o600) : 0o600 }
    },
    readFileSync(file) { if ((options.failPublicRead || publicReadFailure) && file === PUBLIC_ENTRY_PATH) throw Object.assign(new Error('private read detail'), { code: 'EACCES' }); if (!files.has(file)) throw Object.assign(new Error('Synthetic missing file'), { code: 'ENOENT' }); return files.get(file) },
    writeFileSync(target, value, config) { const file = descriptors.get(target)?.file || target; if (options.failDiagnosticWrite && file.endsWith('desktop-service-diagnostic.log') || options.failPublicWrite && file.startsWith(PUBLIC_ENTRY_PATH + '.')) throw new Error('private filesystem detail'); files.set(file, String(value)); if (config) fileOptions.set(file, config) },
    constants: require('node:fs').constants,
    openSync(file, flags, mode) { if (options.failDiagnosticWrite) throw new Error('private filesystem detail'); const descriptor = ++nextDescriptor; descriptors.set(descriptor, { file }); fileOptions.set(file, { flags, mode }); return descriptor },
    fchmodSync(descriptor, mode) { fileOptions.get(descriptors.get(descriptor).file).mode = mode },
    closeSync(descriptor) { descriptors.delete(descriptor) },
    renameSync(from, to) { if (options.failPublicRename && to === PUBLIC_ENTRY_PATH) throw new Error('private rename detail'); files.set(to, files.get(from)); files.delete(from); if (fileOptions.has(from)) { fileOptions.set(to, fileOptions.get(from)); fileOptions.delete(from) } if (options.failPublicReadAfterRename && to === PUBLIC_ENTRY_PATH) publicReadFailure = true },
    unlinkSync(file) { removedPaths.push(file); files.delete(file) },
    appendFileSync(file, value) { files.set(file, (files.get(file) || '') + value) },
    mkdirSync() {}, chmodSync() {},
  }
  const fakeProcess = { platform: options.platform || 'darwin', arch: 'arm64', argv: ['electron', path.join(SYNTHETIC_ROOT, 'app.asar'), ...(options.args || [])], env: { SESSION_SECRET: 'synthetic-session-secret-never-rendered', ...(options.dev ? { NODE_ENV: 'development' } : {}), ...options.env }, resourcesPath: RESOURCES_DIRECTORY }
  const native = require('../native-i18n.js')
  const runtimeModule = { exports: {} }
  const runtimeSource = await readFile(new URL('../runtime-config.js', import.meta.url), 'utf8')
  vm.runInNewContext(runtimeSource, { module: runtimeModule, require(name) {
    if (name === 'net') return require('node:net')
    assert.equal(name, 'os')
    return { networkInterfaces: () => ({ synthetic: privateCandidates.map((address) => ({ address, family: 'IPv4', internal: false })) }) }
  } })
  const runtime = runtimeModule.exports
  const modules = {
    electron: {
      app, BrowserWindow: Window, Tray, Menu: { buildFromTemplate: (menu) => menu },
      dialog: { showMessageBox(...args) { dialogs.push(args.at(-1)); updates.emit('dialog'); return Promise.resolve(answers.shift() || { response: 0 }) } },
      shell: { async openPath(value) { openedPaths.push(value); return '' } },
      clipboard: { writeText(value) { copied.push(value) } },
      ipcMain: { handle(channel, action) { handlers.set(channel, action) }, removeHandler(channel) { handlers.delete(channel) } },
    },
    child_process: { spawn(binary, args, config) {
      const child = new EventEmitter(); child.stdout = new EventEmitter(); child.stderr = new EventEmitter(); child.signals = []
      child.kill = (signal) => { child.signals.push(signal); return true }
      children.push({ child, binary, args, config }); return child
    } },
    http: { get(config, callback) {
      const request = new EventEmitter(); request.destroyed = false; request.destroy = () => { request.destroyed = true }
      probes.push({ config, request, respond(statusCode = 200, body = '{"success":true}') {
        const response = new EventEmitter(); response.statusCode = statusCode; response.setEncoding = () => {}; response.resume = () => {}
        callback(response); response.emit('data', body); response.emit('end')
      } }); return request
    } },
    fs, path, crypto: require('node:crypto'), url: require('node:url'),
    './runtime-config': runtime,
    './service-controller': { createServiceController: (config) => service.createServiceController({ ...config, timers: clock }) },
    './service-panel-ipc': ipc, './native-i18n': native, './service-diagnostics': diagnostics, './public-access-config': publicConfig,
  }
  vm.runInNewContext(source, { require(name) { assert.ok(Object.hasOwn(modules, name), `unexpected main module ${name}`); return modules[name] }, process: fakeProcess, __dirname: directory, console: { log() {}, error() {} }, AbortController, ...clock })
  ready.resolve(); await ready.promise
  const panel = () => windows.findLast((window) => window.loadedFile && !window.destroyed)
  const invoke = (name, ...args) => {
    const current = panel()
    assert.ok(current, 'a live local service panel is required')
    return handlers.get(ipc.SERVICE_PANEL_CHANNELS[name])({ sender: current.webContents, senderFrame: current.webContents.mainFrame }, ...args)
  }
  return {
    app, children, windows, probes, dialogs, answers, menus, copied, openedPaths, removedPaths, files, fileOptions, timers, panel, invoke,
    setCandidates(value) { privateCandidates = value },
    setPublicReadFailure(value) { publicReadFailure = value },
    get quits() { return quits }, get exits() { return exits },
    currentState: () => lastState,
    waitForState(expected) {
      if (lastState?.state === expected) return Promise.resolve(lastState)
      return new Promise((resolve) => { const receive = (state) => { if (state.state === expected) { updates.removeListener('state', receive); resolve(state) } }; updates.on('state', receive) })
    },
    waitForDialog(count) {
      if (dialogs.length >= count) return Promise.resolve()
      return new Promise((resolve) => { const receive = () => { if (dialogs.length >= count) { updates.removeListener('dialog', receive); resolve() } }; updates.on('dialog', receive) })
    },
    fireTimer(delay) {
      const entry = [...timers].find(([, timer]) => timer.delay === delay)
      assert.ok(entry, `expected a ${delay}ms deadline`)
      timers.delete(entry[0]); entry[1].callback()
    },
    translated: (key, language = options.locale || 'en') => native.createNativeI18n({ language }).t(key),
  }
}

async function running(f) {
  const ready = f.waitForState('running')
  f.probes.at(-1).respond(200, '{"success":true}')
  return ready
}

for (const platform of ['darwin', 'win32']) {
  test(`${platform}: real main startup preserves Lite, effective listener, persistent data and isolated local-panel boundaries`, async () => {
    const f = await boot({ platform, args: ['--allow-lan', '--bind-address', '192.168.1.10'], env: { SQL_DSN: 'postgres://synthetic:PRIVATE-DSN@private.invalid/data' } })
    assert.equal(f.children.length, 1)
    const launched = f.children[0]
    assert.equal(launched.binary, path.join(RESOURCES_DIRECTORY, 'bin', platform === 'win32' ? 'my-api.exe' : 'my-api'))
    assert.deepEqual(Array.from(launched.args), ['--log-dir', LOGS_DIRECTORY])
    assert.equal(launched.config.env.MYAPI_EDITION, 'lite')
    assert.equal(launched.config.env.MYAPI_BIND_ADDRESS, '192.168.1.10')
    assert.equal(launched.config.env.FULL_CONTENT_LOG_DIR, path.join(LOGS_DIRECTORY, 'full-content'))
    assert.equal(launched.config.env.SQLITE_PATH, DATABASE_PATH)
    assert.equal(f.probes[0].config.hostname, '192.168.1.10')
    assert.equal(f.probes[0].config.path, '/api/status')
    assert.equal(f.windows.length, 1, 'the local panel does not open an unready console')
    assert.equal(f.panel().config.webPreferences.nodeIntegration, false)
    assert.equal(f.panel().config.webPreferences.contextIsolation, true)
    assert.equal(f.panel().config.webPreferences.sandbox, true)
    const state = await running(f)
    assert.equal(state.applicationVersion, '0.2.0-synthetic')
    assert.equal(Object.hasOwn(state, 'child'), false)
    assert.equal(Object.hasOwn(state, 'identity'), false)
    assert.equal(state.addresses[0], 'http://192.168.1.10:3000/v1')
    assert.ok(Number.isSafeInteger(state.revision))
    assert.ok(!JSON.stringify(state).includes('PRIVATE-DSN'))
    assert.ok(!JSON.stringify(state).includes('synthetic-session-secret-never-rendered'))
    await f.invoke('openConsole')
    assert.equal(f.windows.at(-1).loadedURL, 'http://192.168.1.10:3000')
    assert.equal(f.windows.at(-1).config.webPreferences.preload, path.join(directory, 'preload.js'))
    assert.equal(f.children.length, 1)
  })

  test(`${platform}: closing windows keeps the owned child, cancelled stop is inert, confirmed stop keeps the app and quit waits for close`, async () => {
    const f = await boot({ platform })
    await running(f)
    f.panel().close()
    assert.equal(f.panel().visible, false)
    assert.deepEqual(f.children[0].child.signals, [])
    assert.equal(f.quits, 0)
    f.app.emit('second-instance')
    assert.equal(f.panel().visible, true)
    f.answers.push({ response: 0 })
    await f.invoke('stop')
    assert.deepEqual(f.children[0].child.signals, [])
    f.answers.push({ response: 1 })
    const stop = f.invoke('stop')
    await f.waitForState('stopping')
    assert.deepEqual(f.children[0].child.signals, ['SIGTERM'])
    assert.equal(f.exits, 0)
    f.children[0].child.emit('close', 0)
    const stopped = await stop
    assert.equal(stopped.state, 'stopped')
    assert.equal(f.exits, 0)
    assert.equal(f.timers.size, 0)
    f.answers.push({ response: 1 })
    await f.invoke('start')
    assert.equal(f.children.length, 2)
    await running(f)
    f.answers.push({ response: 1 })
    const quit = f.invoke('quit')
    await f.waitForState('stopping')
    assert.equal(f.exits, 0)
    f.children[1].child.emit('close', 0)
    await quit
    assert.equal(f.exits, 1)
  })
}

test('actual main retries unsuccessful status responses, and stopped startup cancels the probe without late promotion or retry', async () => {
  const f = await boot()
  f.probes[0].respond(503, '{"success":true}')
  assert.equal(f.currentState().state, 'starting')
  f.fireTimer(1000)
  f.probes[1].respond(200, '{"success":false}')
  assert.equal(f.currentState().state, 'starting')
  f.fireTimer(1000)
  f.answers.push({ response: 1 })
  const stop = f.invoke('stop')
  await f.waitForState('stopping')
  assert.equal(f.probes[2].request.destroyed, true)
  f.probes[2].respond(200, '{"success":true}')
  f.children[0].child.emit('close', 0)
  await stop
  assert.equal(f.currentState().state, 'stopped')
  assert.equal(f.timers.size, 0)
  assert.equal(f.probes.length, 3)
})

test('the real main status probe exhausts exactly its existing 30 attempts and retains its unready owned process', async () => {
  const f = await boot()
  const failed = f.waitForState('failed')
  for (let attempt = 0; attempt < 30; attempt += 1) {
    f.probes[attempt].respond(503, '{"success":true}')
    if (attempt < 29) f.fireTimer(1000)
  }
  assert.equal((await failed).owned, true)
  assert.equal(f.probes.length, 30)
  assert.equal(f.timers.size, 0)
  assert.equal(f.windows.length, 1)
})

test('stop-unconfirmed never exits the application or permits another start, even though both kill calls returned true', async () => {
  const f = await boot()
  await running(f)
  f.answers.push({ response: 1 })
  const stop = f.invoke('stop')
  await f.waitForState('stopping')
  f.fireTimer(5000); f.fireTimer(2000)
  const state = await stop
  assert.equal(state.state, 'stop-unconfirmed')
  assert.equal(state.owned, true)
  assert.equal(state.canStart, false)
  assert.equal(f.exits, 0)
  await f.invoke('start')
  assert.equal(f.children.length, 1)
  assert.deepEqual(f.children[0].child.signals, ['SIGTERM', 'SIGKILL'])
})

test('confirmation cannot act after the original local panel is replaced', async () => {
  const f = await boot()
  await running(f)
  const confirmation = deferred()
  f.answers.push(confirmation.promise)
  const stop = f.invoke('stop')
  f.panel().destroy()
  f.app.emit('activate')
  confirmation.resolve({ response: 1 })
  await stop
  assert.deepEqual(f.children[0].child.signals, [])
  assert.equal(f.children.length, 1)
  assert.equal(f.currentState().state, 'running')
})

test('quit can cancel an unfinished startup probe and exits only after the owned child closes', async () => {
  const f = await boot()
  assert.equal(f.currentState().state, 'starting')
  f.answers.push({ response: 1 })
  const quit = f.invoke('quit')
  await f.waitForState('stopping')
  assert.equal(f.probes[0].request.destroyed, true)
  assert.equal(f.exits, 0)
  f.probes[0].respond(200, '{"success":true}')
  f.children[0].child.emit('close', 0)
  await quit
  assert.equal(f.exits, 1)
  assert.equal(f.timers.size, 0)
})

test('native language selection rebuilds the real tray and persists only language while configured paths remain fixed', async () => {
  const f = await boot({ locale: 'zh-CN' })
  await running(f)
  assert.equal(f.menus.at(-1)[0].label, f.translated('Show service panel', 'zh'))
  assert.notEqual(f.menus.at(-1)[0].label, 'Show service panel')
  assert.equal((await f.invoke('setLanguage', 'en')).language, 'en')
  assert.equal(f.menus.at(-1)[0].label, f.translated('Show service panel', 'en'))
  assert.equal(f.files.get(LANGUAGE_PATH), '{"language":"en"}\n')
  await f.invoke('copyKnownAddress', 'http://127.0.0.1:3000/v1')
  assert.deepEqual(f.copied, ['http://127.0.0.1:3000/v1'])
  await f.invoke('copyKnownAddress', 'http://not-configured.invalid/v1')
  assert.equal(f.copied.length, 1)
  await f.invoke('openKnownLogs')
  assert.deepEqual(f.openedPaths, [DIAGNOSTIC_PATH])
})

test('copy uses the exact displayed known address, never a reordered index or an expired address', async () => {
  const first = 'http://192.168.1.20:3000/v1'
  const second = 'http://10.0.0.7:3000/v1'
  const replacement = 'http://10.0.0.8:3000/v1'
  const f = await boot({ args: ['--allow-lan', '--bind-address', '0.0.0.0'], candidates: ['192.168.1.20', '10.0.0.7'] })
  await running(f)
  assert.deepEqual(Array.from((await f.invoke('snapshot')).addresses), [first, second])
  const initialReads = f.probes.length
  await f.invoke('copyKnownAddress', second)
  f.setCandidates(['10.0.0.7', '192.168.1.20'])
  await f.invoke('copyKnownAddress', second)
  assert.deepEqual(f.copied, [second, second])
  f.setCandidates(['192.168.1.20', '10.0.0.8'])
  await f.invoke('copyKnownAddress', second)
  await f.invoke('copyKnownAddress', 'http://not-configured.invalid/v1')
  assert.deepEqual(f.copied, [second, second], 'an expired address cannot be substituted with the new first item')
  await assert.rejects(f.invoke('copyKnownAddress', 1), /Invalid/)
  await assert.rejects(f.invoke('copyKnownAddress', 'a'.repeat(2049)), /Invalid/)
  await f.invoke('copyKnownAddress', replacement)
  assert.deepEqual(f.copied, [second, second, replacement])
  assert.equal(f.probes.length, initialReads)
  assert.deepEqual(f.openedPaths, [])
  assert.equal(f.children.length, 1)
})

test('original stderr stays bounded and private per generation until an explicit diagnostic-open action', async () => {
  const f = await boot()
  await running(f)
  const old = f.children[0].child
  old.stderr.emit('data', 'permission denied PRIVATE-OLD\n')
  assert.equal((await f.invoke('snapshot')).diagnostic.category, 'permission')
  old.emit('close', 1)
  f.answers.push({ response: 1 })
  await f.invoke('start')
  await running(f)
  old.stderr.emit('data', 'out of memory PRIVATE-OLD-LATE\n')
  f.children[1].child.stderr.emit('data', 'connection refused PRIVATE-NEW\n')
  const snapshot = await f.invoke('snapshot')
  assert.equal(snapshot.diagnostic.category, 'network')
  assert.equal(snapshot.diagnostic.hasOriginal, true)
  assert.ok(!JSON.stringify(snapshot).includes('PRIVATE-'))
  const diagnosticPath = DIAGNOSTIC_PATH
  assert.equal(f.files.has(diagnosticPath), false)
  await f.invoke('openKnownLogs')
  const contents = f.files.get(diagnosticPath)
  assert.ok(contents.includes('PRIVATE-NEW'))
  assert.ok(!contents.includes('PRIVATE-OLD'))
  assert.equal(f.fileOptions.get(diagnosticPath).mode, 0o600)
  assert.deepEqual(f.openedPaths, [diagnosticPath])
})

test('a failed explicit diagnostic save never opens another path or returns original stderr or filesystem detail', async () => {
  const f = await boot({ failDiagnosticWrite: true })
  await running(f)
  f.children[0].child.stderr.emit('data', 'database is locked PRIVATE-DATABASE-DETAIL')
  const snapshot = await f.invoke('openKnownLogs')
  assert.equal(snapshot.diagnostic.category, 'database')
  assert.equal(snapshot.notice.tone, 'warning')
  assert.ok(!JSON.stringify(snapshot).includes('PRIVATE-DATABASE-DETAIL'))
  assert.ok(!JSON.stringify(snapshot).includes('private filesystem detail'))
  assert.deepEqual(f.openedPaths, [])
})

test('the preserved seven diagnostic categories and bounded original buffer remain independent across generations', () => {
  for (const [text, category] of [
    ['bind: address already in use', 'port'], ['database is locked', 'database'],
    ['permission denied', 'permission'], ['network is unreachable', 'network'],
    ['failed to parse config', 'configuration'], ['out of memory', 'memory'],
    ['no such file or directory', 'missing-file'], ['unclassified startup detail', 'unknown'],
  ]) {
    assert.equal(diagnostics.classifyServiceError(text), category)
  }
  const old = diagnostics.createServiceDiagnostics(4)
  old.append('x'.repeat(diagnostics.MAX_DIAGNOSTIC_CHARACTERS + 1))
  assert.equal(old.originalText().length, diagnostics.MAX_DIAGNOSTIC_CHARACTERS)
  assert.equal(old.isTruncated(), true)
  const next = diagnostics.createServiceDiagnostics(5)
  assert.equal(next.hasOriginal(), false)
  assert.equal(next.originalText(), '')
  assert.equal(next.category(), 'unknown')
})

test('private-LAN candidate selection defaults to cancel and cannot silently choose the first address', async () => {
  const f = await boot({ candidates: ['192.168.1.20', '10.0.0.7'] })
  await running(f)
  f.answers.push({ response: 0 })
  await f.invoke('restartLAN')
  assert.equal(f.dialogs[0].defaultId, 0)
  assert.equal(f.dialogs[0].cancelId, 0)
  assert.deepEqual(Array.from(f.dialogs[0].buttons), [f.translated('Cancel'), '192.168.1.20', '10.0.0.7'])
  assert.deepEqual(f.children[0].child.signals, [])
  assert.equal(f.app.relaunchArguments, undefined)
})

test('an explicitly selected second private address is confirmed and becomes the exact relaunch target only after close', async () => {
  const f = await boot({ candidates: ['192.168.1.20', '10.0.0.7'] })
  await running(f)
  f.answers.push({ response: 2 }, { response: 1 })
  const restart = f.invoke('restartLAN')
  await f.waitForState('stopping')
  assert.equal(f.app.relaunchArguments, undefined)
  assert.ok(f.dialogs[1].detail.includes('10.0.0.7'))
  f.children[0].child.emit('close', 0)
  await restart
  const args = Array.from(f.app.relaunchArguments.args)
  assert.equal(args[args.indexOf('--bind-address') + 1], '10.0.0.7')
  assert.equal(args[args.indexOf('--port') + 1], '3000')
  assert.equal(args.filter((value) => value === '--allow-lan').length, 1)
  assert.equal(f.exits, 1)
})

test('a selected private address removed during final confirmation is cancelled without substituting another candidate', async () => {
  const f = await boot({ candidates: ['192.168.1.20', '10.0.0.7'] })
  await running(f)
  const confirmation = deferred()
  f.answers.push({ response: 2 }, confirmation.promise)
  const restart = f.invoke('restartLAN')
  // Wait for the real final confirmation boundary, not elapsed time.
  await f.waitForDialog(2)
  assert.equal(f.dialogs.length, 2)
  f.setCandidates(['192.168.1.20'])
  confirmation.resolve({ response: 1 })
  await restart
  assert.deepEqual(f.children[0].child.signals, [])
  assert.equal(f.app.relaunchArguments, undefined)
  assert.equal(f.exits, 0)
})

test('external development mode probes its original frontend path but never adopts or stops an external process', async () => {
  const f = await boot({ dev: true })
  assert.equal(f.children.length, 0)
  assert.equal(f.probes[0].config.port, 5173)
  assert.equal(f.probes[0].config.path, '/')
  assert.equal(f.probes[0].config.hostname, '127.0.0.1')
  f.probes[0].respond(200, '')
  const external = await f.invoke('snapshot')
  assert.equal(external.state, 'external')
  assert.equal(external.owned, false)
  await f.invoke('start'); await f.invoke('stop')
  assert.equal(f.children.length, 0)
  const open = f.invoke('openConsole')
  assert.equal(f.probes[1].config.path, '/')
  f.answers.push({ response: 1 })
  await f.invoke('quit')
  await open
  assert.equal(f.probes[1].request.destroyed, true)
  assert.equal(f.children.length, 0)
  assert.equal(f.windows.length, 1)
  assert.equal(f.exits, 1)
  assert.equal(f.timers.size, 0)
})

test('a public bind is still rejected before a child or console is created', async () => {
  const f = await boot({ args: ['--allow-lan', '--bind-address', '8.8.8.8'] })
  assert.equal(f.children.length, 0)
  assert.equal(f.probes.length, 0)
  assert.equal(f.windows.length, 1, 'only the local diagnostic panel is available')
  assert.equal((await f.invoke('snapshot')).canStart, false)
})

test('public guide saves only an unverified private entry and keeps client, proxy and local-copy sources separate', async () => {
  const f = await boot()
  await running(f)
  const initial = await f.invoke('snapshot')
  assert.deepEqual(JSON.parse(JSON.stringify(initial.publicAccess)), { recordRevision: 0, savedOrigin: null, savedBaseURL: null, status: 'not-configured', verification: 'unknown', busy: false, listenerScope: 'local', proxyTargets: ['http://127.0.0.1:3000'] })
  const requested = await f.invoke('showPublicHelp')
  assert.equal(requested.publicGuideRequest, initial.publicGuideRequest + 1)
  assert.equal(f.dialogs.length, 0)
  const cancelled = await f.invoke('savePublicEntry', 'https://public.example.test', 0)
  assert.equal(cancelled.publicSaveResult.outcome, 'cancelled')
  assert.equal(cancelled.publicSaveResult.unchanged, true)
  assert.equal(f.files.has(PUBLIC_ENTRY_PATH), false, 'default cancellation does not create an entry')
  f.answers.push({ response: 1 })
  const saved = await f.invoke('savePublicEntry', 'https://public.example.test/', 0)
  assert.equal(saved.publicAccess.status, 'unverified')
  assert.equal(saved.publicAccess.verification, 'unknown')
  assert.equal(saved.publicAccess.savedOrigin, 'https://public.example.test')
  assert.equal(saved.publicAccess.savedBaseURL, 'https://public.example.test/v1')
  assert.equal(saved.publicAccess.recordRevision, 1)
  assert.equal(saved.publicAccess.busy, false)
  assert.deepEqual(JSON.parse(JSON.stringify(saved.publicSaveResult)), { outcome: 'saved', expectedRevision: 0, origin: 'https://public.example.test', recordRevision: 1, unchanged: false })
  assert.equal(Object.hasOwn(f.currentState(), 'publicSaveResult'), false)
  assert.equal(Object.hasOwn(await f.invoke('snapshot'), 'publicSaveResult'), false)
  assert.deepEqual(JSON.parse(f.files.get(PUBLIC_ENTRY_PATH)), { schemaVersion: 1, origin: 'https://public.example.test' })
  assert.equal(f.fileOptions.get(PUBLIC_ENTRY_PATH).mode, 0o600)
  await f.invoke('copyPublicBaseURL', saved.publicAccess.savedBaseURL, 0)
  await f.invoke('copyKnownAddress', saved.publicAccess.savedBaseURL)
  assert.equal(f.copied.length, 0, 'public entries never enter the old local copy set')
  await f.invoke('copyPublicBaseURL', saved.publicAccess.savedBaseURL, 1)
  await f.invoke('copyProxyTarget', 'http://127.0.0.1:3000')
  await f.invoke('copyProxyTarget', 'http://127.0.0.1:3000/v1')
  assert.deepEqual(f.copied, ['https://public.example.test/v1', 'http://127.0.0.1:3000'])
  const beforeClear = f.files.get(PUBLIC_ENTRY_PATH)
  await f.invoke('clearPublicEntry', saved.publicAccess.savedOrigin, 1)
  assert.equal(f.files.get(PUBLIC_ENTRY_PATH), beforeClear)
  assert.ok(f.dialogs.at(-1).detail.includes(saved.publicAccess.savedBaseURL))
  f.answers.push({ response: 1 })
  const cleared = await f.invoke('clearPublicEntry', saved.publicAccess.savedOrigin, 1)
  assert.equal(cleared.publicAccess.status, 'not-configured')
  assert.equal(cleared.publicAccess.savedBaseURL, null)
  assert.equal(cleared.publicAccess.recordRevision, 2)
  assert.equal(cleared.publicClearResult.outcome, 'cleared')
  assert.equal(cleared.publicClearResult.unchanged, false)
  assert.deepEqual(JSON.parse(f.files.get(PUBLIC_ENTRY_PATH)), { schemaVersion: 1, origin: null })
  assert.equal(f.removedPaths.includes(PUBLIC_ENTRY_PATH), false)
  await f.invoke('copyPublicBaseURL', saved.publicAccess.savedBaseURL, 1)
  assert.equal(f.copied.length, 2)
  assert.equal(f.children.length, 1)
  assert.deepEqual(f.children[0].child.signals, [])
  assert.equal(f.probes.length, 1, 'no public address, DNS or model probe is added')
  assert.equal(f.openedPaths.length, 0)
  assert.equal(f.app.relaunchArguments, undefined)
})

for (const change of ['frame', 'generation', 'record']) {
  test(`public entry confirmation rejects a changed ${change} instead of writing or replaying`, async () => {
    const f = await boot()
    await running(f)
    const confirmation = deferred()
    f.answers.push(confirmation.promise)
    const pending = f.invoke('savePublicEntry', 'https://first.example.test', 0)
    await f.waitForDialog(1)
    assert.equal((await f.invoke('snapshot')).publicAccess.busy, true)
    await f.invoke('savePublicEntry', 'https://duplicate.example.test', 0)
    assert.equal(f.dialogs.length, 1)
    if (change === 'frame') f.panel().webContents.mainFrame.url = 'https://untrusted.invalid/'
    if (change === 'generation') f.children[0].child.emit('close', 0)
    if (change === 'record') f.files.set(PUBLIC_ENTRY_PATH, JSON.stringify({ schemaVersion: 1, origin: 'https://other.example.test' }))
    confirmation.resolve({ response: 1 })
    const result = await pending
    assert.equal(result.publicAccess.busy, false)
    assert.equal(result.publicSaveResult.outcome, 'rejected')
    if (change === 'record') assert.equal(result.publicAccess.savedOrigin, 'https://other.example.test')
    else assert.equal(f.files.has(PUBLIC_ENTRY_PATH), false)
    assert.equal(f.files.get(PUBLIC_ENTRY_PATH)?.includes('first.example.test') ?? false, false)
    assert.deepEqual(f.copied, [])
    assert.equal(f.probes.length, 1)
  })
}

for (const failure of ['failPublicWrite', 'failPublicRename']) {
  test(`public entry ${failure} preserves the exact previous bytes and saved source`, async () => {
    const original = JSON.stringify({ schemaVersion: 1, origin: 'https://old.example.test' })
    const f = await boot({ publicRecordText: original, [failure]: true })
    await running(f)
    f.answers.push({ response: 1 })
    const result = await f.invoke('savePublicEntry', 'https://new.example.test', 0)
    assert.equal(f.files.get(PUBLIC_ENTRY_PATH), original)
    assert.equal(result.publicAccess.savedOrigin, 'https://old.example.test')
    assert.equal(result.publicAccess.recordRevision, 0)
    assert.equal(result.publicAccess.status, 'unverified')
    assert.equal(result.publicSaveResult.outcome, 'failed')
    assert.equal(result.publicSaveResult.unchanged, true)
    assert.equal(result.notice.message, f.translated('The saved entry could not be updated. The previous record has been retained.'))
    assert.equal(f.removedPaths.includes(PUBLIC_ENTRY_PATH), false)
  })
}

test('a committed save or clear with unreadable read-back is unknown until explicitly read again, never rolled back or reported as success', async () => {
  const f = await boot({ publicRecordText: JSON.stringify({ schemaVersion: 1, origin: 'https://old.example.test' }), failPublicReadAfterRename: true })
  await running(f)
  f.answers.push({ response: 1 })
  const saved = await f.invoke('savePublicEntry', 'https://new.example.test', 0)
  assert.deepEqual(JSON.parse(f.files.get(PUBLIC_ENTRY_PATH)), { schemaVersion: 1, origin: 'https://new.example.test' })
  assert.equal(saved.publicAccess.status, 'unreadable')
  assert.equal(saved.publicAccess.savedOrigin, null)
  assert.equal(saved.publicAccess.savedBaseURL, null)
  assert.equal(saved.publicAccess.recordRevision, 1)
  assert.equal(saved.publicSaveResult.outcome, 'unconfirmed')
  assert.equal(saved.publicSaveResult.unchanged, false)
  assert.equal(saved.notice.message, f.translated('The write result could not be confirmed. Read the saved entry again before taking another action.'))
  await f.invoke('copyPublicBaseURL', 'https://new.example.test/v1', 1)
  assert.deepEqual(f.copied, [])
  f.setPublicReadFailure(false)
  const reread = await f.invoke('snapshot')
  assert.equal(reread.publicAccess.status, 'unverified')
  assert.equal(reread.publicAccess.savedOrigin, 'https://new.example.test')
  assert.equal(reread.publicAccess.recordRevision, 2)
  f.answers.push({ response: 1 })
  const cleared = await f.invoke('clearPublicEntry', reread.publicAccess.savedOrigin, 2)
  assert.deepEqual(JSON.parse(f.files.get(PUBLIC_ENTRY_PATH)), { schemaVersion: 1, origin: null })
  assert.equal(cleared.publicAccess.status, 'unreadable')
  assert.equal(cleared.publicAccess.recordRevision, 3)
  assert.equal(cleared.publicClearResult.outcome, 'unconfirmed')
  assert.equal(cleared.publicClearResult.unchanged, false)
  assert.equal(cleared.notice.message, f.translated('The write result could not be confirmed. Read the saved entry again before taking another action.'))
  assert.equal(f.removedPaths.includes(PUBLIC_ENTRY_PATH), false)
})

test('a corrupt readable entry requires explicit reset to an empty record before a new URL can be saved', async () => {
  const f = await boot({ publicRecordText: '{bad original bytes' })
  await running(f)
  assert.equal((await f.invoke('snapshot')).publicAccess.status, 'unreadable')
  await f.invoke('savePublicEntry', 'https://new.example.test', 0)
  assert.equal(f.dialogs.length, 0)
  assert.equal(f.files.get(PUBLIC_ENTRY_PATH), '{bad original bytes')
  await f.invoke('clearPublicEntry', null, 0)
  assert.equal(f.dialogs.at(-1).message, f.translated("The old entry cannot be read. Reset only this computer's saved entry?"))
  assert.equal(f.files.get(PUBLIC_ENTRY_PATH), '{bad original bytes')
  f.answers.push({ response: 1 })
  const empty = await f.invoke('clearPublicEntry', null, 0)
  assert.equal(empty.publicAccess.status, 'not-configured')
  assert.equal(empty.publicAccess.recordRevision, 1)
  assert.deepEqual(JSON.parse(f.files.get(PUBLIC_ENTRY_PATH)), { schemaVersion: 1, origin: null })
  f.answers.push({ response: 1 })
  const saved = await f.invoke('savePublicEntry', 'https://new.example.test', 1)
  assert.equal(saved.publicAccess.status, 'unverified')
  assert.equal(saved.publicAccess.verification, 'unknown')
  assert.equal(saved.publicAccess.recordRevision, 2)
})

for (const protection of [{ publicSymlink: true }, { publicMode: 0o400 }, { failPublicRead: true }]) {
  test(`an unreadable or unsafe entry is never followed, chmodded or removed: ${Object.keys(protection)[0]}`, async () => {
    const original = JSON.stringify({ schemaVersion: 1, origin: 'https://private.example.test' })
    const f = await boot({ publicRecordText: original, ...protection })
    await running(f)
    assert.equal((await f.invoke('snapshot')).publicAccess.status, 'unreadable')
    const result = await f.invoke('clearPublicEntry', null, 0)
    assert.equal(result.publicAccess.status, 'unreadable')
    assert.equal(f.dialogs.length, 0)
    assert.equal(f.files.get(PUBLIC_ENTRY_PATH), original)
    assert.equal(f.fileOptions.has(PUBLIC_ENTRY_PATH), false)
    assert.deepEqual(f.removedPaths, [])
  })
}

test('a cancelled save cannot acknowledge an external record that happens to match the requested URL', async () => {
  const f = await boot()
  await running(f)
  const confirmation = deferred()
  f.answers.push(confirmation.promise)
  const pending = f.invoke('savePublicEntry', 'https://desired.example.test', 0)
  await f.waitForDialog(1)
  f.files.set(PUBLIC_ENTRY_PATH, JSON.stringify({ schemaVersion: 1, origin: 'https://desired.example.test' }))
  confirmation.resolve({ response: 0 })
  const result = await pending
  assert.equal(result.publicAccess.savedOrigin, 'https://desired.example.test')
  assert.equal(result.publicAccess.recordRevision, 1)
  assert.deepEqual(JSON.parse(JSON.stringify(result.publicSaveResult)), { outcome: 'cancelled', expectedRevision: 0, origin: 'https://desired.example.test', recordRevision: 1, unchanged: false })
  assert.equal(Object.hasOwn(await f.invoke('snapshot'), 'publicSaveResult'), false)
})

test('rejected invalid input is never echoed as an origin in a typed public-save reply', async () => {
  const f = await boot()
  await running(f)
  const result = await f.invoke('savePublicEntry', 'https://user:private-secret@public.example.test', 0)
  assert.equal(result.publicSaveResult.outcome, 'rejected')
  assert.equal(result.publicSaveResult.origin, null)
  assert.equal(JSON.stringify(result).includes('private-secret'), false)
  assert.equal(f.files.has(PUBLIC_ENTRY_PATH), false)
  assert.equal(f.dialogs.length, 0)
})

test('listener scope is a runtime fact while every saved public entry remains unverified', async () => {
  const lan = await boot({ args: ['--allow-lan', '--bind-address', '192.168.1.20'] })
  await running(lan)
  const state = await lan.invoke('snapshot')
  assert.equal(state.publicAccess.listenerScope, 'lan')
  assert.deepEqual(Array.from(state.publicAccess.proxyTargets), ['http://192.168.1.20:3000'])
  assert.equal(state.publicAccess.verification, 'unknown')
  const invalid = await boot({ args: ['--allow-lan', '--bind-address', '8.8.8.8'] })
  const rejected = await invalid.invoke('snapshot')
  assert.equal(rejected.publicAccess.listenerScope, 'unavailable')
  assert.deepEqual(Array.from(rejected.publicAccess.proxyTargets), [])
  assert.equal(invalid.children.length, 0)
})
