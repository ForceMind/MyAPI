import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
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
  let savedName = model.model_name
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/models/24') return { data: { success: true, data: { ...model, model_name: savedName, description: savedDescription } } }
    if (url === '/api/vendors/') return { data: { success: true, data: { items: [], total: 0 } } }
    throw new Error(`Unexpected GET ${url}`)
  })
  render(<QueryClientProvider client={client}><ModelMutateDrawer open currentRow={model} onOpenChange={() => {}} /></QueryClientProvider>)
  await screen.findByDisplayValue(model.model_name)
  fireEvent.change(screen.getByLabelText('Model Name *'), { target: { value: 'unsaved-model-name' } })
  fireEvent.change(screen.getByLabelText('Description'), { target: { value: 'unsaved description' } })
  savedDescription = 'changed saved description'
  savedName = 'fresh-saved-model-name'
  await act(async () => { await client?.invalidateQueries({ queryKey: modelsQueryKeys.detail(model.id) }) })
  await screen.findByText(savedName)
  expect(client.getQueryData<{ data: typeof model }>(modelsQueryKeys.detail(model.id))?.data.description).toBe(savedDescription)
  expect(screen.getByLabelText('Model Name *')).toHaveValue('unsaved-model-name')
  expect(screen.getByLabelText('Description')).toHaveValue('unsaved description')
})

it('submits original metadata fields and explicit false values once without an Admin pricing write', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'synthetic-admin', role: ROLE.ADMIN })
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const endpoints = { openai: { path: '/v1/synthetic', method: 'POST', extension: { enabled: false, count: 0 } } }
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/models/24') return { data: { success: true, data: { ...model, icon: 'OpenAI', tags: 'one,two', vendor_id: 7, endpoints: JSON.stringify(endpoints) } } }
    if (url === '/api/vendors/') return { data: { success: true, data: { items: [{ id: 7, name: 'Synthetic provider' }], total: 1 } } }
    throw new Error(`Unexpected GET ${url}`)
  })
  const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
  const post = vi.spyOn(api, 'post')
  render(<QueryClientProvider client={client}><ModelMutateDrawer open currentRow={model} onOpenChange={() => {}} /></QueryClientProvider>)
  await screen.findByDisplayValue(model.model_name)
  fireEvent.click(screen.getByRole('switch', { name: 'Enabled' }))
  fireEvent.click(screen.getByRole('button', { name: 'Update Model' }))
  await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
  const [path, body] = put.mock.calls[0]
  expect(path).toBe('/api/models/')
  expect(body).toMatchObject({ id: 24, model_name: model.model_name, description: model.description, icon: 'OpenAI', vendor_id: 7, tags: 'one,two', name_rule: 0, status: 0, sync_official: 0 })
  expect(JSON.parse((body as { endpoints: string }).endpoints)).toEqual(endpoints)
  expect(post).not.toHaveBeenCalled()
  expect(put.mock.calls.some(([url]) => url === '/api/option/')).toBe(false)
})
