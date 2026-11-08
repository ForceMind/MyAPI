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
  const { createServiceDiagnostics, classifyServiceError } = require('./service-diagnostics')
  const childDiagnostics = new WeakMap()
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
      // Capture both Go writer streams independently of the presentation
      // console. Each owned generation has its own bounded private buffer;
      // neither original text nor paths enter the qualification report.
      const diagnostics = createServiceDiagnostics(record.generation)
      childDiagnostics.set(record, diagnostics)
      child.stdout?.on('data', value => diagnostics.append(value))
      child.stderr?.on('data', value => diagnostics.append(value))
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
          else if (['failed', 'stop-unconfirmed'].includes(value.state)) {
            const record = children.findLast(child => child.generation === value.generation)
            const original = childDiagnostics.get(record)?.originalText() ?? ''
            // Go FatalLog emits this fixed marker before exiting. Prefer its
            // last bounded evidence over an earlier nonfatal startup warning.
            const fatalOffset = original.lastIndexOf('[FATAL]')
            const evidence = fatalOffset < 0 ? original : original.slice(fatalOffset)
            const categories = {
              port: 'HTTP_START_OR_BIND', database: 'DATABASE', permission: 'PERMISSION',
              network: 'NETWORK', configuration: 'CONFIGURATION', memory: 'MEMORY',
              'missing-file': 'MISSING_FILE', unknown: 'UNKNOWN',
            }
            let category = categories[classifyServiceError(evidence)] ?? 'UNKNOWN'
            if (category === 'UNKNOWN' && fatalOffset >= 0) {
              if (/failed to initialize database:/.test(evidence)) category = 'DATABASE_INITIALIZATION'
              else if (/failed to initialize authorization:/.test(evidence)) category = 'AUTHORIZATION_INITIALIZATION'
              else if (/failed to initialize resources:/.test(evidence)) category = 'RESOURCE_INITIALIZATION'
            }
            let failure = 'NATIVE_HOST_SERVICE_UNREADY_' + category
            if (fatalOffset >= 0 && evidence.includes('Lite configuration check')) {
              failure = 'NATIVE_HOST_LITE_UNKNOWN'
              const fatalLine = evidence.split(/\r?\n/, 1)[0]
              const gate = /^\[FATAL\] \d{4}\/\d{2}\/\d{2} - \d{2}:\d{2}:\d{2} \| \[failed to initialize resources: Lite configuration check (blocked|unverifiable) \(([a-z-]+(?:, [a-z-]+)*)\); run my-api --edition-check=lite --json and resolve the listed conditions in Full before retrying\] $/.exec(fatalLine)
              // Never turn arbitrary captured text into an error message. The
              // two states and nine issue codes define a fixed finite set.
              const states = { blocked: 'BLOCKED', unverifiable: 'UNVERIFIABLE' }
              const issues = {
                'pending-top-ups': 'PENDING_TOP_UPS',
                'pending-subscription-orders': 'PENDING_SUBSCRIPTION_ORDERS',
                'missing-local-login': 'MISSING_LOCAL_LOGIN',
                'database-unavailable': 'DATABASE_UNAVAILABLE',
                'schema-unverifiable': 'SCHEMA_UNVERIFIABLE',
                'order-status-unverifiable': 'ORDER_STATUS_UNVERIFIABLE',
                'authentication-unverifiable': 'AUTHENTICATION_UNVERIFIABLE',
                'invalid-source-edition': 'INVALID_SOURCE_EDITION',
                'invalid-target-edition': 'INVALID_TARGET_EDITION',
              }
              if (gate) {
                const codes = gate[2].split(', ')
                if (codes.every(code => Object.hasOwn(issues, code)) && new Set(codes).size === codes.length) {
                  failure = 'NATIVE_HOST_LITE_' + states[gate[1]] + '_' + (codes.length === 1 ? issues[codes[0]] : 'MULTIPLE')
                }
              }
            }
            if (value.state === 'stop-unconfirmed') failure = 'NATIVE_HOST_SERVICE_UNREADY_STOP_UNCONFIRMED'
            finish(new Error(failure))
          }
        }
        updates.on('state', receive)
        timer = setTimeout(() => finish(new Error('NATIVE_HOST_STATE_TIMEOUT')), timeout)
        if (latest) receive(latest)
      })
    },
  }
}
