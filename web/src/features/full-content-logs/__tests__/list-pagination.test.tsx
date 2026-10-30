/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
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
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { Route as fullContentRoute } from '@/routes/_authenticated/full-content-logs'
import { useAuthStore } from '@/stores/auth-store'

import { FullContentLogs } from '../index'

const clients: QueryClient[] = []
const requests: Array<Record<string, unknown>> = []
let failRequest = false
let resultTotal = 241

function setActor(userId: number, sessionId: string, role = 100) {
  useAuthStore.setState((state) => ({
    auth: {
      ...state.auth,
      user: { id: userId, username: `actor-${userId}`, role },
      session: {
        sid: sessionId,
        current: true,
        login_method: 'synthetic',
        ip: '',
        user_agent: '',
        created_at: 0,
        last_active_at: 0,
        expires_at: 0,
      },
    },
  }))
}

beforeEach(() => {
  requests.length = 0
  failRequest = false
  resultTotal = 241
  setActor(7, 'session-a')
  vi.spyOn(api, 'get').mockImplementation(async (path, config) => {
    if (path !== '/api/full-content-logs') throw new Error('Unexpected API path')
    const params = config?.params as Record<string, unknown>
    requests.push(params)
    if (failRequest) throw new Error('synthetic list unavailable')
    return {
      data: {
        success: true,
        data: {
          enabled: true,
          page: params.p,
          page_size: params.page_size,
          total: resultTotal,
          items: [{
            timestamp: '2026-10-01T02:00:00Z',
            request_id: `actor-${useAuthStore.getState().auth.user?.id}-page-${params.p}-size-${params.page_size}`,
            method: 'POST',
            path: '/v1/responses',
            model: 'synthetic-model',
            status: 200,
            duration_ms: 10,
            request_bytes: 10,
            response_bytes: 20,
            chunk_count: 1,
          }],
          files: { count: 1, total_size: 30, files: [] },
          facets: { models: [], tokens: [] },
        },
      },
    }
  })
})

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.getState().auth.reset('idle')
  vi.restoreAllMocks()
})

function newClient() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  clients.push(client)
  return client
}

function renderStandalone() {
  render(
    <QueryClientProvider client={newClient()}>
      <TooltipProvider><FullContentLogs /></TooltipProvider>
    </QueryClientProvider>
  )
  return userEvent.setup()
}

test('page controls request the last page and reset to page one when page size changes', async () => {
  const user = renderStandalone()
  await screen.findByText('actor-7-page-1-size-20')
  await user.click(screen.getByRole('button', { name: 'Last page' }))
  await screen.findByText('actor-7-page-13-size-20')
  await user.selectOptions(screen.getByRole('combobox', { name: 'Rows per page' }), '50')
  await screen.findByText('actor-7-page-1-size-50')
  await user.selectOptions(screen.getByRole('combobox', { name: 'Rows per page' }), '100')
  await screen.findByText('actor-7-page-1-size-100')
  expect(requests.map((params) => [params.p, params.page_size])).toEqual([
    [1, 20], [13, 20], [1, 50], [1, 100],
  ])
})

test('applying and resetting filters request page one with the selected page size', async () => {
  const user = renderStandalone()
  await screen.findByText('actor-7-page-1-size-20')
  await user.selectOptions(screen.getByRole('combobox', { name: 'Rows per page' }), '50')
  await screen.findByText('actor-7-page-1-size-50')
  await user.click(screen.getByRole('button', { name: 'Next' }))
  await screen.findByText('actor-7-page-2-size-50')
  await user.type(screen.getByLabelText('Model'), 'selected-model')
  await user.type(screen.getByLabelText('API Key name or ID'), '12')
  await user.click(screen.getByRole('button', { name: 'Search' }))
  await screen.findByText('actor-7-page-1-size-50')
  expect(requests.at(-1)).toMatchObject({ p: 1, page_size: 50, model: 'selected-model', token: '12' })
  await user.click(screen.getByRole('button', { name: 'Next' }))
  await screen.findByText('actor-7-page-2-size-50')
  await user.click(screen.getByRole('button', { name: 'Reset filters' }))
  await screen.findByText('actor-7-page-1-size-50')
  expect(requests.at(-1)).toMatchObject({ p: 1, page_size: 50, model: undefined, token: undefined })
})

test('loading the list reports an unknown total and disables page navigation', async () => {
  const implementation = vi.mocked(api.get).getMockImplementation()
  if (!implementation) throw new Error('Expected a mocked API implementation')
  let release = () => {}
  const pending = new Promise<void>((resolve) => { release = resolve })
  vi.mocked(api.get).mockImplementationOnce(async (...args) => {
    await pending
    return implementation(...args)
  })
  renderStandalone()
  const pagination = screen.getByRole('navigation', { name: 'Full content log pagination' })
  expect(within(pagination).getByRole('status')).toHaveTextContent('Loading request total')
  expect(pagination).not.toHaveTextContent('0 requests')
  expect(within(pagination).getByRole('button', { name: 'Next' })).toBeDisabled()
  release()
  await screen.findByText('actor-7-page-1-size-20')
})

test('numbered pages and a direct page jump send the selected server page', async () => {
  const user = renderStandalone()
  await screen.findByText('actor-7-page-1-size-20')
  await user.click(screen.getByRole('button', { name: 'Go to page 3' }))
  await screen.findByText('actor-7-page-3-size-20')
  await user.type(screen.getByRole('textbox', { name: 'Page number' }), '12{Enter}')
  await screen.findByText('actor-7-page-12-size-20')
  expect(requests.at(-1)).toMatchObject({ p: 12, page_size: 20 })
})

test('a smaller confirmed total after refresh moves an out of range page to the last page', async () => {
  const user = renderStandalone()
  await screen.findByText('actor-7-page-1-size-20')
  await user.click(screen.getByRole('button', { name: 'Last page' }))
  await screen.findByText('actor-7-page-13-size-20')
  resultTotal = 21
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  await screen.findByText('actor-7-page-2-size-20')
  expect(requests.at(-1)).toMatchObject({ p: 2, page_size: 20 })
})

test('a failed refresh hides earlier rows and does not report a reliable zero total', async () => {
  const user = renderStandalone()
  await screen.findByText('actor-7-page-1-size-20')
  failRequest = true
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  await screen.findByText('synthetic list unavailable')
  expect(screen.queryByText('actor-7-page-1-size-20')).not.toBeInTheDocument()
  expect(screen.queryByText('241')).not.toBeInTheDocument()
  const pagination = screen.getByRole('navigation', { name: 'Full content log pagination' })
  expect(within(pagination).getByRole('status')).toHaveTextContent('Request total unavailable')
  expect(pagination).not.toHaveTextContent('0 requests')
  expect(within(pagination).getByRole('button', { name: 'Next' })).toBeDisabled()
})

test.each([[8, 'session-a'], [7, 'session-b']] as const)(
  'switching user/session to %s / %s resets list controls and hides earlier rows',
  async (userId, sessionId) => {
    const user = renderStandalone()
    await screen.findByText('actor-7-page-1-size-20')
    await user.type(screen.getByLabelText('Model'), 'private-filter')
    await user.click(screen.getByRole('button', { name: 'Search' }))
    await waitFor(() => expect(requests.at(-1)?.model).toBe('private-filter'))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Next' })).toBeEnabled())
    await user.click(screen.getByRole('button', { name: 'Next' }))
    await screen.findByText('actor-7-page-2-size-20')
    act(() => setActor(userId, sessionId))
    expect(screen.queryByText('actor-7-page-2-size-20')).not.toBeInTheDocument()
    await waitFor(() => expect(requests.at(-1)).toMatchObject({ p: 1, page_size: 20, model: undefined }))
    expect(screen.getByLabelText('Model')).toHaveValue('')
  }
)

test('same-session permission loss clears applied controls and unsent drafts before access returns', async () => {
  const user = renderStandalone()
  await screen.findByText('actor-7-page-1-size-20')
  await user.type(screen.getByLabelText('Model'), 'applied-private-filter')
  await user.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() => expect(requests.at(-1)?.model).toBe('applied-private-filter'))
  await waitFor(() => expect(screen.getByRole('button', { name: 'Next' })).toBeEnabled())
  await user.click(screen.getByRole('button', { name: 'Next' }))
  await screen.findByText('actor-7-page-2-size-20')
  await user.clear(screen.getByLabelText('Model'))
  await user.type(screen.getByLabelText('Model'), 'unsent-private-draft')
  const before = requests.length
  act(() => setActor(7, 'session-a', 1))
  expect(screen.queryByText('actor-7-page-2-size-20')).not.toBeInTheDocument()
  expect(screen.getByLabelText('Model')).toHaveValue('')
  expect(requests).toHaveLength(before)
  act(() => setActor(7, 'session-a', 100))
  await screen.findByText('actor-7-page-1-size-20')
  expect(screen.getByLabelText('Model')).toHaveValue('')
  expect(requests.at(-1)).toMatchObject({ p: 1, page_size: 20, model: undefined })
})

test('browser history return restores filters, page and page size without putting them in URLs or storage', async () => {
  const root = createRootRoute({ component: Outlet })
  const logs = createRoute({
    getParentRoute: () => root,
    path: 'full-content-logs',
    component: fullContentRoute.options.component,
    beforeLoad: (options) => fullContentRoute.options.beforeLoad?.(options as never),
  })
  const other = createRoute({
    getParentRoute: () => root,
    path: 'other',
    component: () => <p>Other destination</p>,
  })
  const router = createRouter({
    routeTree: root.addChildren([logs, other]),
    history: createMemoryHistory({ initialEntries: ['/full-content-logs'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={newClient()}>
      <TooltipProvider><RouterProvider router={router} /></TooltipProvider>
    </QueryClientProvider>
  )
  const user = userEvent.setup()
  await screen.findByText('actor-7-page-1-size-20')
  await user.selectOptions(screen.getByRole('combobox', { name: 'Rows per page' }), '50')
  await screen.findByText('actor-7-page-1-size-50')
  await user.type(screen.getByLabelText('Model'), 'return-model')
  await user.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() => expect(requests.at(-1)?.model).toBe('return-model'))
  await waitFor(() => expect(screen.getByRole('button', { name: 'Next' })).toBeEnabled())
  await user.click(screen.getByRole('button', { name: 'Next' }))
  await screen.findByText('actor-7-page-2-size-50')
  await act(() => router.navigate({ to: '/other' } as never))
  await screen.findByText('Other destination')
  await act(() => { router.history.back() })
  await screen.findByText('actor-7-page-2-size-50')
  expect(screen.getByLabelText('Model')).toHaveValue('return-model')
  expect(screen.getByRole('combobox', { name: 'Rows per page' })).toHaveValue('50')
  expect(router.state.location.href).toBe('/full-content-logs')
  expect(JSON.stringify(router.state.location.state)).not.toContain('return-model')
  expect(JSON.stringify(localStorage)).not.toContain('return-model')
  expect(JSON.stringify(sessionStorage)).not.toContain('return-model')
  await act(() => router.navigate({ to: '/other' } as never))
  await screen.findByText('Other destination')
  act(() => setActor(7, 'session-b'))
  await act(() => { router.history.back() })
  await screen.findByText('actor-7-page-1-size-20')
  expect(screen.getByLabelText('Model')).toHaveValue('')
})
