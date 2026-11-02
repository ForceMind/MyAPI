import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { act, cleanup, render, renderHook, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import axios, { AxiosError, type AxiosAdapter, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios'
import i18next from 'i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { AuthBundle } from '@/stores/auth-store'

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }))
let transport: AxiosAdapter
const adapter: AxiosAdapter = (config) => transport(config)
const originalAdapter = axios.defaults.adapter
axios.defaults.adapter = adapter
const { useAuthStore } = await import('@/stores/auth-store')
const { useSystemConfigStore } = await import('@/stores/system-config-store')
const { useChannelUpstreamUpdates } = await import('../hooks/use-channel-upstream-updates')
const { ApiKeysProvider } = await import('@/features/keys/components/api-keys-provider')
const { KeyActions } = await import('@/features/keys/components/key-page/key-actions')
const { apiKeySchema } = await import('@/features/keys/types')
const { SidebarModulesCard } = await import('@/features/profile/components/sidebar-modules-card')
const { useInterfaceLanguage } = await import('@/components/language-switcher')
axios.defaults.adapter = originalAdapter

const originalAuth = useAuthStore.getState()
const originalConfig = useSystemConfigStore.getState()
const clients: QueryClient[] = []
const key = apiKeySchema.parse({ id: 21, name: 'Synthetic Key', key: 'masked', status: 1, remain_quota: 100, used_quota: 0, unlimited_quota: false, expired_time: -1, created_time: 1, accessed_time: 1, group: 'default', model_limits_enabled: false })
let dispatched: InternalAxiosRequestConfig[]
let refreshes: InternalAxiosRequestConfig[]
let failureStatus: number
let target: string

function bundle(token: string): AuthBundle {
  return {
    access_token: token, token_type: 'Bearer', access_expires_at: 4000000000,
    user: { id: 1, role: 100, username: 'synthetic-maintainer' },
    session: { sid: 'synthetic-session', current: true, login_method: 'password', ip: '192.0.2.1', user_agent: 'test', created_at: 1, last_active_at: 1, expires_at: 4000000000 },
  }
}
function response(config: InternalAxiosRequestConfig, data: unknown): AxiosResponse {
  return { config, data, status: 200, statusText: 'OK', headers: {} }
}
function firstDispatch() {
  const request = dispatched[0]
  if (!request) throw new Error('Expected UI HTTP dispatch')
  return request
}
function requestBody(config: InternalAxiosRequestConfig) {
  return typeof config.data === 'string' ? JSON.parse(config.data) : config.data
}
function newClient() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  clients.push(client)
  return client
}
async function renderKeyActions() {
  const client = newClient()
  const root = createRootRoute({ component: () => (
    <ApiKeysProvider><KeyActions apiKey={key} onRefetch={() => undefined} /></ApiKeysProvider>
  ) })
  const router = createRouter({ routeTree: root, history: createMemoryHistory({ initialEntries: ['/'] }) })
  await router.load()
  render(<QueryClientProvider client={client}><RouterProvider router={router} /></QueryClientProvider>)
}

beforeEach(async () => {
  localStorage.clear()
  await i18next.changeLanguage('en')
  useAuthStore.getState().auth.setBundle(bundle('synthetic-original-access'))
  dispatched = []
  refreshes = []
  failureStatus = 401
  transport = async (config) => {
    if (config.url === '/api/user/auth/refresh') {
      refreshes.push(config)
      return response(config, { success: true, data: bundle('synthetic-refreshed-access') })
    }
    if (config.url === '/api/status') return response(config, { success: true, data: {} })
    if (config.url !== target) throw new Error(`Unexpected synthetic UI request ${config.method} ${config.url}`)
    dispatched.push(config)
    if (dispatched.length === 1 && failureStatus !== 0) {
      throw new AxiosError('Synthetic rejected UI action', AxiosError.ERR_BAD_REQUEST, config, undefined, {
        config, data: { success: false }, status: failureStatus, statusText: 'Rejected', headers: {},
      })
    }
    return response(config, { success: true, data: { key: 'synthetic-only-secret', processed_channels: 1, added_models: 1, removed_models: 0 } })
  }
})
afterEach(async () => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.setState(originalAuth)
  useSystemConfigStore.setState(originalConfig)
  localStorage.clear()
  await i18next.changeLanguage('en')
  vi.restoreAllMocks()
})

const hookCases = [
  { name: 'apply staged updates to all channels', url: '/api/channel/upstream_updates/apply_all', body: {}, run: (hook: ReturnType<typeof useChannelUpstreamUpdates>) => hook.applyAllUpdates() },
  { name: 'detect selected upstream updates', url: '/api/channel/upstream_updates/detect', body: { id: 17 }, run: (hook: ReturnType<typeof useChannelUpstreamUpdates>) => hook.detectChannelUpdates({ id: 17 }) },
  { name: 'queue upstream detection', url: '/api/channel/upstream_updates/detect_all', body: {}, run: (hook: ReturnType<typeof useChannelUpstreamUpdates>) => hook.detectAllUpdates() },
]
for (const scenario of hookCases) {
  describe(scenario.name, () => {
    it.each([401, 403, 0])('dispatches once and preserves payload on status %s', async (status) => {
      target = scenario.url
      failureStatus = status
      const reload = vi.fn().mockResolvedValue(undefined)
      const { result } = renderHook(() => useChannelUpstreamUpdates(reload))
      await act(async () => { await scenario.run(result.current) })
      expect(dispatched).toHaveLength(1)
      expect(refreshes).toHaveLength(0)
      expect(firstDispatch().method).toBe('post')
      expect(requestBody(firstDispatch())).toEqual(scenario.body)
      expect(reload).toHaveBeenCalledTimes(status === 0 ? 1 : 0)
    })
  })
}

it.each([401, 403, 0])('Key page status action preserves exact target and unknown recovery on status %s', async (status) => {
  target = '/api/token/?status_only=true'
  failureStatus = status
  const user = userEvent.setup()
  await renderKeyActions()
  const pause = await screen.findByRole('button', { name: 'Pause Key' })
  await user.click(pause)
  await waitFor(() => expect(pause).toHaveAttribute('aria-busy', 'false'))
  expect(dispatched).toHaveLength(1)
  expect(refreshes).toHaveLength(0)
  expect(firstDispatch().method).toBe('put')
  expect(requestBody(firstDispatch())).toEqual({ id: 21, status: 2 })
  if (status === 0) expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  else {
    expect(await screen.findByRole('alert')).toHaveTextContent('Failed to update API key status')
    expect(pause).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Reload current state' })).toBeVisible()
  }
})
it.each([401, 403, 0])('Key page copy dispatches once and copies only a confirmed response on status %s', async (status) => {
  target = '/api/token/21/key'
  failureStatus = status
  const user = userEvent.setup()
  const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined)
  await renderKeyActions()
  const button = await screen.findByRole('button', { name: 'Copy Key' })
  await user.click(button)
  await waitFor(() => expect(button).toHaveAttribute('aria-busy', 'false'))
  expect(dispatched).toHaveLength(1)
  expect(refreshes).toHaveLength(0)
  expect(firstDispatch().method).toBe('post')
  expect(requestBody(firstDispatch())).toBeUndefined()
  if (status === 0) expect(copy).toHaveBeenCalledExactlyOnceWith('sk-synthetic-only-secret')
  else expect(copy).not.toHaveBeenCalled()
})
it.each([401, 403, 0])('Profile module save retains its original serialized configuration on status %s', async (status) => {
  target = '/api/user/self'
  failureStatus = status
  const config = { chat: { enabled: true, playground: false, chat: true } }
  const serialized = JSON.stringify(config)
  const user = userEvent.setup()
  render(<SidebarModulesCard sidebarModules={serialized} />)
  const save = screen.getByRole('button', { name: 'Save Changes' })
  await user.click(save)
  await waitFor(() => expect(save).toBeEnabled())
  expect(dispatched).toHaveLength(1)
  expect(refreshes).toHaveLength(0)
  expect(firstDispatch().method).toBe('put')
  expect(requestBody(firstDispatch())).toEqual({ sidebar_modules: serialized })
  expect(useAuthStore.getState().auth.user?.sidebar_modules).toBe(status === 0 ? serialized : undefined)
})
it.each([401, 403, 0])('interface language action keeps local selection without replaying persistence on status %s', async (status) => {
  target = '/api/user/self'
  failureStatus = status
  const { result } = renderHook(() => useInterfaceLanguage())
  await act(async () => { await result.current.changeLanguage('zhCN') })
  expect(i18next.language).toBe('zhCN')
  expect(dispatched).toHaveLength(1)
  expect(refreshes).toHaveLength(0)
  expect(firstDispatch().method).toBe('put')
  expect(requestBody(firstDispatch())).toEqual({ language: 'zhCN' })
})
