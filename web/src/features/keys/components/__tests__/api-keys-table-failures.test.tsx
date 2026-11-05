/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient } from '@tanstack/react-query'
import { act, cleanup, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Toaster } from 'sonner'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { apiKeySchema } from '../../types'
import { ApiKeysProvider } from '../api-keys-provider'
import { ApiKeysTable } from '../api-keys-table'

const item = apiKeySchema.parse({
  id: 21,
  name: 'needle-key',
  key: 'masked',
  status: 1,
  remain_quota: 100,
  used_quota: 0,
  unlimited_quota: false,
  expired_time: -1,
  created_time: 1,
  accessed_time: 1,
  group: 'default',
  model_limits_enabled: false,
})
const originalAuth = useAuthStore.getState()
const originalConfig = useSystemConfigStore.getState()
let client: QueryClient
let expiry: number | null
let outcome: 'success' | 'empty' | 'network' | 'envelope'
let reportedTotal: number | undefined
let pendingRead: Promise<void> | undefined

beforeEach(() => {
  localStorage.clear()
  vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected expiry write'))
  vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true, data: { key: 'synthetic-only' } } })
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  outcome = 'success'
  expiry = -1
  reportedTotal = 100
  pendingRead = undefined
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, role: 100, username: 'root-fixture' })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/status') return { data: { success: true, data: {} } }
    if (url === '/api/user/self/groups') {
      return {
        data: {
          success: true,
          data: { default: { desc: 'Default', ratio: 1 } },
        },
      }
    }
    if (
      url.startsWith('/api/token/?') ||
      url.startsWith('/api/token/search?')
    ) {
      if (pendingRead) await pendingRead
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
            items: outcome === 'empty' ? [] : [{ ...item, expired_time: expiry }],
            ...(reportedTotal === undefined ? {} : { total: outcome === 'empty' ? 0 : reportedTotal }),
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

function renderTable(entry = '/keys/') {
  return renderListRoute({
    client,
    path: '/keys/',
    entry,
    element: (
      <>
        <ApiKeysProvider>
          <ApiKeysTable />
        </ApiKeysProvider>
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
        // The Keys page switches to its card list with the console shell.
        matches: query === CONSOLE_MOBILE_QUERY,
      }))
    }
    outcome = failure
    await renderTable()
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Failed to load API keys'
    )
    expect(screen.getByRole('button', { name: 'Retry' })).toBeEnabled()
    expect(screen.getByPlaceholderText('Filter by name...')).toBeEnabled()
    expect(screen.queryByText('No API Keys Found')).not.toBeInTheDocument()
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
    '/keys/?page=3&pageSize=20&filter=needle&token=masked'
  )
  const retry = await screen.findByRole('button', { name: 'Retry' })
  const searchBeforeRetry = router.state.location.search
  outcome = 'success'
  retry.focus()
  await user.keyboard('{Enter}')
  expect(await screen.findByText('needle-key')).toBeVisible()
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Go to next page' })).toBeVisible()
  expect(router.state.location.search).toEqual(searchBeforeRetry)
  expect(api.get).toHaveBeenCalledWith(
    '/api/token/search?keyword=needle&token=masked&p=3&size=20'
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
    expect(await screen.findByText('needle-key')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Open menu' }))
    expect(await screen.findByRole('menu')).toBeVisible()
    outcome = failure
    await act(async () => client.invalidateQueries({ queryKey: ['keys'] }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Failed to load API keys'
    )
    expect(screen.queryByText('needle-key')).not.toBeInTheDocument()
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
  expect(await screen.findByText('No API Keys Found')).toBeVisible()
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Retry' })
  ).not.toBeInTheDocument()
})


test('shared pagination reaches the last page and changing size resets the page while retaining search', async () => {
  const user = userEvent.setup()
  const { router } = await renderTable('/keys/?page=2&pageSize=20&filter=needle&token=masked')
  await screen.findByText('needle-key')
  await user.click(screen.getByRole('button', { name: 'Last page' }))
  await waitFor(() => expect(api.get).toHaveBeenCalledWith('/api/token/search?keyword=needle&token=masked&p=5&size=20'))
  await waitFor(() => expect(router.state.location.search).toMatchObject({ page: 5, pageSize: 20, filter: 'needle', token: 'masked' }))
  await user.selectOptions(screen.getByRole('combobox', { name: 'Rows per page' }), '50')
  await waitFor(() => expect(api.get).toHaveBeenCalledWith('/api/token/search?keyword=needle&token=masked&p=1&size=50'))
  expect(router.state.location.search).toMatchObject({ pageSize: 50, filter: 'needle', token: 'masked' })
  expect(screen.getByRole('button', { name: 'First page' })).toBeDisabled()
})

test.each([undefined, -1, Number.NaN, 0])('an unknown or inconsistent total %s retains real rows but disables page navigation', async (total) => {
  reportedTotal = total
  await renderTable()
  expect(await screen.findByText('needle-key')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Last page' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Go to next page' })).toBeDisabled()
  expect(screen.getByRole('navigation', { name: 'API Keys' })).toHaveTextContent('Record total unavailable')
})

test('mobile cards retain model restrictions, credential masking, selection and advanced fields without reading a full Key', async () => {
  const matchMedia = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({ ...matchMedia(query), matches: query === CONSOLE_MOBILE_QUERY }))
  const { router } = await renderTable()
  const name = await screen.findByText('needle-key')
  const card = name.closest('article')
  expect(card).not.toBeNull()
  expect(card).toHaveTextContent('Models')
  expect(card).toHaveTextContent('API Key')
  expect(card).toHaveTextContent('Last used')
  expect(screen.getByText('Expires')).toBeVisible()
  expect(screen.getAllByText('Expires')).toHaveLength(1)
  expect(screen.getByRole('checkbox', { name: 'Select row' })).toBeEnabled()
  await userEvent.setup().click(screen.getByText('More details'))
  expect(screen.getByText('IP Restriction')).toBeVisible()
  expect(screen.getByText('Expires')).toBeVisible()
  expect(router.state.location.pathname).toBe('/keys/')
  await userEvent.setup().click(screen.getByRole('checkbox', { name: 'Select row' }))
  expect(screen.getByRole('button', { name: 'Copy selected keys' })).toBeVisible()
  expect(screen.getByRole('button', { name: 'Delete selected API keys' })).toBeVisible()
  expect(api.post).not.toHaveBeenCalled()
})

test('switching accounts clears old inventory until the new read completes', async () => {
  await renderTable()
  await screen.findByText('needle-key')
  let release = () => {}
  pendingRead = new Promise<void>((resolve) => { release = resolve })
  await act(async () => useAuthStore.getState().auth.setUser({ id: 2, role: 100, username: 'second-owner' }))
  expect(screen.queryByText('needle-key')).not.toBeInTheDocument()
  await act(async () => release())
  expect(await screen.findByText('needle-key')).toBeVisible()
})

test('a page-size bookmark above the outgoing bound is retained in the URL but requests at most 100 rows', async () => {
  const { router } = await renderTable('/keys/?pageSize=1000')
  await screen.findByText('needle-key')
  expect(api.get).toHaveBeenCalledWith('/api/token/?p=1&size=100')
  expect(router.state.location.search).toMatchObject({ pageSize: 1000 })
})


test('selection follows stable Key IDs across pages and never selects the same row position on another page', async () => {
  const originalGet = vi.mocked(api.get).getMockImplementation()
  vi.mocked(api.get).mockImplementation(async (url, config) => {
    if (url.startsWith('/api/token/?') || url.startsWith('/api/token/search?')) {
      const page = new URL(String(url), 'https://example.test').searchParams.get('p')
      return { data: { success: true, data: { items: [{ ...item, id: page === '2' ? 22 : 21, name: page === '2' ? 'second-key' : 'needle-key' }], total: 100 } } }
    }
    if (!originalGet) throw new Error('Missing API fixture')
    return originalGet(url, config)
  })
  await renderTable()
  const user = userEvent.setup()
  await screen.findByText('needle-key')
  await user.click(screen.getByRole('checkbox', { name: 'Select row' }))
  expect(screen.getByRole('button', { name: 'Copy selected keys' })).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Go to next page' }))
  await screen.findByText('second-key')
  expect(screen.getByRole('checkbox', { name: 'Select row' })).not.toBeChecked()
  expect(screen.queryByRole('button', { name: 'Delete selected API keys' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Go to previous page' }))
  await screen.findByText('needle-key')
  expect(screen.getByRole('checkbox', { name: 'Select row' })).toBeChecked()
})


test('a known empty response corrects a bookmarked page to the first page instead of displaying page nine of one', async () => {
  outcome = 'empty'
  const { router } = await renderTable('/keys/?page=9&pageSize=20')
  await screen.findByText('No API Keys Found')
  await waitFor(() => expect(router.state.location.search.page).toBeUndefined())
  expect(screen.getByRole('navigation', { name: 'API Keys' })).toHaveTextContent('Page 1 of 1')
})


test('a periodic list timestamp refresh preserves an explicitly opened credential popover', async () => {
  const matchMedia = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({ ...matchMedia(query), matches: query === CONSOLE_MOBILE_QUERY }))
  const intervals = vi.spyOn(window, 'setInterval')
  await renderTable()
  await screen.findByText('needle-key')
  await userEvent.setup().click(screen.getByRole('button', { name: 'sk-masked' }))
  expect(await screen.findByText('Full API Key')).toBeVisible()
  await waitFor(() => expect(api.post).toHaveBeenCalledTimes(1))
  const tick = intervals.mock.calls.find((call) => call[1] === 30_000)?.[0]
  expect(typeof tick).toBe('function')
  vi.spyOn(Date, 'now').mockReturnValue(Date.now() + 30_000)
  await act(async () => { if (typeof tick === 'function') tick() })
  expect(screen.getByText('Full API Key')).toBeVisible()
  expect(api.post).toHaveBeenCalledTimes(1)
})


test.each([-1, 0, 1700000000, Number.MAX_SAFE_INTEGER])('default desktop expiry %s stays visible without credential reads or writes', async (value) => {
  expiry = value
  await renderTable()
  const row = (await screen.findByText('needle-key')).closest('tr')
  if (!row) throw new Error('Expected Key table row')
  expect(screen.getByRole('columnheader', { name: /Expires/ })).toBeVisible()
  if (value === -1) expect(within(row).getByText('Never')).toBeVisible()
  else if (value === Number.MAX_SAFE_INTEGER) expect(within(row).getByText('Unknown')).toBeVisible()
  else {
    const expected = value === 0 ? '1970-01-01T00:00:00.000Z' : new Date(value * 1000).toISOString()
    const timestamp = row.querySelector(`time[datetime="${expected}"]`)
    expect(timestamp).toBeVisible()
    expect(timestamp).toHaveClass('text-destructive')
  }
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
})

test('an explicitly hidden expiry column remains the user’s custom preference', async () => {
  localStorage.setItem('api-keys:column-visibility', JSON.stringify({ expired_time: false, group: true }))
  await renderTable()
  await screen.findByText('needle-key')
  expect(screen.queryByRole('columnheader', { name: /Expires/ })).not.toBeInTheDocument()
  expect(screen.getByRole('columnheader', { name: 'Access profile' })).toBeVisible()
  expect(JSON.parse(localStorage.getItem('api-keys:column-visibility') ?? '{}')).toMatchObject({ expired_time: false, group: true })
})

test('a malformed null expiry does not masquerade as a never-expiring Key', async () => {
  expiry = null
  await renderTable()
  expect(await screen.findByRole('alert')).toHaveTextContent('Failed to load API keys')
  expect(screen.queryByText('Never')).not.toBeInTheDocument()
  expect(api.post).not.toHaveBeenCalled()
})

test.each([-1, 0, 1700000000, Number.MAX_SAFE_INTEGER])('mobile expiry %s stays on the first screen exactly once without opening More details', async (value) => {
  expiry = value
  const matchMedia = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({ ...matchMedia(query), matches: query === CONSOLE_MOBILE_QUERY }))
  await renderTable()
  const card = (await screen.findByText('needle-key')).closest('article')
  if (!card) throw new Error('Expected mobile Key card')
  expect(screen.getByText('Expires')).toBeVisible()
  expect(screen.getAllByText('Expires')).toHaveLength(1)
  const details = screen.getByText('More details').closest('details')
  if (!details) throw new Error('Expected supplementary Key fields')
  expect(details).not.toHaveAttribute('open')
  expect(within(details).queryByText('Expires')).not.toBeInTheDocument()
  if (value === -1) expect(screen.getByText('Never')).toBeVisible()
  if (value === Number.MAX_SAFE_INTEGER) expect(within(card).getByText('Unknown')).toBeVisible()
  if (value !== -1 && value !== Number.MAX_SAFE_INTEGER) {
    const expected = value === 0 ? '1970-01-01T00:00:00.000Z' : new Date(value * 1000).toISOString()
    const timestamp = card.querySelector(`time[datetime="${expected}"]`)
    expect(timestamp).toBeVisible()
    expect(timestamp).toHaveClass('text-destructive')
  }
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
})
