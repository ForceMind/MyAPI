import { QueryClient } from '@tanstack/react-query'
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { Wallet } from '../../index'
import { CreemProductsSection } from '../creem-products-section'

const originalAuth = useAuthStore.getState()
const originalConfig = useSystemConfigStore.getState()
let client: QueryClient
let self: unknown
let quote: 'known' | 'failed' | 'zero'
let sequence = 0
function signIn(id = 1, sid = `wallet-current-${++sequence}`) {
  useAuthStore.setState((state) => ({ auth: { ...state.auth, user: { id, role: 1, username: `user-${id}` }, session: { sid, current: true, login_method: 'password', ip: '', user_agent: '', created_at: 1, last_active_at: 1, expires_at: 4000000000 } } }))
}
const capabilities = { mode: 'enabled', epoch: 1, ready: true, can_top_up: true, can_redeem: true, can_transfer_affiliate_rewards: true, can_purchase_subscription: false, can_view_funding_history: true }
beforeEach(() => {
  localStorage.clear()
  signIn()
  self = { id: 1, username: 'first', group: 'default', quota: 500000, used_quota: 250000, request_count: 12, aff_quota: 0, aff_history_quota: 0, aff_count: 0 }
  quote = 'known'
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/status') return { data: { success: true, data: { price: 1, user_funding_mode: 'enabled', user_funding_capabilities: capabilities } } }
    if (url === '/api/user/self') return { data: { success: true, data: self } }
    if (url === '/api/user/topup/info') return { data: { success: true, data: { user_funding_mode: 'enabled', user_funding_capabilities: capabilities, min_topup: 1, stripe_min_topup: 1, enable_online_topup: true, amount_options: [100], discount: {}, pay_methods: [{ name: 'Synthetic Pay', type: 'alipay', min_topup: 1 }] } } }
    if (url === '/api/user/aff') return { data: { success: true, data: 'synthetic-affiliate' } }
    if (url === '/api/subscription/self') return { data: { success: true, data: { subscriptions: [], all_subscriptions: [], billing_preference: 'subscription_first' } } }
    if (String(url).startsWith('/api/user/topup/self?')) return { data: { success: true, data: { items: [], total: 0 } } }
    throw new Error(`Unexpected wallet read ${url}`)
  })
  vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (url !== '/api/user/amount') throw new Error(`Unexpected wallet write ${url}`)
    return { data: { success: quote !== 'failed', data: quote === 'zero' ? '0' : '10' } }
  })
  vi.spyOn(window, 'open').mockReturnValue(null)
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); useSystemConfigStore.setState(originalConfig); localStorage.clear(); vi.restoreAllMocks() })
async function mount() { return renderListRoute({ client, path: '/wallet/', element: <Wallet /> }) }

it('an invalid actor snapshot is visibly retryable and a partial retry preserves each other known number', async () => {
  self = { id: 2, quota: 2000000 }
  await mount()
  expect(await screen.findByText('Wallet account could not be read. Please retry.')).toBeVisible()
  expect(screen.queryByRole('button', { name: 'Synthetic Pay' })).not.toBeInTheDocument()
  self = { id: 1, username: 'current', quota: 500000, request_count: 0 }
  await userEvent.setup().click(screen.getByRole('button', { name: 'Retry', exact: true }))
  await waitFor(() => expect(screen.queryByText('Wallet account could not be read. Please retry.')).not.toBeInTheDocument())
  expect(screen.getByText('Total Usage').nextElementSibling).toHaveTextContent(/^Unknown$/)
  expect(screen.getByText('Current Balance').nextElementSibling).not.toHaveTextContent('Unknown')
  expect(screen.getByText('API Requests').nextElementSibling).toHaveTextContent(/^0$/)
  expect(vi.mocked(api.get).mock.calls.filter(([url]) => String(url).startsWith('/api/user/topup/self?'))).toHaveLength(0)
})

it.each(['actor', 'SID'] as const)('a wallet %s change replaces private balances and drafts before a new account read completes', async (change) => {
  await mount()
  await screen.findByRole('button', { name: 'Synthetic Pay' })
  await waitFor(() => expect(screen.getByText('API Requests').nextElementSibling).toHaveTextContent('12'))
  let release!: () => void
  const gate = new Promise<void>((resolve) => { release = resolve })
  const original = vi.mocked(api.get).getMockImplementation()
  if (!original) throw new Error('Expected wallet fixture')
  vi.mocked(api.get).mockImplementation(async (url, config) => {
    if (url === '/api/user/self') { await gate; return { data: { success: true, data: { id: change === 'actor' ? 2 : 1, request_count: 3 } } } }
    return original(url, config)
  })
  act(() => signIn(change === 'actor' ? 2 : 1, 'wallet-next'))
  expect(screen.getByText('API Requests').nextElementSibling).not.toHaveTextContent(/^12$/)
  expect(screen.queryByRole('alertdialog', { name: 'Confirm Payment' })).not.toBeInTheDocument()
  await act(async () => release())
  await waitFor(() => expect(screen.getByText('API Requests').nextElementSibling).toHaveTextContent(/^3$/))
  expect(screen.getByText('Current Balance').nextElementSibling).toHaveTextContent(/^Unknown$/)
  expect(window.open).not.toHaveBeenCalled()
})

it('a failed quote cannot open payment confirmation or create an order while an explicit zero quote can be reviewed without paying', async () => {
  await mount()
  const user = userEvent.setup()
  const payment = await screen.findByRole('button', { name: 'Synthetic Pay' })
  await waitFor(() => expect(payment).toBeEnabled())
  quote = 'failed'
  await user.click(payment)
  expect(await screen.findByText('Payment quote could not be confirmed. Please try again.')).toBeVisible()
  expect(screen.queryByRole('alertdialog', { name: 'Confirm Payment' })).not.toBeInTheDocument()
  expect(vi.mocked(api.post).mock.calls.every(([url]) => url === '/api/user/amount')).toBe(true)
  quote = 'zero'
  await user.click(payment)
  const confirmation = await screen.findByRole('alertdialog', { name: 'Confirm Payment' })
  expect(within(confirmation).getByRole('button', { name: 'Confirm Payment' })).toBeEnabled()
  expect(within(confirmation).getByText('0', { exact: true })).toBeVisible()
  expect(vi.mocked(api.post).mock.calls.every(([url]) => url === '/api/user/amount')).toBe(true)
  expect(window.open).not.toHaveBeenCalled()
})

it('an unknown order locks payment buttons through close/reopen and a history read does not authorize another POST', async () => {
  vi.mocked(api.post).mockImplementation(async (url) => {
    if (url === '/api/user/amount') return { data: { success: true, data: '10' } }
    if (url === '/api/user/pay') throw new Error('Synthetic order response lost')
    throw new Error(`Unexpected new payment ${url}`)
  })
  await mount()
  const user = userEvent.setup()
  const method = await screen.findByRole('button', { name: 'Synthetic Pay' })
  await user.click(method)
  const first = await screen.findByRole('alertdialog', { name: 'Confirm Payment' })
  await user.click(within(first).getByRole('button', { name: 'Confirm Payment', exact: true }))
  expect(await within(first).findByText('Payment outcome is unconfirmed. New payments are locked; review billing history.')).toBeVisible()
  expect(within(first).getByRole('button', { name: 'Confirm Payment', exact: true })).toBeDisabled()
  const before = vi.mocked(api.post).mock.calls.length
  await user.click(within(first).getByRole('button', { name: 'Confirm Payment', exact: true }))
  await user.click(within(first).getByRole('button', { name: 'Billing History' }))
  const history = await screen.findByRole('dialog', { name: 'Billing History' })
  expect(history).toBeVisible()
  await waitFor(() => expect(vi.mocked(api.get).mock.calls.some(([url]) => String(url).startsWith('/api/user/topup/self?'))).toBe(true))
  await user.click(within(history).getByRole('button', { name: 'Close', exact: true }))
  expect(screen.getByRole('button', { name: 'Synthetic Pay' })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Synthetic Pay' }))
  expect(api.post).toHaveBeenCalledTimes(before)
  expect(vi.mocked(api.post).mock.calls.filter(([url]) => url === '/api/user/pay')).toHaveLength(1)
})


it('a Creem product remains keyboard selectable and becomes visibly disabled when new payments are locked', async () => {
  const product = { productId: 'precise-product', name: 'Complete Creem product name', price: 12.34, quota: 100, currency: 'USD' as const }
  const select = vi.fn()
  const user = userEvent.setup()
  const view = render(<CreemProductsSection products={[product]} onProductSelect={select} />)
  const action = screen.getByRole('button', { name: product.name })
  act(() => action.focus())
  await user.keyboard('{Enter}')
  expect(select).toHaveBeenCalledExactlyOnceWith(product)
  view.rerender(<CreemProductsSection products={[product]} onProductSelect={select} disabled />)
  expect(action).toHaveAttribute('aria-disabled', 'true')
  expect(action).toHaveAttribute('tabindex', '-1')
  await user.keyboard('{Enter}')
  await user.click(action)
  expect(select).toHaveBeenCalledOnce()
  expect(api.post).not.toHaveBeenCalled()
})
