import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'
import vm from 'node:vm'

class Element {
  constructor(tag = 'div') { this.tag = tag; this.dataset = {}; this.textContent = ''; this.children = []; this.listeners = new Map(); this.disabled = false; this.hidden = false; this.value = '' }
  set innerHTML(_value) { throw new Error('Panel content must use text, not HTML interpolation') }
  addEventListener(name, listener) { const listeners = this.listeners.get(name) || []; listeners.push(listener); this.listeners.set(name, listeners) }
  append(...children) { this.children.push(...children) }
  replaceChildren(...children) { this.children = children }
  async fire(name) { await Promise.all((this.listeners.get(name) || []).map((listener) => listener({ target: this }))) }
}

const state = (overrides = {}) => ({
  revision: 1, applicationVersion: '0.2.0-synthetic', language: 'en', messages: {}, state: 'running', stateLabel: 'Running', stateTone: 'success', stateDescription: 'The owned service passed its startup probe.', scopeLabel: 'Local only',
  canStart: false, canStop: true, canQuit: true, canOpenConsole: true, canRestart: true,
  addresses: ['http://127.0.0.1:3000/v1'], locations: { userData: '/synthetic/app', dataDirectory: '/synthetic/app/data', databaseLocationLabel: 'Default database', sessionSecretLabel: 'Stored locally; value not shown', logsDirectory: '/synthetic/app/logs' },
  reasonLabel: 'No error reported', notice: null, ...overrides,
})

async function mount({ available = true, initial = state(), actionResponse } = {}) {
  const source = await readFile(new URL('../service-panel.js', import.meta.url), 'utf8')
  const html = await readFile(new URL('../service-panel.html', import.meta.url), 'utf8')
  const ids = new Map()
  const translated = []
  const nodes = []
  for (const match of html.matchAll(/<([a-z][\w-]*)\b([^>]*)>/gi)) {
    const node = new Element(match[1])
    node.disabled = /\bdisabled(?:\s|=|$)/.test(match[2])
    node.hidden = /\bhidden(?:\s|=|$)/.test(match[2])
    const id = /\bid="([^"]+)"/.exec(match[2])?.[1]
    const key = /\bdata-i18n="([^"]+)"/.exec(match[2])?.[1]
    if (id) ids.set(id, node)
    if (key) { node.dataset.i18n = key; translated.push(node) }
    nodes.push(node)
  }
  const document = {
    documentElement: { lang: 'en' },
    getElementById: (id) => ids.get(id),
    querySelectorAll: (selector) => selector === '[data-i18n]' ? translated : nodes.filter((node) => node.tag === 'button' || node.tag === 'select'),
    createElement: (tag) => new Element(tag),
  }
  const calls = []
  let listener
  let fail = false
  const initialResponse = Promise.resolve(initial)
  const bridge = {
    snapshot() { calls.push({ name: 'snapshot', args: [] }); return initialResponse },
    onState(callback) { listener = callback; return () => { listener = undefined } },
  }
  for (const name of ['start', 'stop', 'quit', 'openConsole', 'setLanguage', 'copyKnownAddress', 'openKnownLogs', 'showLAN', 'showPublicHelp', 'restartLAN', 'restartLoopback']) {
    bridge[name] = (...args) => { calls.push({ name, args }); return fail ? Promise.reject(new Error('private native error')) : (actionResponse || Promise.resolve(initial)) }
  }
  const unload = []
  vm.runInNewContext(source, { document, window: { myapiService: available ? bridge : undefined, addEventListener(name, callback) { assert.equal(name, 'unload'); unload.push(callback) } } })
  await initialResponse
  return { ids, calls, document, translated, publish(next) { listener?.(next) }, failActions() { fail = true }, unload() { unload.forEach((callback) => callback()) } }
}

test('the actual local panel renders immutable text and performs no service action or network request on load', async () => {
  const address = 'http://127.0.0.1:3000/<img onerror="unsafe">'
  const f = await mount({ initial: state({ addresses: [address] }) })
  assert.equal(f.ids.get('state').textContent, 'Running')
  assert.equal(f.ids.get('application-version').textContent, '0.2.0-synthetic')
  assert.equal(f.translated.find((node) => node.dataset.i18n === 'Automatic updates are off.').textContent, 'Automatic updates are off.')
  assert.equal(f.ids.get('start').disabled, true)
  assert.equal(f.ids.get('stop').disabled, false)
  assert.equal(f.ids.get('addresses').children[0].children[0].textContent, address)
  assert.deepEqual(f.calls, [{ name: 'snapshot', args: [] }])
  await f.ids.get('addresses').children[0].children[1].fire('click')
  assert.deepEqual(f.calls.at(-1), { name: 'copyKnownAddress', args: [address] })
  assert.equal(f.calls.some((call) => call.name === 'start' || call.name === 'stop'), false)
  f.publish(state({ revision: 2, applicationVersion: undefined }))
  assert.equal(f.ids.get('application-version').textContent, 'Not provided')
  f.publish(state({ revision: 3, applicationVersion: '   ' }))
  assert.equal(f.ids.get('application-version').textContent, 'Not provided')
})

test('a saved second copy-button handler retains its displayed address across later list reordering', async () => {
  const first = 'http://192.168.1.20:3000/v1'
  const second = 'http://10.0.0.7:3000/v1'
  const f = await mount({ initial: state({ addresses: [first, second] }) })
  const oldSecondButton = f.ids.get('addresses').children[1].children[1]
  f.publish(state({ revision: 2, addresses: [second, first] }))
  await oldSecondButton.fire('click')
  assert.deepEqual(f.calls.at(-1), { name: 'copyKnownAddress', args: [second] })
})

test('unknown stop outcome remains visible, cannot start another service, and a pushed language change updates real controls', async () => {
  const f = await mount()
  f.publish(state({ revision: 2, state: 'stop-unconfirmed', language: 'zh', stateLabel: '停止结果未知', stateTone: 'warning', stateDescription: '未收到进程关闭证据', canStart: false, canStop: true, canQuit: false, canOpenConsole: false, canRestart: false, messages: { 'Local service': '本地服务', 'Copy address': '复制地址', 'Stop bundled service': '停止随附服务' } }))
  assert.equal(f.document.documentElement.lang, 'zh-CN')
  assert.equal(f.ids.get('state').textContent, '停止结果未知')
  assert.equal(f.ids.get('state-description').textContent, '未收到进程关闭证据')
  assert.equal(f.ids.get('start').disabled, true)
  assert.equal(f.ids.get('quit').disabled, true)
  assert.equal(f.ids.get('open-console').disabled, true)
  assert.equal(f.ids.get('restart-lan').disabled, true)
  assert.equal(f.ids.get('stop').textContent, '停止随附服务')
  assert.equal(f.ids.get('addresses').children[0].children[1].textContent, '复制地址')
  assert.equal(f.calls.length, 1, 'state and language presentation never launches or stops a child')
  f.unload()
  f.publish(state())
  assert.equal(f.ids.get('state').textContent, '停止结果未知')
})

test('a delayed invoke response cannot overwrite a newer pushed state or language, and invalid revisions cannot claim health', async () => {
  let release
  const response = new Promise((resolve) => { release = resolve })
  const f = await mount({ actionResponse: response })
  const pending = f.ids.get('stop').fire('click')
  f.publish(state({ revision: 2, language: 'zh', state: 'stopped', stateLabel: '已停止', stateTone: 'neutral', canStart: true, canStop: false, canOpenConsole: false }))
  release(state({ revision: 1, language: 'en', state: 'running', stateLabel: 'Running', canStart: false, canOpenConsole: true }))
  await pending
  assert.equal(f.ids.get('state').textContent, '已停止')
  assert.equal(f.document.documentElement.lang, 'zh-CN')
  assert.equal(f.ids.get('start').disabled, false)
  assert.equal(f.ids.get('open-console').disabled, true)
  f.publish(state({ revision: undefined }))
  f.publish(state({ revision: -1 }))
  assert.equal(f.ids.get('state').textContent, '已停止')
  const unverified = await mount({ initial: state({ revision: undefined }) })
  assert.notEqual(unverified.ids.get('state').textContent, 'Running')
  assert.equal(unverified.ids.get('start').disabled, true)
  assert.equal(unverified.ids.get('stop').disabled, true)
  assert.equal(unverified.ids.get('open-console').disabled, true)
})

test('explicit native actions use fixed methods and errors preserve state without leaking native detail', async () => {
  const f = await mount()
  for (const [id, name] of [['stop', 'stop'], ['open-logs', 'openKnownLogs'], ['show-lan', 'showLAN'], ['show-public', 'showPublicHelp']]) {
    await f.ids.get(id).fire('click')
    assert.deepEqual(f.calls.at(-1), { name, args: [] })
  }
  f.failActions()
  await f.ids.get('stop').fire('click')
  assert.equal(f.ids.get('notice').hidden, false)
  assert.equal(f.ids.get('notice').dataset.tone, 'warning')
  assert.ok(!f.ids.get('notice').textContent.includes('private native error'))
  assert.equal(f.ids.get('state').textContent, 'Running')
})

test('missing native bridge disables every real HTML action instead of presenting an actionable service panel', async () => {
  const f = await mount({ available: false })
  for (const id of ['start', 'stop', 'quit', 'open-console', 'language', 'open-logs', 'show-lan', 'show-public', 'restart-lan', 'restart-loopback']) {
    assert.equal(f.ids.get(id).disabled, true, id)
  }
  assert.match(f.ids.get('state-description').textContent, /unavailable/)
  assert.equal(f.calls.length, 0)
})
