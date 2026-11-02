import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { userSchema } from '../../types'
import { UserQuotaDialog } from '../user-quota-dialog'
import { UsersMutateDrawer } from '../users-mutate-drawer'
import { UsersProvider } from '../users-provider'

const target = userSchema.parse({ id: 22, username: 'synthetic-user', display_name: 'Saved display name', quota: 100, used_quota: 0, request_count: 0, group: 'default', status: 1, role: 1 })
const originalAuth = useAuthStore.getState()
let client: QueryClient
beforeEach(() => {
  localStorage.clear()
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.SUPER_ADMIN, username: 'synthetic-root' })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/user/22') return { data: { success: true, data: target } }
    if (url === '/api/group/') return { data: { success: true, data: ['default'] } }
    if (url === '/api/authz/catalog') return { data: { success: true, data: { resources: [], roles: [] } } }
    throw new Error(`Unexpected GET ${url}`)
  })
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); localStorage.clear(); vi.restoreAllMocks() })
function quotaFixture() {
  return <UserQuotaDialog open onOpenChange={vi.fn()} userId={22} currentQuota={100} onSuccess={vi.fn()} />
}

it('keeps a failed quota adjustment visible and reuses the existing request ID when the same parameters are retried', async () => {
  const post = vi.spyOn(api, 'post').mockRejectedValue(new Error('Synthetic quota adjustment failed'))
  render(quotaFixture())
  fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '1' } })
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Confirm' }))
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
  await waitFor(() => expect(screen.getByRole('button', { name: 'Confirm' })).toBeEnabled())
  await user.click(screen.getByRole('button', { name: 'Confirm' }))
  await waitFor(() => expect(post).toHaveBeenCalledTimes(2))
  expect(post.mock.calls[1]?.[1]).toEqual(post.mock.calls[0]?.[1])
  expect(post.mock.calls[0]?.[1]).toMatchObject({ id: 22, action: 'add_quota', mode: 'add', request_id: expect.any(String) })
  expect(await screen.findByText('Synthetic quota adjustment failed')).toBeVisible()
  expect(screen.getByRole('spinbutton')).toHaveValue(1)
})

function ControlledQuota() {
  const [open, setOpen] = useState(true)
  return <UserQuotaDialog open={open} onOpenChange={setOpen} userId={22} currentQuota={100} onSuccess={vi.fn()} />
}

it('keeps the exact quota adjustment task open when Escape is pressed during its pending write', async () => {
  let complete: ((value: { data: { success: boolean } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean } }>((resolve) => { complete = resolve })
  const post = vi.spyOn(api, 'post').mockReturnValue(pending)
  render(<ControlledQuota />)
  fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '1' } })
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Confirm' }))
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
  try {
    await user.keyboard('{Escape}')
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByRole('spinbutton')).toHaveValue(1)
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
  } finally {
    await act(async () => { complete?.({ data: { success: true } }); await pending })
  }
})

function editor(row = target) {
  return <QueryClientProvider client={client}><UsersProvider><UsersMutateDrawer open onOpenChange={vi.fn()} currentRow={row} /></UsersProvider></QueryClientProvider>
}

it('preserves the user editor draft when a fresh same-user row triggers a detail reread', async () => {
  const view = render(editor())
  await screen.findByDisplayValue(target.username)
  fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'unsaved-user-draft' } })
  const get = vi.mocked(api.get)
  const previous = get.getMockImplementation()
  let complete: ((value: { data: { success: boolean; data: typeof target } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean; data: typeof target } }>((resolve) => { complete = resolve })
  get.mockImplementation((url, ...args) => url === '/api/user/22' ? pending : previous?.(url, ...args) ?? Promise.reject(new Error(`Unexpected GET ${url}`)))
  await act(async () => { view.rerender(editor({ ...target, display_name: 'Fresh saved row' })); complete?.({ data: { success: true, data: { ...target, username: 'new-server-username' } } }); await pending })
  expect(screen.getByLabelText('Username')).toHaveValue('unsaved-user-draft')
})

it('admits one user update when the valid editor form is submitted twice while the first update is pending', async () => {
  let complete: ((value: { data: { success: boolean } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean } }>((resolve) => { complete = resolve })
  const put = vi.spyOn(api, 'put').mockReturnValue(pending)
  render(editor())
  const input = await screen.findByDisplayValue(target.username)
  const form = input.closest('form')
  if (!form) throw new Error('User editor form is missing')
  await act(async () => { fireEvent.submit(form); fireEvent.submit(form); await Promise.resolve() })
  await waitFor(() => expect(put).toHaveBeenCalled())
  try {
    expect(put).toHaveBeenCalledTimes(1)
    expect(put.mock.calls[0]?.[1]).toMatchObject({ id: 22, username: target.username })
  } finally {
    await act(async () => { complete?.({ data: { success: true } }); await pending })
  }
})
