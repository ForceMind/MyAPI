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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { Playground } from '..'
import { createUserMessage } from '../lib/message/message-utils'
import { loadMessages, saveConfig, saveMessages } from '../lib/storage/storage'

function signIn(id: number) {
  useAuthStore.setState((state) => ({
    auth: {
      ...state.auth,
      user: { id, username: `user-${id}`, role: 1 },
      accessToken: 'session',
      accessExpiresAt: Math.floor(Date.now() / 1000) + 3600,
    },
  }))
}

afterEach(() => {
  localStorage.clear()
  useAuthStore.getState().auth.reset()
  vi.restoreAllMocks()
})

test('switching accounts remounts the real composer and flushes pending A history only to A storage', async () => {
  signIn(101)
  saveMessages([createUserMessage('private A history')], 101)
  saveConfig({ keyId: 11, model: 'test-model', stream: false }, 101)
  saveMessages([createUserMessage('private B history')], 202)
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    const userId = useAuthStore.getState().auth.user?.id
    if (url === '/pg/keys') {
      return {
        data: {
          success: true,
          data: {
            items: [
              {
                id: userId === 101 ? 11 : 22,
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
  const post = vi
    .spyOn(api, 'post')
    .mockImplementation(() => new Promise(() => undefined))
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const rendered = render(
    <QueryClientProvider client={client}>
      <Playground />
    </QueryClientProvider>
  )
  const user = userEvent.setup()
  await screen.findByText('private A history')
  await waitFor(() =>
    expect(screen.getByRole('combobox', { name: 'API key' })).toHaveValue('11')
  )
  await user.type(
    screen.getByRole('textbox', { name: 'Message' }),
    'pending A text'
  )
  await user.click(screen.getByRole('button', { name: 'Send' }))
  await waitFor(() => expect(post).toHaveBeenCalledOnce())
  act(() => {
    signIn(202)
  })
  await screen.findByText('private B history')
  expect(screen.queryByText('private A history')).not.toBeInTheDocument()
  expect(screen.queryByText('pending A text')).not.toBeInTheDocument()
  expect(screen.getByRole('combobox', { name: 'API key' })).toHaveValue('')
  expect(screen.getByRole('textbox', { name: 'Message' })).toHaveValue('')
  expect(
    loadMessages(101)?.some(
      (message) => message.versions[0].content === 'pending A text'
    )
  ).toBe(true)
  expect(
    loadMessages(202)?.map((message) => message.versions[0].content)
  ).toEqual(['private B history'])
  rendered.unmount()
  client.clear()
})
