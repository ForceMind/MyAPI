/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { useProfile } from '../hooks/use-profile'

const data = { id: 42, username: 'operator', display_name: 'Operations' }

afterEach(() => vi.useRealTimers())

describe('profile read lifecycle', () => {
  it('waits for the initial shell decision and sends one profile read when enabled', async () => {
    const get = vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data },
    })
    const { result, rerender } = renderHook(
      (enabled: boolean) => useProfile(enabled),
      { initialProps: false }
    )
    expect(get).not.toHaveBeenCalled()
    expect(result.current.loading).toBe(true)
    rerender(true)
    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(get).toHaveBeenCalledTimes(1)
    expect(result.current.profile).toEqual(data)
  })

  it('ends a stalled shell decision and lets retry read the profile independently', async () => {
    vi.useFakeTimers()
    const get = vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data },
    })
    const { result } = renderHook(() => useProfile(false))
    await act(() => vi.advanceTimersByTimeAsync(15000))
    expect(result.current.loading).toBe(false)
    expect(result.current.error).toBe(true)
    expect(get).not.toHaveBeenCalled()
    await act(() => result.current.fetchProfile())
    expect(result.current.profile).toEqual(data)
    expect(result.current.error).toBe(false)
    expect(get).toHaveBeenCalledTimes(1)
  })

  it('aborts a manual retry immediately on unmount while the shell decision is still pending', async () => {
    vi.useFakeTimers()
    let retrySignal: Pick<AbortSignal, 'aborted'> | undefined
    vi.spyOn(api, 'get').mockImplementation((_url, config) => {
      retrySignal = config?.signal
      return new Promise(() => undefined)
    })
    const { result, unmount } = renderHook(() => useProfile(false))
    await act(() => vi.advanceTimersByTimeAsync(15000))
    expect(result.current.error).toBe(true)
    let retry!: Promise<void>
    act(() => {
      retry = result.current.fetchProfile()
    })
    expect(retrySignal?.aborted).toBe(false)
    unmount()
    expect(retrySignal?.aborted).toBe(true)
    await act(() => retry)
  })

  it('ends a stalled read and supports a new successful attempt', async () => {
    vi.useFakeTimers()
    vi.spyOn(api, 'get')
      .mockImplementationOnce(() => new Promise(() => undefined))
      .mockResolvedValueOnce({ data: { success: true, data } })
    const { result } = renderHook(() => useProfile())
    expect(result.current.loading).toBe(true)
    await act(() => vi.advanceTimersByTimeAsync(15000))
    expect(result.current.loading).toBe(false)
    expect(result.current.error).toBe(true)
    await act(() => result.current.fetchProfile())
    expect(result.current.profile).toEqual(data)
    expect(result.current.error).toBe(false)
  })

  it('accepts a profile response that completes just before the read deadline', async () => {
    vi.useFakeTimers()
    vi.spyOn(api, 'get').mockImplementation(
      () =>
        new Promise((resolve) => {
          globalThis.setTimeout(
            () => resolve({ data: { success: true, data } }),
            14000
          )
        })
    )
    const { result } = renderHook(() => useProfile())

    await act(() => vi.advanceTimersByTimeAsync(14000))

    expect(result.current.profile).toEqual(data)
    expect(result.current.loading).toBe(false)
    expect(result.current.error).toBe(false)
  })

  it('keeps loaded profile and controls available if a silent refresh fails', async () => {
    vi.spyOn(api, 'get')
      .mockResolvedValueOnce({ data: { success: true, data } })
      .mockRejectedValueOnce(new Error('Synthetic refresh unavailable'))
    const { result } = renderHook(() => useProfile())
    await waitFor(() => expect(result.current.loading).toBe(false))
    await act(() => result.current.refreshProfile())
    expect(result.current.profile).toEqual(data)
    expect(result.current.error).toBe(false)
    expect(result.current.loading).toBe(false)
  })

  it('keeps the newer snapshot when a cancelled read completes late', async () => {
    type ReadResponse = Awaited<ReturnType<typeof api.get>>
    let resolveOldRead!: (response: ReadResponse) => void
    const oldRead = new Promise<ReadResponse>((resolve) => {
      resolveOldRead = resolve
    })
    const newData = { ...data, display_name: 'Updated Operations' }
    vi.spyOn(api, 'get')
      .mockReturnValueOnce(oldRead)
      .mockResolvedValueOnce({ data: { success: true, data: newData } })
    const { result } = renderHook(() => useProfile())
    await act(() => result.current.refreshProfile())
    await act(async () =>
      resolveOldRead({ data: { success: true, data } } as ReadResponse)
    )
    expect(result.current.profile).toEqual(newData)
    expect(result.current.loading).toBe(false)
    expect(result.current.error).toBe(false)
  })

  it('hides the prior profile immediately and ignores its late read after an identity change', async () => {
    type ReadResponse = Awaited<ReturnType<typeof api.get>>
    let resolveOldRead!: (response: ReadResponse) => void
    const oldRead = new Promise<ReadResponse>((resolve) => {
      resolveOldRead = resolve
    })
    const replacement = {
      ...data,
      id: 84,
      username: 'replacement',
      display_name: 'Replacement',
    }
    const get = vi
      .spyOn(api, 'get')
      .mockReturnValueOnce(oldRead)
      .mockResolvedValueOnce({ data: { success: true, data: replacement } })
    const { result, rerender } = renderHook(
      ({ identityKey }) => useProfile(true, identityKey),
      { initialProps: { identityKey: 'user-42:session-one' } }
    )

    rerender({ identityKey: 'user-84:session-two' })
    expect(result.current.profile).toBeNull()
    expect(result.current.loading).toBe(true)
    await waitFor(() => expect(result.current.profile).toEqual(replacement))

    await act(async () => {
      resolveOldRead({ data: { success: true, data } } as ReadResponse)
      await oldRead
    })
    expect(result.current.profile).toEqual(replacement)
    expect(get).toHaveBeenCalledTimes(2)
  })
})
