import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useBillingHistory } from '../use-billing-history'

const originalAuth = useAuthStore.getState()
beforeEach(() => useAuthStore.setState((state) => ({ auth: { ...state.auth, user: { id: 1, role: 100, username: 'root' }, session: null } })))
afterEach(() => { cleanup(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })

it('closed history never reads and a late page cannot populate a closed task or trigger a completion write', async () => {
  let release!: () => void
  const gate = new Promise<void>((resolve) => { release = resolve })
  const get = vi.spyOn(api, 'get').mockImplementation(async () => { await gate; return { data: { success: true, data: { items: [{ id: 17, trade_no: 'old-order' }], total: 1 } } } })
  const post = vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected completion'))
  const view = renderHook(({ open }) => useBillingHistory({ enabled: open }), { initialProps: { open: false } })
  expect(get).not.toHaveBeenCalled()
  view.rerender({ open: true })
  await waitFor(() => expect(get).toHaveBeenCalledOnce())
  view.rerender({ open: false })
  await act(async () => release())
  expect(view.result.current.records).toEqual([])
  await act(async () => { expect(await view.result.current.handleCompleteOrder('old-order')).toBe(false) })
  expect(post).not.toHaveBeenCalled()
})

it('an old actor cannot complete a previously displayed order or refresh its history after the original write settles', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: { items: [], total: 0 } } })
  let release!: () => void
  const gate = new Promise<void>((resolve) => { release = resolve })
  const post = vi.spyOn(api, 'post').mockImplementation(async () => { await gate; return { data: { success: true } } })
  const view = renderHook(() => useBillingHistory())
  await waitFor(() => expect(view.result.current.loading).toBe(false))
  let first!: Promise<boolean>
  let duplicate!: Promise<boolean>
  act(() => { first = view.result.current.handleCompleteOrder('exact-order'); duplicate = view.result.current.handleCompleteOrder('exact-order') })
  expect(await duplicate).toBe(false)
  const reads = vi.mocked(api.get).mock.calls.length
  act(() => useAuthStore.getState().auth.setUser({ id: 2, role: 1, username: 'ordinary' }))
  await act(async () => { release(); expect(await first).toBe(false) })
  expect(post).toHaveBeenCalledExactlyOnceWith('/api/user/topup/complete', { trade_no: 'exact-order' }, { skipErrorHandler: true, skipBusinessError: true, disableDuplicate: true, isCurrentScope: expect.any(Function), skipAuthRefresh: true })
  expect(api.get).toHaveBeenCalledTimes(reads)
})

it('closing and reopening history does not revive a former completion response or perform its follow-up read', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: { items: [], total: 0 } } })
  let release!: () => void
  const gate = new Promise<void>((resolve) => { release = resolve })
  vi.spyOn(api, 'post').mockImplementation(async () => { await gate; return { data: { success: true } } })
  const view = renderHook(({ open }) => useBillingHistory({ enabled: open }), { initialProps: { open: true } })
  await waitFor(() => expect(view.result.current.loading).toBe(false))
  let pending!: Promise<boolean>
  act(() => { pending = view.result.current.handleCompleteOrder('original-order') })
  view.rerender({ open: false })
  view.rerender({ open: true })
  await waitFor(() => expect(get).toHaveBeenCalledTimes(2))
  await act(async () => { release(); expect(await pending).toBe(false) })
  expect(get).toHaveBeenCalledTimes(2)
  expect(view.result.current.completionUnknown).toBe(true)
  expect(view.result.current.completing).toBe(false)
})
