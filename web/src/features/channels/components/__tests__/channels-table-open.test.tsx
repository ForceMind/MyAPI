/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient } from '@tanstack/react-query'
import { act, cleanup, screen, waitFor } from '@testing-library/react'
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
vi.mock('@/hooks/use-sidebar-config', () => ({ useIsSidebarModuleVisible: () => true }))

let tagMode = false
const originalAuth = useAuthStore.getState()
let client: QueryClient

beforeEach(() => {
  localStorage.clear()
  tagMode = false
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
          data: { items: tagMode ? [{ ...item, tag: 'synthetic-tag' }, { ...item, id: 24, name: 'second-channel', tag: 'synthetic-tag' }] : [item], total: tagMode ? 1 : 220, type_counts: { 1: 220 } },
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

async function renderTable(entry?: string) {
  return renderListRoute({
    client,
    path: '/channels/',
    entry,
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


test('table pagination can reach the last page and changing page size resets to the first page', async () => {
  const { router } = await renderTable('/channels/?pageSize=20&filter=needle&model=public-model')
  const user = userEvent.setup()
  await screen.findByRole('link', { name: 'needle-channel' })
  await user.click(screen.getByRole('button', { name: 'Last page' }))
  await waitFor(() => expect(router.state.location.search).toMatchObject({ page: 11, pageSize: 20, filter: 'needle', model: 'public-model' }))
  await waitFor(() => expect(api.get).toHaveBeenCalledWith('/api/channel/search', { params: expect.objectContaining({ p: 11, page_size: 20 }) }))
  await user.selectOptions(await screen.findByRole('combobox', { name: 'Rows per page' }), '50')
  await waitFor(() => expect(router.state.location.search).toMatchObject({ pageSize: 50, filter: 'needle', model: 'public-model' }))
  await waitFor(() => expect(api.get).toHaveBeenCalledWith('/api/channel/search', { params: expect.objectContaining({ p: 1, page_size: 50 }) }))
})

test('table details carry filters, page size and table origin without invoking a supplier', async () => {
  const { router } = await renderTable('/channels/?page=3&pageSize=50&filter=needle&model=public-model&status=%5B%22enabled%22%5D&type=%5B%221%22%5D&group=%5B%22default%22%5D&quotaChannelId=23')
  await userEvent.setup().click(await screen.findByRole('link', { name: 'needle-channel' }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/connections/23'))
  expect(router.state.location.search).toEqual({ connectionReturn: {
    view: 'table', page: 3, pageSize: 50, filter: 'needle', model: 'public-model', status: 'enabled', type: ['1'], group: ['default'], quotaChannelId: 23,
  } })
  expect(vi.mocked(api.get).mock.calls.every(([url]) => !String(url).includes('/test/'))).toBe(true)
})

test('tag mode uses the number of tags even when a tag contains several channel rows', async () => {
  tagMode = true
  localStorage.setItem('enable-tag-mode', 'true')
  await renderTable()
  expect(await screen.findByText(/^Tag.*synthetic-tag$/)).toBeVisible()
  expect(screen.queryByText('Failed to load channels')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Last page' })).toBeDisabled()
  expect(screen.getByRole('button', { name: /synthetic-tag/ })).toHaveAttribute('aria-expanded', 'false')
})

test('read-only channel permissions disable editing, quick tests, routing switches and advanced tests', async () => {
  useAuthStore.getState().auth.setUser({ id: 2, role: 10, username: 'reader', permissions: { admin_permissions: { channel: { read: true, write: false, operate: false } } } })
  await renderTable()
  await screen.findByRole('link', { name: 'needle-channel' })
  expect(screen.getByRole('button', { name: 'Edit' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Test Connection' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Disable' })).toBeDisabled()
  await userEvent.setup().click(screen.getByRole('button', { name: 'Open menu' }))
  expect(await screen.findByRole('menuitem', { name: 'Advanced test' })).toHaveAttribute('aria-disabled', 'true')
  expect(vi.mocked(api.get).mock.calls.every(([url]) => !String(url).includes('/test/'))).toBe(true)
})

test('a direct return focuses its row once and a later refresh preserves the user input focus', async () => {
  await renderTable('/channels/?focusConnectionId=23')
  const name = await screen.findByRole('link', { name: 'needle-channel' })
  await waitFor(() => expect(name).toHaveFocus())
  const input = screen.getByPlaceholderText('Filter by model...')
  input.focus()
  await act(async () => client.invalidateQueries({ queryKey: ['channels'] }))
  await screen.findByRole('link', { name: 'needle-channel' })
  expect(input).toHaveFocus()
})


test('revoking channel read removes saved rows and prevents another inventory request', async () => {
  await renderTable()
  await screen.findByRole('link', { name: 'needle-channel' })
  const reads = () => vi.mocked(api.get).mock.calls.filter(([url]) => url === '/api/channel' || url === '/api/channel/search').length
  const before = reads()
  await act(async () => useAuthStore.getState().auth.setUser({ id: 1, role: 10, username: 'revoked', permissions: { admin_permissions: { channel: { read: false } } } }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Connection access could not be verified.')
  expect(screen.queryByRole('link', { name: 'needle-channel' })).not.toBeInTheDocument()
  expect(reads()).toBe(before)
})

test('changing account clears the previous inventory while the new account read is pending', async () => {
  await renderTable()
  await screen.findByRole('link', { name: 'needle-channel' })
  const originalGet = vi.mocked(api.get).getMockImplementation()
  let release = () => {}
  const gate = new Promise<void>((resolve) => { release = resolve })
  vi.mocked(api.get).mockImplementation(async (url, config) => {
    if (url === '/api/channel' || url === '/api/channel/search') {
      await gate
      return { data: { success: true, data: { items: [{ ...item, id: 31, name: 'second-account-channel' }], total: 1 } } }
    }
    if (!originalGet) throw new Error('Missing original API fixture')
    return originalGet(url, config)
  })
  await act(async () => useAuthStore.getState().auth.setUser({ id: 2, role: 100, username: 'second-root' }))
  expect(screen.queryByRole('link', { name: 'needle-channel' })).not.toBeInTheDocument()
  await act(async () => release())
  expect(await screen.findByRole('link', { name: 'second-account-channel' })).toBeVisible()
  expect(screen.queryByRole('link', { name: 'needle-channel' })).not.toBeInTheDocument()
})
