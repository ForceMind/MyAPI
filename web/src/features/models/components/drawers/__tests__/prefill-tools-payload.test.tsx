import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import type { PrefillGroup } from '../../../types'
import { PrefillGroupFormDrawer } from '../prefill-group-form-drawer'

let client: QueryClient | undefined
afterEach(() => {
  cleanup()
  client?.clear()
  vi.restoreAllMocks()
})

it.each<Pick<PrefillGroup, 'type' | 'items'>>([
  { type: 'model', items: ['synthetic-chat', 'synthetic-code'] },
  { type: 'tag', items: ['synthetic-tag'] },
  { type: 'endpoint', items: '{"openai":{"path":"/v1/synthetic","method":"POST","extension":{"enabled":false,"count":0}},"disabled":false,"limit":0}' },
])('saving an existing $type group preserves its exact items contract and ID', async (fixture) => {
  const user = userEvent.setup()
  client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
  const close = vi.fn()
  render(<QueryClientProvider client={client}><PrefillGroupFormDrawer open currentGroup={{ id: 22, name: 'Saved selection', description: 'Saved notes', ...fixture }} onClose={close} /></QueryClientProvider>)
  await screen.findByDisplayValue('Saved selection')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
  expect(put.mock.calls[0]?.[0]).toBe('/api/prefill_group')
  expect(put.mock.calls[0]?.[1]).toEqual({ id: 22, name: 'Saved selection', description: 'Saved notes', ...fixture })
  await waitFor(() => expect(close).toHaveBeenCalledTimes(1))
})

it('a failed save retains the edited name, description, and original items for retry', async () => {
  const user = userEvent.setup()
  client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: false, message: 'Synthetic save failed' } })
  const close = vi.fn()
  render(<QueryClientProvider client={client}><PrefillGroupFormDrawer open currentGroup={{ id: 23, name: 'Saved name', description: 'Saved notes', type: 'model', items: ['kept-model'] }} onClose={close} /></QueryClientProvider>)
  const name = await screen.findByLabelText('Group Name')
  await user.clear(name)
  await user.type(name, 'Unsaved name')
  await user.clear(screen.getByLabelText('Description'))
  await user.type(screen.getByLabelText('Description'), 'Unsaved notes')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Synthetic save failed')
  expect(name).toHaveValue('Unsaved name')
  expect(screen.getByLabelText('Description')).toHaveValue('Unsaved notes')
  expect(screen.getByText('kept-model')).toBeVisible()
  expect(close).not.toHaveBeenCalled()
})

it('an unresolved save disables fields and cancel until the response completes', async () => {
  const user = userEvent.setup()
  client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  let finish: (() => void) | undefined
  vi.spyOn(api, 'post').mockImplementation(async () => {
    await new Promise<void>((resolve) => { finish = resolve })
    return { data: { success: true } }
  })
  render(<QueryClientProvider client={client}><PrefillGroupFormDrawer open currentGroup={null} onClose={() => {}} /></QueryClientProvider>)
  await user.type(screen.getByLabelText('Group Name'), 'Pending selection')
  await user.click(screen.getByRole('button', { name: 'Create' }))
  expect(await screen.findByRole('button', { name: 'Saving...' })).toBeDisabled()
  expect(screen.getByLabelText('Group Name')).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
  finish?.()
  await waitFor(() => expect(screen.getByRole('button', { name: 'Create' })).toBeEnabled())
})
