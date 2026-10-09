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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { Playground } from '../../..'
import { DEFAULT_CONFIG, DEFAULT_PARAMETER_ENABLED } from '../../../constants'
import {
  loadConfig,
  loadParameterEnabled,
  saveConfig,
  saveParameterEnabled,
} from '../../../lib/storage/storage'

const preferredConfig = {
  ...DEFAULT_CONFIG,
  keyId: 11,
  model: 'gpt-6.1-sol',
  stream: false,
  max_tokens: 200000,
  temperature: 0.6,
  top_p: 0.9,
  frequency_penalty: 0.4,
  presence_penalty: -0.3,
  seed: 17,
}
const preferredEnabled = { ...DEFAULT_PARAMETER_ENABLED, seed: true }

afterEach(() => {
  localStorage.clear()
  useAuthStore.getState().auth.reset()
  vi.restoreAllMocks()
})

function renderPlayground(keyId = 11) {
  const userId = 315
  useAuthStore.setState((state) => ({
    auth: {
      ...state.auth,
      user: { id: userId, username: 'parameter-user', role: 1 },
      accessToken: 'session',
      accessExpiresAt: Math.floor(Date.now() / 1000) + 3600,
    },
  }))
  saveConfig({ ...preferredConfig, keyId }, userId)
  saveParameterEnabled(preferredEnabled, userId)
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/pg/keys') {
      return {
        data: {
          success: true,
          data: {
            items: [
              {
                id: 11,
                name: 'Normal key',
                status: 1,
                group: 'default',
                remain_quota: 100,
                unlimited_quota: false,
                expired_time: -1,
                strict_token_budget: false,
              },
              {
                id: 24,
                name: 'Strict key',
                status: 1,
                group: 'default',
                remain_quota: 100,
                unlimited_quota: false,
                expired_time: -1,
                strict_token_budget: true,
              },
            ],
            total: 2,
          },
        },
      }
    }
    return {
      data: { success: true, data: [{ id: 'gpt-6.1-sol' }, { id: 'gpt-4o' }] },
    }
  })
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: { choices: [{ message: { content: 'done' } }] },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const rendered = render(
    <QueryClientProvider client={client}>
      <Playground />
    </QueryClientProvider>
  )
  return {
    post,
    userId,
    cleanup: () => {
      rendered.unmount()
      client.clear()
    },
  }
}

test('switching the selected key into strict mode and back preserves normal parameter preferences', async () => {
  const session = renderPlayground()
  const user = userEvent.setup()
  const key = await screen.findByRole('combobox', { name: 'API key' })
  await waitFor(() => expect(key).toHaveValue('11'))
  await user.click(screen.getByRole('button', { name: 'Parameters' }))
  expect(
    screen.getByRole('switch', { name: 'Enable Temperature' })
  ).toBeChecked()
  expect(screen.getByRole('spinbutton', { name: 'Max Tokens' })).toHaveValue(
    200000
  )
  expect(screen.getByRole('spinbutton', { name: 'Max Tokens' })).toBeDisabled()
  await user.keyboard('{Escape}')

  await user.selectOptions(key, '24')
  await user.click(screen.getByRole('button', { name: 'Parameters' }))
  expect(screen.getByRole('spinbutton', { name: 'Max Tokens' })).toHaveValue(
    128000
  )
  expect(screen.getByRole('spinbutton', { name: 'Max Tokens' })).toBeEnabled()
  expect(
    screen.getByRole('switch', { name: 'Enable Temperature' })
  ).not.toBeChecked()
  expect(
    screen.getByRole('switch', { name: 'Enable Temperature' })
  ).toHaveAttribute('aria-disabled', 'true')
  expect(screen.getByRole('alert')).toHaveTextContent('Strict Token budget')
  await user.keyboard('{Escape}')

  await user.selectOptions(key, '11')
  await user.click(screen.getByRole('button', { name: 'Parameters' }))
  expect(
    screen.getByRole('switch', { name: 'Enable Temperature' })
  ).toBeChecked()
  expect(
    screen.getByRole('switch', { name: 'Enable Temperature' })
  ).not.toHaveAttribute('aria-disabled', 'true')
  expect(screen.getByRole('switch', { name: 'Enable Seed' })).toBeChecked()
  expect(
    screen.getByRole('switch', { name: 'Enable Max Tokens' })
  ).not.toBeChecked()
  expect(screen.getByRole('spinbutton', { name: 'Max Tokens' })).toHaveValue(
    200000
  )
  expect(screen.getByRole('spinbutton', { name: 'Seed' })).toHaveValue(17)
  expect(screen.queryByText('Required')).not.toBeInTheDocument()
  expect(loadParameterEnabled(session.userId)).toEqual(preferredEnabled)
  expect(loadConfig(session.userId)).toMatchObject(preferredConfig)
  session.cleanup()
})

test('strict parameter display matches the submitted text payload without changing explicit key or model', async () => {
  const session = renderPlayground(24)
  const user = userEvent.setup()
  await waitFor(() =>
    expect(screen.getByRole('combobox', { name: 'API key' })).toHaveValue('24')
  )
  await user.click(screen.getByRole('button', { name: 'Parameters' }))
  const displayed = screen.getByRole('spinbutton', { name: 'Max Tokens' })
  expect(displayed).toHaveValue(128000)
  await user.keyboard('{Escape}')
  await user.type(screen.getByRole('textbox', { name: 'Message' }), 'Hello')
  await user.click(screen.getByRole('button', { name: 'Send' }))
  await waitFor(() => expect(session.post).toHaveBeenCalledOnce())
  expect(session.post.mock.calls[0]?.[1]).toEqual({
    model: 'gpt-6.1-sol',
    stream: false,
    messages: [{ role: 'user', content: 'Hello' }],
    max_completion_tokens: 128000,
    service_tier: 'default',
  })
  expect(session.post.mock.calls[0]?.[2]?.headers).toMatchObject({
    'X-MyAPI-Key-ID': '24',
  })
  expect(screen.getByRole('combobox', { name: 'API key' })).toHaveValue('24')
  session.cleanup()
})
