import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { toast } from 'sonner'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { DEFAULT_CURRENCY_CONFIG, useSystemConfigStore } from '@/stores/system-config-store'
import { subscriptionPlanSchema } from '../../../types'
import { SubscriptionPurchaseDialog } from '../subscription-purchase-dialog'

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))
const originalAuth = useAuthStore.getState()
const originalConfig = useSystemConfigStore.getState()
const plan = subscriptionPlanSchema.parse({ id: 31, title: 'Original precise plan', price_amount: 12.34, currency: 'USD', duration_unit: 'month', duration_value: 1, quota_reset_period: 'never', enabled: true, sort_order: 0, allow_balance_pay: true, max_purchase_per_user: 0, total_amount: 500000, stripe_price_id: 'stripe-plan', creem_product_id: 'creem-plan', waffo_pancake_product_id: 'waffo-plan' })
let sequence = 0
function signIn(id = 1, sid = `original-${++sequence}`) {
  useAuthStore.setState((state) => ({ auth: { ...state.auth, user: { id, role: 1, username: 'owner' }, session: { sid, current: true, login_method: 'password', ip: '', user_agent: '', created_at: 1, last_active_at: 1, expires_at: 4000000000 } } }))
}
beforeEach(() => { signIn(); useSystemConfigStore.getState().setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG, quotaPerUnit: 500000 } }); vi.spyOn(window, 'open').mockReturnValue(null); vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(() => {}); vi.mocked(toast.success).mockClear(); vi.mocked(toast.error).mockClear() })
afterEach(() => { cleanup(); useAuthStore.setState(originalAuth); useSystemConfigStore.setState(originalConfig); vi.restoreAllMocks() })

const flows = [
  { action: 'Stripe', path: '/api/subscription/stripe/pay', data: { message: 'success', data: { pay_link: 'https://pay.synthetic.invalid' } } },
  { action: 'Creem', path: '/api/subscription/creem/pay', data: { message: 'success', data: { checkout_url: 'https://pay.synthetic.invalid' } } },
  { action: 'Waffo Pancake', path: '/api/subscription/waffo-pancake/pay', data: { message: 'success', data: { checkout_url: 'https://pay.synthetic.invalid' } } },
  { action: 'Pay', path: '/api/subscription/epay/pay', data: { message: 'success', url: 'https://pay.synthetic.invalid', data: { order: 'precise-order' } } },
  { action: 'Pay with Balance', path: '/api/subscription/balance/pay', data: { success: true } },
]

for (const flow of flows) {
  it(`${flow.action} cannot apply a former actor response to the current account and never changes its original plan payload`, async () => {
    let release!: () => void
    const gate = new Promise<void>((resolve) => { release = resolve })
    const post = vi.spyOn(api, 'post').mockImplementation(async () => { await gate; return { data: flow.data } })
    const close = vi.fn()
    const purchased = vi.fn()
    render(<SubscriptionPurchaseDialog open plan={{ plan }} onOpenChange={close} onPurchaseSuccess={purchased} userQuota={100000000} enableStripe enableCreem enableWaffoPancake enableOnlineTopUp epayMethods={[{ type: 'alipay', name: 'Synthetic Pay' }]} />)
    await userEvent.setup().click(screen.getByRole('button', { name: flow.action, exact: true }))
    await waitFor(() => expect(post).toHaveBeenCalledOnce())
    const location = window.location.href
    act(() => signIn(2, 'next'))
    await act(async () => release())
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(window.open).not.toHaveBeenCalled()
    expect(HTMLFormElement.prototype.submit).not.toHaveBeenCalled()
    expect(window.location.href).toBe(location)
    expect(close).not.toHaveBeenCalled()
    expect(purchased).not.toHaveBeenCalled()
    expect(toast.success).not.toHaveBeenCalled()
    expect(toast.error).not.toHaveBeenCalled()
    const expected = flow.action === 'Pay' ? { plan_id: 31, payment_method: 'alipay' } : { plan_id: 31 }
    expect(post).toHaveBeenCalledExactlyOnceWith(flow.path, expected, { skipErrorHandler: true, skipBusinessError: true, disableDuplicate: true, isCurrentScope: expect.any(Function), skipAuthRefresh: true })
  })
}

it.each(['SID', 'unmount', 'close-reopen', 'plan'] as const)('a Stripe response after %s cannot close or redirect a newly opened task', async (change) => {
  let release!: () => void
  const gate = new Promise<void>((resolve) => { release = resolve })
  const post = vi.spyOn(api, 'post').mockImplementation(async () => { await gate; return { data: { message: 'success', data: { pay_link: 'https://pay.synthetic.invalid' } } } })
  const close = vi.fn()
  const show = (open = true, nextPlan = plan) => <SubscriptionPurchaseDialog open={open} plan={{ plan: nextPlan }} onOpenChange={close} onOpenHistory={() => {}} userQuota={100000000} enableStripe />
  const view = render(show())
  const stripe = screen.getByRole('button', { name: 'Stripe', exact: true })
  act(() => { fireEvent.click(stripe); fireEvent.click(stripe) })
  expect(post).toHaveBeenCalledOnce()
  if (change === 'SID') act(() => signIn(1, 'next'))
  if (change === 'unmount') view.unmount()
  if (change === 'close-reopen') { view.rerender(show(false)); view.rerender(show(true)) }
  if (change === 'plan') view.rerender(show(true, { ...plan, id: 32, title: 'New precise plan' }))
  await act(async () => release())
  expect(window.open).not.toHaveBeenCalled()
  expect(close).not.toHaveBeenCalled()
  expect(toast.success).not.toHaveBeenCalled()
  expect(post).toHaveBeenCalledOnce()
  if (change === 'close-reopen') {
    expect(screen.getByRole('dialog', { name: 'Purchase Subscription' })).toBeVisible()
    expect(screen.getByText('Payment outcome is unconfirmed. New payments are locked; review billing history.')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Billing History' })).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Stripe', exact: true })).toBeDisabled()
  }
  if (change === 'plan') expect(screen.getByText('New precise plan')).toBeVisible()
})

it('missing balance is Unknown and cannot authorize a free balance purchase while an explicit zero remains a known balance', () => {
  const post = vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected purchase'))
  const free = { ...plan, price_amount: 0 }
  const view = render(<SubscriptionPurchaseDialog open plan={{ plan: free }} onOpenChange={() => {}} />)
  expect(screen.getByText('Unknown')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Pay with Balance' })).toBeDisabled()
  view.rerender(<SubscriptionPurchaseDialog open plan={{ plan: free }} onOpenChange={() => {}} userQuota={0} />)
  expect(screen.getByRole('button', { name: 'Pay with Balance' })).toBeEnabled()
  expect(post).not.toHaveBeenCalled()
})

it('a lost payment response locks every method for the same plan across close, reopen and plan switches, while history stays reachable', async () => {
  const post = vi.spyOn(api, 'post').mockRejectedValue(new Error('Synthetic response lost after order creation'))
  const history = vi.fn()
  const show = (open = true, selected = plan) => <SubscriptionPurchaseDialog open={open} plan={{ plan: selected }} onOpenChange={() => {}} onOpenHistory={history} userQuota={100000000} enableStripe enableCreem />
  const view = render(show())
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Stripe', exact: true }))
  expect(await screen.findByText('Payment outcome is unconfirmed. New payments are locked; review billing history.')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Stripe', exact: true })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Creem', exact: true })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Pay with Balance' })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Stripe', exact: true }))
  view.rerender(show(false))
  view.rerender(show(true))
  expect(screen.getByRole('button', { name: 'Stripe', exact: true })).toBeDisabled()
  view.rerender(show(true, { ...plan, id: 32, title: 'Other plan' }))
  view.rerender(show())
  expect(screen.getByRole('button', { name: 'Stripe', exact: true })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Billing History' }))
  expect(history).toHaveBeenCalledOnce()
  expect(post).toHaveBeenCalledExactlyOnceWith('/api/subscription/stripe/pay', { plan_id: 31 }, { skipErrorHandler: true, skipBusinessError: true, disableDuplicate: true, isCurrentScope: expect.any(Function), skipAuthRefresh: true })
})
