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
import { act, render, screen, waitFor, within } from '@testing-library/react'
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

async function renderRequest(log: unknown, role = 100, options: { entry?: string; content?: Record<string, unknown> } = {}) {
  let selectedLog = log
  let savedContent = options.content
  let waitForLookup = false
  let releaseLookup = () => {}
  let pendingLookup = Promise.resolve()

  useAuthStore.getState().auth.setUser({ id: 1, username: 'owner', role })
  const requests: URL[] = []
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Synthetic writes forbidden'))
  vi.spyOn(api, 'put').mockRejectedValue(new Error('Synthetic writes forbidden'))
  vi.spyOn(api, 'delete').mockRejectedValue(new Error('Synthetic writes forbidden'))
  vi.spyOn(api, 'get').mockImplementation(async (path) => {
    const url = new URL(String(path), 'http://synthetic.invalid')
    requests.push(url)
    if (url.pathname === '/api/log' || url.pathname === '/api/log/self') {
      if (waitForLookup) await pendingLookup
      return { data: { success: true, data: { items: [selectedLog], total: 1 } } }
    }
    if (url.pathname.startsWith('/api/full-content-logs/')) {
      if (savedContent) return { data: { success: true, data: savedContent } }
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
      initialEntries: [options.entry ?? `/usage-logs/request/${REQUEST_ID}`],
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
  return {
    requests, router, client, user: userEvent.setup(),
    setLog: (next: unknown) => { selectedLog = next },
    setContent: (next: Record<string, unknown>) => { savedContent = next },
    holdLookup: () => { waitForLookup = true; pendingLookup = new Promise<void>((resolve) => { releaseLookup = resolve }) },
    releaseLookup: () => { waitForLookup = false; releaseLookup() },
  }
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
  const { user } = await renderRequest(
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
  await user.click(await screen.findByRole('tab', { name: 'Diagnosis' }))
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
  const { user } = await renderRequest(
    record(
      {
        request_outcome: 'rejected',
        preflight_rejected: true,
        settlement_status: 'not_dispatched',
      },
      { type: 5, prompt_tokens: 0, completion_tokens: 0 }
    )
  )
  await user.click(await screen.findByRole('tab', { name: 'Diagnosis' }))
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
  const { requests, user } = await renderRequest(
    record({ request_outcome: 'completed', http_status: 200 }),
    1
  )
  expect(await screen.findByText('Administrator access required')).toBeVisible()
  await user.click(await screen.findByRole('tab', { name: 'Diagnosis' }))
  const timeline = await screen.findByRole('list', {
    name: 'Diagnosis timeline',
  })
  expect(
    within(timeline).getByText(
      'Routing details are visible to administrators only.'
    )
  ).toBeVisible()
  expect(screen.queryByText('Fixture connection')).not.toBeInTheDocument()
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
  await user.click(screen.getByRole('tab', { name: 'Raw evidence' }))
  expect(router.state.location.search).toEqual({ tab: 'raw' })
  const panel = screen.getByRole('tabpanel')
  expect(within(panel).getByText('Saved call record')).toBeVisible()
  expect(panel).toHaveTextContent(`"request_id": "${REQUEST_ID}"`)
  expect(panel).toHaveTextContent('"request_outcome": "completed"')
})


function savedContent(overrides: Record<string, unknown> = {}) {
  return {
    request_id: REQUEST_ID,
    method: 'POST', path: '/v1/responses', status: 200,
    timestamp: '2026-10-30T08:00:00Z', duration_ms: 30,
    chunk_count: 2, request_bytes: 12, response_bytes: 20,
    request_body: '{"input":"SYNTHETIC-SAVED-QUESTION"}',
    response_body: 'event: response.completed\ndata: {"type":"response.completed","marker":"SYNTHETIC-SAVED-ANSWER"}\n\n',
    request_content_type: 'application/json', response_content_type: 'text/event-stream',
    request_headers: { authorization: ['[REDACTED]'] },
    response_headers: { 'content-type': ['text/event-stream'] },
    query: {},
    ...overrides,
  }
}

test('identity and four independent results stay singular while the evidence tabs change without writes', async () => {
  const fixture = await renderRequest(record({ request_outcome: 'completed', http_status: 200 }), 100, { content: savedContent() })
  const summary = await screen.findByLabelText('Request identity')
  expect(summary).toHaveTextContent(REQUEST_ID)
  expect(summary).toHaveTextContent('fixture-key (#21)')
  expect(summary).toHaveTextContent('Fixture connection (#4)')
  expect(screen.getByRole('tab', { name: 'Body' })).toHaveAttribute('aria-selected', 'true')
  for (const [name, tab] of [['Diagnosis', 'detail'], ['SSE', 'sse'], ['Raw evidence', 'raw'], ['Body', undefined]] as const) {
    await fixture.user.click(screen.getByRole('tab', { name }))
    await waitFor(() => expect(fixture.router.state.location.search).toEqual(tab ? { tab } : {}))
    expect(screen.getAllByLabelText('Request identity')).toHaveLength(1)
    expect(screen.getAllByTestId('request-result-grid')).toHaveLength(1)
    expect(screen.getByRole('tab', { name })).toHaveAttribute('aria-selected', 'true')
  }
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
  expect(api.delete).not.toHaveBeenCalled()
  expect(fixture.requests.every((url) => url.pathname.startsWith('/api/'))).toBe(true)
})

test('raw evidence separates the account call record from saved upstream payloads and headers', async () => {
  const fixture = await renderRequest(record({ cache_tokens: null, request_outcome: 'completed', http_status: 200 }), 100, { content: savedContent() })
  await fixture.user.click(await screen.findByRole('tab', { name: 'Raw evidence' }))
  const callRecord = screen.getByRole('region', { name: 'Saved call record' })
  expect(callRecord).toHaveTextContent('"cache_tokens": null')
  expect(callRecord).not.toHaveTextContent('SYNTHETIC-SAVED-QUESTION')
  const upstream = screen.getByRole('region', { name: 'Saved upstream request and response' })
  await waitFor(() => expect(upstream).toHaveTextContent('SYNTHETIC-SAVED-QUESTION'))
  expect(upstream).toHaveTextContent('SYNTHETIC-SAVED-ANSWER')
  await fixture.user.click(within(upstream).getByText('Request headers', { selector: 'summary' }))
  await waitFor(() => expect(upstream).toHaveTextContent('[REDACTED]'))
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
  expect(api.delete).not.toHaveBeenCalled()
})

test('ordinary viewers may inspect their saved record but never fetch upstream bodies from any evidence tab', async () => {
  const fixture = await renderRequest(record({ request_outcome: 'completed', http_status: 200 }, { channel_name: '' }), 1)
  await screen.findByTestId('request-result-grid')
  for (const name of ['Diagnosis', 'Raw evidence', 'SSE', 'Body']) {
    await fixture.user.click(screen.getByRole('tab', { name }))
  }
  expect(fixture.requests.some((url) => url.pathname === '/api/log')).toBe(false)
  expect(fixture.requests.some((url) => url.pathname === '/api/log/self')).toBe(true)
  expect(fixture.requests.some((url) => url.pathname.startsWith('/api/full-content-logs'))).toBe(false)
  expect(screen.getByText('Administrator access required')).toBeVisible()
  expect(screen.queryByLabelText('Connection')).not.toBeInTheDocument()
})

test('changing a signed-in actor immediately removes prior identity and payload while a self lookup is pending', async () => {
  const fixture = await renderRequest(record({ request_outcome: 'completed', http_status: 200 }), 100, { content: savedContent() })
  await screen.findByText(/SYNTHETIC-SAVED-QUESTION/)
  fixture.holdLookup()
  fixture.setLog(record({ request_outcome: 'completed', usage_outcome: 'missing' }, { user_id: 2, token_name: 'next-key', channel_name: '', token_id: 22 }))
  act(() => useAuthStore.getState().auth.setUser({ id: 2, username: 'next-owner', role: 1 }))
  await waitFor(() => expect(fixture.requests.at(-1)?.pathname).toBe('/api/log/self'))
  expect(screen.queryByText(/SYNTHETIC-SAVED-QUESTION/)).not.toBeInTheDocument()
  expect(screen.queryByText('fixture-key (#21)')).not.toBeInTheDocument()
  expect(screen.queryByText('Fixture connection (#4)')).not.toBeInTheDocument()
  await act(async () => fixture.releaseLookup())
  expect(await screen.findByText('next-key (#22)')).toBeVisible()
  expect(screen.queryByText(/SYNTHETIC-SAVED-QUESTION/)).not.toBeInTheDocument()
})

test('an explicit diagnosis deep link does not fetch request content before a content tab is opened', async () => {
  const fixture = await renderRequest(record({ request_outcome: 'completed', http_status: 200 }), 100, { entry: `/usage-logs/request/${REQUEST_ID}?tab=detail`, content: savedContent() })
  expect(await screen.findByRole('list', { name: 'Diagnosis timeline' })).toBeVisible()
  expect(fixture.requests.some((url) => url.pathname.startsWith('/api/full-content-logs'))).toBe(false)
  await fixture.user.click(screen.getByRole('tab', { name: 'Body' }))
  await screen.findByText(/SYNTHETIC-SAVED-QUESTION/)
  expect(fixture.requests.some((url) => url.pathname.startsWith('/api/full-content-logs/'))).toBe(true)
})


test('rotating a session on the same user removes the previous saved payload until the new lookup resolves', async () => {
  const fixture = await renderRequest(record({ request_outcome: 'completed', http_status: 200 }), 100, { content: savedContent() })
  await screen.findByText(/SYNTHETIC-SAVED-QUESTION/)
  fixture.holdLookup()
  fixture.setContent(savedContent({ request_body: '{"input":"NEW-SESSION-QUESTION"}' }))
  act(() => useAuthStore.setState((state) => ({ auth: { ...state.auth, session: {
    sid: 'new-synthetic-session', current: true, login_method: 'password', ip: '', user_agent: '', created_at: 1, last_active_at: 1, expires_at: 2,
  } } })))
  await waitFor(() => expect(screen.queryByText(/SYNTHETIC-SAVED-QUESTION/)).not.toBeInTheDocument())
  await act(async () => fixture.releaseLookup())
  expect(await screen.findByText(/NEW-SESSION-QUESTION/)).toBeVisible()
  expect(screen.queryByText(/SYNTHETIC-SAVED-QUESTION/)).not.toBeInTheDocument()
  expect(api.post).not.toHaveBeenCalled()
})


test('a role change inside administrator scope cannot reuse the previous role lookup snapshot', async () => {
  const fixture = await renderRequest(record({ request_outcome: 'completed', http_status: 200 }), 100, { content: savedContent() })
  await screen.findByText('fixture-key (#21)')
  const before = fixture.requests.filter((url) => url.pathname === '/api/log').length
  fixture.holdLookup()
  fixture.setLog(record({ request_outcome: 'completed', http_status: 200 }, { token_name: 'new-role-key' }))
  act(() => useAuthStore.getState().auth.setUser({ id: 1, username: 'owner', role: 10 }))
  try {
    await waitFor(() => expect(fixture.requests.filter((url) => url.pathname === '/api/log')).toHaveLength(before + 1))
    expect(screen.queryByText('fixture-key (#21)')).not.toBeInTheDocument()
    expect(screen.queryByTestId('request-result-grid')).not.toBeInTheDocument()
  } finally {
    fixture.releaseLookup()
  }
  expect(await screen.findByText('new-role-key (#21)')).toBeVisible()
})


test('known names do not hide missing identity IDs or fabricate ID zero', async () => {
  await renderRequest(record({ request_outcome: 'completed' }, { token_id: 0, channel: 0 }))
  const identity = await screen.findByLabelText('Request identity')
  expect(identity).toHaveTextContent('fixture-key (ID: Not recorded)')
  expect(identity).toHaveTextContent('Fixture connection (ID: Not recorded)')
  expect(identity).not.toHaveTextContent('#0')
  expect(within(identity).queryByRole('button', { name: 'View this Key' })).not.toBeInTheDocument()
})
