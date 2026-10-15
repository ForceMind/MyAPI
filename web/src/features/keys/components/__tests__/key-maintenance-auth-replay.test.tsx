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
import axios, {
  AxiosError,
  type AxiosAdapter,
  type AxiosResponse,
  type InternalAxiosRequestConfig,
} from 'axios'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import type { AuthBundle } from '@/stores/auth-store'

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}))

// Both genuine Axios instances (application and auth refresh) inherit this
// transport before they are created. Their interceptors and auth logic stay real.
let transport: AxiosAdapter
const adapter: AxiosAdapter = (config) => transport(config)
const defaultAdapter = axios.defaults.adapter
axios.defaults.adapter = adapter
const { api } = await import('@/lib/api')
const { TooltipProvider } = await import('@/components/ui/tooltip')
const { useAuthStore } = await import('@/stores/auth-store')
const { useSystemConfigStore } = await import('@/stores/system-config-store')
const { fetchTokenKey } = await import('../../api')
const { apiKeySchema } = await import('../../types')
const { ApiKeysMutateDrawer } = await import('../api-keys-mutate-drawer')
const { ApiKeysProvider, useApiKeys } = await import('../api-keys-provider')
const { DataTableRowActions } = await import('../data-table-row-actions')
const { toast } = await import('sonner')
axios.defaults.adapter = defaultAdapter

const originalAuth = useAuthStore.getState()
const originalConfig = useSystemConfigStore.getState()
const key = apiKeySchema.parse({
  id: 21,
  name: 'Synthetic Key',
  key: 'masked',
  status: 1,
  remain_quota: 100,
  used_quota: 0,
  unlimited_quota: true,
  expired_time: -1,
  created_time: 1,
  accessed_time: 1,
  group: 'default',
  model_limits_enabled: false,
})
let client: QueryClient
let target: string
let rejectUnauthorized: () => void
let writes: InternalAxiosRequestConfig[]
let refreshes: InternalAxiosRequestConfig[]

function bundle(sid: string): AuthBundle {
  return {
    access_token: `synthetic-access-${sid}`,
    token_type: 'Bearer',
    access_expires_at: 4_000_000_000,
    user: {
      id: sid === 'original' ? 1 : 2,
      role: 100,
      username: 'synthetic-operator',
    },
    session: {
      sid,
      current: true,
      login_method: 'password',
      ip: '127.0.0.1',
      user_agent: 'test',
      created_at: 1,
      last_active_at: 1,
      expires_at: 4_000_000_000,
    },
  }
}

function response(
  config: InternalAxiosRequestConfig,
  data: unknown
): AxiosResponse {
  return { config, data, status: 200, statusText: 'OK', headers: {} }
}

function BatchReveal() {
  const { resolveRealKeysBatch } = useApiKeys()
  return (
    <button
      onClick={() => {
        void resolveRealKeysBatch([21])
      }}
    >
      Reveal selected Keys
    </button>
  )
}

beforeEach(() => {
  localStorage.clear()
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  useAuthStore.getState().auth.setBundle(bundle('original'))
  writes = []
  refreshes = []
  transport = (config) => {
    if (config.url === '/api/user/auth/refresh') {
      refreshes.push(config)
      return Promise.resolve(
        response(config, { success: true, data: bundle('replacement') })
      )
    }
    if (config.method !== 'get' && config.url === target) {
      writes.push(config)
      if (writes.length === 1)
        return new Promise<AxiosResponse>((_resolve, reject) => {
          rejectUnauthorized = () =>
            reject(
              new AxiosError(
                'Synthetic expired authentication',
                AxiosError.ERR_BAD_REQUEST,
                config,
                undefined,
                {
                  config,
                  data: { success: false },
                  status: 401,
                  statusText: 'Unauthorized',
                  headers: {},
                }
              )
            )
        })
      return Promise.resolve(
        response(config, {
          success: true,
          data: { key: 'synthetic-replayed-secret' },
        })
      )
    }
    let data: unknown
    if (config.url === '/api/status')
      data = {
        success: true,
        data: {
          default_use_auto_group: false,
          quota_display_type: 'TOKENS',
          chats: [],
        },
      }
    else if (config.url === '/api/user/models')
      data = { success: true, data: [] }
    else if (config.url === '/api/user/self/groups')
      data = {
        success: true,
        data: { default: { desc: 'Standard', ratio: 1 } },
      }
    else if (config.url === '/api/token/auto-groups')
      data = { success: true, data: { groups: ['default'], max_count: 5 } }
    else if (config.url === '/api/token/21') data = { success: true, data: key }
    else throw new Error(`Unexpected request ${config.method} ${config.url}`)
    return Promise.resolve(response(config, data))
  }
  vi.spyOn(api, 'post')
  vi.spyOn(api, 'put')
  vi.mocked(toast.error).mockClear()
  vi.mocked(toast.success).mockClear()
})

afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.setState(originalAuth)
  useSystemConfigStore.setState(originalConfig)
  localStorage.clear()
  vi.restoreAllMocks()
})

test.each(['create', 'update', 'status', 'reveal', 'batch reveal'] as const)(
  'a pending %s returning 401 after identity changes never refreshes or replays under the new session',
  async (action) => {
    const user = userEvent.setup()
    const onOpenChange = vi.fn()
    target = '/api/token/'
    if (action === 'status') target = '/api/token/?status_only=true'
    if (action === 'reveal') target = '/api/token/21/key'
    if (action === 'batch reveal') target = '/api/token/batch/keys'
    render(
      <QueryClientProvider client={client}>
        <TooltipProvider>
          <ApiKeysProvider>
            {(action === 'create' || action === 'update') && (
              <ApiKeysMutateDrawer
                open
                currentRow={action === 'update' ? key : undefined}
                onOpenChange={onOpenChange}
              />
            )}
            {(action === 'status' || action === 'reveal') && (
              <DataTableRowActions apiKey={key} />
            )}
            {action === 'batch reveal' && <BatchReveal />}
          </ApiKeysProvider>
        </TooltipProvider>
      </QueryClientProvider>
    )
    if (action === 'create' || action === 'update') {
      const save = await screen.findByRole('button', { name: 'Save changes' })
      await waitFor(() => expect(save).toBeEnabled())
      if (action === 'create') {
        await user.type(
          screen.getByRole('textbox', { name: 'Name' }),
          'Synthetic batch'
        )
        fireEvent.change(screen.getByRole('spinbutton', { name: 'Quantity' }), {
          target: { value: '2' },
        })
      }
      await user.click(save)
    } else if (action === 'status')
      await user.click(screen.getByRole('button', { name: 'Disable' }))
    else if (action === 'reveal')
      await user.click(screen.getByRole('button', { name: 'Open menu' }))
    else
      await user.click(
        screen.getByRole('button', { name: 'Reveal selected Keys' })
      )
    await waitFor(() => expect(writes).toHaveLength(1))
    const method =
      action === 'update' || action === 'status' ? api.put : api.post
    const pending = vi.mocked(method).mock.results[0]
      .value as Promise<AxiosResponse>
    expect(writes[0].headers.get('Authorization')).toBe(
      'Bearer synthetic-access-original'
    )
    await act(async () => {
      useAuthStore.getState().auth.setBundle(bundle('replacement'))
      rejectUnauthorized()
      await pending.catch(() => undefined)
    })
    expect(refreshes).toHaveLength(0)
    expect(writes).toHaveLength(1)
    expect(useAuthStore.getState().auth.session?.sid).toBe('replacement')
    expect(useAuthStore.getState().auth.user?.id).toBe(2)
    expect(onOpenChange).not.toHaveBeenCalled()
    expect(toast.error).not.toHaveBeenCalled()
    expect(toast.success).not.toHaveBeenCalled()
  }
)

test('an existing unscoped reveal retains the normal authentication refresh and replay behavior', async () => {
  target = '/api/token/21/key'
  const pending = fetchTokenKey(21)
  await waitFor(() => expect(writes).toHaveLength(1))
  rejectUnauthorized()
  await expect(pending).resolves.toEqual({
    success: true,
    data: { key: 'synthetic-replayed-secret' },
  })
  expect(refreshes).toHaveLength(1)
  expect(writes).toHaveLength(2)
  expect(writes[1].headers.get('Authorization')).toBe(
    'Bearer synthetic-access-replacement'
  )
})
