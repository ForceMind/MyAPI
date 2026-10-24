/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AxiosAdapter } from 'axios'
import { useState, type AnchorHTMLAttributes } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'
import { useInspectorStore } from '@/stores/inspector-store'

import type { ConnectionsSearch } from '../connection-state'
import {
  ConnectionsWorkspace,
  type ConnectionsNavigation,
} from '../connections'

vi.mock('@tanstack/react-router', () => ({
  getRouteApi: () => ({}),
  Link: ({
    to,
    children,
    ...attributes
  }: AnchorHTMLAttributes<HTMLAnchorElement> & { to: string }) => (
    <a {...attributes} href={to}>
      {children}
    </a>
  ),
}))
vi.mock('@/hooks/use-sidebar-config', () => ({
  useIsSidebarModuleVisible: () => true,
}))
vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
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
  useInspectorStore.setState({ current: null })
  api.defaults.adapter = originalAdapter
  if (originalMatchMedia) {
    Object.defineProperty(window, 'matchMedia', originalMatchMedia)
  }
  useAuthStore.getState().auth.reset('idle')
  vi.restoreAllMocks()
})

const connection = (id: number) => ({
  id,
  type: 1,
  name: `Synthetic connection ${id}`,
  status: 1,
  models: 'alpha',
  group: 'default',
  priority: 0,
  weight: 0,
  test_time: 0,
  response_time: 0,
})

function mount(options: {
  initial?: ConnectionsSearch
  operate?: boolean
  testOutcome?: 'success' | 'failure'
}) {
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    value: vi.fn((query: string) => ({
      matches: /min-width/.test(query),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  })
  const grants = {
    read: true,
    write: true,
    ...(options.operate ? { operate: true } : {}),
  }
  const user = {
    id: 7,
    username: 'synthetic-operator',
    role: ROLE.ADMIN,
    permissions: { admin_permissions: { channel: grants } },
  }
  useAuthStore.getState().auth.setBundle({
    access_token: 'synthetic-upstream-access',
    token_type: 'Bearer',
    access_expires_at: 4_000_000_000,
    user,
    session: {
      sid: 'upstream-session',
      current: true,
      login_method: 'password',
      ip: '127.0.0.1',
      user_agent: 'test',
      created_at: 1,
      last_active_at: 1,
      expires_at: 4_000_000_000,
    },
  })
  const requests: string[] = []
  const adapter: AxiosAdapter = async (config) => {
    const path = config.url ?? ''
    requests.push(path)
    let data: unknown
    if (path === '/api/user/self') data = { success: true, data: user }
    else if (path === '/api/channel') {
      data = { success: true, data: { total: 1, items: [connection(7)] } }
    } else if (/^\/api\/channel\/\d+$/.test(path)) {
      data = { success: true, data: connection(Number(path.split('/').at(-1))) }
    } else if (path.startsWith('/api/channel/test/')) {
      data =
        options.testOutcome === 'failure'
          ? {
              success: false,
              message: 'Upstream rejected the key',
              error_code: 'invalid_api_key',
            }
          : { success: true, time: 1.25 }
    } else if (path === '/api/channel/quota/changes') {
      data = { success: true, data: { items: [] } }
    } else throw new Error(`Unexpected request: ${path}`)
    return { config, status: 200, statusText: 'OK', headers: {}, data }
  }
  api.defaults.adapter = adapter
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const navigations: {
    next: ConnectionsSearch
    options?: ConnectionsNavigation
  }[] = []
  function Harness() {
    const [search, setSearch] = useState<ConnectionsSearch>(
      options.initial ?? {}
    )
    return (
      <ConnectionsWorkspace
        search={search}
        onSearch={(next, navigation) => {
          navigations.push({ next, options: navigation })
          setSearch(next)
        }}
      />
    )
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
  return { requests, navigations }
}

test('Test in the inspector runs the existing connection test and shows its result inline', async () => {
  const fixture = mount({ initial: { connection: 7 }, operate: true })
  const user = userEvent.setup()
  const detail = within(await screen.findByTestId('connection-detail'))
  await detail.findByRole('heading', { name: 'Synthetic connection 7' })
  // Opening a connection never contacts its provider.
  expect(fixture.requests.some((path) => path.includes('/test/'))).toBe(false)
  await user.click(detail.getByRole('button', { name: 'Test' }))
  expect(await detail.findByText('Test succeeded')).toBeVisible()
  expect(detail.getByText('Response time: 1.25 s')).toBeVisible()
  expect(fixture.requests).toContain('/api/channel/test/7')
})

test('a failed test is shown as a failure with the provider message and code', async () => {
  mount({ initial: { connection: 7 }, operate: true, testOutcome: 'failure' })
  const user = userEvent.setup()
  const detail = within(await screen.findByTestId('connection-detail'))
  await user.click(await detail.findByRole('button', { name: 'Test' }))
  expect(await detail.findByText('Test failed')).toBeVisible()
  expect(
    detail.getByText('Upstream rejected the key (invalid_api_key)')
  ).toBeVisible()
})

test('without the operate grant there is no Test action and no test request', async () => {
  const fixture = mount({ initial: { connection: 7 } })
  const detail = within(await screen.findByTestId('connection-detail'))
  await detail.findByRole('heading', { name: 'Synthetic connection 7' })
  expect(detail.queryByRole('button', { name: 'Test' })).not.toBeInTheDocument()
  expect(fixture.requests.some((path) => path.includes('/test/'))).toBe(false)
})

test('Add connection saves through the editor, opens the new connection and tests it', async () => {
  const fixture = mount({ operate: true })
  const user = userEvent.setup()
  await user.click(
    await screen.findByRole('button', { name: 'Add connection' })
  )
  const editor = await screen.findByRole('dialog', { name: 'Add connection' })
  await user.click(
    within(editor).getByRole('button', { name: 'Save and test' })
  )
  await waitFor(() =>
    expect(fixture.navigations.at(-1)).toEqual({
      next: { connection: 9, tab: undefined },
      options: { fromList: true },
    })
  )
  const detail = within(await screen.findByTestId('connection-detail'))
  expect(await detail.findByText('Test succeeded')).toBeVisible()
  expect(fixture.requests).toContain('/api/channel/test/9')
})

test('the inspector tab is part of the URL state', async () => {
  const fixture = mount({ initial: { connection: 7, tab: 'routing' } })
  const user = userEvent.setup()
  const detail = within(await screen.findByTestId('connection-detail'))
  expect(await detail.findByRole('tab', { name: 'Routing' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
  expect(detail.getByText('Priority')).toBeVisible()
  await user.click(detail.getByRole('tab', { name: 'Account windows' }))
  expect(fixture.navigations.at(-1)).toEqual({
    next: { connection: 7, tab: 'windows' },
    options: { replace: true },
  })
})
