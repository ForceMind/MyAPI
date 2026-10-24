// Headless Chromium checks of the console shell against the synthetic preview.
// Only 127.0.0.1:4184 is reachable; every other request is blocked and counted.
// Usage: node shell-shots.mjs <outDir>
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
const PORT = 9334
const profile = path.join(path.dirname(new URL(import.meta.url).pathname), 'chrome-profile-shell')
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
listeners.push(async (msg) => {
  if (msg.method !== 'Fetch.requestPaused') return
  const p = msg.params
  const url = p.request.url
  if (!(url.startsWith(ORIGIN) || url.startsWith('data:'))) {
    blocked++
    await send('Fetch.failRequest', { requestId: p.requestId, errorReason: 'BlockedByClient' })
    return
  }
  // Response stage of /api/status: optionally rewrite to a Lite edition.
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
async function clickSelector(selector) {
  const box = await js(`(() => { const r = document.querySelector(${JSON.stringify(selector)}).getBoundingClientRect(); return { x: r.x + r.width / 2, y: r.y + r.height / 2 } })()`)
  for (const type of ['mousePressed', 'mouseReleased']) {
    await send('Input.dispatchMouseEvent', { type, x: box.x, y: box.y, button: 'left', clickCount: 1 })
  }
}
async function viewport(width, height, mobile) {
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
async function shot(file) {
  const r = await send('Page.captureScreenshot', { format: 'png' })
  writeFileSync(path.join(outDir, file), Buffer.from(r.data, 'base64'))
  shots.push(file)
}
const state = `(() => {
  const shell = document.querySelector('[data-myapi-shell]')
  return {
    lang: document.documentElement.lang,
    composition: shell?.dataset.composition,
    collapsed: shell?.dataset.collapsed,
    overflow: document.documentElement.scrollWidth - innerWidth,
    sidebarWidth: document.querySelector('.pc-sidebar')?.getBoundingClientRect().width ?? null,
    bottomTabs: [...document.querySelectorAll('.pc-mobile-nav > a, .pc-mobile-nav > .pc-more')].map((e) => e.textContent.trim()),
  }
})()`

const shots = []
const checks = []
const check = (name, pass, detail) => checks.push({ name, pass: Boolean(pass), detail })

// 1. Four pages x 1280/390 x light/dark (zh).
const pages = [
  ['overview', '/dashboard/overview'],
  ['key-requests', '/usage-logs/common?tokenId=21&pageSize=20'],
  ['upstream-accounts', '/upstream-accounts'],
  ['settings', '/system-settings/site/system-info'],
]
for (const theme of ['light', 'dark']) {
  for (const [width, height, mobile] of [[1280, 900, false], [390, 844, true]]) {
    for (const [name, route] of pages) {
      await open(route, { theme, width, height, mobile })
      const s = await js(state)
      check(`${name}-${width}-${theme}: no horizontal overflow`, s.overflow <= 0, s)
      check(`${name}-${width}-${theme}: ${mobile ? 'mobile' : 'desktop'} composition`, s.composition === (mobile ? 'mobile' : 'desktop'), s.composition)
      await shot(`${name}-${width}-${theme}.png`)
    }
  }
}

// 2. 320 Russian: bottom navigation labels and touch targets.
await open('/dashboard/overview', { lang: 'ru', width: 320, height: 700, mobile: true })
const ru = await js(`(() => [...document.querySelectorAll('.pc-mobile-nav > a, .pc-mobile-nav > .pc-more')].map((e) => {
  const r = e.getBoundingClientRect(); const span = e.querySelector('span'); const lh = parseFloat(getComputedStyle(span).lineHeight)
  return { label: span.textContent, w: Math.round(r.width), h: Math.round(r.height), lines: Math.round(span.getBoundingClientRect().height / lh), clipped: span.scrollWidth > span.clientWidth + 1 }
}))()`)
check('320 ru: five bottom targets', ru.length === 5, ru.map((t) => t.label))
check('320 ru: every target >= 44x44', ru.every((t) => t.w >= 44 && t.h >= 44), ru)
check('320 ru: labels at most two lines, no word clipped', ru.every((t) => t.lines <= 2 && !t.clipped), ru)
check('320 ru: no horizontal overflow', (await js(state)).overflow <= 0)
await shot('overview-320-ru.png')
await clickSelector('.pc-more')
await sleep(600)
await shot('more-320-ru.png')
await key('Escape', 'Escape', 27)
await sleep(400)

// 3. 768 (still mobile composition) and 1024 (rail by default, then expanded).
await open('/usage-logs/common?tokenId=21&pageSize=20', { width: 768, height: 1024, mobile: true })
check('768: mobile composition', (await js(state)).composition === 'mobile')
await shot('key-requests-768-light.png')
await open('/dashboard/overview', { width: 1024, height: 768, mobile: false })
let s1024 = await js(state)
check('1024: desktop composition starts as a 64px rail', s1024.composition === 'desktop' && s1024.collapsed === 'true' && Math.round(s1024.sidebarWidth) === 64, s1024)
await shot('overview-1024-rail.png')
await clickSelector('.pc-collapse')
await sleep(500)
s1024 = await js(state)
check('1024: expand control opens the 240px sidebar', s1024.collapsed === 'false' && Math.round(s1024.sidebarWidth) === 240, s1024)
await shot('overview-1024-expanded.png')

// 4. Lite edition (status rewritten): Commerce and Drawing are absent.
await open('/dashboard/overview', { width: 1280, height: 900, mobile: false, ed: 'lite' })
const liteNav = await js(`(() => ({ groups: [...document.querySelectorAll('.pc-desktop-nav [role=group]')].map((g) => g.getAttribute('aria-label')), links: [...document.querySelectorAll('.pc-desktop-nav a')].map((a) => a.getAttribute('href')) }))()`)
check('lite 1280: no Commerce group and no drawing entry', !liteNav.groups.includes('商业') && !liteNav.links.includes('/usage-logs/drawing'), liteNav)
await shot('overview-1280-lite.png')
await open('/dashboard/overview', { width: 1280, height: 900, mobile: false, ed: 'full' })
const fullNav = await js(`(() => ({ groups: [...document.querySelectorAll('.pc-desktop-nav [role=group]')].map((g) => g.getAttribute('aria-label')), links: [...document.querySelectorAll('.pc-desktop-nav a')].map((a) => a.getAttribute('href')) }))()`)
check('full 1280: drawing entry present', fullNav.links.includes('/usage-logs/drawing'), fullNav)
await open('/dashboard/overview', { width: 390, height: 844, mobile: true, ed: 'lite' })
await clickSelector('.pc-more')
await sleep(600)
const liteMore = await js(`[...document.querySelectorAll('[role=dialog] a')].map((a) => a.getAttribute('href'))`)
check('lite 390: More has no commerce or drawing entries', !liteMore.some((h) => ['/wallet', '/subscriptions', '/redemption-codes', '/usage-logs/drawing'].includes(h)), liteMore)
await shot('more-390-lite.png')

// 5. Keyboard: Ctrl+K opens search, pasted Request ID + Enter opens filtered call records.
await open('/dashboard/overview', { width: 1280, height: 900, mobile: false })
await key('k', 'KeyK', 75, 2)
await sleep(500)
const paletteOpen = await js(`Boolean(document.querySelector('[cmdk-input]')) && document.activeElement === document.querySelector('[cmdk-input]')`)
check('Ctrl+K opens the command search with the input focused', paletteOpen)
await send('Input.insertText', { text: 'SYNTHETIC-CONTEXT-ERROR' })
await sleep(400)
const selected = await js(`document.querySelector('[cmdk-item][aria-selected=true]')?.textContent`)
check('pasted Request ID is the selected command', /SYNTHETIC-CONTEXT-ERROR/.test(selected ?? ''), selected)
await shot('command-request-id-1280.png')
await key('Enter', 'Enter', 13, 0, '\r')
await sleep(2500)
const afterEnter = await js(`({ path: location.pathname, search: location.search, rows: [...document.querySelectorAll('main tbody tr, main [data-request-id]')].length, mentions: document.querySelector('main')?.innerText.split('SYNTHETIC-CONTEXT-ERROR').length - 1 })`)
check('Enter navigates to /usage-logs/common?requestId=...', afterEnter.path === '/usage-logs/common' && /requestId=SYNTHETIC-CONTEXT-ERROR/.test(afterEnter.search), afterEnter)
await shot('request-id-result-1280.png')

// 6. Sidebar collapse/expand with focus kept on the control (1280).
await clickSelector('.pc-collapse')
await sleep(400)
const collapsed = await js(`({ collapsed: document.querySelector('[data-myapi-shell]').dataset.collapsed, focus: document.activeElement?.classList.contains('pc-collapse'), cookie: document.cookie })`)
check('1280: collapse control folds the sidebar and persists sidebar_state', collapsed.collapsed === 'true' && /sidebar_state=false/.test(collapsed.cookie), collapsed)
await shot('key-requests-1280-rail.png')
await clickSelector('.pc-collapse')
await sleep(400)
check('1280: expand control restores the sidebar', (await js(state)).collapsed === 'false')

// 7. Mobile More: keyboard open/close returns focus; M1 controls not covered.
await open('/usage-logs/common?tokenId=21&pageSize=20', { width: 390, height: 844, mobile: true })
await js(`document.querySelector('.pc-more').focus(); true`)
await key('Enter', 'Enter', 13, 0, '\r')
await sleep(600)
const moreOpen = await js(`Boolean(document.querySelector('[role=dialog]'))`)
await key('Escape', 'Escape', 27)
await sleep(600)
const moreClosed = await js(`({ open: Boolean(document.querySelector('[role=dialog]')), focusReturned: document.activeElement === document.querySelector('.pc-more') })`)
check('390: More opens by keyboard and Escape returns focus to it', moreOpen && !moreClosed.open && moreClosed.focusReturned, moreClosed)
const m1 = await js(`(async () => {
  const nav = document.querySelector('.pc-mobile-nav').getBoundingClientRect()
  const header = document.querySelector('.pc-header').getBoundingClientRect()
  const buttons = [...document.querySelectorAll('main button')].filter((b) => !b.disabled && b.offsetParent && b.getBoundingClientRect().height > 0)
  const covered = []
  for (const b of buttons) {
    b.scrollIntoView({ block: 'center' })
    await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)))
    const r = b.getBoundingClientRect()
    const hit = document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2)
    if (!(hit === b || b.contains(hit))) covered.push((b.innerText || b.getAttribute('aria-label') || '').trim().slice(0, 30))
  }
  const content = document.querySelector('.pc-route-content')
  content.scrollTop = content.scrollHeight
  await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)))
  const footer = document.querySelector('.myapi-page-footer')
  const fr = footer?.getBoundingClientRect()
  const shell = document.querySelector('.personal-console')
  const headerAfter = document.querySelector('.pc-header').getBoundingClientRect()
  return { checked: buttons.length, covered, fixedBars: [header.height, nav.height].map(Math.round), footerBottom: fr ? Math.round(fr.bottom) : null, navTop: Math.round(nav.top), shellScrollTop: shell.scrollTop, headerTopAfter: Math.round(headerAfter.top), navBottomAfter: Math.round(document.querySelector('.pc-mobile-nav').getBoundingClientRect().bottom), innerHeight }
})()`)
check('390 key requests: every page button can be scrolled clear of both fixed bars', m1.checked > 0 && m1.covered.length === 0, m1)
check('390 key requests: pagination scrolls with content and ends above the bottom bar', m1.footerBottom === null || m1.footerBottom <= m1.navTop, m1)
check('390 key requests: top bar and bottom bar stay pinned after scrolling', m1.shellScrollTop === 0 && m1.headerTopAfter === 0 && m1.navBottomAfter === m1.innerHeight, m1)
await shot('key-requests-390-scrolled-end.png')

console.log(JSON.stringify({ blocked, pageErrors, shots, checks, failed: checks.filter((c) => !c.pass).map((c) => c.name) }, null, 2))
ws.close()
chrome.kill()
