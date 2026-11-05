import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { formatQuota } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'

import { SubscriptionPlansCard } from '../subscription-plans-card'

const originalAuth = useAuthStore.getState()
function plan(id: number) {
  return { plan: { id, title: `Synthetic plan ${id}`, price_amount: 12.34, currency: 'USD', duration_unit: 'month', duration_value: 1, quota_reset_period: 'monthly', total_amount: 500000, max_purchase_per_user: 100, enabled: true, sort_order: id, allow_balance_pay: true } }
}
function record(id: number) {
  return { subscription: { id, user_id: 17, plan_id: 31, status: id <= 2 ? 'active' : 'expired', start_time: 1, end_time: id <= 2 ? 4000000000 : 1, amount_total: id === 1 ? 0 : 500000, amount_used: 0, next_reset_time: 3000000000 } }
}
let plans: ReturnType<typeof plan>[]
let records: ReturnType<typeof record>[]
function selfData() {
  return { success: true, data: { billing_preference: 'subscription_first', subscriptions: records.filter((row) => row.subscription.status === 'active'), all_subscriptions: records } }
}
function setSession(sid: string) {
  useAuthStore.getState().auth.setBundle({ access_token: 'synthetic-token', token_type: 'Bearer', access_expires_at: 4000000000, user: { id: 17, role: 1, username: 'synthetic-wallet' }, session: { sid, current: true, login_method: 'password', ip: '', user_agent: '', created_at: 1, last_active_at: 1, expires_at: 4000000000 } })
}
beforeEach(() => {
  plans = Array.from({ length: 25 }, (_, index) => plan(index + 31))
  records = Array.from({ length: 25 }, (_, index) => record(index + 1))
  setSession('wallet-session-one')
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/subscription/plans') return { data: { success: true, data: plans } }
    if (url === '/api/subscription/self') return { data: selfData() }
    throw new Error(`Unexpected synthetic wallet GET ${url}`)
  })
  vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected synthetic wallet PUT'))
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected synthetic wallet POST'))
  vi.spyOn(window, 'open').mockReturnValue(null)
})
afterEach(() => { cleanup(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })
function mount(canPurchaseSubscription = true) {
  return render(<TooltipProvider><SubscriptionPlansCard topupInfo={null} userQuota={1000000} canPurchaseSubscription={canPurchaseSubscription} /></TooltipProvider>)
}
function planCard(title: string) {
  const card = screen.getByRole('heading', { name: title }).closest<HTMLElement>('[data-slot="card"]')
  if (!card) throw new Error(`Expected the complete plan card ${title}`)
  return within(card)
}

it('keeps current, locally paged records and plans as explicit tasks without extra reads or writes', async () => {
  plans = plans.map((entry, index) => index === 0 ? { plan: { ...entry.plan, max_purchase_per_user: 25 } } : entry)
  mount()
  await screen.findByText('· Loaded subscription records: 25')
  const user = userEvent.setup()
  expect(screen.getByRole('tab', { name: 'Current subscriptions' })).toHaveAttribute('aria-selected', 'true')
  expect(screen.queryByRole('heading', { name: 'Synthetic plan 31' })).not.toBeInTheDocument()
  expect(within(screen.getByRole('tabpanel', { name: 'Current subscriptions' })).getByText('Total Quota: Unlimited')).toBeVisible()
  const reads = vi.mocked(api.get).mock.calls.length
  await user.click(screen.getByRole('tab', { name: 'Subscription records' }))
  const rows = screen.getByRole('tabpanel', { name: 'Subscription records' })
  await user.click(within(rows).getByRole('button', { name: 'Last page' }))
  expect(within(rows).getByText('Synthetic plan 31 · Subscription #25')).toBeVisible()
  expect(within(rows).queryByText('Synthetic plan 31 · Subscription #1')).not.toBeInTheDocument()
  await user.click(screen.getByRole('tab', { name: 'Browse plans' }))
  const catalog = screen.getByRole('tabpanel', { name: 'Browse plans' })
  await user.click(within(catalog).getByRole('button', { name: 'Last page' }))
  expect(within(catalog).getByRole('heading', { name: 'Synthetic plan 55' })).toBeVisible()
  await user.click(screen.getByRole('tab', { name: 'Subscription records' }))
  expect(within(rows).getByText('Synthetic plan 31 · Subscription #25')).toBeVisible()
  await user.selectOptions(within(rows).getByRole('combobox', { name: 'Rows per page' }), '50')
  expect(within(rows).getByText('Synthetic plan 31 · Subscription #1')).toBeVisible()
  expect(within(rows).getByText('Synthetic plan 31 · Subscription #25')).toBeVisible()
  await user.click(screen.getByRole('tab', { name: 'Browse plans' }))
  expect(within(catalog).getByRole('heading', { name: 'Synthetic plan 31' })).toBeVisible()
  expect(within(catalog).getByRole('heading', { name: 'Synthetic plan 55' })).toBeVisible()
  expect(planCard('Synthetic plan 31').getByRole('button', { name: 'Limit Reached' })).toBeDisabled()
  expect(api.get).toHaveBeenCalledTimes(reads)
  expect(api.put).not.toHaveBeenCalled()
  expect(api.post).not.toHaveBeenCalled()
})

it('keeps independent read failures visible and distinguishes unknown quotes and counts from legitimate zero', async () => {
  let selfFailed = true
  let plansFailed = true
  vi.mocked(api.get).mockImplementation(async (url) => {
    if (url === '/api/subscription/plans') return { data: plansFailed ? { success: false, message: 'Synthetic plans unavailable' } : { success: true, data: [ { plan: { ...plan(31).plan, title: 'Known free plan', price_amount: 0, total_amount: 0 } }, { plan: { ...plan(32).plan, title: 'Unknown price plan', price_amount: null } } ] } }
    if (url === '/api/subscription/self') { if (selfFailed) throw new Error('Synthetic records unavailable'); return { data: selfData() } }
    throw new Error(`Unexpected synthetic wallet GET ${url}`)
  })
  mount()
  await screen.findByText('Synthetic plans unavailable')
  await screen.findByText('Synthetic records unavailable')
  const user = userEvent.setup()
  expect(screen.queryByText('No Active')).not.toBeInTheDocument()
  expect(screen.getByRole('combobox', { name: 'Billing preference' })).toBeDisabled()
  plansFailed = false
  await user.click(screen.getByRole('button', { name: 'Retry plans' }))
  await user.click(screen.getByRole('tab', { name: 'Browse plans' }))
  await screen.findByRole('heading', { name: 'Known free plan' })
  expect(await planCard('Known free plan').findByText('$0.00')).toBeVisible()
  expect(planCard('Known free plan').getByRole('button', { name: 'Quote unavailable' })).toBeDisabled()
  expect(planCard('Unknown price plan').getByText('Unknown')).toBeVisible()
  expect(planCard('Unknown price plan').queryByText('$0.00')).not.toBeInTheDocument()
  records = []
  selfFailed = false
  await user.click(screen.getByRole('button', { name: 'Retry subscriptions' }))
  await waitFor(() => expect(planCard('Known free plan').getByRole('button', { name: 'Subscribe Now' })).toBeEnabled())
  expect(planCard('Unknown price plan').getByRole('button', { name: 'Quote unavailable' })).toBeDisabled()
  expect(screen.getByText(/Wallet will be used automatically/)).toBeVisible()
  expect(api.post).not.toHaveBeenCalled()
})

it('preserves a failed explicit preference command and permits history while purchases are disabled', async () => {
  mount(false)
  await screen.findByText('· Loaded subscription records: 25')
  expect(api.get).toHaveBeenCalledExactlyOnceWith('/api/subscription/self', { skipErrorHandler: true, skipBusinessError: true, disableDuplicate: true, isCurrentScope: expect.any(Function) })
  const user = userEvent.setup()
  const browseTab = screen.getByRole('tab', { name: 'Browse plans' })
  expect(browseTab).toHaveAttribute('aria-disabled', 'true')
  const reads = vi.mocked(api.get).mock.calls.length
  fireEvent.click(browseTab)
  expect(browseTab).toHaveAttribute('aria-selected', 'false')
  expect(screen.getByRole('tab', { name: 'Current subscriptions' })).toHaveAttribute('aria-selected', 'true')
  expect(screen.queryByRole('tabpanel', { name: 'Browse plans' })).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Subscribe Now' })).not.toBeInTheDocument()
  expect(api.get).toHaveBeenCalledTimes(reads)
  expect(api.post).not.toHaveBeenCalled()
  await user.click(screen.getByRole('tab', { name: 'Subscription records' }))
  expect(screen.getByText('Subscription purchases are unavailable. Existing records remain readable.')).toBeVisible()
  vi.mocked(api.put).mockResolvedValue({ data: { success: false, message: 'Synthetic preference rejected' } })
  await user.click(screen.getByRole('combobox', { name: 'Billing preference' }))
  await user.click(screen.getByRole('option', { name: 'Wallet Only' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Synthetic preference rejected')
  expect(screen.getByRole('combobox', { name: 'Billing preference' })).toHaveTextContent('Subscription First')
  expect(api.put).toHaveBeenCalledExactlyOnceWith('/api/subscription/self/preference', { billing_preference: 'wallet_only' }, { skipErrorHandler: true, skipBusinessError: true, disableDuplicate: true, isCurrentScope: expect.any(Function), skipAuthRefresh: true })
  await user.click(screen.getByRole('tab', { name: 'Current subscriptions' }))
  expect(api.put).toHaveBeenCalledTimes(1)
  expect(api.post).not.toHaveBeenCalled()
})

it('opens the original precise purchase confirmation without payment and returns focus while its original close refresh is pending', async () => {
  plans = [plan(31)]
  mount()
  await screen.findByText('· Loaded subscription records: 25')
  const user = userEvent.setup()
  await user.click(screen.getByRole('tab', { name: 'Browse plans' }))
  await user.click(planCard('Synthetic plan 31').getByRole('button', { name: 'Subscribe Now' }))
  const dialog = await screen.findByRole('dialog', { name: 'Purchase Subscription' })
  expect(within(dialog).getByText('Synthetic plan 31')).toBeVisible()
  expect(within(within(dialog).getByRole('group', { name: 'Amount Due' })).getByText('$12.34')).toBeVisible()
  expect(api.post).not.toHaveBeenCalled()
  expect(window.open).not.toHaveBeenCalled()
  let completeRead: ((value: { data: ReturnType<typeof selfData> }) => void) | undefined
  vi.mocked(api.get).mockImplementation((url) => {
    if (url !== '/api/subscription/self') throw new Error(`Unexpected synthetic close GET ${url}`)
    return new Promise((resolve) => { completeRead = resolve })
  })
  await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
  await waitFor(() => expect(screen.getByRole('heading', { name: 'Browse plans' })).toHaveFocus())
  expect(api.get).toHaveBeenCalledTimes(3)
  expect(api.post).not.toHaveBeenCalled()
  await act(async () => { if (!completeRead) throw new Error('Expected the original close refresh'); completeRead({ data: selfData() }) })
})

it('rejects an old SID read without restoring old private records or sending writes', async () => {
  let oldRead: ((value: { data: ReturnType<typeof selfData> }) => void) | undefined
  let selfReads = 0
  vi.mocked(api.get).mockImplementation((url) => {
    if (url === '/api/subscription/plans') return Promise.resolve({ data: { success: true, data: plans } })
    if (url !== '/api/subscription/self') throw new Error(`Unexpected synthetic wallet GET ${url}`)
    selfReads += 1
    if (selfReads === 1) return new Promise((resolve) => { oldRead = resolve })
    return Promise.resolve({ data: { success: true, data: { billing_preference: 'wallet_only', subscriptions: [], all_subscriptions: [] } } })
  })
  mount()
  await waitFor(() => expect(selfReads).toBe(1))
  await act(async () => { setSession('wallet-session-two') })
  await screen.findByText('· Loaded subscription records: 0')
  await act(async () => { if (!oldRead) throw new Error('Expected the old SID request'); oldRead({ data: selfData() }) })
  expect(screen.getByText('· Loaded subscription records: 0')).toBeVisible()
  expect(screen.queryByText('· Loaded subscription records: 25')).not.toBeInTheDocument()
  expect(screen.getByRole('combobox', { name: 'Billing preference' })).toHaveTextContent('Wallet Only')
  expect(api.put).not.toHaveBeenCalled()
  expect(api.post).not.toHaveBeenCalled()
})


it('preserves each known quota and terminal state independently from unknown usage and dates', async () => {
  const entries = [
    { subscription: { ...record(1).subscription, status: 'cancelled', amount_total: 500000, amount_used: null, end_time: null } },
    { subscription: { ...record(2).subscription, amount_total: 0, amount_used: null } },
    { subscription: { ...record(3).subscription, status: 'expired', end_time: null } },
    { subscription: { ...record(4).subscription, status: 'active', end_time: null } },
  ]
  vi.mocked(api.get).mockImplementation(async (url) => {
    if (url === '/api/subscription/plans') return { data: { success: true, data: plans } }
    if (url === '/api/subscription/self') return { data: { success: true, data: { billing_preference: 'subscription_first', subscriptions: entries.filter((entry) => entry.subscription.status === 'active'), all_subscriptions: entries } } }
    throw new Error(`Unexpected synthetic wallet GET ${url}`)
  })
  mount()
  await screen.findByText('· Loaded subscription records: 4')
  const user = userEvent.setup()
  await user.click(screen.getByRole('tab', { name: 'Subscription records' }))
  const rows = screen.getByRole('tabpanel', { name: 'Subscription records' })
  const cancelled = rows.querySelector('[data-wallet-subscription-record="1"]')
  const unlimited = rows.querySelector('[data-wallet-subscription-record="2"]')
  const expired = rows.querySelector('[data-wallet-subscription-record="3"]')
  const activeUnknown = rows.querySelector('[data-wallet-subscription-record="4"]')
  if (!(cancelled instanceof HTMLElement) || !(unlimited instanceof HTMLElement) || !(expired instanceof HTMLElement) || !(activeUnknown instanceof HTMLElement)) throw new Error('Expected all precise subscription records')
  expect(within(cancelled).getByText('Cancelled')).toBeVisible()
  expect(cancelled).toHaveTextContent('Cancelled at Unknown')
  expect(cancelled).toHaveTextContent(`Total Quota: ${formatQuota(500000)} · Used Unknown · Remaining Unknown`)
  expect(within(cancelled).queryByRole('progressbar')).not.toBeInTheDocument()
  expect(cancelled).not.toHaveTextContent('0%')
  expect(within(unlimited).getByText('Total Quota: Unlimited')).toBeVisible()
  expect(unlimited).not.toHaveTextContent('Used 0')
  expect(within(expired).getByText('Expired')).toBeVisible()
  expect(expired).toHaveTextContent('Expired at Unknown')
  expect(within(activeUnknown).queryByText('Active')).not.toBeInTheDocument()
  expect(within(activeUnknown).getByText('Unknown')).toBeVisible()
  expect(activeUnknown).toHaveTextContent('Validity Unknown')
  expect(api.get).toHaveBeenCalledTimes(2)
  expect(api.put).not.toHaveBeenCalled()
  expect(api.post).not.toHaveBeenCalled()
})
