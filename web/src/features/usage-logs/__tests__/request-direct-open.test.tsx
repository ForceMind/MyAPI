/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
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
import { Route as usageLogsRoute } from '@/routes/_authenticated/usage-logs/$section'
import { useAuthStore } from '@/stores/auth-store'
import { useInspectorStore } from '@/stores/inspector-store'

import { UsageLogs } from '../index'

const REQUEST_ID = 'REQ-20261001-OLD'
// 2026-10-01 10:00 local time: older than "today" in every test run after it.
const createdAt = Math.floor(new Date(2026, 9, 1, 10, 0, 0).getTime() / 1000)
const row = {
  id: 501,
  user_id: 1,
  type: 5,
  created_at: createdAt,
  request_id: REQUEST_ID,
  username: 'owner',
  token_name: 'fixture-key',
  token_id: 21,
  model_name: 'fixture-model',
  quota: 0,
  prompt_tokens: 0,
  completion_tokens: 0,
  use_time: 2,
  channel: 1,
  content: 'context_length_exceeded',
  other: JSON.stringify({
    request_outcome: 'failed',
    usage_outcome: 'unknown',
    http_status: 200,
  }),
}
let lookup: 'found' | 'missing' | 'error'
let requests: URL[]
const clients: QueryClient[] = []

beforeEach(() => {
  lookup = 'found'
  requests = []
  useAuthStore.getState().auth.setUser({ id: 1, username: 'owner', role: 100 })
  vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true } })
  vi.spyOn(api, 'get').mockImplementation(async (path) => {
    const url = new URL(String(path), 'http://synthetic.invalid')
    requests.push(url)
    if (url.pathname === '/api/log' || url.pathname === '/api/log/self') {
      if (url.searchParams.get('request_id') === REQUEST_ID) {
        // Only the exact lookup (no time range) fails; the table keeps working.
        if (lookup === 'error' && !url.searchParams.has('start_timestamp')) {
          throw new Error('private transport diagnostic')
        }
        const items = lookup === 'found' ? [row] : []
        return { data: { success: true, data: { items, total: items.length } } }
      }
      return { data: { success: true, data: { items: [], total: 0 } } }
    }
    if (url.pathname.startsWith('/api/full-content-logs/')) {
      return {
        data: {
          success: true,
          data: {
            request_id: REQUEST_ID,
            method: 'POST',
            path: '/v1/chat/completions',
            status: 200,
            timestamp: '2026-10-01T02:00:00Z',
            duration_ms: 2000,
            chunk_count: 2,
            request_bytes: 10,
            response_bytes: 20,
            request_body:
              '{"messages":[{"role":"user","content":"saved question"}]}',
            response_body:
              'data: {"error":{"message":"context too long"}}\n\ndata: [DONE]\n\n',
            response_content_type: 'text/event-stream',
            request_headers: { authorization: ['[REDACTED]'] },
            response_headers: { 'content-type': ['text/event-stream'] },
            query: {},
          },
        },
      }
    }
    return { data: { success: true, data: {} } }
  })
})

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.getState().auth.reset('idle')
  useInspectorStore.setState({ current: null })
  vi.restoreAllMocks()
})

async function renderPage(entry: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  clients.push(client)
  const root = createRootRoute({ component: Outlet })
  const auth = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
    component: Outlet,
  })
  const logs = createRoute({
    getParentRoute: () => auth,
    path: 'usage-logs/$section',
    component: UsageLogs,
    validateSearch: usageLogsRoute.options.validateSearch,
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([logs])]),
    history: createMemoryHistory({ initialEntries: [entry] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <RouterProvider router={router} />
      </TooltipProvider>
    </QueryClientProvider>
  )
  return { router, user: userEvent.setup() }
}

const directLink = `/usage-logs/common?requestId=${REQUEST_ID}&inspect=request`

test('a Request ID link opens that request’s details directly, from any date', async () => {
  const { router } = await renderPage(directLink)
  const detail = await screen.findByTestId('request-inspector')
  expect(within(detail).getAllByText(REQUEST_ID)[0]).toBeVisible()
  expect(within(detail).getAllByText('Request failed').length).toBeGreaterThan(
    0
  )
  // The exact ID lookup is not limited to today's default range.
  expect(
    requests.some(
      (url) =>
        url.searchParams.get('request_id') === REQUEST_ID &&
        !url.searchParams.has('start_timestamp') &&
        !url.searchParams.has('end_timestamp')
    )
  ).toBe(true)
  // The list behind the inspector is widened to the record's day.
  await waitFor(() =>
    expect(router.state.location.search).toMatchObject({
      requestId: REQUEST_ID,
      inspect: 'request',
      startTime: new Date(2026, 9, 1).getTime(),
    })
  )
})

test('an unknown Request ID says so instead of showing an empty list as success', async () => {
  lookup = 'missing'
  await renderPage(directLink)
  expect(
    await screen.findByText('No call record has this Request ID')
  ).toBeVisible()
  expect(screen.queryByTestId('request-inspector')).not.toBeInTheDocument()
})

test('a failed lookup is an error with retry, not "not found"', async () => {
  lookup = 'error'
  const { user } = await renderPage(directLink)
  expect(
    await screen.findByText('Could not look up this Request ID')
  ).toBeVisible()
  expect(
    screen.queryByText('No call record has this Request ID')
  ).not.toBeInTheDocument()
  expect(screen.queryByText(/private transport/)).not.toBeInTheDocument()
  lookup = 'found'
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByTestId('request-inspector')).toBeVisible()
})

test('the body tab reads the saved, redacted body and never resends the request', async () => {
  await renderPage(`${directLink}&requestTab=body`)
  const detail = await screen.findByTestId('request-inspector')
  expect(within(detail).getByRole('tab', { name: 'Body' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
  expect(await within(detail).findByText(/saved question/)).toBeVisible()
  expect(within(detail).getByText(/credentials stay redacted/)).toBeVisible()
  expect(
    requests.some((url) =>
      url.pathname.startsWith(`/api/full-content-logs/${REQUEST_ID}`)
    )
  ).toBe(true)
  expect(api.post).not.toHaveBeenCalled()
})

test('closing the details keeps the call records filtered to the same Request ID', async () => {
  const { router, user } = await renderPage(directLink)
  await screen.findByTestId('request-inspector')
  await user.click(screen.getByRole('button', { name: 'Close details' }))
  await waitFor(() =>
    expect(router.state.location.search).not.toHaveProperty('inspect')
  )
  expect(router.state.location.search).toMatchObject({ requestId: REQUEST_ID })
})
