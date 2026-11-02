import { QueryClient } from '@tanstack/react-query'
import { act, cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { userSchema } from '../../types'
import { UsersDeleteDialog } from '../users-delete-dialog'
import { UsersProvider, useUsers } from '../users-provider'
import { UsersTable } from '../users-table'

const user = userSchema.parse({ id: 22, username: 'synthetic-user', display_name: 'Synthetic display name', email: 'synthetic@example.invalid', quota: 100, used_quota: 0, request_count: 0, group: 'default', status: 1, role: 1, created_at: 1730000000, last_login_at: 1730000010 })
const originalAuth = useAuthStore.getState()
let client: QueryClient
let total: number | undefined
beforeEach(() => {
  localStorage.clear()
  total = 1
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.SUPER_ADMIN, username: 'synthetic-root' })
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/status') return { data: { success: true, data: {} } }
    if (url === '/api/user/' || String(url).startsWith('/api/user/search?')) return { data: { success: true, data: { items: [user], total } } }
    throw new Error(`Unexpected GET ${url}`)
  })
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); localStorage.clear(); vi.restoreAllMocks() })

it('retains returned users while presenting an omitted total as unknown and disabling last-page navigation', async () => {
  total = undefined
  await renderListRoute({ client, path: '/users/', element: <UsersProvider><UsersTable /></UsersProvider> })
  await screen.findByText(user.username)
  const pager = screen.getByRole('navigation', { name: 'User pagination' })
  expect(within(pager).getByText('Record total unavailable')).toBeVisible()
  expect(within(pager).getByRole('button', { name: 'Last page' })).toBeDisabled()
})

it('exposes the original user date columns through the phone record disclosure', async () => {
  const original = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({ ...original(query), matches: query === CONSOLE_MOBILE_QUERY || query === '(max-width: 640px)' }))
  await renderListRoute({ client, path: '/users/', element: <UsersProvider><UsersTable /></UsersProvider> })
  await screen.findByText(user.username)
  await userEvent.setup().click(screen.getByText('More details'))
  expect(screen.getByText('Created At')).toBeVisible()
  expect(screen.getByText('Last Login')).toBeVisible()
})

function DeleteEntry() {
  const { setOpen, setCurrentRow } = useUsers()
  return <><button type='button' onClick={() => { setCurrentRow(user); setOpen('delete') }}>Open selected user</button><UsersDeleteDialog /></>
}

it('clears a selected user confirmation when the login identity changes', async () => {
  render(<QueryClientProvider client={client}><UsersProvider><DeleteEntry /></UsersProvider></QueryClientProvider>)
  await userEvent.setup().click(screen.getByRole('button', { name: 'Open selected user' }))
  expect(screen.getByRole('alertdialog')).toHaveTextContent(user.username)
  await act(async () => { useAuthStore.getState().auth.setUser({ id: 2, role: ROLE.SUPER_ADMIN, username: 'synthetic-new-root' }) })
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
})
