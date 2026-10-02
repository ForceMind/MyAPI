// Built-UI synthetic regression only; no real keys, providers or budgets.
import assert from 'node:assert/strict'
import { resolve } from 'node:path'

export function tokenBudgetBrowserFixture() {
  const policy = { token_id: 1, user_id: 1, enabled: false, limit: 100, used: 0, reserved: 0, pending_request_id: '', revision: 0, fee_enabled: false, fee_limit_usd: '0', fee_used_usd: '0', fee_reserved_usd: '0' }
  const state = { policy, pending: null }
  const writes = []
  let sequence = 0
  return {
    writes,
    hold() {
      Object.assign(policy, { reserved: 30, pending_request_id: `budget-browser-fixture-${++sequence}`, fee_reserved_usd: policy.fee_enabled ? '0.0003' : '0', revision: policy.revision + 1 })
      state.pending = { request_id: policy.pending_request_id, token_id: 1, user_id: 1, state: 'usage_unknown', reserved: 30, model_name: 'fixture-model', fee_enabled: policy.fee_enabled, fee_reserved_usd: policy.fee_reserved_usd }
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
          Object.assign(policy, { enabled: body.enabled, limit: body.limit, fee_enabled: body.fee.enabled, fee_limit_usd: body.fee.limit_usd, revision: policy.revision + 1 })
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
  fixture.hold()
  dialog = await open()
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

}
