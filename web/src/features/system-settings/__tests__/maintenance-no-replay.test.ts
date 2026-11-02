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
const settings = await import('../api')
const instances = await import('@/features/system-info/api')
const oauth = await import('../auth/custom-oauth/api')
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
const provider = { name: 'Synthetic OAuth', slug: 'synthetic-oauth', icon: '', enabled: false, client_id: 'synthetic-client', client_secret: 'synthetic-not-a-real-secret', authorization_endpoint: 'https://synthetic.invalid/authorize', token_endpoint: 'https://synthetic.invalid/token', user_info_endpoint: 'https://synthetic.invalid/user', scopes: 'openid profile', user_id_field: 'sub', username_field: 'name', display_name_field: 'name', email_field: 'email', well_known: '', auth_style: 0, access_policy: '', access_denied_message: '' }
const cases = [
  { name: 'update a single option', method: 'put', url: '/api/option/', body: { key: 'PasswordLoginEnabled', value: false }, run: () => settings.updateSystemOption({ key: 'PasswordLoginEnabled', value: false }) },
  { name: 'apply typed CAS options with false and zero', method: 'put', url: '/api/option/typed-bulk', body: { expected_revision: 7, items: [{ key: 'GlobalSSRFEnabled', type: 'boolean', value: false }, { key: 'GlobalSSRFMaxRedirects', type: 'number', value: 0 }] }, run: () => settings.updateOptionsTypedBulk({ expected_revision: 7, items: [{ key: 'GlobalSSRFEnabled', type: 'boolean', value: false }, { key: 'GlobalSSRFMaxRedirects', type: 'number', value: 0 }] }) },
  { name: 'update funding controls', method: 'put', url: '/api/option/payment-funding', body: { values: { 'payment_setting.commercial_enabled': false, 'payment_setting.retirement_timeout': 0 } }, run: () => settings.updatePaymentFundingOptions({ values: { 'payment_setting.commercial_enabled': false, 'payment_setting.retirement_timeout': 0 } }) },
  { name: 'confirm compliance', method: 'post', url: '/api/option/payment_compliance', body: { confirmed: true }, run: () => settings.confirmPaymentCompliance() },
  { name: 'start exact log cleanup task', method: 'post', url: '/api/system-task/log-cleanup', body: null, params: { target_timestamp: 1730000000 }, run: () => settings.startLogCleanupTask(1730000000) },
  { name: 'reset model ratios', method: 'post', url: '/api/option/rest_model_ratio', body: undefined, run: () => settings.resetModelRatios() },
  { name: 'delete stale system instances', method: 'delete', url: '/api/system-info/stale-instances', body: undefined, run: () => instances.deleteStaleSystemInstances() },
  { name: 'delete exact stale node identifier', method: 'delete', url: '/api/system-info/instances/synthetic%2Fnode%20one', body: undefined, run: () => instances.deleteStaleSystemInstance('synthetic/node one') },
  { name: 'create custom OAuth provider', method: 'post', url: '/api/custom-oauth-provider/', body: provider, run: () => oauth.createCustomOAuthProvider(provider) },
  { name: 'update custom OAuth provider', method: 'put', url: '/api/custom-oauth-provider/7', body: { name: 'Synthetic updated provider', enabled: false, auth_style: 0 }, run: () => oauth.updateCustomOAuthProvider(7, { name: 'Synthetic updated provider', enabled: false, auth_style: 0 }) },
  { name: 'delete custom OAuth provider', method: 'delete', url: '/api/custom-oauth-provider/7', body: undefined, run: () => oauth.deleteCustomOAuthProvider(7) },
  { name: 'explicit OIDC discovery', method: 'post', url: '/api/custom-oauth-provider/discovery', body: { well_known_url: 'https://synthetic.invalid/.well-known/openid-configuration' }, run: () => oauth.discoverOIDCEndpoints('https://synthetic.invalid/.well-known/openid-configuration') },
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

describe('settings and maintenance actions never replay', () => {
  it.each(cases)('$name rejects 401 after one dispatch without authentication refresh or write replay', async (testCase) => {
    target = testCase.url
    await expect(testCase.run()).rejects.toMatchObject({ response: { status: 401 } })
    expect(writes).toHaveLength(1)
    expect(refreshes).toHaveLength(0)
    expect(writes[0].method).toBe(testCase.method)
    if ('params' in testCase) expect(writes[0].params).toEqual(testCase.params)
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
    if ('params' in testCase) expect(writes[0].params).toEqual(testCase.params)
    expect(writes[0].data === undefined ? undefined : JSON.parse(writes[0].data)).toEqual(testCase.body)
    expect(refreshes).toHaveLength(0)
  })
})
