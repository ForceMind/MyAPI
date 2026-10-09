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
  QueryCache,
  QueryClient,
  QueryClientProvider,
} from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import { AxiosError } from 'axios'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { usePlaygroundOptions } from '../use-playground-options'

afterEach(() => {
  useAuthStore.getState().auth.reset()
  vi.restoreAllMocks()
})

function serverError(): AxiosError {
  return new AxiosError(
    'Internal Server Error',
    'ERR_BAD_RESPONSE',
    undefined,
    undefined,
    {
      status: 500,
      statusText: 'Internal Server Error',
      data: {},
      headers: {},
      config: { headers: {} },
    } as unknown as import('axios').AxiosResponse
  )
}

describe('recoverable model and key discovery failures', () => {
  test.each(['keys', 'models'])(
    'a real Axios 500 while loading %s stays in the composer instead of reaching global navigation',
    async (failedRoute) => {
      useAuthStore.setState((state) => ({
        auth: {
          ...state.auth,
          user: { id: 101, username: 'A', role: 1 },
          accessToken: 'session',
          accessExpiresAt: Math.floor(Date.now() / 1000) + 3600,
        },
      }))
      const navigateToErrorPage = vi.fn()
      // Match the actual application's global QueryCache navigation boundary.
      const client = new QueryClient({
        queryCache: new QueryCache({
          onError: (error) => {
            if (error instanceof AxiosError && error.response?.status === 500) {
              navigateToErrorPage('/500')
            }
          },
        }),
        defaultOptions: { queries: { retry: false } },
      })
      vi.spyOn(api, 'get').mockImplementation(async (url) => {
        if (url === `/pg/${failedRoute}`) throw serverError()
        if (url === '/pg/keys') {
          return {
            data: {
              success: true,
              data: {
                items: [
                  {
                    id: 17,
                    name: 'Chat',
                    status: 1,
                    group: 'default',
                    remain_quota: 100,
                    unlimited_quota: false,
                    expired_time: -1,
                  },
                ],
                total: 1,
              },
            },
          }
        }
        return { data: { success: true, data: [{ id: 'test-model' }] } }
      })
      const { result, unmount } = renderHook(
        () =>
          usePlaygroundOptions({
            userId: 101,
            keyId: 17,
            currentModel: 'test-model',
            updateConfig: () => undefined,
          }),
        {
          wrapper: (props: { children: ReactNode }) => (
            <QueryClientProvider client={client}>
              {props.children}
            </QueryClientProvider>
          ),
        }
      )
      await waitFor(() =>
        expect(result.current.keyNotice).toBe(
          failedRoute === 'keys'
            ? 'Failed to load API keys'
            : 'Failed to load playground models'
        )
      )
      expect(result.current.canSend).toBe(false)
      expect(navigateToErrorPage).not.toHaveBeenCalled()
      unmount()
      client.clear()
    }
  )
})
