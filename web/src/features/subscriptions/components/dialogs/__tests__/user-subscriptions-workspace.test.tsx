import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { UserSubscriptionsDialog } from '../user-subscriptions-dialog'

const originalAuth = useAuthStore.getState()
function record(id: number) {
  let total: number | null = 100
  if (id === 1) total = 0
  if (id === 2) total = null
  return { subscription: { id, user_id: 22, plan_id: 7, status: 'active', source: `record-${id}`, start_time: 1, end_time: 0, amount_total: total, amount_used: 0 } }
}
let records: ReturnType<typeof record>[]
let failedRead: boolean
beforeEach(() => {
  records = Array.from({ length: 25 }, (_, index) => record(index + 1))
  failedRead = false
  useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.SUPER_ADMIN, username: 'synthetic-root' })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/subscription/admin/plans') return { data: { success: true, data: [{ plan: { id: 7, title: 'Synthetic plan', price_amount: 1 } }] } }
    if (url === '/api/subscription/admin/users/22/subscriptions') {
      if (failedRead) throw new Error('Synthetic record read failed')
      return { data: { success: true, data: records } }
    }
    throw new Error(`Unexpected synthetic subscription GET ${url}`)
  })
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected synthetic subscription POST'))
  vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected synthetic subscription PUT'))
  vi.spyOn(api, 'delete').mockRejectedValue(new Error('Unexpected synthetic subscription DELETE'))
})
afterEach(() => { cleanup(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })
function mount() {
  render(<UserSubscriptionsDialog open onOpenChange={vi.fn()} user={{ id: 22, username: 'synthetic-user' }} />)
}

it('defaults to locally paged records and keeps the grant selection and record page across explicit task switches without requests', async () => {
  const originalGet = vi.mocked(api.get).getMockImplementation()
  if (!originalGet) throw new Error('Expected the synthetic subscription transport')
  vi.mocked(api.get).mockImplementation((url, ...args) => {
    if (url === '/api/subscription/admin/plans') {
      return Promise.resolve({ data: { success: true, data: [
      { plan: { id: 7, title: 'Synthetic plan' } },
      { plan: { id: 8, title: 'Known free plan', price_amount: 0 } },
      { plan: { id: 9, title: 'Invalid price plan', price_amount: Number.NaN } },
      { plan: { id: 10, title: 'Null price plan', price_amount: null } },
      ] } })
    }
    return originalGet(url, ...args)
  })
  mount()
  await screen.findByText('Loaded subscription records: 25')
  const user = userEvent.setup()
  expect(screen.getByRole('tab', { name: 'Subscription records' })).toHaveAttribute('aria-selected', 'true')
  expect(screen.queryByRole('button', { name: 'Add subscription' })).not.toBeInTheDocument()
  const rows = screen.getByRole('tabpanel', { name: 'Subscription records' })
  expect(within(rows).getByText('Unlimited')).toBeVisible()
  expect(within(rows).getByText('Unknown')).toBeVisible()
  expect(screen.queryByText('Source: record-21')).not.toBeInTheDocument()
  const reads = vi.mocked(api.get).mock.calls.length
  await user.click(screen.getByRole('button', { name: 'Last page' }))
  expect(screen.getByText('Source: record-21')).toBeVisible()
  expect(screen.queryByText('Source: record-1')).not.toBeInTheDocument()
  await user.click(screen.getByRole('tab', { name: 'Grant subscription' }))
  const select = screen.getByRole('combobox')
  await user.click(select)
  expect(screen.getByRole('option', { name: 'Known free plan ($0.00)' })).toBeVisible()
  expect(screen.getByRole('option', { name: 'Invalid price plan (Unknown)' })).toBeVisible()
  expect(screen.getByRole('option', { name: 'Null price plan (Unknown)' })).toBeVisible()
  expect(screen.getByRole('option', { name: 'Synthetic plan (Unknown)' })).toBeVisible()
  await user.click(screen.getByRole('option', { name: /Synthetic plan/ }))
  await user.click(screen.getByRole('tab', { name: 'Subscription records' }))
  expect(screen.getByText('Source: record-21')).toBeVisible()
  await user.selectOptions(within(rows).getByRole('combobox', { name: 'Rows per page' }), '50')
  expect(screen.getByText('Source: record-1')).toBeVisible()
  expect(screen.getByText('Source: record-25')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Last page' })).toBeDisabled()
  await user.click(screen.getByRole('tab', { name: 'Grant subscription' }))
  expect(screen.getByRole('combobox')).toBe(select)
  expect(select).toHaveTextContent('Synthetic plan')
  expect(select).toHaveTextContent('Unknown')
  expect(screen.getByRole('button', { name: 'Add subscription' })).toBeEnabled()
  expect(api.get).toHaveBeenCalledTimes(reads)
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
  expect(api.delete).not.toHaveBeenCalled()
})

it('deletes the precise record on the final local page and returns to the records heading when that last page disappears', async () => {
  records = records.slice(0, 21)
  const remove = vi.mocked(api.delete).mockImplementation(async (url) => {
    if (url !== '/api/subscription/admin/user_subscriptions/21') throw new Error(`Unexpected subscription deletion ${url}`)
    records = records.filter((item) => item.subscription.id !== 21)
    return { data: { success: true } }
  })
  mount()
  await screen.findByText('Loaded subscription records: 21')
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Last page' }))
  const actions = document.querySelector<HTMLElement>('[data-user-subscription-actions="21"]')
  if (!actions) throw new Error('Expected the final subscription record actions')
  await user.click(within(actions).getByRole('button', { name: 'Actions' }))
  await user.click(screen.getByRole('menuitem', { name: 'Delete' }))
  const confirmation = await screen.findByRole('alertdialog')
  expect(confirmation).toHaveTextContent('ID: 21')
  expect(confirmation).toHaveTextContent('User ID: 22')
  await user.click(within(confirmation).getByRole('button', { name: 'Continue' }))
  await screen.findByText('Loaded subscription records: 20')
  await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
  expect(screen.getByText('Source: record-1')).toBeVisible()
  expect(screen.queryByText('Source: record-21')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Last page' })).toBeDisabled()
  await waitFor(() => expect(screen.getByRole('heading', { name: 'Subscription records' })).toHaveFocus())
  expect(remove).toHaveBeenCalledExactlyOnceWith('/api/subscription/admin/user_subscriptions/21', { skipAuthRefresh: true })
  expect(api.post).not.toHaveBeenCalled()
})

it('keeps an unconfirmed grant lock and selected plan across both tasks and a failed read until explicit successful refresh and a new grant', async () => {
  const post = vi.mocked(api.post).mockRejectedValueOnce(new Error('Synthetic transport outcome unknown')).mockResolvedValue({ data: { success: true } })
  mount()
  await screen.findByText('Loaded subscription records: 25')
  const user = userEvent.setup()
  await user.click(screen.getByRole('tab', { name: 'Grant subscription' }))
  await user.click(screen.getByRole('combobox'))
  await user.click(screen.getByRole('option', { name: /Synthetic plan/ }))
  await user.click(screen.getByRole('button', { name: 'Add subscription' }))
  await screen.findByText('The subscription creation outcome is unconfirmed. Refresh and check the records before submitting again.')
  await user.click(screen.getByRole('tab', { name: 'Subscription records' }))
  expect(screen.getByText('The subscription creation outcome is unconfirmed. Refresh and check the records before submitting again.')).toBeVisible()
  await user.click(screen.getByRole('tab', { name: 'Grant subscription' }))
  expect(screen.getByRole('combobox')).toHaveTextContent('Synthetic plan')
  expect(screen.getByRole('button', { name: 'Add subscription' })).toBeDisabled()
  failedRead = true
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  expect(await screen.findByText('Loading failed')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Add subscription' })).toBeDisabled()
  expect(post).toHaveBeenCalledTimes(1)
  failedRead = false
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  await waitFor(() => expect(screen.getByRole('button', { name: 'Add subscription' })).toBeEnabled())
  await user.click(screen.getByRole('tab', { name: 'Subscription records' }))
  expect(screen.getByText('Source: record-1')).toBeVisible()
  await user.click(screen.getByRole('tab', { name: 'Grant subscription' }))
  expect(post).toHaveBeenCalledTimes(1)
  await user.click(screen.getByRole('button', { name: 'Add subscription' }))
  await waitFor(() => expect(post).toHaveBeenCalledTimes(2))
  expect(post.mock.calls.map((call) => call.slice(0, 2))).toEqual([
    ['/api/subscription/admin/users/22/subscriptions', { plan_id: 7 }],
    ['/api/subscription/admin/users/22/subscriptions', { plan_id: 7 }],
  ])
})
