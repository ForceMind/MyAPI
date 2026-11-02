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
const { createVendor, createPrefillGroup, updatePrefillGroup, deletePrefillGroup } = await import('../api')
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
  { name: 'create vendor', method: 'post', url: '/api/vendors/', body: { name: 'Synthetic vendor', description: '', icon: '', status: 1 }, run: () => createVendor({ name: 'Synthetic vendor', description: '', icon: '', status: 1 }) },
  { name: 'create model prefill', method: 'post', url: '/api/prefill_group', body: { name: 'Synthetic models', type: 'model', items: ['synthetic-model'], description: '' }, run: () => createPrefillGroup({ name: 'Synthetic models', type: 'model', items: ['synthetic-model'], description: '' }) },
  { name: 'update endpoint prefill', method: 'put', url: '/api/prefill_group', body: { id: 7, name: 'Synthetic endpoint', type: 'endpoint', items: '{"custom":{"enabled":false,"count":0}}' }, run: () => updatePrefillGroup({ id: 7, name: 'Synthetic endpoint', type: 'endpoint', items: '{"custom":{"enabled":false,"count":0}}' }) },
  { name: 'delete prefill', method: 'delete', url: '/api/prefill_group/7', body: undefined, run: () => deletePrefillGroup(7) },
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

describe('local model maintenance writes do not replay', () => {
  it.each(cases)('$name rejects 401 after one dispatch without authentication refresh or write replay', async (testCase) => {
    target = testCase.url
    await expect(testCase.run()).rejects.toMatchObject({ response: { status: 401 } })
    expect(writes).toHaveLength(1)
    expect(refreshes).toHaveLength(0)
    expect(writes[0].method).toBe(testCase.method)
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
    expect(writes[0].data === undefined ? undefined : JSON.parse(writes[0].data)).toEqual(testCase.body)
    expect(refreshes).toHaveLength(0)
  })
})
