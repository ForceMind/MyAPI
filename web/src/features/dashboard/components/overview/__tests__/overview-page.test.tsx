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
import type { AxiosAdapter, InternalAxiosRequestConfig } from 'axios'
import type { AnchorHTMLAttributes } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { PRODUCT_CAPABILITIES } from '@/lib/product-edition'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { OverviewPage } from '../overview-page'

// The page reads and writes its selection through the route's search; this
// stands in for the URL so a test can start from a shared link and observe
// every navigation the page makes.
const url = vi.hoisted(() => {
  let search: Record<string, unknown> = {}
  const listeners = new Set<() => void>()
  return {
    navigations: [] as Record<string, unknown>[],
    get: () => search,
    set(next: Record<string, unknown>) {
      search = next
      listeners.forEach((listener) => listener())
    },
    subscribe(listener: () => void) {
      listeners.add(listener)
      return () => void listeners.delete(listener)
    },
  }
})

vi.mock('@tanstack/react-router', async () => {
  const { useSyncExternalStore } = await import('react')
  return {
    Link: ({
      to,
      params,
      search,
      children,
      ...attributes
    }: AnchorHTMLAttributes<HTMLAnchorElement> & {
      to: string
      params?: { section?: string }
      search?: Record<string, unknown>
    }) => {
      const query = new URLSearchParams(
        Object.entries(search ?? {})
          .filter(([, value]) => value !== undefined)
          .map(([key, value]) => [
            key,
            Array.isArray(value) ? value.join(',') : String(value),
          ])
      ).toString()
      return (
        <a
          {...attributes}
          href={`${to.replace('$section', params?.section ?? '')}${
            query ? `?${query}` : ''
          }`}
        >
          {children}
        </a>
      )
    },
    getRouteApi: () => ({
      useSearch: () => useSyncExternalStore(url.subscribe, url.get),
      useNavigate:
        () =>
        (options: {
          search:
            | Record<string, unknown>
            | ((previous: Record<string, unknown>) => Record<string, unknown>)
        }) => {
          const next =
            typeof options.search === 'function'
              ? options.search(url.get())
              : options.search
          url.navigations.push({ ...options, search: next })
          url.set(next)
          return Promise.resolve()
        },
    }),
  }
})
// These panels have their own tests; here only their presence matters.
vi.mock('../summary-cards', () => ({
  SummaryCards: () => <section aria-label='Funds summary' />,
}))
vi.mock('../performance-health-panel', () => ({
  PerformanceHealthPanel: () => <section aria-label='Performance health' />,
}))
vi.mock('../announcements-panel', () => ({
  AnnouncementsPanel: () => <section aria-label='Announcements' />,
}))
vi.mock('../api-info-panel', () => ({ ApiInfoPanel: () => null }))
vi.mock('../faq-panel', () => ({ FAQPanel: () => null }))
vi.mock('../uptime-panel', () => ({ UptimePanel: () => null }))

const NOW = Math.floor(Date.now() / 1000)
type Row = Record<string, unknown>
const row = (overrides: Row): Row => ({
  id: 1,
  user_id: 1,
  created_at: NOW - 60,
  type: 2,
  content: 'PRIVATE CONTENT SENTINEL',
  token_name: 'Desktop Key',
  token_id: 21,
  model_name: 'synthetic-model',
  prompt_tokens: 10,
  completion_tokens: 5,
  use_time: 2,
  channel: 1,
  channel_name: 'Synthetic connection',
  request_id: 'REQ-OK',
  other: JSON.stringify({
    request_outcome: 'completed',
    usage_outcome: 'reported',
  }),
  ...overrides,
})

let requests: InternalAxiosRequestConfig[]
let logRows: Row[]
let logFailures: number
let countFailures: boolean
let channelTotals: { all: number; enabled: number }
let channelItems: Row[]
let keyItems: Row[]
const originalAdapter = api.defaults.adapter
const clients: QueryClient[] = []

function setUser(role: number, grants: Record<string, boolean> = {}, id = 1) {
  useAuthStore.getState().auth.setUser({
    id,
    username: `user-${id}`,
    role,
    request_count: 3,
    permissions: { admin_permissions: { channel: grants } },
  })
}

beforeEach(() => {
  url.navigations = []
  url.set({})
  requests = []
  logRows = [row({})]
  logFailures = 0
  countFailures = false
  channelTotals = { all: 3, enabled: 2 }
  channelItems = [
    { id: 1, type: 57, name: 'Account A link', status: 1, priority: 5 },
    { id: 2, type: 57, name: 'Account B link', status: 1, priority: 3 },
    { id: 3, type: 1, name: 'Metered link', status: 2, priority: 0 },
  ]
  keyItems = [
    {
      id: 21,
      name: 'Desktop Key',
      status: 1,
      group: 'default',
      remain_quota: 0,
      unlimited_quota: true,
      expired_time: -1,
    },
  ]
  const adapter: AxiosAdapter = async (config) => {
    requests.push(config)
    const requestUrl = config.url ?? ''
    const path = requestUrl.split('?')[0]
    const query = new URLSearchParams(requestUrl.split('?')[1] ?? '')
    let data: unknown = { success: true, data: {} }
    if (path === '/api/user/self') {
      data = { success: true, data: useAuthStore.getState().auth.user }
    } else if (path === '/api/log' || path === '/api/log/self') {
      if (query.get('page_size') === '1') {
        const type = Number(query.get('type'))
        data = countFailures
          ? { success: false, message: 'private count detail' }
          : {
              success: true,
              data: {
                items: [],
                total: logRows.filter((item) => item.type === type).length,
              },
            }
      } else if (logFailures > 0) {
        logFailures--
        data = { success: false, message: 'private upstream detail' }
      } else {
        data = {
          success: true,
          data: { items: logRows, total: logRows.length },
        }
      }
    } else if (path === '/api/channel') {
      const enabled = config.params?.status === 'enabled'
      data = {
        success: true,
        data: {
          items: enabled ? [] : channelItems,
          total: enabled ? channelTotals.enabled : channelTotals.all,
        },
      }
    } else if (path === '/api/channel/quota/changes') {
      data = {
        success: true,
        data: {
          items: [
            {
              channel_id: 1,
              series_id: 'a',
              name: 'Account A',
              metric_type: 'codex_rate_limit',
              unit: 'percent',
              status: 'success',
              window_type: 'five_hour',
              window_seconds: 18000,
              current_available: 18,
              observed_at: NOW - 30,
            },
            {
              channel_id: 2,
              series_id: 'b',
              name: 'Account B',
              metric_type: 'codex_rate_limit',
              unit: 'percent',
              status: 'success',
              window_type: 'weekly',
              window_seconds: 604800,
              current_available: 60,
              observed_at: NOW - 30,
            },
          ],
        },
      }
    } else if (path === '/pg/keys') {
      data = {
        success: true,
        data: {
          items: keyItems,
          total: keyItems.length,
          page: 1,
          page_size: 100,
        },
      }
    } else if (path === '/api/full-content-logs') {
      data = { success: true, data: { enabled: true, items: [], total: 0 } }
    }
    return { config, status: 200, statusText: 'OK', headers: {}, data }
  }
  api.defaults.adapter = adapter
})

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  api.defaults.adapter = originalAdapter
  useAuthStore.getState().auth.reset('idle')
  localStorage.clear()
  vi.restoreAllMocks()
})

function renderOverview(status: Record<string, unknown> = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  client.setQueryData(['status'], status)
  render(
    <QueryClientProvider client={client}>
      <OverviewPage />
    </QueryClientProvider>
  )
  return client
}

const paths = () => requests.map((config) => (config.url ?? '').split('?')[0])
const board = () => screen.getByTestId('flow-board')
const core = () => screen.getByTestId('flow-core')
const node = (name: string) =>
  within(board())
    .getAllByRole('button')
    .find((button) => button.dataset.flowNode === name) as HTMLElement

function mockPhone() {
  vi.spyOn(window, 'matchMedia').mockImplementation(
    (query: string) =>
      ({
        matches: query === '(max-width: 1023px)',
        media: query,
        onchange: null,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
        addListener: () => undefined,
        removeListener: () => undefined,
        dispatchEvent: () => false,
      }) as MediaQueryList
  )
}

describe('overview for an ordinary user', () => {
  test('reads only its own call records, never channels, and shows only its Keys and requests', async () => {
    setUser(ROLE.USER)
    renderOverview()
    const recent = await screen.findByRole('region', {
      name: 'Recent requests',
    })
    expect(await within(recent).findByText('Desktop Key')).toBeVisible()
    expect(
      within(recent).queryByRole('columnheader', { name: 'Connection' })
    ).not.toBeInTheDocument()
    expect(screen.queryByText(/PRIVATE CONTENT/)).not.toBeInTheDocument()
    expect(paths().every((path) => !path.startsWith('/api/channel'))).toBe(true)
    expect(paths()).toContain('/api/log/self')
    expect(paths()).not.toContain('/api/log')
    // No upstream column, no account windows, no upstream actions.
    expect(
      within(board()).queryByRole('heading', { name: /Upstream/ })
    ).not.toBeInTheDocument()
    expect(within(board()).queryByRole('img')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Add connection' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Pending requests' })
    ).not.toBeInTheDocument()
    expect(await within(board()).findByText('Desktop Key')).toBeVisible()
  })

  test('a completed request without usage stays successful and invents no tokens', async () => {
    setUser(ROLE.USER)
    logRows = [
      row({
        type: 5,
        prompt_tokens: 0,
        completion_tokens: 0,
        other: JSON.stringify({
          request_outcome: 'completed',
          usage_outcome: 'missing',
        }),
      }),
    ]
    renderOverview()
    const recent = await screen.findByRole('region', {
      name: 'Recent requests',
    })
    expect(await within(recent).findByText('Request successful')).toBeVisible()
    expect(within(recent).queryByText(/0 Tokens/)).not.toBeInTheDocument()
    expect(
      within(within(recent).getByRole('table')).getByText(/Usage not provided/)
    ).toBeVisible()
    const attention = screen.getByRole('region', { name: 'Needs attention' })
    expect(
      within(attention).getByText('1 requests completed without a usage report')
    ).toBeVisible()
    expect(within(core()).getByText(/Usage not provided: 1/)).toBeVisible()
  })

  test('without Keys the board offers to create one from the safe list, never reading Key objects', async () => {
    setUser(ROLE.USER)
    keyItems = []
    logRows = []
    renderOverview()
    expect(await within(board()).findByText('No API Keys yet')).toBeVisible()
    expect(
      within(board()).getByRole('link', { name: 'Create API Key' })
    ).toHaveAttribute('href', '/keys')
    expect(paths()).toContain('/pg/keys')
    expect(paths().some((path) => path.startsWith('/api/token'))).toBe(false)
  })

  test('a failed read shows a generic retry without the server message, then the rows', async () => {
    setUser(ROLE.USER)
    logFailures = 1
    renderOverview()
    const recent = await screen.findByRole('region', {
      name: 'Recent requests',
    })
    expect(
      await within(recent).findByText('Unable to load recent requests')
    ).toBeVisible()
    expect(
      screen.queryByText(/private upstream detail/)
    ).not.toBeInTheDocument()
    // Counts that could not be read are unknown, not zero.
    expect(within(node('key-21')).getByText('Unknown')).toBeVisible()
    expect(within(core()).getAllByText('—').length).toBeGreaterThan(0)
    await userEvent
      .setup()
      .click(within(recent).getByRole('button', { name: 'Retry' }))
    expect(await within(recent).findByText('Desktop Key')).toBeVisible()
  })

  test('an unreadable 24-hour count is shown as unavailable, never 0', async () => {
    setUser(ROLE.USER)
    countFailures = true
    renderOverview()
    await waitFor(() =>
      expect(
        within(core()).getByText('Last 24 h').nextElementSibling
      ).toHaveTextContent('—')
    )
    expect(screen.queryByText(/private count detail/)).not.toBeInTheDocument()
  })

  test('an invalid timestamp degrades to an unknown time', async () => {
    setUser(ROLE.USER)
    logRows = [row({ created_at: Number.MAX_VALUE })]
    renderOverview()
    const recent = await screen.findByRole('region', {
      name: 'Recent requests',
    })
    await within(recent).findByText('Desktop Key')
    expect(within(recent).getByText('Unknown')).toBeVisible()
  })
})

describe('selecting a flow node', () => {
  beforeEach(() => {
    logRows = [
      row({ id: 3, token_id: 22, token_name: 'Laptop Key', request_id: 'R3' }),
      row({ id: 2, token_id: 21, request_id: 'R2', channel: 2 }),
      row({ id: 1, token_id: 21, request_id: 'R1' }),
    ]
    keyItems = [
      { id: 21, name: 'Desktop Key', status: 1 },
      { id: 22, name: 'Laptop Key', status: 1 },
    ]
  })

  test('counts each Key by its stable ID among the sampled requests', async () => {
    setUser(ROLE.USER)
    renderOverview()
    await waitFor(() =>
      expect(within(node('key-21')).getByText('2 requests')).toBeVisible()
    )
    expect(within(node('key-22')).getByText('1 requests')).toBeVisible()
  })

  test('selecting a Key writes it to the URL and filters the requests with a clearable chip', async () => {
    setUser(ROLE.USER)
    renderOverview()
    const recent = await screen.findByRole('region', {
      name: 'Recent requests',
    })
    await within(recent).findByText('Laptop Key')
    const user = userEvent.setup()
    await user.click(node('key-21'))
    expect(url.navigations.at(-1)).toMatchObject({
      search: { key: 21, connection: undefined },
      resetScroll: false,
    })
    expect(node('key-21')).toHaveAttribute('aria-pressed', 'true')
    expect(within(recent).queryByText('Laptop Key')).not.toBeInTheDocument()
    expect(within(recent).getAllByText('Desktop Key')).toHaveLength(2)
    expect(within(recent).getByTestId('flow-filter-chip')).toHaveTextContent(
      'Key: Desktop Key'
    )
    expect(
      within(recent).getByRole('link', { name: 'View call records' })
    ).toHaveAttribute('href', '/usage-logs/common?tokenId=21')

    await user.click(
      within(recent).getByRole('button', { name: 'Clear filter' })
    )
    expect(url.get()).toEqual({ key: undefined, connection: undefined })
    expect(node('key-21')).toHaveAttribute('aria-pressed', 'false')
    expect(within(recent).getByText('Laptop Key')).toBeVisible()
    expect(within(recent).queryByTestId('flow-filter-chip')).toBeNull()
  })

  test('selecting the same node again clears the filter', async () => {
    setUser(ROLE.USER)
    renderOverview()
    await waitFor(() => expect(node('key-21')).toBeDefined())
    const user = userEvent.setup()
    await user.click(node('key-21'))
    await user.click(node('key-21'))
    expect(url.get()).toEqual({ key: undefined, connection: undefined })
  })

  test('a shared or refreshed link restores the selected Key', async () => {
    url.set({ key: 22 })
    setUser(ROLE.USER)
    renderOverview()
    const recent = await screen.findByRole('region', {
      name: 'Recent requests',
    })
    expect(await within(recent).findByText('Laptop Key')).toBeVisible()
    expect(within(recent).queryByText('Desktop Key')).not.toBeInTheDocument()
    expect(node('key-22')).toHaveAttribute('aria-pressed', 'true')
  })

  test('an administrator filters by connection and the URL names it', async () => {
    setUser(ROLE.ADMIN, { read: true, write: true })
    url.set({ connection: 2 })
    renderOverview()
    const recent = await screen.findByRole('region', {
      name: 'Recent requests',
    })
    await waitFor(() =>
      expect(node('connection-2')).toHaveAttribute('aria-pressed', 'true')
    )
    expect(
      within(recent)
        .getAllByRole('link')
        .map((link) => link.getAttribute('href'))
    ).toEqual(
      expect.arrayContaining([
        '/usage-logs/common?requestId=R2&inspect=request',
        '/usage-logs/common?channel=2',
      ])
    )
    expect(
      within(recent).queryByRole('link', { name: /R1|R3/ })
    ).not.toBeInTheDocument()
    expect(within(recent).getByTestId('flow-filter-chip')).toHaveTextContent(
      'Upstream: Account B link'
    )
  })

  test('on a phone selecting a node brings the requests into view', async () => {
    mockPhone()
    const scroll = vi.spyOn(HTMLElement.prototype, 'scrollIntoView')
    setUser(ROLE.USER)
    renderOverview()
    await waitFor(() => expect(node('key-21')).toBeDefined())
    await userEvent.setup().click(node('key-21'))
    expect(scroll).toHaveBeenCalledTimes(1)
    expect(scroll.mock.contexts[0]).toBe(
      screen.getByRole('region', { name: 'Recent requests' })
    )
  })
})

describe('overview for a channel administrator', () => {
  test('shows connections with saved windows, failures and links them to filtered places', async () => {
    setUser(ROLE.ADMIN, { read: true, write: true })
    logRows = [
      row({
        id: 2,
        request_id: 'REQ-FAIL',
        type: 5,
        other: JSON.stringify({ request_outcome: 'failed' }),
      }),
      row({ id: 1 }),
    ]
    renderOverview()
    expect(await within(core()).findByText('50%')).toBeVisible()
    const attention = screen.getByRole('region', { name: 'Needs attention' })
    expect(
      await within(attention).findByText('1 connections are disabled')
    ).toBeVisible()
    expect(
      within(attention).getByRole('link', {
        name: /1 connections are disabled/,
      })
    ).toHaveAttribute('href', '/connections?status=disabled')
    expect(
      within(attention)
        .getByRole('link', { name: /1 of the latest requests failed/ })
        .getAttribute('href')
    ).toMatch(/^\/usage-logs\/common\?type=5&startTime=\d+&endTime=\d+$/)
    const account = node('connection-1')
    expect(
      within(account).getByRole('img', { name: /82% used, 18% left/ })
    ).toHaveTextContent('82%')
    expect(within(node('connection-3')).getByText('Metered')).toBeVisible()
    expect(within(node('connection-3')).getByText('Disabled')).toBeVisible()
    const recent = screen.getByRole('region', { name: 'Recent requests' })
    // Every row opens its request's details directly.
    const links = within(recent)
      .getAllByRole('link')
      .map((link) => link.getAttribute('href'))
    expect(links).toContain(
      '/usage-logs/common?requestId=REQ-FAIL&inspect=request'
    )
    expect(
      screen.getByRole('link', { name: 'Add connection' })
    ).toHaveAttribute('href', '/connections')
  })

  test('a malformed connection total is unavailable, never a fabricated zero', async () => {
    setUser(ROLE.ADMIN, { read: true })
    channelTotals = { all: -1, enabled: 0 }
    renderOverview()
    expect(
      await within(board()).findByText('Connections could not be loaded.')
    ).toBeVisible()
    expect(
      within(board()).getByRole('heading', { name: 'Upstream' })
    ).toBeVisible()
    expect(
      screen.queryByText(/connections are disabled/)
    ).not.toBeInTheDocument()
  })

  test('without channel read permission no channel or window is requested', async () => {
    setUser(ROLE.ADMIN, { read: false })
    renderOverview()
    await screen.findByRole('region', { name: 'Recent requests' })
    await waitFor(() => expect(paths()).toContain('/api/user/self'))
    expect(paths().some((path) => path.startsWith('/api/channel'))).toBe(false)
    expect(
      within(board()).queryByRole('heading', { name: /Upstream/ })
    ).not.toBeInTheDocument()
  })
})

describe('gateway core card by edition', () => {
  test('Lite shows the current access address and that it was opened on this computer', async () => {
    setUser(ROLE.USER)
    renderOverview({
      product_edition: 'lite',
      version: '0.2.0-beta.26',
      capabilities: Object.fromEntries(
        PRODUCT_CAPABILITIES.map((capability) => [capability, false])
      ),
    })
    const card = core()
    expect(within(card).getByText(`${window.location.origin}/v1`)).toBeVisible()
    expect(within(card).getByText('Current access address')).toBeVisible()
    expect(
      within(card).getByText(
        /Lite · 0\.2\.0-beta\.26 · Opened on this computer/
      )
    ).toBeVisible()
  })

  test('Full shows the configured server address and no local access mode', async () => {
    setUser(ROLE.USER)
    renderOverview({
      server_address: 'https://gateway.example.test/',
      version: '0.2.0-beta.26',
    })
    const card = core()
    expect(
      within(card).getByText('https://gateway.example.test/v1')
    ).toBeVisible()
    expect(within(card).getByText('Configured server address')).toBeVisible()
    expect(within(card).getByText(/Full · 0\.2\.0-beta\.26/)).toBeVisible()
    expect(
      within(card).queryByText(/Opened on this computer/)
    ).not.toBeInTheDocument()
  })

  test('a missing version is said to be missing', () => {
    setUser(ROLE.USER)
    renderOverview()
    expect(within(core()).getByText(/Version not provided/)).toBeVisible()
  })
})

describe('edition, funding and module boundaries', () => {
  const funding = (mode: 'disabled' | 'enabled' | 'retirement') => ({
    user_funding_mode: mode,
    user_funding_capabilities: { mode, epoch: 3, ready: true },
  })

  test.each([
    ['enabled', true],
    ['retirement', true],
    ['disabled', false],
  ] as const)('confirmed %s funding shows funds: %s', async (mode, shown) => {
    setUser(ROLE.USER)
    renderOverview(funding(mode))
    await screen.findByRole('region', { name: 'Recent requests' })
    expect(
      Boolean(screen.queryByRole('region', { name: 'Funds summary' }))
    ).toBe(shown)
  })

  test('a cached commercial status is not trusted before the server confirms it', async () => {
    setUser(ROLE.USER)
    localStorage.setItem('status', JSON.stringify(funding('enabled')))
    let release!: () => void
    const held = new Promise<void>((resolve) => {
      release = resolve
    })
    const adapter = api.defaults.adapter as AxiosAdapter
    api.defaults.adapter = async (config) => {
      if (config.url === '/api/status') {
        await held
        return {
          config,
          status: 200,
          statusText: 'OK',
          headers: {},
          data: { success: true, data: funding('disabled') },
        }
      }
      return adapter(config)
    }
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    clients.push(client)
    render(
      <QueryClientProvider client={client}>
        <OverviewPage />
      </QueryClientProvider>
    )
    await screen.findByRole('region', { name: 'Recent requests' })
    expect(
      screen.queryByRole('region', { name: 'Funds summary' })
    ).not.toBeInTheDocument()
    await act(async () => release())
    await waitFor(() =>
      expect(client.getQueryData(['status'])).toMatchObject(funding('disabled'))
    )
    expect(
      screen.queryByRole('region', { name: 'Funds summary' })
    ).not.toBeInTheDocument()
  })

  test('Lite hides site content and performance panels even when their switches are on', async () => {
    setUser(ROLE.ADMIN, { read: true })
    renderOverview({
      announcements_enabled: true,
      product_edition: 'lite',
      capabilities: Object.fromEntries(
        PRODUCT_CAPABILITIES.map((capability) => [capability, false])
      ),
    })
    await screen.findByRole('region', { name: 'Recent requests' })
    expect(
      screen.queryByRole('region', { name: 'Announcements' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('region', { name: 'Performance health' })
    ).not.toBeInTheDocument()
  })

  test('Full keeps them for administrators', async () => {
    setUser(ROLE.ADMIN, { read: true })
    renderOverview({ announcements_enabled: true })
    expect(
      await screen.findByRole('region', { name: 'Announcements' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('region', { name: 'Performance health' })
    ).toBeInTheDocument()
  })

  test('module exclusions remove the Key, connection and review actions', async () => {
    setUser(ROLE.SUPER_ADMIN, { read: true, write: true })
    renderOverview({
      SidebarModulesAdmin: JSON.stringify({
        console: { token: false, log: false },
        admin: { channel: false },
      }),
    })
    await screen.findByRole('region', { name: 'Recent requests' })
    expect(
      screen.queryByRole('link', { name: 'Create API Key' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Add connection' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Pending requests' })
    ).not.toBeInTheDocument()
  })

  test('a root operator reaches pending reviews from the overview', async () => {
    setUser(ROLE.SUPER_ADMIN, { read: true })
    renderOverview()
    const attention = await screen.findByRole('region', {
      name: 'Needs attention',
    })
    expect(
      within(attention).getByRole('button', { name: 'Pending requests' })
    ).toBeVisible()
  })

  test('switching accounts removes the previous administrator data', async () => {
    setUser(ROLE.ADMIN, { read: true })
    logRows = [row({ token_name: 'Admin-only Key', token_id: 77 })]
    renderOverview()
    const recent = await screen.findByRole('region', {
      name: 'Recent requests',
    })
    expect(await within(recent).findByText('Admin-only Key')).toBeVisible()
    logRows = [row({ token_name: 'Second user Key', user_id: 2 })]
    act(() => setUser(ROLE.USER, {}, 2))
    expect(await screen.findAllByText('Second user Key')).not.toHaveLength(0)
    expect(screen.queryByText('Admin-only Key')).not.toBeInTheDocument()
    expect(requests.at(-1)?.url).not.toBe(undefined)
  })
})
