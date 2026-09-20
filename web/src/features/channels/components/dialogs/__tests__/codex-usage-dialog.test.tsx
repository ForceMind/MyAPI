/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { getChannelQuotaHistory, getCodexQuotaSeries } from '../../../api'
import { CodexUsageDialog, CodexUsageHistoryPanel } from '../codex-usage-dialog'

vi.mock('../../../api', () => ({
  getCodexQuotaSeries: vi.fn(),
  getChannelQuotaHistory: vi.fn(),
  getCodexResetCredits: vi.fn(),
  resetCodexUsage: vi.fn(),
}))
const primary = {
  channel_id: 12,
  name: 'Codex',
  metric_type: 'codex_rate_limit',
  source: 'codex_wham_usage_primary',
  window_type: 'five_hour',
  plan_type: 'team',
  unit: 'percent',
  window_seconds: 18000,
  observed_at: 200,
  status: 'success',
}
const secondary = {
  ...primary,
  source: 'codex_wham_usage_secondary',
  window_type: 'weekly',
  window_seconds: 604800,
  observed_at: 100,
  status: 'success',
}

function renderHistory() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <CodexUsageHistoryPanel channelId={12} open />
    </QueryClientProvider>
  )
}

function renderUsage(data: Record<string, unknown>) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const content = (nextData: Record<string, unknown>) => (
    <QueryClientProvider client={queryClient}>
      <CodexUsageDialog
        open
        onOpenChange={vi.fn()}
        channelId={12}
        channelName='Codex'
        response={{ success: true, upstream_status: 200, data: nextData }}
      />
    </QueryClientProvider>
  )
  const view = render(content(data))
  return {
    ...view,
    rerenderUsage: (nextData: Record<string, unknown>) =>
      view.rerender(content(nextData)),
    rerenderUsageFailure: (message: string) =>
      view.rerender(
        <QueryClientProvider client={queryClient}>
          <CodexUsageDialog
            open
            onOpenChange={vi.fn()}
            channelId={12}
            channelName='Codex'
            response={{ success: false, message }}
          />
        </QueryClientProvider>
      ),
  }
}

describe('Codex usage history uses the unified detailed chart', () => {
  beforeEach(() => {
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      x: 0,
      y: 0,
      left: 0,
      top: 0,
      right: 800,
      bottom: 288,
      width: 800,
      height: 288,
      toJSON: () => ({}),
    })
    vi.mocked(getCodexQuotaSeries).mockResolvedValue({
      success: true,
      data: { items: [primary, secondary] },
    })
    vi.mocked(getChannelQuotaHistory).mockResolvedValue({
      success: true,
      data: {
        channel_id: 12,
        start: 100,
        end: 220,
        limit: 5000,
        unit: 'percent',
        points: [
          { timestamp: 100, status: 'success', used: 1 },
          {
            timestamp: 160,
            status: 'success',
            used: 3,
            consumption: 2,
            rate_per_minute: 2,
          },
          {
            timestamp: 220,
            status: 'success',
            used: 4,
            consumption: 1,
            rate_per_minute: 1,
          },
        ],
      },
    })
  })
  afterEach(() => {
    vi.restoreAllMocks()
    vi.clearAllMocks()
  })

  test('retains time and metric controls when no observations exist', async () => {
    vi.mocked(getCodexQuotaSeries).mockResolvedValue({
      success: true,
      data: { items: [] },
    })
    vi.mocked(getChannelQuotaHistory).mockResolvedValue({
      success: true,
      data: { channel_id: 12, start: 100, end: 220, limit: 5000, points: [] },
    })
    renderHistory()
    expect(
      await screen.findByText('No quota history data yet')
    ).toBeInTheDocument()
    expect(screen.getByLabelText('Time range')).toBeEnabled()
    expect(screen.getByLabelText('Chart style')).toBeEnabled()
  })

  test('defaults to the newest active series instead of an unavailable newer series', async () => {
    vi.mocked(getCodexQuotaSeries).mockResolvedValue({
      success: true,
      data: {
        items: [
          { ...secondary, observed_at: 300, status: 'unavailable' },
          { ...primary, observed_at: 200, status: 'success' },
        ],
      },
    })

    renderHistory()
    await screen.findByTestId('quota-history-chart-bar')
    await waitFor(() =>
      expect(getChannelQuotaHistory).toHaveBeenCalledWith(
        12,
        expect.objectContaining({
          source: primary.source,
          window_type: primary.window_type,
        })
      )
    )
  })

  test('reports series endpoint failure without presenting it as zero consumption', async () => {
    vi.mocked(getCodexQuotaSeries).mockRejectedValue(
      new Error('history endpoint unavailable')
    )
    vi.mocked(getChannelQuotaHistory).mockResolvedValue({
      success: true,
      data: { channel_id: 12, start: 100, end: 220, limit: 5000, points: [] },
    })
    renderHistory()
    expect(
      await screen.findByText('Unable to load Codex usage history')
    ).toBeInTheDocument()
    expect(
      screen.queryByTestId('quota-history-chart-bar')
    ).not.toBeInTheDocument()
  })

  test('renders real consumption bars and changes to rates and user-selected area style', async () => {
    const { container } = renderHistory()
    await waitFor(() =>
      expect(
        container.querySelector('.recharts-bar-rectangle path')
      ).toHaveAttribute('d')
    )
    expect(getChannelQuotaHistory).toHaveBeenCalledWith(
      12,
      expect.objectContaining({
        metric_type: primary.metric_type,
        source: primary.source,
        window_type: primary.window_type,
        unit: 'percent',
        window_seconds: 18000,
        plan_type: 'team',
      })
    )
    fireEvent.change(screen.getByLabelText('Metric'), {
      target: { value: 'rate_per_minute' },
    })
    await waitFor(() =>
      expect(container.querySelector('.recharts-line-curve')).toHaveAttribute(
        'd'
      )
    )
    fireEvent.change(screen.getByLabelText('Chart style'), {
      target: { value: 'area' },
    })
    await waitFor(() =>
      expect(container.querySelector('.recharts-area-area')).toHaveAttribute(
        'd'
      )
    )
    fireEvent.change(screen.getByLabelText('Metric'), {
      target: { value: 'consumption' },
    })
    expect(screen.getByLabelText('Chart style')).toHaveValue('area')
  })

  test('passes window, granularity, custom dates and refresh through to the history endpoint', async () => {
    renderHistory()
    await screen.findByTestId('quota-history-chart-bar')
    expect(
      screen.getByText(
        'Historical series keep their recorded plan and window. Changing plans does not rewrite history.'
      )
    ).toBeInTheDocument()
    expect(
      screen.getByRole('option', {
        name: 'Historical window: 5-hour window · 5h 0m · Historical plan: team',
      })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('option', {
        name: 'Historical window: Weekly window · 168h 0m · Historical plan: team',
      })
    ).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Codex quota window'), {
      target: {
        value:
          '12|codex_rate_limit|codex_wham_usage_secondary|weekly|team|percent||604800',
      },
    })
    await waitFor(() =>
      expect(getChannelQuotaHistory).toHaveBeenLastCalledWith(
        12,
        expect.objectContaining({
          source: secondary.source,
          window_type: 'weekly',
        })
      )
    )
    fireEvent.change(screen.getByLabelText('Chart granularity'), {
      target: { value: 'minute' },
    })
    await waitFor(() =>
      expect(getChannelQuotaHistory).toHaveBeenLastCalledWith(
        12,
        expect.objectContaining({ granularity: 'minute' })
      )
    )
    fireEvent.change(screen.getByLabelText('Time range'), {
      target: { value: 'custom' },
    })
    fireEvent.change(screen.getByLabelText('Custom range start'), {
      target: { value: '2026-09-01T08:00' },
    })
    fireEvent.change(screen.getByLabelText('Custom range end'), {
      target: { value: '2026-09-01T10:00' },
    })
    fireEvent.click(screen.getByText('Apply Filters'))
    await waitFor(() =>
      expect(getChannelQuotaHistory).toHaveBeenLastCalledWith(
        12,
        expect.objectContaining({
          range: 'custom',
          start: new Date('2026-09-01T08:00').toISOString(),
          end: new Date('2026-09-01T10:00').toISOString(),
        })
      )
    )
    await waitFor(() => expect(screen.getByLabelText('Refresh')).toBeEnabled())
    const previousCalls = vi.mocked(getChannelQuotaHistory).mock.calls.length
    fireEvent.click(screen.getByLabelText('Refresh'))
    await waitFor(() =>
      expect(getChannelQuotaHistory).toHaveBeenCalledTimes(previousCalls + 1)
    )
  })
})

describe('Codex current usage window classification', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.clearAllMocks()
  })

  test('uses exact provider durations and removes the old five-hour window after a single-weekly plan response', () => {
    const view = renderUsage({
      plan_type: 'free',
      rate_limit: {
        primary_window: { used_percent: 25, limit_window_seconds: 18000 },
        secondary_window: {
          used_percent: 60,
          limit_window_seconds: 604800,
        },
      },
    })
    expect(screen.getByText('5-Hour Window')).toBeInTheDocument()
    expect(screen.getByText('Weekly Window')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Current windows come only from the latest upstream response. Their durations are reported by that response.'
      )
    ).toBeInTheDocument()

    view.rerenderUsage({
      plan_type: 'plus',
      rate_limit: {
        primary_window: {
          used_percent: 30,
          limit_window_seconds: 604800,
        },
      },
    })
    expect(screen.queryByText('5-Hour Window')).not.toBeInTheDocument()
    expect(screen.getByText('Weekly Window')).toBeInTheDocument()

    view.rerenderUsageFailure('Latest Codex usage refresh failed')
    expect(screen.queryByText('Weekly Window')).not.toBeInTheDocument()
    expect(screen.queryByText('5-Hour Window')).not.toBeInTheDocument()
    expect(
      screen.getByText('Latest Codex usage refresh failed')
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('tab', { name: 'History trend' }))
    expect(screen.getByTestId('codex-usage-history-panel')).toBeInTheDocument()
  })

  test('does not let free plan metadata guess an unknown short window as weekly or five-hour', () => {
    renderUsage({
      plan_type: 'free',
      rate_limit: {
        primary_window: { used_percent: 10, limit_window_seconds: 7200 },
      },
    })
    expect(screen.getByText('Primary window')).toBeInTheDocument()
    expect(screen.queryByText('5-Hour Window')).not.toBeInTheDocument()
    expect(screen.queryByText('Weekly Window')).not.toBeInTheDocument()
  })

  test('renders an exact one-day provider window as daily', () => {
    renderUsage({
      plan_type: 'team',
      rate_limit: {
        secondary_window: { used_percent: 15, limit_window_seconds: 86400 },
      },
    })
    expect(screen.getByText('Daily window')).toBeInTheDocument()
    expect(screen.queryByText('5-Hour Window')).not.toBeInTheDocument()
    expect(screen.queryByText('Weekly Window')).not.toBeInTheDocument()
  })

  test('renders a single weekly account without inventing a missing five-hour card', () => {
    renderUsage({
      plan_type: 'free',
      rate_limit: {
        primary_window: { used_percent: 20, limit_window_seconds: 604800 },
      },
    })
    expect(screen.getByText('Weekly Window')).toBeInTheDocument()
    expect(screen.queryByText('5-Hour Window')).not.toBeInTheDocument()
  })
})
