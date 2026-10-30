/*
Copyright (C) 2026 ForceMind
SPDX-License-Identifier: AGPL-3.0-or-later
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRoute, createRouter, getRouteApi, Outlet, RouterProvider } from '@tanstack/react-router'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { useMediaQuery } from '@/hooks'
import { api } from '@/lib/api'
import { Route as logsRoute } from '@/routes/_authenticated/usage-logs/$section'
import { useAuthStore } from '@/stores/auth-store'

import { UsageLogsTable } from '../components/usage-logs-table'
import { UsageLogsProvider } from '../components/usage-logs-provider'
import type { LogCategory } from '../types'

vi.mock('@/hooks', async () => {
  const actual = await vi.importActual<typeof import('@/hooks')>('@/hooks')
  return { ...actual, useMediaQuery: vi.fn(() => false) }
})
// Positioning is outside this non-browser query/navigation regression.
vi.mock('@/components/layout/components/page-footer', () => ({
  PageFooterPortal: ({ children }: { children: ReactNode }) => <>{children}</>,
}))
vi.mock('../components/common-logs-filter-bar', () => ({ CommonLogsFilterBar: () => null }))
vi.mock('../components/task-logs-filter-bar', () => ({ TaskLogsFilterBar: () => null }))
vi.mock('../components/usage-logs-mobile-card', () => ({ UsageLogsMobileList: () => null }))
vi.mock('../lib/columns', () => ({ useColumnsByCategory: () => [{ accessorKey: 'id', header: 'Fixture ID' }] }))

function ListPage() {
  const { section } = getRouteApi('/_authenticated/usage-logs/$section').useParams()
  return <UsageLogsProvider><UsageLogsTable logCategory={section as LogCategory} /></UsageLogsProvider>
}

const clients: QueryClient[] = []
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.getState().auth.reset('idle')
  localStorage.removeItem('page-size')
  vi.restoreAllMocks()
})

async function setup(category: LogCategory, options: { mobile?: boolean; role?: number; query?: string; total?: unknown } = {}) {
  vi.mocked(useMediaQuery).mockReturnValue(options.mobile ?? false)
  useAuthStore.getState().auth.setUser({ id: 1, username: 'synthetic', role: options.role ?? 100 })
  const requests: URL[] = []
  let total: unknown = Object.hasOwn(options, 'total') ? options.total : 121
  let reject = false
  let hold = false
  let release = () => {}
  let pending = Promise.resolve()
  const endpoint = { common: '/api/log', drawing: '/api/mj', task: '/api/task' }[category]
  vi.spyOn(api, 'get').mockImplementation(async (path) => {
    const url = new URL(String(path), 'http://synthetic.invalid')
    requests.push(url)
    if (hold) await pending
    if (reject) throw new Error('synthetic list unavailable')
    return { data: { success: true, data: { items: [{ id: `${useAuthStore.getState().auth.user?.id}-${url.searchParams.get('p')}` }], total, page: Number(url.searchParams.get('p')), page_size: Number(url.searchParams.get('page_size')) } } }
  })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: Infinity } } })
  clients.push(client)
  const root = createRootRoute({ component: Outlet })
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated', component: Outlet })
  const logs = createRoute({
    getParentRoute: () => auth,
    path: 'usage-logs/$section',
    validateSearch: logsRoute.options.validateSearch,
    component: ListPage,
  })
  const router = createRouter({ routeTree: root.addChildren([auth.addChildren([logs])]), history: createMemoryHistory({ initialEntries: [`/usage-logs/${category}?${options.query?.includes('pageSize=') ? options.query.replace(/^&/, '') : `pageSize=20${options.query ?? ''}`}`] }) })
  await router.load()
  render(<QueryClientProvider client={client}><RouterProvider router={router} /></QueryClientProvider>)
  const pager = await screen.findByRole('navigation', { name: 'Log pagination' })
  await waitFor(() => expect(within(pager).getByRole('status')).not.toHaveTextContent('Loading record total'))
  return {
    router, client, requests, endpoint, pager, user: userEvent.setup(),
    setTotal: (value: unknown) => { total = value },
    fail: () => { reject = true },
    hold: () => { hold = true; pending = new Promise<void>((resolve) => { release = resolve }) },
    release: () => { hold = false; release() },
  }
}

for (const category of ['common', 'drawing', 'task'] as const) {
  test.each([false, true])(`${category}: server paging, exact jump, first/last and page-size reset (mobile=%s)`, async (mobile) => {
    const { pager, user, requests, endpoint, router } = await setup(category, { mobile })
    expect(within(pager).getByRole('status')).toHaveTextContent('Page 1 of 7 · 121 records')
    await user.click(within(pager).getByRole('button', { name: 'Last page' }))
    await waitFor(() => expect(router.state.location.search.page).toBe(7))
    await waitFor(() => expect(requests.at(-1)?.searchParams.get('p')).toBe('7'))
    await waitFor(() => expect(within(pager).getByRole('button', { name: 'First page' })).toBeEnabled())
    await user.click(within(pager).getByRole('button', { name: 'First page' }))
    await waitFor(() => expect(router.state.location.search.page).toBeUndefined())
    await waitFor(() => expect(within(pager).getByRole('button', { name: 'Last page' })).toBeEnabled())
    await user.type(within(pager).getByRole('textbox', { name: 'Page number' }), '4')
    await user.click(within(pager).getByRole('button', { name: 'Go to page' }))
    await waitFor(() => expect(requests.at(-1)?.searchParams.get('p')).toBe('4'))
    await waitFor(() => expect(within(pager).getByRole('combobox', { name: 'Rows per page' })).toBeEnabled())
    await user.selectOptions(within(pager).getByRole('combobox', { name: 'Rows per page' }), '50')
    await waitFor(() => expect(requests.at(-1)?.searchParams.get('page_size')).toBe('50'))
    expect(requests.at(-1)?.searchParams.get('p')).toBe('1')
    await waitFor(() => expect(within(pager).getByRole('combobox', { name: 'Rows per page' })).toBeEnabled())
    await user.selectOptions(within(pager).getByRole('combobox', { name: 'Rows per page' }), '100')
    await waitFor(() => expect(requests.at(-1)?.searchParams.get('page_size')).toBe('100'))
    expect(requests.at(-1)?.searchParams.get('p')).toBe('1')
    expect(requests.every((url) => url.pathname === endpoint)).toBe(true)
  })

  test(`${category}: ordinary role only uses its self endpoint`, async () => {
    const { requests, endpoint } = await setup(category, { role: 1 })
    expect(requests.length).toBeGreaterThan(0)
    expect(requests.every((url) => url.pathname === `${endpoint}/self`)).toBe(true)
  })
}

test.each([undefined, null, -1, '121', 1.5])('unknown or invalid total (%s) is never presented as zero', async (total) => {
  const { pager } = await setup('common', { total, query: '&page=3' })
  expect(within(pager).getByRole('status')).toHaveTextContent('Record total unavailable')
  expect(within(pager).getByRole('status')).not.toHaveTextContent('0 records')
  expect(within(pager).getByRole('button', { name: 'Last page' })).toBeDisabled()
})

test('an error after success hides the old total and rows', async () => {
  const { pager, fail, client } = await setup('common')
  expect(await screen.findByText('1-1')).toBeInTheDocument()
  fail()
  await act(async () => { await client.invalidateQueries({ queryKey: ['logs'] }) })
  expect(within(pager).getByRole('status')).toHaveTextContent('Record total unavailable')
  expect(screen.queryByText('1-1')).not.toBeInTheDocument()
  expect(screen.getByText('synthetic list unavailable')).toBeInTheDocument()
})

test('a shrinking total returns to the last valid server page', async () => {
  const { pager, router, client, setTotal, requests } = await setup('task', { query: '&page=7' })
  setTotal(21)
  await act(async () => { await client.invalidateQueries({ queryKey: ['logs'] }) })
  await waitFor(() => expect(router.state.location.search.page).toBe(2))
  await waitFor(() => expect(requests.at(-1)?.searchParams.get('p')).toBe('2'))
  expect(within(pager).getByRole('status')).toHaveTextContent('Page 2 of 2 · 21 records')
})

test('a category change never displays the previous category total while waiting', async () => {
  const fixture = await setup('common')
  fixture.hold()
  await act(async () => { await fixture.router.navigate({ to: '/usage-logs/$section', params: { section: 'task' }, search: { pageSize: 20 } }) })
  const pager = await screen.findByRole('navigation', { name: 'Log pagination' })
  expect(within(pager).getByRole('status')).toHaveTextContent('Loading record total')
  expect(within(pager).getByRole('status')).not.toHaveTextContent('121 records')
  await act(async () => fixture.release())
  await waitFor(() => expect(within(pager).getByRole('status')).toHaveTextContent('121 records'))
})

test('oversized old links are bounded and valid legacy page sizes are preserved', async () => {
  const { requests, router } = await setup('common', { query: '&pageSize=500' })
  expect(requests.at(-1)?.searchParams.get('page_size')).toBe('100')
  await act(async () => { await router.navigate({ to: '/usage-logs/$section', params: { section: 'common' }, search: { pageSize: 1 } }) })
  await waitFor(() => expect(requests.at(-1)?.searchParams.get('page_size')).toBe('1'))
  expect(screen.getByRole('combobox', { name: 'Rows per page' })).toHaveValue('1')
})


test('changing user and management scope cannot show privileged rows or totals while the self query waits', async () => {
  const fixture = await setup('common')
  expect(await screen.findByText('1-1')).toBeInTheDocument()
  fixture.hold()
  act(() => useAuthStore.getState().auth.setUser({ id: 2, username: 'other-synthetic', role: 1 }))
  await waitFor(() => expect(fixture.requests.at(-1)?.pathname).toBe('/api/log/self'))
  expect(screen.queryByText('1-1')).not.toBeInTheDocument()
  const pager = screen.getByRole('navigation', { name: 'Log pagination' })
  expect(within(pager).getByRole('status')).toHaveTextContent('Loading record total')
  expect(within(pager).getByRole('status')).not.toHaveTextContent('121 records')
  await act(async () => fixture.release())
  expect(await screen.findByText('2-1')).toBeInTheDocument()
})
