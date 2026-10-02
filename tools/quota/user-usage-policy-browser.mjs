// Synthetic user-management policy workflow. No real users, keys or accounts.
import assert from 'node:assert/strict'
import { resolve } from 'node:path'

export function userUsagePolicyBrowserFixture() {
  const policy = { user_id: 1, no_balance: false, revision: 0, legacy_remaining_quota: 0 }
  const writes = []
  return {
    writes,
    async route(route, url) {
      const path = url.pathname.replace(/\/$/, '')
      if (path === '/api/user' || path === '/api/user/search') {
        await route.fulfill({ json: { success: true, data: { items: [{ id: 1, username: 'self-use-fixture', display_name: 'self-use-fixture', role: 1, status: 1, quota: 0, used_quota: 0, request_count: 0, group: 'default', account_tier_id: 'standard', self_use_no_balance: policy.no_balance, usage_policy_revision: policy.revision }], total: 1, page: 1, page_size: 10 } } })
        return true
      }
      if (path === '/api/group') {
        await route.fulfill({ json: { success: true, data: ['default'] } })
        return true
      }
      if (path === '/api/authz/catalog') {
        await route.fulfill({ json: { success: true, data: { resources: [], roles: [] } } })
        return true
      }
      if (path !== '/api/user/1/usage-policy') return false
      if (route.request().method() === 'PUT') {
        const body = route.request().postDataJSON()
        assert.equal(body.confirmed, true)
        assert.equal(body.expected_revision, policy.revision)
        assert.match(body.id, /^[a-f0-9]{64}$/)
        writes.push(body)
        policy.no_balance = body.no_balance
        policy.revision++
      } else assert.equal(route.request().method(), 'GET')
      await route.fulfill({ json: { success: true, data: policy } })
      return true
    },
  }
}

export async function checkUserUsagePolicyBrowser({ page, origin, output, label, fixture }) {
  const open = async () => {
    await page.goto(`${origin}/users`, { waitUntil: 'networkidle' })
    const row = page.getByRole('row').filter({ hasText: 'self-use-fixture' })
    await row.getByRole('button', { name: label('Open menu'), exact: true }).click()
    await page.getByRole('menuitem', { name: label('User usage policy'), exact: true }).click()
    const dialog = page.getByRole('dialog')
    await dialog.getByRole('button', { name: label('Save'), exact: true }).waitFor()
    return dialog
  }
  await page.setViewportSize({ width: 1280, height: 900 })
  let dialog = await open()
  await dialog.getByRole('button', { name: label('Save'), exact: true }).click()
  await dialog.getByText(label('Required'), { exact: true }).waitFor()
  assert.equal(fixture.writes.length, 0, 'unchecked confirmation cannot mutate policy')
  await dialog.getByRole('button', { name: label('Close'), exact: true }).click()
  assert.equal(fixture.writes.length, 0, 'closing the policy is read-only')
  dialog = await open()
  await dialog.getByRole('checkbox', { name: label('Use Key limits without a user wallet'), exact: true }).check()
  for (const width of [320, 1280]) {
    await page.setViewportSize({ width, height: 900 })
    await page.evaluate(async () => {
      await Promise.all(document.getAnimations().filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity).map((animation) => animation.finished.catch(() => {})))
    })
    assert(await dialog.getByRole('checkbox', { name: label('Use Key limits without a user wallet'), exact: true }).isChecked(), `unsaved policy survives the ${width}px table layout switch`)
    const size = await dialog.evaluate((element) => ({ width: element.getBoundingClientRect().width, scroll: element.scrollWidth, client: element.clientWidth }))
    assert(size.width <= width && size.scroll <= size.client + 1, `user policy fits ${width}px: ${JSON.stringify(size)}`)
    await dialog.screenshot({ path: resolve(output, `user-usage-policy-${width}.png`) })
  }
  await dialog.getByRole('checkbox', { name: label('I confirm all running instances support this policy and I understand the supported request paths.'), exact: true }).check()
  await Promise.all([
    page.waitForResponse((response) => response.url().endsWith('/api/user/1/usage-policy') && response.request().method() === 'PUT'),
    dialog.getByRole('button', { name: label('Save'), exact: true }).click(),
  ])
  assert.equal(fixture.writes.length, 1)
  assert.equal(fixture.writes[0].no_balance, true)
  await dialog.getByRole('button', { name: label('Close'), exact: true }).click()
  await page.getByText(label('No user allowance cap'), { exact: true }).waitFor()
  dialog = await open()
  assert(await dialog.getByRole('checkbox', { name: label('Use Key limits without a user wallet'), exact: true }).isChecked())
  assert.equal(fixture.writes.length, 1, 'reload never resubmits a user policy change')
}
