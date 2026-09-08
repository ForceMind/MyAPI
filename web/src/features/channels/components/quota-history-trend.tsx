/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { RefreshCw } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Line,
  LineChart,
  ReferenceLine,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ChartContainer } from '@/components/ui/chart'
import { Skeleton } from '@/components/ui/skeleton'
import { toIntlLocale } from '@/i18n/languages'
import { cn } from '@/lib/utils'

import type { QuotaCustomRange } from '../hooks/use-quota-history-time'
import {
  buildQuotaHistoryTrend,
  formatQuotaDuration,
  formatQuotaAmount as formatQuotaValue,
  formatQuotaRate,
  isIntervalMetric,
  quotaAnalysisMethodLabel,
  quotaETAStatus,
  quotaPredictionTiming,
  type QuotaHistoryChartStyle,
  type QuotaHistoryMetric,
  type QuotaHistoryTrend,
} from '../lib/quota-history'
import type {
  ChannelQuotaAnalysisMethod,
  ChannelQuotaHistoryData,
} from '../types'

const DEFAULT_RANGE_OPTIONS = [
  '1h',
  '6h',
  '24h',
  '7d',
  '30d',
  '90d',
  'custom',
] as const
const DEFAULT_GRANULARITY_OPTIONS = [
  'auto',
  'raw',
  'minute',
  '5m',
  '15m',
  'hour',
  'day',
  'week',
] as const
const METRICS: QuotaHistoryMetric[] = [
  'available',
  'used',
  'total',
  'consumption',
  'rate_per_minute',
]
const CHART_STYLES: QuotaHistoryChartStyle[] = ['line', 'area', 'bar']

type Translate = (key: string, options?: Record<string, unknown>) => string

export interface QuotaHistoryTrendProps {
  data?: ChannelQuotaHistoryData
  title?: string
  range: string
  granularity: string
  metric: QuotaHistoryMetric
  chartStyle: QuotaHistoryChartStyle
  rangeOptions?: readonly string[]
  /** Optional label mapping for caller-specific values such as `custom`. */
  rangeOptionLabel?: (range: string) => string
  granularityOptions?: readonly string[]
  isLoading?: boolean
  error?: unknown
  errorMessage?: string
  onRangeChange?: (range: string) => void
  onGranularityChange?: (granularity: string) => void
  onMetricChange?: (metric: QuotaHistoryMetric) => void
  onChartStyleChange?: (chartStyle: QuotaHistoryChartStyle) => void
  analysisMethod?: ChannelQuotaAnalysisMethod
  rateWindowSeconds?: number
  ewmaHalfLifeSeconds?: number
  rateWindowOptions?: readonly number[]
  ewmaHalfLifeOptions?: readonly number[]
  onAnalysisMethodChange?: (method: ChannelQuotaAnalysisMethod) => void
  onRateWindowChange?: (seconds: number) => void
  onEWMAHalfLifeChange?: (seconds: number) => void
  onRefresh?: () => void
  className?: string
  customRange?: QuotaCustomRange
  onCustomRangeChange?: (range: QuotaCustomRange) => void
}

function finiteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}

function metricLabel(metric: QuotaHistoryMetric, t: Translate) {
  if (metric === 'available') return t('Available quota')
  if (metric === 'used') return t('Used quota')
  if (metric === 'total') return t('Total quota')
  if (metric === 'rate_per_minute') return t('Estimated consumption per minute')
  return t('Observed consumption')
}

function chartStyleLabel(chartStyle: QuotaHistoryChartStyle, t: Translate) {
  if (chartStyle === 'line') return t('Line')
  if (chartStyle === 'area') return t('Area')
  return t('Bar')
}

function granularityLabel(value: string, t: Translate) {
  if (value === 'auto') return t('Auto')
  if (value === 'raw') return t('Raw')
  if (value === 'minute' || value === '1m') return t('1 minute')
  if (value === '5m') return t('5 minutes')
  if (value === '15m') return t('15 minutes')
  if (value === 'hour') return t('Hour')
  if (value === 'day') return t('Day')
  if (value === 'week') return t('Week')
  return value
}

function statusLabel(status: string | null, t: Translate) {
  if (status === 'success') return t('Success')
  if (status === 'error') return t('Error')
  if (status === 'unsupported') return t('Unsupported')
  if (status === 'unavailable') return t('Unavailable')
  return t('Unknown')
}

function statusVariant(status: string | null) {
  if (status === 'error') return 'destructive' as const
  if (status === 'unsupported' || status === 'unavailable') {
    return 'warning' as const
  }
  return 'outline' as const
}

function formatTimestamp(
  timestamp: number | null,
  locale: string | undefined,
  compact: boolean
) {
  if (!finiteNumber(timestamp) || timestamp <= 0) return '-'
  const options: Intl.DateTimeFormatOptions = compact
    ? { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' }
    : {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
      }
  return new Intl.DateTimeFormat(toIntlLocale(locale), options).format(
    timestamp * 1000
  )
}

function errorText(
  error: unknown,
  errorMessage: string | undefined,
  fallback: string
) {
  if (errorMessage) return errorMessage
  if (error instanceof Error && error.message) return error.message
  return fallback
}

function incompleteReasons(trend: QuotaHistoryTrend, t: Translate) {
  const reasons: string[] = []
  if (trend.coverage.failedCount > 0) {
    reasons.push(
      t('{{count}} failed sample', { count: trend.coverage.failedCount })
    )
  }
  if (trend.coverage.unsupportedCount > 0) {
    reasons.push(
      t('{{count}} unsupported sample', {
        count: trend.coverage.unsupportedCount,
      })
    )
  }
  if (trend.coverage.invalidCount > 0) {
    reasons.push(
      t('{{count}} invalid sample', { count: trend.coverage.invalidCount })
    )
  }
  if (trend.coverage.interruptedCount > 0) {
    reasons.push(
      t('{{count}} interrupted interval', {
        count: trend.coverage.interruptedCount,
      })
    )
  }
  if (trend.coverage.recoveryCount > 0) {
    reasons.push(
      t('{{count}} recovery event', { count: trend.coverage.recoveryCount })
    )
  }
  if (trend.coverage.resetBoundaries > 0) {
    reasons.push(
      t('{{count}} reset boundary', {
        count: trend.coverage.resetBoundaries,
      })
    )
  }
  if (trend.coverage.truncated) reasons.push(t('Response was truncated'))
  if (trend.coverage.gapCount) {
    reasons.push(
      t('{{count}} sampling gap', { count: trend.coverage.gapCount })
    )
  }
  if (trend.coverage.baselineChangeCount) {
    reasons.push(
      t('{{count}} quota baseline change', {
        count: trend.coverage.baselineChangeCount,
      })
    )
  }
  return reasons
}

function localDateTime(iso: string, seconds = false): string {
  const date = new Date(iso)
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, seconds ? 19 : 16)
}

export function QuotaCustomRangeControls(props: {
  range: QuotaCustomRange
  seconds?: boolean
  onApply: (range: QuotaCustomRange) => void
}) {
  const { t } = useTranslation()
  const [start, setStart] = useState(() =>
    localDateTime(props.range.start, props.seconds)
  )
  const [end, setEnd] = useState(() =>
    localDateTime(props.range.end, props.seconds)
  )
  const startAt = new Date(start).getTime()
  const endAt = new Date(end).getTime()
  const valid =
    Number.isFinite(startAt) &&
    Number.isFinite(endAt) &&
    startAt < endAt &&
    endAt - startAt <= 180 * 86400000
  return (
    <div className='grid grid-cols-1 items-end gap-2 sm:grid-cols-[1fr_1fr_auto]'>
      <label className='grid min-w-0 gap-1 text-xs'>
        <span>{t('Start')}</span>
        <input
          aria-label={t('Custom range start')}
          type='datetime-local'
          step={props.seconds ? 1 : 60}
          className='h-9 min-w-0 rounded-md border bg-transparent px-2'
          value={start}
          onChange={(event) => setStart(event.target.value)}
        />
      </label>
      <label className='grid min-w-0 gap-1 text-xs'>
        <span>{t('End')}</span>
        <input
          aria-label={t('Custom range end')}
          type='datetime-local'
          step={props.seconds ? 1 : 60}
          className='h-9 min-w-0 rounded-md border bg-transparent px-2'
          value={end}
          onChange={(event) => setEnd(event.target.value)}
        />
      </label>
      <Button
        type='button'
        size='sm'
        disabled={!valid}
        onClick={() =>
          props.onApply({
            start: new Date(startAt).toISOString(),
            end: new Date(endAt).toISOString(),
          })
        }
      >
        {t('Apply Filters')}
      </Button>
      {!valid ? (
        <p className='text-destructive col-span-full text-xs'>
          {t('Choose a valid time range of at most 180 days.')}
        </p>
      ) : null}
    </div>
  )
}

function SummaryCards(props: {
  data?: ChannelQuotaHistoryData
  trend: QuotaHistoryTrend
  t: Translate
}) {
  const summary = props.trend.summary
  const consumption = isIntervalMetric(props.trend.metric)
  const cards = consumption
    ? [
        [
          props.t('Observed consumption'),
          formatQuotaValue(
            summary.observedConsumption,
            props.data,
            'consumption',
            props.t
          ),
        ],
        [
          props.t('Average consumption per minute'),
          formatQuotaValue(
            summary.averageRate,
            props.data,
            'rate_per_minute',
            props.t
          ),
        ],
        [
          props.t('Peak consumption per minute'),
          formatQuotaValue(
            summary.peakRate,
            props.data,
            'rate_per_minute',
            props.t
          ),
        ],
        [
          props.t('Valid observed duration'),
          props.t('{{count}} minutes', {
            count: Math.round((summary.observedSeconds / 60) * 10) / 10,
          }),
        ],
      ]
    : [
        [
          props.t('Start'),
          formatQuotaValue(
            summary.start,
            props.data,
            props.trend.metric,
            props.t
          ),
        ],
        [
          summary.isLatestResetSegment
            ? props.t('Change since reset')
            : props.t('Change'),
          formatQuotaValue(
            summary.change,
            props.data,
            props.trend.metric,
            props.t
          ),
        ],
        [
          props.t('Minimum'),
          formatQuotaValue(
            summary.minimum,
            props.data,
            props.trend.metric,
            props.t
          ),
        ],
        [
          props.t('Maximum'),
          formatQuotaValue(
            summary.maximum,
            props.data,
            props.trend.metric,
            props.t
          ),
        ],
      ]

  return (
    <div className='grid grid-cols-2 gap-2 sm:grid-cols-4'>
      {cards.map(([label, value]) => (
        <div key={String(label)} className='bg-muted/40 min-w-0 rounded-lg p-2'>
          <div className='text-muted-foreground truncate text-[11px]'>
            {label}
          </div>
          <div className='font-semibold break-words'>{value}</div>
        </div>
      ))}
    </div>
  )
}

function AnalysisSummary(props: {
  data: ChannelQuotaHistoryData
  method: ChannelQuotaAnalysisMethod
  locale: string | undefined
  t: Translate
}) {
  const analysis = props.data.analysis
  if (!analysis) return null
  const selected = analysis.methods[props.method]
  const etaStatus = quotaETAStatus(selected.eta)
  let etaLabel = props.t('Insufficient data')
  let etaValue = props.t('No depletion forecast available')
  let etaTime: number | null = null
  if (etaStatus === 'depletion') {
    etaLabel = props.t('Estimated depletion')
    etaTime = selected.eta.estimated_depletion_at ?? null
  } else if (etaStatus === 'reset') {
    etaLabel = props.t('Reset before depletion')
    etaTime = selected.eta.reset_at ?? null
  } else if (etaStatus === 'stable') {
    etaLabel = props.t('Stable or no observed consumption')
    etaValue = props.t('Stable')
  }
  if (etaStatus === 'depletion' || etaStatus === 'reset') {
    const timing = quotaPredictionTiming(etaTime, analysis.as_of)
    if (timing.status === 'future') {
      etaValue = `${props.t('Estimated time from analysis point')}: ${formatQuotaDuration(
        timing.seconds,
        props.t
      )}`
    } else if (timing.status === 'passed') {
      etaValue = props.t('Prediction time has passed')
    }
  }

  return (
    <section
      className='grid min-w-0 gap-2 rounded-lg border p-3'
      aria-label={props.t('Consumption rate')}
      data-testid='quota-analysis-summary'
    >
      <div className='flex min-w-0 flex-wrap items-center justify-between gap-2'>
        <span className='text-xs font-medium'>
          {props.t('Consumption rate')} ·{' '}
          {quotaAnalysisMethodLabel(props.method, props.t)}
        </span>
        <Badge variant={analysis.complete ? 'outline' : 'warning'}>
          {props.t('Observed coverage')}: {Math.round(selected.coverage * 100)}%
        </Badge>
      </div>
      <div className='grid grid-cols-1 gap-2 sm:grid-cols-3'>
        <div className='bg-muted/30 rounded-md p-2'>
          <div className='text-muted-foreground text-[11px]'>
            {props.t('Estimated consumption per minute')}
          </div>
          <div className='font-semibold tabular-nums'>
            {formatQuotaRate(
              selected.rate_per_minute,
              props.data,
              'minute',
              props.t,
              props.locale
            )}
          </div>
        </div>
        <div className='bg-muted/30 rounded-md p-2'>
          <div className='text-muted-foreground text-[11px]'>
            {props.t('Estimated consumption per hour')}
          </div>
          <div className='font-semibold tabular-nums'>
            {formatQuotaRate(
              selected.rate_per_hour,
              props.data,
              'hour',
              props.t,
              props.locale
            )}
          </div>
        </div>
        <div className='bg-muted/30 rounded-md p-2'>
          <div className='text-muted-foreground text-[11px]'>{etaLabel}</div>
          <div className='font-semibold'>{etaValue}</div>
          <div className='text-muted-foreground text-[11px] tabular-nums'>
            {etaTime !== null
              ? formatTimestamp(etaTime, props.locale, false)
              : null}
          </div>
        </div>
      </div>
    </section>
  )
}

function QuotaHistoryChart(props: {
  data?: ChannelQuotaHistoryData
  trend: QuotaHistoryTrend
  chartStyle: QuotaHistoryChartStyle
  locale: string | undefined
  metricLabel: string
  t: Translate
}) {
  const chartData =
    props.chartStyle === 'bar'
      ? props.trend.points.filter((point) => point.status !== 'gap')
      : props.trend.points
  const resetPoints = chartData.filter((point) => point.isResetBoundary)
  const commonAxes = (
    <>
      <CartesianGrid vertical={false} strokeDasharray='3 3' />
      <XAxis
        type='number'
        dataKey='timestamp'
        scale='time'
        domain={['dataMin', 'dataMax']}
        tickLine={false}
        axisLine={false}
        minTickGap={42}
        tick={{ fontSize: 10 }}
        tickFormatter={(timestamp: number) =>
          formatTimestamp(timestamp, props.locale, true)
        }
      />
      <YAxis
        tickLine={false}
        axisLine={false}
        width={66}
        tick={{ fontSize: 10 }}
        tickFormatter={(value: number) =>
          new Intl.NumberFormat(toIntlLocale(props.locale), {
            maximumFractionDigits: 2,
          }).format(value)
        }
      />
      <Tooltip
        formatter={(value) => [
          formatQuotaValue(
            Number(value),
            props.data,
            props.trend.metric,
            props.t
          ),
          props.metricLabel,
        ]}
        labelFormatter={(timestamp) =>
          formatTimestamp(Number(timestamp), props.locale, false)
        }
      />
      {resetPoints.map((point) => (
        <ReferenceLine
          key={`${point.timestamp}:${point.resetAt ?? 0}`}
          x={point.timestamp}
          stroke='var(--muted-foreground)'
          strokeDasharray='4 4'
        />
      ))}
    </>
  )
  const margin = { top: 8, right: 8, left: 0, bottom: 4 }

  if (props.chartStyle === 'bar') {
    return (
      <BarChart data={chartData} margin={margin}>
        {commonAxes}
        <Bar
          dataKey='value'
          fill='var(--color-value)'
          radius={[3, 3, 0, 0]}
          isAnimationActive={false}
        />
      </BarChart>
    )
  }
  if (props.chartStyle === 'area') {
    return (
      <AreaChart data={chartData} margin={margin}>
        {commonAxes}
        <Area
          type='linear'
          dataKey='value'
          connectNulls={false}
          stroke='var(--color-value)'
          fill='var(--color-value)'
          fillOpacity={0.16}
          strokeWidth={2}
          dot={chartData.length < 80}
          activeDot={{ r: 4 }}
          isAnimationActive={false}
        />
      </AreaChart>
    )
  }
  return (
    <LineChart data={chartData} margin={margin}>
      {commonAxes}
      <Line
        type='linear'
        dataKey='value'
        connectNulls={false}
        stroke='var(--color-value)'
        strokeWidth={2}
        dot={chartData.length < 80}
        activeDot={{ r: 4 }}
        isAnimationActive={false}
      />
    </LineChart>
  )
}

/**
 * A controlled, presentation-only trend view. Callers own fetching and must
 * pass the same query state to its controls and refresh action.
 */
export function QuotaHistoryTrend(props: QuotaHistoryTrendProps) {
  const { i18n, t } = useTranslation()
  const trend = useMemo(
    () =>
      props.data ? buildQuotaHistoryTrend(props.data, props.metric) : undefined,
    [props.data, props.metric]
  )
  const rangeOptions = props.rangeOptions ?? DEFAULT_RANGE_OPTIONS
  const granularityOptions =
    props.granularityOptions ?? DEFAULT_GRANULARITY_OPTIONS
  const currentMetricLabel = metricLabel(props.metric, t)
  const title = props.title ?? t('Quota trend')
  const hasMetricValues = (trend?.summary.valueCount ?? 0) > 0
  const reasons = trend ? incompleteReasons(trend, t) : []
  const error = props.error ?? props.errorMessage

  return (
    <Card
      className={cn('min-w-0', props.className)}
      data-testid='quota-history-trend'
    >
      <CardHeader className='gap-3 pb-3'>
        <div className='flex min-w-0 flex-wrap items-start justify-between gap-2'>
          <CardTitle className='min-w-0 truncate text-sm'>{title}</CardTitle>
          {props.onRefresh ? (
            <Button
              type='button'
              variant='ghost'
              size='icon-xs'
              onClick={props.onRefresh}
              disabled={props.isLoading}
              aria-label={t('Refresh')}
            >
              <RefreshCw
                className={cn('size-4', props.isLoading && 'animate-spin')}
                aria-hidden='true'
              />
            </Button>
          ) : null}
        </div>
        <div className='grid grid-cols-2 gap-2 sm:flex sm:flex-wrap sm:items-end'>
          <label className='grid min-w-0 gap-1 text-xs'>
            <span className='text-muted-foreground'>{t('Time range')}</span>
            <select
              aria-label={t('Time range')}
              value={props.range}
              onChange={(event) => props.onRangeChange?.(event.target.value)}
              disabled={!props.onRangeChange}
              className='h-8 min-w-0 rounded-lg border bg-transparent px-2 text-sm disabled:cursor-not-allowed disabled:opacity-60'
            >
              {rangeOptions.map((option) => (
                <option key={option} value={option}>
                  {props.rangeOptionLabel?.(option) ??
                    (option === 'custom' ? t('Custom') : option)}
                </option>
              ))}
            </select>
          </label>
          <label className='grid min-w-0 gap-1 text-xs'>
            <span className='text-muted-foreground'>
              {t('Chart granularity')}
            </span>
            <select
              aria-label={t('Chart granularity')}
              value={props.granularity}
              onChange={(event) =>
                props.onGranularityChange?.(event.target.value)
              }
              disabled={!props.onGranularityChange}
              className='h-8 min-w-0 rounded-lg border bg-transparent px-2 text-sm disabled:cursor-not-allowed disabled:opacity-60'
            >
              {granularityOptions.map((option) => (
                <option key={option} value={option}>
                  {granularityLabel(option, t)}
                </option>
              ))}
            </select>
          </label>
          <label className='grid min-w-0 gap-1 text-xs'>
            <span className='text-muted-foreground'>{t('Metric')}</span>
            <select
              aria-label={t('Metric')}
              value={props.metric}
              onChange={(event) =>
                props.onMetricChange?.(event.target.value as QuotaHistoryMetric)
              }
              disabled={!props.onMetricChange}
              className='h-8 min-w-0 rounded-lg border bg-transparent px-2 text-sm disabled:cursor-not-allowed disabled:opacity-60'
            >
              {METRICS.map((metric) => (
                <option key={metric} value={metric}>
                  {metricLabel(metric, t)}
                </option>
              ))}
            </select>
          </label>
          <label className='grid min-w-0 gap-1 text-xs'>
            <span className='text-muted-foreground'>{t('Chart style')}</span>
            <select
              aria-label={t('Chart style')}
              value={props.chartStyle}
              onChange={(event) =>
                props.onChartStyleChange?.(
                  event.target.value as QuotaHistoryChartStyle
                )
              }
              disabled={!props.onChartStyleChange}
              className='h-8 min-w-0 rounded-lg border bg-transparent px-2 text-sm disabled:cursor-not-allowed disabled:opacity-60'
            >
              {CHART_STYLES.map((chartStyle) => (
                <option key={chartStyle} value={chartStyle}>
                  {chartStyleLabel(chartStyle, t)}
                </option>
              ))}
            </select>
          </label>
        </div>
        {props.analysisMethod &&
        props.rateWindowSeconds &&
        props.ewmaHalfLifeSeconds ? (
          <div className='grid grid-cols-1 gap-2 sm:grid-cols-3'>
            <label className='grid min-w-0 gap-1 text-xs'>
              <span className='text-muted-foreground'>
                {t('Consumption rate')}
              </span>
              <select
                aria-label={t('Consumption rate')}
                value={props.analysisMethod}
                onChange={(event) =>
                  props.onAnalysisMethodChange?.(
                    event.target.value as ChannelQuotaAnalysisMethod
                  )
                }
                disabled={!props.onAnalysisMethodChange}
                className='h-8 min-w-0 rounded-lg border bg-transparent px-2 text-sm disabled:cursor-not-allowed disabled:opacity-60'
              >
                {(['latest_interval', 'observed_window', 'ewma'] as const).map(
                  (method) => (
                    <option key={method} value={method}>
                      {quotaAnalysisMethodLabel(method, t)}
                    </option>
                  )
                )}
              </select>
            </label>
            <label className='grid min-w-0 gap-1 text-xs'>
              <span className='text-muted-foreground'>
                {t('Analysis window')}
              </span>
              <select
                aria-label={t('Analysis window')}
                value={props.rateWindowSeconds}
                onChange={(event) =>
                  props.onRateWindowChange?.(Number(event.target.value))
                }
                disabled={!props.onRateWindowChange}
                className='h-8 min-w-0 rounded-lg border bg-transparent px-2 text-sm disabled:cursor-not-allowed disabled:opacity-60'
              >
                {(props.rateWindowOptions ?? [props.rateWindowSeconds]).map(
                  (seconds) => (
                    <option key={seconds} value={seconds}>
                      {formatQuotaDuration(seconds, t)}
                    </option>
                  )
                )}
              </select>
            </label>
            <label className='grid min-w-0 gap-1 text-xs'>
              <span className='text-muted-foreground'>
                {t('EWMA half-life')}
              </span>
              <select
                aria-label={t('EWMA half-life')}
                value={props.ewmaHalfLifeSeconds}
                onChange={(event) =>
                  props.onEWMAHalfLifeChange?.(Number(event.target.value))
                }
                disabled={!props.onEWMAHalfLifeChange}
                className='h-8 min-w-0 rounded-lg border bg-transparent px-2 text-sm disabled:cursor-not-allowed disabled:opacity-60'
              >
                {(props.ewmaHalfLifeOptions ?? [props.ewmaHalfLifeSeconds]).map(
                  (seconds) => (
                    <option key={seconds} value={seconds}>
                      {formatQuotaDuration(seconds, t)}
                    </option>
                  )
                )}
              </select>
            </label>
          </div>
        ) : null}
        {props.range === 'custom' &&
        props.customRange &&
        props.onCustomRangeChange ? (
          <QuotaCustomRangeControls
            range={props.customRange}
            onApply={props.onCustomRangeChange}
          />
        ) : null}
      </CardHeader>
      <CardContent className='min-w-0 space-y-3'>
        {props.isLoading ? <Skeleton className='h-72 w-full' /> : null}
        {!props.isLoading && error ? (
          <Alert variant='destructive'>
            <AlertTitle>{t('Unable to load quota history')}</AlertTitle>
            <AlertDescription>
              <p>
                {t(
                  'Try a shorter time range or refresh to retry. Existing history is retained.'
                )}
              </p>
              <details>
                <summary className='cursor-pointer'>{t('Details')}</summary>
                {errorText(
                  error,
                  props.errorMessage,
                  t('Please try again later.')
                )}
              </details>
            </AlertDescription>
          </Alert>
        ) : null}
        {!props.isLoading && !props.data && !error ? (
          <p className='text-muted-foreground rounded-lg border border-dashed p-5 text-center text-sm'>
            {t('No quota history data yet')}
          </p>
        ) : null}
        {!props.isLoading && trend ? (
          <>
            <div className='bg-muted/30 flex min-w-0 flex-wrap items-center justify-between gap-2 rounded-lg border p-2 text-xs'>
              <div className='flex min-w-0 flex-wrap items-center gap-2'>
                <span className='font-medium'>{t('Latest raw sample')}</span>
                <Badge variant={statusVariant(trend.latest.status)}>
                  {statusLabel(trend.latest.status, t)}
                </Badge>
                <span className='text-muted-foreground'>
                  {formatTimestamp(
                    trend.latest.observedAt,
                    i18n.language,
                    false
                  )}
                </span>
              </div>
              {!isIntervalMetric(props.metric) ? (
                <span className='font-medium'>
                  {currentMetricLabel}:{' '}
                  {formatQuotaValue(
                    trend.latest.value,
                    props.data,
                    props.metric,
                    t
                  )}
                </span>
              ) : null}
            </div>
            {trend.hasIncompleteData ? (
              <Alert>
                <AlertTitle>{t('Incomplete quota history')}</AlertTitle>
                <AlertDescription className='space-y-1'>
                  <p>
                    {t(
                      'Failed or discontinuous samples remain visible as chart gaps.'
                    )}
                  </p>
                  {reasons.length > 0 ? <p>{reasons.join(' · ')}</p> : null}
                </AlertDescription>
              </Alert>
            ) : null}
            {trend.summary.isLatestResetSegment ? (
              <p className='text-muted-foreground text-xs'>
                {t('Summary values start after the latest reset boundary.')}
              </p>
            ) : null}
            <SummaryCards data={props.data} trend={trend} t={t} />
            {props.data && props.analysisMethod ? (
              <AnalysisSummary
                data={props.data}
                method={props.analysisMethod}
                locale={i18n.language}
                t={t}
              />
            ) : null}
            {isIntervalMetric(props.metric) ? (
              <p className='text-muted-foreground text-xs leading-5'>
                {t(
                  'Consumption is estimated from consecutive valid samples using their actual elapsed time. Each interval belongs to its ending bucket; missing minutes are not reconstructed.'
                )}
                {trend.summary.peakObservedAt ? (
                  <>
                    {' '}
                    {t('Peak observed at')}:{' '}
                    {formatTimestamp(
                      trend.summary.peakObservedAt,
                      i18n.language,
                      false
                    )}
                  </>
                ) : null}
              </p>
            ) : null}
            {!hasMetricValues ? (
              <Alert>
                <AlertTitle>
                  {trend.points.length
                    ? t('Selected metric is unavailable')
                    : t('No quota history data yet')}
                </AlertTitle>
                <AlertDescription>
                  {isIntervalMetric(props.metric)
                    ? t(
                        'At least two consecutive valid samples are required to estimate consumption.'
                      )
                    : t(
                        'The upstream did not return a usable value for this metric.'
                      )}
                </AlertDescription>
              </Alert>
            ) : (
              <div
                className='h-72 w-full min-w-0 touch-pan-y'
                aria-label={t('Quota history chart')}
                data-testid={`quota-history-chart-${props.chartStyle}`}
              >
                <ChartContainer
                  className='aspect-auto h-full w-full'
                  config={{
                    value: {
                      label: currentMetricLabel,
                      color: 'var(--chart-1)',
                    },
                  }}
                  initialDimension={{ width: 320, height: 288 }}
                >
                  <QuotaHistoryChart
                    data={props.data}
                    trend={trend}
                    chartStyle={props.chartStyle}
                    locale={i18n.language}
                    metricLabel={currentMetricLabel}
                    t={t}
                  />
                </ChartContainer>
              </div>
            )}
            <div className='text-muted-foreground grid gap-1 text-xs sm:grid-cols-3 sm:gap-2'>
              <span>
                {t('Last plotted value')}:{' '}
                {formatQuotaValue(
                  trend.latestPlotted?.value,
                  props.data,
                  props.metric,
                  t
                )}
              </span>
              <span>
                {t('Samples')}: {trend.coverage.successCount}
              </span>
              <span>
                {t('Observed coverage')}:{' '}
                {formatTimestamp(
                  trend.coverage.observedStart,
                  i18n.language,
                  true
                )}{' '}
                –{' '}
                {formatTimestamp(
                  trend.coverage.observedEnd,
                  i18n.language,
                  true
                )}
              </span>
            </div>
          </>
        ) : null}
      </CardContent>
    </Card>
  )
}
