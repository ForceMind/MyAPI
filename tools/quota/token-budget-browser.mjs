// Built-UI synthetic regression only; no real keys, providers or budgets.
import assert from 'node:assert/strict'
import { resolve } from 'node:path'

export function tokenBudgetBrowserFixture() {
  const policy = { account_threshold_enabled: false, account_min_remaining_bps: 2000, account_max_age_seconds: 300, token_id: 1, user_id: 1, enabled: false, limit: 2000000, used: 0, reserved: 0, pending_request_id: '', revision: 0, fee_enabled: false, fee_limit_usd: '0', fee_used_usd: '0', fee_reserved_usd: '0' }
  const state = { policy, pending: null }
  const writes = []
  let sequence = 0
  return {
    writes,
    hold(chat = false) {
      const reserved = chat ? 1050000 : 30
      Object.assign(policy, { reserved, pending_request_id: `budget-browser-fixture-${++sequence}`, fee_reserved_usd: policy.fee_enabled ? '0.0003' : '0', revision: policy.revision + 1 })
      state.pending = { request_id: policy.pending_request_id, token_id: 1, user_id: 1, state: 'usage_unknown', reserved, model_name: chat ? 'gpt-6.1-sol' : 'fixture-model', ...(chat ? { bound_source: 'openai_chat_context_window', input_tokens_bound: 1050000, max_output_tokens: 128000 } : {}), fee_enabled: policy.fee_enabled, fee_reserved_usd: policy.fee_reserved_usd }
      state.review = { request_id: policy.pending_request_id, token_id: 1, user_id: 1, state: 'usage_unknown', reserved_quota: 100, actual_quota: null }
    },
    async route(route, url) {
      const path = url.pathname.replace(/\/$/, '')
      if (path === '/api/token') {
        await route.fulfill({ json: { success: true, data: { items: [{ id: 1, name: 'budget-browser-fixture', key: 'masked', status: 1, remain_quota: 1000, used_quota: 0, unlimited_quota: false, expired_time: -1, created_time: 1, accessed_time: 1, group: 'default', model_limits_enabled: false }], total: 1, page: 1, page_size: 10 } } })
        return true
      }
      if (path !== '/api/token/1/budget' && path !== '/api/token/1/budget/recover') return false
      if (route.request().method() !== 'GET') {
        const body = route.request().postDataJSON()
        writes.push(body)
        if (path.endsWith('/recover')) {
          assert.equal(body.action, 'reconcile')
          assert.equal(body.request_id, policy.pending_request_id)
          if (policy.fee_enabled) {
            assert.equal(body.actual_fee_usd, '0.00000000000000000000000036')
            policy.fee_used_usd = body.actual_fee_usd
          }
          assert.equal(body.confirmed_reliable_evidence, true)
          assert.equal(body.actual_input_tokens, 10)
          assert.equal(body.actual_output_tokens, 5)
          assert.equal(body.actual_quota, 20)
          Object.assign(policy, { used: policy.used + 15, reserved: 0, fee_reserved_usd: '0', pending_request_id: '', revision: policy.revision + 1 })
          state.pending = null
          delete state.review
        } else {
          assert.equal(body.confirmed, true)
          assert.equal(body.expected_revision, policy.revision)
          assert.match(body.id, /^[a-f0-9]{64}$/)
          Object.assign(policy, { account_threshold_enabled: body.account_threshold.enabled, account_min_remaining_bps: body.account_threshold.minimum_remaining_bps, account_max_age_seconds: body.account_threshold.max_age_seconds, enabled: body.enabled, limit: body.limit, fee_enabled: body.fee.enabled, fee_limit_usd: body.fee.limit_usd, revision: policy.revision + 1 })
        }
      }
      await route.fulfill({ json: { success: true, data: state } })
      return true
    },
  }
}

export async function checkTokenBudgetBrowser({ page, origin, output, label, fixture }) {
  const open = async () => {
    await page.goto(`${origin}/keys`, { waitUntil: 'networkidle' })
    await page.getByRole('button', { name: label('API Key usage budgets'), exact: true }).click()
    return page.getByRole('dialog')
  }
  await page.setViewportSize({ width: 1280, height: 900 })
  let dialog = await open()
  const qualification = [
    dialog.getByText(`${label('Official OpenAI Responses')}: max_output_tokens`, { exact: true }),
    dialog.getByText(`${label('Official OpenAI Chat')} · ${label('Exact Match')}: gpt-6.1-sol · max_completion_tokens`, { exact: true }),
    dialog.getByText(label('Chat reserves 1,050,000 total tokens for input and completion, including reasoning. This is a conservative bound, not measured usage or a tokenizer estimate. Even a small request can fail if the remaining budget cannot cover this bound.'), { exact: true }),
  ]
  for (const item of qualification) await item.waitFor({ state: 'visible' })
  for (const width of [320, 1280]) {
    await page.setViewportSize({ width, height: 900 })
    let geometry
    for (let step = 0; step < 20; step++) {
      await page.evaluate(async () => {
        await Promise.all(document.getAnimations().filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity).map((animation) => animation.finished.catch(() => {})))
        await new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done)))
      })
      geometry = await Promise.all(qualification.map((item) => item.evaluate((element) => {
        const rect = element.getBoundingClientRect()
        const dialog = element.closest('[role="dialog"]')
        if (!dialog) return { reachable: false }
        const clip = dialog.getBoundingClientRect()
        const left = Math.max(0, clip.left + dialog.clientLeft)
        const right = Math.min(innerWidth, clip.left + dialog.clientLeft + dialog.clientWidth)
        const top = Math.max(0, clip.top + dialog.clientTop)
        const bottom = Math.min(innerHeight, clip.top + dialog.clientTop + dialog.clientHeight)
        const fullyVisible = rect.width > 0 && rect.height > 0 && rect.left >= left && rect.right <= right && rect.top >= top && rect.bottom <= bottom
        const hits = [rect.top + Math.min(1, rect.height / 2), rect.y + rect.height / 2, rect.bottom - Math.min(1, rect.height / 2)].map((y) => {
          const target = document.elementFromPoint(rect.x + rect.width / 2, y)
          return target === element || element.contains(target)
        })
        return { x: rect.x, y: rect.y, right: rect.right, bottom: rect.bottom, fullyVisible, hits, reachable: fullyVisible && hits.every(Boolean) }
      })))
      if (geometry.every((item) => item.reachable)) break
      const bounds = await dialog.boundingBox()
      assert(bounds, 'budget qualification dialog has layout bounds')
      // Return to the top content with native wheel input if autofocus or a
      // responsive layout change left the dialog's own scrollport lower down.
      await page.mouse.move(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2)
      await page.mouse.wheel(0, -220)
    }
    assert(geometry.every((item) => item.reachable), `both protocol scopes and the conservative small-request warning are jointly unclipped and unoccluded at ${width}px: ${JSON.stringify(geometry)}`)
    await page.screenshot({ path: resolve(output, `token-budget-qualification-${width}.png`) })
  }
  assert.equal(fixture.writes.length, 0, 'qualification screenshots do not change budget policy')
  await dialog.getByText(label('Supported request fields'), { exact: true }).click()
  await dialog.getByText(label('Responses requires explicit max_output_tokens. Chat requires explicit max_completion_tokens from 1 to 128,000 and n omitted or 1; streaming requires stream_options.include_usage=true.'), { exact: true }).waitFor()
  await dialog.getByRole('button', { name: label('Save'), exact: true }).click()
  await dialog.getByText(label('Required'), { exact: true }).waitFor()
  assert.equal(fixture.writes.length, 0, 'opening or unchecked confirmation does not change a budget')
  await page.keyboard.press('Escape')
  assert.equal(fixture.writes.length, 0, 'cancelling the form does not write')
  dialog = await open()
  await dialog.getByRole('checkbox', { name: label('Enable strict Token budget'), exact: true }).check()
  await dialog.getByRole('checkbox', { name: label('I confirm all running instances support this budget and I understand its request restrictions.'), exact: true }).check()
  await Promise.all([
    page.waitForResponse((response) => response.url().endsWith('/api/token/1/budget') && response.request().method() === 'PUT'),
    dialog.getByRole('button', { name: label('Save'), exact: true }).click(),
  ])
  await dialog.getByRole('checkbox', { name: label('I confirm all running instances support this budget and I understand its request restrictions.'), exact: true }).waitFor()
  assert.equal(fixture.writes.length, 1)
  await page.keyboard.press('Escape')
  fixture.hold(true)
  dialog = await open()
  const boundPanel = dialog.getByRole('region', { name: label('Reservation and actual usage'), exact: true })
  await boundPanel.getByText('openai_chat_context_window', { exact: false }).waitFor()
  assert.equal(await boundPanel.getByText(label('Not confirmed'), { exact: true }).count(), 3, 'reservation is not rendered as actual input or output')
  await dialog.getByLabel(label('Confirmed quota (internal units)'), { exact: true }).fill('20')
  await dialog.getByLabel(label('Evidence reference'), { exact: true }).fill('verified synthetic terminal fixture')
  await dialog.getByRole('checkbox', { name: label('I verified that the request ended and the actual token counts and frozen pricing are correct.'), exact: true }).check()
  await dialog.getByRole('button', { name: label('Confirm reconciliation'), exact: true }).click()
  assert.equal(fixture.writes.length, 1, 'missing actual token counts cannot reconcile')
  for (const width of [320, 1280]) {
    await page.setViewportSize({ width, height: 900 })
    await page.evaluate(async () => {
      await Promise.all(document.getAnimations().filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity).map((animation) => animation.finished.catch(() => {})))
    })
    const size = await dialog.evaluate((element) => ({ width: element.getBoundingClientRect().width, scroll: element.scrollWidth, client: element.clientWidth }))
    assert(size.width <= width && size.scroll <= size.client + 1, `budget dialog fits ${width}px: ${JSON.stringify(size)}`)
    await dialog.screenshot({ path: resolve(output, `token-budget-${width}.png`) })
  }
  await dialog.getByLabel(label('Confirmed input tokens'), { exact: true }).fill('10')
  await dialog.getByLabel(label('Confirmed output tokens'), { exact: true }).fill('5')
  await dialog.getByRole('button', { name: label('Confirm reconciliation'), exact: true }).click()
  await dialog.getByRole('button', { name: label('Save'), exact: true }).waitFor()
  assert.equal(fixture.writes.length, 2)
  await page.reload({ waitUntil: 'networkidle' })
  await page.getByRole('button', { name: label('API Key usage budgets'), exact: true }).click()
  await page.getByRole('dialog').getByRole('button', { name: label('Save'), exact: true }).waitFor()
  assert.equal(fixture.writes.length, 2, 'reload never resubmits a recovery')
  dialog = page.getByRole('dialog')
  await dialog.getByRole('checkbox', { name: label('Enable strict Token budget'), exact: true }).uncheck()
  await dialog.getByRole('checkbox', { name: label('Enable USD fee budget'), exact: true }).check()
  await dialog.getByLabel(label('USD total limit'), { exact: true }).fill('0.001')
  await dialog.getByRole('checkbox', { name: label('I confirm all running instances support this budget and I understand its request restrictions.'), exact: true }).check()
  await Promise.all([
    page.waitForResponse((response) => response.url().endsWith('/api/token/1/budget') && response.request().method() === 'PUT'),
    dialog.getByRole('button', { name: label('Save'), exact: true }).click(),
  ])
  assert.equal(fixture.writes.length, 3)
  assert.equal(fixture.writes[2].enabled, false)
  assert.deepEqual(fixture.writes[2].fee, { enabled: true, limit_usd: '0.001' })
  fixture.hold()
  dialog = await open()
  await dialog.getByLabel(label('Confirmed quota (internal units)'), { exact: true }).fill('20')
  await dialog.getByLabel(label('Confirmed input tokens'), { exact: true }).fill('10')
  await dialog.getByLabel(label('Confirmed output tokens'), { exact: true }).fill('5')
  await dialog.getByLabel(label('Evidence reference'), { exact: true }).fill('verified synthetic USD fixture')
  await dialog.getByRole('checkbox', { name: label('I verified that the request ended and the actual token counts, USD cost and frozen pricing are correct.'), exact: true }).check()
  await dialog.getByRole('button', { name: label('Confirm reconciliation'), exact: true }).click()
  assert.equal(fixture.writes.length, 3, 'missing USD amount cannot release the reservation')
  for (const width of [320, 1280]) {
    await page.setViewportSize({ width, height: 900 })
    await page.evaluate(async () => {
      await Promise.all(document.getAnimations().filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity).map((animation) => animation.finished.catch(() => {})))
    })
    const size = await dialog.evaluate((element) => ({ width: element.getBoundingClientRect().width, scroll: element.scrollWidth, client: element.clientWidth }))
    assert(size.width <= width && size.scroll <= size.client + 1, `fee budget fits ${width}px: ${JSON.stringify(size)}`)
    await dialog.screenshot({ path: resolve(output, `fee-budget-${width}.png`) })
  }
  const tiny = '0.00000000000000000000000036'
  await dialog.getByLabel(label('Confirmed API usage cost (USD)'), { exact: true }).fill(tiny)
  await dialog.getByRole('button', { name: label('Confirm reconciliation'), exact: true }).click()
  await dialog.getByRole('button', { name: label('Save'), exact: true }).waitFor()
  assert.equal(fixture.writes.length, 4)
  await page.reload({ waitUntil: 'networkidle' })
  await page.getByRole('button', { name: label('API Key usage budgets'), exact: true }).click()
  await page.getByRole('dialog').getByText(tiny, { exact: true }).waitFor()
  assert.equal(fixture.writes.length, 4, 'fee recovery is not resubmitted by reload')
  dialog = page.getByRole('dialog')
  await dialog.getByRole('checkbox', { name: label('Enable account safety threshold'), exact: true }).check()
  await dialog.getByRole('checkbox', { name: label('I confirm all running instances support this budget and I understand its request restrictions.'), exact: true }).check()
  await dialog.getByRole('button', { name: label('Save'), exact: true }).click()
  await dialog.getByText(label('Account thresholds cannot be combined with Token or USD budgets on this key.'), { exact: true }).waitFor()
  assert.equal(fixture.writes.length, 4, 'mixed provider budget modes cannot save')
  await dialog.getByRole('checkbox', { name: label('Enable USD fee budget'), exact: true }).uncheck()
  await dialog.getByText(label('Account thresholds cannot be combined with Token or USD budgets on this key.'), { exact: true }).waitFor({ state: 'hidden' })
  await dialog.getByLabel(label('Minimum remaining percentage'), { exact: true }).fill('20.01')
  await dialog.getByLabel(label('Maximum observation age (seconds)'), { exact: true }).fill('120')
  for (const width of [320, 1280]) {
    await page.setViewportSize({ width, height: 900 })
    await page.evaluate(async () => {
      await Promise.all(document.getAnimations().filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity).map((animation) => animation.finished.catch(() => {})))
    })
    const size = await dialog.evaluate((element) => ({ width: element.getBoundingClientRect().width, scroll: element.scrollWidth, client: element.clientWidth }))
    assert(size.width <= width && size.scroll <= size.client + 1, `account threshold fits ${width}px: ${JSON.stringify(size)}`)
    const thresholdInput = dialog.getByLabel(label('Minimum remaining percentage'), { exact: true })
    // Exercise the dialog's real scrollport and center the control rather
    // than relying on programmatic edge alignment after a width change.
    for (let step = 0; step < 30; step++) {
      const input = await thresholdInput.boundingBox()
      const bounds = await dialog.boundingBox()
      assert(input && bounds, 'threshold input and dialog have layout bounds')
      if (input.y >= bounds.y + 1 && input.y + input.height <= bounds.y + bounds.height - 1) break
      await page.mouse.move(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2)
      const delta = input.y + input.height / 2 - bounds.y - bounds.height / 2
      await page.mouse.wheel(0, Math.max(-220, Math.min(220, delta)))
      await page.evaluate(() => new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done))))
    }
    const inputBox = await thresholdInput.boundingBox()
    const dialogBox = await dialog.boundingBox()
    assert(inputBox && dialogBox && inputBox.y >= dialogBox.y && inputBox.y + inputBox.height <= dialogBox.y + dialogBox.height, `threshold input is scroll-accessible at ${width}px: ${JSON.stringify({ inputBox, dialogBox })}`)
    assert(await thresholdInput.evaluate((input) => {
      const rect = input.getBoundingClientRect()
      return document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2) === input
    }), 'threshold input is not covered by another surface')
    await dialog.screenshot({ path: resolve(output, `account-threshold-${width}.png`) })
  }
  async function reachBudgetFooter() {
    for (let step = 0; step < 40; step++) {
      const remaining = await dialog.evaluate((element) => element.scrollHeight - element.clientHeight - element.scrollTop)
      if (remaining <= 1) break
      const bounds = await dialog.boundingBox()
      assert(bounds, 'budget dialog has layout bounds')
      await page.mouse.move(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2)
      await page.mouse.wheel(0, Math.min(400, remaining))
      await page.evaluate(() => new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done))))
    }
    assert(await dialog.evaluate((element) => element.scrollHeight - element.clientHeight - element.scrollTop <= 1), 'native wheel reaches the bottom of the budget dialog')
    // The text Close precedes the corner icon Close in the popup DOM.
    const controls = [dialog.getByRole('button', { name: label('Save'), exact: true }), dialog.getByRole('button', { name: label('Close'), exact: true }).first()]
    const geometry = []
    for (const control of controls) {
      const state = await control.evaluate((element) => {
        const rect = element.getBoundingClientRect()
        const popup = element.closest('[role="dialog"]')
        const clip = popup.getBoundingClientRect()
        const left = Math.max(0, clip.left + popup.clientLeft)
        const right = Math.min(innerWidth, clip.left + popup.clientLeft + popup.clientWidth)
        const top = Math.max(0, clip.top + popup.clientTop)
        const bottom = Math.min(innerHeight, clip.top + popup.clientTop + popup.clientHeight)
        const points = [rect.top + 1, rect.y + rect.height / 2, rect.bottom - 1]
        const hit = points.every((y) => {
          const target = document.elementFromPoint(rect.x + rect.width / 2, y)
          return target === element || element.contains(target)
        })
        const style = getComputedStyle(popup)
        const children = [...popup.children].map((child) => {
          const box = child.getBoundingClientRect()
          return { tag: child.tagName, text: child.textContent?.slice(0, 40), top: box.top, bottom: box.bottom, height: box.height }
        })
        return { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2, rect: { top: rect.top, bottom: rect.bottom, width: rect.width, height: rect.height }, clip: { left, right, top, bottom }, scroll: { top: popup.scrollTop, height: popup.scrollHeight, client: popup.clientHeight }, layout: { display: style.display, rows: style.gridTemplateRows, overflow: style.overflowY, padding: style.padding, active: document.activeElement?.getAttribute('id'), viewport: [innerWidth, innerHeight], children }, hit, reachable: rect.width > 0 && rect.height > 0 && rect.left >= left && rect.right <= right && rect.top >= top && rect.bottom <= bottom && hit }
      })
      assert(state.reachable, `budget footer control is fully visible and unoccluded: ${JSON.stringify(state)}`)
      geometry.push(state)
    }
    return geometry
  }
  let footer
  for (const height of [900, 640]) {
    await page.setViewportSize({ width: 320, height })
    await page.evaluate(async () => {
      await new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done)))
      await Promise.all(document.getAnimations().filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity).map((animation) => animation.finished.catch(() => {})))
      await new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done)))
    })
    footer = await reachBudgetFooter()
    await page.screenshot({ path: resolve(output, `budget-footer-320x${height}.png`) })
  }
  await Promise.all([
    page.waitForResponse((response) => response.url().endsWith('/api/token/1/budget') && response.request().method() === 'PUT'),
    page.mouse.click(footer[0].x, footer[0].y),
  ])
  assert.equal(fixture.writes.length, 5)
  assert.deepEqual(fixture.writes[4].account_threshold, { enabled: true, minimum_remaining_bps: 2001, max_age_seconds: 120 })
  await dialog.getByText('20.01%', { exact: true }).waitFor()
  footer = await reachBudgetFooter()
  await page.mouse.click(footer[1].x, footer[1].y)
  await dialog.waitFor({ state: 'hidden' })
  assert.equal(fixture.writes.length, 5, 'closing through the mobile footer does not resubmit the budget')
  await page.reload({ waitUntil: 'networkidle' })
  await page.getByRole('button', { name: label('API Key usage budgets'), exact: true }).click()
  await page.getByRole('dialog').getByText('20.01%', { exact: true }).waitFor()
  assert.equal(fixture.writes.length, 5, 'threshold reload never rewrites policy')


}
