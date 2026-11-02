import axios, { AxiosError, type AxiosAdapter, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { AuthBundle } from '@/stores/auth-store'

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }))
let transport: AxiosAdapter
const adapter: AxiosAdapter = (config) => transport(config)
const defaultAdapter = axios.defaults.adapter
axios.defaults.adapter = adapter
const { useAuthStore } = await import('@/stores/auth-store')
const auth = await import('../api')
const profile = await import('@/features/profile/api')
const passkey = await import('../passkey/api')
const verification = await import('../secure-verification/api')
axios.defaults.adapter = defaultAdapter

const originalAuth = useAuthStore.getState()
let dispatched: InternalAxiosRequestConfig[]
let refreshes: InternalAxiosRequestConfig[]
let failureStatus: number
let target: string
const credential = { id: 'synthetic-credential', type: 'public-key', response: { clientDataJSON: 'synthetic-client-data' } }
function bundle(token: string): AuthBundle {
  return { access_token: token, token_type: 'Bearer', access_expires_at: 4_000_000_000,
    user: { id: 1, role: 100, username: 'synthetic-maintainer' },
    session: { sid: 'synthetic-same-session', current: true, login_method: 'password', ip: '127.0.0.1', user_agent: 'test', created_at: 1, last_active_at: 1, expires_at: 4_000_000_000 } }
}
function response(config: InternalAxiosRequestConfig, data: unknown): AxiosResponse {
  return { config, data, status: 200, statusText: 'OK', headers: {} }
}
const cases = [
  { name: 'update profile metadata', method: 'put', url: '/api/user/self', body: { display_name: 'Synthetic name' }, run: () => profile.updateUserProfile({ display_name: 'Synthetic name' }) },
  { name: 'update password with existing rotation', method: 'put', url: '/api/user/self', body: { original_password: 'synthetic-old-password', password: 'synthetic-new-password' }, run: () => profile.updateUserProfile({ original_password: 'synthetic-old-password', password: 'synthetic-new-password' }) },
  { name: 'save profile settings with explicit zero and false', method: 'put', url: '/api/user/setting', body: { quota_warning_threshold: 0, gotify_priority: 0, accept_unset_model_ratio_model: false, record_ip_log: false }, run: () => profile.updateUserSettings({ quota_warning_threshold: 0, gotify_priority: 0, accept_unset_model_ratio_model: false, record_ip_log: false }) },
  { name: 'save language', method: 'put', url: '/api/user/self', body: { language: 'zh' }, run: () => profile.updateUserLanguage('zh') },
  { name: 'claim daily checkin', method: 'post', url: '/api/user/checkin', body: undefined, run: () => profile.performCheckin() },
  { name: 'claim daily checkin with original challenge', method: 'post', url: '/api/user/checkin?turnstile=synthetic%2Btoken', body: undefined, run: () => profile.performCheckin('synthetic+token') },
  { name: 'send reset email', method: 'get', url: '/api/reset_password', body: undefined, params: { email: 'synthetic@example.invalid', turnstile: 'synthetic-challenge' }, run: () => auth.sendPasswordResetEmail('synthetic@example.invalid', 'synthetic-challenge') },
  { name: 'start OAuth binding', method: 'post', url: '/api/oauth/state', body: { provider: 'github', intent: 'bind' }, run: () => auth.createOAuthFlow('github', 'bind') },
  { name: 'start OAuth login', method: 'post', url: '/api/oauth/state', body: { provider: 'github', intent: 'login' }, run: () => auth.createOAuthFlow('github', 'login') },
  { name: 'exchange WeChat code', method: 'get', url: '/api/oauth/wechat', body: undefined, params: { code: 'synthetic-code' }, run: () => auth.wechatLoginByCode('synthetic-code') },
  { name: 'register account', method: 'post', url: '/api/user/register', body: { username: 'synthetic-account', password: 'synthetic-not-a-real-password', turnstile: 'synthetic-challenge' }, params: { turnstile: 'synthetic-challenge' }, run: () => auth.register({ username: 'synthetic-account', password: 'synthetic-not-a-real-password', turnstile: 'synthetic-challenge' }) },
  { name: 'send registration code', method: 'get', url: '/api/verification', body: undefined, params: { email: 'synthetic@example.invalid', turnstile: 'synthetic-challenge' }, run: () => auth.sendEmailVerification('synthetic@example.invalid', 'synthetic-challenge') },
  { name: 'bind email', method: 'post', url: '/api/oauth/email/bind', body: { email: 'synthetic@example.invalid', code: 'synthetic-code' }, run: () => auth.bindEmail('synthetic@example.invalid', 'synthetic-code') },
  { name: 'begin Passkey registration', method: 'post', url: '/api/user/passkey/register/begin', body: undefined, proof: 'synthetic-proof', run: () => passkey.beginPasskeyRegistration('synthetic-proof') },
  { name: 'finish Passkey registration', method: 'post', url: '/api/user/passkey/register/finish', body: { flow_token: 'synthetic-flow', credential }, proof: 'synthetic-proof', run: () => passkey.finishPasskeyRegistration('synthetic-flow', credential, 'synthetic-proof') },
  { name: 'delete Passkey', method: 'delete', url: '/api/user/passkey', body: undefined, proof: 'synthetic-proof', run: () => passkey.deletePasskey('synthetic-proof') },
  { name: 'begin Passkey login', method: 'post', url: '/api/user/passkey/login/begin', body: undefined, run: () => passkey.beginPasskeyLogin() },
  { name: 'finish Passkey login', method: 'post', url: '/api/user/passkey/login/finish', body: { flow_token: 'synthetic-flow', credential }, run: () => passkey.finishPasskeyLogin('synthetic-flow', credential) },
  { name: 'begin scoped Passkey verification', method: 'post', url: '/api/user/passkey/verify/begin', body: { scope: 'passkey.delete' }, run: () => passkey.beginPasskeyVerification('passkey.delete') },
  { name: 'finish scoped Passkey verification', method: 'post', url: '/api/user/passkey/verify/finish', body: { flow_token: 'synthetic-flow', credential }, run: () => passkey.finishPasskeyVerification('synthetic-flow', credential) },
  { name: 'issue 2FA verification proof', method: 'post', url: '/api/verify', body: { method: '2fa', code: 'AB12CD34', scope: 'passkey.delete' }, run: () => verification.verify('2fa', 'passkey.delete', 'AB12CD34') },
]

beforeEach(() => {
  localStorage.clear()
  dispatched = []
  refreshes = []
  failureStatus = 401
  useAuthStore.getState().auth.setBundle(bundle('synthetic-original-access'))
  transport = async (config) => {
    if (config.url === '/api/user/auth/refresh') {
      refreshes.push(config)
      return response(config, { success: true, data: bundle('synthetic-refreshed-access') })
    }
    if (config.url !== target) throw new Error(`Unexpected request ${config.method} ${config.url}`)
    dispatched.push(config)
    if (dispatched.length === 1 && failureStatus !== 0) throw new AxiosError('Synthetic rejected auth action', AxiosError.ERR_BAD_REQUEST, config, undefined, { config, data: { success: false }, status: failureStatus, statusText: 'Rejected', headers: {} })
    return response(config, { success: true, data: { ...bundle('synthetic-rotated-access'), flow_token: 'synthetic-only-flow', proof_token: 'synthetic-only-proof', scope: 'passkey.delete', method: '2fa', expires_at: 4_000_000_000 } })
  }
})
afterEach(() => { useAuthStore.setState(originalAuth); localStorage.clear(); vi.restoreAllMocks() })

describe('profile and authentication actions never replay', () => {
  it.each(cases)('$name rejects 401 once without refresh or replay', async (testCase) => {
    target = testCase.url
    await expect(testCase.run()).rejects.toMatchObject({ response: { status: 401 } })
    expect(dispatched).toHaveLength(1)
    expect(refreshes).toHaveLength(0)
  })
  it.each(cases)('$name rejects 403 once without replay', async (testCase) => {
    target = testCase.url
    failureStatus = 403
    await expect(testCase.run()).rejects.toMatchObject({ response: { status: 403 } })
    expect(dispatched).toHaveLength(1)
    expect(refreshes).toHaveLength(0)
  })
  it.each(cases)('$name preserves the original method, data, challenge and rotation', async (testCase) => {
    target = testCase.url
    failureStatus = 0
    const result = await testCase.run()
    if (testCase.url === '/api/oauth/state') expect(result).toBe('synthetic-only-flow')
    else if (testCase.url === '/api/verify') expect(result).toMatchObject({ proof_token: 'synthetic-only-proof', scope: 'passkey.delete', method: '2fa' })
    else expect(result).toMatchObject({ success: true })
    expect(dispatched).toHaveLength(1)
    expect(refreshes).toHaveLength(0)
    expect(dispatched[0].method).toBe(testCase.method)
    expect(dispatched[0].data === undefined ? undefined : JSON.parse(dispatched[0].data)).toEqual(testCase.body)
    if ('params' in testCase) expect(dispatched[0].params).toEqual(testCase.params)
    if ('proof' in testCase) expect(dispatched[0].headers.get('X-Security-Proof')).toBe(testCase.proof)
    if (dispatched[0].acceptAuthRotation) expect(useAuthStore.getState().auth.accessToken).toBe('synthetic-rotated-access')
  })
})
