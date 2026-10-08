/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient } from '@tanstack/react-query'
import { act, cleanup, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Toaster } from 'sonner'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { userSchema } from '../../types'
import { UsersProvider } from '../users-provider'
import { UsersTable } from '../users-table'

const item = userSchema.parse({
  id: 22,
  username: 'needle-user',
  display_name: 'List fixture',
  quota: 100,
  used_quota: 0,
  request_count: 0,
  group: 'default',
  status: 1,
  role: 1,
})
const originalAuth = useAuthStore.getState()
const originalConfig = useSystemConfigStore.getState()
let client: QueryClient
let outcome: 'success' | 'empty' | 'network' | 'envelope'

beforeEach(() => {
  localStorage.clear()
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  outcome = 'success'
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, role: 100, username: 'root-fixture' })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/status') return { data: { success: true, data: {} } }

    if (url === '/api/user/' || url.startsWith('/api/user/search?')) {
      if (outcome === 'network') throw new Error('private transport diagnostic')
      if (outcome === 'envelope') {
        return {
          data: { success: false, message: 'private server diagnostic' },
        }
      }
      return {
        data: {
          success: true,
          data: {
            items: outcome === 'empty' ? [] : [item],
            total: outcome === 'empty' ? 0 : 100,
          },
        },
      }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.setState(originalAuth)
  useSystemConfigStore.setState(originalConfig)
  localStorage.clear()
  vi.restoreAllMocks()
})

function renderTable(entry = '/users/') {
  return renderListRoute({
    client,
    path: '/users/',
    entry,
    element: (
      <>
        <UsersProvider>
          <UsersTable />
        </UsersProvider>
        <Toaster />
      </>
    ),
  })
}

test.each([
  { failure: 'network', layout: 'desktop' },
  { failure: 'envelope', layout: 'desktop' },
  { failure: 'network', layout: 'mobile' },
  { failure: 'envelope', layout: 'mobile' },
] as const)(
  'a $failure failure on $layout is an error with retry rather than a successful empty list',
  async ({ failure, layout }) => {
    if (layout === 'mobile') {
      const matchMedia = window.matchMedia
      vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
        ...matchMedia(query),
        matches: query === '(max-width: 640px)',
      }))
    }
    outcome = failure
    await renderTable()
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Failed to load users'
    )
    expect(screen.getByRole('button', { name: 'Retry' })).toBeEnabled()
    expect(
      screen.getByPlaceholderText('Filter by username, name or email...')
    ).toBeEnabled()
    expect(screen.queryByText('No Users Found')).not.toBeInTheDocument()
    expect(screen.queryByText(/private .* diagnostic/)).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Go to next page' })
    ).not.toBeInTheDocument()
  }
)

test('keyboard retry restores actual data without changing the selected search and page', async () => {
  const user = userEvent.setup()
  outcome = 'envelope'
  const { router } = await renderTable(
    '/users/?page=3&pageSize=20&filter=needle&group=default'
  )
  const retry = await screen.findByRole('button', { name: 'Retry' })
  const searchBeforeRetry = router.state.location.search
  outcome = 'success'
  retry.focus()
  await user.keyboard('{Enter}')
  expect(await screen.findByText('needle-user')).toBeVisible()
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Go to next page' })).toBeVisible()
  expect(router.state.location.search).toEqual(searchBeforeRetry)
  expect(api.get).toHaveBeenCalledWith(
    '/api/user/search?keyword=needle&group=default&p=3&page_size=20'
  )
})

test.each(['network', 'envelope'] as const)(
  'a %s refresh failure removes stale rows and their open action menu',
  async (failure) => {
    const user = userEvent.setup()
    vi.spyOn(api, 'post').mockResolvedValue({
      data: { success: true, data: { key: 'synthetic-only' } },
    })
    await renderTable()
    expect(await screen.findByText('needle-user')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Open menu' }))
    expect(await screen.findByRole('menu')).toBeVisible()
    outcome = failure
    await act(async () => client.invalidateQueries({ queryKey: ['users'] }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Failed to load users'
    )
    expect(screen.queryByText('needle-user')).not.toBeInTheDocument()
    await waitFor(() =>
      expect(screen.queryByRole('menu')).not.toBeInTheDocument()
    )
    expect(
      screen.queryByRole('button', { name: 'Open menu' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Go to next page' })
    ).not.toBeInTheDocument()
  }
)

test('a successful empty response still displays the real empty state', async () => {
  outcome = 'empty'
  await renderTable()
  expect(await screen.findByText('No Users Found')).toBeVisible()
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Retry' })
  ).not.toBeInTheDocument()
})
