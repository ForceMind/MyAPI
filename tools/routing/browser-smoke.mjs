// Real built application, isolated synthetic APIs only. Run in the existing
// GitHub Chromium job; this does not contact a provider or use real credentials.
import assert from 'node:assert/strict'
import { createReadStream, existsSync, mkdirSync, readFileSync, statSync } from 'node:fs'
import { createServer } from 'node:http'
import { extname, resolve, sep } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { quotaFixtures } from '../quota/browser-fixtures.mjs'

const repo = fileURLToPath(new URL('../../', import.meta.url))
const root = resolve(repo, 'web/dist')
const output = resolve(process.env.MYAPI_BROWSER_ARTIFACTS || `${repo}/.local-tests/routing-browser`)
const translations = JSON.parse(readFileSync(resolve(repo, 'web/src/i18n/locales/zh.json'), 'utf8')).translation
const label = (key) => translations[key] || key
const mime = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.png': 'image/png', '.woff2': 'font/woff2' }
assert(existsSync(resolve(root, 'index.html')), 'Build the frontend before browser regression')
const server = createServer((request, response) => {
  let path
  try { path = resolve(root, `.${decodeURIComponent(new URL(request.url || '/', 'http://localhost').pathname)}`) }
  catch { response.writeHead(400).end(); return }
  if (!path.startsWith(`${root}${sep}`) && path !== root) { response.writeHead(403).end(); return }
  if (!existsSync(path) || statSync(path).isDirectory()) path = resolve(root, 'index.html')
  response.writeHead(200, { 'content-type': mime[extname(path)] || 'application/octet-stream', 'cache-control': 'no-store' })
  createReadStream(path).pipe(response)
})
const driver = process.env.MYAPI_PLAYWRIGHT_MODULE
const { chromium } = await import(driver ? pathToFileURL(resolve(driver)).href : 'playwright')
const errors = []
const unexpected = new Set()
const submissions = []
let browser
let page
let refreshCount = 0
let channel = {
  ...quotaFixtures().response(new URL('http://localhost/api/channel/1')).data,
  routing_config_digest: 'synthetic-original-digest',
  name: 'Synthetic routing channel', type: 1, models: 'public-model',
  settings: JSON.stringify({ retained_setting: 'must-survive', model_routes: [{ public_model: 'public-model', upstream_model: 'upstream-original', endpoint: '/v1/responses', match: 'exact', priority: 0 }] }),
}
let discovery = { models: ['upstream-original'], source: 'openai_models', status: 'success', fetched_at: 1791000000, checked_at: 1791000000, stale: false }
try {
  await new Promise(done => server.listen(0, '127.0.0.1', done))
  const origin = `http://127.0.0.1:${server.address().port}`
  browser = await chromium.launch({ headless: true, executablePath: process.env.MYAPI_CHROMIUM_PATH || undefined, args: ['--disable-dev-shm-usage'] })
  const context = await browser.newContext({ viewport: { width: 1280, height: 720 }, locale: 'zh-CN', reducedMotion: 'reduce' })
  await context.addInitScript(() => { localStorage.setItem('i18nextLng', 'zhCN'); localStorage.setItem('theme', 'light') })
  await context.route('**/api/**', async route => {
    const url = new URL(route.request().url())
    const path = url.pathname.replace(/\/$/, '')
    let response
    if (path === '/api/channel/model-discovery/1') {
      if (route.request().method() === 'POST') {
        refreshCount++
        if (refreshCount === 1) discovery = { ...discovery, status: 'failed', stale: true, checked_at: 1791000100 }
        else if (refreshCount === 2) discovery = { ...discovery, models: [], status: 'empty', stale: false, fetched_at: 1791000200, checked_at: 1791000200 }
        else discovery = { ...discovery, models: ['upstream-refreshed'], status: 'success', stale: false, fetched_at: 1791000300, checked_at: 1791000300 }
      }
      response = { success: discovery.status !== 'failed', data: discovery }
    } else if (path === '/api/channel' && route.request().method() === 'PUT') {
      const body = route.request().postDataJSON()
      assert.equal(body.expected_routing_config, 'synthetic-original-digest', 'update carries the originally loaded configuration digest')
      assert.equal(body.models, 'public-model', 'routes do not enable upstream or wildcard models')
      assert.equal(JSON.parse(body.settings).retained_setting, 'must-survive')
      submissions.push(body)
      channel = { ...channel, ...body }
      response = { success: true, data: channel }
    } else if (path === '/api/channel/1') response = { success: true, data: channel }
    else if (path === '/api/channel' || path === '/api/channel/search') response = { success: true, data: { items: [channel], total: 1, page: 1, page_size: 10, type_counts: { 1: 1 } } }
    else if (path === '/api/channel/models' || path === '/api/user/models') response = { success: true, data: ['public-model'] }
    else if (path === '/api/channel/routing-preview') {
      assert.equal(url.searchParams.get('model'), 'public-model')
      assert.equal(url.searchParams.get('request_path'), '/v1/responses')
      response = { success: true, data: { group: 'default', model: 'public-model', request_path: '/v1/responses', source: 'database', generation: 1, data_generation: 1, published_generation: 1, cluster_committed_epoch: 1, local_published_epoch: 1, cache_enabled: false, cache_pending: false, affinity: { evaluated: false }, tiers: [{ priority: 0, fallback_index: 0, channels: [{ id: 1, name: channel.name, type: 1, weight: 1, effective_weight: 1, expected_share: 1, upstream_model: JSON.parse(channel.settings).model_routes[0].upstream_model, request_path: '/v1/responses', route_reason: 'explicit_exact', config_digest: 'synthetic-config-evidence' }] }] } }
    } else if (path === '/api/log' || path === '/api/log/self') {
      const fixture = quotaFixtures().response(url)
      const log = fixture.data.items[0]
      response = { success: true, data: { ...fixture.data, total: 1, items: [{ ...log, type: 5, model_name: 'public-model', content: 'Synthetic route evidence', other: JSON.stringify({ admin_info: { model_route: { requested_model: 'public-model', upstream_model: 'upstream-updated', endpoint: '/v1/responses', reason: 'explicit_exact', channel_id: 1, config_digest: 'synthetic-config-evidence' } } }) }] } }
    } else response = quotaFixtures().response(url)
    if (!response) unexpected.add(`${route.request().method()} ${path}`)
    await route.fulfill({ status: response ? 200 : 501, json: response || { success: false, message: 'Unconfigured routing fixture' } })
  })
  await context.route('**/*', route => {
    const url = new URL(route.request().url())
    return url.origin === origin || url.protocol === 'data:' ? route.fallback() : route.abort()
  })
  page = await context.newPage()
  page.setDefaultTimeout(15000)
  page.on('pageerror', error => errors.push(error.message))
  page.on('console', message => { if (message.type() === 'error') errors.push(message.text()) })
  mkdirSync(output, { recursive: true })
  async function settled() {
    await page.evaluate(async () => {
      await Promise.all(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished.catch(() => {})))
      await new Promise(done => requestAnimationFrame(() => requestAnimationFrame(done)))
    })
  }
  async function reach(locator) {
    await locator.waitFor({ state: 'visible' })
    for (let step = 0; step < 50; step++) {
      const box = await locator.boundingBox()
      const viewport = page.viewportSize()
      const reachable = await locator.evaluate(element => {
        const rect = element.getBoundingClientRect()
        const target = document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2)
        return target === element || element.contains(target)
      })
      if (reachable && box.y >= 65 && box.y + box.height <= viewport.height - 75) return
      await page.mouse.move(Math.min(viewport.width - 40, Math.max(40, box.x + box.width / 2)), viewport.height / 2)
      await page.mouse.wheel(0, box.y < 65 ? -260 : 260)
      await settled()
    }
    throw new Error(`Control unreachable through real scrolling: ${await locator.textContent()}`)
  }
  async function assertLayout(name) {
    await settled()
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), `${name}: no page-wide horizontal overflow`)
    await page.screenshot({ path: resolve(output, `${name}.png`), fullPage: true })
  }
  await page.goto(`${origin}/channels`, { waitUntil: 'networkidle' })
  const menu = page.getByRole('button', { name: label('Open menu'), exact: true }).last()
  await reach(menu)
  await menu.click()
  await page.getByRole('menuitem', { name: label('Edit'), exact: true }).click()
  const drawer = page.getByRole('dialog').filter({ hasText: label('Edit Channel') }).last()
  const evidence = drawer.getByRole('region', { name: label('Model discovery evidence'), exact: true })
  const refresh = evidence.getByRole('button', { name: label('Refresh model discovery'), exact: true })
  await reach(refresh)
  await refresh.click()
  await evidence.getByText(label('Stale discovery evidence'), { exact: true }).waitFor()
  assert((await evidence.innerText()).includes('upstream-original'), 'failed refresh retains previous authorized evidence')
  await assertLayout('discovery-failed-1280')
  await refresh.click()
  await evidence.getByText(label('Upstream returned no models'), { exact: true }).waitFor()
  assert.equal(await evidence.getByRole('list').count(), 0)
  await refresh.click()
  await evidence.getByText('upstream-refreshed', { exact: true }).waitFor()
  const routes = drawer.getByRole('region', { name: label('Explicit model routes'), exact: true })
  for (const width of [320, 1280]) {
    await page.setViewportSize({ width, height: 640 })
    const target = routes.getByLabel(label('Upstream model'), { exact: true })
    await reach(target)
    await target.fill('upstream-updated')
    await assertLayout(`model-routes-${width}x640`)
  }
  const add = routes.getByRole('button', { name: label('Add model route'), exact: true })
  await reach(add)
  await add.click()
  await routes.getByLabel(label('Public model'), { exact: true }).last().fill('public-model')
  await routes.getByLabel(label('Upstream model'), { exact: true }).last().fill('conflicting-target')
  await routes.getByLabel(label('Endpoint'), { exact: true }).last().selectOption('/v1/responses')
  const conflict = routes.getByRole('alert')
  await conflict.waitFor({ state: 'visible' })
  assert.equal(await conflict.innerText(), label('Conflicting model routes have the same match, endpoint, and priority.'), 'equal-specificity routes show the exact conflict message')
  await routes.getByRole('button', { name: label('Remove route'), exact: true }).last().click()
  await drawer.getByRole('button', { name: label('Update Channel'), exact: true }).click()
  await drawer.waitFor({ state: 'hidden' })
  assert.equal(submissions.length, 1, 'actual drawer saves one route update')
  for (const width of [320, 1280]) {
    await page.setViewportSize({ width, height: 640 })
    await page.goto(`${origin}/channels`, { waitUntil: 'networkidle' })
    await page.getByLabel(label('Routing preview model'), { exact: true }).fill('public-model')
    await page.getByLabel(label('Routing preview request path'), { exact: true }).fill('/v1/responses')
    await page.getByRole('button', { name: label('Preview routing'), exact: true }).click()
    const target = page.getByText(`${label('Upstream model')}: upstream-updated`, { exact: true })
    await reach(target)
    await assertLayout(`routing-preview-${width}x640`)
  }
  // Desktop cells and mobile cards own separate dialog state. Open details
  // through each layout's real entry point after resizing, rather than assuming
  // a cell-owned dialog survives that component being unmounted.
  for (const width of [320, 1280]) {
    await page.setViewportSize({ width, height: 640 })
    await page.goto(`${origin}/usage-logs/common`, { waitUntil: 'networkidle' })
    const openDetails = page.getByTitle(label('Click to view full details')).first()
    await reach(openDetails)
    await openDetails.click()
    const details = page.getByRole('dialog').filter({ hasText: label('Actual route evidence') })
    await details.waitFor({ state: 'visible' })
    await reach(details.getByText('synthetic-config-evidence', { exact: true }))
    for (const [field, value] of [
      ['Request Model', 'public-model'],
      ['Upstream model', 'upstream-updated'],
      ['Endpoint', '/v1/responses'],
      ['Route reason', 'explicit_exact'],
      ['Channel ID', '1'],
      ['Configuration digest', 'synthetic-config-evidence'],
    ]) {
      const evidenceRow = details.getByText(label(field), { exact: true }).locator('..')
      assert.equal(await evidenceRow.getByText(value, { exact: true }).count(), 1, `${field}: actual route evidence matches at ${width}px`)
    }
    await assertLayout(`actual-route-log-${width}x640`)
    await page.keyboard.press('Escape')
    await details.waitFor({ state: 'hidden' })
  }
  assert.deepEqual([...unexpected], [], 'every application endpoint has an explicit fixture')
  assert.deepEqual(errors, [], 'no browser runtime errors')
  console.log('Routing browser regression passed: discovery failure/empty/recovery, actual drawer save/conflict, saved preview, admin log, 320/1280px. Synthetic fixtures only.')
} catch (error) {
  if (page) {
    await page.screenshot({ path: resolve(output, 'failure.png'), fullPage: true }).catch(() => {})
    console.error('Routing browser diagnostics:', JSON.stringify({ errors, unexpected: [...unexpected], pageText: (await page.locator('body').innerText().catch(() => '')).slice(-7000) }))
  }
  throw error
} finally {
  if (browser) await browser.close()
  await new Promise(done => server.close(done))
}
