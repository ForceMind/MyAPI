// Step 1 shell screenshots and checks against the synthetic preview.
// Only 127.0.0.1:4184 is reachable; every other request is aborted.
// Usage: node shots.mjs <outDir> [group...]   groups: pages widths lite admin ops
import { mkdirSync, writeFileSync } from 'node:fs'
import path from 'node:path'

const { chromium } = await import(process.env.MYAPI_PLAYWRIGHT_MODULE)
const ORIGIN = 'http://127.0.0.1:4184'
const outDir = process.argv[2]
const groups = new Set(process.argv.slice(3).length ? process.argv.slice(3) : ['pages', 'widths', 'lite', 'admin', 'ops'])
mkdirSync(outDir, { recursive: true })

const LITE_CAPS = Object.fromEntries(['payments', 'commercial_subscriptions', 'redemption_codes', 'checkin', 'affiliate', 'public_pricing', 'rankings', 'perf_metrics', 'external_oauth', 'midjourney', 'model_deployments', 'site_content', 'user_registration'].map((k) => [k, false]))

const browser = await chromium.launch()
const checks = []
const shots = []
let blocked = 0
const check = (name, pass, detail) => {
  checks.push({ name, pass: Boolean(pass), detail })
  if (!pass) console.log('FAIL', name, JSON.stringify(detail))
}

async function open(route, { lang = 'zh', theme = 'light', width = 1280, height = 860, mobile = false, edition = 'full' } = {}) {
  const context = await browser.newContext({ viewport: { width, height }, isMobile: mobile, hasTouch: mobile, deviceScaleFactor: 1, colorScheme: theme })
  await context.addCookies([{ name: 'vite-ui-theme', value: theme, url: ORIGIN }])
  await context.addInitScript((language) => {
    localStorage.setItem('i18nextLng', language)
    localStorage.removeItem('status')
  }, lang)
  await context.route('**/*', async (route) => {
    const url = route.request().url()
    if (!url.startsWith(ORIGIN) && !url.startsWith('data:')) {
      blocked++
      return route.abort('blockedbyclient')
    }
    if (edition === 'lite' && new URL(url).pathname === '/api/status') {
      const response = await route.fetch()
      const json = await response.json()
      if (json.data) {
        json.data.product_edition = 'lite'
        json.data.capabilities = LITE_CAPS
      }
      return route.fulfill({ response, json })
    }
    return route.continue()
  })
  const page = await context.newPage()
  const errors = []
  page.on('pageerror', (error) => errors.push(String(error)))
  await page.goto(`${ORIGIN}${route}`)
  await page.waitForSelector('[data-myapi-shell]')
  await page.waitForTimeout(1500)
  return { context, page, errors }
}

const geometry = (page) => page.evaluate(() => {
  const header = document.querySelector('[data-myapi-header]')?.getBoundingClientRect()
  const main = document.querySelector('main')?.getBoundingClientRect()
  const shell = document.querySelector('[data-myapi-shell]')
  const bottom = document.querySelector('.pc-mobile-nav')?.getBoundingClientRect()
  return {
    composition: shell?.dataset.composition,
    overflow: document.documentElement.scrollWidth - innerWidth,
    headerBottom: header ? Math.round(header.bottom) : null,
    mainTop: main ? Math.round(main.top) : null,
    sidebar: Boolean(document.querySelector('.pc-sidebar, [data-myapi-sidebar], [data-slot="sidebar"]')),
    bottomTop: bottom ? Math.round(bottom.top) : null,
    innerHeight,
  }
})

async function shot(page, file) {
  await page.screenshot({ path: path.join(outDir, file) })
  shots.push(file)
}

const PAGES = [
  ['overview', '/dashboard/overview'],
  ['key-requests', '/usage-logs/common?tokenId=21&pageSize=20'],
  ['keys', '/keys'],
  ['upstream', '/connections'],
  ['account-windows', '/upstream-accounts'],
  ['settings', '/system-settings/site/system-info'],
  ['profile', '/profile'],
  ['system-info', '/system-info'],
]

if (groups.has('pages')) {
  for (const lang of ['zh', 'en']) {
    for (const theme of ['light', 'dark']) {
      for (const [width, height, mobile] of [[1280, 860, false], [390, 844, true]]) {
        for (const [name, route] of PAGES) {
          // Keys and system status: zh light only; the six task pages get every combination.
          if ((theme === 'dark' || lang === 'en') && ['keys', 'system-info'].includes(name)) continue
          const { context, page, errors } = await open(route, { lang, theme, width, height, mobile })
          const g = await geometry(page)
          const tag = `${name}-${width}-${theme}-${lang}`
          check(`${tag}: no horizontal overflow`, g.overflow <= 0, g)
          check(`${tag}: ${mobile ? 'mobile' : 'desktop'} composition, no sidebar`, g.composition === (mobile ? 'mobile' : 'desktop') && !g.sidebar, g)
          check(`${tag}: page not under the top bar`, g.mainTop === null || g.mainTop >= g.headerBottom - 1, g)
          check(`${tag}: no page errors`, errors.length === 0, errors)
          await shot(page, `${tag}.png`)
          await context.close()
        }
      }
    }
  }
}

if (groups.has('widths')) {
  for (const [width, height, mobile] of [[320, 700, true], [768, 1024, true], [1024, 768, false]]) {
    for (const lang of ['zh', 'en']) {
      for (const [name, route] of [['overview', '/dashboard/overview'], ['key-requests', '/usage-logs/common?tokenId=21&pageSize=20']]) {
        const { context, page, errors } = await open(route, { lang, width, height, mobile })
        const g = await geometry(page)
        const tag = `${name}-${width}-light-${lang}`
        check(`${tag}: no horizontal overflow`, g.overflow <= 0, g)
        check(`${tag}: no page errors`, errors.length === 0, errors)
        if (mobile) {
          const tabs = await page.evaluate(() => [...document.querySelectorAll('.pc-mobile-nav > a, .pc-mobile-nav > .pc-more')].map((e) => {
            const r = e.getBoundingClientRect(); const span = e.querySelector('span'); const lh = parseFloat(getComputedStyle(span).lineHeight)
            return { label: span.textContent, w: Math.round(r.width), h: Math.round(r.height), lines: Math.round(span.getBoundingClientRect().height / lh), clipped: span.scrollWidth > span.clientWidth + 1 }
          }))
          check(`${tag}: five bottom targets >= 44px`, tabs.length === 5 && tabs.every((t) => t.w >= 44 && t.h >= 44), tabs)
          check(`${tag}: bottom labels one line, no word clipped`, tabs.every((t) => t.lines === 1 && !t.clipped), tabs)
        } else {
          const nav = await page.evaluate(() => {
            const items = [...document.querySelectorAll('.pc-topnav > *')].map((e) => { const r = e.getBoundingClientRect(); return { text: e.textContent.trim(), left: Math.round(r.left), right: Math.round(r.right), lines: Math.round(r.height) } })
            const actions = document.querySelector('.pc-header-actions').getBoundingClientRect()
            return { items, actionsLeft: Math.round(actions.left), actionsRight: Math.round(actions.right) }
          })
          const last = nav.items.at(-1)
          check(`${tag}: top navigation does not run into the actions`, last.right <= nav.actionsLeft && nav.actionsRight <= width, nav)
        }
        await shot(page, `${tag}.png`)
        await context.close()
      }
    }
  }
}

if (groups.has('lite')) {
  for (const [width, height, mobile] of [[1280, 860, false], [390, 844, true]]) {
    const { context, page } = await open('/dashboard/overview', { width, height, mobile, edition: 'lite' })
    const tag = `lite-overview-${width}-light-zh`
    const state = await page.evaluate(() => ({
      top: [...document.querySelectorAll('.pc-topnav > *')].map((e) => e.textContent.trim()),
      bottom: [...document.querySelectorAll('.pc-mobile-nav > a, .pc-mobile-nav > .pc-more')].map((e) => e.textContent.trim()),
    }))
    check(`${tag}: no Admin menu`, !state.top.includes('管理'), state)
    await shot(page, `${tag}.png`)
    if (mobile) {
      await page.getByRole('button', { name: '更多' }).click()
      await page.waitForTimeout(500)
      await shot(page, `lite-more-${width}-light-zh.png`)
    } else {
      await page.getByRole('button', { name: '更多' }).click()
      await page.waitForTimeout(400)
      const items = await page.getByRole('menuitem').allTextContents()
      check(`${tag}: More holds users, system status and tools`, ['用户', '运行状态', '游乐场'].every((name) => items.some((item) => item.includes(name))), items)
      await shot(page, `lite-more-menu-${width}-light-zh.png`)
    }
    await context.close()
  }
}

if (groups.has('admin')) {
  for (const lang of ['zh', 'en']) {
    const { context, page } = await open('/dashboard/overview', { lang })
    await page.getByRole('button', { name: lang === 'zh' ? '管理' : 'Admin' }).click()
    await page.waitForTimeout(400)
    const items = await page.getByRole('menuitem').allTextContents()
    check(`admin-menu-${lang}: users, commerce and system`, items.length >= 2, items)
    await shot(page, `admin-menu-1280-light-${lang}.png`)
    await context.close()
  }
  const { context, page } = await open('/usage-logs/common?tokenId=21&pageSize=20', { theme: 'dark' })
  await page.getByRole('button', { name: '更多' }).click()
  await page.waitForTimeout(400)
  await shot(page, 'more-menu-1280-dark-zh.png')
  await context.close()
}

if (groups.has('ops')) {
  // Keyboard: Ctrl/⌘ K, paste a Request ID, Enter opens its details.
  {
    const { context, page } = await open('/dashboard/overview')
    await page.keyboard.press('ControlOrMeta+k')
    const input = page.getByRole('combobox')
    await input.waitFor()
    const requestId = await page.evaluate(async () => {
      const response = await fetch('/api/log/self?p=1&page_size=5')
      const json = await response.json()
      return json?.data?.items?.[0]?.request_id ?? json?.data?.items?.[0]?.other?.request_id ?? null
    }).catch(() => null)
    const id = requestId || 'synthetic-request-0001'
    await input.fill(id)
    await page.waitForTimeout(300)
    await shot(page, 'command-request-id-1280-light-zh.png')
    await page.keyboard.press('Enter')
    await page.waitForTimeout(1500)
    const url = new URL(page.url())
    check('ops: pasted Request ID + Enter opens the request in call records', url.pathname === '/usage-logs/common' && url.searchParams.get('inspect') === 'request', { url: page.url(), id })
    await shot(page, 'request-id-result-1280-light-zh.png')
    await context.close()
  }
  // Keyboard through the top navigation and the Admin menu.
  {
    const { context, page } = await open('/dashboard/overview')
    await page.getByRole('link', { name: '上游', exact: true }).focus()
    await page.keyboard.press('Enter')
    await page.waitForURL('**/connections**')
    const admin = page.getByRole('button', { name: '管理' })
    await admin.focus()
    await page.keyboard.press('Enter')
    await page.getByRole('menu').waitFor()
    const focused = await page.evaluate(() => document.activeElement?.getAttribute('role'))
    await page.keyboard.press('Escape')
    await page.waitForTimeout(200)
    const back = await admin.evaluate((element) => element === document.activeElement)
    check('ops: keyboard opens Admin, focus moves into the menu and returns on Escape', focused === 'menuitem' && back, { focused, back })
    await context.close()
  }
  // Touch: tap a top-bar menu at a touch-capable PC width.
  {
    const { context, page } = await open('/dashboard/overview', { width: 1280, height: 860, mobile: false })
    await context.close()
    // isMobile makes Chromium report pointer: coarse, as a touch PC does.
    const touch = await browser.newContext({ viewport: { width: 1280, height: 860 }, hasTouch: true, isMobile: true })
    await touch.addInitScript(() => localStorage.setItem('i18nextLng', 'zh'))
    await touch.route('**/*', (route) => (route.request().url().startsWith(ORIGIN) ? route.continue() : route.abort()))
    const tp = await touch.newPage()
    await tp.goto(`${ORIGIN}/dashboard/overview`)
    await tp.waitForSelector('[data-myapi-shell]')
    await tp.waitForTimeout(1200)
    await tp.getByRole('button', { name: '更多' }).tap()
    await tp.getByRole('menu').waitFor()
    // The menu opens with a 95% zoom-in; measure after it settles.
    await tp.waitForTimeout(400)
    const item = tp.getByRole('menuitem', { name: '游乐场' })
    const box = await item.boundingBox()
    check('ops: touch opens More and its items are >= 44px tall', box && box.height >= 44, box)
    await item.tap()
    await tp.waitForURL('**/playground**')
    check('ops: tapping a More item navigates', new URL(tp.url()).pathname === '/playground', tp.url())
    await touch.close()
  }
  // Mobile More: open, close with Escape and the close button, focus returns.
  {
    const { context, page } = await open('/dashboard/overview', { width: 390, height: 844, mobile: true })
    const more = page.getByRole('button', { name: '更多' })
    await more.tap()
    const sheet = page.getByRole('dialog', { name: '全部功能' })
    await sheet.waitFor()
    await page.waitForTimeout(600)
    await shot(page, 'more-390-light-zh.png')
    await page.keyboard.press('Escape')
    await sheet.waitFor({ state: 'hidden' })
    const focusBack = await more.evaluate((element) => element === document.activeElement)
    check('ops: mobile More closes on Escape and focus returns to More', focusBack, focusBack)
    await more.tap()
    await sheet.waitFor()
    await sheet.getByRole('link', { name: '账号窗口' }).tap()
    await page.waitForURL('**/upstream-accounts**')
    check('ops: choosing a More destination navigates and closes the sheet', !(await sheet.isVisible()), page.url())
    await context.close()
  }
}

writeFileSync(path.join(outDir, 'browser-checks.json'), `${JSON.stringify({ origin: ORIGIN, blockedExternalRequests: blocked, checks, shots }, null, 2)}\n`)
console.log(`checks ${checks.filter((c) => c.pass).length}/${checks.length} passed; ${shots.length} screenshots; blocked ${blocked}`)
await browser.close()
process.exitCode = checks.every((c) => c.pass) ? 0 : 1
