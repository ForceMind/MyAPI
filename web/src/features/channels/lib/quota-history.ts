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
  ChannelQuotaAnalysisMethod,
  ChannelQuotaETA,
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
export const quotaAnalysisDurationOptions = [
  60,
  5 * 60,
  15 * 60,
  30 * 60,
  60 * 60,
  6 * 60 * 60,
  24 * 60 * 60,
  7 * 24 * 60 * 60,
  30 * 24 * 60 * 60,
  90 * 24 * 60 * 60,
] as const

type Translate = (key: string, options?: Record<string, unknown>) => string

function formatQuotaScalar(
  value: number,
  units: { unit?: string; currency?: string } | undefined,
  locale?: string
): string {
  const currency = units?.currency?.toUpperCase()
  const intlLocale = toIntlLocale(locale)
  if (
    currency === 'USD' ||
    (!currency && units?.unit?.toLowerCase() === 'usd')
  ) {
    return formatCurrencyFromUSD(value, {
      digitsLarge: 2,
      digitsSmall: 4,
      abbreviate: false,
    })
  }
  if (currency) {
    try {
      return new Intl.NumberFormat(intlLocale, {
        style: 'currency',
        currency,
        maximumFractionDigits: 4,
      }).format(value)
    } catch {
      return `${new Intl.NumberFormat(intlLocale, { maximumFractionDigits: 4 }).format(value)} ${currency}`
    }
  }
  let amount = new Intl.NumberFormat(intlLocale, {
    maximumFractionDigits: 4,
  }).format(value)
  if (units?.unit) amount += ` ${units.unit}`
  return amount
}

export function formatQuotaRate(
  value: number | null | undefined,
  units: { unit?: string; currency?: string } | undefined,
  period: 'minute' | 'hour',
  t: Translate,
  locale?: string
): string {
  if (!finite(value)) return '-'
  if (units?.unit === 'percent') {
    const key =
      period === 'minute'
        ? '{{value}} percentage points/min'
        : '{{value}} percentage points/hour'
    return t(key, { value: value.toFixed(2) })
  }
  const amount = formatQuotaScalar(value, units, locale)
  const key = period === 'minute' ? '{{value}}/min' : '{{value}}/hour'
  return t(key, { value: amount })
}

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
      return formatQuotaRate(value, units, 'minute', t, locale)
    }
    if (metric === 'consumption') {
      return t('{{value}} percentage points', { value: value.toFixed(2) })
    }
    return `${value.toFixed(1)}%`
  }
  if (metric === 'rate_per_minute') {
    return formatQuotaRate(value, units, 'minute', t, locale)
  }
  return formatQuotaScalar(value, units, locale)
}

export function quotaHistoryRangeSeconds(
  range: ChannelQuotaHistoryRange,
  customRange?: { start: string; end: string }
): number {
  const secondsByRange: Partial<Record<ChannelQuotaHistoryRange, number>> = {
    '1h': 60 * 60,
    '6h': 6 * 60 * 60,
    '24h': 24 * 60 * 60,
    '7d': 7 * 24 * 60 * 60,
    '30d': 30 * 24 * 60 * 60,
    '90d': 90 * 24 * 60 * 60,
  }
  if (range !== 'custom') return secondsByRange[range] ?? 60 * 60
  if (!customRange) return 60 * 60
  const start = new Date(customRange.start).getTime()
  const end = new Date(customRange.end).getTime()
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) {
    return 60 * 60
  }
  return Math.max(1, Math.floor((end - start) / 1000))
}

export function boundedQuotaDuration(value: number, maximum: number): number {
  return Math.max(1, Math.min(Math.floor(value), Math.floor(maximum)))
}

export function quotaDurationOptions(
  maximum: number,
  selected: number
): number[] {
  const bounded = boundedQuotaDuration(selected, maximum)
  return [...new Set([...quotaAnalysisDurationOptions, bounded])]
    .filter((seconds) => seconds <= maximum)
    .sort((left, right) => left - right)
}

export function formatQuotaDuration(seconds: number, t: Translate): string {
  const rounded = (value: number) => Math.round(value * 10) / 10
  if (seconds >= 24 * 60 * 60) {
    const count = rounded(seconds / (24 * 60 * 60))
    return count === 1 ? t('1 day') : t('{{count}} days', { count })
  }
  if (seconds >= 60 * 60) {
    const count = rounded(seconds / (60 * 60))
    return count === 1 ? t('1 hour') : t('{{count}} hours', { count })
  }
  if (seconds >= 60) {
    const count = rounded(seconds / 60)
    return count === 1 ? t('1 minute') : t('{{count}} minutes', { count })
  }
  const count = Math.max(0, Math.round(seconds))
  return count === 1 ? t('1 second') : t('{{count}} seconds', { count })
}

export function quotaAnalysisMethodLabel(
  method: ChannelQuotaAnalysisMethod,
  t: Translate
): string {
  if (method === 'latest_interval') return t('Latest observed interval')
  if (method === 'ewma') return t('EWMA')
  return t('Observed window')
}

export function quotaETAStatus(
  eta: ChannelQuotaETA | undefined
): 'depletion' | 'reset' | 'stable' | 'insufficient' {
  if (eta?.outcome === 'depletes_before_reset') return 'depletion'
  if (eta?.outcome === 'reset_before_depletion') return 'reset'
  if (eta?.outcome === 'stable_or_no_observed_consumption') return 'stable'
  return 'insufficient'
}

export function quotaPredictionTiming(
  eventAt: number | null | undefined,
  analysisAsOf: number | null | undefined
):
  | { status: 'future'; seconds: number }
  | { status: 'passed' | 'unavailable' } {
  if (!finite(eventAt) || !finite(analysisAsOf)) {
    return { status: 'unavailable' }
  }
  if (eventAt <= analysisAsOf) {
    return { status: 'passed' }
  }
  return { status: 'future', seconds: eventAt - analysisAsOf }
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
