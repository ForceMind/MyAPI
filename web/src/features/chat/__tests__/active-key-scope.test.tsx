import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { apiKeySchema } from '@/features/keys/types'
import { api } from '@/lib/api'
import { useAuthStore, type AuthBundle } from '@/stores/auth-store'

import { fetchActiveChatKey, useActiveChatKey } from '../hooks/use-active-chat-key'

const originalAuth = useAuthStore.getState()
let client: QueryClient
function bundle(sid: string): AuthBundle {
  return { access_token: 'synthetic-chat-access', token_type: 'Bearer', access_expires_at: 4_000_000_000, user: { id: 1, role: 1, username: 'synthetic-user' }, session: { sid, current: true, login_method: 'password', ip: '127.0.0.1', user_agent: 'test', created_at: 1, last_active_at: 1, expires_at: 4_000_000_000 } }
}
function list(id: number) {
  return { data: { success: true, data: { total: 1, items: [apiKeySchema.parse({ id, name: 'Synthetic chat key', key: 'synthetic-masked', status: 1, remain_quota: 500000, unlimited_quota: true })] } } }
}
beforeEach(() => {
  localStorage.clear()
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  useAuthStore.getState().auth.setBundle(bundle('synthetic-session-one'))
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); localStorage.clear(); vi.restoreAllMocks() })

it('never reveals the old account list selection after the login scope changes during its pending read', async () => {
  let finish: ((value: ReturnType<typeof list>) => void) | undefined
  const pending = new Promise<ReturnType<typeof list>>((resolve) => { finish = resolve })
  vi.spyOn(api, 'get').mockReturnValue(pending)
  const reveal = vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected synthetic reveal'))
  const result = fetchActiveChatKey()
  const rejected = expect(result).rejects.toMatchObject({ code: 'ERR_CANCELED' })
  useAuthStore.getState().auth.setBundle(bundle('synthetic-session-two'))
  finish?.(list(7))
  await rejected
  expect(reveal).not.toHaveBeenCalled()
})

it('keeps a same-user new session from reusing the old cached chat credential and reads only its new key', async () => {
  let reads = 0
  let finish: ((value: ReturnType<typeof list>) => void) | undefined
  const nextRead = new Promise<ReturnType<typeof list>>((resolve) => { finish = resolve })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url !== '/api/token/?p=1&size=50') throw new Error(`Unexpected GET ${url}`)
    reads += 1
    return reads === 1 ? list(7) : nextRead
  })
  const reveal = vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (url === '/api/token/7/key') return { data: { success: true, data: { key: 'synthetic-one' } } }
    if (url === '/api/token/9/key') return { data: { success: true, data: { key: 'synthetic-two' } } }
    throw new Error(`Unexpected POST ${url}`)
  })
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>
  const { result } = renderHook(() => useActiveChatKey(true), { wrapper })
  await waitFor(() => expect(result.current.data).toBe('sk-synthetic-one'))
  await act(async () => { useAuthStore.getState().auth.setBundle(bundle('synthetic-session-two')) })
  await waitFor(() => expect(reads).toBe(2))
  expect(result.current.data).toBeUndefined()
  await act(async () => { finish?.(list(9)); await nextRead })
  await waitFor(() => expect(result.current.data).toBe('sk-synthetic-two'))
  expect(reveal).toHaveBeenCalledTimes(2)
  expect(reveal).toHaveBeenLastCalledWith('/api/token/9/key', undefined, expect.objectContaining({ signal: expect.any(AbortSignal), skipAuthRefresh: true }))
})
