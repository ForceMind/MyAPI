/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { getChannelQuotaChanges, getChannelQuotaSamplingStatus } from '@/features/channels/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { AccountQuotaChangesPanel } from '../account-quota-changes-panel'

vi.mock('@/features/channels/api', () => ({
  getChannelQuotaChanges: vi.fn(),
  getChannelQuotaSamplingStatus: vi.fn(),
}))

vi.mock('@tanstack/react-router', () => ({
  Link: (props: { children?: ReactNode; to?: string; [key: string]: unknown }) => (
    <a href={props.to} {...props}>{props.children}</a>
  ),
}))

function renderPanel() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <AccountQuotaChangesPanel />
    </QueryClientProvider>
  )
}

function setUser(canReadChannels: boolean) {
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'admin',
    role: ROLE.ADMIN,
    permissions: {
      admin_permissions: {
        channel: { read: canReadChannels },
      },
    },
  })
}

describe('account quota changes dashboard panel', () => {
  beforeEach(() => setUser(true))

  afterEach(() => {
    useAuthStore.getState().auth.setUser(null)
  })

  test('does not request or expose upstream quota to users without channel read permission', () => {
    setUser(false)

    renderPanel()

    expect(screen.queryByText('Account quota changes')).not.toBeInTheDocument()
    expect(getChannelQuotaChanges).not.toHaveBeenCalled()
    expect(getChannelQuotaSamplingStatus).not.toHaveBeenCalled()
  })

  test('keeps movement details and channel navigation visible on narrow layouts', async () => {
    vi.mocked(getChannelQuotaChanges).mockResolvedValueOnce({
      success: true,
      data: {
        items: [{
          channel_id: 12,
          name: 'Codex production channel',
          account_label: 'A deliberately long provider account label',
          change_per_minute: -1234.56,
          abs_change_per_minute: 1234.56,
          current_available: 9876.54,
          direction: 'decrease',
          unit: 'USD',
        }],
      },
    })
    vi.mocked(getChannelQuotaSamplingStatus).mockResolvedValueOnce({
      success: true,
      data: { enabled: true, interval_seconds: 300, max_channels: 20 },
    })

    renderPanel()

    expect(await screen.findByText('A deliberately long provider account label')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Channels' })).toHaveAttribute('href', '/channels')
    expect(screen.getByRole('list', { name: 'Account quota changes' })).toBeInTheDocument()
  })

  test('renders a useful empty state after a successful response', async () => {
    vi.mocked(getChannelQuotaChanges).mockResolvedValueOnce({
      success: true,
      data: { items: [] },
    })
    vi.mocked(getChannelQuotaSamplingStatus).mockResolvedValueOnce({
      success: true,
      data: { enabled: true, interval_seconds: 300, max_channels: 20 },
    })

    renderPanel()

    expect(
      await screen.findByText('No account quota changes recorded yet. Background sampling is enabled and will populate this panel after the next interval.')
    ).toBeInTheDocument()
  })

  test('labels unsupported provider quota instead of showing stable movement', async () => {
    vi.mocked(getChannelQuotaChanges).mockResolvedValueOnce({
      success: true,
      data: {
        items: [{
          channel_id: 18,
          name: 'Claude subscription',
          account_label: 'Anthropic account',
          status: 'unsupported',
          direction: 'stable',
          change_per_minute: null,
          abs_change_per_minute: null,
          current_available: null,
        }],
      },
    })
    vi.mocked(getChannelQuotaSamplingStatus).mockResolvedValueOnce({
      success: true,
      data: { enabled: true, interval_seconds: 900, max_channels: 20 },
    })

    renderPanel()

    expect(await screen.findByText('Unsupported')).toBeInTheDocument()
    expect(screen.queryByText('Stable')).not.toBeInTheDocument()
  })

  test('does not mix units in the summary movement cards', async () => {
    vi.mocked(getChannelQuotaChanges).mockResolvedValueOnce({
      success: true,
      data: {
        items: [
          {
            channel_id: 21,
            name: 'Dollar account',
            change_per_minute: 5,
            abs_change_per_minute: 5,
            current_available: 90,
            direction: 'increase',
            unit: 'USD',
            metric_type: 'balance',
            window_type: 'none',
          },
          {
            channel_id: 22,
            name: 'Percent account',
            change_per_minute: 100,
            abs_change_per_minute: 100,
            current_available: 80,
            direction: 'increase',
            unit: 'percent',
            metric_type: 'codex_rate_limit',
            window_type: 'five_hour',
          },
        ],
      },
    })
    vi.mocked(getChannelQuotaSamplingStatus).mockResolvedValueOnce({
      success: true,
      data: { enabled: true, interval_seconds: 900, max_channels: 20 },
    })

    renderPanel()

    const increaseCard = (await screen.findByText('Max increase / minute')).parentElement
    expect(increaseCard).toBeTruthy()
    expect(within(increaseCard as HTMLElement).getByText('5 USD')).toBeInTheDocument()
  })

  test('keeps loading state visible while requests are pending', () => {
    vi.mocked(getChannelQuotaChanges).mockImplementationOnce(
      () => new Promise(() => undefined)
    )
    vi.mocked(getChannelQuotaSamplingStatus).mockImplementationOnce(
      () => new Promise(() => undefined)
    )

    renderPanel()

    expect(document.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThan(0)
    expect(screen.getByText('Account quota changes')).toBeInTheDocument()
  })

  test('renders a retry action for an expired session', async () => {
    vi.mocked(getChannelQuotaChanges).mockRejectedValueOnce({ response: { status: 401 } })
    vi.mocked(getChannelQuotaSamplingStatus).mockResolvedValueOnce({
      success: true,
      data: { enabled: false, interval_seconds: 300, max_channels: 20 },
    })

    renderPanel()

    expect(await screen.findByText('Your session is missing or expired. Sign in again to load provider account quota.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Sign in again' })).toHaveAttribute('href', '/sign-in')
  })
})
