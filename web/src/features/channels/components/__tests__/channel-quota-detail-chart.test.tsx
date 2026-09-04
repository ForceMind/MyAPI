/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { getChannelQuotaHistory } from '../../api'
import type { ChannelQuotaAnalysis } from '../../types'
import { ChannelQuotaDetailChart } from '../channel-quota-detail-chart'

vi.mock('../../api', () => ({ getChannelQuotaHistory: vi.fn() }))

function analysis(): ChannelQuotaAnalysis {
  const method = (
    rate: number,
    outcome: 'depletes_before_reset' | 'reset_before_depletion'
  ) => ({
    rate_per_minute: rate,
    rate_per_hour: rate * 60,
    coverage: 0.75,
    observed_seconds: 2700,
    interval_count: 45,
    observed_at: 200,
    eta:
      outcome === 'depletes_before_reset'
        ? {
            outcome,
            estimated_depletion_at: 300,
            seconds_to_depletion: 100,
          }
        : { outcome, reset_at: 400, seconds_until_reset: 200 },
  })
  return {
    default_method: 'observed_window',
    rate_window_seconds: 3600,
    window_start: 100,
    window_end: 200,
    as_of: 200,
    ewma_half_life_seconds: 1800,
    complete: true,
    methods: {
      latest_interval: method(3, 'depletes_before_reset'),
      observed_window: method(1, 'reset_before_depletion'),
      ewma: method(2, 'depletes_before_reset'),
    },
  }
}

function renderDetail() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ChannelQuotaDetailChart
        item={{
          channel_id: 12,
          name: 'Codex',
          metric_type: 'codex_rate_limit',
          source: 'codex_wham_usage_primary',
          window_type: 'five_hour',
          unit: 'percent',
        }}
        range='24h'
        onRangeChange={vi.fn()}
      />
    </QueryClientProvider>
  )
}

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
  vi.mocked(getChannelQuotaHistory).mockResolvedValue({
    success: true,
    data: {
      channel_id: 12,
      start: 100,
      end: 200,
      limit: 5000,
      unit: 'percent',
      points: [
        { timestamp: 100, status: 'success', available: 90 },
        { timestamp: 200, status: 'success', available: 80 },
      ],
      analysis: analysis(),
    },
  })
})

afterEach(() => {
  vi.restoreAllMocks()
})

test('keeps method and chart style local while query parameters refetch independently', async () => {
  renderDetail()

  await waitFor(() => expect(getChannelQuotaHistory).toHaveBeenCalledTimes(1))
  expect(getChannelQuotaHistory).toHaveBeenLastCalledWith(
    12,
    expect.objectContaining({
      range: '24h',
      granularity: 'auto',
      rate_window: 3600,
      ewma_half_life: 1800,
    })
  )

  fireEvent.change(screen.getByLabelText('Consumption rate'), {
    target: { value: 'ewma' },
  })
  expect(screen.getByText('2.00 percentage points/min')).toBeInTheDocument()
  fireEvent.change(screen.getByLabelText('Chart style'), {
    target: { value: 'area' },
  })
  await Promise.resolve()
  expect(getChannelQuotaHistory).toHaveBeenCalledTimes(1)

  fireEvent.change(screen.getByLabelText('Analysis window'), {
    target: { value: '21600' },
  })
  await waitFor(() => expect(getChannelQuotaHistory).toHaveBeenCalledTimes(2))
  expect(getChannelQuotaHistory).toHaveBeenLastCalledWith(
    12,
    expect.objectContaining({ rate_window: 21600 })
  )

  fireEvent.change(screen.getByLabelText('EWMA half-life'), {
    target: { value: '3600' },
  })
  await waitFor(() => expect(getChannelQuotaHistory).toHaveBeenCalledTimes(3))
  expect(getChannelQuotaHistory).toHaveBeenLastCalledWith(
    12,
    expect.objectContaining({ ewma_half_life: 3600 })
  )

  fireEvent.change(screen.getByLabelText('Chart granularity'), {
    target: { value: 'minute' },
  })
  await waitFor(() => expect(getChannelQuotaHistory).toHaveBeenCalledTimes(4))
  expect(getChannelQuotaHistory).toHaveBeenLastCalledWith(
    12,
    expect.objectContaining({
      granularity: 'minute',
      rate_window: 21600,
      ewma_half_life: 3600,
    })
  )
})
