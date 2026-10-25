/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient } from '@tanstack/react-query'
import { cleanup, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { channelSchema } from '../../types'
import { ChannelsProvider } from '../channels-provider'
import { ChannelsTable } from '../channels-table'

const item = channelSchema.parse({
  id: 23,
  type: 1,
  name: 'needle-channel',
  key: '',
  status: 1,
  created_time: 1,
  test_time: 0,
  response_time: 0,
  balance_updated_time: 0,
  models: 'public-model',
})
const originalAuth = useAuthStore.getState()
let client: QueryClient

beforeEach(() => {
  localStorage.clear()
  // The table view (not the card view) of the connections' table page.
  localStorage.setItem('channels:view-mode', 'table')
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, role: 100, username: 'root-fixture' })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/status') return { data: { success: true, data: {} } }
    if (url === '/api/group/') {
      return { data: { success: true, data: ['default'] } }
    }
    if (url === '/api/channel' || url === '/api/channel/search') {
      return {
        data: {
          success: true,
          data: { items: [item], total: 1, type_counts: { 1: 1 } },
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
  localStorage.clear()
  vi.restoreAllMocks()
})

async function renderTable() {
  return renderListRoute({
    client,
    path: '/channels/',
    element: (
      <ChannelsProvider>
        <ChannelsTable />
      </ChannelsProvider>
    ),
  })
}

test('the channel name opens its connection page', async () => {
  const { router } = await renderTable()
  const user = userEvent.setup()
  await user.click(await screen.findByRole('link', { name: 'needle-channel' }))
  await waitFor(() =>
    expect(router.state.location.pathname).toBe('/connections/23')
  )
})

test('a click elsewhere on the row opens the connection page, but not on its controls', async () => {
  const { router } = await renderTable()
  const user = userEvent.setup()
  const name = await screen.findByRole('link', { name: 'needle-channel' })
  const row = name.closest('tr') as HTMLTableRowElement
  // The row's own controls (menu, switches) act in place.
  await user.click(screen.getByRole('button', { name: 'Open menu' }))
  expect(router.state.location.pathname).toBe('/channels/')
  await user.keyboard('{Escape}')
  const plainCell = [...row.querySelectorAll('td')].find(
    (cell) => !cell.querySelector('button,a,input,[role="switch"]')
  ) as HTMLElement
  await user.click(plainCell)
  await waitFor(() =>
    expect(router.state.location.pathname).toBe('/connections/23')
  )
})
