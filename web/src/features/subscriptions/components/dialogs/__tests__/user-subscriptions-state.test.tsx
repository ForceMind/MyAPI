import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { UserSubscriptionsDialog } from '../user-subscriptions-dialog'

const originalAuth = useAuthStore.getState()
afterEach(() => { cleanup(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })

it('keeps the selected user subscription deletion confirmation open after its request fails', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.SUPER_ADMIN, username: 'synthetic-root' })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/subscription/admin/plans') return { data: { success: true, data: [{ plan: { id: 7, title: 'Synthetic plan', price_amount: 1 } }] } }
    if (url === '/api/subscription/admin/users/22/subscriptions') return { data: { success: true, data: [{ subscription: { id: 41, user_id: 22, plan_id: 7, status: 'active', source: 'admin', start_time: 1, end_time: 0, amount_total: 100, amount_used: 0 } }] } }
    throw new Error(`Unexpected GET ${url}`)
  })
  const remove = vi.spyOn(api, 'delete').mockRejectedValue(new Error('Synthetic subscription delete failure'))
  render(<UserSubscriptionsDialog open onOpenChange={vi.fn()} user={{ id: 22, username: 'synthetic-user' }} />)
  await screen.findByText('Synthetic plan')
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Actions' }))
  await user.click(screen.getByRole('menuitem', { name: 'Delete' }))
  const confirm = await screen.findByRole('alertdialog')
  await user.click(within(confirm).getByRole('button', { name: 'Continue' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Operation failed')
  expect(screen.getByRole('alertdialog')).toBeInTheDocument()
  expect(remove).toHaveBeenCalledTimes(1)
})
