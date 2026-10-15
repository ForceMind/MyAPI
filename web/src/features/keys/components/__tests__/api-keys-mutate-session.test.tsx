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

import { ApiKeysMutateDrawer } from '../api-keys-mutate-drawer'
import { ApiKeysProvider } from '../api-keys-provider'

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

async function startBatch() {
  const user = userEvent.setup()
  const onOpenChange = vi.fn()
  const view = render(
    <QueryClientProvider client={client}>
      <ApiKeysProvider>
        <ApiKeysMutateDrawer open onOpenChange={onOpenChange} />
      </ApiKeysProvider>
    </QueryClientProvider>
  )
  const save = await screen.findByRole('button', { name: 'Save changes' })
  await waitFor(() => expect(save).toBeEnabled())
  await user.type(screen.getByRole('textbox', { name: 'Name' }), 'Device')
  const quantity = screen.getByRole('spinbutton', { name: 'Quantity' })
  fireEvent.change(quantity, { target: { value: '2' } })
  await user.click(save)
  await waitFor(() => expect(api.post).toHaveBeenCalledOnce())
  return { ...view, onOpenChange, save }
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

test('a current creation failure remains visible and is never replayed automatically', async () => {
  const pending = deferredCreate()
  vi.mocked(api.post).mockReturnValueOnce(pending.promise)
  const view = await startBatch()
  await act(async () =>
    pending.reject(new Error('synthetic request disconnected'))
  )
  await waitFor(() => expect(view.save).toBeEnabled())
  expect(api.post).toHaveBeenCalledOnce()
  expect(toast.error).toHaveBeenCalledOnce()
  expect(toast.success).not.toHaveBeenCalled()
  expect(view.onOpenChange).not.toHaveBeenCalled()
})
