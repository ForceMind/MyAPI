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
import { act, renderHook } from '@testing-library/react'
import { toast } from 'sonner'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useAuthStore, type AuthBundle } from '@/stores/auth-store'

import { useSecurityRequestScope } from '../hooks/use-security-request-scope'

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

const originalAuth = useAuthStore.getState()

function authBundle(sid: string, userId = 1, role = 100): AuthBundle {
  return {
    access_token: `access-${sid}`,
    token_type: 'Bearer',
    access_expires_at: 4_000_000_000,
    user: { id: userId, username: `user-${userId}`, role },
    session: {
      sid,
      current: true,
      login_method: 'password',
      ip: '127.0.0.1',
      user_agent: 'synthetic-test',
      created_at: 1,
      last_active_at: 1,
      expires_at: 4_000_000_000,
    },
  }
}

beforeEach(() => {
  useAuthStore.getState().auth.setBundle(authBundle('session-1'))
})

afterEach(() => {
  useAuthStore.setState(originalAuth)
  vi.clearAllMocks()
})

describe('security request scope', () => {
  it('aborts the old request and rejects its result after the signed-in identity changes', async () => {
    let resolveOldRequest!: (value: string) => void
    let oldSignal: AbortSignal | undefined
    const oldRequest = new Promise<string>((resolve) => {
      resolveOldRequest = resolve
    })
    const { result, rerender } = renderHook(
      ({ scopeKey }) => useSecurityRequestScope(scopeKey),
      { initialProps: { scopeKey: 'user-1:session-1' } }
    )

    let pending!: Promise<{ current: boolean; value?: string }>
    act(() => {
      pending = result.current.run((signal) => {
        oldSignal = signal
        return oldRequest
      })
    })

    rerender({ scopeKey: 'user-2:session-2' })
    expect(oldSignal?.aborted).toBe(true)

    resolveOldRequest('old-account-secret')
    await expect(pending).resolves.toEqual({ current: false })
  })

  it('does not start an old-scope request after the store identity changes before effect cleanup', async () => {
    const request = vi.fn(async () => 'new-account-secret')
    const { result } = renderHook(() =>
      useSecurityRequestScope('user-1:session-1')
    )

    useAuthStore.getState().auth.setBundle(authBundle('session-2', 2))

    await expect(result.current.run(request)).resolves.toEqual({
      current: false,
    })
    expect(request).not.toHaveBeenCalled()
  })

  it.each(['resolve', 'reject'] as const)(
    'suppresses a late %s and its feedback after the store identity changes',
    async (outcome) => {
      let resolveRequest!: (value: string) => void
      let rejectRequest!: (reason: Error) => void
      const request = new Promise<string>((resolve, reject) => {
        resolveRequest = resolve
        rejectRequest = reject
      })
      const { result } = renderHook(() =>
        useSecurityRequestScope('user-1:session-1')
      )

      const pending = result.current
        .run(() => request)
        .then((requestResult) => {
          if (requestResult.current) toast.success('stale success')
          return requestResult
        })
        .catch((error) => {
          toast.error('stale error')
          throw error
        })

      useAuthStore.getState().auth.setBundle(authBundle('session-2', 2))
      if (outcome === 'resolve') resolveRequest('old-account-secret')
      else rejectRequest(new Error('old-account-error'))

      await expect(pending).resolves.toEqual({ current: false })
      expect(toast.success).not.toHaveBeenCalled()
      expect(toast.error).not.toHaveBeenCalled()
    }
  )

  it('keeps the current request valid after a same-user same-session token rotation', async () => {
    const { result } = renderHook(() =>
      useSecurityRequestScope('user-1:session-1')
    )

    const rotated = authBundle('session-1')
    rotated.access_token = 'rotated-access'
    rotated.session.last_active_at = 2
    useAuthStore.getState().auth.setBundle(rotated)

    await expect(result.current.run(async () => 'accepted')).resolves.toEqual({
      current: true,
      value: 'accepted',
    })
  })
})
