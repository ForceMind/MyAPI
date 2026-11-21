/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { KEY_21, renderKeysPage } from '../../__tests__/keys-page-fixture'
import { ApiKeysMutateDrawer } from '../api-keys-mutate-drawer'
import { ApiKeysProvider, useApiKeys } from '../api-keys-provider'

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))
const { toast } = await import('sonner')
const originalAuth = useAuthStore.getState()
const originalConfig = useSystemConfigStore.getState()
let client: QueryClient
type CreateResponse = {
  data: { success: boolean; data: Record<string, unknown> }
}

function deferredCreate() {
  let resolve!: (response: CreateResponse) => void
  let reject!: (error: Error) => void
  const promise = new Promise<CreateResponse>((accept, deny) => {
    resolve = accept
    reject = deny
  })
  return { promise, resolve, reject }
}

function changeIdentity(kind: 'user' | 'role' | 'session') {
  useAuthStore.setState((state) => ({
    ...state,
    auth: {
      ...state.auth,
      user: {
        id: kind === 'user' ? 2 : 1,
        role: kind === 'role' ? 1 : 100,
        username: 'maintenance-operator',
      },
      session: state.auth.session && {
        ...state.auth.session,
        sid: kind === 'session' ? 'new-session' : state.auth.session.sid,
      },
    },
  }))
}

function KeyListRefresh() {
  const { refreshTrigger } = useApiKeys()
  return <output data-testid='key-list-refresh'>{refreshTrigger}</output>
}

async function startBatch(
  configureKey?: (id: number) => void,
  count = 2,
  reviewSavedKeys = vi.fn<(name: string) => Promise<void>>().mockResolvedValue(undefined)
) {
  const user = userEvent.setup()
  const onOpenChange = vi.fn()
  const drawer = (open: boolean) => (
    <QueryClientProvider client={client}>
      <ApiKeysProvider configureKey={configureKey}>
        <KeyListRefresh />
        <ApiKeysMutateDrawer open={open} onOpenChange={onOpenChange} onReviewSavedKeys={reviewSavedKeys} />
      </ApiKeysProvider>
    </QueryClientProvider>
  )
  const view = render(drawer(true))
  const save = await screen.findByRole('button', { name: 'Save changes' })
  await waitFor(() => expect(save).toBeEnabled())
  await user.type(screen.getByRole('textbox', { name: 'Name' }), 'Device')
  const quantity = screen.getByRole('spinbutton', { name: 'Quantity' })
  fireEvent.change(quantity, { target: { value: String(count) } })
  await user.click(save)
  await waitFor(() => expect(api.post).toHaveBeenCalledOnce())
  return { ...view, onOpenChange, save, setOpen: (open: boolean) => view.rerender(drawer(open)) }
}

beforeEach(() => {
  localStorage.clear()
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  useAuthStore.setState((state) => ({
    ...state,
    auth: {
      ...state.auth,
      user: { id: 1, role: 100, username: 'maintenance-operator' },
      session: {
        sid: 'original-session',
        current: true,
        login_method: 'password',
        ip: '',
        user_agent: '',
        created_at: 1,
        last_active_at: 1,
        expires_at: 2,
      },
    },
  }))
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/status') {
      return {
        data: {
          success: true,
          data: { default_use_auto_group: false, quota_display_type: 'TOKENS' },
        },
      }
    }
    if (url === '/api/user/models') return { data: { success: true, data: [] } }
    if (url === '/api/user/self/groups') {
      return {
        data: {
          success: true,
          data: { default: { desc: 'Standard', ratio: 1 } },
        },
      }
    }
    if (url === '/api/token/auto-groups') {
      return {
        data: { success: true, data: { groups: ['default'], max_count: 5 } },
      }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true, data: {} } })
  vi.mocked(toast.success).mockClear()
  vi.mocked(toast.error).mockClear()
})

afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.setState(originalAuth)
  useSystemConfigStore.setState(originalConfig)
  localStorage.clear()
  vi.restoreAllMocks()
})

test.each(['unmount', 'user', 'role', 'session'] as const)(
  'a batch stops after its in-flight creation when the original %s changes',
  async (change) => {
    const pending = deferredCreate()
    vi.mocked(api.post).mockReturnValueOnce(pending.promise)
    const view = await startBatch()
    await act(async () => {
      if (change === 'unmount') view.unmount()
      else changeIdentity(change)
      pending.resolve({ data: { success: true, data: {} } })
    })
    expect(api.post).toHaveBeenCalledOnce()
    expect(view.onOpenChange).not.toHaveBeenCalled()
    expect(toast.success).not.toHaveBeenCalled()
    expect(toast.error).not.toHaveBeenCalled()
  }
)

test.each(['unmount', 'session'] as const)(
  'a delayed creation failure after %s produces no stale notification or further write',
  async (change) => {
    const pending = deferredCreate()
    vi.mocked(api.post).mockReturnValueOnce(pending.promise)
    const view = await startBatch()
    await act(async () => {
      if (change === 'unmount') view.unmount()
      else changeIdentity(change)
      pending.reject(new Error('synthetic request disconnected'))
    })
    expect(api.post).toHaveBeenCalledOnce()
    expect(view.onOpenChange).not.toHaveBeenCalled()
    expect(toast.success).not.toHaveBeenCalled()
    expect(toast.error).not.toHaveBeenCalled()
  }
)

test('a current session completes the requested two-Key batch and closes once', async () => {
  const pending = deferredCreate()
  vi.mocked(api.post).mockReturnValueOnce(pending.promise)
  const view = await startBatch()
  await act(async () => pending.resolve({ data: { success: true, data: {} } }))
  await waitFor(() =>
    expect(view.onOpenChange).toHaveBeenCalledExactlyOnceWith(false)
  )
  expect(api.post).toHaveBeenCalledTimes(2)
  expect(toast.success).toHaveBeenCalledOnce()
  expect(toast.error).not.toHaveBeenCalled()
})

test('an uncertain creation stays visible and cannot be replayed from the original form', async () => {
  const pending = deferredCreate()
  vi.mocked(api.post).mockReturnValueOnce(pending.promise)
  const view = await startBatch()
  await act(async () =>
    pending.reject(new Error('synthetic request disconnected'))
  )
  expect(await screen.findByText('Key creation result unknown')).toBeVisible()
  expect(screen.getByText('Check whether the Key named Device is in the saved list.')).toBeVisible()
  await waitFor(() => expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled())
  fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
  expect(api.post).toHaveBeenCalledOnce()
  expect(toast.error).toHaveBeenCalledOnce()
  expect(toast.success).not.toHaveBeenCalled()
  expect(view.onOpenChange).not.toHaveBeenCalled()
})

test('an interrupted batch keeps its confirmed Key and makes review a separate read action', async () => {
  const first = deferredCreate()
  const unknown = deferredCreate()
  vi.mocked(api.post).mockReturnValueOnce(first.promise).mockReturnValueOnce(unknown.promise)
  const configure = vi.fn()
  const review = vi.fn<(name: string) => Promise<void>>().mockResolvedValue(undefined)
  const view = await startBatch(configure, 3, review)
  await act(async () => first.resolve({ data: { success: true, data: { id: 31 } } }))
  await waitFor(() => expect(api.post).toHaveBeenCalledTimes(2))
  await act(async () => unknown.reject(new Error('synthetic connection loss')))
  expect(await screen.findByText('1 of 3 Keys confirmed')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Configure Key #31' })).toBeEnabled()
  expect(screen.queryByRole('button', { name: 'Start a new Key' })).not.toBeInTheDocument()
  expect(screen.getByTestId('key-list-refresh')).toHaveTextContent('0')

  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Review saved Keys' }))
  expect(review).toHaveBeenCalledOnce()
  expect(api.post).toHaveBeenNthCalledWith(2, '/api/token/', expect.objectContaining({ name: review.mock.calls[0][0] }), expect.any(Object))
  expect(view.onOpenChange).toHaveBeenCalledWith(false)
  expect(screen.getByTestId('key-list-refresh')).toHaveTextContent('1')
  expect(configure).not.toHaveBeenCalled()
  expect(api.post).toHaveBeenCalledTimes(2)
  view.setOpen(false)
  view.setOpen(true)
  expect(await screen.findByText('1 of 3 Keys confirmed')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Start a new Key' }))
  await waitFor(() => expect(screen.getByRole('button', { name: 'Save changes' })).toBeEnabled())
  expect(screen.getByRole('textbox', { name: 'Name' })).toHaveValue('')
  expect(api.post).toHaveBeenCalledTimes(2)
})

test('review opens the literal creation name on the first page and keeps the old list in history', async () => {
  const user = userEvent.setup()
  const existingGet = vi.mocked(api.get).getMockImplementation()
  const name = 'Device%_!'
  const readRequests: URLSearchParams[] = []
  vi.mocked(api.get).mockImplementation(async (url, config) => {
    if (!url.startsWith('/api/token/search?')) {
      if (!existingGet) throw new Error('Expected the original API fixture')
      return existingGet(url, config)
    }
    const query = new URLSearchParams(url.split('?')[1])
    readRequests.push(query)
    const items = query.get('keyword') === name && query.get('exact_name') === 'true' &&
      query.get('p') === '1' && !query.has('token')
      ? [{ ...KEY_21, id: 41, name }]
      : []
    return { data: { success: true, data: { items, total: items.length || 100 } } }
  })
  vi.mocked(api.post).mockRejectedValueOnce(new Error('synthetic response lost after save'))
  const previous = { filter: 'old', token: 'old-token-filter', status: ['2'], page: 3, pageSize: 25 }
  const { router } = await renderKeysPage(
    client,
    '/keys?filter=old&token=old-token-filter&status=%5B%222%22%5D&page=3&pageSize=25'
  )
  await waitFor(() => expect(readRequests[0]?.get('p')).toBe('3'))
  await user.click(await screen.findByRole('button', { name: 'Create API Key' }))
  const save = await screen.findByRole('button', { name: 'Save changes' })
  await waitFor(() => expect(save).toBeEnabled())
  await user.type(screen.getByRole('textbox', { name: 'Name' }), name)
  await user.click(save)
  const review = await screen.findByRole('button', { name: 'Review saved Keys' })
  expect(review).toBeEnabled()
  expect(api.post).toHaveBeenCalledExactlyOnceWith('/api/token/', expect.objectContaining({ name }), expect.any(Object))
  await user.click(review)
  await waitFor(() => expect(router.state.location.search).toEqual({
    filter: name, exactName: true, pageSize: 25,
  }))
  await waitFor(() => expect(readRequests.map((query) => query.toString()))
    .toContain('keyword=Device%25_%21&exact_name=true&p=1&size=25'))
  expect(await screen.findByText(name)).toBeVisible()
  expect(api.post).toHaveBeenCalledOnce()

  act(() => router.history.back())
  await waitFor(() => expect(router.state.location.search).toEqual(previous))
  expect(api.post).toHaveBeenCalledOnce()
})

test('an explicitly rejected first write still permits correcting the draft', async () => {
  const pending = deferredCreate()
  vi.mocked(api.post).mockReturnValueOnce(pending.promise)
  const view = await startBatch(undefined, 1)
  await act(async () => pending.resolve({ data: { success: false, data: {} } }))
  await waitFor(() => expect(screen.getByRole('button', { name: 'Save changes' })).toBeEnabled())
  expect(screen.queryByText('Key creation result unknown')).not.toBeInTheDocument()
  expect(screen.getByRole('textbox', { name: 'Name' })).toHaveValue('Device')
  expect(view.onOpenChange).not.toHaveBeenCalled()
  expect(api.post).toHaveBeenCalledTimes(1)
})


test('a returned created ID adds an explicit configure action without reading a credential or navigating automatically', async () => {
  const pending = deferredCreate()
  vi.mocked(api.post).mockReturnValueOnce(pending.promise)
  const configure = vi.fn()
  const view = await startBatch(configure, 1)
  await act(async () => pending.resolve({ data: { success: true, data: { id: 31 } } }))
  await waitFor(() => expect(view.onOpenChange).toHaveBeenCalledExactlyOnceWith(false))
  expect(configure).not.toHaveBeenCalled()
  const options = vi.mocked(toast.success).mock.calls[0]?.[1]
  expect(options).toMatchObject({ action: { label: 'Configure this Key', onClick: expect.any(Function) } })
  const action = options?.action
  if (!action || typeof action !== 'object' || !('onClick' in action) || typeof action.onClick !== 'function') throw new Error('Expected explicit configure action')
  Reflect.apply(action.onClick, undefined, [])
  expect(configure).toHaveBeenCalledExactlyOnceWith(31)
  expect(api.post).toHaveBeenCalledOnce()
})

test.each(['unmount', 'user', 'role', 'session'] as const)('the created-Key action cannot navigate after its original %s changes', async (change) => {
  const pending = deferredCreate()
  vi.mocked(api.post).mockReturnValueOnce(pending.promise)
  const configure = vi.fn()
  const view = await startBatch(configure, 1)
  await act(async () => pending.resolve({ data: { success: true, data: { id: 31 } } }))
  await waitFor(() => expect(toast.success).toHaveBeenCalledOnce())
  const action = vi.mocked(toast.success).mock.calls[0]?.[1]?.action
  if (!action || typeof action !== 'object' || !('onClick' in action) || typeof action.onClick !== 'function') throw new Error('Expected explicit configure action')
  act(() => { if (change === 'unmount') view.unmount(); else changeIdentity(change) })
  Reflect.apply(action.onClick, undefined, [])
  expect(configure).not.toHaveBeenCalled()
  expect(api.post).toHaveBeenCalledOnce()
})

test('a batch action identifies its exact last confirmed ID instead of presenting it as all created Keys', async () => {
  const pending = deferredCreate()
  vi.mocked(api.post).mockReturnValueOnce(pending.promise).mockResolvedValueOnce({ data: { success: true, data: { id: 32 } } })
  const configure = vi.fn()
  const view = await startBatch(configure)
  await act(async () => pending.resolve({ data: { success: true, data: { id: 31 } } }))
  await waitFor(() => expect(view.onOpenChange).toHaveBeenCalledExactlyOnceWith(false))
  expect(vi.mocked(toast.success).mock.calls[0]?.[1]).toMatchObject({ action: { label: 'Configure Key #32' } })
  expect(configure).not.toHaveBeenCalled()
  expect(api.post).toHaveBeenCalledTimes(2)
})

test('an older server without a created ID preserves success, close and no navigation without guessing a Key', async () => {
  const pending = deferredCreate()
  vi.mocked(api.post).mockReturnValueOnce(pending.promise)
  const configure = vi.fn()
  const view = await startBatch(configure, 1)
  await act(async () => pending.resolve({ data: { success: true, data: {} } }))
  await waitFor(() => expect(view.onOpenChange).toHaveBeenCalledExactlyOnceWith(false))
  expect(toast.success).toHaveBeenCalledOnce()
  expect(vi.mocked(toast.success).mock.calls[0]?.[1]?.action).toBeUndefined()
  expect(toast.error).not.toHaveBeenCalled()
  expect(configure).not.toHaveBeenCalled()
  expect(api.post).toHaveBeenCalledOnce()
})
