import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { UserSubscriptionsDialog } from '../user-subscriptions-dialog'

const originalAuth = useAuthStore.getState()
afterEach(() => { cleanup(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })

it('requires a successful explicit record refresh and a new user confirmation before repeating an unconfirmed subscription creation', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.SUPER_ADMIN, username: 'synthetic-root' })
  let refreshing = false
  let complete: ((value: { data: { success: boolean; data: [] } }) => void) | undefined
  const pendingRefresh = new Promise<{ data: { success: boolean; data: [] } }>((resolve) => { complete = resolve })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/subscription/admin/plans') return { data: { success: true, data: [{ plan: { id: 7, title: 'Synthetic plan', price_amount: 1 } }] } }
    if (url === '/api/subscription/admin/users/22/subscriptions') return refreshing ? pendingRefresh : { data: { success: true, data: [] } }
    throw new Error(`Unexpected GET ${url}`)
  })
  const post = vi.spyOn(api, 'post').mockRejectedValueOnce(new Error('Synthetic transport outcome unknown')).mockResolvedValue({ data: { success: true } })
  render(<UserSubscriptionsDialog open onOpenChange={vi.fn()} user={{ id: 22, username: 'synthetic-user' }} />)
  const user = userEvent.setup()
  await screen.findByText('No subscription records')
  await user.click(screen.getByRole('tab', { name: 'Grant subscription' }))
  await user.click(screen.getByRole('combobox'))
  await user.click(screen.getByRole('option', { name: /Synthetic plan/ }))
  await user.click(screen.getByRole('button', { name: 'Add subscription' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('unconfirmed')
  expect(screen.getByRole('button', { name: 'Add subscription' })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Add subscription' }))
  expect(post).toHaveBeenCalledTimes(1)

  refreshing = true
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  expect(screen.getByRole('button', { name: 'Add subscription' })).toBeDisabled()
  expect(post).toHaveBeenCalledTimes(1)
  await act(async () => { complete?.({ data: { success: true, data: [] } }); await pendingRefresh })
  await waitFor(() => expect(screen.getByRole('button', { name: 'Add subscription' })).toBeEnabled())
  await user.click(screen.getByRole('tab', { name: 'Subscription records' }))
  expect(screen.getByText('No subscription records')).toBeVisible()
  await user.click(screen.getByRole('tab', { name: 'Grant subscription' }))
  expect(post).toHaveBeenCalledTimes(1)
  await user.click(screen.getByRole('button', { name: 'Add subscription' }))
  await waitFor(() => expect(post).toHaveBeenCalledTimes(2))
  expect(post.mock.calls.map((call) => call.slice(0, 2))).toEqual([
    ['/api/subscription/admin/users/22/subscriptions', { plan_id: 7 }],
    ['/api/subscription/admin/users/22/subscriptions', { plan_id: 7 }],
  ])
})
