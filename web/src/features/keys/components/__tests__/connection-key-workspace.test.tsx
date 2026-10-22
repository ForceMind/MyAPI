/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AxiosAdapter } from 'axios'
import type { AnchorHTMLAttributes } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import type { ConnectionKeyContext } from '@/features/connections/key-context'
import { poolUsageFixture } from '@/features/personal-upstream/__tests__/pool-usage-fixture'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { ConnectionKeyWorkspace } from '../connection-key-workspace'

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    to,
    search,
    params,
    children,
    ...attributes
  }: AnchorHTMLAttributes<HTMLAnchorElement> & {
    to: string
    search?: unknown
    params?: { section: string }
  }) => (
    <a
      {...attributes}
      href={`${to.replace('$section', params?.section ?? '')}?${encodeURIComponent(JSON.stringify(search))}`}
    >
      {children}
    </a>
  ),
}))
vi.mock('@/hooks/use-sidebar-config', () => ({
  useIsSidebarModuleVisible: () => true,
}))
const originalAdapter = api.defaults.adapter
let cleanup: (() => void) | undefined
afterEach(() => {
  cleanup?.()
  api.defaults.adapter = originalAdapter
  vi.restoreAllMocks()
})

function mount(
  context: ConnectionKeyContext,
  options: {
    wrongPool?: boolean
    wrongChannel?: boolean
    freshRole?: number
    fail?: boolean
    empty?: boolean
    deleted?: boolean
    closed?: boolean
  } = {}
) {
  const user = { id: 9, username: 'operator', role: ROLE.SUPER_ADMIN }
  const session = {
    sid: 'associated-test',
    current: true,
    login_method: 'password',
    ip: '127.0.0.1',
    user_agent: 'test',
    created_at: 1,
    last_active_at: 1,
    expires_at: 4_000_000_000,
  }
  useAuthStore.getState().auth.setBundle({
    access_token: 'synthetic-associated',
    token_type: 'Bearer',
    access_expires_at: 4_000_000_000,
    user,
    session,
  })
  const requests: string[] = []
  let fail = options.fail
  const adapter: AxiosAdapter = async (config) => {
    const path = config.url ?? ''
    requests.push(path)
    expect(config.method).toBe('get')
    let data: unknown
    if (path === '/api/user/self') {
      data = { ...user, role: options.freshRole ?? user.role }
    } else if (path === '/api/subscription-share/channels/1/pools') {
      data = {
        channel_id: 1,
        has_more: false,
        items: options.empty
          ? []
          : [
              {
                pool_id: 7,
                channel_id: 1,
                enabled: !options.closed,
                window_source: 'primary',
                window_type: 'five_hour',
              },
            ],
      }
    } else if (path === '/api/subscription-share/pools/7/usage') {
      if (fail) throw new Error('unavailable')
      const pool = poolUsageFixture({
        pool_id: options.wrongPool ? 8 : 7,
        channel_id: options.wrongChannel ? 2 : 1,
        state: options.closed ? 'closed' : 'active',
      })
      pool.allocations.forEach((item) => {
        item.token_name = 'Same display name'
      })
      if (options.deleted) pool.allocations[0].token_state = 'deleted'
      data = pool
    } else if (/^\/api\/token\/(21|22)\/subscription-share$/.test(path)) {
      data = {
        token_id: Number(path.split('/')[3]),
        pools: [
          {
            ...poolUsageFixture(),
            share_bps: 1000,
            window_source: 'codex_wham_usage_primary',
          },
        ],
      }
    } else throw new Error(`Unexpected request ${path}`)
    return {
      data: { success: true, data },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    }
  }
  api.defaults.adapter = adapter
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const rendered = render(
    <QueryClientProvider client={client}>
      <ConnectionKeyWorkspace context={context} />
    </QueryClientProvider>
  )
  cleanup = () => {
    rendered.unmount()
    client.clear()
  }
  return {
    requests,
    changeContext: (next: ConnectionKeyContext) =>
      rendered.rerender(
        <QueryClientProvider client={client}>
          <ConnectionKeyWorkspace context={next} />
        </QueryClientProvider>
      ),
    recover: () => {
      fail = false
    },
    client,
  }
}

const context: ConnectionKeyContext = {
  connection: {
    connection: 1,
    page: 3,
    filter: 'same name',
    status: 'disabled',
  },
  pool: 7,
  poolPage: 3,
  key: 21,
}

test('same-name Keys use allocation IDs and preserve the exact connection context', async () => {
  const fixture = mount(context)
  const link = await screen.findByRole('link', { name: 'Key #22' })
  expect(
    JSON.parse(
      decodeURIComponent((link.getAttribute('href') ?? '').split('?')[1])
    )
  ).toEqual({ connectionContext: { ...context, key: 22 } })
  const back = screen.getByRole('link', { name: 'Back to connections' })
  expect(
    JSON.parse(
      decodeURIComponent((back.getAttribute('href') ?? '').split('?')[1])
    )
  ).toEqual(context.connection)
  expect(
    screen.getByRole('button', { name: 'Manage Key shares' })
  ).toBeEnabled()
  expect(fixture.requests.some((path) => path.includes('/key'))).toBe(false)
  const calls = screen.getByRole('link', { name: 'View Key requests' })
  const search = JSON.parse(
    decodeURIComponent((calls.getAttribute('href') ?? '').split('?')[1])
  )
  expect(search).toMatchObject({
    channel: '1',
    tokenId: 21,
    connectionContext: context,
  })
  expect(search.token).toBeUndefined()
  expect(search.returnToKeys).toBe(true)
  const windows = screen.getByRole('link', { name: 'All saved share windows' })
  expect(
    JSON.parse(
      decodeURIComponent((windows.getAttribute('href') ?? '').split('?')[1])
    )
  ).toEqual({
    connectionContext: { connection: context.connection, poolPage: 3 },
  })
})

test('a requested Key without a persisted allocation cannot open its editor', async () => {
  mount({ ...context, key: 99 })
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'This Key has no saved allocation'
  )
  expect(
    screen.queryByRole('button', { name: 'Manage Key shares' })
  ).not.toBeInTheDocument()
})

for (const mismatch of ['wrongPool', 'wrongChannel'] as const) {
  test(`rejects ${mismatch} before exposing any allocation actions`, async () => {
    mount(context, { [mismatch]: true })
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Unable to load window usage'
    )
    expect(
      screen.queryByRole('link', { name: 'Key #22' })
    ).not.toBeInTheDocument()
  })
}

test('fresh role revocation prevents pool reads despite a cached root role', async () => {
  const fixture = mount(context, { freshRole: ROLE.ADMIN })
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Root access is required'
  )
  expect(fixture.requests).toEqual(['/api/user/self'])
})

test('local role changes immediately remove allocations and their editor', async () => {
  mount(context)
  await screen.findByRole('link', { name: 'Key #22' })
  act(() =>
    useAuthStore
      .getState()
      .auth.setUser({ id: 9, username: 'operator', role: ROLE.ADMIN })
  )
  expect(
    screen.queryByRole('link', { name: 'Key #22' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Manage Key shares' })
  ).not.toBeInTheDocument()
})

test('failed pool reads expose retry and recover without a generic Key name search', async () => {
  const fixture = mount(context, { fail: true })
  await screen.findByRole('alert')
  fixture.recover()
  await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
  await screen.findByRole('link', { name: 'Key #22' })
  expect(
    fixture.requests.every((path) => !path.includes('/token/search'))
  ).toBe(true)
})

test('closed windows retain historical allocations while deleted Keys cannot be edited', async () => {
  mount(context, { closed: true, deleted: true })
  await screen.findByRole('link', { name: 'Key #22' })
  expect(
    screen.getByText(/Closed share window; historical allocations/)
  ).toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: 'Manage Key shares' })
  ).toBeDisabled()
})

test('no saved allocations does not claim the connection is unavailable', async () => {
  mount({ connection: context.connection }, { empty: true })
  expect(
    await screen.findByText('No saved Key allocations for this connection.')
  ).toBeInTheDocument()
  expect(screen.queryByText('Connection unavailable')).not.toBeInTheDocument()
})

test('leaving the workspace cancels an outstanding pool read', async () => {
  const fixture = mount(context)
  await screen.findByRole('link', { name: 'Key #22' })
  let signal: AbortSignal | undefined
  api.defaults.adapter = async (config) => {
    signal = config.signal as AbortSignal
    return new Promise(() => {})
  }
  void fixture.client.invalidateQueries({ queryKey: ['connection-key-usage'] })
  await waitFor(() => expect(signal).toBeDefined())
  cleanup?.()
  cleanup = undefined
  expect(signal?.aborted).toBe(true)
})

test('canceling the scoped editor returns to the same Key and preserves inventory context', async () => {
  const fixture = mount(context)
  const edit = await screen.findByRole('button', { name: 'Manage Key shares' })
  await userEvent.click(edit)
  await screen.findByRole('radio', { name: 'Subscription account' })
  await userEvent.keyboard('{Escape}')
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
  expect(screen.getByRole('heading', { name: 'Key #21' })).toBeInTheDocument()
  expect(
    screen
      .getByRole('link', { name: 'Back to connections' })
      .getAttribute('href')
  ).toContain(encodeURIComponent(JSON.stringify(context.connection)))
  expect(fixture.requests).toContain('/api/token/21/subscription-share')
})

test('newer Key navigation and back do not reopen an editor for the old selection', async () => {
  const fixture = mount(context)
  await userEvent.click(
    await screen.findByRole('button', { name: 'Manage Key shares' })
  )
  await screen.findByRole('radio', { name: 'Subscription account' })
  fixture.changeContext({ ...context, key: 22 })
  await screen.findByRole('heading', { name: 'Key #22' })
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  fixture.changeContext(context)
  await screen.findByRole('heading', { name: 'Key #21' })
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})
