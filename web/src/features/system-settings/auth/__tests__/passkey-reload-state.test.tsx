import { QueryClient } from '@tanstack/react-query'
import { act, cleanup, fireEvent, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AxiosError, AxiosHeaders } from 'axios'
import { toast, Toaster } from 'sonner'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { AuthSettings } from '../index'
import { TYPED_BULK_REVISION_QUERY_KEY } from '../../hooks/use-typed-bulk-options'

const originalAuth = useAuthStore.getState()
let client: QueryClient
let saved: Record<string, string>
let revision: number
beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  saved = { 'passkey.rp_display_name': 'Saved RP', 'passkey.rp_id': 'saved.synthetic.invalid' }
  revision = 7
  useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.SUPER_ADMIN, username: 'synthetic-root' })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/option/') return { data: { success: true, data: Object.entries(saved).map(([key, value]) => ({ key, value })) } }
    if (url === '/api/option/typed-bulk/revision') return { data: { success: true, data: { revision } } }
    if (url === '/api/status') return { data: { success: true, data: {} } }
    throw new Error(`Unexpected GET ${url}`)
  })
  vi.spyOn(api, 'put').mockRejectedValue(new AxiosError('Synthetic revision conflict', 'ERR_BAD_REQUEST', undefined, undefined, { status: 409, statusText: 'Conflict', headers: {}, config: { headers: new AxiosHeaders() }, data: { message: 'Synthetic revision conflict' } }))
})
afterEach(() => { act(() => toast.dismiss()); cleanup(); client.clear(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })

async function renderPage() {
  const view = await renderListRoute({ client, path: '/system-settings/auth/$section', entry: '/system-settings/auth/passkey', element: <><AuthSettings /><Toaster /></> })
  await screen.findByDisplayValue('Saved RP')
  return view
}

it('preserves a Passkey draft across an observed background reread and accepts current saved values only after the original 409 Reload action', async () => {
  await renderPage()
  fireEvent.change(screen.getByLabelText('Relying Party Display Name'), { target: { value: 'Unsaved RP' } })
  saved = { 'passkey.rp_display_name': 'Remote RP', 'passkey.rp_id': 'remote.synthetic.invalid' }
  await act(async () => { await client.refetchQueries({ queryKey: ['system-options'] }) })
  expect(screen.getByLabelText('Relying Party ID')).toHaveValue('remote.synthetic.invalid')
  expect(screen.getByLabelText('Relying Party Display Name')).toHaveValue('Unsaved RP')
  expect(screen.getByText('Unsaved changes')).toBeVisible()
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Save Changes' }))
  await screen.findByRole('button', { name: 'Reload' })
  saved = { 'passkey.rp_display_name': 'Latest confirmed RP', 'passkey.rp_id': 'latest.synthetic.invalid' }
  revision = 8
  await user.click(screen.getByRole('button', { name: 'Reload' }))
  await waitFor(() => expect(screen.getByLabelText('Relying Party Display Name')).toHaveValue('Latest confirmed RP'))
  expect(screen.getByLabelText('Relying Party ID')).toHaveValue('latest.synthetic.invalid')
  expect(screen.queryByText('Unsaved changes')).not.toBeInTheDocument()
  expect(api.put).toHaveBeenCalledTimes(1)
})

it('does not let an old conflict toast reset a newly mounted actor draft', async () => {
  await renderPage()
  fireEvent.change(screen.getByLabelText('Relying Party Display Name'), { target: { value: 'Old actor draft' } })
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Save Changes' }))
  await screen.findByRole('button', { name: 'Reload' })
  await act(async () => { useAuthStore.getState().auth.setUser({ id: 2, role: ROLE.SUPER_ADMIN, username: 'synthetic-new-root' }) })
  await waitFor(() => expect(screen.getByLabelText('Relying Party Display Name')).toHaveValue('Saved RP'))
  fireEvent.change(screen.getByLabelText('Relying Party Display Name'), { target: { value: 'New actor draft' } })
  saved['passkey.rp_display_name'] = 'Latest server RP'
  saved['passkey.rp_id'] = 'new-read.synthetic.invalid'
  revision = 8
  await user.click(screen.getByRole('button', { name: 'Reload' }))
  await waitFor(() => expect(screen.getByLabelText('Relying Party ID')).toHaveValue('new-read.synthetic.invalid'))
  expect(screen.getByLabelText('Relying Party Display Name')).toHaveValue('New actor draft')
  expect(screen.getByText('Unsaved changes')).toBeVisible()
  expect(api.put).toHaveBeenCalledTimes(1)
})


it('does not dispatch an old Passkey PUT after revision lookup crosses an actor change and allows an explicit save in the new frame', async () => {
  await renderPage()
  await act(async () => { client.setQueryData(TYPED_BULK_REVISION_QUERY_KEY, { success: true, data: { revision: 7 } }, { updatedAt: 0 }) })
  const originalGet = vi.mocked(api.get).getMockImplementation()!
  let release: ((value: { data: { success: boolean; data: { revision: number } } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean; data: { revision: number } } }>((resolve) => { release = resolve })
  let deferredReads = 0
  vi.mocked(api.get).mockImplementation((url, ...args) => {
    if (url === '/api/option/typed-bulk/revision') { deferredReads += 1; return pending }
    return originalGet(url, ...args)
  })
  vi.mocked(api.put).mockResolvedValue({ data: { success: true, data: { revision: 9 } } })
  fireEvent.change(screen.getByLabelText('Relying Party Display Name'), { target: { value: 'Old delayed draft' } })
  await userEvent.setup().click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() => expect(deferredReads).toBe(1))
  await act(async () => { useAuthStore.getState().auth.setUser({ id: 2, role: ROLE.SUPER_ADMIN, username: 'new-synthetic-root' }) })
  await waitFor(() => expect(screen.getByLabelText('Relying Party Display Name')).toHaveValue('Saved RP'))
  await act(async () => { release?.({ data: { success: true, data: { revision: 8 } } }); await pending })
  expect(api.put).not.toHaveBeenCalled()
  fireEvent.change(screen.getByLabelText('Relying Party Display Name'), { target: { value: 'New explicit draft' } })
  await userEvent.setup().click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1))
  expect(vi.mocked(api.put).mock.calls[0]?.slice(0, 2)).toEqual(['/api/option/typed-bulk', { expected_revision: 8, items: [{ key: 'passkey.rp_display_name', type: 'string', value: 'New explicit draft' }] }])
})


it('does not dispatch the delayed Passkey PUT after discarding that section and lets the same actor save a newly opened section task', async () => {
  const { router } = await renderPage()
  await act(async () => { client.setQueryData(TYPED_BULK_REVISION_QUERY_KEY, { success: true, data: { revision: 7 } }, { updatedAt: 0 }) })
  const originalGet = vi.mocked(api.get).getMockImplementation()!
  let release: ((value: { data: { success: boolean; data: { revision: number } } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean; data: { revision: number } } }>((resolve) => { release = resolve })
  let deferredReads = 0
  vi.mocked(api.get).mockImplementation((url, ...args) => {
    if (url === '/api/option/typed-bulk/revision') { deferredReads += 1; return pending }
    return originalGet(url, ...args)
  })
  vi.mocked(api.put).mockResolvedValue({ data: { success: true, data: { revision: 9 } } })
  const user = userEvent.setup()
  fireEvent.change(screen.getByLabelText('Relying Party Display Name'), { target: { value: 'Discarded delayed draft' } })
  await user.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() => expect(deferredReads).toBe(1))
  await act(async () => { void router.navigate({ to: '/system-settings/auth/basic-auth' }) })
  await user.click(await screen.findByRole('button', { name: 'Leave', exact: true }))
  await waitFor(() => expect(screen.queryByLabelText('Relying Party Display Name')).not.toBeInTheDocument())
  await act(async () => { release?.({ data: { success: true, data: { revision: 8 } } }); await pending })
  expect(api.put).not.toHaveBeenCalled()
  await act(async () => { await router.navigate({ to: '/system-settings/auth/passkey' }) })
  await screen.findByDisplayValue('Saved RP')
  fireEvent.change(screen.getByLabelText('Relying Party Display Name'), { target: { value: 'New section task' } })
  await user.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1))
  expect(vi.mocked(api.put).mock.calls[0]?.slice(0, 2)).toEqual(['/api/option/typed-bulk', { expected_revision: 8, items: [{ key: 'passkey.rp_display_name', type: 'string', value: 'New section task' }] }])
})
