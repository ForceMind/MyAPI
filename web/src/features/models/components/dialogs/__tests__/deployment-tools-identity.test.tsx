import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { RenameDeploymentDialog } from '../rename-deployment-dialog'
import { ViewDetailsDialog } from '../view-details-dialog'

const originalAuth = useAuthStore.getState()
let client: QueryClient

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.ADMIN, username: 'synthetic-one' })
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected deployment write'))
  vi.spyOn(api, 'delete').mockRejectedValue(new Error('Unexpected deployment write'))
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })

it('rereads the same deployment inside the new identity instead of displaying the previous identity cache', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/deployments/17/containers') return { data: { success: true, data: { containers: [] } } }
    if (url === '/api/deployments/17') return { data: { success: true, data: { status: `identity-${useAuthStore.getState().auth.user?.id}` } } }
    throw new Error(`Unexpected GET ${url}`)
  })
  render(<QueryClientProvider client={client}><ViewDetailsDialog open onOpenChange={vi.fn()} deploymentId={17} /></QueryClientProvider>)
  await screen.findByText('identity-1')
  await act(async () => { useAuthStore.getState().auth.setUser({ id: 2, role: ROLE.ADMIN, username: 'synthetic-two' }) })
  expect(await screen.findByText('identity-2')).toBeVisible()
  expect(screen.queryByText('identity-1')).not.toBeInTheDocument()
})

it('ignores an old identity rename result and keeps the new identity draft open', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: { available: true } } })
  let complete: ((value: { data: { success: boolean } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean } }>((resolve) => { complete = resolve })
  const put = vi.spyOn(api, 'put').mockReturnValue(pending)
  const close = vi.fn()
  render(<QueryClientProvider client={client}><RenameDeploymentDialog open onOpenChange={close} deploymentId={17} currentName='saved-name' /></QueryClientProvider>)
  fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'identity-one-draft' } })
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Rename' }))
  await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
  await act(async () => { useAuthStore.getState().auth.setUser({ id: 2, role: ROLE.ADMIN, username: 'synthetic-two' }) })
  await waitFor(() => expect(screen.getByLabelText('Name')).toHaveValue('saved-name'))
  fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'identity-two-draft' } })
  await act(async () => { complete?.({ data: { success: true } }); await pending })
  expect(screen.getByLabelText('Name')).toHaveValue('identity-two-draft')
  expect(close).not.toHaveBeenCalled()
  expect(put).toHaveBeenCalledTimes(1)
})
