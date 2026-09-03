/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import {
  getChannelQuotaHistory,
  getChannelQuotaChanges,
  getChannelQuotaSamplingStatus,
  getCodexQuotaSeries,
} from '@/features/channels/api'
import { getSelf } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { AccountQuotaChangesPanel } from '../account-quota-changes-panel'

vi.mock('@/features/channels/api', () => ({
  getChannelQuotaHistory: vi.fn(),
  getChannelQuotaChanges: vi.fn(),
  getChannelQuotaSamplingStatus: vi.fn(),
  getCodexQuotaSeries: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  getSelf: vi.fn(),
}))

vi.mock('@tanstack/react-router', () => ({
  Link: (props: {
    children?: ReactNode
    to?: string
    [key: string]: unknown
  }) => (
    <a href={props.to} {...props}>
      {props.children}
    </a>
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

function setUser(canReadChannels: boolean, id = 1) {
  useAuthStore.getState().auth.setUser({
    id,
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
  beforeEach(() => {
    vi.mocked(getSelf).mockResolvedValue({ success: false })
    vi.mocked(getCodexQuotaSeries).mockResolvedValue({
      success: true,
      data: { items: [] },
    })
    vi.mocked(getChannelQuotaHistory).mockResolvedValue({
      success: true,
      data: {
        channel_id: 12,
        start: 100,
        end: 200,
        limit: 5000,
        unit: 'percent',
        points: [],
      },
    })
    setUser(true)
  })

  afterEach(() => {
    useAuthStore.getState().auth.reset('idle')
    vi.clearAllMocks()
  })

  test('explains missing permission without requesting upstream quota', () => {
    setUser(false)

    renderPanel()

    expect(screen.getByText('Quota consumption')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Administrator permission required to view account quota changes'
      )
    ).toBeInTheDocument()
    expect(getChannelQuotaChanges).not.toHaveBeenCalled()
    expect(getChannelQuotaSamplingStatus).not.toHaveBeenCalled()
  })

  test('does not reuse quota data across login identities', async () => {
    vi.mocked(getChannelQuotaChanges)
      .mockResolvedValueOnce({
        success: true,
        data: {
          items: [
            {
              channel_id: 41,
              name: 'First session account',
              status: 'success',
              direction: 'stable',
              current_available: 90,
            },
          ],
        },
      })
      .mockResolvedValueOnce({
        success: true,
        data: {
          items: [
            {
              channel_id: 42,
              name: 'Second session account',
              status: 'success',
              direction: 'stable',
              current_available: 80,
            },
          ],
        },
      })
    vi.mocked(getChannelQuotaSamplingStatus)
      .mockResolvedValueOnce({
        success: true,
        data: { enabled: true, interval_seconds: 300, max_channels: 20 },
      })
      .mockResolvedValueOnce({
        success: true,
        data: { enabled: true, interval_seconds: 300, max_channels: 20 },
      })

    renderPanel()
    expect(await screen.findByText('First session account')).toBeInTheDocument()

    setUser(true, 2)

    expect(
      await screen.findByText('Second session account')
    ).toBeInTheDocument()
    expect(screen.queryByText('First session account')).not.toBeInTheDocument()
    expect(getChannelQuotaChanges).toHaveBeenCalledTimes(2)
  })

  test('refreshes an administrator capability matrix once per session', async () => {
    const auth = useAuthStore.getState().auth
    const initialUser = {
      id: 7,
      username: 'admin',
      role: ROLE.ADMIN,
      permissions: { admin_permissions: { channel: { read: false } } },
    }
    auth.setBundle({
      access_token: 'test-token',
      token_type: 'Bearer',
      access_expires_at: Math.floor(Date.now() / 1000) + 600,
      user: initialUser,
      session: {
        sid: 'session-capability-refresh',
        current: true,
        login_method: 'password',
        ip: '127.0.0.1',
        user_agent: 'test',
        created_at: 1,
        last_active_at: 1,
        expires_at: 1000,
      },
    })
    vi.mocked(getSelf).mockResolvedValueOnce({
      success: true,
      data: {
        ...initialUser,
        permissions: { admin_permissions: { channel: { read: true } } },
      },
    })
    vi.mocked(getChannelQuotaChanges).mockResolvedValueOnce({
      success: true,
      data: { items: [] },
    })
    vi.mocked(getChannelQuotaSamplingStatus).mockResolvedValueOnce({
      success: true,
      data: { enabled: false, interval_seconds: 900, max_channels: 20 },
    })

    renderPanel()

    await waitFor(() => expect(getSelf).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(getChannelQuotaChanges).toHaveBeenCalledTimes(1))
    expect(
      useAuthStore.getState().auth.user?.permissions?.admin_permissions
    ).toEqual({
      channel: { read: true },
    })
  })

  test('keeps movement details and channel navigation visible on narrow layouts', async () => {
    vi.mocked(getChannelQuotaChanges).mockResolvedValueOnce({
      success: true,
      data: {
        items: [
          {
            channel_id: 12,
            name: 'Codex production channel',
            account_label: 'A deliberately long provider account label',
            change_per_minute: -1234.56,
            abs_change_per_minute: 1234.56,
            current_available: 9876.54,
            direction: 'decrease',
            unit: 'USD',
          },
        ],
      },
    })
    vi.mocked(getChannelQuotaSamplingStatus).mockResolvedValueOnce({
      success: true,
      data: { enabled: true, interval_seconds: 300, max_channels: 20 },
    })

    renderPanel()

    expect(
      await screen.findByText('A deliberately long provider account label')
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Channels' })).toHaveAttribute(
      'href',
      '/channels'
    )
    expect(
      screen.getByRole('list', { name: 'Account quota changes' })
    ).toBeInTheDocument()
  })

  test('renders Codex consumption bars and switches to available quota on the overview', async () => {
    vi.mocked(getCodexQuotaSeries).mockResolvedValueOnce({
      success: true,
      data: {
        items: [
          {
            channel_id: 12,
            name: 'Codex production channel',
            account_label: 'Codex team account',
            metric_type: 'codex_rate_limit',
            source: 'codex_wham_usage_primary',
            window_type: 'five_hour',
            plan_type: 'team',
            unit: 'percent',
          },
        ],
      },
    })
    vi.mocked(getChannelQuotaChanges).mockResolvedValueOnce({
      success: true,
      data: {
        items: [
          {
            channel_id: 12,
            name: 'Codex production channel',
            account_label: 'Codex team account',
            metric_type: 'codex_rate_limit',
            source: 'codex_wham_usage_primary',
            window_type: 'five_hour',
            plan_type: 'team',
            unit: 'percent',
            status: 'success',
            direction: 'decrease',
            current_available: 72,
          },
        ],
      },
    })
    vi.mocked(getChannelQuotaSamplingStatus).mockResolvedValueOnce({
      success: true,
      data: { enabled: true, interval_seconds: 300, max_channels: 20 },
    })
    vi.mocked(getChannelQuotaHistory).mockResolvedValueOnce({
      success: true,
      data: {
        channel_id: 12,
        start: 100,
        end: 200,
        limit: 500,
        unit: 'percent',
        points: [
          { timestamp: 100, status: 'success', available: 90 },
          {
            timestamp: 200,
            status: 'success',
            available: 72,
            consumption: 18,
            rate_per_minute: 10.8,
          },
        ],
      },
    })

    renderPanel()

    expect(
      await screen.findByTestId('codex-account-quota-chart')
    ).toBeInTheDocument()
    expect(
      await screen.findByTestId('quota-history-chart-bar')
    ).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Metric'), {
      target: { value: 'available' },
    })
    expect(screen.getByText('Last plotted value: 72.0%')).toBeInTheDocument()
    expect(getChannelQuotaHistory).toHaveBeenCalledWith(
      12,
      expect.objectContaining({
        metric_type: 'codex_rate_limit',
        source: 'codex_wham_usage_primary',
        window_type: 'five_hour',
        plan_type: 'team',
      })
    )
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
      await screen.findByText(
        'No account quota changes recorded yet. Background sampling is enabled and will populate this panel after the next interval.'
      )
    ).toBeInTheDocument()
    expect(
      screen.queryByTestId('quota-history-chart-bar')
    ).not.toBeInTheDocument()
  })

  test('labels unsupported provider quota instead of showing stable movement', async () => {
    vi.mocked(getChannelQuotaChanges).mockResolvedValueOnce({
      success: true,
      data: {
        items: [
          {
            channel_id: 18,
            name: 'Claude subscription',
            account_label: 'Anthropic account',
            status: 'unsupported',
            direction: 'stable',
            change_per_minute: null,
            abs_change_per_minute: null,
            current_available: null,
          },
        ],
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

  test('shows provider plan type and keeps sampling errors visible', async () => {
    vi.mocked(getChannelQuotaChanges).mockResolvedValueOnce({
      success: true,
      data: {
        items: [
          {
            channel_id: 31,
            name: 'Codex team account',
            account_label: 'Codex account',
            plan_type: 'team',
            status: 'success',
            direction: 'unknown',
            current_available: 80,
          },
          {
            channel_id: 32,
            name: 'Unavailable account',
            status: 'error',
            direction: 'unknown',
            current_available: null,
          },
        ],
      },
    })
    vi.mocked(getChannelQuotaSamplingStatus).mockResolvedValueOnce({
      success: true,
      data: { enabled: true, interval_seconds: 900, max_channels: 20 },
    })

    renderPanel()

    expect(await screen.findByText('· team')).toBeInTheDocument()
    expect(screen.getByText('Error')).toBeInTheDocument()
    expect(screen.getByText('Unavailable account')).toBeInTheDocument()
  })

  test('keeps currencies in separate account rows without a mixed-unit maximum card', async () => {
    vi.mocked(getCodexQuotaSeries).mockResolvedValueOnce({
      success: true,
      data: {
        items: [
          {
            channel_id: 22,
            name: 'Percent account',
            metric_type: 'codex_rate_limit',
            unit: 'percent',
            window_type: 'five_hour',
          },
        ],
      },
    })
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

    expect(await screen.findByText('Dollar account')).toBeInTheDocument()
    expect(screen.getByText('Percent account')).toBeInTheDocument()
    expect(screen.queryByText('Max increase / minute')).not.toBeInTheDocument()
    expect(
      within(screen.getByTestId('codex-account-quota-chart')).queryByText(/USD/)
    ).not.toBeInTheDocument()
  })

  test('keeps provider sources separate instead of adding their latest movements', async () => {
    vi.mocked(getChannelQuotaChanges).mockResolvedValueOnce({
      success: true,
      data: {
        items: [
          {
            channel_id: 31,
            name: 'Codex account',
            change_per_minute: 7,
            abs_change_per_minute: 7,
            current_available: 70,
            direction: 'increase',
            unit: 'USD',
            metric_type: 'balance',
            window_type: 'none',
            source: 'codex',
          },
          {
            channel_id: 31,
            name: 'Claude account',
            change_per_minute: 12,
            abs_change_per_minute: 12,
            current_available: 60,
            direction: 'increase',
            unit: 'USD',
            metric_type: 'balance',
            window_type: 'none',
            source: 'claude',
          },
        ],
      },
    })
    vi.mocked(getChannelQuotaSamplingStatus).mockResolvedValueOnce({
      success: true,
      data: { enabled: true, interval_seconds: 900, max_channels: 20 },
    })

    renderPanel()

    expect(await screen.findByText('Codex account')).toBeInTheDocument()
    expect(screen.getByText('Claude account')).toBeInTheDocument()
    expect(screen.queryByText('Max increase / minute')).not.toBeInTheDocument()
  })

  test('keeps loading state visible while requests are pending', () => {
    vi.mocked(getChannelQuotaChanges).mockImplementationOnce(
      () => new Promise(() => undefined)
    )
    vi.mocked(getChannelQuotaSamplingStatus).mockImplementationOnce(
      () => new Promise(() => undefined)
    )
    vi.mocked(getCodexQuotaSeries).mockImplementationOnce(
      () => new Promise(() => undefined)
    )

    renderPanel()

    expect(
      document.querySelectorAll('[data-slot="skeleton"]').length
    ).toBeGreaterThan(0)
    expect(screen.getByText('Quota consumption')).toBeInTheDocument()
  })

  test('discovers Codex independently when fifty other quota series occupy the movement response', async () => {
    vi.mocked(getChannelQuotaChanges).mockResolvedValue({
      success: true,
      data: {
        items: Array.from({ length: 50 }, (_, index) => ({
          channel_id: index + 1,
          name: `Other provider ${index}`,
          metric_type: 'balance',
        })),
      },
    })
    vi.mocked(getCodexQuotaSeries).mockResolvedValue({
      success: true,
      data: {
        items: [
          {
            channel_id: 99,
            name: 'Separate Codex',
            metric_type: 'codex_rate_limit',
            source: 'codex_wham_usage_primary',
            window_type: 'weekly',
          },
        ],
      },
    })
    vi.mocked(getChannelQuotaSamplingStatus).mockResolvedValue({
      success: true,
      data: { enabled: true, interval_seconds: 60, max_channels: 100 },
    })
    renderPanel()
    expect(
      await screen.findByTestId('codex-account-quota-chart')
    ).toBeInTheDocument()
    await waitFor(() =>
      expect(getChannelQuotaHistory).toHaveBeenCalledWith(
        99,
        expect.objectContaining({
          metric_type: 'codex_rate_limit',
          source: 'codex_wham_usage_primary',
        })
      )
    )
    expect(getCodexQuotaSeries).toHaveBeenCalledWith(
      expect.objectContaining({ range: '24h', limit: 2000 })
    )
  })

  test('keeps range and custom controls available after a failed quota query so the user can recover', async () => {
    vi.mocked(getChannelQuotaChanges).mockImplementation(async (params) => {
      if (params?.range === '1h') return { success: true, data: { items: [] } }
      throw new Error('range too large')
    })
    vi.mocked(getCodexQuotaSeries).mockResolvedValue({
      success: true,
      data: { items: [] },
    })
    vi.mocked(getChannelQuotaSamplingStatus).mockResolvedValue({
      success: true,
      data: { enabled: true, interval_seconds: 60, max_channels: 100 },
    })
    renderPanel()
    expect(
      await screen.findByText('Unable to load account quota changes')
    ).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Time range'), {
      target: { value: 'custom' },
    })
    expect(screen.getByLabelText('Custom range start')).toBeEnabled()
    fireEvent.change(screen.getByLabelText('Time range'), {
      target: { value: '1h' },
    })
    await waitFor(() =>
      expect(
        screen.queryByText('Unable to load account quota changes')
      ).not.toBeInTheDocument()
    )
    expect(getChannelQuotaChanges).toHaveBeenLastCalledWith(
      expect.objectContaining({ range: '1h' })
    )
  })

  test('renders a retry action for an expired session', async () => {
    vi.mocked(getChannelQuotaChanges).mockRejectedValueOnce({
      response: { status: 401 },
    })
    vi.mocked(getChannelQuotaSamplingStatus).mockResolvedValueOnce({
      success: true,
      data: { enabled: false, interval_seconds: 300, max_channels: 20 },
    })

    renderPanel()

    expect(
      await screen.findByText(
        'Your session is missing or expired. Sign in again to load provider account quota.'
      )
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Sign in again' })
    ).toHaveAttribute('href', '/sign-in')
  })
})
