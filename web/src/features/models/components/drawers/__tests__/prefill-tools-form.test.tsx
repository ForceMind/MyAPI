import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { PrefillGroupFormDrawer } from '../prefill-group-form-drawer'

let client: QueryClient | undefined
afterEach(() => {
  cleanup()
  client?.clear()
  vi.restoreAllMocks()
})

it('switching model to endpoint and back preserves the model draft and submits its array', async () => {
  const user = userEvent.setup()
  client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
  render(
    <QueryClientProvider client={client}>
      <PrefillGroupFormDrawer open currentGroup={{ id: 17, name: 'Chat models', description: 'Original notes', type: 'model', items: ['synthetic-chat'] }} onClose={() => {}} />
    </QueryClientProvider>
  )
  await screen.findByText('synthetic-chat')
  await user.click(screen.getByRole('combobox', { name: 'Group Type' }))
  await user.click(await screen.findByRole('option', { name: /Endpoint Group/ }))
  await user.click(screen.getByRole('combobox', { name: 'Group Type' }))
  await user.click(await screen.findByRole('option', { name: /Model Group/ }))
  expect(screen.getByText('synthetic-chat')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
  expect(put.mock.calls[0]?.[1]).toEqual({ id: 17, name: 'Chat models', description: 'Original notes', type: 'model', items: ['synthetic-chat'] })
})

it('switching model to tag starts a separate tag draft and restores both arrays', async () => {
  const user = userEvent.setup()
  client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
  render(
    <QueryClientProvider client={client}>
      <PrefillGroupFormDrawer open currentGroup={{ id: 18, name: 'Separate drafts', type: 'model', items: ['synthetic-model'] }} onClose={() => {}} />
    </QueryClientProvider>
  )
  await screen.findByText('synthetic-model')
  await user.click(screen.getByRole('combobox', { name: 'Group Type' }))
  await user.click(await screen.findByRole('option', { name: /Tag Group/ }))
  expect(screen.queryByText('synthetic-model')).not.toBeInTheDocument()
  await user.type(screen.getByPlaceholderText('Enter a value and press Enter'), 'synthetic-tag{Enter}')
  await user.click(screen.getByRole('combobox', { name: 'Group Type' }))
  await user.click(await screen.findByRole('option', { name: /Model Group/ }))
  expect(screen.getByText('synthetic-model')).toBeVisible()
  expect(screen.queryByText('synthetic-tag')).not.toBeInTheDocument()
  await user.click(screen.getByRole('combobox', { name: 'Group Type' }))
  await user.click(await screen.findByRole('option', { name: /Tag Group/ }))
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
  expect(put.mock.calls[0]?.[1]).toEqual({ id: 18, name: 'Separate drafts', description: '', type: 'tag', items: ['synthetic-tag'] })
})

it('switching away from endpoint and back preserves the original JSON string including false and zero extensions', async () => {
  const user = userEvent.setup()
  const items = '{"openai":{"path":"/v1/synthetic","method":"POST","extension":{"enabled":false,"count":0}}}'
  client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
  render(
    <QueryClientProvider client={client}>
      <PrefillGroupFormDrawer open currentGroup={{ id: 19, name: 'Endpoint draft', type: 'endpoint', items }} onClose={() => {}} />
    </QueryClientProvider>
  )
  await screen.findByDisplayValue('openai')
  await user.click(screen.getByRole('combobox', { name: 'Group Type' }))
  await user.click(await screen.findByRole('option', { name: /Model Group/ }))
  await user.type(screen.getByPlaceholderText('Enter a value and press Enter'), 'separate-model{Enter}')
  await user.click(screen.getByRole('combobox', { name: 'Group Type' }))
  await user.click(await screen.findByRole('option', { name: /Endpoint Group/ }))
  expect(screen.getByDisplayValue('openai')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
  expect(put.mock.calls[0]?.[1]).toEqual({ id: 19, name: 'Endpoint draft', description: '', type: 'endpoint', items })
})
