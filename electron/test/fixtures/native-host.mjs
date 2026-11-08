// Only the Electron presentation boundary is simulated. Product modules,
// filesystem, HTTP, timers and owned subprocesses retain their real behavior.
import { EventEmitter } from 'node:events'
import { readFile } from 'node:fs/promises'
import { createRequire } from 'node:module'
import { pathToFileURL } from 'node:url'
import path from 'node:path'
import vm from 'node:vm'
import * as childProcess from 'node:child_process'

export async function createNativeHost({ electronDirectory, resources, userData, env, version, port }) {
  const require = createRequire(path.join(electronDirectory, 'main.js'))
  const { SERVICE_PANEL_CHANNELS, SERVICE_PANEL_STATE_CHANNEL } = require('./service-panel-ipc')
  const windows = [], children = [], answers = [], handlers = new Map(), updates = new EventEmitter()
  let latest = null, exits = 0
  const app = new EventEmitter()
  app.whenReady = () => Promise.resolve()
  app.isReady = () => true
  app.requestSingleInstanceLock = () => true
  app.getLocale = () => 'en-US'
  app.getVersion = () => version
  app.getPath = name => {
    if (name !== 'userData') throw new Error('NATIVE_HOST_PATH_REJECTED')
    return userData
  }
  app.dock = { show: () => Promise.resolve(), hide() {} }
  app.relaunch = () => { throw new Error('NATIVE_HOST_RELAUNCH_REJECTED') }
  app.quit = () => {
    const event = { prevented: false, preventDefault() { this.prevented = true } }
    app.emit('before-quit', event)
    if (!event.prevented) exits++
  }
  class Window extends EventEmitter {
    constructor() {
      super()
      this.visible = false
      this.destroyed = false
      const contents = new EventEmitter()
      contents.mainFrame = { url: '' }
      contents.getURL = () => contents.mainFrame.url
      contents.isDestroyed = () => this.destroyed
      contents.setWindowOpenHandler = () => {}
      contents.send = (channel, value) => {
        if (channel !== SERVICE_PANEL_STATE_CHANNEL) throw new Error('NATIVE_HOST_CHANNEL_REJECTED')
        latest = value
        updates.emit('state', value)
      }
      this.webContents = contents
      windows.push(this)
    }
    isDestroyed() { return this.destroyed }
    isVisible() { return this.visible }
    show() { this.visible = true; this.emit('show') }
    hide() { this.visible = false; this.emit('hide') }
    focus() {}
    loadFile(filename) {
      if (filename !== path.join(electronDirectory, 'service-panel.html')) throw new Error('NATIVE_HOST_FILE_REJECTED')
      this.webContents.mainFrame.url = pathToFileURL(filename).href
      return Promise.resolve().then(() => this.webContents.emit('did-finish-load'))
    }
    loadURL() { throw new Error('NATIVE_HOST_GUI_REJECTED') }
    close() {
      const event = { prevented: false, preventDefault() { this.prevented = true } }
      this.emit('close', event)
      if (!event.prevented) { this.destroyed = true; this.emit('closed') }
    }
  }
  class Tray extends EventEmitter {
    isDestroyed() { return false }
    setContextMenu() {}
    setToolTip() {}
  }
  const electron = {
    app, BrowserWindow: Window, Tray, Menu: { buildFromTemplate: value => value },
    dialog: { showMessageBox(...args) {
      const options = args.at(-1)
      if (options.defaultId !== 0 || options.cancelId !== 0) throw new Error('NATIVE_HOST_CONFIRMATION_INVALID')
      return Promise.resolve({ response: answers.shift() ?? 0 })
    } },
    shell: { openPath() { throw new Error('NATIVE_HOST_EXTERNAL_OPEN_REJECTED') } },
    clipboard: { writeText() { throw new Error('NATIVE_HOST_CLIPBOARD_REJECTED') } },
    ipcMain: { handle: (channel, fn) => handlers.set(channel, fn), removeHandler: channel => handlers.delete(channel) },
  }
  const instrumentedChildren = {
    spawn(binary, args, options) {
      if (binary !== path.join(resources, 'bin', process.platform === 'win32' ? 'my-api.exe' : 'my-api') ||
          options.cwd !== resources || options.env.SQLITE_PATH !== path.join(userData, 'data', 'my-api.db') ||
          options.env.MYAPI_BIND_ADDRESS !== '127.0.0.1' || options.env.MYAPI_EDITION !== 'lite') {
        throw new Error('NATIVE_HOST_SPAWN_REJECTED')
      }
      const child = childProcess.spawn(binary, args, options)
      const record = { child, generation: children.length + 1, pid: child.pid ?? null,
        requested_signals: [], exit_code: null, signal_code: null, closed: false }
      const originalKill = child.kill.bind(child)
      child.kill = signal => { record.requested_signals.push(signal ?? 'SIGTERM'); return originalKill(signal) }
      record.closedPromise = new Promise(resolve => child.once('close', (code, signal) => {
        record.exit_code = code
        record.signal_code = signal
        record.closed = true
        resolve()
      }))
      children.push(record)
      return child
    },
  }
  const modules = new Set(['crypto', 'path', 'url', 'http', 'fs', './runtime-config', './service-controller',
    './service-panel-ipc', './native-i18n', './service-diagnostics', './public-access-config'])
  const source = await readFile(path.join(electronDirectory, 'main.js'), 'utf8')
  vm.runInNewContext(source, {
    require(name) {
      if (name === 'electron') return electron
      if (name === 'child_process') return instrumentedChildren
      if (!modules.has(name)) throw new Error('NATIVE_HOST_MODULE_REJECTED')
      return require(name)
    },
    __dirname: electronDirectory,
    process: { platform: process.platform, arch: process.arch, argv: ['qualification', '--port', String(port)],
      env: { ...env }, resourcesPath: resources },
    console: { log() {}, error() {} }, AbortController, setTimeout, clearTimeout,
  }, { filename: path.join(electronDirectory, 'main.js') })
  const panel = () => windows.findLast(window => !window.destroyed)
  return {
    children, panel, answers,
    get exits() { return exits },
    invoke(name, ...args) {
      const window = panel()
      const fn = handlers.get(SERVICE_PANEL_CHANNELS[name])
      if (!window || !fn) throw new Error('NATIVE_HOST_PANEL_UNAVAILABLE')
      return fn({ sender: window.webContents, senderFrame: window.webContents.mainFrame }, ...args)
    },
    waitForState(expected, timeout = 65000) {
      return new Promise((resolve, reject) => {
        let timer
        const finish = (error, value) => {
          clearTimeout(timer)
          updates.off('state', receive)
          if (error) reject(error)
          else resolve(value)
        }
        const receive = value => {
          if (value.state === expected) finish(null, value)
          else if (['failed', 'stop-unconfirmed'].includes(value.state)) finish(new Error('NATIVE_HOST_SERVICE_UNREADY'))
        }
        updates.on('state', receive)
        timer = setTimeout(() => finish(new Error('NATIVE_HOST_STATE_TIMEOUT')), timeout)
        if (latest) receive(latest)
      })
    },
  }
}
