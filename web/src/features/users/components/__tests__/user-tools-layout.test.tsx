import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useRef, useState, type ReactNode } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { UserSubscriptionsDialog } from '@/features/subscriptions/components/dialogs/user-subscriptions-dialog'
import { AccessPolicyDialog } from '@/features/access-policy/components/access-policy-dialog'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { UserBindingDialog } from '../dialogs/user-binding-dialog'
import { UserQuotaDialog } from '../user-quota-dialog'
import { UserUsagePolicyDialog } from '../user-usage-policy-dialog'
import { UsersMutateDrawer } from '../users-mutate-drawer'
import { UsersProvider } from '../users-provider'

const originalAuth = useAuthStore.getState()
let client: QueryClient
beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.SUPER_ADMIN, username: 'synthetic-root' })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/status') return { data: { success: true, data: {} } }
    if (url === '/api/authz/catalog') return { data: { success: true, data: { resources: [], roles: [] } } }
    if (url === '/api/group/') return { data: { success: true, data: ['default'] } }
    if (url === '/api/user/22') return { data: { success: true, data: { id: 22, username: 'synthetic-user', email: 'synthetic@example.invalid' } } }
    if (url === '/api/user/22/usage-policy') return { data: { success: true, data: { user_id: 22, no_balance: false, revision: 0, legacy_remaining_quota: 0 } } }
    if (url === '/api/access-policy/user/22') return { data: { success: true, data: { subject: 'user', subject_id: 22, owner_user_id: 22, revision: 0, assigned: false, enabled: true, public_models: null, upstream_models: null, channel_ids: null } } }
    if (url === '/api/user/22/oauth/bindings' || url === '/api/subscription/admin/plans' || url === '/api/subscription/admin/users/22/subscriptions') return { data: { success: true, data: [] } }
    throw new Error(`Unexpected GET ${url}`)
  })
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected write'))
  vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected write'))
  vi.spyOn(api, 'delete').mockRejectedValue(new Error('Unexpected write'))
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })

const tasks: Array<[string, () => ReactNode]> = [
  ['Adjust Quota', () => <UserQuotaDialog open onOpenChange={vi.fn()} userId={22} currentQuota={100} onSuccess={vi.fn()} />],
  ['Create User', () => <UsersMutateDrawer open onOpenChange={vi.fn()} />],
  ['Account Binding Management', () => <UserBindingDialog open onOpenChange={vi.fn()} userId={22} />],
  ['User Subscription Management', () => <UserSubscriptionsDialog open onOpenChange={vi.fn()} user={{ id: 22, username: 'synthetic-user' }} />],
  ['User usage policy', () => <UserUsagePolicyDialog userId={22} onClose={vi.fn()} />],
  ['Assigned access', () => <AccessPolicyDialog userId={22} onClose={vi.fn()} />],
]

it.each(tasks)('keeps %s to one vertical body scroll area with the close action outside it', async (name, content) => {
  render(<QueryClientProvider client={client}><UsersProvider>{content()}</UsersProvider></QueryClientProvider>)
  const dialog = await screen.findByRole('dialog', { name })
  const scrollAreas = [...dialog.querySelectorAll<HTMLElement>('*')].filter((element) => element.classList.contains('overflow-y-auto'))
  expect(scrollAreas).toHaveLength(1)
  expect(dialog).toHaveClass('overflow-hidden', 'h-dvh')
  const footer = dialog.querySelector('[data-slot=dialog-footer], [data-slot=sheet-footer]')
  await waitFor(() => expect(footer).not.toBeNull())
  expect(scrollAreas[0].contains(footer)).toBe(false)
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
  expect(api.delete).not.toHaveBeenCalled()
})

function QuotaEntry() {
  const [open, setOpen] = useState(false)
  const button = useRef<HTMLButtonElement>(null)
  return <><button type='button' ref={button} onClick={() => setOpen(true)}>Open quota task</button><UserQuotaDialog open={open} onOpenChange={setOpen} userId={22} currentQuota={100} onSuccess={vi.fn()} returnFocus={() => button.current} /></>
}

it('returns quota cancellation to the actual opener without issuing a write', async () => {
  render(<QuotaEntry />)
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Open quota task' }))
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  await waitFor(() => expect(screen.getByRole('button', { name: 'Open quota task' })).toHaveFocus())
  expect(api.post).not.toHaveBeenCalled()
})
