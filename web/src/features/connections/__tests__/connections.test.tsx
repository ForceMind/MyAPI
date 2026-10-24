/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AxiosError, CanceledError, type AxiosAdapter } from 'axios'
import { useState, type AnchorHTMLAttributes } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'
import { useInspectorStore } from '@/stores/inspector-store'

import type { ConnectionsSearch } from '../connection-state'
import { ConnectionsWorkspace } from '../connections'

vi.mock('@tanstack/react-router', () => ({
  getRouteApi: () => ({}),
  Link: ({
    to,
    params,
    search,
    children,
    ...attributes
  }: AnchorHTMLAttributes<HTMLAnchorElement> & {
    to: string
    params?: { section: string }
    search?: Record<string, unknown>
  }) => (
    <a
      {...attributes}
      href={`${to.replace('$section', params?.section ?? '')}${
        search
          ? `?${new URLSearchParams(
              Object.entries(search)
                .filter(([, v]) => v !== undefined)
                .map(([k, v]) => [
                  k,
                  typeof v === 'object' ? JSON.stringify(v) : String(v),
                ])
            )}`
          : ''
      }`}
    >
      {children}
    </a>
  ),
}))
vi.mock('@/hooks/use-sidebar-config', () => ({
  useIsSidebarModuleVisible: () => true,
}))
// The established editor has its own mutation/security tests. Here we assert its
// target, explicit mount and cancellation boundary without duplicating the form.
vi.mock('@/features/channels/components/drawers/channel-mutate-drawer', () => ({
  ChannelMutateDrawer: ({
    currentRow,
    onOpenChange,
  }: {
    currentRow: { id: number } | null
    onOpenChange: (open: boolean) => void
  }) => (
    <div
      role='dialog'
      aria-label={currentRow ? `Edit #${currentRow.id}` : 'Add connection'}
    >
      <button type='button' onClick={() => onOpenChange(false)}>
        Cancel edit
      </button>
    </div>
  ),
}))

const originalAdapter = api.defaults.adapter
const originalMatchMedia = Object.getOwnPropertyDescriptor(
  window,
  'matchMedia'
) ?? { configurable: true, value: window.matchMedia }
const connection = {
  id: 7,
  type: 1,
  name: 'Synthetic connection',
  status: 1,
  models: 'alpha,beta',
  group: 'default',
  priority: 0,
  key: 'SECRET-SENTINEL',
}
let teardown: (() => void) | undefined

async function leaveDetail(
  user: ReturnType<typeof userEvent.setup>,
  mobile: boolean
) {
  if (mobile) {
    // The console top bar's Back closes the full-screen inspector.
    act(() => useInspectorStore.getState().current?.close())
    return
  }
  await user.click(screen.getByRole('button', { name: 'Close details' }))
}

afterEach(() => {
  useInspectorStore.setState({ current: null })
  teardown?.()
  api.defaults.adapter = originalAdapter
  Object.defineProperty(window, 'matchMedia', originalMatchMedia)
  vi.restoreAllMocks()
})

function mount(
  options: {
    mobile?: boolean
    initial?: ConnectionsSearch
    read?: boolean
    write?: boolean
    failDetail?: boolean
    windowsFail?: boolean
    wrongWindow?: boolean
    empty?: boolean
    holdList?: boolean
    failList?: boolean
    missingRow?: boolean
    holdDetail?: boolean
    freshId?: number
  } = {}
) {
  // 390px phone or 1440px desktop (inspector beside the list).
  const width = options.mobile ? 390 : 1440
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    value: vi.fn((query: string) => {
      const max = /max-width:\s*(\d+)px/.exec(query)
      const min = /min-width:\s*(\d+)px/.exec(query)
      let matches = false
      if (max) matches = width <= Number(max[1])
      if (min) matches = width >= Number(min[1])
      return {
        matches,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }
    }),
  })
  const user = {
    id: 7,
    username: 'synthetic-operator',
    role: ROLE.ADMIN,
    permissions: {
      admin_permissions: { channel: { read: true, write: true } },
    },
  }
  const session = {
    sid: 'connections-test-session',
    current: true,
    login_method: 'password',
    ip: '127.0.0.1',
    user_agent: 'test',
    created_at: 1,
    last_active_at: 1,
    expires_at: 4_000_000_000,
  }
  useAuthStore.getState().auth.setBundle({
    access_token: 'synthetic-connection-access',
    token_type: 'Bearer',
    access_expires_at: 4_000_000_000,
    user,
    session,
  })
  const requests: { path: string; params: Record<string, unknown> }[] = []
  let releaseList!: () => void
  const listGate = new Promise<void>((resolve) => {
    releaseList = resolve
  })
  let failDetail = options.failDetail
  let detailAborted = false
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const adapter: AxiosAdapter = async (config) => {
    const path = config.url ?? ''
    requests.push({ path, params: config.params ?? {} })
    expect(config.method).toBe('get')
    let data: unknown
    if (path === '/api/user/self') {
      data = {
        success: true,
        data: {
          ...user,
          id: options.freshId ?? user.id,
          permissions: {
            admin_permissions: {
              channel: {
                read: options.read ?? true,
                write: options.write ?? true,
              },
            },
          },
        },
      }
    } else if (path === '/api/channel' || path === '/api/channel/search') {
      if (options.holdList) await listGate
      if (options.failList) throw new Error('Inventory unavailable')
      data = {
        success: true,
        data: {
          total: options.empty ? 0 : 21,
          items: options.empty
            ? []
            : [{ ...connection, id: options.missingRow ? 8 : 7 }],
        },
      }
    } else if (path === '/api/channel/7') {
      if (options.holdDetail) {
        return new Promise((_resolve, reject) => {
          config.signal?.addEventListener?.('abort', () => {
            detailAborted = true
            reject(new CanceledError('Observer left', config))
          })
        })
      }
      if (failDetail) {
        throw new AxiosError(
          'Service unavailable',
          'ERR_BAD_RESPONSE',
          config,
          undefined,
          {
            config,
            status: 503,
            statusText: 'Unavailable',
            headers: {},
            data: { success: false },
          }
        )
      }
      data = { success: true, data: connection }
    } else if (path === '/api/channel/quota/changes') {
      expect(config.params.channel_ids).toBe('7')
      data = options.windowsFail
        ? { success: false }
        : {
            success: true,
            data: {
              items: options.wrongWindow
                ? [
                    {
                      channel_id: 8,
                      name: 'Wrong connection',
                      metric_type: 'codex_rate_limit',
                      unit: 'percent',
                    },
                  ]
                : [],
            },
          }
    } else throw new Error(`Unexpected request: ${path}`)
    return { config, status: 200, statusText: 'OK', headers: {}, data }
  }
  api.defaults.adapter = adapter
  let latestSearch = options.initial ?? {}
  let navigate!: (next: ConnectionsSearch) => void
  function Harness() {
    const [search, setSearch] = useState<ConnectionsSearch>(
      options.initial ?? {}
    )
    latestSearch = search
    navigate = setSearch
    return <ConnectionsWorkspace search={search} onSearch={setSearch} />
  }
  const view = render(
    <QueryClientProvider client={client}>
      <Harness />
    </QueryClientProvider>
  )
  teardown = () => {
    view.unmount()
    client.clear()
  }
  return {
    requests,
    releaseList,
    navigate: (next: ConnectionsSearch) => navigate(next),
    client,
    user,
    session,
    get search() {
      return latestSearch
    },
    recover: () => {
      failDetail = false
    },
    get detailAborted() {
      return detailAborted
    },
  }
}

test('desktop list/detail uses one inventory read, scopes logs by stable ID and never caches credentials', async () => {
  const fixture = mount()
  const user = userEvent.setup()
  await user.click(
    await screen.findByRole('button', { name: /Synthetic connection/ })
  )
  await screen.findByRole('heading', { name: 'Synthetic connection' })
  expect(screen.getByTestId('connections-inventory')).toBeVisible()
  expect(screen.getByTestId('connection-detail')).toBeVisible()
  const calls = new URL(
    screen
      .getByRole('link', { name: 'View connection requests' })
      .getAttribute('href') ?? '',
    'https://example.test'
  )
  expect(calls.pathname).toBe('/usage-logs/common')
  expect(calls.searchParams.get('channel')).toBe('7')
  expect(JSON.parse(calls.searchParams.get('connectionContext') ?? '')).toEqual(
    {
      connection: fixture.search,
    }
  )
  expect(calls.searchParams.get('page')).toBe('1')
  expect(calls.searchParams.get('pageSize')).toBe('20')
  expect(
    screen.getByText(
      'Configuration status does not confirm upstream availability.'
    )
  ).toBeInTheDocument()
  await user.click(screen.getByRole('tab', { name: 'Account windows' }))
  expect(
    await screen.findByText(
      'No saved subscription windows for this connection. This does not mean the connection is unavailable.'
    )
  ).toBeInTheDocument()
  expect(
    fixture.requests.filter((request) => request.path === '/api/channel')
  ).toHaveLength(1)
  expect(
    JSON.stringify(
      fixture.client
        .getQueryCache()
        .getAll()
        .map((query) => query.state.data)
    )
  ).not.toContain('SECRET-SENTINEL')
})

test('mobile opens a separate detail and restores selected-row focus and filter context on return', async () => {
  const fixture = mount({
    mobile: true,
    initial: { filter: 'Synthetic', page: 2, status: 'enabled' },
  })
  const user = userEvent.setup()
  await user.click(
    await screen.findByRole('button', { name: /Synthetic connection/ })
  )
  await screen.findByRole('heading', { name: 'Synthetic connection' })
  expect(screen.getByTestId('connections-inventory')).not.toBeVisible()
  expect(
    screen.getByRole('heading', { level: 2, name: 'Synthetic connection' })
  ).toHaveFocus()
  await leaveDetail(user, true)
  expect(screen.queryByTestId('connection-detail')).not.toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: /Synthetic connection/ })
  ).toHaveFocus()
  expect(fixture.search).toEqual({
    filter: 'Synthetic',
    page: 2,
    status: 'enabled',
    connection: undefined,
  })
})

test('search and pagination use server parameters rather than filtering one page locally', async () => {
  const fixture = mount()
  const user = userEvent.setup()
  await screen.findByRole('button', { name: /Synthetic connection/ })
  await user.type(
    screen.getByRole('textbox', { name: 'Search connections' }),
    'alpha'
  )
  await user.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() =>
    expect(
      fixture.requests.some(
        (request) =>
          request.path === '/api/channel/search' &&
          request.params.keyword === 'alpha' &&
          request.params.p === 1
      )
    ).toBe(true)
  )
  await user.click(await screen.findByRole('button', { name: 'Next page' }))
  await waitFor(() =>
    expect(
      fixture.requests.some(
        (request) =>
          request.params.keyword === 'alpha' && request.params.p === 2
      )
    ).toBe(true)
  )
})

test('editing is explicit, targets the selected connection, and cancel restores focus without writes', async () => {
  mount({ initial: { connection: 7 } })
  const user = userEvent.setup()
  await screen.findByRole('button', { name: 'Edit connection' })
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Edit connection' }))
  expect(screen.getByRole('dialog', { name: 'Edit #7' })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Cancel edit' }))
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Edit connection' })).toHaveFocus()
})

test('a fresh read-only grant hides editing while retaining the read workflow', async () => {
  mount({ initial: { connection: 7 }, write: false })
  await screen.findByRole('heading', { name: 'Synthetic connection' })
  expect(
    screen.queryByRole('button', { name: 'Edit connection' })
  ).not.toBeInTheDocument()
})

test.each([{ read: false }, { freshId: 8 }])(
  'unverified or revoked identity never starts channel reads',
  async (options) => {
    const fixture = mount(options)
    await screen.findByText('Connection access could not be verified.')
    expect(fixture.requests.map((request) => request.path)).toEqual([
      '/api/user/self',
    ])
  }
)

test('detail 503 remains an error, never an empty list, and explicit retry recovers', async () => {
  const fixture = mount({ initial: { connection: 7 }, failDetail: true })
  const detail = within(await screen.findByTestId('connection-detail'))
  await detail.findByText(
    'Unable to load this connection. It may have been removed or access may have changed.'
  )
  expect(detail.queryByText('No connections yet')).not.toBeInTheDocument()
  fixture.recover()
  await userEvent.setup().click(detail.getByRole('button', { name: 'Retry' }))
  await detail.findByRole('heading', { name: 'Synthetic connection' })
})

test.each([{ windowsFail: true }, { wrongWindow: true }])(
  'missing or mismatched saved-window evidence is not represented as a healthy empty result',
  async (options) => {
    mount({ ...options, initial: { connection: 7 } })
    await userEvent
      .setup()
      .click(await screen.findByRole('tab', { name: 'Account windows' }))
    await screen.findByText('Unable to load account windows')
    expect(
      screen.queryByText(
        'No saved subscription windows for this connection. This does not mean the connection is unavailable.'
      )
    ).not.toBeInTheDocument()
  }
)

test('leaving a pending detail aborts only that observer and does not fetch provider data', async () => {
  const fixture = mount({ initial: { connection: 7 }, holdDetail: true })
  await waitFor(() =>
    expect(
      fixture.requests.some((request) => request.path === '/api/channel/7')
    ).toBe(true)
  )
  await leaveDetail(userEvent.setup(), false)
  await waitFor(() => expect(fixture.detailAborted).toBe(true))
  expect(
    fixture.requests.every((request) =>
      ['/api/user/self', '/api/channel', '/api/channel/7'].includes(
        request.path
      )
    )
  ).toBe(true)
})

test('same-session permission revocation immediately removes detail and editor', async () => {
  const fixture = mount({ initial: { connection: 7 } })
  await userEvent
    .setup()
    .click(await screen.findByRole('button', { name: 'Edit connection' }))
  expect(screen.getByRole('dialog')).toBeVisible()
  act(() =>
    useAuthStore.getState().auth.setUser({
      ...fixture.user,
      permissions: {
        admin_permissions: { channel: { read: false, write: false } },
      },
    })
  )
  await screen.findByText('Connection access could not be verified.')
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(screen.queryByTestId('connection-detail')).not.toBeInTheDocument()
})

test('empty inventory stays distinct from a read error and keeps a working advanced entry', async () => {
  mount({ empty: true })
  await screen.findByText('No connections yet')
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Table view' })).toHaveAttribute(
    'href',
    '/channels'
  )
})

test('a session change closes the previous editor before the next identity is verified', async () => {
  const fixture = mount({ initial: { connection: 7 } })
  await userEvent
    .setup()
    .click(await screen.findByRole('button', { name: 'Edit connection' }))
  const before = fixture.requests.filter((request) =>
    request.path.startsWith('/api/channel')
  ).length
  act(() =>
    useAuthStore.getState().auth.setBundle({
      access_token: 'synthetic-new-session',
      token_type: 'Bearer',
      access_expires_at: 4_000_000_000,
      user: { ...fixture.user, id: 8 },
      session: { ...fixture.session, sid: 'connections-next-session' },
    })
  )
  await screen.findByText('Connection access could not be verified.')
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(screen.queryByText('Synthetic connection')).not.toBeInTheDocument()
  expect(
    fixture.requests.filter((request) =>
      request.path.startsWith('/api/channel')
    )
  ).toHaveLength(before)
})

test('returning before the inventory arrives restores focus when the row becomes available', async () => {
  const fixture = mount({
    mobile: true,
    initial: { connection: 7 },
    holdList: true,
  })
  await screen.findByRole('heading', { name: 'Synthetic connection' })
  await leaveDetail(userEvent.setup(), true)
  await act(async () => fixture.releaseList())
  const row = await screen.findByRole('button', {
    name: /Synthetic connection/,
  })
  await waitFor(() => expect(row).toHaveFocus())
})

test.each([{ empty: true }, { missingRow: true }, { failList: true }])(
  'returning to an inventory without the target or with an error focuses its heading: %j',
  async (options) => {
    const fixture = mount({
      ...options,
      mobile: true,
      initial: { connection: 7 },
      holdList: true,
    })
    await screen.findByRole('heading', { name: 'Synthetic connection' })
    await leaveDetail(userEvent.setup(), true)
    await act(async () => fixture.releaseList())
    await waitFor(() =>
      expect(
        screen.getByRole('heading', { name: 'Configured connections' })
      ).toHaveFocus()
    )
  }
)

test.each(['focus', 'pointer', 'keyboard'] as const)(
  'interacting with the list by %s cancels delayed focus restoration',
  async (interaction) => {
    const fixture = mount({
      mobile: true,
      initial: { connection: 7 },
      holdList: true,
    })
    const user = userEvent.setup()
    await screen.findByRole('heading', { name: 'Synthetic connection' })
    await leaveDetail(user, true)
    const input = screen.getByRole('textbox', { name: 'Search connections' })
    if (interaction === 'focus') {
      act(() => input.focus())
    } else if (interaction === 'pointer') {
      await user.click(
        screen.getByRole('heading', { name: 'Configured connections' })
      )
    } else {
      await user.keyboard('{Escape}')
    }
    const focusBefore = document.activeElement
    await act(async () => fixture.releaseList())
    await screen.findByRole('button', { name: /Synthetic connection/ })
    expect(document.activeElement).toBe(focusBefore)
  }
)

test.each([
  { page: 2 },
  { filter: 'Synthetic' },
  { status: 'enabled' as const },
])(
  'new inventory navigation cancels a pending return even without a pointer event: %j',
  async (next) => {
    const fixture = mount({
      mobile: true,
      initial: { connection: 7 },
      holdList: true,
    })
    await screen.findByRole('heading', { name: 'Synthetic connection' })
    await leaveDetail(userEvent.setup(), true)
    act(() => fixture.navigate(next))
    await act(async () => fixture.releaseList())
    const row = await screen.findByRole('button', {
      name: /Synthetic connection/,
    })
    expect(row).not.toHaveFocus()
    expect(
      screen.getByRole('heading', { name: 'Configured connections' })
    ).not.toHaveFocus()
  }
)

test('reopening detail cancels an old pending return and a later Back restores the current row', async () => {
  const fixture = mount({
    mobile: true,
    initial: { connection: 7 },
    holdList: true,
  })
  const user = userEvent.setup()
  await screen.findByRole('heading', { name: 'Synthetic connection' })
  await leaveDetail(user, true)
  act(() => fixture.navigate({ connection: 7 }))
  const heading = screen.getByRole('heading', {
    level: 2,
    name: 'Synthetic connection',
  })
  expect(heading).toHaveFocus()
  await act(async () => fixture.releaseList())
  await waitFor(() => expect(fixture.client.isFetching()).toBe(0))
  expect(heading).toHaveFocus()
  await leaveDetail(user, true)
  expect(
    await screen.findByRole('button', { name: /Synthetic connection/ })
  ).toHaveFocus()
})
