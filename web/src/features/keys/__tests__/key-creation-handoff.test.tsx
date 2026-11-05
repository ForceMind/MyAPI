import type { QueryClient } from '@tanstack/react-query'
import { act, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'
import { useInspectorStore } from '@/stores/inspector-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { KEY_21, mockKeysApi, newClient, renderKeysPage, signIn } from './keys-page-fixture'

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }))
const originalAuth = useAuthStore.getState()
const originalConfig = useSystemConfigStore.getState()
let client: QueryClient
beforeEach(() => {
  client = newClient()
  localStorage.clear()
  signIn(100)
  vi.mocked(toast.success).mockClear()
  vi.mocked(toast.error).mockClear()
})
afterEach(() => {
  client.clear()
  useAuthStore.setState(originalAuth)
  useSystemConfigStore.setState(originalConfig)
  useInspectorStore.setState({ current: null })
  localStorage.clear()
  vi.restoreAllMocks()
})

test('an explicit creation action opens exactly the new Key delivery with the complete list return context and no credential read', async () => {
  const keys = [KEY_21]
  const created = { ...KEY_21, id: 31, name: KEY_21.name }
  const mocks = mockKeysApi({
    keys, total: 100, logs: [], status: {},
    extraGet: (path) => path === '/api/token/auto-groups'
      ? { data: { success: true, data: { groups: ['default'], max_count: 5 } } } : undefined,
  })
  mocks.post.mockImplementation(async (url) => {
    if (url !== '/api/token/') throw new Error(`Unexpected credential read ${url}`)
    keys.push(created)
    return { data: { success: true, data: { id: created.id } } }
  })
  const user = userEvent.setup()
  const { router } = await renderKeysPage(client, '/keys?page=3&pageSize=20&filter=saved&status=%5B%221%22%5D')
  await user.click(await screen.findByRole('button', { name: 'Create API Key' }))
  const save = await screen.findByRole('button', { name: 'Save changes' })
  await waitFor(() => expect(save).toBeEnabled())
  await user.type(screen.getByRole('textbox', { name: 'Name' }), KEY_21.name)
  await user.click(save)
  await waitFor(() => expect(toast.success).toHaveBeenCalledOnce())
  expect(router.state.location.pathname).toBe('/keys')
  expect(mocks.post).toHaveBeenCalledOnce()
  const action = vi.mocked(toast.success).mock.calls[0]?.[1]?.action
  expect(action).toMatchObject({ label: 'Configure this Key' })
  if (!action || typeof action !== 'object' || !('onClick' in action) || typeof action.onClick !== 'function') throw new Error('Expected explicit configure action')
  await act(async () => Reflect.apply(action.onClick, undefined, []))
  await screen.findByRole('region', { name: 'Connection details' })
  expect(router.state.location.pathname).toBe('/keys/31')
  expect(router.state.location.search).toMatchObject({ tab: 'delivery', keyReturn: { page: 3, pageSize: 20, filter: 'saved', status: ['1'] } })
  expect(mocks.post).toHaveBeenCalledExactlyOnceWith('/api/token/', expect.objectContaining({ name: KEY_21.name }), expect.objectContaining({ skipAuthRefresh: true }))
  expect(mocks.put).not.toHaveBeenCalled()
  expect(mocks.delete).not.toHaveBeenCalled()
})
