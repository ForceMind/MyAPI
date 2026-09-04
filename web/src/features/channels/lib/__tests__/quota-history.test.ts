import i18next from 'i18next'
/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { describe, expect, test } from 'vitest'

import type {
  ChannelQuotaHistoryData,
  ChannelQuotaHistoryPoint,
} from '../../types'
import {
  boundedQuotaDuration,
  buildQuotaHistoryTrend,
  formatQuotaAmount,
  formatQuotaDuration,
  formatQuotaRate,
  quotaDurationOptions,
  quotaHistoryRangeSeconds,
  quotaPredictionTiming,
  quotaSeriesKey,
} from '../quota-history'

function history(
  points: ChannelQuotaHistoryPoint[],
  overrides: Partial<ChannelQuotaHistoryData> = {}
): ChannelQuotaHistoryData {
  return {
    channel_id: 1,
    start: 100,
    end: 1000,
    limit: 5000,
    points,
    ...overrides,
  }
}
const summary = {
  start_available: 90,
  end_available: 80,
  change: -10,
  change_percent: -11.1,
  minimum: 80,
  maximum: 90,
}

describe('quota history server contract', () => {
  test('formats hourly analysis rates without treating percentages as balances', () => {
    expect(
      formatQuotaRate(90, { unit: 'percent' }, 'hour', i18next.t, 'en-US')
    ).toBe('90.00 percentage points/hour')
    expect(
      formatQuotaRate(12.5, { currency: 'EUR' }, 'hour', i18next.t, 'en-US')
    ).toBe('€12.50/hour')
  })

  test('bounds analysis durations independently within preset and custom ranges', () => {
    expect(quotaHistoryRangeSeconds('24h')).toBe(24 * 60 * 60)
    expect(
      quotaHistoryRangeSeconds('custom', {
        start: '2026-01-01T00:00:00Z',
        end: '2026-01-01T00:20:00Z',
      })
    ).toBe(20 * 60)
    expect(boundedQuotaDuration(6 * 60 * 60, 60 * 60)).toBe(60 * 60)
    expect(quotaDurationOptions(20 * 60, 60 * 60)).toContain(20 * 60)
    expect(formatQuotaDuration(200, i18next.t)).toBe('3.3 minutes')
    expect(formatQuotaDuration(60 * 60, i18next.t)).toBe('1 hour')
    expect(formatQuotaDuration(6 * 60 * 60, i18next.t)).toBe('6 hours')
  })

  test('keeps overview React identities distinct across unit currency and window seconds', () => {
    const base = {
      channel_id: 7,
      metric_type: 'balance',
      source: 'provider',
      window_type: 'monthly',
      plan_type: 'team',
    }
    const keys = [
      quotaSeriesKey({
        ...base,
        unit: 'usd',
        currency: 'USD',
        window_seconds: 1,
      }),
      quotaSeriesKey({
        ...base,
        unit: 'usd',
        currency: 'EUR',
        window_seconds: 1,
      }),
      quotaSeriesKey({
        ...base,
        unit: 'percent',
        currency: 'USD',
        window_seconds: 1,
      }),
      quotaSeriesKey({
        ...base,
        unit: 'usd',
        currency: 'USD',
        window_seconds: 2,
      }),
    ]
    expect(new Set(keys).size).toBe(keys.length)
  })

  test('derives prediction timing from the analysis point rather than stale relative seconds', () => {
    expect(quotaPredictionTiming(300, 250)).toEqual({
      status: 'future',
      seconds: 50,
    })
    expect(quotaPredictionTiming(250, 250)).toEqual({ status: 'passed' })
    expect(quotaPredictionTiming(200, 250)).toEqual({ status: 'passed' })
    expect(quotaPredictionTiming(undefined, 250)).toEqual({
      status: 'unavailable',
    })
  })

  test('preserves a non-USD source currency instead of interpreting it as USD', () => {
    expect(
      formatQuotaAmount(
        12.5,
        { unit: 'usd', currency: 'EUR' },
        'available',
        i18next.t,
        'en-US'
      )
    ).toBe('€12.50')
    expect(
      formatQuotaAmount(
        12.5,
        { currency: 'CNY' },
        'rate_per_minute',
        i18next.t,
        'en-US'
      )
    ).toBe('CN¥12.50/min')
    expect(
      formatQuotaAmount(
        2,
        { unit: 'percent' },
        'consumption',
        i18next.t,
        'en-US'
      )
    ).toBe('2.00 percentage points')
  })

  test('accepts internal Chinese language identifiers and unknown locale values safely', () => {
    expect(() =>
      formatQuotaAmount(1, { currency: 'EUR' }, 'available', i18next.t, 'zhCN')
    ).not.toThrow()
    expect(() =>
      formatQuotaAmount(1, { currency: 'CNY' }, 'available', i18next.t, 'zhTW')
    ).not.toThrow()
    expect(() =>
      formatQuotaAmount(
        1,
        { unit: 'tokens' },
        'available',
        i18next.t,
        'invalid_locale'
      )
    ).not.toThrow()
  })
  test('does not invent consumption or rates from available balance when interval fields are missing', () => {
    const data = history([
      { timestamp: 100, status: 'success', available: 90 },
      { timestamp: 160, status: 'success', available: 80 },
    ])
    expect(
      buildQuotaHistoryTrend(data, 'consumption').points.map(
        (point) => point.value
      )
    ).toEqual([null, null])
    expect(
      buildQuotaHistoryTrend(data, 'rate_per_minute').summary
        .observedConsumption
    ).toBeNull()
  })

  test('uses server rates for actual 60 second and 600 second intervals', () => {
    const data = history([
      { timestamp: 100, status: 'success', used: 1 },
      {
        timestamp: 160,
        status: 'success',
        used: 3,
        consumption: 2,
        rate_per_minute: 2,
        observed_seconds: 60,
      },
      {
        timestamp: 760,
        status: 'success',
        used: 5,
        consumption: 2,
        rate_per_minute: 0.2,
        observed_seconds: 600,
      },
    ])
    expect(
      buildQuotaHistoryTrend(data, 'rate_per_minute').points.map(
        (point) => point.value
      )
    ).toEqual([null, 2, 0.2])
    expect(
      buildQuotaHistoryTrend(data, 'consumption').points.map(
        (point) => point.value
      )
    ).toEqual([null, 2, 2])
  })

  test('retains trusted consumption inside a mixed failed bucket and keeps the latest failure separate', () => {
    const trend = buildQuotaHistoryTrend(
      history(
        [
          {
            timestamp: 100,
            status: 'success',
            consumption: 2,
            rate_per_minute: 2,
          },
          {
            timestamp: 200,
            observed_at: 259,
            status: 'error',
            continuity_break: true,
            failed_count: 1,
            consumption: 3,
            rate_per_minute: 1,
            observed_seconds: 180,
          },
        ],
        {
          current: {
            observed_at: 259,
            status: 'error',
            error_code: 'upstream_timeout',
          },
          summary: {
            ...summary,
            consumption: {
              observed: 5,
              pair_count: 3,
              observed_seconds: 240,
              average_rate_per_minute: 1.25,
              peak_rate_per_minute: 2,
              peak_rate_observed_at: 160,
            },
          },
        }
      ),
      'consumption'
    )
    expect(trend.points.map((point) => point.value)).toEqual([2, null, 3])
    expect(trend.latest).toMatchObject({
      status: 'error',
      value: null,
      observedAt: 259,
    })
    expect(trend.summary).toMatchObject({
      observedConsumption: 5,
      peakRate: 2,
      peakObservedAt: 160,
    })
    expect(trend.hasIncompleteData).toBe(true)
  })

  test('preserves post-reset balances with a gap and does not fill unobserved consumption', () => {
    const data = history([
      { timestamp: 100, status: 'success', available: 50 },
      {
        timestamp: 200,
        status: 'success',
        available: 100,
        reset: true,
        continuity_break: true,
      },
      { timestamp: 300, status: 'error' },
    ])
    expect(
      buildQuotaHistoryTrend(data, 'available').points.map(
        (point) => point.value
      )
    ).toEqual([50, null, 100, null])
    expect(
      buildQuotaHistoryTrend(data, 'consumption').points.every(
        (point) => point.value == null
      )
    ).toBe(true)
  })

  test('selected metric statistics come from the matching server summary', () => {
    const data = history(
      [{ timestamp: 100, status: 'success', available: 80, used: 20 }],
      {
        summary: {
          ...summary,
          used: { start: 10, end: 20, change: 10, minimum: 10, maximum: 20 },
        },
      }
    )
    expect(buildQuotaHistoryTrend(data, 'used').summary).toMatchObject({
      start: 10,
      end: 20,
      change: 10,
      minimum: 10,
      maximum: 20,
    })
  })

  test('changing display granularity cannot change the trusted consumption total or historical peak', () => {
    const common = {
      ...summary,
      consumption: {
        observed: 7,
        pair_count: 2,
        peak_rate_per_minute: 4,
        average_rate_per_minute: 3.5,
        observed_seconds: 120,
      },
    }
    const raw = history(
      [
        { timestamp: 100, status: 'success', consumption: 3 },
        { timestamp: 160, status: 'success', consumption: 4 },
      ],
      { summary: common }
    )
    const hourly = history(
      [{ timestamp: 0, observed_at: 160, status: 'success', consumption: 7 }],
      { summary: common }
    )
    expect(
      buildQuotaHistoryTrend(raw, 'consumption').summary.observedConsumption
    ).toBe(7)
    expect(
      buildQuotaHistoryTrend(hourly, 'consumption').summary.observedConsumption
    ).toBe(7)
    expect(
      buildQuotaHistoryTrend(hourly, 'rate_per_minute').summary.peakRate
    ).toBe(4)
  })
})
