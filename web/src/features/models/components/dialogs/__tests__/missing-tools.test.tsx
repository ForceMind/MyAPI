import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { ModelsProvider } from '../../models-provider'
import { MissingModelsDialog } from '../missing-models-dialog'

const originalAuth = useAuthStore.getState()
let client: QueryClient
beforeEach(() => {
  localStorage.clear()
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.ADMIN, username: 'synthetic-admin' })
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); localStorage.clear(); vi.restoreAllMocks() })

it.each(['transport', 'business'] as const)('a %s missing-model read failure is retryable and cannot claim every model is configured', async (kind) => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url !== '/api/models/missing') throw new Error(`Unexpected GET ${url}`)
    if (kind === 'transport') throw new Error('Synthetic missing read failed')
    return { data: { success: false, message: 'Synthetic missing read failed' } }
  })
  render(<QueryClientProvider client={client}><ModelsProvider><MissingModelsDialog open onOpenChange={() => {}} /></ModelsProvider></QueryClientProvider>)
  expect(await screen.findByRole('alert')).toHaveTextContent('Unable to load missing models')
  expect(screen.getByRole('button', { name: 'Retry' })).toBeEnabled()
  expect(screen.queryByText('All models in use are properly configured.')).not.toBeInTheDocument()
})

it('retrying a failed missing-model read displays a confirmed empty result only after the successful retry', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValueOnce({ data: { success: false, message: 'Synthetic missing read failed' } }).mockResolvedValue({ data: { success: true, data: [] } })
  const user = userEvent.setup()
  render(<QueryClientProvider client={client}><ModelsProvider><MissingModelsDialog open onOpenChange={() => {}} /></ModelsProvider></QueryClientProvider>)
  await screen.findByRole('alert')
  expect(screen.queryByText('All models in use are properly configured.')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByText('All models in use are properly configured.')).toBeVisible()
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(get).toHaveBeenCalledTimes(2)
})

it('the backend null missing-model result is a confirmed empty list', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: null } })
  render(<QueryClientProvider client={client}><ModelsProvider><MissingModelsDialog open onOpenChange={() => {}} /></ModelsProvider></QueryClientProvider>)
  expect(await screen.findByText('All models in use are properly configured.')).toBeVisible()
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

it.each([
  { name: 'missing data', data: undefined },
  { name: 'non-array data', data: 'invalid synthetic list' },
  { name: 'non-string model', data: ['synthetic-model', 4] },
])('a successful response with $name is an error rather than an invented empty list', async ({ data }) => {
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data } })
  render(<QueryClientProvider client={client}><ModelsProvider><MissingModelsDialog open onOpenChange={() => {}} /></ModelsProvider></QueryClientProvider>)
  expect(await screen.findByRole('alert')).toHaveTextContent('Invalid missing model response')
  expect(screen.queryByText('All models in use are properly configured.')).not.toBeInTheDocument()
})
