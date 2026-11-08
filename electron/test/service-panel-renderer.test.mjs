import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'
import vm from 'node:vm'

class Element {
  constructor(tag = 'div') { this.tag = tag; this.dataset = {}; this.textContent = ''; this.children = []; this.listeners = new Map(); this.attributes = new Map(); this.disabled = false; this.hidden = false; this.value = ''; this.checked = false; this.open = false }
  set innerHTML(_value) { throw new Error('Panel content must use text, not HTML interpolation') }
  addEventListener(name, listener) { const listeners = this.listeners.get(name) || []; listeners.push(listener); this.listeners.set(name, listeners) }
  removeEventListener(name, listener) { this.listeners.set(name, (this.listeners.get(name) || []).filter((value) => value !== listener)) }
  append(...children) { this.children.push(...children) }
  replaceChildren(...children) { this.children = children; this.onChildrenReplaced?.() }
  setAttribute(name, value) { this.attributes.set(name, String(value)); if (name === 'value') this.value = String(value) }
  getAttribute(name) { return this.attributes.get(name) ?? null }
  removeAttribute(name) { this.attributes.delete(name) }
  focus() { this.onFocus?.() }
  scrollIntoView() { this.scrolledIntoView = true }
  async fire(name, values = {}) { const event = { target: this, ...values, preventDefault() { this.defaultPrevented = true } }; await Promise.all((this.listeners.get(name) || []).map((listener) => listener(event))); return event }
}

const publicAccess = (overrides = {}) => ({ recordRevision: 0, savedOrigin: null, savedBaseURL: null, status: 'not-configured', verification: 'unknown', busy: false, listenerScope: 'local', proxyTargets: ['http://127.0.0.1:3000'], ...overrides })
const state = (overrides = {}) => ({
  revision: 1, generation: 1, applicationVersion: '0.2.0-synthetic', language: 'en', messages: {}, state: 'running', stateLabel: 'Running', stateTone: 'success', stateDescription: 'The owned service passed its startup probe.', scopeLabel: 'Local only',
  canStart: false, canStop: true, canQuit: true, canOpenConsole: true, canRestart: true,
  addresses: ['http://127.0.0.1:3000/v1'], locations: { userData: '/synthetic/app', dataDirectory: '/synthetic/app/data', databaseLocationLabel: 'Default database', sessionSecretLabel: 'Stored locally; value not shown', logsDirectory: '/synthetic/app/logs' },
  reasonLabel: 'No error reported', notice: null, publicGuideRequest: 0, publicAccess: publicAccess(), ...overrides,
})

async function mount({ available = true, initial = state(), actionResponse, expectInitialRender = true, validationAvailable = true, responses = {}, checkBridgeAvailable = true } = {}) {
  const source = await readFile(new URL('../service-panel.js', import.meta.url), 'utf8')
  const validationSource = await readFile(new URL('../public-access-config.js', import.meta.url), 'utf8')
  const html = await readFile(new URL('../service-panel.html', import.meta.url), 'utf8')
  const ids = new Map()
  const translated = []
  const nodes = []
  let activeElement = null
  const createElement = (tag) => { const node = new Element(tag); node.onFocus = () => { activeElement = node }; return node }
  for (const match of html.matchAll(/<([a-z][\w-]*)\b([^>]*)>/gi)) {
    const node = createElement(match[1])
    for (const attribute of match[2].matchAll(/([\w-]+)="([^"]*)"/g)) node.setAttribute(attribute[1], attribute[2])
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
    get activeElement() { return activeElement },
    getElementById: (id) => ids.get(id),
    querySelectorAll: (selector) => selector === '[data-i18n]' ? translated : nodes.filter((node) => selector.split(',').map((tag) => tag.trim()).includes(node.tag)),
    createElement,
  }
  const calls = []
  let listener
  let fail = false
  let completeRender, consumeRevision
  const rendered = new Promise((resolve) => { completeRender = resolve })
  const revisionConsumed = new Promise((resolve) => { consumeRevision = resolve })
  const renderObservers = []
  // The real render finishes with this DOM update. Awaiting the host response
  // alone does not wait for the VM realm's async continuation to populate it.
  ids.get('locations').onChildrenReplaced = () => { completeRender(); renderObservers.splice(0).forEach((resolve) => resolve()) }
  const initialResponse = Promise.resolve(new Proxy(initial, {
    get(target, property, receiver) {
      if (property === 'revision') consumeRevision()
      return Reflect.get(target, property, receiver)
    },
  }))
  const bridge = {
    snapshot() { calls.push({ name: 'snapshot', args: [] }); return initialResponse },
    onState(callback) { listener = callback; return () => { listener = undefined } },
  }
  for (const name of ['start', 'stop', 'quit', 'openConsole', 'setLanguage', 'copyKnownAddress', 'openKnownLogs', 'showLAN', 'showPublicHelp', 'restartLAN', 'restartLoopback', 'savePublicEntry', 'clearPublicEntry', 'copyPublicBaseURL', 'copyProxyTarget']) {
    bridge[name] = (...args) => { calls.push({ name, args }); return fail ? Promise.reject(new Error('private native error')) : (responses[name] ? responses[name](...args) : actionResponse || Promise.resolve(initial)) }
  }
  if (checkBridgeAvailable) {
    for (const name of ['checkPublicEntry', 'cancelPublicEntryCheck']) {
      bridge[name] = (...args) => { calls.push({ name, args }); return fail ? Promise.reject(new Error('private native error')) : (responses[name] ? responses[name](...args) : actionResponse || Promise.resolve(initial)) }
    }
  }
  const unload = []
  const context = vm.createContext({ document, URL, window: { myapiService: available ? bridge : undefined, addEventListener(name, callback) { assert.equal(name, 'unload'); unload.push(callback) } } })
  assert.deepEqual([...html.matchAll(/<script src="([^"]+)" defer>/g)].map((match) => match[1]), ['public-access-config.js', 'service-panel.js'])
  if (validationAvailable) vm.runInContext(validationSource, context)
  vm.runInContext(source, context)
  if (available) {
    // Invalid snapshots deliberately never reach the DOM completion boundary;
    // wait until the real validator consumed them before asserting rejection.
    await (expectInitialRender ? rendered : revisionConsumed)
  }
  return { ids, calls, document, translated, nextRender: () => new Promise((resolve) => renderObservers.push(resolve)), publish(next) { listener?.(next) }, failActions() { fail = true }, unload() { unload.forEach((callback) => callback()) } }
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
  const unverified = await mount({ initial: state({ revision: undefined }), expectInitialRender: false })
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
  for (const id of ['start', 'stop', 'quit', 'open-console', 'language', 'open-logs', 'show-lan', 'show-public', 'restart-lan', 'restart-loopback', 'public-origin', 'public-proxy-same', 'public-proxy-lan', 'public-save', 'public-open-console', 'public-step-1', 'public-step-2', 'public-step-3', 'public-step-4']) {
    assert.equal(f.ids.get(id).disabled, true, id)
  }
  assert.match(f.ids.get('state-description').textContent, /unavailable/)
  assert.equal(f.calls.length, 0)
  assert.equal(f.ids.get('public-check-start').disabled, true)
  assert.equal(f.ids.get('public-check-cancel').disabled, true)
})

async function enterPublicDraft(f, origin) {
  f.ids.get('public-proxy-same').checked = true
  await f.ids.get('public-proxy-same').fire('change')
  await f.ids.get('public-next-1').fire('click')
  f.ids.get('public-prerequisites').value = 'ready'
  await f.ids.get('public-prerequisites').fire('change')
  await f.ids.get('public-next-2').fire('click')
  assert.equal(f.ids.get('public-page-3').hidden, false)
  f.ids.get('public-origin').value = origin
  await f.ids.get('public-origin').fire('input')
}

test('the collapsed four-step guide separates actual listener facts, explicit LAN controls and shared address validation without a native side effect', async () => {
  const f = await mount()
  assert.equal(f.ids.get('public-guide').open, false)
  assert.equal(f.ids.get('public-record-status').textContent, 'No public entry is saved on this computer.')
  assert.equal(f.ids.get('public-proxy-targets').children[0].children[0].textContent, 'http://127.0.0.1:3000')
  f.ids.get('public-proxy-lan').checked = true
  await f.ids.get('public-proxy-lan').fire('change')
  assert.equal(f.ids.get('public-next-1').disabled, true)
  assert.equal(f.ids.get('public-listener-scope').textContent, 'Configured listener: this computer only.')
  await f.ids.get('public-next-1').fire('click')
  assert.equal(f.ids.get('public-page-1').hidden, false)
  await f.ids.get('public-review-lan').fire('click')
  assert.equal(f.ids.get('local-access-controls').open, true)
  assert.equal(f.document.activeElement, f.ids.get('local-access-summary'))
  f.publish(state({ revision: 2, scopeLabel: 'Private LAN', addresses: ['http://192.168.1.20:3000/v1'], publicAccess: publicAccess({ listenerScope: 'lan', proxyTargets: ['http://192.168.1.20:3000'] }) }))
  assert.equal(f.ids.get('public-next-1').disabled, false)
  await f.ids.get('public-next-1').fire('click')
  assert.equal(f.ids.get('public-step-2').getAttribute('aria-current'), 'step')
  f.ids.get('public-prerequisites').value = 'missing'
  await f.ids.get('public-prerequisites').fire('change')
  assert.equal(f.ids.get('public-next-2').disabled, true)
  f.ids.get('public-prerequisites').value = 'ready'
  await f.ids.get('public-prerequisites').fire('change')
  await f.ids.get('public-next-2').fire('click')
  assert.equal(f.document.activeElement, f.ids.get('public-heading-3'))
  f.ids.get('public-origin').value = 'https://gateway.example.com/v1'
  await f.ids.get('public-origin').fire('input')
  assert.equal(f.ids.get('public-save').disabled, true)
  assert.equal(f.ids.get('public-origin').getAttribute('aria-invalid'), 'true')
  assert.equal(f.ids.get('public-origin-feedback').textContent, 'Path prefixes are not supported. Enter only the HTTPS root origin.')
  f.ids.get('public-origin').value = 'https://gateway.example.com'
  await f.ids.get('public-origin').fire('input')
  assert.equal(f.ids.get('public-preview').textContent, 'https://gateway.example.com/v1')
  assert.equal(f.ids.get('public-save').disabled, false)
  f.publish(state({ revision: 3, publicAccess: publicAccess({ listenerScope: 'local' }) }))
  assert.equal(f.ids.get('public-save').disabled, true, 'a later loss of LAN scope cannot leave the LAN draft actionable')
  assert.deepEqual(f.calls, [{ name: 'snapshot', args: [] }])
})

test('only a bound successful save acknowledgement advances to client configuration and copies the exact saved public URL and revision', async () => {
  const origin = 'https://gateway.example.com'
  const saved = publicAccess({ recordRevision: 1, status: 'unverified', savedOrigin: origin, savedBaseURL: 'https://gateway.example.com/v1' })
  const f = await mount({ responses: { savePublicEntry: async () => state({ revision: 2, publicAccess: saved, publicSaveResult: { outcome: 'saved', expectedRevision: 0, origin, recordRevision: 1, unchanged: false } }) } })
  await enterPublicDraft(f, origin)
  const rendered = f.nextRender()
  const event = await f.ids.get('public-origin').fire('keydown', { key: 'Enter' })
  await rendered
  assert.equal(event.defaultPrevented, true)
  assert.equal(f.ids.get('public-page-4').hidden, false)
  assert.equal(f.document.activeElement, f.ids.get('public-heading-4'))
  assert.equal(f.ids.get('public-action-feedback').textContent, 'Entry saved on this computer. It remains unverified.')
  assert.deepEqual(f.calls.at(-1), { name: 'savePublicEntry', args: [origin, 0] })
  assert.equal(f.ids.get('public-saved-entry').children[0].children[0].textContent, 'https://gateway.example.com/v1')
  await f.ids.get('public-saved-entry').children[0].children[1].fire('click')
  assert.deepEqual(f.calls.at(-1), { name: 'copyPublicBaseURL', args: ['https://gateway.example.com/v1', 1] })
  await f.ids.get('public-open-console').fire('click')
  assert.deepEqual(f.calls.at(-1), { name: 'openConsole', args: [] })
  assert.equal(f.calls.some((call) => call.name === 'restartLAN' || call.name === 'restartLoopback'), false)
})

test('periodic snapshots preserve the draft, current step and focus; an explicit native guide request brings its summary into view', async () => {
  const old = publicAccess({ recordRevision: 5, status: 'unverified', savedOrigin: 'https://old.example.com', savedBaseURL: 'https://old.example.com/v1' })
  const f = await mount({ initial: state({ publicAccess: old }) })
  await enterPublicDraft(f, 'https://draft.example.com')
  const input = f.ids.get('public-origin')
  input.focus()
  f.publish(state({ revision: 2, publicAccess: old }))
  assert.equal(input.value, 'https://draft.example.com')
  assert.equal(f.document.activeElement, input)
  const changed = publicAccess({ recordRevision: 6, status: 'unverified', savedOrigin: 'https://draft.example.com', savedBaseURL: 'https://draft.example.com/v1' })
  f.publish(state({ revision: 3, publicGuideRequest: 1, publicAccess: changed }))
  assert.equal(f.ids.get('public-guide').open, true)
  assert.equal(f.document.activeElement, f.ids.get('public-guide-summary'))
  assert.equal(f.ids.get('public-guide-summary').scrolledIntoView, true)
  assert.equal(f.ids.get('public-page-3').hidden, false, 'a matching external record is not a save acknowledgement')
  assert.equal(input.value, 'https://draft.example.com')
  assert.equal(f.ids.get('public-draft-note').hidden, false)
  input.focus()
  f.ids.get('public-guide-summary').scrolledIntoView = false
  f.publish(state({ revision: 4, publicGuideRequest: 1, publicAccess: changed }))
  assert.equal(f.document.activeElement, input)
  assert.equal(f.ids.get('public-guide-summary').scrolledIntoView, false)
  assert.deepEqual(f.calls, [{ name: 'snapshot', args: [] }])
})

for (const outcome of ['cancelled', 'rejected']) {
  test(`${outcome} save cannot be inferred successful from an externally changed matching origin`, async () => {
    const origin = 'https://gateway.example.com'
    const f = await mount({ responses: { savePublicEntry: async () => state({ revision: 2, publicAccess: publicAccess({ recordRevision: 1, status: 'unverified', savedOrigin: origin, savedBaseURL: 'https://gateway.example.com/v1' }), publicSaveResult: { outcome, expectedRevision: 0, origin, recordRevision: 1, unchanged: false } }) } })
    await enterPublicDraft(f, origin)
    await f.ids.get('public-save').fire('click')
    assert.equal(f.ids.get('public-page-3').hidden, false)
    assert.equal(f.ids.get('public-origin').value, origin)
    assert.equal(f.ids.get('public-action-feedback').textContent, 'The save was cancelled or rejected. Your draft is preserved; review the current saved entry.')
  })
}

test('an unconfirmed post-write result preserves the draft and never claims that the old record was unchanged', async () => {
  const origin = 'https://gateway.example.com'
  const f = await mount({ responses: { savePublicEntry: async () => state({ revision: 2, publicAccess: publicAccess({ recordRevision: 1, status: 'unreadable' }), publicSaveResult: { outcome: 'unconfirmed', expectedRevision: 0, origin, recordRevision: 1, unchanged: false } }) } })
  await enterPublicDraft(f, origin)
  await f.ids.get('public-save').fire('click')
  assert.equal(f.ids.get('public-page-3').hidden, false)
  assert.equal(f.ids.get('public-origin').value, origin)
  assert.equal(f.ids.get('public-save').disabled, true)
  assert.equal(f.ids.get('public-action-feedback').textContent, 'Save result is unconfirmed. Your draft is preserved; review the saved entry.')
  assert.equal(f.ids.get('public-recovery').hidden, false)
})

test('a failed save only reports unchanged when the native result proves it and preserves the exact old record and draft', async () => {
  const origin = 'https://draft.example.com'
  const old = publicAccess({ recordRevision: 5, status: 'unverified', savedOrigin: 'https://old.example.com', savedBaseURL: 'https://old.example.com/v1' })
  const f = await mount({ initial: state({ publicAccess: old }), responses: { savePublicEntry: async () => state({ revision: 2, publicAccess: old, publicSaveResult: { outcome: 'failed', expectedRevision: 5, origin, recordRevision: 5, unchanged: true } }) } })
  await enterPublicDraft(f, origin)
  await f.ids.get('public-save').fire('click')
  assert.equal(f.ids.get('public-page-3').hidden, false)
  assert.equal(f.ids.get('public-origin').value, origin)
  assert.equal(f.ids.get('public-saved-entry').children[0].children[0].textContent, 'https://old.example.com/v1')
  assert.equal(f.ids.get('public-action-feedback').textContent, 'The saved entry did not change. Your draft is preserved.')
})

for (const mismatch of [{ expectedRevision: 1 }, { origin: 'https://other.example.com' }, { recordRevision: 2 }, { unchanged: true }]) {
  test(`a save acknowledgement with a mismatched request or commit proof cannot advance: ${JSON.stringify(mismatch)}`, async () => {
    const origin = 'https://gateway.example.com'
    const f = await mount({ responses: { savePublicEntry: async () => state({ revision: 2, publicAccess: publicAccess({ recordRevision: 1, status: 'unverified', savedOrigin: origin, savedBaseURL: 'https://gateway.example.com/v1' }), publicSaveResult: { outcome: 'saved', expectedRevision: 0, origin, recordRevision: 1, unchanged: false, ...mismatch } }) } })
    await enterPublicDraft(f, origin)
    await f.ids.get('public-save').fire('click')
    assert.equal(f.ids.get('public-page-3').hidden, false)
    assert.equal(f.ids.get('public-origin').value, origin)
    assert.equal(f.ids.get('public-action-feedback').textContent, 'Save result is unconfirmed. Your draft is preserved; review the saved entry.')
  })
}

test('a late successful save reply cannot replace a newer record, reset the draft or advance the step', async () => {
  let release
  const response = new Promise((resolve) => { release = resolve })
  const origin = 'https://gateway.example.com'
  const f = await mount({ responses: { savePublicEntry: () => response } })
  await enterPublicDraft(f, origin)
  const pending = f.ids.get('public-save').fire('click')
  assert.equal(f.ids.get('public-save').disabled, true)
  await f.ids.get('public-save').fire('click')
  assert.equal(f.calls.filter((call) => call.name === 'savePublicEntry').length, 1)
  f.publish(state({ revision: 4, publicAccess: publicAccess({ recordRevision: 9, status: 'unverified', savedOrigin: 'https://current.example.com', savedBaseURL: 'https://current.example.com/v1' }) }))
  release(state({ revision: 2, publicAccess: publicAccess({ recordRevision: 1, status: 'unverified', savedOrigin: origin, savedBaseURL: 'https://gateway.example.com/v1' }), publicSaveResult: { outcome: 'saved', expectedRevision: 0, origin, recordRevision: 1, unchanged: false } }))
  await pending
  assert.equal(f.ids.get('public-page-3').hidden, false)
  assert.equal(f.ids.get('public-origin').value, origin)
  assert.equal(f.ids.get('public-saved-entry').children[0].children[0].textContent, 'https://current.example.com/v1')
  assert.equal(f.ids.get('public-action-feedback').textContent, 'Save result is unconfirmed. Your draft is preserved; review the saved entry.')
})

test('old saved-copy, clear and proxy-copy handlers retain the displayed values and revisions instead of guessing the replacement record', async () => {
  const previous = publicAccess({ recordRevision: 5, status: 'unverified', savedOrigin: 'https://old.example.com', savedBaseURL: 'https://old.example.com/v1' })
  const current = state({ revision: 2, publicAccess: publicAccess({ recordRevision: 6, status: 'unverified', savedOrigin: 'https://current.example.com', savedBaseURL: 'https://current.example.com/v1', proxyTargets: ['http://192.168.1.20:3000'] }) })
  const f = await mount({ initial: state({ publicAccess: previous }), responses: { copyPublicBaseURL: async () => current, clearPublicEntry: async () => current, copyProxyTarget: async () => current } })
  const oldCopy = f.ids.get('public-saved-entry').children[0].children[1]
  const oldClear = f.ids.get('public-saved-entry').children[1]
  const oldTarget = f.ids.get('public-proxy-targets').children[0].children[1]
  f.publish(current)
  await oldCopy.fire('click')
  assert.deepEqual(f.calls.at(-1), { name: 'copyPublicBaseURL', args: ['https://old.example.com/v1', 5] })
  await oldClear.fire('click')
  assert.deepEqual(f.calls.at(-1), { name: 'clearPublicEntry', args: ['https://old.example.com', 5] })
  await oldTarget.fire('click')
  assert.deepEqual(f.calls.at(-1), { name: 'copyProxyTarget', args: ['http://127.0.0.1:3000'] })
  assert.equal(f.ids.get('public-saved-entry').children[0].children[0].textContent, 'https://current.example.com/v1')
})

for (const result of ['cleared', 'cancelled', 'failed']) {
  test(`unreadable-record recovery ${result} only requests clear(null, revision), preserving the draft and external state`, async () => {
    const f = await mount({ responses: { clearPublicEntry: async () => {
      if (result === 'failed') throw new Error('private native error')
      return state({ revision: 3, publicAccess: publicAccess({ recordRevision: result === 'cleared' ? 2 : 1, status: result === 'cleared' ? 'not-configured' : 'unreadable' }), publicClearResult: { outcome: result, expectedRevision: 1, origin: null, recordRevision: result === 'cleared' ? 2 : 1, unchanged: result === 'cancelled' } })
    } } })
    await enterPublicDraft(f, 'https://draft.example.com')
    f.publish(state({ revision: 2, publicAccess: publicAccess({ recordRevision: 1, status: 'unreadable' }) }))
    assert.equal(f.ids.get('public-save').disabled, true)
    assert.equal(f.ids.get('public-open-console').disabled, true)
    assert.equal(f.ids.get('public-record-status').textContent, 'The saved public entry is unreadable.')
    await f.ids.get('public-recovery-actions').children[0].fire('click')
    assert.deepEqual(f.calls.at(-1), { name: 'clearPublicEntry', args: [null, 1] })
    assert.equal(f.ids.get('public-origin').value, 'https://draft.example.com')
    assert.equal(f.ids.get('public-recovery').hidden, result === 'cleared')
    assert.equal(f.ids.get('public-save').disabled, result !== 'cleared')
    let feedback = 'Public entry action failed. Your draft is preserved; review the saved entry before retrying.'
    if (result === 'cleared') feedback = 'The saved entry on this computer was cleared. External configuration is unchanged.'
    else if (result === 'cancelled') feedback = 'The saved entry did not change. Your draft is preserved.'
    assert.equal(f.ids.get('public-action-feedback').textContent, feedback)
    assert.equal(f.calls.some((call) => call.name === 'restartLAN' || call.name === 'restartLoopback'), false)
  })
}

test('a bound clear acknowledgement confirms only the local record and preserves the current step and unsaved draft', async () => {
  const origin = 'https://old.example.com'
  const initial = state({ publicAccess: publicAccess({ recordRevision: 5, status: 'unverified', savedOrigin: origin, savedBaseURL: 'https://old.example.com/v1' }) })
  const f = await mount({ initial, responses: { clearPublicEntry: async () => state({ revision: 2, publicAccess: publicAccess({ recordRevision: 6 }), publicClearResult: { outcome: 'cleared', expectedRevision: 5, origin, recordRevision: 6, unchanged: false } }) } })
  await enterPublicDraft(f, 'https://draft.example.com')
  await f.ids.get('public-step-4').fire('click')
  await f.ids.get('public-saved-entry').children[1].fire('click')
  assert.deepEqual(f.calls.at(-1), { name: 'clearPublicEntry', args: [origin, 5] })
  assert.equal(f.ids.get('public-action-feedback').textContent, 'The saved entry on this computer was cleared. External configuration is unchanged.')
  assert.equal(f.ids.get('public-page-4').hidden, false)
  assert.equal(f.ids.get('public-origin').value, 'https://draft.example.com')
  assert.equal(f.ids.get('public-record-status').textContent, 'No public entry is saved on this computer.')
})

for (const outcome of ['cancelled', 'rejected']) {
  test(`${outcome} clear does not infer success when an external change already produced the requested empty record`, async () => {
    const origin = 'https://old.example.com'
    const initial = state({ publicAccess: publicAccess({ recordRevision: 5, status: 'unverified', savedOrigin: origin, savedBaseURL: 'https://old.example.com/v1' }) })
    const f = await mount({ initial, responses: { clearPublicEntry: async () => state({ revision: 2, publicAccess: publicAccess({ recordRevision: 6 }), publicClearResult: { outcome, expectedRevision: 5, origin, recordRevision: 6, unchanged: false } }) } })
    await enterPublicDraft(f, 'https://draft.example.com')
    await f.ids.get('public-step-4').fire('click')
    await f.ids.get('public-saved-entry').children[1].fire('click')
    assert.equal(f.ids.get('public-action-feedback').textContent, 'The saved entry changed. Review the current record before trying again.')
    assert.equal(f.ids.get('public-page-4').hidden, false)
    assert.equal(f.ids.get('public-origin').value, 'https://draft.example.com')
    assert.equal(f.ids.get('public-record-status').textContent, 'No public entry is saved on this computer.')
  })
}

test('an unconfirmed clear result never reports a successful clear or an unchanged record', async () => {
  const origin = 'https://old.example.com'
  const initial = state({ publicAccess: publicAccess({ recordRevision: 5, status: 'unverified', savedOrigin: origin, savedBaseURL: 'https://old.example.com/v1' }) })
  const f = await mount({ initial, responses: { clearPublicEntry: async () => state({ revision: 2, publicAccess: publicAccess({ recordRevision: 6, status: 'unreadable' }), publicClearResult: { outcome: 'unconfirmed', expectedRevision: 5, origin, recordRevision: 6, unchanged: false } }) } })
  await enterPublicDraft(f, 'https://draft.example.com')
  await f.ids.get('public-step-4').fire('click')
  await f.ids.get('public-saved-entry').children[1].fire('click')
  assert.equal(f.ids.get('public-action-feedback').textContent, 'The write result could not be confirmed. Read the saved entry again before taking another action.')
  assert.equal(f.ids.get('public-page-4').hidden, false)
  assert.equal(f.ids.get('public-origin').value, 'https://draft.example.com')
  assert.equal(f.ids.get('public-recovery').hidden, false)
})

test('an old clear reply cannot replace a newer record or confirm an operation from its older context', async () => {
  let release
  const response = new Promise((resolve) => { release = resolve })
  const origin = 'https://old.example.com'
  const initial = state({ publicAccess: publicAccess({ recordRevision: 5, status: 'unverified', savedOrigin: origin, savedBaseURL: 'https://old.example.com/v1' }) })
  const f = await mount({ initial, responses: { clearPublicEntry: () => response } })
  await enterPublicDraft(f, 'https://draft.example.com')
  await f.ids.get('public-step-4').fire('click')
  const pending = f.ids.get('public-saved-entry').children[1].fire('click')
  f.publish(state({ revision: 4, publicAccess: publicAccess({ recordRevision: 9, status: 'unverified', savedOrigin: 'https://current.example.com', savedBaseURL: 'https://current.example.com/v1' }) }))
  release(state({ revision: 2, publicAccess: publicAccess({ recordRevision: 6 }), publicClearResult: { outcome: 'cleared', expectedRevision: 5, origin, recordRevision: 6, unchanged: false } }))
  await pending
  assert.equal(f.ids.get('public-action-feedback').textContent, 'The write result could not be confirmed. Read the saved entry again before taking another action.')
  assert.equal(f.ids.get('public-page-4').hidden, false)
  assert.equal(f.ids.get('public-origin').value, 'https://draft.example.com')
  assert.equal(f.ids.get('public-saved-entry').children[0].children[0].textContent, 'https://current.example.com/v1')
})

for (const record of [
  publicAccess({ verification: 'verified' }),
  publicAccess({ listenerScope: 'unknown' }),
  publicAccess({ proxyTargets: ['https://public.example.com'] }),
  publicAccess({ status: 'unverified', savedOrigin: 'https://gateway.example.com', savedBaseURL: 'https://other.example.com/v1' }),
]) {
  test(`malformed public data is never treated as an empty successful configuration: ${JSON.stringify(record)}`, async () => {
    const f = await mount({ initial: state({ publicAccess: record }) })
    assert.equal(f.ids.get('state').textContent, 'Running', 'existing service controls remain independent')
    assert.equal(f.ids.get('public-record-status').textContent, 'Public entry status is unknown.')
    for (const id of ['public-save', 'public-step-1', 'public-origin', 'public-open-console']) assert.equal(f.ids.get(id).disabled, true)
    assert.equal(f.ids.get('public-recovery-actions').children.length, 0)
    await f.ids.get('public-save').fire('click')
    assert.deepEqual(f.calls, [{ name: 'snapshot', args: [] }])
  })
}

test('missing shared validation keeps the public guide disabled without replacing validation or breaking the original service panel', async () => {
  const f = await mount({ validationAvailable: false })
  assert.equal(f.ids.get('state').textContent, 'Running')
  assert.equal(f.ids.get('public-origin').disabled, true)
  assert.equal(f.ids.get('public-save').disabled, true)
  assert.equal(f.ids.get('public-record-status').textContent, 'Public entry status is unknown.')
})

const checkedOrigin = 'https://public.example.test'
const entryCheck = (overrides = {}) => ({ id: 'a'.repeat(32), origin: checkedOrigin, recordRevision: 4,
  generation: 1, state: 'checking', startedAt: 1700000000000, result: null, ...overrides })
const entryReport = (overrides = {}) => ({ schemaVersion: 1, origin: checkedOrigin, outcome: 'passed', code: 'CHECK_PASSED',
  checkedAt: 1700000001000, source: 'this-computer', externalVerification: 'unknown', sameInstanceVerified: false,
  modelVerified: false, quotaVerified: false, dns: { state: 'passed', source: 'lookup' }, tls: { state: 'passed' },
  status: { state: 'passed', version: null, productEdition: null }, ...overrides })
const checkSnapshot = (revision, localCheck, overrides = {}) => state({ revision,
  publicAccess: publicAccess({ status: 'unverified', savedOrigin: checkedOrigin, savedBaseURL: checkedOrigin + '/v1',
    recordRevision: 4, localCheck }), ...overrides })

test('the entry check is collapsed and inert until a click binds the displayed saved origin and revision', async () => {
  const response = checkSnapshot(2, entryCheck())
  response.publicCheckControlResult = { action: 'start', outcome: 'started', expectedRevision: 4, origin: checkedOrigin, checkId: 'a'.repeat(32) }
  const f = await mount({ initial: checkSnapshot(1, null), responses: { checkPublicEntry: () => Promise.resolve(response) } })
  assert.equal(f.ids.get('public-entry-check').open, false)
  assert.equal(f.ids.get('public-check-start').disabled, false)
  assert.equal(f.ids.get('public-check-cancel').disabled, true)
  assert.equal(f.ids.get('public-check-time').textContent, 'Not provided')
  for (const id of ['dns', 'tls', 'status']) assert.equal(f.ids.get('public-check-' + id).textContent, 'Not checked')
  f.ids.get('public-entry-check').open = true
  f.publish(checkSnapshot(1, null))
  assert.deepEqual(f.calls, [{ name: 'snapshot', args: [] }])
  await f.ids.get('public-check-start').fire('click')
  assert.deepEqual(f.calls.at(-1), { name: 'checkPublicEntry', args: [checkedOrigin, 4] })
  assert.equal(f.ids.get('public-check-feedback').textContent, 'In progress')
  assert.equal(f.ids.get('public-check-start').disabled, true)
  assert.equal(f.ids.get('public-check-cancel').disabled, false)
  assert.equal(f.ids.get('stop').disabled, false)
  assert.equal(f.ids.get('quit').disabled, false)
  assert.equal(f.ids.get('public-record-status').textContent, 'Saved on this computer · unverified')
})

test('real observation fields distinguish a literal-address check, unrun phases and a completed local result', async () => {
  const progress = entryReport({ outcome: 'unknown', code: 'TLS_CHECKING', checkedAt: null,
    dns: { state: 'passed', source: 'literal' }, tls: { state: 'checking' }, status: { state: 'not-run', version: null, productEdition: null } })
  // Public literal IPs are valid entry syntax; this fixture makes no connection.
  const origin = 'https://203.0.113.20'
  const view = (revision, report, stateValue) => checkSnapshot(revision, entryCheck({ origin, state: stateValue,
    result: { ...report, origin } }), { publicAccess: publicAccess({ status: 'unverified', savedOrigin: origin,
      savedBaseURL: origin + '/v1', recordRevision: 4, localCheck: entryCheck({ origin, state: stateValue, result: { ...report, origin } }) }) })
  const f = await mount({ initial: view(1, progress, 'checking') })
  assert.equal(f.ids.get('public-check-origin').textContent, origin)
  assert.equal(f.ids.get('public-check-dns-label').textContent, 'Address check')
  assert.equal(f.ids.get('public-check-dns').textContent, 'Passed')
  assert.equal(f.ids.get('public-check-tls').textContent, 'In progress')
  assert.equal(f.ids.get('public-check-status').textContent, 'Not checked')
  assert.equal(f.ids.get('public-check-time').textContent, 'Not provided')
  f.publish(view(2, entryReport({ dns: { state: 'passed', source: 'literal' } }), 'completed'))
  assert.equal(f.ids.get('public-check-time').textContent, new Date(1700000001000).toLocaleString('en'))
  assert.equal(f.ids.get('public-check-feedback').textContent, 'Checks completed from this computer. External access remains unverified.')
  for (const id of ['dns', 'tls', 'status']) assert.equal(f.ids.get('public-check-' + id).textContent, 'Passed')
  assert.equal(f.ids.get('public-record-status').textContent, 'Saved on this computer · unverified')
  assert.deepEqual(f.calls, [{ name: 'snapshot', args: [] }])
})

test('cancel binds the visible check ID and preserves the draft, step, focus and original stop and quit actions', async () => {
  const cancelled = checkSnapshot(2, entryCheck({ state: 'cancelled', result: entryReport({ outcome: 'cancelled', code: 'CANCELLED',
    tls: { state: 'unknown' }, status: { state: 'not-run', version: null, productEdition: null } }) }))
  cancelled.publicCheckControlResult = { action: 'cancel', outcome: 'cancelled', expectedRevision: null, origin: checkedOrigin, checkId: 'a'.repeat(32) }
  const f = await mount({ initial: checkSnapshot(1, entryCheck()), responses: { cancelPublicEntryCheck: () => Promise.resolve(cancelled) } })
  await enterPublicDraft(f, 'https://draft.example.test')
  await f.ids.get('public-step-4').fire('click')
  f.ids.get('public-check-cancel').focus()
  await f.ids.get('public-check-cancel').fire('click')
  assert.deepEqual(f.calls.at(-1), { name: 'cancelPublicEntryCheck', args: ['a'.repeat(32)] })
  assert.equal(f.ids.get('public-origin').value, 'https://draft.example.test')
  assert.equal(f.ids.get('public-page-4').hidden, false)
  assert.equal(f.document.activeElement, f.ids.get('public-check-cancel'))
  assert.equal(f.ids.get('public-check-feedback').textContent, 'Cancelled')
  assert.equal(f.ids.get('public-check-status').textContent, 'Not checked')
  assert.equal(f.ids.get('stop').disabled, false)
  assert.equal(f.ids.get('quit').disabled, false)
  await f.ids.get('stop').fire('click')
  await f.ids.get('quit').fire('click')
  assert.deepEqual(f.calls.slice(-2).map(value => value.name), ['stop', 'quit'])
  assert.equal(f.calls.some(value => ['savePublicEntry', 'clearPublicEntry'].includes(value.name)), false)
})

test('a delayed start reply cannot replace a newer entry or check, advance steps or discard an edited draft', async () => {
  let release
  const pending = new Promise(resolve => { release = resolve })
  const f = await mount({ initial: checkSnapshot(1, null), responses: { checkPublicEntry: () => pending } })
  await enterPublicDraft(f, 'https://unsaved.example.test')
  await f.ids.get('public-step-4').fire('click')
  const click = f.ids.get('public-check-start').fire('click')
  await f.ids.get('public-back-4').fire('click')
  f.ids.get('public-origin').focus()
  const nextOrigin = 'https://replacement.example.test'
  f.publish(checkSnapshot(10, null, { publicAccess: publicAccess({ status: 'unverified', savedOrigin: nextOrigin,
    savedBaseURL: nextOrigin + '/v1', recordRevision: 5, localCheck: entryCheck({ id: 'b'.repeat(32), origin: nextOrigin,
      recordRevision: 5, state: 'completed', result: entryReport({ origin: nextOrigin }) }) }) }))
  const old = checkSnapshot(2, entryCheck())
  old.publicCheckControlResult = { action: 'start', outcome: 'started', expectedRevision: 4, origin: checkedOrigin, checkId: 'a'.repeat(32) }
  release(old)
  await click
  assert.equal(f.ids.get('public-check-origin').textContent, nextOrigin)
  assert.equal(f.ids.get('public-origin').value, 'https://unsaved.example.test')
  assert.equal(f.ids.get('public-page-3').hidden, false)
  assert.equal(f.document.activeElement, f.ids.get('public-origin'))
  assert.equal(f.ids.get('public-check-dns').textContent, 'Passed')
  assert.equal(f.ids.get('public-check-feedback').textContent, 'The check response could not be matched. Review the current entry.')
})

test('old cancel handlers do not cancel a replacement check and same-origin revision or generation changes invalidate old observations', async () => {
  const done = entryCheck({ state: 'completed', result: entryReport() })
  const f = await mount({ initial: checkSnapshot(1, entryCheck()) })
  const oldCancel = f.ids.get('public-check-cancel').listeners.get('click')[0]
  f.publish(checkSnapshot(2, entryCheck({ id: 'b'.repeat(32) })))
  await oldCancel()
  assert.deepEqual(f.calls, [{ name: 'snapshot', args: [] }])
  f.publish(checkSnapshot(3, done, { generation: 2 }))
  for (const id of ['dns', 'tls', 'status']) assert.equal(f.ids.get('public-check-' + id).textContent, 'Unknown')
  assert.equal(f.ids.get('public-check-feedback').textContent, 'This result belongs to an earlier saved entry or service state. Run a new check.')
  f.publish(checkSnapshot(4, done, { publicAccess: publicAccess({ status: 'unverified', savedOrigin: checkedOrigin,
    savedBaseURL: checkedOrigin + '/v1', recordRevision: 5, localCheck: done }) }))
  assert.equal(f.ids.get('public-check-status').textContent, 'Unknown')
  assert.equal(f.ids.get('public-check-time').textContent, new Date(1700000001000).toLocaleString('en'))
  assert.equal(f.ids.get('public-entry-check').open, false)
})

test('old snapshots or a missing check bridge disable only the new check area', async () => {
  for (const options of [
    { initial: checkSnapshot(1, null, { publicAccess: publicAccess({ status: 'unverified', savedOrigin: checkedOrigin,
      savedBaseURL: checkedOrigin + '/v1', recordRevision: 4 }) }) },
    { initial: checkSnapshot(1, null), checkBridgeAvailable: false },
  ]) {
    const f = await mount(options)
    assert.equal(f.ids.get('public-check-start').disabled, true)
    assert.equal(f.ids.get('public-check-cancel').disabled, true)
    assert.equal(f.ids.get('public-check-feedback').textContent, 'Entry checks are unavailable in this version.')
    assert.equal(f.ids.get('public-step-4').disabled, false)
    assert.equal(f.ids.get('stop').disabled, false)
    await f.ids.get('public-saved-entry').children[0].children[1].fire('click')
    assert.deepEqual(f.calls.at(-1), { name: 'copyPublicBaseURL', args: [checkedOrigin + '/v1', 4] })
  }
})

test('malformed or overclaiming check results stay unknown without disabling the existing guide', async () => {
  const malformed = [
    entryCheck({ id: 'not-an-id' }),
    entryCheck({ result: entryReport({ source: 'another-computer' }) }),
    entryCheck({ result: entryReport({ sameInstanceVerified: true }) }),
    entryCheck({ result: entryReport({ externalVerification: 'passed' }) }),
    entryCheck({ result: entryReport({ checkedAt: 'unknown' }) }),
    entryCheck({ state: 'completed', result: entryReport({ status: { state: 'not-run', version: null, productEdition: null } }) }),
  ]
  for (const value of malformed) {
    const f = await mount({ initial: checkSnapshot(1, value) })
    assert.equal(f.ids.get('public-check-start').disabled, true)
    assert.equal(f.ids.get('public-check-feedback').textContent, 'The check could not be completed. Its result is unknown.')
    for (const id of ['dns', 'tls', 'status']) assert.equal(f.ids.get('public-check-' + id).textContent, 'Unknown')
    assert.equal(f.ids.get('public-step-4').disabled, false)
    assert.equal(f.ids.get('stop').disabled, false)
    assert.deepEqual(f.calls, [{ name: 'snapshot', args: [] }])
  }
})

test('a reply with a mismatched control acknowledgement cannot invent a started check', async () => {
  const response = checkSnapshot(2, entryCheck())
  response.publicCheckControlResult = { action: 'start', outcome: 'started', expectedRevision: 999, origin: checkedOrigin, checkId: 'a'.repeat(32) }
  const f = await mount({ initial: checkSnapshot(1, null), responses: { checkPublicEntry: () => Promise.resolve(response) } })
  await f.ids.get('public-check-start').fire('click')
  assert.equal(f.ids.get('public-check-feedback').textContent, 'The check response could not be matched. Review the current entry.')
  assert.equal(f.ids.get('public-check-cancel').disabled, true)
  assert.equal(f.ids.get('public-check-status').textContent, 'Not checked')
  assert.equal(f.ids.get('public-check-start').disabled, false)
})

test('a bound older control receipt accepts newer progress or completion of that same check without rendering the old snapshot', async t => {
  const progress = entryReport({ outcome: 'unknown', code: 'TLS_CHECKING', checkedAt: null,
    tls: { state: 'checking' }, status: { state: 'not-run', version: null, productEdition: null } })
  const cancelled = entryReport({ outcome: 'cancelled', code: 'CANCELLED', checkedAt: 1700000004000,
    tls: { state: 'unknown' }, status: { state: 'not-run', version: null, productEdition: null } })
  for (const scenario of [
    { name: 'started receipt after progress', action: 'start', latest: entryCheck({ result: progress }), feedback: 'In progress', tls: 'In progress', time: 'Not provided' },
    { name: 'started receipt after completion', action: 'start', latest: entryCheck({ state: 'completed', result: entryReport({ checkedAt: 1700000004000 }) }),
      feedback: 'Checks completed from this computer. External access remains unverified.', tls: 'Passed', time: new Date(1700000004000).toLocaleString('en') },
    { name: 'cancelled receipt after newer cancelled state', action: 'cancel', latest: entryCheck({ state: 'cancelled', result: cancelled }),
      feedback: 'Cancelled', tls: 'Unknown', time: new Date(1700000004000).toLocaleString('en') },
  ]) {
    await t.test(scenario.name, async () => {
      let release
      const pending = new Promise(resolve => { release = resolve })
      const method = scenario.action === 'start' ? 'checkPublicEntry' : 'cancelPublicEntryCheck'
      const f = await mount({ initial: checkSnapshot(1, scenario.action === 'start' ? null : entryCheck()), responses: { [method]: () => pending } })
      const button = f.ids.get(scenario.action === 'start' ? 'public-check-start' : 'public-check-cancel')
      button.focus()
      const click = button.fire('click')
      f.publish(checkSnapshot(10, scenario.latest))
      const oldCheck = scenario.action === 'start' ? entryCheck() : entryCheck({ state: 'cancelled',
        result: entryReport({ outcome: 'cancelled', code: 'CANCELLED', dns: { state: 'unknown', source: 'lookup' },
          tls: { state: 'not-run' }, status: { state: 'not-run', version: null, productEdition: null } }) })
      const old = checkSnapshot(2, oldCheck)
      old.publicCheckControlResult = { action: scenario.action, outcome: scenario.action === 'start' ? 'started' : 'cancelled',
        expectedRevision: scenario.action === 'start' ? 4 : null, origin: checkedOrigin, checkId: 'a'.repeat(32) }
      release(old)
      await click
      assert.equal(f.ids.get('public-check-feedback').textContent, scenario.feedback)
      assert.equal(f.ids.get('public-check-dns').textContent, 'Passed')
      assert.equal(f.ids.get('public-check-tls').textContent, scenario.tls)
      assert.equal(f.ids.get('public-check-time').textContent, scenario.time)
      assert.equal(f.document.activeElement, button)
      assert.equal(f.ids.get('public-entry-check').open, false)
      assert.deepEqual(f.calls.at(-1), { name: method, args: scenario.action === 'start' ? [checkedOrigin, 4] : ['a'.repeat(32)] })
    })
  }
})

test('current final check failures show only fixed actionable hints, while stale, cancelled and in-progress states keep their own meaning', async t => {
  const cases = [
    { code: 'address-type-unsupported', phase: 'dns', hint: 'This address is private or special-use. This check supports only ordinary public addresses.' },
    ...['tls-untrusted', 'tls-hostname', 'tls-validity'].map(code => ({ code, phase: 'tls', hint: 'Check the entry domain and its HTTPS certificate trust, hostname and validity.' })),
    { code: 'http-auth-required', phase: 'status', hint: 'The entry requires sign-in. This check sent no credentials.' },
    { code: 'http-redirect', phase: 'status', hint: 'The entry returned a redirect. This check did not follow it.' },
    ...['invalid-status', 'http-status'].map(code => ({ code, phase: 'status', hint: 'The entry did not return a recognizable gateway status.' })),
    { code: 'response-too-large', phase: 'status', hint: 'The response exceeded the check limit.', unknown: true },
    { code: 'timeout', phase: 'tls', hint: 'The check timed out. Review the entry before retrying.', unknown: true },
  ]
  for (const item of cases) {
    await t.test(item.code, async () => {
      const report = entryReport({ outcome: item.unknown ? 'unknown' : 'failed', code: item.code })
      for (const phase of ['dns', 'tls', 'status']) {
        if (phase === item.phase) report[phase].state = item.unknown ? 'unknown' : 'failed'
        else if (['dns', 'tls', 'status'].indexOf(phase) > ['dns', 'tls', 'status'].indexOf(item.phase)) report[phase].state = 'not-run'
      }
      const f = await mount({ initial: checkSnapshot(1, entryCheck({ state: 'completed', result: report })) })
      const summary = item.unknown ? 'The check could not be completed. Its result is unknown.' : 'Some checks failed. External access remains unverified.'
      assert.equal(f.ids.get('public-check-feedback').textContent, summary + ' ' + item.hint)
      assert.equal(f.ids.get('public-check-' + item.phase).textContent, item.unknown ? 'Unknown' : 'Failed')
      assert.equal(f.ids.get('public-record-status').textContent, 'Saved on this computer · unverified')
      assert.deepEqual(f.calls, [{ name: 'snapshot', args: [] }])
    })
  }
  for (const item of [
    { state: 'stale', outcome: 'failed', checkedAt: 1700000001000, feedback: 'This result belongs to an earlier saved entry or service state. Run a new check.' },
    { state: 'cancelled', outcome: 'cancelled', checkedAt: 1700000001000, feedback: 'Cancelled' },
    { state: 'checking', outcome: 'unknown', checkedAt: null, feedback: 'In progress' },
  ]) {
    const f = await mount({ initial: checkSnapshot(1, entryCheck({ state: item.state, result: entryReport({ code: 'timeout',
      outcome: item.outcome, checkedAt: item.checkedAt, tls: { state: item.state === 'checking' ? 'checking' : 'unknown' },
      status: { state: 'not-run', version: null, productEdition: null } }) })) })
    assert.equal(f.ids.get('public-check-feedback').textContent, item.feedback)
  }
  for (const code of ['UNRECOGNIZED_SAFE_CODE', 'constructor']) {
    const unknown = await mount({ initial: checkSnapshot(1, entryCheck({ state: 'unknown', result: entryReport({ outcome: 'unknown',
      code, status: { state: 'unknown', version: null, productEdition: null } }) })) })
    assert.equal(unknown.ids.get('public-check-feedback').textContent, 'The check could not be completed. Its result is unknown.')
  }
})
