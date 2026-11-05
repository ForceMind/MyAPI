import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { AxiosError, type AxiosAdapter, type AxiosInstance } from 'axios'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { toast } from 'sonner'

import { api, applyAuthBundle } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { usePayment } from '../use-payment'
import { useWalletSession, walletRequestConfig } from '../use-wallet-session'
import { getWalletAccount } from '../../api'

const axiosTransport = vi.hoisted(() => ({ clients: [] as AxiosInstance[], unexpected: [] as string[] }))
// Keep real Axios clients/interceptors; replace only each client's HTTP adapter.
vi.mock('axios', async (importOriginal) => {
  const actual = await importOriginal<typeof import('axios')>()
  return { ...actual, default: { ...actual.default, create: (...args: Parameters<typeof actual.default.create>) => {
    const client = actual.default.create(...args)
    axiosTransport.clients.push(client)
    return client
  } } }
})

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))
const originalAuth = useAuthStore.getState()
const originalAdapters = new Map(axiosTransport.clients.map((client) => [client, client.defaults.adapter]))
let sequence = 0
let initialSid: string
function signIn(id: number, sid: string) {
  applyAuthBundle({ access_token: `synthetic-${sid}`, token_type: 'Bearer', access_expires_at: 4000000000, user: { id, role: 1, username: `user-${id}` }, session: { sid, current: true, login_method: 'password', ip: '', user_agent: '', created_at: 1, last_active_at: 1, expires_at: 4000000000 } }, false)
}
beforeEach(() => { axiosTransport.unexpected = []; for (const client of axiosTransport.clients) client.defaults.adapter = async (config) => { axiosTransport.unexpected.push(config.url ?? ''); throw new Error(`Unexpected authentication request ${config.url}`) }; initialSid = `original-${++sequence}`; signIn(1, initialSid); vi.mocked(toast.error).mockClear(); vi.mocked(toast.success).mockClear(); vi.spyOn(window, 'open').mockReturnValue(null) })
afterEach(() => { cleanup(); for (const [client, adapter] of originalAdapters) client.defaults.adapter = adapter; useAuthStore.setState(originalAuth); vi.restoreAllMocks() })

it.each(['business', '401', 'transport'] as const)('the actual Axios %s response after a session change cannot notify, refresh, replay or redirect the new account', async (failure) => {
  const calls: { path?: string; method?: string; body: unknown; auth: unknown; noRefresh?: boolean }[] = []
  let release!: () => void
  const gate = new Promise<void>((resolve) => { release = resolve })
  const adapter: AxiosAdapter = async (config) => {
    calls.push({ path: config.url, method: config.method, body: config.data ? JSON.parse(config.data) : undefined, auth: config.headers.Authorization, noRefresh: config.skipAuthRefresh })
    if (config.url !== '/api/user/stripe/pay') throw new Error(`Unexpected replay or refresh ${config.url}`)
    await gate
    if (failure === 'business') return { config, status: 200, statusText: 'OK', headers: {}, data: { success: false, message: 'Private original-account failure' } }
    if (failure === '401') throw new AxiosError('old unauthorized', 'ERR_BAD_REQUEST', config, undefined, { config, status: 401, statusText: 'Unauthorized', headers: {}, data: { message: 'Private original-account failure' } })
    throw new AxiosError('Private original-account transport loss', 'ERR_NETWORK', config)
  }
  api.defaults.adapter = adapter
  const view = renderHook(usePayment)
  let request!: Promise<boolean>
  act(() => { request = view.result.current.processPayment(120, 'stripe') })
  await waitFor(() => expect(calls).toHaveLength(1))
  act(() => signIn(2, 'next'))
  await act(async () => { release(); expect(await request).toBe(false) })
  expect(calls).toEqual([{ path: '/api/user/stripe/pay', method: 'post', body: { amount: 120, payment_method: 'stripe' }, auth: `Bearer synthetic-${initialSid}`, noRefresh: true }])
  expect(toast.error).not.toHaveBeenCalled()
  expect(toast.success).not.toHaveBeenCalled()
  expect(window.open).not.toHaveBeenCalled()
  expect(axiosTransport.unexpected).toEqual([])
  expect(useAuthStore.getState().auth.user?.id).toBe(2)
  expect(useAuthStore.getState().auth.session?.sid).toBe('next')
})

it('a current transport loss reports an unknown outcome once without replaying the order', async () => {
  const calls: string[] = []
  api.defaults.adapter = async (config) => { calls.push(config.url ?? ''); throw new AxiosError('Synthetic transport loss', 'ERR_NETWORK', config) }
  const view = renderHook(usePayment)
  await act(async () => { expect(await view.result.current.processPayment(120, 'stripe')).toBe(false) })
  expect(view.result.current.outcomeUnknown).toBe(true)
  expect(toast.error).toHaveBeenCalledExactlyOnceWith('Payment request outcome is unknown. Check billing history before trying again.')
  expect(calls).toEqual(['/api/user/stripe/pay'])
  expect(window.open).not.toHaveBeenCalled()
})

it('a current definitive HTTP rejection preserves its server message and is not mislabeled as an unknown write', async () => {
  api.defaults.adapter = async (config) => { throw new AxiosError('Bad Request', 'ERR_BAD_REQUEST', config, undefined, { config, status: 400, statusText: 'Bad Request', headers: {}, data: { message: 'Synthetic payment method unavailable' } }) }
  const view = renderHook(usePayment)
  await act(async () => { expect(await view.result.current.processPayment(120, 'stripe')).toBe(false) })
  expect(view.result.current.outcomeUnknown).toBe(false)
  expect(toast.error).toHaveBeenCalledExactlyOnceWith('Synthetic payment method unavailable')
})


it('a stale scoped GET cannot refresh or replay a former account while ordinary current GETs retain renewal', async () => {
  const calls: string[] = []
  let release!: () => void
  const gate = new Promise<void>((resolve) => { release = resolve })
  api.defaults.adapter = async (config) => {
    calls.push(config.url ?? '')
    await gate
    throw new AxiosError('old read unauthorized', 'ERR_BAD_REQUEST', config, undefined, { config, status: 401, statusText: 'Unauthorized', headers: {}, data: {} })
  }
  const view = renderHook(() => {
    const isCurrent = useWalletSession()
    return () => getWalletAccount(walletRequestConfig(isCurrent))
  })
  let request!: Promise<unknown>
  act(() => { request = view.result.current().catch(() => undefined) })
  await waitFor(() => expect(calls).toHaveLength(1))
  act(() => signIn(2, 'next'))
  await act(async () => { release(); await request })
  expect(calls).toEqual(['/api/user/self'])
  expect(axiosTransport.unexpected).toEqual([])
  expect(useAuthStore.getState().auth.user?.id).toBe(2)
  expect(useAuthStore.getState().auth.session?.sid).toBe('next')
  expect(toast.error).not.toHaveBeenCalled()
})

it('an active scoped GET renews once and retries only its read using the new token', async () => {
  const calls: { path?: string; method?: string; auth: unknown; noRefresh?: boolean }[] = []
  let reads = 0
  const adapter: AxiosAdapter = async (config) => {
    calls.push({ path: config.url, method: config.method, auth: config.headers.Authorization, noRefresh: config.skipAuthRefresh })
    if (config.url === '/api/user/auth/refresh') {
      const auth = useAuthStore.getState().auth
      return { config, status: 200, statusText: 'OK', headers: {}, data: { success: true, data: { access_token: 'synthetic-renewed', token_type: 'Bearer', access_expires_at: 4000000000, user: auth.user, session: auth.session } } }
    }
    if (config.url !== '/api/user/self') throw new Error(`Unexpected side-effect request ${config.url}`)
    reads += 1
    if (reads === 1) throw new AxiosError('read unauthorized', 'ERR_BAD_REQUEST', config, undefined, { config, status: 401, statusText: 'Unauthorized', headers: {}, data: {} })
    return { config, status: 200, statusText: 'OK', headers: {}, data: { success: true, data: { id: 1, request_count: 0 } } }
  }
  for (const client of axiosTransport.clients) client.defaults.adapter = adapter
  const view = renderHook(() => {
    const isCurrent = useWalletSession()
    return () => getWalletAccount(walletRequestConfig(isCurrent))
  })
  await act(async () => { expect(await view.result.current()).toMatchObject({ success: true, data: { id: 1 } }) })
  expect(calls.map((call) => [call.method, call.path])).toEqual([['get', '/api/user/self'], ['post', '/api/user/auth/refresh'], ['get', '/api/user/self']])
  expect(calls[0]?.noRefresh).not.toBe(true)
  expect(calls[2]?.auth).toBe('Bearer synthetic-renewed')
  expect(useAuthStore.getState().auth.session?.sid).toBe(initialSid)
  expect(toast.error).not.toHaveBeenCalled()
})

it('an unconfirmed payment cannot be submitted again under the same session even after its hook remounts', async () => {
  const calls: string[] = []
  api.defaults.adapter = async (config) => { calls.push(config.url ?? ''); throw new AxiosError('Synthetic committed response lost', 'ERR_NETWORK', config) }
  const first = renderHook(usePayment)
  await act(async () => { expect(await first.result.current.processPayment(120, 'stripe')).toBe(false) })
  await act(async () => { expect(await first.result.current.processPayment(120, 'stripe')).toBe(false) })
  expect(calls).toEqual(['/api/user/stripe/pay'])
  first.unmount()
  const reopened = renderHook(usePayment)
  expect(reopened.result.current.outcomeUnknown).toBe(true)
  await act(async () => { expect(await reopened.result.current.processPayment(120, 'alipay')).toBe(false) })
  expect(calls).toEqual(['/api/user/stripe/pay'])
  expect(toast.error).toHaveBeenCalledOnce()
})

it.each(['actor', 'closed scope'] as const)('a scoped GET never replays when its %s changes while authentication renewal is pending', async (change) => {
  const calls: string[] = []
  const original = useAuthStore.getState().auth
  let release!: () => void
  const gate = new Promise<void>((resolve) => { release = resolve })
  const adapter: AxiosAdapter = async (config) => {
    calls.push(config.url ?? '')
    if (config.url === '/api/user/auth/refresh') {
      await gate
      return { config, status: 200, statusText: 'OK', headers: {}, data: { success: true, data: { access_token: 'synthetic-renewed-old', token_type: 'Bearer', access_expires_at: 4000000000, user: original.user, session: original.session } } }
    }
    throw new AxiosError('read unauthorized', 'ERR_BAD_REQUEST', config, undefined, { config, status: 401, statusText: 'Unauthorized', headers: {}, data: {} })
  }
  for (const client of axiosTransport.clients) client.defaults.adapter = adapter
  let open = true
  const view = renderHook(() => {
    const isCurrent = useWalletSession()
    return () => getWalletAccount(walletRequestConfig(() => isCurrent() && open))
  })
  let request!: Promise<unknown>
  act(() => { request = view.result.current().catch(() => undefined) })
  await waitFor(() => expect(calls).toEqual(['/api/user/self', '/api/user/auth/refresh']))
  act(() => { if (change === 'actor') signIn(2, 'next'); else open = false })
  await act(async () => { release(); await request })
  expect(calls).toEqual(['/api/user/self', '/api/user/auth/refresh'])
  expect(toast.error).not.toHaveBeenCalled()
  expect(useAuthStore.getState().auth.user?.id).toBe(change === 'actor' ? 2 : 1)
  expect(useAuthStore.getState().auth.session?.sid).toBe(change === 'actor' ? 'next' : initialSid)
})
