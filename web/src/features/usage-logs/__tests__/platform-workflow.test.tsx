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

import { api } from '@/lib/api'
import { Route as usageLogsRoute } from '@/routes/_authenticated/usage-logs/$section'
import { useAuthStore } from '@/stores/auth-store'

import { UsageLogs } from '../index'

const clients: QueryClient[] = []
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.getState().auth.reset('idle')
  vi.restoreAllMocks()
})

async function renderWorkflow(
  role = 100,
  holdSelf = false,
  initialEntry = '/usage-logs/common?pageSize=1&model=fixture-model',
  holdKeyId?: number
) {
  let releaseSelf = () => {}
  const selfResponse = new Promise<void>((resolve) => {
    releaseSelf = resolve
  })
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'synthetic-maintainer', role })
  const requests: URL[] = []
  vi.spyOn(api, 'get').mockImplementation(async (path) => {
    const url = new URL(String(path), 'http://synthetic.invalid')
    requests.push(url)
    if (holdSelf && url.pathname === '/api/log/self') await selfResponse
    if (
      holdKeyId &&
      url.pathname === '/api/log' &&
      url.searchParams.get('token_id') === String(holdKeyId)
    ) {
      await selfResponse
    }
    let data: unknown = {}
    if (url.pathname === '/api/status') {
      data = {
        user_funding_mode: 'disabled',
        user_funding_capabilities: { mode: 'disabled', epoch: 1, ready: true },
      }
    }
    if (url.pathname === '/api/log' || url.pathname === '/api/log/self') {
      const page = Number(url.searchParams.get('p') ?? 1)
      data = {
        total: 2,
        page,
        page_size: 1,
        items: [
          {
            id: page,
            user_id: 1,
            type: 5,
            created_at: 1792470000,
            request_id: `REQ-SYNTHETIC-${page}`,
            username:
              url.pathname === '/api/log'
                ? 'SYNTHETIC-ADMIN-ONLY'
                : 'synthetic-maintainer',
            token_name: 'fixture-key',
            model_name: 'fixture-model',
            quota: 1234567,
            prompt_tokens: 0,
            completion_tokens: 0,
            use_time: 1,
            channel: 1,
            other: JSON.stringify({
              request_outcome: 'completed',
              usage_outcome: 'missing',
              settlement_status: 'pending_review',
            }),
          },
        ],
      }
    }
    if (
      url.pathname === '/api/log/stat' ||
      url.pathname === '/api/log/self/stat'
    ) {
      data = { quota: 1234567, rpm: 1, tpm: 0 }
    }
    if (url.pathname.startsWith('/api/full-content-logs/')) {
      data = {
        request_id: 'REQ-SYNTHETIC-2',
        method: 'POST',
        path: '/v1/responses',
        status: 200,
        timestamp: '2026-10-20T06:00:00Z',
        duration_ms: 1000,
        chunk_count: 1,
        request_bytes: 2,
        response_bytes: 2,
        request_body: '{"input":"SYNTHETIC"}',
        response_body: '{"choices":[{"message":{"content":"saved response"}}]}',
        request_headers: {},
        response_headers: {},
        query: {},
      }
    }
    if (url.pathname === '/api/user/1') {
      data = {
        username: 'fixture-owner',
        quota: 1234567,
        used_quota: 7654321,
        aff_quota: 1234567,
        request_count: 2,
      }
    }
    if (url.pathname.startsWith('/api/usage-review/')) {
      data = {
        request_id: url.pathname.split('/').pop(),
        status: 'pending_review',
        holds: [],
        facts: [],
      }
    }
    return { data: { success: true, data } }
  })
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0, staleTime: Infinity },
    },
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
    history: createMemoryHistory({
      initialEntries: [initialEntry],
    }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return { router, requests, releaseSelf, user: userEvent.setup() }
}

test('disabled funding preserves admin server pagination, saved content and return context without monetary cells', async () => {
  const { requests, router, user } = await renderWorkflow()
  expect(await screen.findByText('REQ-SYNTHETIC-1')).toBeInTheDocument()
  expect(
    requests.some(
      (url) =>
        url.pathname === '/api/log' &&
        url.searchParams.get('model_name') === 'fixture-model'
    )
  ).toBe(true)
  expect(
    screen.queryByRole('columnheader', { name: 'Cost' })
  ).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Expand' }))
  await user.type(screen.getByPlaceholderText('Request ID'), 'REQ-SYNTHETIC-')
  await user.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() =>
    expect(
      requests.some(
        (url) =>
          url.pathname === '/api/log' &&
          url.searchParams.get('request_id') === 'REQ-SYNTHETIC-'
      )
    ).toBe(true)
  )
  await user.click(screen.getByRole('button', { name: 'Go to next page' }))
  await screen.findByText('REQ-SYNTHETIC-2')
  expect(
    requests.some(
      (url) => url.pathname === '/api/log' && url.searchParams.get('p') === '2'
    )
  ).toBe(true)
  const before = router.state.location.href
  await user.click(screen.getByTitle('Click to view full details'))
  const detail = await screen.findByRole('dialog', { name: /Log Details/ })
  expect(within(detail).getAllByText('Unknown').length).toBeGreaterThanOrEqual(
    2
  )
  expect(within(detail).queryByText('Total Cost')).not.toBeInTheDocument()
  await user.click(
    within(detail).getByRole('button', { name: 'View request content' })
  )
  const content = await screen.findByRole('dialog', {
    name: 'Full Content Log Detail',
  })
  await user.click(
    await within(content).findByRole('tab', { name: /^Response/ })
  )
  await within(content).findByText('saved response', { exact: false })
  await user.keyboard('{Escape}')
  await waitFor(() =>
    expect(screen.getByRole('dialog', { name: /Log Details/ })).toBeVisible()
  )
  await user.keyboard('{Escape}')
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
  expect(router.state.location.href).toBe(before)
  expect(screen.getByText('REQ-SYNTHETIC-2')).toBeInTheDocument()
})

test('ordinary role uses only the self endpoint and never receives an all-users control', async () => {
  const { requests } = await renderWorkflow(1)
  await screen.findByText('REQ-SYNTHETIC-1')
  expect(requests.some((url) => url.pathname === '/api/log/self')).toBe(true)
  expect(requests.some((url) => url.pathname === '/api/log')).toBe(false)
  expect(screen.queryByRole('tab', { name: 'All' })).not.toBeInTheDocument()
})

test('switching to self scope removes admin data while the self response is pending', async () => {
  const { requests, releaseSelf, user } = await renderWorkflow(100, true)
  await screen.findByText('SYNTHETIC-ADMIN-ONLY')
  await user.click(screen.getByRole('tab', { name: 'Only Mine' }))
  await waitFor(() =>
    expect(requests.some((url) => url.pathname === '/api/log/self')).toBe(true)
  )
  try {
    expect(screen.queryByText('SYNTHETIC-ADMIN-ONLY')).not.toBeInTheDocument()
    expect(screen.queryByText('REQ-SYNTHETIC-1')).not.toBeInTheDocument()
  } finally {
    releaseSelf()
  }
  await screen.findByText('REQ-SYNTHETIC-1')
})

test('changing the signed-in actor closes request details from the previous actor', async () => {
  const { user } = await renderWorkflow()
  await screen.findByText('REQ-SYNTHETIC-1')
  await user.click(screen.getByTitle('Click to view full details'))
  await screen.findByRole('dialog', { name: /Log Details/ })
  act(() =>
    useAuthStore
      .getState()
      .auth.setUser({ id: 2, username: 'synthetic-other-owner', role: 1 })
  )
  await waitFor(() =>
    expect(
      screen.queryByRole('dialog', { name: /Log Details/ })
    ).not.toBeInTheDocument()
  )
  expect(screen.queryByRole('tab', { name: 'All' })).not.toBeInTheDocument()
})

for (const action of ['Search', 'Reset']) {
  test(`${action} keeps the validated Key return context without retaining cleared log filters`, async () => {
    const context = {
      connection: {
        connection: 7,
        page: 4,
        filter: 'saved connection',
        status: 'disabled',
      },
      pool: 19,
      poolPage: 2,
      key: 31,
    }
    const query = new URLSearchParams({
      pageSize: '1',
      model: 'fixture-model',
      channel: JSON.stringify('7'),
      token: 'same-name',
      username: 'operator',
      group: 'group-name',
      requestId: 'original-request',
      upstreamRequestId: 'original-upstream',
      connectionContext: JSON.stringify(context),
      returnToKeys: 'true',
      untrustedReturnUrl: 'https://example.invalid',
    })
    const { router, user } = await renderWorkflow(
      100,
      false,
      `/usage-logs/common?${query}`
    )
    await screen.findByText('REQ-SYNTHETIC-1')
    const originalReturn = screen
      .getByRole('link', { name: 'Associated Key shares' })
      .getAttribute('href')
    if (action === 'Search') {
      await user.clear(screen.getByPlaceholderText('Model Name'))
      await user.type(
        screen.getByPlaceholderText('Model Name'),
        'changed-model'
      )
    }
    await user.click(screen.getByRole('button', { name: action }))
    await waitFor(() =>
      expect(router.state.location.search).toMatchObject({
        connectionContext: context,
        returnToKeys: true,
        page: 1,
        pageSize: 1,
      })
    )
    expect(
      screen
        .getByRole('link', { name: 'Associated Key shares' })
        .getAttribute('href')
    ).toBe(originalReturn)
    const search = router.state.location.search as Record<string, unknown>
    expect(search.untrustedReturnUrl).toBeUndefined()
    if (action === 'Reset') {
      for (const key of [
        'model',
        'channel',
        'token',
        'username',
        'group',
        'requestId',
        'upstreamRequestId',
      ]) {
        expect(search[key] || undefined).toBeUndefined()
      }
    } else {
      expect(search.model).toBe('changed-model')
      expect(search.channel).toBe('7')
      expect(search.requestId).toBe('original-request')
    }
  })
}

test('selected Key identity survives filter apply/reset and scopes both list and statistics', async () => {
  const back = { page: 3, pageSize: 20, filter: 'same-name', focusKeyId: 21 }
  const fixture = await renderWorkflow(
    100,
    false,
    `/usage-logs/common?tokenId=21&pageSize=1&keyReturn=${encodeURIComponent(JSON.stringify(back))}`
  )
  expect(await screen.findByText('REQ-SYNTHETIC-1')).toBeInTheDocument()
  expect(screen.getByText('Requests for Key #21')).toBeVisible()
  await waitFor(() => {
    expect(
      fixture.requests.some(
        (url) =>
          url.pathname === '/api/log' &&
          url.searchParams.get('token_id') === '21'
      )
    ).toBe(true)
    expect(
      fixture.requests.some(
        (url) =>
          url.pathname === '/api/log/stat' &&
          url.searchParams.get('token_id') === '21'
      )
    ).toBe(true)
  })
  expect(screen.getByRole('link', { name: 'Back to Keys' })).toBeInTheDocument()
  const main = screen.getByRole('main', { name: 'Call records' })
  await fixture.user.click(
    within(main).getByRole('button', { name: /^Search$/ })
  )
  expect(fixture.router.state.location.search).toMatchObject({
    tokenId: 21,
    keyReturn: back,
  })
  await fixture.user.click(
    within(main).getByRole('button', { name: /^Reset$/ })
  )
  expect(fixture.router.state.location.search).toMatchObject({
    tokenId: 21,
    keyReturn: back,
  })
})

test('changing the selected Key hides the previous Key rows while its request is pending', async () => {
  const fixture = await renderWorkflow(
    100,
    false,
    '/usage-logs/common?tokenId=21&pageSize=1',
    22
  )
  expect(await screen.findByText('REQ-SYNTHETIC-1')).toBeInTheDocument()
  await act(async () => {
    await fixture.router.navigate({
      to: '/usage-logs/$section',
      params: { section: 'common' },
      search: { tokenId: 22, pageSize: 1 },
    })
  })
  await waitFor(() =>
    expect(screen.queryByText('REQ-SYNTHETIC-1')).not.toBeInTheDocument()
  )
  await act(async () => fixture.releaseSelf())
  expect(await screen.findByText('REQ-SYNTHETIC-1')).toBeInTheDocument()
})
