/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { getChannelQuotaHistory } from '../../../api'
import type { Channel, ChannelQuotaHistoryData } from '../../../types'
import { ChannelQuotaHistory } from '../channel-quota-history'

type TrendProps = {
  data?: ChannelQuotaHistoryData
  range: string
  granularity: string
  metric: string
  chartStyle: string
  rangeOptions?: readonly string[]
  granularityOptions?: readonly string[]
  rangeOptionLabel?: (value: string) => string
  error?: unknown
  errorMessage?: string
  onRangeChange?: (value: string) => void
  onGranularityChange?: (value: string) => void
  onMetricChange?: (
    value: 'available' | 'used' | 'total' | 'consumption'
  ) => void
  onChartStyleChange?: (value: 'line' | 'area' | 'bar') => void
  onRefresh?: () => void
}

vi.mock('../../../api', () => ({
  getChannelQuotaHistory: vi.fn(),
}))

vi.mock('../../quota-history-trend', () => ({
  QuotaHistoryTrend(props: TrendProps) {
    const error = props.error instanceof Error ? props.error.message : undefined

    return (
      <section data-testid='quota-history-trend'>
        <select
          aria-label='Time range'
          value={props.range}
          onChange={(event) => props.onRangeChange?.(event.target.value)}
        >
          {props.rangeOptions?.map((option) => (
            <option key={option} value={option}>
              {props.rangeOptionLabel?.(option) ?? option}
            </option>
          ))}
        </select>
        <select
          aria-label='Chart granularity'
          value={props.granularity}
          onChange={(event) => props.onGranularityChange?.(event.target.value)}
        >
          {props.granularityOptions?.map((option) => (
            <option key={option} value={option}>
              {option}
            </option>
          ))}
        </select>
        <select
          aria-label='Metric'
          value={props.metric}
          onChange={(event) =>
            props.onMetricChange?.(
              event.target.value as
                | 'available'
                | 'used'
                | 'total'
                | 'consumption'
            )
          }
        >
          <option value='available'>Available quota</option>
          <option value='used'>Used quota</option>
          <option value='total'>Total quota</option>
          <option value='consumption'>Observed consumption</option>
        </select>
        <select
          aria-label='Chart style'
          value={props.chartStyle}
          onChange={(event) =>
            props.onChartStyleChange?.(
              event.target.value as 'line' | 'area' | 'bar'
            )
          }
        >
          <option value='line'>Line</option>
          <option value='area'>Area</option>
          <option value='bar'>Bar</option>
        </select>
        <button type='button' aria-label='Refresh' onClick={props.onRefresh}>
          Refresh
        </button>
        {error || props.errorMessage ? (
          <div>
            <div>Unable to load quota history</div>
            <div>{error ?? props.errorMessage}</div>
          </div>
        ) : null}
        <output data-testid='trend-data'>
          {props.data?.points.length ?? 'no-data'}
        </output>
      </section>
    )
  },
}))

const baseChannel = {
  id: 12,
  type: 1,
  key: 'masked',
  status: 1,
  name: 'Example',
  created_time: 0,
  test_time: 0,
  response_time: 0,
  balance: 10,
  balance_updated_time: 0,
  models: 'gpt-5',
  group: 'default',
  used_quota: 0,
  channel_info: {
    is_multi_key: false,
    multi_key_size: 0,
    multi_key_polling_index: 0,
    multi_key_mode: 'random' as const,
  },
} as Channel

function emptyHistory(): ChannelQuotaHistoryData {
  return {
    channel_id: baseChannel.id,
    start: 0,
    end: 1,
    limit: 5000,
    points: [],
  }
}

function renderQuota(channel = baseChannel) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ChannelQuotaHistory channel={channel} open />
    </QueryClientProvider>
  )
}

async function waitForInitialHistoryRequest() {
  await waitFor(() => expect(getChannelQuotaHistory).toHaveBeenCalledTimes(1))
}

describe('channel quota history', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(getChannelQuotaHistory).mockResolvedValue({
      success: true,
      data: emptyHistory(),
    })
  })

  test('explains why multi-key channels have no combined history', () => {
    renderQuota({
      ...baseChannel,
      channel_info: { ...baseChannel.channel_info, is_multi_key: true },
    })

    expect(screen.getByText('Quota history unavailable')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Multi-key channels do not expose one combined account quota.'
      )
    ).toBeInTheDocument()
    expect(screen.queryByTestId('quota-history-trend')).not.toBeInTheDocument()
    expect(getChannelQuotaHistory).not.toHaveBeenCalled()
  })

  test('uses a complete request identity and refetches when range or granularity changes', async () => {
    renderQuota()
    await waitForInitialHistoryRequest()

    expect(getChannelQuotaHistory).toHaveBeenLastCalledWith(baseChannel.id, {
      range: '30d',
      start: undefined,
      end: undefined,
      granularity: 'auto',
      timezone_offset: expect.any(Number),
      limit: 5000,
    })

    expect(
      [
        ...(screen.getByLabelText('Time range') as HTMLSelectElement).options,
      ].map((option) => option.value)
    ).toEqual(['1h', '6h', '24h', '7d', '30d', '90d', 'custom'])
    expect(
      [
        ...(screen.getByLabelText('Chart granularity') as HTMLSelectElement)
          .options,
      ].map((option) => option.value)
    ).toEqual(['auto', 'raw', 'minute', '5m', '15m', 'hour', 'day', 'week'])

    fireEvent.change(screen.getByLabelText('Time range'), {
      target: { value: '1h' },
    })
    await waitFor(() =>
      expect(getChannelQuotaHistory).toHaveBeenLastCalledWith(
        baseChannel.id,
        expect.objectContaining({
          range: '1h',
          granularity: 'auto',
          limit: 5000,
        })
      )
    )

    fireEvent.change(screen.getByLabelText('Chart granularity'), {
      target: { value: 'minute' },
    })
    await waitFor(() =>
      expect(getChannelQuotaHistory).toHaveBeenLastCalledWith(
        baseChannel.id,
        expect.objectContaining({
          range: '1h',
          granularity: 'minute',
          limit: 5000,
        })
      )
    )
  })

  test('forwards metric and chart style controls without corrupting the history request', async () => {
    renderQuota()
    await waitForInitialHistoryRequest()
    const initialCallCount = vi.mocked(getChannelQuotaHistory).mock.calls.length

    fireEvent.change(screen.getByLabelText('Metric'), {
      target: { value: 'consumption' },
    })
    fireEvent.change(screen.getByLabelText('Chart style'), {
      target: { value: 'bar' },
    })

    expect(screen.getByLabelText('Metric')).toHaveValue('consumption')
    expect(screen.getByLabelText('Chart style')).toHaveValue('bar')
    expect(vi.mocked(getChannelQuotaHistory).mock.calls).toHaveLength(
      initialCallCount
    )
  })

  test('preserves the custom date workflow and does not query an unapplied range', async () => {
    renderQuota()
    await waitForInitialHistoryRequest()
    vi.clearAllMocks()

    fireEvent.change(screen.getByLabelText('Time range'), {
      target: { value: 'custom' },
    })

    expect(screen.getByRole('option', { name: 'Custom' })).toBeInTheDocument()
    expect(screen.queryByTestId('quota-history-trend')).not.toBeInTheDocument()
    expect(getChannelQuotaHistory).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('Start'), {
      target: { value: '2026-08-01' },
    })
    fireEvent.change(screen.getByLabelText('End'), {
      target: { value: '2026-08-15' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Apply Filters' }))

    await waitFor(() => expect(getChannelQuotaHistory).toHaveBeenCalledTimes(1))
    expect(getChannelQuotaHistory).toHaveBeenLastCalledWith(baseChannel.id, {
      range: undefined,
      start: new Date('2026-08-01T00:00:00').toISOString(),
      end: new Date('2026-08-15T23:59:59.999').toISOString(),
      granularity: 'auto',
      timezone_offset: expect.any(Number),
      limit: 5000,
    })
  })

  test('forwards a backend error instead of rendering a misleading empty chart', async () => {
    vi.mocked(getChannelQuotaHistory).mockResolvedValueOnce({
      success: false,
      message: 'quota endpoint unavailable',
    })

    renderQuota()

    expect(
      await screen.findByText('Unable to load quota history')
    ).toBeInTheDocument()
    expect(screen.getByText('quota endpoint unavailable')).toBeInTheDocument()
  })

  test('keeps the existing quota alert status visible alongside the shared trend', async () => {
    vi.mocked(getChannelQuotaHistory).mockResolvedValueOnce({
      success: true,
      data: {
        ...emptyHistory(),
        alert: {
          enabled: true,
          status: 'critical',
          ratio_percent: 5,
          warning_percent: 20,
          critical_percent: 10,
        },
      },
    })

    renderQuota()

    expect(await screen.findByText('Quota alert')).toBeInTheDocument()
    expect(screen.getByText('Critical')).toBeInTheDocument()
    expect(screen.getByText('Remaining: 5.0%')).toBeInTheDocument()
  })

  test('refreshes the same request on demand', async () => {
    renderQuota()
    await waitForInitialHistoryRequest()

    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))

    await waitFor(() => expect(getChannelQuotaHistory).toHaveBeenCalledTimes(2))
  })
})
