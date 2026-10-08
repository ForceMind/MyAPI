/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { getChannels } from '@/features/channels/api'
import { getApiKeys } from '@/features/keys/api'
import { getUserModels } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { SELF_USE_MINIMAL } from '@/lib/self-use-build'
import { useAuthStore } from '@/stores/auth-store'

import { OverviewDashboard } from '../overview-dashboard'

vi.mock('@/features/channels/api', () => ({
  getChannels: vi.fn(),
}))

vi.mock('@/features/keys/api', () => ({
  fetchTokenKey: vi.fn(),
  getApiKeys: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  getUserModels: vi.fn(),
}))

vi.mock('@/features/dashboard/hooks/use-status-data', () => ({
  useApiInfo: () => ({ items: [], loading: false }),
  useDashboardContentVisibility: () => ({
    apiInfo: false,
    announcements: false,
    faq: false,
    uptimeKuma: false,
  }),
}))

vi.mock('@/components/page-transition', () => ({
  CardStaggerContainer: (props: {
    children?: ReactNode
    className?: string
  }) => <div className={props.className}>{props.children}</div>,
  CardStaggerItem: (props: { children?: ReactNode; className?: string }) => (
    <div className={props.className}>{props.children}</div>
  ),
}))

vi.mock(
  '@/features/dashboard/components/overview/account-quota-changes-panel',
  () => ({
    AccountQuotaChangesPanel: () => (
      <section aria-label='Account quota changes' />
    ),
  })
)
vi.mock('@/features/dashboard/components/overview/announcements-panel', () => ({
  AnnouncementsPanel: () => null,
}))
vi.mock('@/features/dashboard/components/overview/api-info-panel', () => ({
  ApiInfoPanel: () => null,
}))
vi.mock('@/features/dashboard/components/overview/faq-panel', () => ({
  FAQPanel: () => null,
}))
vi.mock(
  '@/features/dashboard/components/overview/performance-health-panel',
  () => ({
    PerformanceHealthPanel: () => null,
  })
)
vi.mock('@/features/dashboard/components/overview/summary-cards', () => ({
  SummaryCards: () => <section aria-label='Usage summary' />,
}))
vi.mock(
  '@/features/dashboard/components/overview/recent-activity-panel',
  () => ({
    RecentActivityPanel: () => <section aria-label='Recent activity' />,
  })
)
vi.mock('@/features/dashboard/components/overview/uptime-panel', () => ({
  UptimePanel: () => null,
}))

vi.mock('@tanstack/react-router', () => ({
  Link: (props: { children?: ReactNode; to?: string }) => (
    <a href={props.to}>{props.children}</a>
  ),
}))

vi.mock('motion/react', () => ({
  motion: { div: 'div' },
  useReducedMotion: () => true,
}))

function renderDashboard(status: Record<string, unknown> = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(['status'], status)
  return render(
    <QueryClientProvider client={client}>
      <OverviewDashboard />
    </QueryClientProvider>
  )
}

function setUser(role: number, canReadChannels = true) {
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'setup-guide-test',
    role,
    permissions: {
      admin_permissions: {
        channel: { read: canReadChannels },
      },
    },
  })
}

describe('overview setup guide edition and role gating', () => {
  beforeEach(() => {
    vi.mocked(getApiKeys).mockResolvedValue({
      success: true,
      data: { items: [] },
    } as never)
    vi.mocked(getUserModels).mockResolvedValue({
      success: true,
      data: ['gpt-4o-mini'],
    } as never)
    vi.mocked(getChannels).mockResolvedValue({
      success: true,
      data: { total: 1, items: [] },
    } as never)
  })

  afterEach(() => {
    vi.clearAllMocks()
    useAuthStore.getState().auth.setUser(null)
  })

  test('shows the channel setup step only to a permitted full-build administrator', async () => {
    setUser(ROLE.ADMIN, true)
    renderDashboard()

    if (SELF_USE_MINIMAL) {
      expect(
        screen.queryByText('Configure upstream channels')
      ).not.toBeInTheDocument()
      expect(
        screen.queryByRole('link', { name: 'Channels' })
      ).not.toBeInTheDocument()
      expect(getChannels).not.toHaveBeenCalled()
      return
    }

    expect(
      await screen.findByText('Configure upstream channels')
    ).toBeInTheDocument()
    await waitFor(() => {
      expect(getChannels).toHaveBeenCalledWith({ p: 1, page_size: 1 })
    })
  })

  test('does not request or show the channel step for regular users or denied admins', async () => {
    setUser(ROLE.USER, true)
    const { unmount } = renderDashboard()

    await waitFor(() => expect(getApiKeys).toHaveBeenCalled())
    expect(
      screen.queryByText('Configure upstream channels')
    ).not.toBeInTheDocument()
    expect(getChannels).not.toHaveBeenCalled()

    unmount()
    vi.clearAllMocks()
    setUser(ROLE.ADMIN, false)
    renderDashboard()

    await waitFor(() => expect(getApiKeys).toHaveBeenCalled())
    expect(
      screen.queryByText('Configure upstream channels')
    ).not.toBeInTheDocument()
    expect(getChannels).not.toHaveBeenCalled()
  })

  test.each([
    ['administrator', ROLE.ADMIN],
    ['regular user', ROLE.USER],
  ])(
    '%s sees operational content before setup and no sales prompts',
    async (_, role) => {
      setUser(role)
      renderDashboard()

      const recent = screen.getByRole('region', { name: 'Recent activity' })
      const summary = screen.getByRole('region', { name: 'Usage summary' })
      const guide = await screen.findByText('Get started')

      expect(
        recent.compareDocumentPosition(guide) & Node.DOCUMENT_POSITION_FOLLOWING
      ).toBeTruthy()
      expect(
        summary.compareDocumentPosition(guide) &
          Node.DOCUMENT_POSITION_FOLLOWING
      ).toBeTruthy()
      expect(screen.queryByText('Add credits')).not.toBeInTheDocument()
      expect(
        screen.queryByRole('link', { name: 'Pricing' })
      ).not.toBeInTheDocument()
    }
  )
})

test('root operator can reach existing pending review from the overview before the usage panels', async () => {
  setUser(ROLE.SUPER_ADMIN)
  renderDashboard()
  const console = screen.getByRole('navigation', { name: 'Console' })
  expect(
    within(console).getByRole('button', { name: 'Pending requests' })
  ).toBeInTheDocument()
  expect(
    within(console).getByRole('link', { name: 'API Keys' })
  ).toHaveAttribute('href', '/keys')
  expect(
    console.compareDocumentPosition(
      screen.getByRole('region', { name: 'Usage summary' })
    ) & Node.DOCUMENT_POSITION_FOLLOWING
  ).toBeTruthy()
})

test('ordinary users do not receive root recovery actions or an empty provider column', async () => {
  setUser(ROLE.USER)
  renderDashboard()
  const console = screen.getByRole('navigation', { name: 'Console' })
  expect(
    within(console).queryByRole('button', { name: 'Pending requests' })
  ).not.toBeInTheDocument()
  expect(
    within(console).queryByRole('link', { name: 'Channels' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('region', { name: 'Account quota changes' })
  ).not.toBeInTheDocument()
})

test('overview actions honor both site module exclusions and user narrowing', async () => {
  setUser(ROLE.SUPER_ADMIN)
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'root',
    role: ROLE.SUPER_ADMIN,
    sidebar_modules: JSON.stringify({ console: { log: false } }),
  })
  renderDashboard({
    SidebarModulesAdmin: JSON.stringify({
      console: { token: false },
      admin: { channel: false },
    }),
  })
  const console = screen.getByRole('navigation', { name: 'Console' })
  for (const name of ['API Keys', 'Channels', 'Usage Logs']) {
    expect(
      within(console).queryByRole('link', { name })
    ).not.toBeInTheDocument()
  }
  expect(
    within(console).queryByRole('button', { name: 'Pending requests' })
  ).not.toBeInTheDocument()
})
