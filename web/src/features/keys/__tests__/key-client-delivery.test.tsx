import type { QueryClient } from '@tanstack/react-query'
import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'
import { useInspectorStore } from '@/stores/inspector-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { KEY_21, KEY_22, mockKeysApi, newClient, renderKeysPage, signIn } from './keys-page-fixture'

const originalAuth = useAuthStore.getState()
const originalConfig = useSystemConfigStore.getState()
let client: QueryClient

beforeEach(() => {
  localStorage.clear()
  client = newClient()
  signIn(100)
  vi.spyOn(window, 'open').mockReturnValue(null)
})
afterEach(() => {
  client.clear()
  useAuthStore.setState(originalAuth)
  useSystemConfigStore.setState(originalConfig)
  useInspectorStore.setState({ current: null })
  vi.restoreAllMocks()
  localStorage.clear()
})

test('delivery offers Key-scoped candidates without selecting a model or reading the secret', async () => {
  const mocks = mockKeysApi({
    keys: [{ ...KEY_21, model_limits_enabled: true, model_limits: 'allowed-model' }],
    logs: [], status: { server_address: 'https://gateway.invalid/root/v1/' },
    extraGet: (path) => path === '/api/user/models'
      ? { data: { success: true, data: ['allowed-model', 'other-model'] } } : undefined,
  })
  const user = userEvent.setup()
  await renderKeysPage(client, '/keys/21?tab=delivery')
  const connection = await screen.findByRole('region', { name: 'Connection details' })
  expect(await within(connection).findByText('https://gateway.invalid/root/v1')).toBeVisible()
  const model = within(connection).getByRole('combobox', { name: 'Candidate model' })
  expect(model).toHaveValue('')
  expect(within(connection).getByRole('button', { name: 'Import to CC Switch' })).toBeDisabled()
  await user.click(model)
  expect(await screen.findByRole('option', { name: 'allowed-model' })).toBeVisible()
  expect(screen.queryByRole('option', { name: 'other-model' })).not.toBeInTheDocument()
  await user.keyboard('{ArrowDown}{Enter}')
  expect(within(connection).getByRole('button', { name: 'Copy model name' })).toBeEnabled()
  expect(mocks.post).not.toHaveBeenCalled()
  const clipboard = vi.spyOn(navigator.clipboard, 'writeText')
  await user.click(within(connection).getByRole('button', { name: 'Copy model name' }))
  expect(clipboard).toHaveBeenCalledWith('allowed-model')
  expect(mocks.post).not.toHaveBeenCalled()
  await user.click(within(connection).getByRole('button', { name: 'Import to CC Switch' }))
  const dialog = await screen.findByRole('dialog', { name: 'Import to CC Switch' })
  expect(within(dialog).getByRole('combobox', { name: 'Primary Model *' })).toHaveValue('allowed-model')
  expect(mocks.post).toHaveBeenCalledExactlyOnceWith('/api/token/21/key', undefined, expect.objectContaining({ skipAuthRefresh: true }))
  expect(window.open).not.toHaveBeenCalled()
  expect(mocks.put).not.toHaveBeenCalled()
})

test('delivery opens the original scope tab while preserving list return context', async () => {
  const mocks = mockKeysApi({ keys: [KEY_21], logs: [], status: {} })
  const user = userEvent.setup()
  const { router } = await renderKeysPage(client, '/keys/21?tab=delivery&keyReturn=%7B%22page%22%3A3%2C%22filter%22%3A%22saved%22%7D')
  await user.click(await screen.findByRole('button', { name: 'View Key restrictions' }))
  expect(await screen.findByRole('region', { name: 'Access' })).toBeVisible()
  expect(router.state.location.search).toMatchObject({ tab: 'scope', keyReturn: { page: 3, filter: 'saved' } })
  await waitFor(() => expect(screen.getByRole('tab', { name: 'Scope and shares' })).toHaveFocus())
  expect(mocks.post).not.toHaveBeenCalled()
  expect(mocks.put).not.toHaveBeenCalled()
})

test('a failed model directory read exposes retry and keeps client export disabled without reading the secret', async () => {
  let failed = true
  const mocks = mockKeysApi({
    keys: [KEY_21], logs: [], status: {},
    extraGet: (path) => path === '/api/user/models'
      ? { data: failed ? { success: false, message: 'Synthetic catalog failed' } : { success: true, data: ['allowed-model'] } } : undefined,
  })
  const user = userEvent.setup()
  await renderKeysPage(client, '/keys/21?tab=delivery')
  const connection = await screen.findByRole('region', { name: 'Connection details' })
  expect(await within(connection).findByRole('alert')).toHaveTextContent('Synthetic catalog failed')
  expect(within(connection).getByRole('button', { name: 'Import to CC Switch' })).toBeDisabled()
  failed = false
  await user.click(within(connection).getByRole('button', { name: 'Retry' }))
  await waitFor(() => expect(within(connection).queryByRole('alert')).not.toBeInTheDocument())
  expect(within(connection).getByRole('combobox', { name: 'Candidate model' })).toHaveValue('')
  expect(mocks.post).not.toHaveBeenCalled()
})

test('a session change removes the previous Key candidate directory and import draft', async () => {
  mockKeysApi({
    keys: [KEY_21], logs: [], status: {},
    extraGet: (path) => path === '/api/user/models'
      ? { data: { success: true, data: ['previous-session-model'] } } : undefined,
  })
  const user = userEvent.setup()
  await renderKeysPage(client, '/keys/21?tab=delivery')
  await user.click(await screen.findByRole('combobox', { name: 'Candidate model' }))
  await screen.findByRole('option', { name: 'previous-session-model' })
  await user.keyboard('{ArrowDown}{Enter}')
  await user.click(screen.getByRole('button', { name: 'Import to CC Switch' }))
  await screen.findByRole('dialog', { name: 'Import to CC Switch' })
  act(() => useAuthStore.setState((state) => ({ ...state, auth: { ...state.auth, user: null } })))
  expect(screen.queryByRole('dialog', { name: 'Import to CC Switch' })).not.toBeInTheDocument()
  expect(screen.queryByDisplayValue('previous-session-model')).not.toBeInTheDocument()
  expect(window.open).not.toHaveBeenCalled()
})


test('opening another Key removes the previous secret-bearing import and does not carry its model selection', async () => {
  const mocks = mockKeysApi({
    keys: [
      { ...KEY_21, model_limits_enabled: true, model_limits: 'first-model' },
      { ...KEY_22, model_limits_enabled: true, model_limits: 'next-model' },
    ], logs: [], status: {},
    extraGet: (path) => path === '/api/user/models'
      ? { data: { success: true, data: ['first-model', 'next-model'] } } : undefined,
  })
  const user = userEvent.setup()
  const { router } = await renderKeysPage(client, '/keys/21?tab=delivery')
  await user.click(await screen.findByRole('combobox', { name: 'Candidate model' }))
  await screen.findByRole('option', { name: 'first-model' })
  await user.keyboard('{ArrowDown}{Enter}')
  await user.click(screen.getByRole('button', { name: 'Import to CC Switch' }))
  await screen.findByRole('dialog', { name: 'Import to CC Switch' })
  await act(async () => router.navigate({ to: '/keys/$keyId', params: { keyId: '22' }, search: { tab: 'delivery' } }))
  expect(screen.queryByRole('dialog', { name: 'Import to CC Switch' })).not.toBeInTheDocument()
  const model = await screen.findByRole('combobox', { name: 'Candidate model' })
  expect(model).toHaveValue('')
  await user.click(model)
  expect(await screen.findByRole('option', { name: 'next-model' })).toBeVisible()
  expect(screen.queryByRole('option', { name: 'first-model' })).not.toBeInTheDocument()
  expect(mocks.post).toHaveBeenCalledOnce()
  expect(window.open).not.toHaveBeenCalled()
})

test('CC Switch exports the displayed server address when the public status storage stays stale', async () => {
  localStorage.setItem('status', JSON.stringify({ server_address: 'https://stale.gateway.invalid' }))
  const write = Storage.prototype.setItem
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(function (this: Storage, name, value) {
    if (name === 'status') throw new Error('Synthetic status storage unavailable')
    write.call(this, name, value)
  })
  mockKeysApi({
    keys: [KEY_21], logs: [], status: { server_address: 'https://current.gateway.invalid/root/v1///' },
    extraGet: (path) => path === '/api/user/models'
      ? { data: { success: true, data: ['selected-model'] } } : undefined,
  })
  const user = userEvent.setup()
  await renderKeysPage(client, '/keys/21?tab=delivery')
  expect(await screen.findByText('https://current.gateway.invalid/root/v1')).toBeVisible()
  await user.click(screen.getByRole('combobox', { name: 'Candidate model' }))
  await screen.findByRole('option', { name: 'selected-model' })
  await user.keyboard('{ArrowDown}{Enter}')
  await user.click(screen.getByRole('button', { name: 'Import to CC Switch' }))
  const dialog = await screen.findByRole('dialog', { name: 'Import to CC Switch' })
  expect(within(dialog).getByText('https://current.gateway.invalid/root')).toBeVisible()
  await user.click(within(dialog).getByRole('button', { name: 'Open CC Switch' }))
  const target = vi.mocked(window.open).mock.calls[0]?.[0]
  if (typeof target !== 'string') throw new Error('Expected a synthetic CC Switch URL')
  expect(new URL(target).searchParams.get('endpoint')).toBe('https://current.gateway.invalid/root')
  expect(new URL(target).searchParams.get('homepage')).toBe('https://current.gateway.invalid/root')
})

test.each(['failure', 'removed'] as const)('a catalog refresh %s preserves the open CC Switch application, name and manual model draft', async (refresh) => {
  let refreshed = false
  const mocks = mockKeysApi({
    keys: [KEY_21], logs: [], status: {},
    extraGet: (path) => {
      if (path !== '/api/user/models') return undefined
      if (!refreshed) return { data: { success: true, data: ['selected-model'] } }
      if (refresh === 'failure') return { data: { success: false, message: 'Synthetic refreshed catalog failed' } }
      return { data: { success: true, data: [] } }
    },
  })
  const user = userEvent.setup()
  await renderKeysPage(client, '/keys/21?tab=delivery')
  await user.click(await screen.findByRole('combobox', { name: 'Candidate model' }))
  await screen.findByRole('option', { name: 'selected-model' })
  await user.keyboard('{ArrowDown}{Enter}')
  await user.click(screen.getByRole('button', { name: 'Import to CC Switch' }))
  const dialog = await screen.findByRole('dialog', { name: 'Import to CC Switch' })
  await user.click(within(dialog).getByRole('radio', { name: 'Codex' }))
  const name = within(dialog).getByRole('combobox', { name: 'Name' })
  await user.clear(name)
  await user.type(name, 'My manual client draft')
  const primary = within(dialog).getByRole('combobox', { name: 'Primary Model *' })
  await user.type(primary, 'manual-model-id')
  await user.keyboard('{Escape}')
  refreshed = true
  await act(async () => client.invalidateQueries({ queryKey: ['user-models-key-client'] }))
  if (refresh === 'failure') {
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('Synthetic refreshed catalog failed')
    expect(within(dialog).getByRole('button', { name: 'Retry' })).toBeEnabled()
  } else {
    expect(within(dialog).queryByRole('alert')).not.toBeInTheDocument()
  }
  expect(within(dialog).getByRole('radio', { name: 'Codex' })).toBeChecked()
  expect(name).toHaveValue('My manual client draft')
  expect(primary).toHaveValue('manual-model-id')
  expect(within(dialog).getByRole('button', { name: 'Open CC Switch' })).toBeEnabled()
  expect(mocks.post).toHaveBeenCalledOnce()
  expect(mocks.put).not.toHaveBeenCalled()
  expect(window.open).not.toHaveBeenCalled()
})


test.each(['', null])('delivery treats enabled empty or null model restriction %s as no allowed models', async (modelLimits) => {
  const mocks = mockKeysApi({
    keys: [{ ...KEY_21, model_limits_enabled: true, model_limits: modelLimits }],
    logs: [], status: {},
    extraGet: (path) => path === '/api/user/models'
      ? { data: { success: true, data: ['catalog-model'] } } : undefined,
  })
  await renderKeysPage(client, '/keys/21?tab=delivery')
  const connection = await screen.findByRole('region', { name: 'Connection details' })
  expect(within(connection).getByText('Model restriction enabled: no models allowed')).toBeVisible()
  expect(await within(connection).findByText('No candidate models for this Key')).toBeVisible()
  expect(within(connection).getByRole('button', { name: 'Import to CC Switch' })).toBeDisabled()
  expect(mocks.post).not.toHaveBeenCalled()
  expect(mocks.put).not.toHaveBeenCalled()
})
