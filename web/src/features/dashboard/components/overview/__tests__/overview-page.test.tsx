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

vi.mock('@tanstack/react-router', () => ({
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
  }) => (
    <a
      {...attributes}
      href={`${to.replace('$section', params?.section ?? '')}${
        search
          ? `?${new URLSearchParams(
              Object.entries(search)
                .filter(([, value]) => value !== undefined)
                .map(([key, value]) => [
                  key,
                  Array.isArray(value) ? value.join(',') : String(value),
                ])
            )}`
          : ''
      }`}
    >
      {children}
    </a>
  ),
}))
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
let channelTotals: { all: number; enabled: number }
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
  requests = []
  logRows = [row({})]
  logFailures = 0
  channelTotals = { all: 2, enabled: 1 }
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
    const url = config.url ?? ''
    const path = url.split('?')[0]
    const query = new URLSearchParams(url.split('?')[1] ?? '')
    let data: unknown = { success: true, data: {} }
    if (path === '/api/user/self') {
      data = { success: true, data: useAuthStore.getState().auth.user }
    } else if (path === '/api/log' || path === '/api/log/self') {
      if (logFailures > 0) {
        logFailures--
        data = { success: false, message: 'private upstream detail' }
      } else if (query.get('page_size') === '1') {
        const type = Number(query.get('type'))
        data = {
          success: true,
          data: {
            items: [],
            total: logRows.filter((item) => item.type === type).length,
          },
        }
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
          items: [],
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

describe('overview for an ordinary user', () => {
  test('reads only its own call records, never channels, and shows no provider column or upstream actions', async () => {
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
    expect(
      screen.queryByRole('region', { name: 'Most used account windows' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Add connection' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Pending requests' })
    ).not.toBeInTheDocument()
    expect(screen.queryByText('Enabled connections')).not.toBeInTheDocument()
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
    const attention = screen.getByRole('region', { name: 'Needs attention' })
    expect(
      within(attention).getByText('1 requests completed without a usage report')
    ).toBeVisible()
  })

  test('counts Keys from the safe list and never reads Key objects', async () => {
    setUser(ROLE.USER)
    keyItems = []
    renderOverview()
    const next = await screen.findByRole('region', { name: 'Next steps' })
    expect(
      within(next).getByRole('link', {
        name: /Create an API Key for your client/,
      })
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
    await userEvent
      .setup()
      .click(within(recent).getByRole('button', { name: 'Retry' }))
    expect(await within(recent).findByText('Desktop Key')).toBeVisible()
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

describe('overview for a channel administrator', () => {
  test('shows enabled connections, the most used window and links failures to filtered call records', async () => {
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
    expect(await screen.findByText('1 / 2')).toBeVisible()
    expect(await screen.findByText('50%')).toBeVisible()
    const attention = screen.getByRole('region', { name: 'Needs attention' })
    expect(
      await within(attention).findByText('1 connections are disabled')
    ).toBeVisible()
    expect(
      within(attention).getByRole('link', {
        name: /1 connections are disabled/,
      })
    ).toHaveAttribute('href', '/connections?status=disabled')
    const windows = screen.getByRole('region', {
      name: 'Most used account windows',
    })
    const first = (await within(windows).findAllByRole('listitem'))[0]
    expect(within(first).getByText(/Account A/)).toBeVisible()
    expect(within(first).getByText('82% used')).toBeVisible()
    const recent = screen.getByRole('region', { name: 'Recent requests' })
    const failed = within(recent).getByRole('link', { name: /Failed 1/ })
    expect(failed.getAttribute('href')).toMatch(
      /^\/usage-logs\/common\?type=5&startTime=\d+&endTime=\d+$/
    )
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

  test('on a phone only the most used window is shown', async () => {
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
    setUser(ROLE.ADMIN, { read: true, write: true })
    renderOverview()
    const windows = await screen.findByRole('region', {
      name: 'Most used account windows',
    })
    const items = await within(windows).findAllByRole('listitem')
    expect(items).toHaveLength(1)
    expect(within(items[0]).getByText(/Account A/)).toBeVisible()
  })

  test('a malformed connection total is unavailable, never a fabricated zero', async () => {
    setUser(ROLE.ADMIN, { read: true })
    channelTotals = { all: -1, enabled: 0 }
    renderOverview()
    const health = await screen.findByRole('region', { name: 'Health' })
    await waitFor(() =>
      expect(
        within(health).getAllByText('Not provided').length
      ).toBeGreaterThan(0)
    )
    expect(within(health).queryByText('0 / 0')).not.toBeInTheDocument()
  })

  test('without channel read permission no channel or window is requested', async () => {
    setUser(ROLE.ADMIN, { read: false })
    renderOverview()
    await screen.findByRole('region', { name: 'Recent requests' })
    await waitFor(() => expect(paths()).toContain('/api/user/self'))
    expect(paths().some((path) => path.startsWith('/api/channel'))).toBe(false)
    expect(screen.queryByText('Enabled connections')).not.toBeInTheDocument()
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
    logRows = [row({ token_name: 'Admin-only Key' })]
    renderOverview()
    expect(await screen.findByText('Admin-only Key')).toBeVisible()
    logRows = [row({ token_name: 'Second user Key', user_id: 2 })]
    act(() => setUser(ROLE.USER, {}, 2))
    expect(await screen.findByText('Second user Key')).toBeVisible()
    expect(screen.queryByText('Admin-only Key')).not.toBeInTheDocument()
    expect(requests.at(-1)?.url).not.toBe(undefined)
  })
})
