// Real built UI with isolated synthetic APIs. No provider, credentials or
// external notification endpoint is contacted by this journey.
import assert from 'node:assert/strict'
import { createReadStream, existsSync, mkdirSync, readFileSync, statSync } from 'node:fs'
import { createServer } from 'node:http'
import { extname, resolve, sep } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { quotaFixtures } from '../quota/browser-fixtures.mjs'
import { assertTextContrast } from '../ui/contrast.mjs'

const repo = fileURLToPath(new URL('../../', import.meta.url))
const root = resolve(repo, 'web/dist')
const output = resolve(process.env.MYAPI_BROWSER_ARTIFACTS || `${repo}/.local-tests/access-browser`)
const translations = JSON.parse(readFileSync(resolve(repo, 'web/src/i18n/locales/zh.json'), 'utf8')).translation
const label = key => translations[key] || key
const mime = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml', '.png': 'image/png', '.woff2': 'font/woff2' }
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
const userPolicy = { subject: 'user', subject_id: 2, owner_user_id: 2, revision: 0, assigned: false, enabled: true, public_models: null, upstream_models: null, channel_ids: null }
const keyPolicy = { ...userPolicy, subject: 'token', subject_id: 21 }
const writes = [], previews = [], unexpected = [], errors = []
let ordinary = false, conflict = false, eventError = false, browser
const decisions = candidate => [
  { model: 'public-allowed', allowed: Boolean(candidate.enabled && (candidate.public_models == null || candidate.public_models.includes('public-allowed')) && (candidate.channel_ids == null || candidate.channel_ids.includes(1))), reasons: !candidate.enabled ? ['policy_disabled'] : candidate.channel_ids?.length === 0 ? ['channel_denied'] : [] },
  { model: 'public-denied', allowed: false, reasons: [candidate.enabled ? 'channel_denied' : 'policy_disabled'] },
]
const eventRows = [
  ['recovery', 'healthy', 80], ['threshold', 'exhausted', 0], ['threshold', 'warning', 15],
].map(([kind, status, remaining], i) => ({ id: 3-i, event_key: `synthetic-event-${3-i}`, series_ref: 'synthetic-account-window-a', snapshot_id: 3-i, channel_id: 1, kind, status, state: 'pending', attempt_count: 0, observed_at: 1791000100-i, created_at: 1791000100-i, updated_at: 1791000100-i, evidence: { version: 1, snapshot_id: 3-i, channel_id: 1, observed_at: 1791000100-i, available: remaining, total: 100, unit: 'percent', metric_type: 'codex_rate_limit', window_type: 'five_hour', window_seconds: 18000, reset_at: 1791018000 } }))
try {
  await new Promise(done => server.listen(0, '127.0.0.1', done))
  const origin = `http://127.0.0.1:${server.address().port}`
  browser = await chromium.launch({ headless: true, executablePath: process.env.MYAPI_CHROMIUM_PATH || undefined, args: ['--disable-dev-shm-usage'] })
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, locale: 'zh-CN', reducedMotion: 'reduce' })
  await context.addInitScript(() => { localStorage.setItem('i18nextLng', 'zhCN'); localStorage.setItem('theme', 'light') })
  await context.route('**/api/**', async route => {
    const url = new URL(route.request().url()), path = url.pathname.replace(/\/$/, ''), method = route.request().method()
    const ok = data => ({ success: true, data })
    let response, status = 200
    if (path === '/api/user' || path === '/api/user/search') response = ok({ items: [{ id: 2, username: 'policy-user', display_name: 'policy-user', role: 1, status: 1, quota: 0, used_quota: 0, request_count: 0, group: 'default', account_tier_id: 'standard' }], total: 1, page: 1, page_size: 10 })
    else if (path === '/api/group') response = ok(['default'])
    else if (path === '/api/authz/catalog') response = ok({ resources: [], roles: [] })
    else if (path === '/api/token' || path === '/api/token/search') response = ok({ items: [{ id: 21, user_id: 2, name: 'policy-key', key: 'masked', status: 1, remain_quota: 1000, used_quota: 0, unlimited_quota: false, expired_time: -1, created_time: 1, accessed_time: 1, group: 'default', model_limits_enabled: false }], total: 1, page: 1, page_size: 10 })
    else if (path === '/api/token/21/key') { assert.equal(method, 'POST'); response = ok({ key: 'synthetic-browser-key-not-a-credential' }) }
    else if (path === '/api/token/21/access') response = ok({ token_id: 21, assigned: true, user_revision: userPolicy.revision, token_revision: keyPolicy.revision, models: decisions({ enabled: userPolicy.enabled && keyPolicy.enabled, public_models: userPolicy.public_models, channel_ids: keyPolicy.channel_ids }) })
    else if (path.startsWith('/api/access-policy/')) {
      assert(!ordinary, 'ordinary users never request administrative policy metadata')
      const target = path.includes('/token/') ? keyPolicy : userPolicy
      if (path.endsWith('/preview')) {
        assert.equal(method, 'POST'); const candidate = route.request().postDataJSON(); previews.push(candidate)
        response = ok({ policy: { ...target, ...candidate, assigned: true }, models: decisions(candidate) })
      } else if (method === 'GET') response = ok(target)
      else {
        const body = route.request().postDataJSON(); assert.equal(body.expected_revision, target.revision)
        if (conflict) { status = 409; response = { success: false, code: 'access_policy_conflict' }; conflict = false }
        else {
          writes.push({ method, target: target.subject, body })
          if (method === 'DELETE') Object.assign(target, { assigned: false, enabled: true, public_models: null, upstream_models: null, channel_ids: null, revision: target.revision+1 })
          else Object.assign(target, body, { assigned: true, revision: target.revision+1 })
          response = ok(target)
        }
      }
    } else if (path === '/api/channel/quota/events') {
      assert(!ordinary, 'ordinary users never request private account quota events')
      if (eventError) { status = 503; response = { success: false } }
      else { const rows = eventRows.filter(row => !url.searchParams.get('status') || row.status === url.searchParams.get('status')); response = ok({ items: rows, total: rows.length, page: 1, page_size: 10 }) }
    } else {
      if (ordinary) process.env.MYAPI_BROWSER_ORDINARY = '1'
      else delete process.env.MYAPI_BROWSER_ORDINARY
      response = quotaFixtures().response(url)
    }
    if (!response) unexpected.push(`${method} ${path}`)
    await route.fulfill({ status: response ? status : 501, json: response || { success: false } })
  })
  await context.route('**/*', route => {
    const url = new URL(route.request().url())
    return url.origin === origin || url.protocol === 'data:' ? route.fallback() : route.abort()
  })
  const page = await context.newPage(); page.setDefaultTimeout(15000); page.on('pageerror', error => errors.push(error.message)); mkdirSync(output, { recursive: true })
  page.on('console', message => {
    if (message.type() === 'log' && message.text().includes('AxiosError')) errors.push('Raw HTTP error reached the console')
  })
  const openUser = async () => {
    await page.setViewportSize({ width: 1280, height: 900 }); await page.goto(`${origin}/users`, { waitUntil: 'networkidle' })
    await page.getByRole('row').filter({ hasText: 'policy-user' }).getByRole('button', { name: label('Open menu'), exact: true }).click()
    await page.getByRole('menuitem', { name: label('Assigned access'), exact: true }).click()
    const dialog = page.getByRole('dialog'); await dialog.getByRole('button', { name: label('Save assignment'), exact: true }).waitFor(); return dialog
  }
  const screenshot = async (name, width, locator) => {
    await page.setViewportSize({ width, height: 900 })
    await page.evaluate(async () => { await Promise.all(document.getAnimations().filter(a => a.effect?.getComputedTiming().iterations !== Infinity).map(a => a.finished.catch(() => {}))) })
    const geometry = await locator.evaluate(el => ({ width: el.getBoundingClientRect().width, scroll: el.scrollWidth, client: el.clientWidth }))
    assert(geometry.width <= width && geometry.scroll <= geometry.client+1, `${name} fits ${width}: ${JSON.stringify(geometry)}`)
    await page.screenshot({ path: resolve(output, `${name}-${width}.png`) })
  }
  let dialog = await openUser()
  const publicScope = dialog.getByRole('group', { name: label('Public model scope'), exact: true })
  await publicScope.getByRole('checkbox', { name: label('Inherit existing scope'), exact: true }).uncheck()
  await publicScope.getByRole('textbox').fill('public-allowed')
  await dialog.getByRole('button', { name: label('Preview access'), exact: true }).click()
  await dialog.getByText(label('Preview only. No changes have been saved and no access has been granted.'), { exact: true }).waitFor()
  assert.equal(writes.length, 0); assert.equal(previews.length, 1)
  for (const width of [320, 1280]) await screenshot('assigned-user-policy', width, dialog)
  await dialog.getByRole('button', { name: label('Close'), exact: true }).click()
  dialog = await openUser(); assert.equal(writes.length, 0, 'closing preview does not save')
  await dialog.getByRole('group', { name: label('Public model scope'), exact: true }).getByRole('checkbox').uncheck()
  await dialog.getByRole('group', { name: label('Public model scope'), exact: true }).getByRole('textbox').fill('public-allowed')
  await dialog.getByRole('button', { name: label('Save assignment'), exact: true }).click(); await dialog.getByText(label('Assignment saved.'), { exact: true }).waitFor()
  assert.equal(writes.length, 1); assert.deepEqual(writes[0].body.public_models, ['public-allowed'])
  await dialog.getByRole('combobox', { name: label('Assignment target'), exact: true }).selectOption('token')
  await dialog.getByRole('textbox', { name: label('Key ID'), exact: true }).fill('21')
  await dialog.getByRole('button', { name: label('Load Key policy'), exact: true }).click()
  await dialog.getByRole('button', { name: label('Save assignment'), exact: true }).waitFor()
  await dialog.getByRole('group', { name: label('Channel ID scope'), exact: true }).getByRole('checkbox').uncheck()
  await dialog.getByRole('button', { name: label('Save assignment'), exact: true }).click(); await dialog.getByText(label('Assignment saved.'), { exact: true }).waitFor()
  assert.deepEqual(writes[1].body.channel_ids, [], 'explicit empty list remains deny-all')
  conflict = true
  await dialog.getByRole('button', { name: label('Save assignment'), exact: true }).click()
  await dialog.getByRole('alert').waitFor(); assert.equal(writes.length, 2, 'conflict cannot overwrite newer assignment')
  assert.equal(await page.locator('[data-sonner-toast][data-type="error"]').count(), 0, 'policy conflicts use the inline error without a duplicate global toast')
  await dialog.getByRole('button', { name: label('Refresh policy'), exact: true }).click()
  await dialog.getByRole('group', { name: label('Channel ID scope'), exact: true }).getByRole('textbox').fill('1')
  await dialog.getByRole('checkbox', { name: label('Enable assigned access'), exact: true }).uncheck()
  await dialog.getByRole('button', { name: label('Save assignment'), exact: true }).click(); await dialog.getByText(label('Assignment saved.'), { exact: true }).waitFor()
  assert.equal(writes[2].body.enabled, false)
  for (const width of [320, 1280]) await screenshot('assigned-key-revoked', width, dialog)
  await dialog.getByRole('button', { name: label('Close'), exact: true }).click()
  await page.goto(`${origin}/channels`, { waitUntil: 'networkidle' })
  await page.getByRole('button', { name: label('Show events'), exact: true }).click()
  const events = page.locator('#quota-events-panel'); await events.getByText(label('Quota recovered'), { exact: true }).last().waitFor()
  for (const width of [320, 1280]) { await page.setViewportSize({ width, height: 900 }); await events.scrollIntoViewIfNeeded(); await screenshot('account-window-events', width, events) }
  await events.getByRole('combobox').selectOption('exhausted')
  await events.getByText(label('Quota exhausted'), { exact: true }).last().waitFor()
  assert.equal(await events.getByRole('listitem').count(), 1)
  eventError = true; await events.getByRole('button', { name: label('Refresh'), exact: true }).click(); await events.getByRole('alert').waitFor()
  assert.equal(await events.getByRole('listitem').count(), 0, 'error removes stale account rows')
  eventError = false
  ordinary = true; await page.setViewportSize({ width: 1280, height: 900 }); await page.goto(`${origin}/keys`, { waitUntil: 'networkidle' })
  await page.getByRole('button', { name: label('Open menu'), exact: true }).click(); await page.getByRole('menuitem', { name: label('Assigned access'), exact: true }).click()
  dialog = page.getByRole('dialog'); await dialog.getByText('public-allowed', { exact: true }).waitFor()
  assert.equal(await dialog.getByRole('button', { name: label('Save assignment'), exact: true }).count(), 0)
  assert.equal(await dialog.getByText(label('Channel ID scope'), { exact: true }).count(), 0)
  assert.equal(await dialog.getByText(label('Available'), { exact: true }).count(), 0, 'disabled assignment remains denied in owner view')
  for (const width of [320, 1280]) await screenshot('owner-access-revoked', width, dialog)
  keyPolicy.enabled = true; keyPolicy.revision++ // Simulate another administrator restoring the saved scope.
  await dialog.getByRole('button', { name: label('Refresh'), exact: true }).click()
  await dialog.getByText(label('Available'), { exact: true }).waitFor()
  await assertTextContrast(
    dialog.getByText(label('Available'), { exact: true }),
    'restored owner Available badge',
    await page.locator('html').evaluate(element => element.classList.contains('dark') ? 'dark' : 'light'),
  )
  for (const width of [320, 1280]) await screenshot('owner-model-access', width, dialog)
  assert.equal(writes.length, 3); assert.equal(unexpected.length, 0, `unexpected API calls: ${unexpected.join(', ')}`); assert.deepEqual(errors, [])
  console.log('Assigned user/Key preview, write, conflict, revoke, owner privacy and in-app account-window events passed at 320/1280')
} finally {
  delete process.env.MYAPI_BROWSER_ORDINARY
  if (browser) await browser.close()
  await new Promise(done => server.close(done))
}
