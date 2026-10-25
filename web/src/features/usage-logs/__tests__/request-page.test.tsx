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
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { Route as requestRoute } from '@/routes/_authenticated/usage-logs/request/$requestId'
import { useAuthStore } from '@/stores/auth-store'
import { useInspectorStore } from '@/stores/inspector-store'

const REQUEST_ID = 'REQ-PAGE-1'

function record(other: Record<string, unknown>, overrides = {}) {
  return {
    id: 7,
    user_id: 1,
    type: 2,
    created_at: 1_797_000_000,
    content: '',
    request_id: REQUEST_ID,
    username: 'owner',
    token_name: 'fixture-key',
    token_id: 21,
    model_name: 'fixture-model',
    quota: 0,
    prompt_tokens: 10,
    completion_tokens: 5,
    use_time: 3,
    is_stream: true,
    channel: 4,
    channel_name: 'Fixture connection',
    other: JSON.stringify(other),
    ...overrides,
  }
}

const clients: QueryClient[] = []
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.getState().auth.reset('idle')
  useInspectorStore.setState({ current: null })
  vi.restoreAllMocks()
})

async function renderRequest(log: unknown, role = 100) {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'owner', role })
  const requests: URL[] = []
  vi.spyOn(api, 'get').mockImplementation(async (path) => {
    const url = new URL(String(path), 'http://synthetic.invalid')
    requests.push(url)
    if (url.pathname === '/api/log' || url.pathname === '/api/log/self') {
      return { data: { success: true, data: { items: [log], total: 1 } } }
    }
    if (url.pathname.startsWith('/api/full-content-logs/')) {
      throw Object.assign(new Error('not found'), {
        isAxiosError: true,
        response: { status: 404, data: { success: false } },
      })
    }
    return { data: { success: true, data: {} } }
  })
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
  const request = createRoute({
    getParentRoute: () => auth,
    path: 'usage-logs/request/$requestId',
    component: requestRoute.options.component,
    validateSearch: requestRoute.options.validateSearch,
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([request])]),
    history: createMemoryHistory({
      initialEntries: [`/usage-logs/request/${REQUEST_ID}`],
    }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <RouterProvider router={router} />
      </TooltipProvider>
    </QueryClientProvider>
  )
  return { requests, router, user: userEvent.setup() }
}

function cell(grid: HTMLElement, label: string) {
  const term = within(grid).getByText(label, { selector: 'dt' })
  return term.nextElementSibling as HTMLElement
}

test('the four results report missing facts as missing, never as success or zero', async () => {
  // Completed, but with no HTTP status, usage report or settlement record.
  await renderRequest(
    record(
      { request_outcome: 'completed', usage_outcome: 'missing' },
      { type: 5, prompt_tokens: 0, completion_tokens: 0 }
    )
  )
  const grid = await screen.findByTestId('request-result-grid')
  expect(cell(grid, 'Business result')).toHaveTextContent('Request successful')
  expect(cell(grid, 'HTTP status')).toHaveTextContent('Not recorded')
  expect(cell(grid, 'Usage reliability')).toHaveTextContent(
    'Usage not provided'
  )
  expect(cell(grid, 'Settlement')).toHaveTextContent('Settlement not recorded')
  expect(grid).not.toHaveTextContent(/\b0\b/)
})

test('the four results read the recorded status, provenance and settlement', async () => {
  await renderRequest(
    record({
      request_outcome: 'completed',
      usage_outcome: 'reported',
      usage_accuracy: 'estimated',
      http_status: 200,
      settlement_status: 'settled',
      token_usage_evidence: { schema: 1, input_tokens: 10, output_tokens: 5 },
    })
  )
  const grid = await screen.findByTestId('request-result-grid')
  expect(cell(grid, 'HTTP status')).toHaveTextContent('200')
  expect(cell(grid, 'Usage reliability')).toHaveTextContent('Estimated usage')
  expect(cell(grid, 'Settlement')).toHaveTextContent('Settled')
})

test('an administrator sees each relay attempt in the timeline, in recorded order', async () => {
  await renderRequest(
    record({
      request_outcome: 'completed',
      http_status: 200,
      frt: 900,
      admin_info: {
        relay_attempts: [
          {
            channel_id: 4,
            key_index: 0,
            upstream_model: 'upstream-a',
            outcome: 'retryable_refusal',
            status: 429,
            cooldown_seconds: 12,
          },
          {
            channel_id: 5,
            key_index: 1,
            upstream_model: 'upstream-b',
            outcome: 'completed',
          },
        ],
      },
    })
  )
  const timeline = await screen.findByRole('list', {
    name: 'Diagnosis timeline',
  })
  expect(within(timeline).getByText('Request received')).toBeVisible()
  const attempts = within(timeline).getByRole('list', {
    name: 'Relay attempts',
  })
  const rows = within(attempts).getAllByRole('listitem')
  expect(rows).toHaveLength(2)
  expect(rows[0]).toHaveTextContent('Refused; retry allowed')
  expect(rows[0]).toHaveTextContent('429')
  expect(rows[0]).toHaveTextContent('Future-request account hold: 12 seconds')
  expect(rows[1]).toHaveTextContent('Completed')
  expect(
    within(rows[1]).queryByText('Failure HTTP status')
  ).not.toBeInTheDocument()
  expect(within(timeline).getByText('Upstream completed')).toBeVisible()
  expect(
    within(timeline).getByText(/HTTP 200 · First byte 0.9 s/)
  ).toBeVisible()
  // The saved record below the timeline does not list the attempts again.
  expect(screen.getAllByRole('list', { name: 'Relay attempts' })).toHaveLength(
    1
  )
})

test('a request rejected before sending shows no routing or upstream step', async () => {
  await renderRequest(
    record(
      {
        request_outcome: 'rejected',
        preflight_rejected: true,
        settlement_status: 'not_dispatched',
      },
      { type: 5, prompt_tokens: 0, completion_tokens: 0 }
    )
  )
  const timeline = await screen.findByRole('list', {
    name: 'Diagnosis timeline',
  })
  expect(
    within(timeline).getByText(
      'Rejected before sending; nothing was sent upstream'
    )
  ).toBeVisible()
  expect(within(timeline).queryByText('Routing')).not.toBeInTheDocument()
  expect(within(timeline).getByText('Not sent, not charged')).toBeVisible()
})

test('an ordinary user reads only their own records, with no routing details or body request', async () => {
  const { requests } = await renderRequest(
    record({ request_outcome: 'completed', http_status: 200 }),
    1
  )
  const timeline = await screen.findByRole('list', {
    name: 'Diagnosis timeline',
  })
  expect(
    within(timeline).getByText(
      'Routing details are visible to administrators only.'
    )
  ).toBeVisible()
  expect(screen.queryByText('Fixture connection')).not.toBeInTheDocument()
  expect(await screen.findByText('Administrator access required')).toBeVisible()
  expect(requests.some((url) => url.pathname === '/api/log')).toBe(false)
  expect(requests.some((url) => url.pathname === '/api/log/self')).toBe(true)
  expect(
    requests.some((url) => url.pathname.startsWith('/api/full-content-logs'))
  ).toBe(false)
})

test('the raw tab shows the saved call record and writes its tab to the URL', async () => {
  const { router, user } = await renderRequest(
    record({ request_outcome: 'completed', http_status: 200 })
  )
  await screen.findByTestId('request-result-grid')
  await user.click(screen.getByRole('tab', { name: 'Raw' }))
  expect(router.state.location.search).toEqual({ tab: 'raw' })
  const panel = screen.getByRole('tabpanel')
  expect(within(panel).getByText('Saved call record')).toBeVisible()
  expect(panel).toHaveTextContent(`"request_id": "${REQUEST_ID}"`)
  expect(panel).toHaveTextContent('"request_outcome": "completed"')
})
