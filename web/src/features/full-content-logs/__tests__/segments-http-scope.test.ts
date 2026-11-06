import axios, { AxiosError, type AxiosAdapter, type InternalAxiosRequestConfig } from 'axios'
import { toast } from 'sonner'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import type { AuthBundle } from '@/stores/auth-store'

let transport: AxiosAdapter
const adapter: AxiosAdapter = (config) => transport(config)
const defaultAdapter = axios.defaults.adapter
axios.defaults.adapter = adapter
const { api } = await import('@/lib/api')
const { useAuthStore } = await import('@/stores/auth-store')
const { getFullContentLogSegment } = await import('../api')
axios.defaults.adapter = defaultAdapter
const originalAuth = useAuthStore.getState()
const originalApiAdapter = api.defaults.adapter
const path = '/api/full-content-logs/http-scope/segments'
let reads: InternalAxiosRequestConfig[]
let refreshes: InternalAxiosRequestConfig[]

function bundle(token: string): AuthBundle {
  return { access_token: token, token_type: 'Bearer', access_expires_at: 4000000000,
    user: { id: 1, role: 100, username: 'synthetic-reader' },
    session: { sid: 'segments-http-session', current: true, login_method: 'password', ip: '', user_agent: 'test', created_at: 1, last_active_at: 1, expires_at: 4000000000 },
  }
}
function reject401(config: InternalAxiosRequestConfig): never {
  throw new AxiosError('Synthetic expired read', AxiosError.ERR_BAD_REQUEST, config, undefined, { config, status: 401, statusText: 'Unauthorized', headers: {}, data: { success: false } })
}
beforeEach(() => {
  localStorage.clear()
  api.defaults.adapter = adapter
  reads = []
  refreshes = []
  useAuthStore.getState().auth.setBundle(bundle('synthetic-original-access'))
})
afterEach(() => { api.defaults.adapter = originalApiAdapter; useAuthStore.setState(originalAuth); localStorage.clear(); vi.restoreAllMocks() })

test('an active saved-segment GET retains exactly one normal session refresh and replays only that read', async () => {
  transport = async (config) => {
    if (config.url === '/api/user/auth/refresh') {
      refreshes.push(config)
      return { config, status: 200, statusText: 'OK', headers: {}, data: { success: true, data: bundle('synthetic-refreshed-access') } }
    }
    if (config.url !== path) throw new Error('Unexpected synthetic read')
    reads.push(config)
    if (reads.length === 1) reject401(config)
    return { config, status: 200, statusText: 'OK', headers: {}, data: { success: true, data: {
      request_id: 'http-scope', part: 'response', requested_offset: '0', offset: '0',
      next_offset: null, total_bytes: '4', expected_bytes: null, has_more: false,
      snapshot: 'a'.repeat(64), encoding: 'utf-8', content_type: 'text/plain', body: 'TAIL', source_complete: null,
    } } }
  }
  const result = await getFullContentLogSegment('http-scope', { part: 'response', offset: '0', limit: 4 }, new AbortController().signal)
  expect(result.body).toBe('TAIL')
  expect(refreshes).toHaveLength(1)
  expect(reads).toHaveLength(2)
  for (const read of reads) {
    expect(read.method).toBe('get')
    expect(read.params).toEqual({ part: 'response', offset: '0', limit: 4 })
  }
})

test.each(['actor', 'role', 'session', 'close'] as const)('a 401 after %s changes cannot refresh, replay, notify or clear the current authentication', async (change) => {
  let release = () => {}
  const pending = new Promise<void>((resolve) => { release = resolve })
  transport = async (config) => {
    if (config.url === '/api/user/auth/refresh') {
      refreshes.push(config)
      throw new Error('An obsolete saved read must not refresh')
    }
    if (config.url !== path) throw new Error('Unexpected synthetic read')
    reads.push(config)
    await pending
    return reject401(config)
  }
  const errorToast = vi.spyOn(toast, 'error')
  const controller = new AbortController()
  const outcome = getFullContentLogSegment('http-scope', { part: 'response', offset: '0', limit: 4 }, controller.signal).catch((error: unknown) => error)
  if (change === 'close') controller.abort()
  else {
    const next = bundle('synthetic-new-access')
    if (change === 'actor') next.user.id = 2
    if (change === 'role') next.user.role = 10
    if (change === 'session') next.session.sid = 'new-segments-http-session'
    useAuthStore.getState().auth.setBundle(next)
  }
  release()
  expect(await outcome).toMatchObject({ code: 'ERR_CANCELED' })
  expect(refreshes).toHaveLength(0)
  expect(reads).toHaveLength(1)
  expect(errorToast).not.toHaveBeenCalled()
  expect(useAuthStore.getState().auth.accessToken).toBe(change === 'close' ? 'synthetic-original-access' : 'synthetic-new-access')
})
