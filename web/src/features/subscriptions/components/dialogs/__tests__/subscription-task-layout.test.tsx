import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { subscriptionPlanSchema } from '../../../types'
import { SubscriptionsProvider, useSubscriptions } from '../../subscriptions-provider'
import { ResetSubscriptionsDialog } from '../reset-subscriptions-dialog'
import { SubscriptionPurchaseDialog } from '../subscription-purchase-dialog'
import { ToggleStatusDialog } from '../toggle-status-dialog'

const originalAuth = useAuthStore.getState()
const plan = subscriptionPlanSchema.parse({ id: 31, title: 'Synthetic subscription plan with a complete long title and final suffix', price_amount: 12.34, currency: 'USD', duration_unit: 'month', duration_value: 1, quota_reset_period: 'never', enabled: true, sort_order: 0, allow_balance_pay: false, allow_wallet_overflow: true, max_purchase_per_user: 1, total_amount: 500000, stripe_price_id: 'synthetic-stripe-price', creem_product_id: 'synthetic-creem-product', waffo_pancake_product_id: 'synthetic-waffo-product' })
let client: QueryClient
beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, role: 100, username: 'synthetic-root' })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url !== '/api/option/') throw new Error(`Unexpected synthetic GET ${url}`)
    return { data: { success: true, data: [{ key: 'payment_setting.compliance_confirmed', value: 'true' }, { key: 'payment_setting.compliance_terms_version', value: 'v1' }] } }
  })
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected synthetic subscription write'))
  vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected synthetic subscription write'))
  vi.spyOn(api, 'patch').mockRejectedValue(new Error('Unexpected synthetic subscription write'))
  vi.spyOn(window, 'open').mockReturnValue(null)
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })

function Confirmations() {
  const { setOpen, setCurrentRow } = useSubscriptions()
  return <><button type='button' onClick={() => { setCurrentRow({ plan }); setOpen('toggle-status') }}>Open status</button><button type='button' onClick={() => { setCurrentRow({ plan }); setOpen('reset-subscriptions') }}>Open reset</button><ToggleStatusDialog /><ResetSubscriptionsDialog /></>
}

it('subscription purchase keeps the complete plan name and all original purchase-limit locks outside its one details scroll region', () => {
  render(<SubscriptionPurchaseDialog open onOpenChange={() => {}} plan={{ plan }} enableStripe enableCreem enableWaffoPancake enableOnlineTopUp epayMethods={[{ type: 'synthetic-epay', name: 'Synthetic payment channel with a complete long name' }]} purchaseLimit={1} purchaseCount={1} userQuota={1000000} />)
  const body = screen.getByRole('region', { name: 'Subscription purchase details' })
  expect(within(body).getByText(plan.title)).not.toHaveClass('truncate')
  expect(within(body).getByText('$12.34')).toBeVisible()
  expect(body).toHaveClass('min-h-0', 'overflow-y-auto')
  for (const name of ['Pay with Balance', 'Stripe', 'Creem', 'Waffo Pancake', 'Pay']) {
    const action = screen.getByRole('button', { name })
    expect(action).toBeDisabled()
    expect(body).not.toContainElement(action)
  }
  expect(api.post).not.toHaveBeenCalled()
  expect(window.open).not.toHaveBeenCalled()
})

it.each([{ entry: 'Open status', title: 'Confirm disable' }, { entry: 'Open reset', title: 'Reset subscription quota' }])('$entry preserves its independent confirmation and shows the exact plan identity before any write', async ({ entry, title }) => {
  const user = userEvent.setup()
  render(<QueryClientProvider client={client}><SubscriptionsProvider><Confirmations /></SubscriptionsProvider></QueryClientProvider>)
  await user.click(screen.getByRole('button', { name: entry }))
  const confirmation = await screen.findByRole('alertdialog', { name: title })
  expect(confirmation).toHaveTextContent(plan.title)
  expect(within(confirmation).getByText('#31')).toBeVisible()
  if (entry === 'Open reset') expect(within(confirmation).getByRole('switch', { name: 'Advance next reset time' })).toBeChecked()
  await user.click(within(confirmation).getByRole('button', { name: 'Cancel' }))
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
  expect(api.patch).not.toHaveBeenCalled()
})

it.each([{ entry: 'Open status', title: 'Confirm disable', confirm: 'Disable' }, { entry: 'Open reset', title: 'Reset subscription quota', confirm: 'Reset quota' }])('a failed $entry request keeps the precise plan confirmation open and readable', async ({ entry, title, confirm }) => {
  vi.mocked(api.post).mockResolvedValue({ data: { success: false, message: 'Synthetic plan write failed' } })
  vi.mocked(api.put).mockResolvedValue({ data: { success: false, message: 'Synthetic plan write failed' } })
  vi.mocked(api.patch).mockResolvedValue({ data: { success: false, message: 'Synthetic plan write failed' } })
  const user = userEvent.setup()
  render(<QueryClientProvider client={client}><SubscriptionsProvider><Confirmations /></SubscriptionsProvider></QueryClientProvider>)
  await user.click(screen.getByRole('button', { name: entry }))
  const dialog = await screen.findByRole('alertdialog', { name: title })
  await user.click(within(dialog).getByRole('button', { name: confirm }))
  expect(await within(dialog).findByRole('alert')).toHaveTextContent('Synthetic plan write failed')
  expect(dialog).toHaveTextContent(plan.title)
  expect(within(dialog).getByText('#31')).toBeVisible()
  expect(within(dialog).getByRole('button', { name: 'Cancel' })).toBeEnabled()
  if (entry === 'Open status') {
    expect(vi.mocked(api.patch).mock.calls[0]?.[0]).toBe('/api/subscription/admin/plans/31')
    expect(vi.mocked(api.patch).mock.calls[0]?.[1]).toEqual({ enabled: false })
  } else {
    expect(vi.mocked(api.post).mock.calls[0]?.[0]).toBe('/api/subscription/admin/plans/31/subscriptions/reset')
    expect(vi.mocked(api.post).mock.calls[0]?.[1]).toEqual({ advance_reset_time: true })
  }
})
