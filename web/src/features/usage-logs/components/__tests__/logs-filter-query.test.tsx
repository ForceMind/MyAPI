/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  getRouteApi,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import type { Table } from '@tanstack/react-table'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { useMediaQuery } from '@/hooks'
import { api } from '@/lib/api'
import { Route as usageLogsRoute } from '@/routes/_authenticated/usage-logs/$section'
import { useAuthStore } from '@/stores/auth-store'

import { fetchLogsByCategory } from '../../lib/utils'
import type { LogCategory } from '../../types'
import { CommonLogsFilterBar } from '../common-logs-filter-bar'
import { TaskLogsFilterBar } from '../task-logs-filter-bar'
import { UsageLogsProvider, useLogsViewScope } from '../usage-logs-provider'

vi.mock('@/hooks', async () => {
  const actual = await vi.importActual<typeof import('@/hooks')>('@/hooks')
  return { ...actual, useMediaQuery: vi.fn(() => false) }
})

const route = getRouteApi('/_authenticated/usage-logs/$section')
const table = { getAllColumns: () => [] } as unknown as Table<unknown>
const clients: QueryClient[] = []
const originalAuth = useAuthStore.getState()
const start = new Date(2026, 9, 12, 9, 10, 11)
const end = new Date(2026, 9, 13, 17, 18, 19)
const nextStart = new Date(2026, 9, 12, 10, 23, 45)
const nextEnd = new Date(2026, 9, 13, 20, 34, 56)
const endpoints = { common: '/api/log', drawing: '/api/mj', task: '/api/task' }

beforeEach(() => {
  vi.mocked(useMediaQuery).mockReturnValue(false)
})

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.setState(originalAuth)
  vi.restoreAllMocks()
  vi.useRealTimers()
})

/** Keep the actual route, filters, picker, query serializer and API path real. */
function FilterQueryFixture(props: { category: LogCategory }) {
  const search = route.useSearch()
  const { isAdminView } = useLogsViewScope()
  const query = useQuery({
    queryKey: ['logs', props.category, search, isAdminView],
    queryFn: () =>
      fetchLogsByCategory({
        logCategory: props.category,
        isAdmin: isAdminView,
        page: search.page ?? 1,
        pageSize: search.pageSize ?? 20,
        searchParams: search,
        columnFilters: [],
      }),
  })
  return (
    <>
      {props.category === 'common' ? (
        <CommonLogsFilterBar table={table} />
      ) : (
        <TaskLogsFilterBar table={table} logCategory={props.category} />
      )}
      <output aria-label='Query state'>{query.isSuccess ? 'Loaded' : 'Loading'}</output>
    </>
  )
}

async function renderFilters(category: LogCategory, mobile: boolean, role = 100) {
  vi.mocked(useMediaQuery).mockReturnValue(mobile)
  useAuthStore.getState().auth.setUser({ id: 1, username: 'owner', role })
  const requests: URL[] = []
  vi.spyOn(api, 'get').mockImplementation(async (path) => {
    requests.push(new URL(String(path), 'http://synthetic.invalid'))
    return {
      data: {
        success: true,
        data: { items: [], total: 0, quota: 0, rpm: 0, tpm: 0 },
      },
    }
  })
  const query = new URLSearchParams({
    page: '3',
    pageSize: '20',
    startTime: String(start.getTime()),
    endTime: String(end.getTime()),
    channel: JSON.stringify('7'),
  })
  if (category === 'common') {
    query.set('model', 'original-model')
    query.set('token', 'fixture-key')
    query.set('group', 'fixture-group')
    query.set('username', 'operator')
    query.set('requestId', 'REQ-EXACT-1')
    query.set('upstreamRequestId', 'UPSTREAM-EXACT-1')
    query.set('type', JSON.stringify(['5']))
    query.set('result', 'failed')
    query.set('tokenId', '21')
  } else {
    query.set('filter', 'original-task')
  }
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: Infinity } },
  })
  clients.push(client)
  const root = createRootRoute({ component: Outlet })
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated', component: Outlet })
  const logs = createRoute({
    getParentRoute: () => auth,
    path: 'usage-logs/$section',
    validateSearch: usageLogsRoute.options.validateSearch,
    component: () => <FilterQueryFixture category={category} />,
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([logs])]),
    history: createMemoryHistory({ initialEntries: [`/usage-logs/${category}?${query}`] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <UsageLogsProvider><RouterProvider router={router} /></UsageLogsProvider>
      </TooltipProvider>
    </QueryClientProvider>
  )
  await waitFor(() => expect(screen.getByLabelText('Query state')).toHaveTextContent('Loaded'))
  await waitFor(() => expect(screen.getByRole('button', { name: 'Search' })).toBeEnabled())
  const listRequests = () => requests.filter((url) => url.pathname === endpoints[category] || url.pathname === `${endpoints[category]}/self`)
  return { router, requests, listRequests, user: userEvent.setup() }
}

for (const category of ['common', 'drawing', 'task'] as const) {
  for (const mobile of [false, true]) {
    test(`${category} ${mobile ? 'phone' : 'desktop'} Apply stages seconds precisely, Cancel keeps that draft, and only Search queries`, async () => {
      const fixture = await renderFilters(category, mobile)
      const beforeHref = fixture.router.state.location.href
      const beforeQueries = fixture.listRequests().length
      const field = screen.getByLabelText(category === 'common' ? 'Model Name' : 'Task ID')
      await fixture.user.clear(field)
      await fixture.user.type(field, category === 'common' ? 'draft-model' : 'draft-task')
      await fixture.user.click(screen.getByRole('button', { name: /^Date Range:/ }))
      fireEvent.change(screen.getByLabelText('Start Time'), { target: { value: '10:23:45' } })
      fireEvent.change(screen.getByLabelText('End Time'), { target: { value: '20:34:56' } })
      await fixture.user.click(screen.getByRole('button', { name: 'Apply range' }))
      expect(fixture.router.state.location.href).toBe(beforeHref)
      expect(fixture.listRequests()).toHaveLength(beforeQueries)
      expect(screen.getByRole('button', { name: /^Date Range:/ })).toHaveAccessibleName(/2026-10-12 10:23:45.*2026-10-13 20:34:56/)

      await fixture.user.click(screen.getByRole('button', { name: /^Date Range:/ }))
      await fixture.user.click(screen.getByRole('button', { name: 'Clear range' }))
      await fixture.user.click(screen.getByRole('button', { name: 'Cancel' }))
      expect(fixture.router.state.location.href).toBe(beforeHref)
      expect(fixture.listRequests()).toHaveLength(beforeQueries)
      expect(screen.getByRole('button', { name: /^Date Range:/ })).toHaveAccessibleName(/2026-10-12 10:23:45.*2026-10-13 20:34:56/)

      const search = screen.getByRole('button', { name: 'Search' })
      if (mobile) expect(search).toHaveClass('min-h-11')
      await fixture.user.click(search)
      await waitFor(() => expect(fixture.router.state.location.search).toMatchObject({
        page: 1,
        pageSize: 20,
        startTime: nextStart.getTime(),
        endTime: nextEnd.getTime(),
        channel: '7',
      }))
      const multiplier = category === 'drawing' ? 1 : 1000
      await waitFor(() => expect(fixture.listRequests().at(-1)?.searchParams.get('start_timestamp')).toBe(String(nextStart.getTime() / multiplier)))
      const lastRequest = fixture.listRequests().at(-1)
      if (!lastRequest) throw new Error('Expected synthetic list request')
      const params = Object.fromEntries(lastRequest.searchParams)
      const expectedBase = {
        p: '1',
        page_size: '20',
        start_timestamp: String(nextStart.getTime() / multiplier),
        end_timestamp: String(nextEnd.getTime() / multiplier),
      }
      if (category === 'common') {
        expect(params).toEqual({
          ...expectedBase,
          type: '5', result: 'failed', model_name: 'draft-model',
          token_name: 'fixture-key', token_id: '21', group: 'fixture-group',
          username: 'operator', channel: '7', request_id: 'REQ-EXACT-1',
          upstream_request_id: 'UPSTREAM-EXACT-1',
        })
      } else {
        expect(params).toEqual({
          ...expectedBase, channel_id: '7',
          [category === 'drawing' ? 'mj_id' : 'task_id']: 'draft-task',
        })
      }
    })
  }
}

test.each(['common', 'drawing', 'task'] as const)('%s ordinary role keeps administrator filters unavailable in the mobile sheet', async (category) => {
  const fixture = await renderFilters(category, true, 1)
  expect(fixture.listRequests().every((url) => url.pathname.endsWith('/self'))).toBe(true)
  if (category === 'common') {
    expect(screen.getByLabelText('Request ID')).toHaveValue('REQ-EXACT-1')
    await fixture.user.click(screen.getByRole('button', { name: /^Filter/ }))
    const sheet = await screen.findByRole('dialog')
    expect(within(sheet).getByLabelText('Upstream Request ID')).toHaveValue('UPSTREAM-EXACT-1')
    expect(within(sheet).queryByLabelText('Username')).not.toBeInTheDocument()
    expect(within(sheet).queryByLabelText('Channel ID')).not.toBeInTheDocument()
  } else {
    expect(screen.queryByRole('button', { name: /^Filter/ })).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Channel ID')).not.toBeInTheDocument()
    expect(screen.getByLabelText('Task ID')).toHaveValue('original-task')
  }
})

test.each(['common', 'drawing', 'task'] as const)('%s phone Reset restores default dates and clears filters while preserving page size and selected Key identity', async (category) => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date(2026, 9, 30, 14))
  const fixture = await renderFilters(category, true)
  await fixture.user.click(screen.getByRole('button', { name: 'Reset' }))
  const expectedStart = new Date(2026, 9, 30).getTime()
  const expectedEnd = new Date(2026, 9, 30, 15).getTime()
  await waitFor(() => expect(fixture.router.state.location.search).toMatchObject({
    page: 1, pageSize: 20, startTime: expectedStart, endTime: expectedEnd,
  }))
  const search = fixture.router.state.location.search as Record<string, unknown>
  for (const key of ['channel', 'model', 'token', 'group', 'username', 'requestId', 'upstreamRequestId', 'filter', 'result']) {
    expect(search[key] || undefined).toBeUndefined()
  }
  const multiplier = category === 'drawing' ? 1 : 1000
  await waitFor(() => expect(fixture.listRequests().at(-1)?.searchParams.get('start_timestamp')).toBe(String(expectedStart / multiplier)))
  const lastRequest = fixture.listRequests().at(-1)
  if (!lastRequest) throw new Error('Expected synthetic list request')
  expect(Object.fromEntries(lastRequest.searchParams)).toEqual({
    p: '1', page_size: '20',
    start_timestamp: String(expectedStart / multiplier),
    end_timestamp: String(expectedEnd / multiplier),
    ...(category === 'common' ? { type: '0', token_id: '21' } : {}),
  })
})

test.each([false, true])('common %s Request ID is directly reachable and Enter queries that exact ID', async (mobile) => {
  const fixture = await renderFilters('common', mobile)
  const field = screen.getByLabelText('Request ID')
  await fixture.user.clear(field)
  await fixture.user.type(field, 'REQ-DIRECT-2{Enter}')

  await waitFor(() => expect(fixture.router.state.location.search).toMatchObject({
    page: 1, pageSize: 20, requestId: 'REQ-DIRECT-2',
  }))
  await waitFor(() => expect(fixture.listRequests().at(-1)?.searchParams.get('request_id')).toBe('REQ-DIRECT-2'))
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})

test.each(['common', 'drawing', 'task'] as const)('%s phone keeps an explicitly cleared start boundary absent after Search and reopening', async (category) => {
  const fixture = await renderFilters(category, true)
  const beforeQueries = fixture.listRequests().length
  await fixture.user.click(screen.getByRole('button', { name: /^Date Range:/ }))
  await fixture.user.click(screen.getByRole('button', { name: 'Clear start date' }))
  await fixture.user.click(screen.getByRole('button', { name: 'Apply range' }))
  expect(fixture.listRequests()).toHaveLength(beforeQueries)
  await fixture.user.click(screen.getByRole('button', { name: 'Search' }))

  await waitFor(() => expect(fixture.router.state.location.search).toMatchObject({ endTime: end.getTime() }))
  expect((fixture.router.state.location.search as Record<string, unknown>).startTime).toBeUndefined()
  await waitFor(() => expect(fixture.listRequests().at(-1)?.searchParams.has('start_timestamp')).toBe(false))
  expect(fixture.listRequests().at(-1)?.searchParams.get('end_timestamp')).toBe(String(end.getTime() / (category === 'drawing' ? 1 : 1000)))
  await fixture.user.click(screen.getByRole('button', { name: /^Date Range: No start date/ }))
  expect(screen.getByLabelText('Start Time')).toBeDisabled()
})


test.each(['common', 'drawing', 'task'] as const)('%s preserves an explicit Unix epoch start through URL, picker and query', async (category) => {
  const fixture = await renderFilters(category, false)
  await act(async () => { await fixture.router.navigate({
    to: '/usage-logs/$section', params: { section: category },
    search: { startTime: 0, endTime: 86_400_000, pageSize: 20 },
  }) })
  await waitFor(() => expect(screen.getByRole('button', { name: 'Search' })).toBeEnabled())
  await fixture.user.click(screen.getByRole('button', { name: /^Date Range:/ }))
  expect(screen.getByLabelText('Start Time')).toBeEnabled()
  await fixture.user.click(screen.getByRole('button', { name: 'Apply range' }))
  await fixture.user.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() => expect(fixture.listRequests().at(-1)?.searchParams.get('start_timestamp')).toBe('0'))
  expect(fixture.listRequests().at(-1)?.searchParams.get('end_timestamp')).toBe(category === 'drawing' ? '86400000' : '86400')
})


test.each(['common', 'drawing', 'task'] as const)('%s explicitly cleared range stays empty in the URL, picker and actual list query', async (category) => {
  const fixture = await renderFilters(category, true)
  const before = fixture.listRequests().length
  await fixture.user.click(screen.getByRole('button', { name: /^Date Range:/ }))
  await fixture.user.click(screen.getByRole('button', { name: 'Clear range' }))
  await fixture.user.click(screen.getByRole('button', { name: 'Apply range' }))
  expect(fixture.listRequests()).toHaveLength(before)
  await fixture.user.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() => expect(fixture.router.state.location.search).toMatchObject({ allTime: true, page: 1, pageSize: 20 }))
  const search = fixture.router.state.location.search as Record<string, unknown>
  expect(search.startTime).toBeUndefined()
  expect(search.endTime).toBeUndefined()
  await waitFor(() => expect(fixture.listRequests().at(-1)?.searchParams.has('start_timestamp')).toBe(false))
  expect(fixture.listRequests().at(-1)?.searchParams.has('end_timestamp')).toBe(false)
  expect(fixture.listRequests().at(-1)?.searchParams.has('allTime')).toBe(false)
  await fixture.user.click(screen.getByRole('button', { name: /^Date Range/ }))
  expect(screen.getByLabelText('Start Time')).toBeDisabled()
  expect(screen.getByLabelText('End Time')).toBeDisabled()
})
