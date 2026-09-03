import { toIntlLocale } from '@/i18n/languages'
import { formatCurrencyFromUSD } from '@/lib/currency'

/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import type {
  ChannelQuotaHistoryData,
  ChannelQuotaHistoryRange,
  ChannelQuotaHistoryGranularity,
  ChannelQuotaHistoryMetric,
} from '../types'

export const quotaHistoryRangeOptions: readonly ChannelQuotaHistoryRange[] = [
  '1h',
  '6h',
  '24h',
  '7d',
  '30d',
  '90d',
  'custom',
]
export const quotaHistoryGranularityOptions: readonly ChannelQuotaHistoryGranularity[] =
  ['auto', 'raw', 'minute', '5m', '15m', 'hour', 'day', 'week']

export function formatQuotaAmount(
  value: number | null | undefined,
  units: { unit?: string; currency?: string } | undefined,
  metric: QuotaHistoryMetric,
  t: (key: string, options?: Record<string, unknown>) => string,
  locale?: string
): string {
  if (!finite(value)) return '-'
  if (units?.unit === 'percent') {
    if (metric === 'rate_per_minute') {
      return t('{{value}} percentage points/min', { value: value.toFixed(2) })
    }
    if (metric === 'consumption') {
      return t('{{value}} percentage points', { value: value.toFixed(2) })
    }
    return `${value.toFixed(1)}%`
  }
  const currency = units?.currency?.toUpperCase()
  const intlLocale = toIntlLocale(locale)
  let amount: string
  if (
    currency === 'USD' ||
    (!currency && units?.unit?.toLowerCase() === 'usd')
  ) {
    amount = formatCurrencyFromUSD(value, {
      digitsLarge: 2,
      digitsSmall: 4,
      abbreviate: false,
    })
  } else if (currency) {
    try {
      amount = new Intl.NumberFormat(intlLocale, {
        style: 'currency',
        currency,
        maximumFractionDigits: 4,
      }).format(value)
    } catch {
      amount = `${new Intl.NumberFormat(intlLocale, { maximumFractionDigits: 4 }).format(value)} ${currency}`
    }
  } else {
    amount = new Intl.NumberFormat(intlLocale, {
      maximumFractionDigits: 4,
    }).format(value)
    if (units?.unit) amount += ` ${units.unit}`
  }
  return metric === 'rate_per_minute'
    ? t('{{value}}/min', { value: amount })
    : amount
}

export type QuotaHistoryMetric = ChannelQuotaHistoryMetric
export type QuotaHistoryChartStyle = 'line' | 'area' | 'bar'
export interface QuotaHistoryTrendPoint {
  timestamp: number
  observedAt: number
  value: number | null
  status: string
  resetAt?: number
  errorCode?: string
  isResetBoundary: boolean
  hasContinuityBreak: boolean
  observedSeconds?: number
  intervalCount?: number
}
export interface QuotaHistoryTrendSummary {
  metric: QuotaHistoryMetric
  start: number | null
  end: number | null
  change: number | null
  minimum: number | null
  maximum: number | null
  observedConsumption: number | null
  averageRate: number | null
  peakRate: number | null
  peakObservedAt: number | null
  observedSeconds: number
  valueCount: number
  isLatestResetSegment: boolean
}
export interface QuotaHistoryTrendLatest {
  observedAt: number | null
  status: string | null
  value: number | null
  errorCode?: string
}
export interface QuotaHistoryTrendCoverage {
  requestedStart: number | null
  requestedEnd: number | null
  observedStart: number | null
  observedEnd: number | null
  successCount: number
  failedCount: number
  unsupportedCount: number
  invalidCount: number
  interruptedCount: number
  recoveryCount: number
  resetBoundaries: number
  gapCount: number
  baselineChangeCount: number
  truncated: boolean
}
export interface QuotaHistoryTrend {
  metric: QuotaHistoryMetric
  points: QuotaHistoryTrendPoint[]
  latest: QuotaHistoryTrendLatest
  latestPlotted: { timestamp: number; value: number } | null
  summary: QuotaHistoryTrendSummary
  coverage: QuotaHistoryTrendCoverage
  hasIncompleteData: boolean
}

function finite(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}
function numberOrNull(value: unknown): number | null {
  return finite(value) ? value : null
}
export function isIntervalMetric(metric: QuotaHistoryMetric): boolean {
  return metric === 'consumption' || metric === 'rate_per_minute'
}

/**
 * Display server-calculated intervals. Never infer consumption by subtracting
 * display buckets: that loses resets, failures and the actual elapsed time.
 */
export function buildQuotaHistoryTrend(
  data: ChannelQuotaHistoryData,
  metric: QuotaHistoryMetric
): QuotaHistoryTrend {
  const rawPoints = [...data.points].sort((a, b) => a.timestamp - b.timestamp)
  const points: QuotaHistoryTrendPoint[] = []
  const intervalMetric = isIntervalMetric(metric)
  for (const point of rawPoints) {
    const hasBreak = Boolean(
      point.continuity_break ||
      point.reset ||
      point.gap ||
      point.recovery ||
      point.baseline_change
    )
    // Preserve the post-reset observation while breaking the line before it.
    // An interval bucket may have valid pairs before its final failed attempt.
    if (hasBreak && points.length > 0) {
      points.push({
        timestamp: point.timestamp - 0.001,
        observedAt: point.observed_at ?? point.timestamp,
        value: null,
        status: 'gap',
        isResetBoundary: false,
        hasContinuityBreak: true,
      })
    }
    points.push({
      timestamp: point.timestamp,
      observedAt: point.observed_at ?? point.timestamp,
      value:
        intervalMetric || point.status === 'success'
          ? numberOrNull(point[metric])
          : null,
      status: point.status,
      resetAt: point.reset_at,
      errorCode: point.error_code,
      isResetBoundary: Boolean(point.reset),
      hasContinuityBreak: hasBreak,
      observedSeconds: point.observed_seconds,
      intervalCount: point.interval_count,
    })
  }
  const values = points.map((point) => point.value).filter(finite)
  const lastPlotted = [...points].reverse().find((point) => finite(point.value))
  const lastRaw = rawPoints.at(-1)
  const current = data.current
  const latestStatus = current?.status ?? lastRaw?.status ?? null
  let latestValue: number | null = null
  if (!intervalMetric && latestStatus === 'success') {
    const carrier = current ?? lastRaw
    if (carrier) {
      latestValue = numberOrNull(
        carrier[metric as 'available' | 'used' | 'total']
      )
    }
  }
  const metricSummary = intervalMetric
    ? undefined
    : data.summary?.[metric as 'available' | 'used' | 'total']
  const consumption = data.summary?.consumption
  const quality = data.data_quality
  const count = (
    status: string,
    field: 'success_count' | 'failed_count' | 'unsupported_count'
  ) =>
    rawPoints.reduce(
      (sum, point) => sum + (point[field] ?? Number(point.status === status)),
      0
    )
  const coverage: QuotaHistoryTrendCoverage = {
    requestedStart: numberOrNull(data.start),
    requestedEnd: numberOrNull(data.end),
    observedStart: rawPoints[0]?.observed_at ?? rawPoints[0]?.timestamp ?? null,
    observedEnd: lastRaw?.observed_at ?? lastRaw?.timestamp ?? null,
    successCount: quality?.success_count ?? count('success', 'success_count'),
    failedCount: quality?.error_count ?? count('error', 'failed_count'),
    unsupportedCount:
      quality?.unsupported_count ?? count('unsupported', 'unsupported_count'),
    invalidCount: quality?.invalid_count ?? 0,
    interruptedCount: consumption?.interrupted_count ?? 0,
    recoveryCount: consumption?.recovery_count ?? 0,
    resetBoundaries:
      quality?.reset_boundaries ??
      consumption?.reset_boundaries ??
      rawPoints.filter((point) => point.reset).length,
    gapCount: consumption?.gap_count ?? 0,
    baselineChangeCount: consumption?.baseline_change_count ?? 0,
    truncated: Boolean(
      data.truncated ||
      data.complete === false ||
      data.source_complete === false ||
      data.points_complete === false
    ),
  }
  return {
    metric,
    points,
    latest: {
      observedAt:
        current?.observed_at ??
        lastRaw?.observed_at ??
        lastRaw?.timestamp ??
        null,
      status: latestStatus,
      value: latestValue,
      errorCode: current?.error_code ?? lastRaw?.error_code,
    },
    latestPlotted:
      lastPlotted && finite(lastPlotted.value)
        ? { timestamp: lastPlotted.timestamp, value: lastPlotted.value }
        : null,
    summary: {
      metric,
      start: numberOrNull(metricSummary?.start),
      end: numberOrNull(metricSummary?.end),
      change: numberOrNull(metricSummary?.change),
      minimum: numberOrNull(metricSummary?.minimum),
      maximum: numberOrNull(metricSummary?.maximum),
      observedConsumption: numberOrNull(consumption?.observed),
      averageRate: numberOrNull(consumption?.average_rate_per_minute),
      peakRate: numberOrNull(consumption?.peak_rate_per_minute),
      peakObservedAt: numberOrNull(consumption?.peak_rate_observed_at),
      observedSeconds: consumption?.observed_seconds ?? 0,
      valueCount: values.length,
      isLatestResetSegment: !intervalMetric && coverage.resetBoundaries > 0,
    },
    coverage,
    hasIncompleteData:
      coverage.failedCount > 0 ||
      coverage.unsupportedCount > 0 ||
      coverage.invalidCount > 0 ||
      coverage.interruptedCount > 0 ||
      coverage.recoveryCount > 0 ||
      coverage.gapCount > 0 ||
      coverage.baselineChangeCount > 0 ||
      coverage.truncated,
  }
}

export function quotaWindowLabel(
  window: string | undefined,
  t: (key: string) => string
): string {
  if (window === 'weekly') return t('Weekly window')
  if (window === 'five_hour' || window === '5h') return t('5-hour window')
  if (window === 'primary') return t('Primary window')
  if (window === 'secondary') return t('Secondary window')
  if (window === 'daily') return t('Daily window')
  if (!window || window === 'none') return t('Quota')
  return t('Other quota window')
}

export function quotaSeriesKey(item: {
  channel_id: number
  metric_type?: string
  source?: string
  window_type?: string
  plan_type?: string
  unit?: string
  currency?: string
  window_seconds?: number
}): string {
  return [
    item.channel_id,
    item.metric_type,
    item.source,
    item.window_type,
    item.plan_type,
    item.unit,
    item.currency,
    item.window_seconds,
  ]
    .map((value) => value ?? '')
    .join('|')
}
