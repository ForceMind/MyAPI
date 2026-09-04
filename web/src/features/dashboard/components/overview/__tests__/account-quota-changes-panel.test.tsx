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

const analysis = {
  default_method: 'observed_window' as const,
  rate_window_seconds: 3600,
  window_start: 100,
  window_end: 250,
  as_of: 250,
  ewma_half_life_seconds: 1800,
  complete: true,
  methods: {
    latest_interval: {
      rate_per_minute: 3,
      rate_per_hour: 180,
      coverage: 0.25,
      observed_seconds: 900,
      interval_count: 1,
      observed_at: 200,
      eta: {
        outcome: 'reset_before_depletion' as const,
        reset_at: 400,
        seconds_until_reset: 200,
      },
    },
    observed_window: {
      rate_per_minute: 1.5,
      rate_per_hour: 90,
      coverage: 0.75,
      observed_seconds: 2700,
      interval_count: 3,
      observed_at: 200,
      eta: {
        outcome: 'reset_before_depletion' as const,
        reset_at: 400,
        seconds_until_reset: 200,
      },
    },
    ewma: {
      rate_per_minute: 2,
      rate_per_hour: 120,
      coverage: 0.75,
      observed_seconds: 2700,
      interval_count: 3,
      observed_at: 200,
      eta: {
        outcome: 'depletes_before_reset' as const,
        estimated_depletion_at: 300,
      },
    },
  },
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
    expect(screen.getByTestId('quota-overview-card')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'View details' })
    ).toHaveAttribute('href', '/channels')
  })

  test('renders one compact analysis card without advanced controls or duplicate requests', async () => {
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
            previous_available: 80,
            current_available: 72,
            analysis,
            overview_points: [
              { timestamp: 100, available: 90, continuity_break: false },
              { timestamp: 120, available: 80, continuity_break: false },
              { timestamp: 140, available: null, continuity_break: true },
              { timestamp: 160, available: 75, continuity_break: true },
              { timestamp: 180, available: 72, continuity_break: false },
            ],
          },
        ],
      },
    })

    renderPanel()

    expect(
      await screen.findByTestId('codex-account-quota-chart')
    ).toBeInTheDocument()
    const sparkline = screen.getByTestId('quota-overview-sparkline')
    expect(sparkline).toBeInTheDocument()
    expect(
      screen.getByRole('img', {
        name: /Remaining quota trend from 90\.0% to 72\.0%, .+ to .+, across 2 continuous segments/,
      })
    ).toBeInTheDocument()
    expect(
      within(sparkline).getAllByTestId('quota-overview-sparkline-segment')
    ).toHaveLength(2)
    const renderedPointCount = [
      ...sparkline.querySelectorAll('polyline'),
    ].reduce(
      (count, line) =>
        count + (line.getAttribute('points')?.split(' ').length ?? 0),
      0
    )
    expect(renderedPointCount).toBe(4)
    expect(screen.getByText('72.0%')).toBeInTheDocument()
    expect(screen.getByText('3.00 percentage points/min')).toBeInTheDocument()
    expect(screen.getByText('1.50 percentage points/min')).toBeInTheDocument()
    expect(screen.getByText('90.00 percentage points/hour')).toBeInTheDocument()
    expect(screen.getByText('75%')).toBeInTheDocument()
    expect(screen.getByText('Reset before depletion')).toBeInTheDocument()
    expect(
      screen.getByText('Estimated time from analysis point: 2.5 minutes')
    ).toBeInTheDocument()
    expect(screen.queryByLabelText('Time range')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Chart granularity')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Metric')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Chart style')).not.toBeInTheDocument()
    expect(getChannelQuotaChanges).toHaveBeenCalledWith(
      expect.objectContaining({
        range: '24h',
        rate_window: 3600,
        overview_points: 48,
        limit: 4,
        sort: 'observed_desc',
      })
    )
    expect(getCodexQuotaSeries).not.toHaveBeenCalled()
    expect(getChannelQuotaHistory).not.toHaveBeenCalled()
  })

  test('describes one sparkline segment using its first and last valid points', async () => {
    vi.mocked(getChannelQuotaChanges).mockResolvedValueOnce({
      success: true,
      data: {
        items: [
          {
            channel_id: 13,
            name: 'Interrupted account',
            unit: 'percent',
            status: 'success',
            current_available: 80,
            overview_points: [
              { timestamp: 0, available: null, continuity_break: true },
              { timestamp: 3_600, available: 90, continuity_break: false },
              { timestamp: 7_200, available: 80, continuity_break: false },
              { timestamp: 10_800, available: null, continuity_break: true },
            ],
          },
        ],
      },
    })

    renderPanel()

    const sparkline = await screen.findByRole('img', {
      name: /Remaining quota trend from 90\.0% to 80\.0%, .+ across 1 continuous segment$/,
    })
    const description = sparkline.getAttribute('aria-label') ?? ''
    const timestampFormat = new Intl.DateTimeFormat('en', {
      month: 'numeric',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    })
    expect(description).toContain(timestampFormat.format(3_600_000))
    expect(description).toContain(timestampFormat.format(7_200_000))
    expect(description).not.toContain(timestampFormat.format(10_800_000))
    expect(description).not.toContain('segments')
  })

  test('does not present a past overview prediction as current remaining time', async () => {
    vi.mocked(getChannelQuotaChanges).mockResolvedValueOnce({
      success: true,
      data: {
        items: [
          {
            channel_id: 12,
            name: 'Past prediction account',
            unit: 'percent',
            current_available: 0,
            analysis: {
              ...analysis,
              methods: {
                ...analysis.methods,
                observed_window: {
                  ...analysis.methods.observed_window,
                  eta: {
                    outcome: 'depletes_before_reset',
                    estimated_depletion_at: 200,
                    seconds_to_depletion: 100,
                  },
                },
              },
            },
          },
        ],
      },
    })

    renderPanel()

    expect(
      await screen.findByText('Prediction time has passed')
    ).toBeInTheDocument()
    expect(screen.queryByText(/Time remaining:/)).not.toBeInTheDocument()
    expect(
      screen.queryByText(/Estimated time from analysis point:/)
    ).not.toBeInTheDocument()
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
        'No account quota changes recorded yet. Enable quota sampling or query a provider account to start history.'
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

    expect(await screen.findByText('team')).toBeInTheDocument()
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

  test('keeps the overview bounded without a second Codex discovery request', async () => {
    vi.mocked(getChannelQuotaChanges).mockResolvedValue({
      success: true,
      data: {
        items_complete: false,
        source_complete: true,
        items: Array.from({ length: 50 }, (_, index) => ({
          channel_id: index + 1,
          name: `Other provider ${index}`,
          metric_type: 'balance',
        })),
      },
    })
    renderPanel()
    expect(
      await screen.findByTestId('codex-account-quota-chart')
    ).toBeInTheDocument()
    expect(screen.getAllByTestId('quota-overview-card')).toHaveLength(4)
    expect(
      screen.queryByText('Incomplete quota history')
    ).not.toBeInTheDocument()
    expect(getCodexQuotaSeries).not.toHaveBeenCalled()
    expect(getChannelQuotaHistory).not.toHaveBeenCalled()
  })

  test('retries the fixed compact query after a failure', async () => {
    vi.mocked(getChannelQuotaChanges)
      .mockRejectedValueOnce(new Error('quota unavailable'))
      .mockResolvedValueOnce({ success: true, data: { items: [] } })
    renderPanel()
    expect(
      await screen.findByText('Unable to load account quota changes')
    ).toBeInTheDocument()
    fireEvent.click(screen.getByLabelText('Refresh'))
    await waitFor(() =>
      expect(
        screen.queryByText('Unable to load account quota changes')
      ).not.toBeInTheDocument()
    )
    expect(getChannelQuotaChanges).toHaveBeenLastCalledWith(
      expect.objectContaining({ range: '24h', rate_window: 3600, limit: 4 })
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
