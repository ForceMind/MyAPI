// Headless Chromium checks of the three redesigned pages (overview, API Keys
// with the Key/request inspector, upstream connections) against the
// synthetic read-only preview. Only 127.0.0.1:4184 is reachable; every other
// request is blocked and counted. Usage: node pages-shots.mjs <outDir>
import { spawn } from 'node:child_process'
import { mkdirSync, rmSync, writeFileSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'

const ORIGIN = 'http://127.0.0.1:4184'
const outDir = process.argv[2]
mkdirSync(outDir, { recursive: true })
const CHROME = path.join(
  os.homedir(),
  'Library/Caches/ms-playwright/chromium_headless_shell-1243/chrome-headless-shell-mac-arm64/chrome-headless-shell'
)
const PORT = 9335
const profile = path.join(os.tmpdir(), 'myapi-pages-profile')
rmSync(profile, { recursive: true, force: true })
const chrome = spawn(CHROME, [
  `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${profile}`,
  '--no-first-run',
  '--hide-scrollbars',
  'about:blank',
])
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

let targets = []
for (let i = 0; i < 50; i++) {
  try {
    targets = await (await fetch(`http://127.0.0.1:${PORT}/json`)).json()
    if (targets.length) break
  } catch {
    // not up yet
  }
  await sleep(200)
}
const ws = new WebSocket(targets.find((t) => t.type === 'page').webSocketDebuggerUrl)
await new Promise((r) => ws.addEventListener('open', r, { once: true }))
let nextId = 1
const pending = new Map()
const listeners = []
ws.addEventListener('message', (event) => {
  const msg = JSON.parse(event.data)
  if (msg.id && pending.has(msg.id)) {
    const { resolve, reject } = pending.get(msg.id)
    pending.delete(msg.id)
    if (msg.error) reject(new Error(JSON.stringify(msg.error)))
    else resolve(msg.result)
  } else if (msg.method) {
    for (const l of [...listeners]) l(msg)
  }
})
const send = (method, params = {}) =>
  new Promise((resolve, reject) => {
    const id = nextId++
    pending.set(id, { resolve, reject })
    ws.send(JSON.stringify({ id, method, params }))
  })

let blocked = 0
let edition = 'full'
const writes = []
listeners.push(async (msg) => {
  if (msg.method !== 'Fetch.requestPaused') return
  const p = msg.params
  const url = p.request.url
  if (!(url.startsWith(ORIGIN) || url.startsWith('data:'))) {
    blocked++
    await send('Fetch.failRequest', { requestId: p.requestId, errorReason: 'BlockedByClient' })
    return
  }
  if (p.responseStatusCode !== undefined) {
    const { body, base64Encoded } = await send('Fetch.getResponseBody', { requestId: p.requestId })
    const json = JSON.parse(base64Encoded ? Buffer.from(body, 'base64').toString('utf8') : body)
    if (edition === 'lite' && json.data) {
      json.data.product_edition = 'lite'
      json.data.capabilities = Object.fromEntries(
        ['payments', 'commercial_subscriptions', 'redemption_codes', 'checkin', 'affiliate', 'public_pricing', 'rankings', 'perf_metrics', 'external_oauth', 'midjourney', 'model_deployments', 'site_content', 'user_registration'].map((k) => [k, false])
      )
    }
    await send('Fetch.fulfillRequest', {
      requestId: p.requestId,
      responseCode: p.responseStatusCode,
      responseHeaders: [{ name: 'Content-Type', value: 'application/json; charset=utf-8' }],
      body: Buffer.from(JSON.stringify(json)).toString('base64'),
    })
    return
  }
  if (p.request.method !== 'GET' && !url.includes('/api/user/auth/refresh')) {
    writes.push(`${p.request.method} ${new URL(url).pathname}`)
  }
  await send('Fetch.continueRequest', { requestId: p.requestId })
})
const pageErrors = []
listeners.push((msg) => {
  if (msg.method === 'Runtime.exceptionThrown') pageErrors.push(msg.params.exceptionDetails.exception?.description ?? msg.params.exceptionDetails.text)
})

await send('Page.enable')
await send('Runtime.enable')
await send('Network.enable')
await send('Fetch.enable', {
  patterns: [
    { urlPattern: '*/api/status*', requestStage: 'Response' },
    { urlPattern: '*', requestStage: 'Request' },
  ],
})

async function load(url) {
  const loaded = new Promise((resolve) => {
    const l = (msg) => {
      if (msg.method === 'Page.loadEventFired') {
        listeners.splice(listeners.indexOf(l), 1)
        resolve()
      }
    }
    listeners.push(l)
  })
  await send('Page.navigate', { url })
  await loaded
}
async function js(expression) {
  const r = await send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true })
  if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description ?? r.exceptionDetails.text)
  return r.result.value
}
async function key(keyName, code, vk, modifiers = 0, text) {
  await send('Input.dispatchKeyEvent', { type: 'keyDown', key: keyName, code, windowsVirtualKeyCode: vk, modifiers, text })
  await send('Input.dispatchKeyEvent', { type: 'keyUp', key: keyName, code, windowsVirtualKeyCode: vk, modifiers })
}
let mobileNow = false
async function tap(selector, index = 0) {
  const box = await js(`(() => { const el = document.querySelectorAll(${JSON.stringify(selector)})[${index}]; if (!el) return null; el.scrollIntoView({ block: 'center' }); const r = el.getBoundingClientRect(); return { x: r.x + r.width / 2, y: r.y + r.height / 2 } })()`)
  if (!box) throw new Error(`no element for ${selector}[${index}]`)
  await sleep(120)
  for (const type of ['mousePressed', 'mouseReleased']) {
    await send('Input.dispatchMouseEvent', { type, x: box.x, y: box.y, button: 'left', clickCount: 1 })
  }
}
async function tapText(selector, pattern) {
  const find = `[...document.querySelectorAll(${JSON.stringify(selector)})].findIndex((el) => new RegExp(${JSON.stringify(pattern)}).test((el.innerText || el.getAttribute('aria-label') || '').trim()))`
  await waitFor(`${find} >= 0`)
  const index = await js(find)
  if (index < 0) throw new Error(`no ${selector} matching ${pattern}`)
  await tap(selector, index)
}
async function waitFor(expression, timeout = 6000) {
  const start = Date.now()
  while (Date.now() - start < timeout) {
    if (await js(expression)) return true
    await sleep(150)
  }
  return false
}
async function viewport(width, height, mobile) {
  mobileNow = mobile
  await send('Emulation.setDeviceMetricsOverride', { width, height, deviceScaleFactor: 1, mobile })
  await send('Emulation.setTouchEmulationEnabled', { enabled: mobile })
}
async function open(route, { lang = 'zhCN', theme = 'light', width, height, mobile, ed = 'full' }) {
  edition = ed
  await send('Network.clearBrowserCookies')
  await send('Network.setCookie', { name: 'vite-ui-theme', value: theme, url: ORIGIN })
  await viewport(width, height, mobile)
  await load(`${ORIGIN}/__preview`)
  await js(`localStorage.setItem('i18nextLng', ${JSON.stringify(lang)}); localStorage.removeItem('status'); true`)
  await load(`${ORIGIN}${route}`)
  await sleep(2200)
}
const shots = []
async function shot(file) {
  const r = await send('Page.captureScreenshot', { format: 'png' })
  writeFileSync(path.join(outDir, file), Buffer.from(r.data, 'base64'))
  shots.push(file)
}
const checks = []
const check = (name, pass, detail) => checks.push({ name, pass: Boolean(pass), detail })
const layout = `(() => {
  const shell = document.querySelector('[data-myapi-shell]')
  return {
    lang: document.documentElement.lang,
    composition: shell?.dataset.composition,
    overflow: document.documentElement.scrollWidth - innerWidth,
    inspector: document.querySelector('[data-inspector]')?.dataset.inspector ?? null,
    h1: document.querySelector('main h1')?.textContent,
    topTitle: document.querySelector('.pc-mobile-title')?.textContent ?? null,
    crumbs: [...document.querySelectorAll('[data-myapi-context] li')].map((li) => li.textContent.trim()).filter(Boolean),
  }
})()`
const firstScreen = (selector) => `(() => { const el = document.querySelector(${JSON.stringify(selector)}); if (!el) return null; const r = el.getBoundingClientRect(); return { top: Math.round(r.top), bottom: Math.round(r.bottom), innerHeight } })()`
const touchTargets = (scope) => `(() => [...document.querySelectorAll(${JSON.stringify(scope)} + ' :is(button, a, [role=tab])')].filter((el) => el.offsetParent && !el.closest('[aria-hidden=true]')).map((el) => { const r = el.getBoundingClientRect(); return { label: (el.innerText || el.getAttribute('aria-label') || '').trim().slice(0, 24), w: Math.round(r.width), h: Math.round(r.height) } }).filter((t) => t.w < 44 || t.h < 44))()`

// 1. Main screenshots: three pages, PC and phone, light and dark (zh).
const mainPages = [
  ['overview', '/dashboard/overview'],
  ['keys-inspector', '/keys?inspect=21'],
  ['request-body', '/keys?inspect=21&request=SYNTHETIC-CONTEXT-ERROR&requestTab=body'],
  ['connections-inspector', '/connections?connection=1'],
]
for (const theme of ['light', 'dark']) {
  for (const [width, height, mobile] of [[1280, 800, false], [390, 844, true]]) {
    for (const [name, route] of mainPages) {
      await open(route, { theme, width, height, mobile })
      const s = await js(layout)
      check(`${name}-${width}-${theme}: no horizontal overflow`, s.overflow <= 0, s)
      check(`${name}-${width}-${theme}: ${mobile ? 'mobile' : 'desktop'} composition`, s.composition === (mobile ? 'mobile' : 'desktop'), s.composition)
      if (name !== 'overview') {
        check(`${name}-${width}-${theme}: inspector is ${mobile ? 'a full page' : 'beside the list'}`, s.inspector === (mobile ? 'page' : 'side'), s.inspector)
      }
      await shot(`${name}-${width}-${theme}.png`)
    }
  }
}

// 2. Lists without an inspector, PC and phone light.
for (const [width, height, mobile] of [[1280, 800, false], [390, 844, true]]) {
  for (const [name, route] of [['keys', '/keys'], ['connections', '/connections'], ['account-windows', '/upstream-accounts']]) {
    await open(route, { width, height, mobile })
    const s = await js(layout)
    check(`${name}-${width}: no horizontal overflow`, s.overflow <= 0, s)
    await shot(`${name}-${width}-light.png`)
  }
}

// 3. First screen shows data (P1, D1/H1).
await open('/dashboard/overview', { width: 1280, height: 800, mobile: false })
const ov1280 = await js(`(() => {
  const vis = (sel) => { const el = document.querySelector(sel); if (!el) return null; const r = el.getBoundingClientRect(); return r.top < innerHeight && r.bottom > 0 }
  const rows = [...document.querySelectorAll('[aria-labelledby=overview-recent-title] tbody tr')].filter((tr) => tr.getBoundingClientRect().bottom <= innerHeight).length
  return { health: vis('[aria-labelledby=overview-health-title]'), attention: vis('[aria-labelledby=overview-attention-title]'), windows: vis('[aria-labelledby=overview-windows-title]'), recentRowsInView: rows }
})()`)
check('overview 1280x800: health, attention, windows and >=3 recent rows in the first screen', ov1280.health && ov1280.attention && ov1280.windows && ov1280.recentRowsInView >= 3, ov1280)
await open('/dashboard/overview', { width: 390, height: 844, mobile: true })
const ov390 = await js(`(() => {
  const top = (sel) => document.querySelector(sel)?.getBoundingClientRect().top ?? null
  return { health: top('[aria-labelledby=overview-health-title]'), attention: top('[aria-labelledby=overview-attention-title]'), innerHeight }
})()`)
check('overview 390x844: health and attention start in the first screen', ov390.health !== null && ov390.attention !== null && ov390.attention < ov390.innerHeight, ov390)
await open('/keys', { width: 390, height: 844, mobile: true })
const keys390 = await js(firstScreen('[data-inspect-id]'))
check('keys 390x844: the first Key row is in the first screen', keys390 && keys390.bottom <= keys390.innerHeight - 60, keys390)
const clippedStatus = await js(`[...document.querySelectorAll('[data-inspect-id] [data-slot=status-badge], [data-inspect-id] [data-slot=status-badge] *')].filter((el) => el.scrollWidth > el.clientWidth + 1).map((el) => el.innerText)`)
check('keys 390: Key status labels are not cut off', clippedStatus.length === 0, clippedStatus)
await open('/keys?inspect=21', { width: 390, height: 844, mobile: true })
const keyReq390 = await js(firstScreen('[data-request-row]'))
check('Key inspector 390x844: the first request row is in the first screen (D1/H1)', keyReq390 && keyReq390.bottom <= keyReq390.innerHeight - 60, keyReq390)
const keyTargets = await js(touchTargets('[data-inspector=page]'))
check('Key inspector 390: every control in the inspector is at least 44x44', keyTargets.length === 0, keyTargets)
await open('/connections', { width: 390, height: 844, mobile: true })
const conn390 = await js(firstScreen('[data-connection-id]'))
check('connections 390x844: the first connection row is in the first screen', conn390 && conn390.bottom <= conn390.innerHeight - 60, conn390)

// 4. Journey: task 1 on PC. Keys list -> row -> recent request -> body tab.
const steps = []
await open('/keys', { width: 1280, height: 800, mobile: false })
let clicks = 0
await tap('[data-inspect-id="21"]'); clicks++
check('task 1 PC: the Key row opens its recent requests beside the list', await waitFor(`Boolean(document.querySelector('[data-inspector=side] [data-request-row]'))`), await js(layout))
await shot('task1-1280-key-open.png')
await tap('[data-request-row]'); clicks++
const reqOpen = await waitFor(`Boolean(document.querySelector('[data-testid=request-inspector]'))`)
const reqState = await js(`({ search: location.search, facts: document.querySelector('[data-testid=request-inspector]')?.innerText.slice(0, 400) })`)
check('task 1 PC: the request opens with its result and diagnosis (2 clicks)', reqOpen && /request=/.test(reqState.search), reqState)
await shot('task1-1280-request.png')
steps.push({ task: 1, layout: 'PC 1280', clicks, path: 'Keys list row -> recent request' })
await tapText('[data-testid=request-inspector] [role=tab]', '正文')
const body = await waitFor(`/合成|Synthetic/.test(document.querySelector('[data-testid=request-inspector] [role=tabpanel]')?.innerText ?? '')`)
check('task 1 PC: the body tab shows the saved, redacted body without resending', body && writes.length === 0, { writes, search: await js('location.search') })
await shot('task1-1280-body.png')
await tapText('[data-testid=request-inspector] [role=tab]', 'SSE')
await sleep(600)
await shot('task1-1280-sse.png')
// Back returns to the Key with focus on the request row.
await tapText('[data-testid=request-inspector] button', '返回')
await sleep(700)
const backPc = await js(`({ search: location.search, focus: document.activeElement?.getAttribute('data-request-row') })`)
check('task 1 PC: Back returns to the Key with focus on the request', !/request=/.test(backPc.search) && backPc.focus !== null && backPc.focus !== undefined, backPc)

// 5. Journey: task 1 on a phone, with the top-bar Back and focus return.
await open('/keys', { width: 390, height: 844, mobile: true })
clicks = 0
await tap('[data-inspect-id="21"]'); clicks++
await waitFor(`Boolean(document.querySelector('[data-inspector=page] [data-request-row]'))`)
const keyPage = await js(layout)
check('task 1 phone: the Key opens as a full page with Back in the top bar', keyPage.inspector === 'page' && keyPage.topTitle === 'API Key #21' && (await js(`Boolean(document.querySelector('.pc-header-mobile button[aria-label="返回"]'))`)), keyPage)
await shot('task1-390-key-page.png')
await tap('[data-request-row]'); clicks++
await waitFor(`Boolean(document.querySelector('[data-testid=request-inspector]'))`)
await shot('task1-390-request.png')
steps.push({ task: 1, layout: 'phone 390', clicks: clicks + 1, path: 'bottom nav Keys -> Key -> request (+1 nav tap from overview)' })
await tapText('[data-testid=request-inspector] [role=tab]', '正文')
await sleep(900)
await shot('task1-390-body.png')
await tap('.pc-header-mobile button[aria-label="返回"]')
await sleep(800)
const phoneBack1 = await js(`({ search: location.search, focus: document.activeElement?.getAttribute('data-request-row') })`)
check('phone: top-bar Back from the request returns to the Key with focus on the request', !/request=/.test(phoneBack1.search) && phoneBack1.focus, phoneBack1)
await tap('.pc-header-mobile button[aria-label="返回"]')
await sleep(800)
const phoneBack2 = await js(`({ search: location.search, focus: document.activeElement?.getAttribute('data-inspect-id'), listVisible: Boolean(document.querySelector('[data-inspect-id="21"]')?.offsetParent) })`)
check('phone: top-bar Back from the Key returns to the list with focus on the Key', !/inspect=/.test(phoneBack2.search) && phoneBack2.focus === '21' && phoneBack2.listVisible, phoneBack2)
// The inspector's controls are not covered by the fixed bars.
await open('/keys?inspect=21', { width: 390, height: 844, mobile: true })
const covered = await js(`(async () => {
  const buttons = [...document.querySelectorAll('[data-inspector=page] button')].filter((b) => !b.disabled && b.offsetParent)
  const out = []
  for (const b of buttons) {
    b.scrollIntoView({ block: 'center' })
    await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)))
    const r = b.getBoundingClientRect()
    const hit = document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2)
    if (!(hit === b || b.contains(hit))) out.push((b.innerText || b.getAttribute('aria-label') || '').trim().slice(0, 30))
  }
  return { checked: buttons.length, covered: out }
})()`)
check('phone: every inspector button can be scrolled clear of the top and bottom bars', covered.checked > 0 && covered.covered.length === 0, covered)

// 6. Journey: task 2. Ctrl+K, paste a Request ID, Enter opens its details.
await open('/dashboard/overview', { width: 1280, height: 800, mobile: false })
await key('k', 'KeyK', 75, 2)
await sleep(500)
await send('Input.insertText', { text: 'SYNTHETIC-CONTEXT-ERROR' })
await sleep(400)
const option = await js(`document.querySelector('[cmdk-item][aria-selected=true]')?.textContent`)
check('task 2: the pasted Request ID is the selected command', /打开请求 SYNTHETIC-CONTEXT-ERROR/.test(option ?? ''), option)
await shot('task2-1280-command.png')
await key('Enter', 'Enter', 13, 0, '\r')
const direct = await waitFor(`/SYNTHETIC-CONTEXT-ERROR/.test(document.querySelector('[data-testid=request-inspector]')?.innerText ?? '')`, 8000)
const directState = await js(`({ path: location.pathname, search: decodeURIComponent(location.search), inspector: document.querySelector('[data-inspector]')?.dataset.inspector, crumbs: [...document.querySelectorAll('[data-myapi-context] li')].map((li) => li.textContent.trim()).filter(Boolean) })`)
check('task 2: Enter opens the request details directly (1 step after paste)', direct && directState.path === '/usage-logs/common' && /inspect=request/.test(directState.search), directState)
await shot('task2-1280-details.png')
steps.push({ task: 2, layout: 'PC 1280', clicks: 1, path: 'Ctrl+K, paste, Enter' })
await open('/dashboard/overview', { width: 390, height: 844, mobile: true })
await tap('.pc-header-mobile button[aria-label="搜索"]')
await sleep(500)
await send('Input.insertText', { text: 'SYNTHETIC-CONTEXT-ERROR' })
await sleep(300)
await key('Enter', 'Enter', 13, 0, '\r')
const directPhone = await waitFor(`/SYNTHETIC-CONTEXT-ERROR/.test(document.querySelector('[data-inspector=page] [data-testid=request-inspector]')?.innerText ?? '')`, 8000)
check('task 2 phone: search icon, paste, Enter opens the full-page details (2 steps)', directPhone, await js(layout))
await shot('task2-390-details.png')
steps.push({ task: 2, layout: 'phone 390', clicks: 2, path: 'search icon, paste, Enter' })

// 7. Journey: task 6. Connection row -> Test (the preview refuses provider calls).
await open('/connections', { width: 1280, height: 800, mobile: false })
await tap('[data-connection-id="1"]')
await waitFor(`Boolean(document.querySelector('[data-testid=connection-detail] h2'))`)
await tapText('[data-testid=connection-detail] button', '^测试$')
const tested = await waitFor(`/测试失败|测试成功/.test(document.querySelector('[data-testid=connection-detail]')?.innerText ?? '')`, 8000)
const testText = await js(`document.querySelector('[data-testid=connection-detail] [aria-live]')?.innerText`)
check('task 6: row then Test runs the existing test and shows its result inline (2 clicks)', tested, testText)
await shot('task6-1280-test.png')
steps.push({ task: 6, layout: 'PC 1280', clicks: 2, path: 'connection row -> Test' })
await open('/connections', { width: 390, height: 844, mobile: true })
await tap('[data-connection-id="1"]')
await waitFor(`Boolean(document.querySelector('[data-inspector=page] [data-testid=connection-detail]'))`)
await tapText('[data-testid=connection-detail] button', '^测试$')
await waitFor(`/测试失败|测试成功/.test(document.querySelector('[data-testid=connection-detail]')?.innerText ?? '')`, 8000)
await shot('task6-390-test.png')
steps.push({ task: 6, layout: 'phone 390', clicks: 2, path: 'connection row -> Test' })

// 8. Journey: task 4. Add connection opens the editor with "Save and test".
await open('/connections', { width: 1280, height: 800, mobile: false })
await tapText('main button', '添加连接')
const editor = await waitFor(`/添加连接/.test(document.querySelector('[data-slot=sheet-content]')?.innerText ?? '') && [...document.querySelectorAll('[data-slot=sheet-content] button')].some((b) => b.innerText.trim() === '保存并测试')`, 8000)
check('task 4: Add connection opens the editor titled 添加连接 with 保存并测试', editor, await js(`document.querySelector('[data-slot=sheet-content] h2')?.innerText`))
const headerOverlaps = await js(`(() => {
  const sheet = document.querySelector('[data-slot=sheet-content]')
  const top = sheet.getBoundingClientRect().top
  const buttons = [...sheet.querySelectorAll('button')].filter((b) => b.getBoundingClientRect().top < top + 90 && b.offsetParent)
  const out = []
  for (const a of buttons) for (const b of buttons) {
    if (a === b) continue
    const r = a.getBoundingClientRect(), s = b.getBoundingClientRect()
    if (r.left < s.right && s.left < r.right && r.top < s.bottom && s.top < r.bottom) out.push([a.innerText.trim() || a.getAttribute('aria-label'), b.innerText.trim() || b.getAttribute('aria-label')])
  }
  return out
})()`)
check('task 4: no buttons overlap in the editor header (paste vs close)', headerOverlaps.length === 0, headerOverlaps)
await shot('task4-1280-editor.png')
steps.push({ task: 4, layout: 'PC 1280', clicks: 2, path: 'Add connection -> (fill form) -> Save and test; preview refuses the write' })

// 9. Journey: task 9. Overview "失败 N" opens the failed requests.
await open('/dashboard/overview', { width: 1280, height: 800, mobile: false })
const failedLink = await js(`[...document.querySelectorAll('[aria-labelledby=overview-recent-title] a')].find((a) => /^失败 \\d+/.test(a.innerText.trim()))?.getAttribute('href') ?? null`)
check('task 9: the overview offers 失败 N linked to failed requests of 24 hours', failedLink && /type=\["5"\]/.test(decodeURIComponent(failedLink)) && /startTime=/.test(failedLink), failedLink && decodeURIComponent(failedLink))
if (failedLink) {
  await tapText('[aria-labelledby=overview-recent-title] a', '^失败 \\d+')
  await waitFor(`location.pathname === '/usage-logs/common'`)
  const landed = await js(`({ search: decodeURIComponent(location.search), filter: document.querySelector('main')?.innerText.includes('错误') })`)
  check('task 9: one click lands on the call records filtered to errors', /type=\["5"\]/.test(landed.search), landed)
  const failedRows = await waitFor(`[...document.querySelectorAll('main tbody tr')].filter((tr) => tr.innerText.trim() && getComputedStyle(tr).opacity === '1').length >= 1`)
  check('task 9: the failed requests are listed on arrival', failedRows, await js(`[...document.querySelectorAll('main tbody tr')].map((tr) => ({ text: tr.innerText.trim().slice(0, 40), opacity: getComputedStyle(tr).opacity }))`))
  await shot('task9-1280-failed.png')
}
steps.push({ task: 9, layout: 'PC 1280', clicks: 1, path: 'overview 失败 N' })

// 9b. Key inspector -> all of its call records -> Back to Keys returns focus
// to that Key (PC: its "View Key requests" button; phone: the Key card).
for (const [width, height, mobile] of [[1280, 800, false], [390, 844, true]]) {
  await open('/keys?inspect=21', { width, height, mobile })
  await waitFor(`Boolean(document.querySelector('[data-inspector] [data-request-row]'))`)
  await tapText('[data-inspector] a', '^在调用记录中查看全部$')
  await waitFor(`location.pathname === '/usage-logs/common' && /tokenId=21/.test(location.search)`)
  const backLabel = await waitFor(`[...document.querySelectorAll('main a')].some((a) => a.innerText.trim() === '返回 API Key 列表')`)
  await tapText('main a', '^返回 API Key 列表$')
  await waitFor(`location.pathname === '/keys'`)
  await sleep(1200)
  const returned = await js(`({ search: decodeURIComponent(location.search), active: document.activeElement?.getAttribute('data-inspect-id') ?? document.activeElement?.textContent?.trim() ?? null })`)
  check(`${width}: from the Key's call records, Back to Keys returns focus to that Key`, /focusKeyId=21/.test(returned.search) && (returned.active === '21' || returned.active === '查看 Key 请求'), { backLabel, ...returned })
}

// 10. 1024 overlay inspector and 768 page inspector; 320 Russian.
await open('/keys?inspect=21', { width: 1024, height: 768, mobile: false })
const s1024 = await js(layout)
check('1024: the Key inspector overlays the list', s1024.inspector === 'overlay' && s1024.overflow <= 0, s1024)
await shot('keys-inspector-1024-light.png')
await open('/connections?connection=1', { width: 1024, height: 768, mobile: false })
check('1024: the connection inspector overlays the list', (await js(layout)).inspector === 'overlay')
await shot('connections-inspector-1024-light.png')
await open('/dashboard/overview', { width: 1024, height: 768, mobile: false })
check('overview 1024: no horizontal overflow', (await js(layout)).overflow <= 0)
await shot('overview-1024-light.png')
for (const [name, route] of [['overview', '/dashboard/overview'], ['keys-inspector', '/keys?inspect=21']]) {
  await open(route, { width: 768, height: 1024, mobile: true })
  const s = await js(layout)
  check(`${name} 768: mobile composition without overflow`, s.composition === 'mobile' && s.overflow <= 0, s)
  await shot(`${name}-768-light.png`)
}
for (const [name, route] of [['overview', '/dashboard/overview'], ['keys-inspector', '/keys?inspect=21'], ['request-body', '/keys?inspect=21&request=SYNTHETIC-CONTEXT-ERROR&requestTab=body'], ['connections', '/connections?connection=1']]) {
  await open(route, { lang: 'ru', width: 320, height: 700, mobile: true })
  const s = await js(layout)
  check(`${name} 320 ru: no horizontal overflow, lang ru`, s.overflow <= 0 && s.lang === 'ru', s)
  // The page can clip a wide tab row without overflowing, so measure tabs.
  const clippedTabs = await js(`[...document.querySelectorAll('[role=tablist]')].flatMap((list) => { const box = list.getBoundingClientRect(); return [...list.querySelectorAll('[role=tab]')].filter((tab) => { const r = tab.getBoundingClientRect(); return r.left < box.left - 1 || r.right > box.right + 1 || tab.scrollWidth > tab.clientWidth + 1 }).map((tab) => tab.innerText) })`)
  check(`${name} 320 ru: every tab label fits inside its tab row`, clippedTabs.length === 0, clippedTabs)
  await shot(`${name}-320-ru.png`)
}

// 11. Lite overview: no performance or content panels.
await open('/dashboard/overview', { width: 1280, height: 800, mobile: false, ed: 'lite' })
const lite = await js(`({ text: document.querySelector('main')?.innerText ?? '' })`)
check('lite overview: no performance or site content panels', !/性能|公告|常见问题/.test(lite.text), lite.text.slice(0, 200))
await shot('overview-1280-lite.png')

// 12. Headings match the navigation names.
const headings = []
for (const [route, nav] of [['/dashboard/overview', '概览'], ['/keys', 'API Key'], ['/connections', '连接'], ['/upstream-accounts', '账号窗口']]) {
  await open(route, { width: 1280, height: 800, mobile: false })
  const h1 = await js(`document.querySelector('main h1')?.textContent`)
  headings.push({ route, h1, nav })
}
check('page h1 equals the navigation name on all four pages', headings.every((h) => h.h1 === h.nav), headings)

writeFileSync(path.join(outDir, 'browser-checks.json'), JSON.stringify({ blocked, pageErrors, writes, steps, shots, checks, failed: checks.filter((c) => !c.pass).map((c) => c.name) }, null, 2))
console.log(JSON.stringify({ blocked, pageErrors: pageErrors.length, writes, shots: shots.length, checks: checks.length, failed: checks.filter((c) => !c.pass) }, null, 2))
ws.close()
chrome.kill()
