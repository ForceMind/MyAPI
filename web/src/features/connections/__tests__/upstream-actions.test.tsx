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

import {
  CONNECTION,
  connectionLog,
  mountConnections,
} from './connections-page-fixture'

vi.mock('@/hooks/use-sidebar-config', () => ({
  useIsSidebarModuleVisible: () => true,
}))
vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn(), warning: vi.fn() },
}))
// The connection editor has its own tests; this stand-in saves connection 9.
vi.mock('@/features/channels/components/drawers/channel-mutate-drawer', () => ({
  ChannelMutateDrawer: (props: {
    currentRow: { id: number } | null
    title?: string
    submitLabel?: string
    onSaved?: (ids: number[]) => void
    onOpenChange: (open: boolean) => void
  }) => (
    <div role='dialog' aria-label={props.title ?? 'Edit connection form'}>
      <button
        type='button'
        onClick={() => {
          props.onOpenChange(false)
          props.onSaved?.([9])
        }}
      >
        {props.submitLabel ?? 'Save changes'}
      </button>
    </div>
  ),
}))

const originalAdapter = api.defaults.adapter
const originalMatchMedia = Object.getOwnPropertyDescriptor(window, 'matchMedia')
let teardown: (() => void) | undefined

afterEach(() => {
  teardown?.()
  teardown = undefined
  api.defaults.adapter = originalAdapter
  if (originalMatchMedia) {
    Object.defineProperty(window, 'matchMedia', originalMatchMedia)
  }
  useAuthStore.getState().auth.reset('idle')
  vi.restoreAllMocks()
})

async function mount(options: Parameters<typeof mountConnections>[0] = {}) {
  const fixture = await mountConnections(options)
  teardown = fixture.teardown
  return fixture
}

const NOW = Math.floor(Date.now() / 1000)
const subscription = { ...CONNECTION, id: 7, name: 'Subscription A', type: 57 }
const metered = { ...CONNECTION, id: 8, name: 'API connection B', type: 1 }
const local = { ...CONNECTION, id: 10, name: 'Local C', type: 4, status: 2 }
const window5h = {
  channel_id: 7,
  series_id: 'a'.repeat(64),
  name: 'Subscription A',
  metric_type: 'codex_rate_limit',
  unit: 'percent',
  status: 'success',
  window_type: 'five_hour',
  window_seconds: 18000,
  current_available: 18,
  observed_at: NOW - 30,
}
const card = (name: string) =>
  screen.getByRole('article', { name }) as HTMLElement
const tests = (requests: { path: string }[]) =>
  requests.filter((request) => request.path.startsWith('/api/channel/test/'))

test('Test on the connection page runs the existing connection test and shows its result inline', async () => {
  const fixture = await mount({ entry: '/connections/7', operate: true })
  const user = userEvent.setup()
  await screen.findByRole('heading', { level: 1, name: 'Synthetic connection' })
  // Opening a connection never contacts its provider.
  expect(tests(fixture.requests)).toEqual([])
  await user.click(screen.getByRole('button', { name: 'Test' }))
  expect(await screen.findByText('Test succeeded')).toBeVisible()
  expect(screen.getByText('Response time: 1.25 s')).toBeVisible()
  expect(tests(fixture.requests).map((request) => request.path)).toEqual([
    '/api/channel/test/7',
  ])
})

test('a failed test is shown as a failure with the provider message and code', async () => {
  await mount({
    entry: '/connections/7',
    operate: true,
    testOutcome: 'failure',
  })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Test' }))
  expect(await screen.findByText('Test failed')).toBeVisible()
  expect(
    screen.getByText('Upstream rejected the key (invalid_api_key)')
  ).toBeVisible()
})

test('without the operate grant there is no Test, no switch and no test request', async () => {
  const fixture = await mount({ items: [subscription] })
  expect(
    await screen.findByRole('link', { name: 'Subscription A' })
  ).toBeVisible()
  expect(screen.queryByRole('button', { name: 'Test' })).not.toBeInTheDocument()
  expect(screen.queryByRole('switch')).not.toBeInTheDocument()
  await act(() =>
    fixture.router.navigate({
      to: '/connections/$connectionId',
      params: { connectionId: '7' },
    })
  )
  await screen.findByRole('heading', { level: 1, name: 'Subscription A' })
  expect(screen.queryByRole('button', { name: 'Test' })).not.toBeInTheDocument()
  expect(tests(fixture.requests)).toEqual([])
})

test('Add connection saves through the editor, opens the new connection and tests it', async () => {
  const fixture = await mount({ operate: true })
  const user = userEvent.setup()
  await user.click(
    await screen.findByRole('button', { name: 'Add connection' })
  )
  const editor = await screen.findByRole('dialog', { name: 'Add connection' })
  await user.click(
    within(editor).getByRole('button', { name: 'Save and test' })
  )
  await waitFor(() =>
    expect(fixture.router.state.location.pathname).toBe('/connections/9')
  )
  // The result of the test started by "Save and test" is shown on the new
  // connection's page.
  expect(await screen.findByText('Test succeeded')).toBeVisible()
  expect(tests(fixture.requests).map((request) => request.path)).toEqual([
    '/api/channel/test/9',
  ])
})

test('the page tab is part of the URL; the default tab stays out of it', async () => {
  const fixture = await mount({ entry: '/connections/7?tab=routing' })
  const user = userEvent.setup()
  expect(
    await screen.findByRole('tab', { name: 'Routing and models' })
  ).toHaveAttribute('aria-selected', 'true')
  expect(screen.getByText('Priority')).toBeVisible()
  await user.click(screen.getByRole('tab', { name: 'History' }))
  expect(fixture.router.state.location.search).toEqual({ tab: 'history' })
  await user.click(screen.getByRole('tab', { name: 'Windows' }))
  expect(fixture.router.state.location.search).toEqual({})
})

test.each([
  [
    '/connections?connection=7&tab=routing&filter=Synthetic&page=2&status=enabled',
    {
      tab: 'routing',
      connectionReturn: { filter: 'Synthetic', page: 2, status: 'enabled' },
    },
  ],
  ['/connections?connection=7&tab=windows', {}],
  ['/connections?connection=7&tab=overview', {}],
])(
  'an earlier inspector link %s opens the connection page in place',
  async (entry, expected) => {
    const fixture = await mount({ entry })
    await screen.findByRole('heading', {
      level: 1,
      name: 'Synthetic connection',
    })
    expect(fixture.router.state.location.pathname).toBe('/connections/7')
    expect(fixture.router.state.location.search).toEqual(expected)
    // Replaced, not pushed: Back does not return to the old link.
    expect(fixture.router.history.length).toBe(1)
  }
)

test.each(['/connections/0', '/connections/abc', '/connections/1.5'])(
  'an invalid connection address %s goes back to the grid',
  async (entry) => {
    const fixture = await mount({ entry })
    await waitFor(() =>
      expect(fixture.router.state.location.pathname).toBe('/connections')
    )
  }
)

test('the grid shows one card per connection with its kind, state and the saved window ring with its number', async () => {
  await mount({
    items: [subscription, metered, local],
    windows: [window5h],
    logs: [connectionLog({ id: 1, channel: 8 })],
  })
  const grid = await screen.findByTestId('connection-grid')
  expect(within(grid).getAllByRole('article')).toHaveLength(3)
  // The saved windows arrive after the inventory.
  const ring = await within(card('Subscription A')).findByRole('img', {
    name: /82% used, 18% left/,
  })
  // The ring always writes its number; the remaining share is beside it.
  expect(ring).toHaveTextContent('82%')
  expect(within(card('Subscription A')).getByText(/18% left/)).toBeVisible()
  expect(within(card('Subscription A')).getByText('Subscription')).toBeVisible()
  expect(within(card('Subscription A')).getByText('Available')).toBeVisible()
  // A metered connection draws no ring and says why.
  expect(within(card('API connection B')).queryByRole('img')).toBeNull()
  expect(
    within(card('API connection B')).getByText(
      'Metered, no subscription window'
    )
  ).toBeVisible()
  expect(within(card('API connection B')).getByText('API Key')).toBeVisible()
  // Carrying the latest requests: in use.
  expect(within(card('API connection B')).getByText('In use')).toBeVisible()
  expect(within(card('Local C')).getByText('Local')).toBeVisible()
  expect(within(card('Local C')).getByText('Disabled')).toBeVisible()
  expect(
    screen.getByText(
      'Opening this page does not send requests to any upstream; only Test contacts the provider.'
    )
  ).toBeVisible()
})

test('an unreadable window sample shows its state instead of a number', async () => {
  await mount({
    items: [subscription],
    windows: [{ ...window5h, status: 'error', current_available: 45 }],
  })
  await screen.findByTestId('connection-grid')
  expect(
    await within(card('Subscription A')).findByText(/Usage sample failed/)
  ).toBeVisible()
  expect(within(card('Subscription A')).queryByRole('img')).toBeNull()
  expect(within(card('Subscription A')).queryByText(/55%/)).toBeNull()
})

test('the switch and Test on a card act in place and never open the page', async () => {
  const fixture = await mount({ operate: true, items: [subscription] })
  const user = userEvent.setup()
  await screen.findByTestId('connection-grid')
  const target = card('Subscription A')
  await user.click(
    within(target).getByRole('switch', { name: 'Enable Subscription A' })
  )
  await waitFor(() =>
    expect(
      fixture.requests.some(
        (request) =>
          request.method === 'post' && request.path === '/api/channel/7/status'
      )
    ).toBe(true)
  )
  await waitFor(() =>
    expect(
      within(card('Subscription A')).getByRole('switch', {
        name: 'Enable Subscription A',
      })
    ).toHaveAttribute('aria-checked', 'false')
  )
  await user.click(
    within(card('Subscription A')).getByRole('button', { name: 'Test' })
  )
  expect(
    await within(card('Subscription A')).findByText('Test succeeded')
  ).toBeVisible()
  expect(fixture.router.state.location.pathname).toBe('/connections')
})

test('a click anywhere on a card body opens the page with the list to return to', async () => {
  const fixture = await mount({
    entry: '/connections?filter=Sub',
    items: [subscription],
  })
  const user = userEvent.setup()
  await screen.findByTestId('connection-grid')
  await user.click(await screen.findByRole('link', { name: 'Subscription A' }))
  await waitFor(() =>
    expect(fixture.router.state.location.pathname).toBe('/connections/7')
  )
  expect(fixture.router.state.location.search).toEqual({
    connectionReturn: { filter: 'Sub' },
  })
})

test('the connection page shows exact 24-hour figures and links the failures with result=failed', async () => {
  await mount({
    entry: '/connections/7',
    windows: [window5h],
    logs: [
      connectionLog({ id: 1 }),
      connectionLog({
        id: 2,
        type: 5,
        other: JSON.stringify({ request_outcome: 'failed' }),
      }),
      connectionLog({
        id: 3,
        type: 5,
        other: JSON.stringify({
          request_outcome: 'rejected',
          preflight_rejected: true,
        }),
      }),
    ],
  })
  const requests = await screen.findByText('Requests in 24 hours')
  await waitFor(() => expect(requests.closest('div')).toHaveTextContent('3'))
  const failedLink = await screen.findByRole('link', { name: '1' })
  expect(failedLink.getAttribute('href')).toMatch(
    /result=%22failed%22|result=failed/
  )
  expect(screen.getByText('5 hours left').closest('div')).toHaveTextContent(
    '18%'
  )
})

test('figures that cannot be read are unknown, never zero', async () => {
  await mount({ entry: '/connections/7', windowsFail: true, failLogs: true })
  await screen.findByRole('heading', { level: 1, name: 'Synthetic connection' })
  for (const label of [
    'Requests in 24 hours',
    'Failed in 24 hours',
    'Account window',
  ]) {
    await waitFor(() =>
      expect(screen.getByText(label).closest('div')).toHaveTextContent(
        'Unknown'
      )
    )
    expect(screen.getByText(label).closest('div')).not.toHaveTextContent(
      /\b0\b/
    )
  }
})
