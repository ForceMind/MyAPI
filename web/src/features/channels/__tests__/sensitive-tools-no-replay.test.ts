import axios, { AxiosError, type AxiosAdapter, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { AuthBundle } from '@/stores/auth-store'

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }))
// Keep both real Axios instances and their interceptors; replace only transport.
let transport: AxiosAdapter
const adapter: AxiosAdapter = (config) => transport(config)
const defaultAdapter = axios.defaults.adapter
axios.defaults.adapter = adapter
const { useAuthStore } = await import('@/stores/auth-store')
const { createChannel, updateChannel, startCodexOAuth, completeCodexOAuth, manageMultiKeys } = await import('../api')
axios.defaults.adapter = defaultAdapter

const originalAuth = useAuthStore.getState()
let writes: InternalAxiosRequestConfig[]
let refreshes: InternalAxiosRequestConfig[]
let failureStatus: number
let target: string

function bundle(token: string): AuthBundle {
  return { access_token: token, token_type: 'Bearer', access_expires_at: 4_000_000_000,
    user: { id: 1, role: 100, username: 'synthetic-maintainer' },
    session: { sid: 'synthetic-same-session', current: true, login_method: 'password', ip: '127.0.0.1', user_agent: 'test', created_at: 1, last_active_at: 1, expires_at: 4_000_000_000 } }
}
function response(config: InternalAxiosRequestConfig, data: unknown): AxiosResponse {
  return { config, data, status: 200, statusText: 'OK', headers: {} }
}
const cases = [
  { name: 'create channel', method: 'post', url: '/api/channel', body: { mode: 'single', channel: { name: 'Synthetic channel', type: 57, key: 'synthetic-placeholder' } }, run: () => createChannel({ mode: 'single', channel: { name: 'Synthetic channel', type: 57, key: 'synthetic-placeholder' } }, 'synthetic-stable-operation-key') },
  { name: 'update channel', method: 'put', url: '/api/channel/', body: { id: 17, name: 'Synthetic channel', weight: 0, auto_ban: 0 }, run: () => updateChannel(17, { name: 'Synthetic channel', weight: 0, auto_ban: 0 }) },
  { name: 'start new OAuth task', method: 'post', url: '/api/channel/codex/oauth/start', body: {}, run: () => startCodexOAuth() },
  { name: 'start selected channel OAuth task', method: 'post', url: '/api/channel/17/codex/oauth/start', body: {}, run: () => startCodexOAuth(17) },
  { name: 'complete new OAuth task', method: 'post', url: '/api/channel/codex/oauth/complete', body: { input: 'synthetic-callback', create: { mode: 'single', channel: { name: 'Synthetic OAuth channel', type: 57 } } }, run: () => completeCodexOAuth('synthetic-callback', undefined, { mode: 'single', channel: { name: 'Synthetic OAuth channel', type: 57 } }) },
  { name: 'complete selected channel OAuth task', method: 'post', url: '/api/channel/17/codex/oauth/complete', body: { input: 'synthetic-callback' }, run: () => completeCodexOAuth('synthetic-callback', 17) },
  ...(['disable_key', 'enable_key', 'enable_all_keys', 'disable_all_keys', 'delete_key', 'delete_disabled_keys'] as const).map((action) => ({ name: action, method: 'post', url: '/api/channel/multi_key/manage', body: { channel_id: 17, action, key_index: 0 }, run: () => manageMultiKeys({ channel_id: 17, action, key_index: 0 }) })),
]


beforeEach(() => {
  localStorage.clear()
  writes = []
  refreshes = []
  failureStatus = 401
  useAuthStore.getState().auth.setBundle(bundle('synthetic-original-access'))
  transport = async (config) => {
    if (config.url === '/api/user/auth/refresh') {
      refreshes.push(config)
      return response(config, { success: true, data: bundle('synthetic-refreshed-access') })
    }
    if (config.url !== target || config.method === 'get') throw new Error(`Unexpected request ${config.method} ${config.url}`)
    writes.push(config)
    if (writes.length === 1 && failureStatus !== 0) {
      throw new AxiosError('Synthetic rejected write', AxiosError.ERR_BAD_REQUEST, config, undefined, { config, data: { success: false }, status: failureStatus, statusText: 'Rejected', headers: {} })
    }
    return response(config, { success: true })
  }
})
afterEach(() => { useAuthStore.setState(originalAuth); localStorage.clear(); vi.restoreAllMocks() })

describe('sensitive channel writes do not replay', () => {
  it.each(cases)('$name rejects 401 after one dispatch without authentication refresh or write replay', async (testCase) => {
    target = testCase.url
    await expect(testCase.run()).rejects.toMatchObject({ response: { status: 401 } })
    expect(writes).toHaveLength(1)
    expect(refreshes).toHaveLength(0)
    expect(writes[0].method).toBe(testCase.method)
    if (testCase.name === 'create channel') expect(writes[0].headers.get('Idempotency-Key')).toBe('synthetic-stable-operation-key')
    expect(writes[0].data === undefined ? undefined : JSON.parse(writes[0].data)).toEqual(testCase.body)
  })
  it.each(cases)('$name rejects 403 once without replay', async (testCase) => {
    target = testCase.url
    failureStatus = 403
    await expect(testCase.run()).rejects.toMatchObject({ response: { status: 403 } })
    expect(writes).toHaveLength(1)
    expect(refreshes).toHaveLength(0)
  })
  it.each(cases)('$name preserves successful method and exact payload', async (testCase) => {
    target = testCase.url
    failureStatus = 0
    await expect(testCase.run()).resolves.toMatchObject({ success: true })
    expect(writes).toHaveLength(1)
    expect(writes[0].method).toBe(testCase.method)
    if (testCase.name === 'create channel') expect(writes[0].headers.get('Idempotency-Key')).toBe('synthetic-stable-operation-key')
    expect(writes[0].data === undefined ? undefined : JSON.parse(writes[0].data)).toEqual(testCase.body)
    expect(refreshes).toHaveLength(0)
  })
})

it('keeps the read-only multi-key status request eligible for authentication refresh without converting it to a write', async () => {
  target = '/api/channel/multi_key/manage'
  await expect(manageMultiKeys({ channel_id: 17, action: 'get_key_status', page: 2, page_size: 50 })).resolves.toMatchObject({ success: true })
  expect(writes).toHaveLength(2)
  expect(refreshes).toHaveLength(1)
  expect(writes.map((config) => JSON.parse(config.data))).toEqual([
    { channel_id: 17, action: 'get_key_status', page: 2, page_size: 50 },
    { channel_id: 17, action: 'get_key_status', page: 2, page_size: 50 },
  ])
})
