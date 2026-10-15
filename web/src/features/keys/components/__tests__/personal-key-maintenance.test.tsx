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
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { apiKeySchema, type ApiKey } from '../../types'
import { ApiKeysDialogs } from '../api-keys-dialogs'
import { ApiKeysProvider } from '../api-keys-provider'
import { PersonalApiKeys } from '../personal-api-keys'

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}))

const originalAuth = useAuthStore.getState()
const originalConfig = useSystemConfigStore.getState()
const existingKey = apiKeySchema.parse({
  id: 21,
  name: 'Desktop application',
  key: 'masked-value',
  status: 1,
  remain_quota: 100,
  used_quota: 0,
  unlimited_quota: true,
  expired_time: -1,
  created_time: 1,
  accessed_time: 0,
  group: 'default',
  model_limits_enabled: false,
})
let client: QueryClient
let keys: ApiKey[]

function renderKeys() {
  const root = createRootRoute({ component: Outlet })
  const auth = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
    component: Outlet,
  })
  const keyRoute = createRoute({
    getParentRoute: () => auth,
    path: 'keys/',
    validateSearch: (search) => ({
      filter: typeof search.filter === 'string' ? search.filter : '',
    }),
    component: PersonalApiKeys,
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([keyRoute])]),
    history: createMemoryHistory({ initialEntries: ['/keys'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <ApiKeysProvider>
          <RouterProvider router={router} />
          <ApiKeysDialogs />
        </ApiKeysProvider>
      </TooltipProvider>
    </QueryClientProvider>
  )
}

beforeEach(() => {
  localStorage.clear()
  keys = [existingKey]
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
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (String(url).startsWith('/api/token/?')) {
      return {
        data: {
          success: true,
          data: { items: keys, total: keys.length, page: 1, page_size: 10 },
        },
      }
    }
    const share = String(url).match(/^\/api\/token\/(\d+)\/subscription-share$/)
    if (share) {
      return {
        data: {
          success: true,
          data: { token_id: Number(share[1]), pools: [] },
        },
      }
    }
    if (url === '/api/status') {
      return {
        data: {
          success: true,
          data: {
            chats: [],
            default_use_auto_group: false,
            quota_display_type: 'TOKENS',
          },
        },
      }
    }
    if (url === '/api/user/models') {
      return { data: { success: true, data: [] } }
    }
    if (url === '/api/user/self/groups') {
      return {
        data: {
          success: true,
          data: { default: { desc: 'Standard', ratio: 1 } },
        },
      }
    }
    if (url === '/api/token/auto-groups') {
      return {
        data: { success: true, data: { groups: ['default'], max_count: 5 } },
      }
    }
    if (url === '/api/token/21') {
      return { data: { success: true, data: existingKey } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  vi.spyOn(api, 'post').mockImplementation(async (url, body) => {
    if (url === '/api/token/') {
      const input = body as { name: string }
      const created = { ...existingKey, id: 22, name: input.name }
      keys = [...keys, created]
      return { data: { success: true, data: created } }
    }
    if (url === '/api/token/21/key') {
      return { data: { success: true, data: { key: 'inert-existing-key' } } }
    }
    throw new Error(`Unexpected POST ${url}`)
  })
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
  vi.spyOn(api, 'delete').mockResolvedValue({ data: { success: true } })
})

afterEach(() => {
  client.clear()
  useAuthStore.setState(originalAuth)
  useSystemConfigStore.setState(originalConfig)
  localStorage.clear()
  vi.restoreAllMocks()
})

test('creating a Key through the retained drawer refreshes the personal cards without reloading the page', async () => {
  const user = userEvent.setup()
  renderKeys()
  await user.click(
    await screen.findByRole('button', { name: 'Create API Key' })
  )
  const drawer = await screen.findByRole('dialog', { name: 'Create API Key' })
  const save = within(drawer).getByRole('button', { name: 'Save changes' })
  await waitFor(() => expect(save).toBeEnabled())
  await user.type(
    within(drawer).getByRole('textbox', { name: 'Name' }),
    'Shared device'
  )
  await user.click(save)
  expect(await screen.findByText('Shared device')).toBeVisible()
  expect(api.post).toHaveBeenCalledWith(
    '/api/token/',
    expect.objectContaining({ name: 'Shared device' }),
    { skipErrorHandler: true, skipBusinessError: true, skipAuthRefresh: true }
  )
  expect(screen.getByText('Desktop application')).toBeVisible()
})

test('advanced edit uses the existing Key drawer and Escape cancels without writing', async () => {
  const user = userEvent.setup()
  renderKeys()
  await user.click(await screen.findByText('Advanced options'))
  await user.click(await screen.findByRole('button', { name: 'Edit' }))
  const drawer = await screen.findByRole('dialog', { name: 'Update API Key' })
  await waitFor(() =>
    expect(
      within(drawer).getByRole('button', { name: 'Save changes' })
    ).toBeEnabled()
  )
  const name = within(drawer).getByRole('textbox', { name: 'Name' })
  expect(name).toHaveValue('Desktop application')
  await user.clear(name)
  await user.type(name, 'Uncommitted name')
  await user.keyboard('{Escape}')
  await waitFor(() =>
    expect(
      screen.queryByRole('dialog', { name: 'Update API Key' })
    ).not.toBeInTheDocument()
  )
  expect(screen.getByText('Desktop application')).toBeVisible()
  expect(api.put).not.toHaveBeenCalled()
  expect(api.delete).not.toHaveBeenCalled()
})

test('advanced options retain assigned access, budgets, client export and deletion controls', async () => {
  const user = userEvent.setup()
  renderKeys()
  await user.click(await screen.findByText('Advanced options'))
  expect(
    await screen.findByRole('button', { name: 'API Key usage budgets' })
  ).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Open menu' }))
  expect(
    await screen.findByRole('menuitem', { name: 'Assigned access' })
  ).toBeVisible()
  expect(screen.getByRole('menuitem', { name: 'CC Switch' })).toBeVisible()
  expect(screen.getByRole('menuitem', { name: 'Delete' })).toBeVisible()
})

test('cancelling the retained delete confirmation keeps the existing Key and sends no deletion', async () => {
  const user = userEvent.setup()
  renderKeys()
  await user.click(await screen.findByText('Advanced options'))
  await user.click(await screen.findByRole('button', { name: 'Open menu' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
  const confirmation = await screen.findByRole('alertdialog', {
    name: 'Are you sure?',
  })
  await user.click(within(confirmation).getByRole('button', { name: 'Cancel' }))
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  )
  expect(screen.getByText('Desktop application')).toBeVisible()
  expect(api.delete).not.toHaveBeenCalled()
})
