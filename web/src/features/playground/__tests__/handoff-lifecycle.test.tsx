/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { Playground } from '..'
import {
  completePlaygroundKeyHandoff,
  createPlaygroundKeyHandoff,
  readPlaygroundKeyHandoff,
} from '../lib/playground-key-handoff'
import { loadConfig, saveConfig } from '../lib/storage/storage'
import type { PlaygroundKey } from '../types'

const handedOffKey: PlaygroundKey = {
  id: 23,
  name: 'Workbench Key',
  status: 1,
  group: 'default',
  remain_quota: 100,
  unlimited_quota: false,
  expired_time: -1,
}

function deferred<T>() {
  let resolve: (value: T) => void = () => undefined
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

function wrapper(client: QueryClient) {
  return function QueryWrapper(props: { children: ReactNode }) {
    return (
      <QueryClientProvider client={client}>
        {props.children}
      </QueryClientProvider>
    )
  }
}

afterEach(() => {
  const handoff = readPlaygroundKeyHandoff(7, 'session-a')
  if (handoff.status !== 'none') {
    completePlaygroundKeyHandoff(handoff.requestId)
  }
  useAuthStore.getState().auth.reset()
  localStorage.clear()
  vi.restoreAllMocks()
})

test('unmounting a pending handoff prevents it from selecting the Key after remount', async () => {
  useAuthStore.getState().auth.setBundle({
    access_token: 'session-access-token',
    token_type: 'Bearer',
    access_expires_at: Math.floor(Date.now() / 1000) + 3600,
    user: { id: 7, username: 'owner', role: 1 },
    session: {
      sid: 'session-a',
      current: true,
      login_method: 'password',
      ip: '127.0.0.1',
      user_agent: 'test',
      created_at: 1,
      last_active_at: 1,
      expires_at: Math.floor(Date.now() / 1000) + 3600,
    },
  })
  saveConfig({ keyId: 22, model: 'personal-model' }, 7)
  createPlaygroundKeyHandoff({
    userId: 7,
    sessionId: 'session-a',
    keyId: handedOffKey.id,
  })

  const firstKeysResponse = deferred<{
    data: {
      success: true
      data: { items: PlaygroundKey[]; total: number }
    }
  }>()
  let firstKeysSignal: { readonly aborted: boolean } | undefined
  let keysRequests = 0
  const get = vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
    if (url === '/pg/keys') {
      keysRequests += 1
      if (keysRequests === 1) {
        firstKeysSignal = config?.signal
        return firstKeysResponse.promise
      }
      return {
        data: {
          success: true,
          data: { items: [handedOffKey], total: 1 },
        },
      }
    }
    throw new Error(`Unexpected request: ${url}`)
  })
  const post = vi
    .spyOn(api, 'post')
    .mockRejectedValue(new Error('Unexpected Playground POST'))
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  const firstMount = render(<Playground />, { wrapper: wrapper(client) })

  await waitFor(() => expect(firstKeysSignal).toBeDefined())
  expect(loadConfig(7).keyId).toBeNull()

  firstMount.unmount()
  expect(firstKeysSignal?.aborted).toBe(true)
  expect(readPlaygroundKeyHandoff(7, 'session-a')).toEqual({ status: 'none' })

  const secondMount = render(<Playground />, { wrapper: wrapper(client) })
  await waitFor(() => expect(keysRequests).toBe(2))
  expect(screen.getByRole('combobox', { name: 'API key' })).toHaveValue('')
  expect(loadConfig(7).keyId).toBeNull()

  await act(async () => {
    firstKeysResponse.resolve({
      data: {
        success: true,
        data: { items: [handedOffKey], total: 1 },
      },
    })
    await firstKeysResponse.promise
    await Promise.resolve()
  })

  expect(screen.getByRole('combobox', { name: 'API key' })).toHaveValue('')
  expect(get).not.toHaveBeenCalledWith('/pg/models', expect.anything())
  expect(post).not.toHaveBeenCalled()
  expect(loadConfig(7).keyId).toBeNull()

  secondMount.unmount()
  client.clear()
})
