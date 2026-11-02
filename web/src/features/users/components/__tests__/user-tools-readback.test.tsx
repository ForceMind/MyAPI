import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { userSchema } from '../../types'
import { UsersMutateDrawer } from '../users-mutate-drawer'
import { UsersProvider } from '../users-provider'

const originalAuth = useAuthStore.getState()
let client: QueryClient | undefined
afterEach(() => { cleanup(); client?.clear(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })

it('keeps an unsaved user draft and marks quota unknown when the confirmed adjustment succeeds but its quota reread fails', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.SUPER_ADMIN, username: 'synthetic-root' })
  const target = userSchema.parse({ id: 22, username: 'synthetic-user', display_name: '', quota: 0, used_quota: 0, request_count: 0, group: 'default', status: 1, role: 1 })
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  let failReadback = false
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/group/') return { data: { success: true, data: ['default'] } }
    if (url === '/api/authz/catalog') return { data: { success: true, data: { resources: [], roles: [] } } }
    if (url === '/api/user/22') {
      if (failReadback) throw new Error('Synthetic quota reread failed')
      return { data: { success: true, data: target } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  const post = vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true } })
  const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
  render(<QueryClientProvider client={client}><UsersProvider><UsersMutateDrawer open currentRow={target} onOpenChange={vi.fn()} /></UsersProvider></QueryClientProvider>)
  await screen.findByDisplayValue(target.username)
  fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'unsaved-user-draft' } })
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Adjust Quota' }))
  fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '1' } })
  failReadback = true
  await user.click(screen.getByRole('button', { name: 'Confirm' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Failed to load user')
  await waitFor(() => expect(screen.getByLabelText('Username')).toHaveValue('unsaved-user-draft'))
  expect(screen.getByLabelText(/Remaining Quota/)).toHaveValue('Unknown')
  expect(screen.getByRole('button', { name: 'Adjust Quota' })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Adjust Quota' }))
  expect(post).toHaveBeenCalledTimes(1)
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
  expect(put.mock.calls[0]?.slice(0, 2)).toEqual(['/api/user/', {
    username: 'unsaved-user-draft', display_name: 'unsaved-user-draft', password: undefined,
    group: 'default', account_tier_id: 'standard', remark: undefined, id: 22,
  }])
  expect(post).toHaveBeenCalledTimes(1)
})
