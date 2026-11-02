import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
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
