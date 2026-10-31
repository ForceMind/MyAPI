/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useInspectorStore } from '@/stores/inspector-store'

import { mountConnections } from './connections-page-fixture'

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
let teardown: (() => void) | undefined

afterEach(() => {
  teardown?.()
  teardown = undefined
  api.defaults.adapter = originalAdapter
  Object.defineProperty(window, 'matchMedia', originalMatchMedia)
  useAuthStore.getState().auth.reset('idle')
  vi.restoreAllMocks()
})

async function mount(options: Parameters<typeof mountConnections>[0] = {}) {
  const fixture = await mountConnections(options)
  teardown = fixture.teardown
  return fixture
}

/** The card's link (the card's name); the whole card opens the page. */
const cardLink = () =>
  screen.findByRole('link', { name: 'Synthetic connection' })
const search = (fixture: Awaited<ReturnType<typeof mount>>) =>
  fixture.router.state.location.search as Record<string, unknown>

/** Back from the connection page, as the shell's breadcrumb or top bar does. */
async function leavePage() {
  await act(async () => useInspectorStore.getState().current?.close())
}

test('the grid reads the inventory once, a card opens its page, call records are scoped by the stable ID and credentials are never cached', async () => {
  const fixture = await mount()
  const user = userEvent.setup()
  await user.click(await cardLink())
  await screen.findByRole('heading', { level: 1, name: 'Synthetic connection' })
  expect(fixture.router.state.location.pathname).toBe('/connections/7')
  await user.click(screen.getByRole('tab', { name: 'Requests' }))
  const calls = new URL(
    screen
      .getByRole('link', { name: 'View connection requests' })
      .getAttribute('href') ?? '',
    'https://example.test'
  )
  expect(calls.pathname).toBe('/usage-logs/common')
  expect(JSON.parse(calls.searchParams.get('channel') ?? '')).toBe('7')
  expect(JSON.parse(calls.searchParams.get('connectionContext') ?? '')).toEqual(
    { connection: { connection: 7 } }
  )
  expect(calls.searchParams.get('page')).toBe('1')
  expect(calls.searchParams.get('pageSize')).toBe('20')
  await user.click(screen.getByRole('tab', { name: 'Routing and models' }))
  expect(
    screen.getByText(
      'Configuration status does not confirm upstream availability.'
    )
  ).toBeInTheDocument()
  await user.click(screen.getByRole('tab', { name: 'Windows' }))
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

test('mobile: a card opens the page and Back restores the card focus and the list filters', async () => {
  const fixture = await mount({
    mobile: true,
    entry: '/connections?filter=Synthetic&page=2&status=enabled',
  })
  const user = userEvent.setup()
  await user.click(await cardLink())
  const heading = await screen.findByRole('heading', {
    level: 1,
    name: 'Synthetic connection',
  })
  await waitFor(() => expect(heading).toHaveFocus())
  expect(search(fixture).connectionReturn).toEqual({
    filter: 'Synthetic',
    page: 2,
    status: 'enabled',
  })
  await leavePage()
  await waitFor(() =>
    expect(fixture.router.state.location.pathname).toBe('/connections')
  )
  await waitFor(async () => expect(await cardLink()).toHaveFocus())
  expect(search(fixture)).toEqual({
    filter: 'Synthetic',
    page: 2,
    status: 'enabled',
  })
})

test('Back from a shared connection link returns to its list and focuses the card', async () => {
  const fixture = await mount({
    entry: `/connections/7?connectionReturn=${encodeURIComponent(
      JSON.stringify({ filter: 'Synthetic', page: 2 })
    )}`,
  })
  await screen.findByRole('heading', { level: 1, name: 'Synthetic connection' })
  await leavePage()
  await waitFor(() =>
    expect(fixture.router.state.location.pathname).toBe('/connections')
  )
  expect(search(fixture)).toEqual({
    filter: 'Synthetic',
    page: 2,
    focusConnectionId: 7,
  })
  await waitFor(async () => expect(await cardLink()).toHaveFocus())
  // Replaced: the shared link is not a step back.
  expect(fixture.router.history.length).toBe(1)
})

test('search and pagination use server parameters rather than filtering one page locally', async () => {
  const fixture = await mount({ total: 21 })
  const user = userEvent.setup()
  await cardLink()
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
  await user.click(await screen.findByRole('button', { name: 'Go to next page' }))
  await waitFor(() =>
    expect(
      fixture.requests.some(
        (request) =>
          request.params.keyword === 'alpha' && request.params.p === 2
      )
    ).toBe(true)
  )
})

test('editing is explicit, targets the open connection, and cancel restores focus without writes', async () => {
  const fixture = await mount({ entry: '/connections/7' })
  const user = userEvent.setup()
  await screen.findByRole('button', { name: 'Edit connection' })
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Edit connection' }))
  expect(screen.getByRole('dialog', { name: 'Edit #7' })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Cancel edit' }))
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Edit connection' })).toHaveFocus()
  expect(fixture.requests.every((request) => request.method === 'get')).toBe(
    true
  )
})

test('a fresh read-only grant hides editing while retaining the read workflow', async () => {
  await mount({ entry: '/connections/7', write: false })
  await screen.findByRole('heading', { level: 1, name: 'Synthetic connection' })
  expect(
    screen.queryByRole('button', { name: 'Edit connection' })
  ).not.toBeInTheDocument()
})

test.each([
  { read: false, entry: '/connections' },
  { freshId: 8, entry: '/connections' },
  { read: false, entry: '/connections/7' },
  { freshId: 8, entry: '/connections/7' },
])(
  'unverified or revoked identity never starts channel reads: %j',
  async (options) => {
    const fixture = await mount(options)
    await screen.findByText('Connection access could not be verified.')
    expect(fixture.requests.map((request) => request.path)).toEqual([
      '/api/user/self',
    ])
  }
)

test('detail 503 remains an error, never an empty list, and explicit retry recovers', async () => {
  const fixture = await mount({ entry: '/connections/7', failDetail: true })
  const page = within(await screen.findByTestId('connection-page'))
  await page.findByText(
    'Unable to load this connection. It may have been removed or access may have changed.'
  )
  expect(page.queryByText('No connections yet')).not.toBeInTheDocument()
  fixture.recover()
  await userEvent.setup().click(page.getByRole('button', { name: 'Retry' }))
  await page.findByRole('heading', { level: 1, name: 'Synthetic connection' })
})

test.each([{ windowsFail: true }, { wrongWindow: true }])(
  'missing or mismatched saved-window evidence is not represented as a healthy empty result: %j',
  async (options) => {
    await mount({ ...options, entry: '/connections/7' })
    await screen.findByText('Unable to load account windows')
    expect(
      screen.queryByText(
        'No saved subscription windows for this connection. This does not mean the connection is unavailable.'
      )
    ).not.toBeInTheDocument()
  }
)

test('leaving a pending page aborts only that observer and does not fetch provider data', async () => {
  const fixture = await mount({ entry: '/connections/7', holdDetail: true })
  await waitFor(() =>
    expect(
      fixture.requests.some((request) => request.path === '/api/channel/7')
    ).toBe(true)
  )
  await act(() => fixture.router.navigate({ to: '/connections' }))
  await waitFor(() => expect(fixture.detailAborted).toBe(true))
  // Saved configuration, observations and call records only.
  expect(
    fixture.requests.every(
      (request) =>
        request.method === 'get' &&
        [
          '/api/user/self',
          '/api/channel',
          '/api/channel/7',
          '/api/channel/quota/changes',
          '/api/log',
          '/pg/keys',
        ].includes(request.path)
    )
  ).toBe(true)
})

test('same-session permission revocation immediately removes the page and editor', async () => {
  const fixture = await mount({ entry: '/connections/7' })
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
  expect(screen.queryByText('Windows')).not.toBeInTheDocument()
})

test('empty inventory stays distinct from a read error and keeps a working advanced entry', async () => {
  await mount({ empty: true })
  await screen.findByText('No connections yet')
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  const table = new URL(
    screen.getByRole('link', { name: 'Table view' }).getAttribute('href') ?? '',
    'https://example.test'
  )
  expect(table.pathname).toBe('/channels')
  expect(table.searchParams.get('pageSize')).toBe('20')
})

test('card editing targets the selected connection and cancellation restores focus without any writes or provider test', async () => {
  const fixture = await mount({ operate: true })
  const user = userEvent.setup()
  const card = within(await screen.findByRole('article', { name: 'Synthetic connection' }))
  await user.click(card.getByRole('button', { name: 'Edit' }))
  expect(screen.getByRole('dialog', { name: 'Edit #7' })).toBeVisible()
  expect(fixture.router.state.location.pathname).toBe('/connections')
  await user.click(screen.getByRole('button', { name: 'Cancel edit' }))
  expect(card.getByRole('button', { name: 'Edit' })).toHaveFocus()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(fixture.requests.every((request) => request.method === 'get')).toBe(true)
  expect(fixture.requests.some((request) => request.path.startsWith('/api/channel/test/'))).toBe(false)
})

test('a fresh read-only grant keeps card viewing while hiding edit controls', async () => {
  await mount({ write: false })
  const card = within(await screen.findByRole('article', { name: 'Synthetic connection' }))
  expect(card.getByRole('link', { name: 'View' })).toBeVisible()
  expect(card.queryByRole('button', { name: 'Edit' })).not.toBeInTheDocument()
})

test('changing page size resets to page one and preserves model, provider, group and status filters', async () => {
  const fixture = await mount({
    entry: `/connections?filter=Synthetic&model=alpha&type=${encodeURIComponent('["57"]')}&group=${encodeURIComponent('["default","premium"]')}&status=enabled&page=2&pageSize=20`,
    total: 75,
  })
  const user = userEvent.setup()
  await cardLink()
  await user.selectOptions(screen.getByRole('combobox', { name: 'Rows per page' }), '50')
  await waitFor(() => expect(search(fixture)).toEqual({
    filter: 'Synthetic',
    model: 'alpha',
    type: ['57'],
    group: ['default', 'premium'],
    status: 'enabled',
    page: 1,
    pageSize: 50,
  }))
  await waitFor(() => expect(fixture.requests.some((request) =>
    request.path === '/api/channel/search' && request.params.page_size === 50 && request.params.p === 1 && request.params.model === 'alpha'
  )).toBe(true))
})

test('submitting model search retains provider and group arrays and uses the server search endpoint', async () => {
  const fixture = await mount({
    entry: `/connections?type=${encodeURIComponent('["57","1"]')}&group=${encodeURIComponent('["default","premium"]')}&pageSize=50`,
  })
  const user = userEvent.setup()
  await cardLink()
  expect(screen.getByText('Only the first value of each filter is applied.')).toBeVisible()
  await user.type(screen.getByRole('textbox', { name: 'Model' }), 'beta')
  await user.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() => expect(search(fixture)).toEqual({
    type: ['57', '1'],
    group: ['default', 'premium'],
    pageSize: 50,
    page: 1,
    model: 'beta',
  }))
  await waitFor(() => expect(fixture.requests.some((request) =>
    request.path === '/api/channel/search' && request.params.model === 'beta'
  )).toBe(true))
})

test('explicit single-value filter edits replace legacy arrays only when submitted', async () => {
  const fixture = await mount({
    entry: `/connections?model=alpha&type=${encodeURIComponent('["57","1"]')}&group=${encodeURIComponent('["default","premium"]')}&status=enabled&pageSize=50`,
  })
  const user = userEvent.setup()
  await cardLink()
  expect(screen.getByLabelText('Provider')).toHaveValue('57')
  expect(screen.getByLabelText('Group')).toHaveValue('default')
  await user.selectOptions(screen.getByLabelText('Provider'), '1')
  await user.clear(screen.getByLabelText('Group'))
  await user.type(screen.getByLabelText('Group'), 'research')
  expect(search(fixture).type).toEqual(['57', '1'])
  expect(search(fixture).group).toEqual(['default', 'premium'])
  await user.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() => expect(search(fixture)).toEqual({
    model: 'alpha', type: ['1'], group: ['research'], status: 'enabled', pageSize: 50, page: 1,
  }))
  expect(screen.queryByText('Only the first value of each filter is applied.')).not.toBeInTheDocument()
})

test('switching to the table keeps the complete filter context and never starts a provider request', async () => {
  const fixture = await mount({
    entry: `/connections?filter=Synthetic&model=alpha&type=${encodeURIComponent('["57","1"]')}&group=${encodeURIComponent('["default","premium"]')}&status=enabled&page=2&pageSize=50`,
  })
  await cardLink()
  await userEvent.setup().click(screen.getByRole('link', { name: 'Table view' }))
  await waitFor(() => expect(fixture.router.state.location.pathname).toBe('/channels'))
  expect(search(fixture)).toEqual({
    filter: 'Synthetic', model: 'alpha', type: ['57', '1'], group: ['default', 'premium'], status: ['enabled'], page: 2, pageSize: 50,
  })
  expect(fixture.requests.every((request) => request.method === 'get')).toBe(true)
  expect(fixture.requests.some((request) => request.path.startsWith('/api/channel/test/'))).toBe(false)
})

test('an inventory read error is an error with retry, not an empty grid', async () => {
  await mount({ failList: true })
  await screen.findByText('Unable to load connections')
  expect(screen.queryByText('No connections yet')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Retry' })).toBeVisible()
})

test('a session change closes the previous editor before the next identity is verified', async () => {
  const fixture = await mount({ entry: '/connections/7' })
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

/** Opens the card's page, then goes Back while the next list read waits. */
async function openAndReturnWhileListLoads(
  fixture: Awaited<ReturnType<typeof mount>>
) {
  const user = userEvent.setup()
  await user.click(await cardLink())
  await screen.findByRole('heading', { level: 1, name: 'Synthetic connection' })
  fixture.client.removeQueries({ queryKey: ['channels', 'list'] })
  fixture.holdLists()
  await leavePage()
  await waitFor(() =>
    expect(fixture.router.state.location.pathname).toBe('/connections')
  )
  return user
}

test('returning before the inventory arrives restores focus when the card becomes available', async () => {
  const fixture = await mount({ mobile: true })
  await openAndReturnWhileListLoads(fixture)
  await act(async () => fixture.releaseList())
  const link = await cardLink()
  await waitFor(() => expect(link).toHaveFocus())
})

test.each([{ empty: true }, { missingRow: true }, { failList: true }])(
  'returning to an inventory without the card or with an error focuses its heading: %j',
  async (options) => {
    const fixture = await mount({ mobile: true })
    await openAndReturnWhileListLoads(fixture)
    Object.assign(fixture.state, options)
    await act(async () => fixture.releaseList())
    await waitFor(() =>
      expect(
        screen.getByRole('heading', { name: 'Configured connections' })
      ).toHaveFocus()
    )
  }
)

test.each(['focus', 'pointer', 'keyboard'] as const)(
  'interacting with the grid by %s cancels delayed focus restoration',
  async (interaction) => {
    const fixture = await mount({ mobile: true })
    const user = await openAndReturnWhileListLoads(fixture)
    const input = screen.getByRole('textbox', { name: 'Search connections' })
    if (interaction === 'focus') {
      act(() => input.focus())
    } else if (interaction === 'pointer') {
      await user.click(input)
    } else {
      await user.keyboard('{Escape}')
    }
    const focusBefore = document.activeElement
    await act(async () => fixture.releaseList())
    await cardLink()
    expect(document.activeElement).toBe(focusBefore)
  }
)

test.each([
  { page: 2 },
  { filter: 'Synthetic' },
  { status: 'enabled' as const },
])(
  'new grid navigation cancels a pending return even without a pointer event: %j',
  async (next) => {
    const fixture = await mount({ mobile: true })
    await openAndReturnWhileListLoads(fixture)
    await act(() =>
      fixture.router.navigate({ to: '/connections', search: next })
    )
    await act(async () => fixture.releaseList())
    const link = await cardLink()
    expect(link).not.toHaveFocus()
    expect(
      screen.getByRole('heading', { name: 'Configured connections' })
    ).not.toHaveFocus()
  }
)

test('reopening a card cancels an old pending return and a later Back restores that card', async () => {
  const fixture = await mount({ mobile: true })
  const user = await openAndReturnWhileListLoads(fixture)
  await act(async () => fixture.releaseList())
  await user.click(await cardLink())
  const heading = await screen.findByRole('heading', {
    level: 1,
    name: 'Synthetic connection',
  })
  await waitFor(() => expect(heading).toHaveFocus())
  await leavePage()
  await waitFor(async () => expect(await cardLink()).toHaveFocus())
})
