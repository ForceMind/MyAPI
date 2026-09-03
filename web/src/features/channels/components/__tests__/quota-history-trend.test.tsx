/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import i18next from 'i18next'
import { useState } from 'react'
import { beforeEach, afterEach, describe, expect, test, vi } from 'vitest'

import zhTW from '@/i18n/locales/zh-TW.json'
import zh from '@/i18n/locales/zh.json'

import type {
  QuotaHistoryChartStyle,
  QuotaHistoryMetric,
} from '../../lib/quota-history'
import type {
  ChannelQuotaHistoryData,
  ChannelQuotaHistoryPoint,
} from '../../types'
import {
  QuotaCustomRangeControls,
  QuotaHistoryTrend,
} from '../quota-history-trend'

// Only the browser's layout boundary is simulated. Recharts itself is real.
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
})
afterEach(async () => {
  vi.restoreAllMocks()
  await i18next.changeLanguage('en')
})

function makeHistory(
  points: ChannelQuotaHistoryPoint[],
  overrides: Partial<ChannelQuotaHistoryData> = {}
): ChannelQuotaHistoryData {
  return {
    channel_id: 12,
    start: 100,
    end: 300,
    limit: 500,
    unit: 'percent',
    points,
    ...overrides,
  }
}

function ControlledTrend(props: { data: ChannelQuotaHistoryData }) {
  const [range, setRange] = useState('24h')
  const [granularity, setGranularity] = useState('raw')
  const [metric, setMetric] = useState<QuotaHistoryMetric>('available')
  const [chartStyle, setChartStyle] = useState<QuotaHistoryChartStyle>('line')

  return (
    <QuotaHistoryTrend
      data={props.data}
      range={range}
      granularity={granularity}
      metric={metric}
      chartStyle={chartStyle}
      onRangeChange={setRange}
      onGranularityChange={setGranularity}
      onMetricChange={setMetric}
      onChartStyleChange={setChartStyle}
    />
  )
}

describe('quota history trend', () => {
  test('prevents custom ranges longer than the backend 180-day limit and accepts a shorter range', () => {
    const onApply = vi.fn()
    render(
      <QuotaCustomRangeControls
        range={{ start: '2026-01-01T00:00:00Z', end: '2026-01-02T00:00:00Z' }}
        onApply={onApply}
      />
    )
    fireEvent.change(screen.getByLabelText('Custom range start'), {
      target: { value: '2026-01-01T00:00' },
    })
    fireEvent.change(screen.getByLabelText('Custom range end'), {
      target: { value: '2026-09-01T00:00' },
    })
    expect(screen.getByText('Apply Filters')).toBeDisabled()
    expect(
      screen.getByText('Choose a valid time range of at most 180 days.')
    ).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Custom range end'), {
      target: { value: '2026-01-02T00:00' },
    })
    fireEvent.click(screen.getByText('Apply Filters'))
    expect(onApply).toHaveBeenCalledWith({
      start: new Date('2026-01-01T00:00').toISOString(),
      end: new Date('2026-01-02T00:00').toISOString(),
    })
  })
  test.each(['zhCN', 'zhTW'])(
    'renders a real chart with the %s interface language without an Intl crash',
    async (language) => {
      i18next.addResourceBundle('zhCN', 'translation', zh.translation)
      i18next.addResourceBundle('zhTW', 'translation', zhTW.translation)
      await i18next.changeLanguage(language)
      const { container } = render(
        <ControlledTrend
          data={makeHistory([
            { timestamp: 100, status: 'success', available: 90 },
            { timestamp: 200, status: 'success', available: 80 },
          ])}
        />
      )
      await waitFor(() =>
        expect(container.querySelector('.recharts-line-curve')).toHaveAttribute(
          'd'
        )
      )
      expect(screen.getByLabelText(i18next.t('Chart style'))).toBeEnabled()
      expect(screen.queryByText('Latest raw sample')).not.toBeInTheDocument()
      fireEvent.change(screen.getByLabelText(i18next.t('Chart style')), {
        target: { value: 'bar' },
      })
      await waitFor(() =>
        expect(
          container.querySelector('.recharts-bar-rectangle path')
        ).toHaveAttribute('d')
      )
    }
  )
  test('renders the selected line, area, and bar chart styles as real SVG paths through controlled options', async () => {
    const { container } = render(
      <ControlledTrend
        data={makeHistory([
          { timestamp: 100, status: 'success', available: 90, used: 10 },
          { timestamp: 200, status: 'success', available: 80, used: 20 },
        ])}
      />
    )

    expect(screen.getByTestId('quota-history-chart-line')).toBeInTheDocument()
    await waitFor(() =>
      expect(container.querySelector('.recharts-line-curve')).toHaveAttribute(
        'd'
      )
    )

    fireEvent.change(screen.getByLabelText('Chart style'), {
      target: { value: 'area' },
    })
    expect(screen.getByTestId('quota-history-chart-area')).toBeInTheDocument()
    await waitFor(() =>
      expect(container.querySelector('.recharts-area-area')).toHaveAttribute(
        'd'
      )
    )

    fireEvent.change(screen.getByLabelText('Chart style'), {
      target: { value: 'bar' },
    })
    expect(screen.getByTestId('quota-history-chart-bar')).toBeInTheDocument()
    await waitFor(() =>
      expect(
        container.querySelector('.recharts-bar-rectangle path')
      ).toHaveAttribute('d')
    )
  })

  test('passes range, granularity, and selected metric through controlled callbacks', () => {
    const onRangeChange = vi.fn()
    const onGranularityChange = vi.fn()
    const onMetricChange = vi.fn()
    const onChartStyleChange = vi.fn()

    render(
      <QuotaHistoryTrend
        data={makeHistory([
          { timestamp: 100, status: 'success', available: 90, used: 10 },
          { timestamp: 200, status: 'success', available: 80, used: 20 },
        ])}
        range='24h'
        granularity='raw'
        metric='available'
        chartStyle='line'
        onRangeChange={onRangeChange}
        onGranularityChange={onGranularityChange}
        onMetricChange={onMetricChange}
        onChartStyleChange={onChartStyleChange}
      />
    )

    fireEvent.change(screen.getByLabelText('Time range'), {
      target: { value: '7d' },
    })
    fireEvent.change(screen.getByLabelText('Chart granularity'), {
      target: { value: 'hour' },
    })
    fireEvent.change(screen.getByLabelText('Metric'), {
      target: { value: 'used' },
    })
    fireEvent.change(screen.getByLabelText('Chart style'), {
      target: { value: 'area' },
    })

    expect(onRangeChange).toHaveBeenCalledWith('7d')
    expect(onGranularityChange).toHaveBeenCalledWith('hour')
    expect(onMetricChange).toHaveBeenCalledWith('used')
    expect(onChartStyleChange).toHaveBeenCalledWith('area')
  })

  test('keeps a failed latest sample distinct from the last plotted value', () => {
    render(
      <QuotaHistoryTrend
        data={makeHistory(
          [
            { timestamp: 100, status: 'success', available: 90 },
            { timestamp: 200, status: 'success', available: 70 },
            { timestamp: 300, status: 'error', error_code: 'upstream_timeout' },
          ],
          {
            current: {
              observed_at: 300,
              status: 'error',
              error_code: 'upstream_timeout',
            },
          }
        )}
        range='24h'
        granularity='raw'
        metric='available'
        chartStyle='line'
      />
    )

    expect(screen.getByText('Latest raw sample')).toBeInTheDocument()
    expect(screen.getByText('Error')).toBeInTheDocument()
    expect(screen.getByText('Last plotted value: 70.0%')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Failed or discontinuous samples remain visible as chart gaps.'
      )
    ).toBeInTheDocument()
  })

  test('shows an actionable history error instead of a blank chart', () => {
    render(
      <QuotaHistoryTrend
        range='24h'
        granularity='raw'
        metric='available'
        chartStyle='line'
        error={new Error('quota endpoint unavailable')}
      />
    )

    expect(screen.getByText('Unable to load quota history')).toBeInTheDocument()
    expect(screen.getByText('quota endpoint unavailable')).toBeInTheDocument()
  })
  test('shows latest error and historical statistics when all plotted consumption values are missing', () => {
    render(
      <QuotaHistoryTrend
        data={makeHistory([{ timestamp: 300, status: 'error' }], {
          current: { observed_at: 300, status: 'error' },
          summary: {
            start_available: 90,
            end_available: 80,
            change: -10,
            change_percent: -11.1,
            minimum: 80,
            maximum: 90,
            consumption: {
              observed: 10,
              pair_count: 2,
              peak_rate_per_minute: 2,
            },
          },
        })}
        range='24h'
        granularity='hour'
        metric='consumption'
        chartStyle='bar'
      />
    )
    expect(screen.getByText('Latest raw sample')).toBeInTheDocument()
    expect(screen.getByText('Error')).toBeInTheDocument()
    expect(screen.getByText('10.00 percentage points')).toBeInTheDocument()
    expect(
      screen.getByText('Selected metric is unavailable')
    ).toBeInTheDocument()
    expect(
      screen.queryByTestId('quota-history-chart-bar')
    ).not.toBeInTheDocument()
  })
})
