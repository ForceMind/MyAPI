import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { modelsQueryKeys } from '../../../lib/query-keys'
import { ModelMutateDrawer } from '../model-mutate-drawer'

const originalAuth = useAuthStore.getState()
let client: QueryClient | undefined
const model = { id: 24, model_name: 'saved-metadata-model', description: 'saved description', status: 1, sync_official: 0, created_time: 1, updated_time: 1, name_rule: 0 }
afterEach(() => { cleanup(); client?.clear(); useAuthStore.setState(originalAuth); localStorage.clear(); vi.restoreAllMocks() })

it('preserves an unsaved metadata draft when a fresh detail response changes saved fields', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'synthetic-admin', role: ROLE.ADMIN })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  let savedDescription = model.description
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/models/24') return { data: { success: true, data: { ...model, description: savedDescription } } }
    if (url === '/api/vendors/') return { data: { success: true, data: { items: [], total: 0 } } }
    throw new Error(`Unexpected GET ${url}`)
  })
  render(<QueryClientProvider client={client}><ModelMutateDrawer open currentRow={model} onOpenChange={() => {}} /></QueryClientProvider>)
  await screen.findByDisplayValue(model.model_name)
  fireEvent.change(screen.getByLabelText('Model Name *'), { target: { value: 'unsaved-model-name' } })
  fireEvent.change(screen.getByLabelText('Description'), { target: { value: 'unsaved description' } })
  savedDescription = 'changed saved description'
  await act(async () => { await client?.invalidateQueries({ queryKey: modelsQueryKeys.detail(model.id) }) })
  expect(client.getQueryData<{ data: typeof model }>(modelsQueryKeys.detail(model.id))?.data.description).toBe(savedDescription)
  expect(screen.getByLabelText('Model Name *')).toHaveValue('unsaved-model-name')
  expect(screen.getByLabelText('Description')).toHaveValue('unsaved description')
})
