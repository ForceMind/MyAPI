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
import {
  AxiosError,
  type AxiosAdapter,
  type AxiosResponse,
  type InternalAxiosRequestConfig,
} from 'axios'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api, AuthRotationError } from '@/lib/api'
import { useAuthStore, type AuthBundle } from '@/stores/auth-store'

import { updateUserProfile } from '../api'

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}))

const originalAuth = useAuthStore.getState()
const originalAdapter = api.defaults.adapter

function authBundle(sid: string, userId = 1): AuthBundle {
  return {
    access_token: `access-${sid}`,
    token_type: 'Bearer',
    access_expires_at: 4_000_000_000,
    user: { id: userId, username: `user-${userId}`, role: 100 },
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

function response(
  config: InternalAxiosRequestConfig,
  data: unknown
): AxiosResponse {
  return { config, data, status: 200, statusText: 'OK', headers: {} }
}

beforeEach(() => {
  useAuthStore.getState().auth.setBundle(authBundle('current'))
})

afterEach(() => {
  api.defaults.adapter = originalAdapter
  useAuthStore.setState(originalAuth)
  vi.restoreAllMocks()
})

describe('profile security API authentication boundaries', () => {
  it('accepts the password success rotation while preventing a 401 refresh replay', async () => {
    const writes: InternalAxiosRequestConfig[] = []
    const rotated = authBundle('current')
    rotated.access_token = 'rotated-access-token'
    rotated.session.last_active_at = 2
    api.defaults.adapter = (async (config) => {
      writes.push(config)
      return response(config, {
        success: true,
        data: {
          access_token: rotated.access_token,
          token_type: rotated.token_type,
          access_expires_at: rotated.access_expires_at,
          session: rotated.session,
        },
      })
    }) as AxiosAdapter

    await expect(
      updateUserProfile({
        original_password: 'old-password',
        password: 'new-password',
      })
    ).resolves.toMatchObject({ success: true })

    expect(writes).toHaveLength(1)
    expect(writes[0]?.acceptAuthRotation).toBe(true)
    expect(writes[0]?.skipAuthRefresh).toBe(true)
    expect(useAuthStore.getState().auth.accessToken).toBe(
      'rotated-access-token'
    )
    expect(useAuthStore.getState().auth.session?.last_active_at).toBe(2)
  })

  it('does not replay a rejected password request after the active identity changes', async () => {
    const writes: InternalAxiosRequestConfig[] = []
    api.defaults.adapter = (async (config) => {
      writes.push(config)
      useAuthStore.getState().auth.setBundle(authBundle('replacement', 2))
      throw new AxiosError(
        'Synthetic expired authentication',
        AxiosError.ERR_BAD_REQUEST,
        config,
        undefined,
        {
          config,
          data: { success: false },
          status: 401,
          statusText: 'Unauthorized',
          headers: {},
        }
      )
    }) as AxiosAdapter

    await expect(
      updateUserProfile({
        original_password: 'old-password',
        password: 'new-password',
      })
    ).rejects.toBeInstanceOf(AxiosError)

    expect(writes).toHaveLength(1)
    expect(writes[0]?.skipAuthRefresh).toBe(true)
    expect(useAuthStore.getState().auth.user?.id).toBe(2)
    expect(useAuthStore.getState().auth.session?.sid).toBe('replacement')
  })

  it('rejects a late successful rotation from the previous session without replacing the new identity', async () => {
    let releaseResponse!: () => void
    let markRequestStarted!: () => void
    const heldResponse = new Promise<void>((resolve) => {
      releaseResponse = resolve
    })
    const requestStarted = new Promise<void>((resolve) => {
      markRequestStarted = resolve
    })
    api.defaults.adapter = (async (config) => {
      markRequestStarted()
      await heldResponse
      const rotated = authBundle('current')
      rotated.access_token = 'late-old-session-token'
      return response(config, {
        success: true,
        data: {
          access_token: rotated.access_token,
          token_type: rotated.token_type,
          access_expires_at: rotated.access_expires_at,
          session: rotated.session,
        },
      })
    }) as AxiosAdapter

    const pending = updateUserProfile({
      original_password: 'old-password',
      password: 'new-password',
    })
    await requestStarted
    useAuthStore.getState().auth.setBundle(authBundle('replacement', 2))
    releaseResponse()

    await expect(pending).rejects.toBeInstanceOf(AuthRotationError)
    expect(useAuthStore.getState().auth.accessToken).toBe('access-replacement')
    expect(useAuthStore.getState().auth.user?.id).toBe(2)
    expect(useAuthStore.getState().auth.session?.sid).toBe('replacement')
  })
})
