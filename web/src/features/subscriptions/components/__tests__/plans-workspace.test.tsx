import { QueryClient } from '@tanstack/react-query'
import { act, cleanup, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { SubscriptionsDialogs } from '../subscriptions-dialogs'
import { SubscriptionsProvider } from '../subscriptions-provider'
import { SubscriptionsTable } from '../subscriptions-table'

const originalAuth = useAuthStore.getState()
let client: QueryClient
let plans: Array<{ plan: Record<string, unknown> }>
let failRead: boolean

function plan(id: number, extra: Record<string, unknown> = {}) {
  return { plan: { id, title: `synthetic-plan-${String(id).padStart(3, '0')}`, subtitle: 'Complete plan subtitle', enabled: true, price_amount: 10, currency: 'USD', duration_unit: 'month', duration_value: 1, custom_seconds: 0, quota_reset_period: 'never', quota_reset_custom_seconds: 0, total_amount: 500000, purchase_limit: 0, sort_order: 7, upgrade_group: 'tier-b', wallet_overflow_enabled: true, stripe_price_id: 'synthetic-product', creem_product_id: '', waffo_pancake_product_id: '', ...extra } }
}

beforeEach(() => {
  localStorage.clear()
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, role: 100, username: 'synthetic-root' })
  plans = [plan(1)]
  failRead = false
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/subscription/admin/plans') {
      if (failRead) throw new Error('Synthetic plan read failed')
      return { data: { success: true, data: plans } }
    }
    if (url === '/api/option/') return { data: { success: true, data: [{ key: 'payment_setting.compliance_confirmed', value: 'true' }, { key: 'payment_setting.compliance_terms_version', value: 'v1' }] } }
    if (url === '/api/group/') return { data: { success: true, data: ['default', 'tier-b'] } }
    if (url === '/api/status') return { data: { success: true, data: { quota_per_unit: 500000, display_in_currency_enabled: true } } }
    throw new Error(`Unexpected GET ${url}`)
  })
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected plan write'))
  vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected plan write'))
  vi.spyOn(api, 'patch').mockRejectedValue(new Error('Unexpected plan write'))
  vi.spyOn(api, 'delete').mockRejectedValue(new Error('Unexpected plan write'))
})
afterEach(() => { cleanup(); client.clear(); localStorage.clear(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })

function phone() {
  const original = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({ ...original(query), matches: query === '(max-width: 1023px)' }))
}
async function renderPlans(dialogs = false) {
  return renderListRoute({ client, path: '/subscriptions/', element: <SubscriptionsProvider><SubscriptionsTable />{dialogs && <SubscriptionsDialogs />}</SubscriptionsProvider> })
}

it('exposes complete long plan text and every original payment or priority field on a phone', async () => {
  phone()
  const title = 'synthetic/plan-with-a-complete-extremely-long-name-and-a-final-suffix'
  plans = [plan(1, { title })]
  await renderPlans()
  const card = await screen.findByRole('article', { name: title })
  expect(within(card).getByText(title)).toBeVisible()
  const user = userEvent.setup()
  await user.click(within(card).getByText('More details'))
  expect(within(card).getByText('Payment Channel')).toBeVisible()
  expect(within(card).getByText('Stripe')).toBeVisible()
  expect(within(card).getByText('Priority')).toBeVisible()
  expect(within(card).getByText('7')).toBeVisible()
  expect(within(card).getByText('Upgrade Group')).toBeVisible()
  expect(api.post).not.toHaveBeenCalled()
})

it('clears stale plans and totals on a failed reread rather than presenting successful empty data', async () => {
  await renderPlans()
  await screen.findByText('synthetic-plan-001')
  failRead = true
  await act(async () => { await client.refetchQueries({ queryKey: ['admin-subscription-plans'] }) })
  expect(await screen.findByRole('alert')).toHaveTextContent('Failed to load subscription plans')
  expect(screen.queryByText('synthetic-plan-001')).not.toBeInTheDocument()
  expect(screen.queryByText('No subscription plans yet')).not.toBeInTheDocument()
  expect(screen.getByRole('navigation', { name: 'Subscription plans pagination' })).toHaveTextContent('Record total unavailable')
})

it('keeps explicit free or unlimited values distinct from unavailable price and quota fields', async () => {
  phone()
  plans = [plan(1, { price_amount: 0, total_amount: 0 }), plan(2, { price_amount: undefined, total_amount: undefined })]
  await renderPlans()
  const free = await screen.findByRole('article', { name: 'synthetic-plan-001' })
  const unknown = screen.getByRole('article', { name: 'synthetic-plan-002' })
  expect(within(free).getByText('$0.00')).toBeVisible()
  expect(within(free).getByText('Unlimited')).toBeVisible()
  expect(within(unknown).getAllByText('Not available')).toHaveLength(2)
  expect(within(unknown).queryByText('$0.00')).not.toBeInTheDocument()
  expect(within(unknown).queryByText('Unlimited')).not.toBeInTheDocument()
})

it('filters the returned plan set by name and status without dispatching a write or another list read', async () => {
  plans = [plan(1), plan(2, { enabled: false })]
  await renderPlans()
  await screen.findByText('synthetic-plan-001')
  const user = userEvent.setup()
  await user.type(screen.getByPlaceholderText('Search plans by name or ID...'), 'synthetic-plan-002')
  await waitFor(() => expect(screen.queryByText('synthetic-plan-001')).not.toBeInTheDocument())
  expect(screen.getByText('synthetic-plan-002')).toBeVisible()
  await user.clear(screen.getByPlaceholderText('Search plans by name or ID...'))
  await screen.findByText('synthetic-plan-001')
  await user.click(screen.getByRole('button', { name: 'Status' }))
  await user.click(screen.getByRole('menuitemcheckbox', { name: 'Disable' }))
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByText('synthetic-plan-001')).not.toBeInTheDocument())
  expect(screen.getByText('synthetic-plan-002')).toBeVisible()
  expect(vi.mocked(api.get).mock.calls.filter(([url]) => url === '/api/subscription/admin/plans')).toHaveLength(1)
  expect(api.patch).not.toHaveBeenCalled()
})

it('returns from the existing plan editor to the same filtered page and action without a write', async () => {
  phone()
  plans = Array.from({ length: 26 }, (_, index) => plan(index + 1))
  await renderPlans(true)
  await screen.findByText('synthetic-plan-001')
  const user = userEvent.setup()
  const search = screen.getByPlaceholderText('Search plans by name or ID...')
  await user.type(search, 'synthetic-plan')
  await user.click(screen.getByRole('button', { name: 'Go to page 2' }))
  const card = await screen.findByRole('article', { name: 'synthetic-plan-021' })
  const edit = within(card).getByRole('button', { name: 'Edit' })
  await waitFor(() => expect(edit).toBeEnabled())
  await user.click(edit)
  const editor = await screen.findByRole('dialog', { name: 'Update plan info' })
  await user.click(within(editor).getByRole('button', { name: 'Cancel' }))
  await waitFor(() => expect(edit).toHaveFocus())
  expect(search).toHaveValue('synthetic-plan')
  expect(screen.queryByText('synthetic-plan-001')).not.toBeInTheDocument()
  expect(screen.getByRole('article', { name: 'synthetic-plan-021' })).toBeVisible()
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
})
