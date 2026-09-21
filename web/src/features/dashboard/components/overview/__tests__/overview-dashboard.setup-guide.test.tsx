/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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
    AccountQuotaChangesPanel: () => null,
  })
)
vi.mock(
  '@/features/dashboard/components/overview/operational-attention-panel',
  () => ({
    OperationalAttentionPanel: () => null,
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
  SummaryCards: () => null,
}))
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

function renderDashboard() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
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

  test('keeps the guide collapsed until a permitted full-build administrator expands it', async () => {
    setUser(ROLE.ADMIN, true)
    const user = userEvent.setup()
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
      await screen.findByText('Setup guide is collapsed. Expand it anytime.')
    ).toBeInTheDocument()
    expect(
      screen.queryByText('Configure upstream channels')
    ).not.toBeInTheDocument()
    await waitFor(() => {
      expect(getChannels).toHaveBeenCalledWith({ p: 1, page_size: 1 })
    })
    await user.click(screen.getByRole('button', { name: 'Show setup guide' }))
    expect(
      await screen.findByText('Configure upstream channels')
    ).toBeInTheDocument()
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
})
