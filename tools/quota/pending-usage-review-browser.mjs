// Synthetic crashed-request discovery and manual recovery, never a real bill.
import assert from 'node:assert/strict'
import { resolve } from 'node:path'

export function pendingUsageReviewBrowserFixture() {
  const item = { request_id: 'crashed-request-without-log-'.repeat(2), user_id: 1, token_id: 2, state: 'open', reserved_quota: 100, actual_quota: null, text_dispatch_pending: true, can_recover_text_dispatch: true, review_metadata: '{"version":1,"text_dispatch_pending":true,"model":"synthetic","quota_unit":500000}' }
  const writes = []
  return {
    item, writes,
    async route(route, url) {
      const path = url.pathname.replace(/\/$/, '')
      if (path === '/api/usage-reviews/pending') {
        assert.equal(route.request().method(), 'GET')
        const first = url.searchParams.get('after') === '0'
        const pending = item.actual_quota === null
        await route.fulfill({ json: { success: true, data: { items: !first && pending ? [item] : [], next_after: first && pending ? '100' : '' } } })
        return true
      }
      if (!path.startsWith(`/api/usage-review/${item.request_id}`)) return false
      if (route.request().method() === 'POST') {
        assert.equal(path, `/api/usage-review/${item.request_id}/recover-dispatch`)
        const body = route.request().postDataJSON()
        assert.deepEqual(body, { actual_quota: 20, evidence_reference: 'synthetic completed request proof', confirmed_reliable_evidence: true, confirmed_request_finished: true })
        writes.push(body)
        item.actual_quota = 20
        item.state = 'applied'
        item.can_recover_text_dispatch = false
      }
      await route.fulfill({ json: { success: true, data: item } })
      return true
    },
  }
}

export async function checkPendingUsageReviewBrowser({ page, origin, output, label, fixture }) {
  const open = async () => {
    await page.goto(`${origin}/usage-logs/common`, { waitUntil: 'networkidle' })
    await page.getByRole('button', { name: label('Pending requests'), exact: true }).click()
    const dialog = page.getByRole('dialog')
    await dialog.getByText(label('No requests on this page'), { exact: true }).waitFor()
    await dialog.getByRole('button', { name: label('Next'), exact: true }).click()
    await dialog.getByRole('button', { name: `${label('Request ID')}: ${fixture.item.request_id}`, exact: true }).click()
    await dialog.getByLabel(label('Confirmed quota (internal units)'), { exact: true }).waitFor()
    return dialog
  }
  await page.setViewportSize({ width: 1280, height: 900 })
  let dialog = await open()
  await dialog.getByRole('button', { name: label('Close'), exact: true }).first().click()
  assert.equal(fixture.writes.length, 0, 'opening and closing an unlogged request never charges it')
  dialog = await open()
  await dialog.getByLabel(label('Confirmed quota (internal units)'), { exact: true }).fill('20')
  await dialog.getByLabel(label('Evidence reference'), { exact: true }).fill('synthetic completed request proof')
  await dialog.getByRole('button', { name: label('Confirm reconciliation'), exact: true }).click()
  await dialog.getByText(label('Required'), { exact: true }).waitFor()
  assert.equal(fixture.writes.length, 0, 'a finished-request confirmation is mandatory')
  for (const width of [320, 1280]) {
    await page.setViewportSize({ width, height: 900 })
    await page.evaluate(async () => { await Promise.all(document.getAnimations().filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity).map((animation) => animation.finished.catch(() => {}))) })
    assert.equal(await dialog.getByLabel(label('Confirmed quota (internal units)'), { exact: true }).inputValue(), '20')
    const size = await dialog.evaluate((element) => ({ width: element.getBoundingClientRect().width, scroll: element.scrollWidth, client: element.clientWidth }))
    assert(size.width <= width && size.scroll <= size.client + 1, `pending request recovery fits ${width}px: ${JSON.stringify(size)}`)
    await dialog.screenshot({ path: resolve(output, `pending-usage-review-${width}.png`) })
  }
  await dialog.getByRole('checkbox', { name: label('I verified that the request ended and the actual usage and frozen pricing are correct.'), exact: true }).check()
  await dialog.getByRole('button', { name: label('Confirm reconciliation'), exact: true }).click()
  await dialog.getByText(label('Reconciled'), { exact: true }).waitFor()
  assert.equal(fixture.writes.length, 1)
  await dialog.getByRole('button', { name: label('Close'), exact: true }).first().click()
  await page.reload({ waitUntil: 'networkidle' })
  await page.getByRole('button', { name: label('Pending requests'), exact: true }).click()
  dialog = page.getByRole('dialog')
  await dialog.getByText(label('No requests on this page'), { exact: true }).waitFor()
  assert(await dialog.getByRole('button', { name: label('Next'), exact: true }).isDisabled())
  assert.equal(fixture.writes.length, 1, 'refresh does not repeat manual settlement')
}
