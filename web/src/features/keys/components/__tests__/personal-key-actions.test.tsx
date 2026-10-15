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
import {
  createRootRoute,
  createRoute,
  createRouter,
  createMemoryHistory,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { ApiKeysProvider } from '../api-keys-provider'
import { PersonalApiKeys } from '../personal-api-keys'

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn() },
}))
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))
const originalAuth = useAuthStore.getState()
const key = {
  id: 21,
  name: 'Desktop application',
  key: 'masked-value',
  status: 1,
  remain_quota: 123456,
  used_quota: 100,
  unlimited_quota: false,
  expired_time: -1,
  created_time: 1,
  accessed_time: 0,
  model_limits_enabled: false,
}
let client: QueryClient
let status: number
const listResponse = () => ({
  data: {
    success: true,
    data: { items: [{ ...key, status }], total: 1, page: 1, page_size: 10 },
  },
})
function renderKeys(initialEntry = '/keys') {
  const root = createRootRoute({ component: Outlet })
  const auth = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
    component: Outlet,
  })
  const keys = createRoute({
    getParentRoute: () => auth,
    path: 'keys/',
    validateSearch: (search) => ({
      filter: typeof search.filter === 'string' ? search.filter : '',
    }),
    component: PersonalApiKeys,
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([keys])]),
    history: createMemoryHistory({ initialEntries: [initialEntry] }),
  })
  render(
    <QueryClientProvider client={client}>
      <ApiKeysProvider>
        <RouterProvider router={router} />
      </ApiKeysProvider>
    </QueryClientProvider>
  )
  return router
}
beforeEach(() => {
  vi.resetAllMocks()
  status = 1
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  useAuthStore.setState((state) => ({
    ...state,
    auth: {
      ...state.auth,
      user: { id: 1, role: 100, username: 'owner' },
      session: null,
    },
  }))
  vi.mocked(api.get).mockImplementation(async (url) => {
    if (
      String(url).startsWith('/api/token/?') ||
      String(url).startsWith('/api/token/search?')
    ) {
      return listResponse()
    }
    if (url === '/api/token/21/subscription-share') {
      return { data: { success: true, data: { token_id: 21, pools: [] } } }
    }
    throw new Error(`Unexpected URL ${url}`)
  })
  vi.mocked(api.put).mockImplementation(async (_url, body) => {
    status = (body as { status: number }).status
    return { data: { success: true } }
  })
  vi.mocked(api.post).mockResolvedValue({
    data: { success: true, data: { key: 'inert-existing-key' } },
  })
})
afterEach(() => {
  client.clear()
  useAuthStore.setState(originalAuth)
})

test('existing Keys appear as cards with no financial balance or automatic Key creation', async () => {
  renderKeys()
  expect(await screen.findByText('Desktop application')).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'My usage policy' })
  ).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Create API Key' })).toBeVisible()
  expect(
    screen.queryByText(/remaining quota|used quota|balance|wallet|123456/i)
  ).not.toBeInTheDocument()
  expect(api.post).not.toHaveBeenCalled()
})

test('explicit pause and resume change only the existing Key status', async () => {
  const user = userEvent.setup()
  renderKeys()
  await user.click(await screen.findByRole('button', { name: 'Pause Key' }))
  expect(
    await screen.findByRole('button', { name: 'Resume Key' })
  ).toBeVisible()
  expect(api.put).toHaveBeenNthCalledWith(1, '/api/token/?status_only=true', {
    id: 21,
    status: 2,
  })
  await user.click(screen.getByRole('button', { name: 'Resume Key' }))
  expect(await screen.findByRole('button', { name: 'Pause Key' })).toBeVisible()
  expect(api.put).toHaveBeenNthCalledWith(2, '/api/token/?status_only=true', {
    id: 21,
    status: 1,
  })
  expect(api.post).not.toHaveBeenCalled()
})

test('copy reads the selected existing Key only after a user click', async () => {
  const user = userEvent.setup()
  const clipboard = vi.spyOn(navigator.clipboard, 'writeText')
  renderKeys()
  const copy = await screen.findByRole('button', { name: 'Copy' })
  expect(api.post).not.toHaveBeenCalled()
  await user.click(copy)
  await waitFor(() =>
    expect(clipboard).toHaveBeenCalledWith('sk-inert-existing-key')
  )
  expect(api.post).toHaveBeenCalledOnce()
  expect(api.post).toHaveBeenCalledWith('/api/token/21/key')
})

test('a failed Key list has a retry action and never fabricates an empty success', async () => {
  vi.mocked(api.get).mockRejectedValueOnce(new Error('network unavailable'))
  const user = userEvent.setup()
  renderKeys()
  expect(await screen.findByText('Failed to load API keys')).toBeVisible()
  expect(screen.queryByText('No API keys found')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByText('Desktop application')).toBeVisible()
})

test('unknown status writes require reloading current state before another toggle', async () => {
  vi.mocked(api.put).mockRejectedValueOnce(new Error('connection closed'))
  const user = userEvent.setup()
  renderKeys()
  await user.click(await screen.findByRole('button', { name: 'Pause Key' }))
  expect(
    await screen.findByText('Failed to update API key status')
  ).toBeVisible()
  expect(screen.getByRole('button', { name: 'Pause Key' })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Reload current state' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Pause Key' })).toBeEnabled()
  )
  expect(api.put).toHaveBeenCalledOnce()
})

test('a Key link from call logs applies its filter and same-route navigation updates the search', async () => {
  const router = renderKeys('/keys?filter=Desktop')
  expect(
    await screen.findByRole('textbox', { name: 'Search keys' })
  ).toHaveValue('Desktop')
  await waitFor(() =>
    expect(api.get).toHaveBeenCalledWith(
      '/api/token/search?keyword=Desktop&p=1&size=10'
    )
  )
  await act(async () => {
    await router.navigate({ to: '/keys', search: { filter: 'Shared device' } })
  })
  expect(screen.getByRole('textbox', { name: 'Search keys' })).toHaveValue(
    'Shared device'
  )
  await waitFor(() =>
    expect(api.get).toHaveBeenCalledWith(
      '/api/token/search?keyword=Shared+device&p=1&size=10'
    )
  )
})
