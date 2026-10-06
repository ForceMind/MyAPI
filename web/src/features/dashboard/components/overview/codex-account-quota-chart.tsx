/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { ChevronDown } from 'lucide-react'
import { useEffect, useMemo, useRef, useState, type PointerEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  quotaSeriesKey,
  quotaWindowLabel,
} from '@/features/channels/lib/quota-history'
import type {
  ChannelQuotaChangeItem,
  ChannelQuotaOverviewPoint,
} from '@/features/channels/types'
import { toIntlLocale } from '@/i18n/languages'
import { cn } from '@/lib/utils'

import { QuotaOverviewCard } from './quota-overview-card'

const WIDTH = 960
const HEIGHT = 240
const LEFT = 42
const TOP = 12
const PLOT_HEIGHT = HEIGHT - TOP - 30
const COLORS = [
  'var(--chart-1)',
  'var(--chart-2)',
  'var(--chart-3)',
  'var(--chart-4)',
  'var(--chart-5)',
]
const DASH_PATTERNS = ['6 3', '3 3', '9 3 2 3']

function QuotaSeriesSwatch(props: { color: string; dashPattern?: string }) {
  return (
    <svg className='h-3 w-6 shrink-0' viewBox='0 0 24 12' aria-hidden='true'>
      <line
        x1='0'
        x2='24'
        y1='6'
        y2='6'
        stroke={props.color}
        strokeWidth='1.5'
        strokeDasharray={props.dashPattern}
        vectorEffect='non-scaling-stroke'
      />
    </svg>
  )
}

function continuousSegments(points: ChannelQuotaOverviewPoint[]) {
  const segments: ChannelQuotaOverviewPoint[][] = []
  let current: ChannelQuotaOverviewPoint[] = []
  for (const point of points) {
    if (
      point.continuity_break ||
      point.available == null ||
      !Number.isFinite(point.available)
    ) {
      if (current.length > 0) segments.push(current)
      current = []
    }
    if (
      typeof point.available === 'number' &&
      Number.isFinite(point.available)
    ) {
      current.push(point)
    }
  }
  if (current.length > 0) segments.push(current)
  return segments
}

function defaultVisibleSeriesKeys(
  series: { key: string; channelId: number }[]
): string[] {
  const defaults: string[] = []
  const selected = new Set<string>()
  const seenChannels = new Set<number>()
  for (const item of series) {
    if (seenChannels.has(item.channelId)) continue
    seenChannels.add(item.channelId)
    defaults.push(item.key)
    selected.add(item.key)
    if (defaults.length === 8) return defaults
  }
  for (const item of series) {
    if (selected.has(item.key)) continue
    defaults.push(item.key)
    if (defaults.length === 8) break
  }
  return defaults
}

function visibleSeriesKeys(
  visibilityOverrides: Map<string, boolean>,
  availableKeys: string[],
  defaults: string[]
) {
  const defaultKeys = new Set(defaults)
  return availableKeys.filter(
    (key) => visibilityOverrides.get(key) ?? defaultKeys.has(key)
  )
}

/** Shows comparable percentage series on one time axis without adding balances. */
export function CodexAccountQuotaChart(props: {
  items: ChannelQuotaChangeItem[]
}) {
  const { i18n, t } = useTranslation()
  const [visibilityOverrides, setVisibilityOverrides] = useState<
    Map<string, boolean>
  >(() => new Map())
  const [detailsExpanded, setDetailsExpanded] = useState(false)
  const [allSeriesExpanded, setAllSeriesExpanded] = useState(false)
  const [cursorTimestamp, setCursorTimestamp] = useState<number | null>(null)
  const plotContainerRef = useRef<HTMLDivElement>(null)
  const [plotWidth, setPlotWidth] = useState(WIDTH)
  useEffect(() => {
    const container = plotContainerRef.current
    if (!container) return
    const updateWidth = () => {
      if (container.clientWidth > 0) setPlotWidth(container.clientWidth)
    }
    updateWidth()
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(updateWidth)
    observer.observe(container)
    return () => observer.disconnect()
  }, [])
  const hasUnidentifiedHistory = props.items.some(
    (item) =>
      item.unit === 'percent' &&
      (!item.series_id || quotaSeriesKey(item) !== item.series_id)
  )
  const series = useMemo(() => {
    const candidates = props.items
      .filter(
        (item) =>
          item.unit === 'percent' &&
          item.series_id &&
          quotaSeriesKey(item) === item.series_id &&
          item.overview_points?.some(
            (point) =>
              typeof point.available === 'number' &&
              Number.isFinite(point.available)
          )
      )
      .map((item, index) => {
        const key = quotaSeriesKey(item)
        const accountLabel =
          item.account_label && item.account_label !== item.name
            ? item.account_label
            : ''
        const windowLabel = quotaWindowLabel(item.window_type, t)
        return {
          key,
          channelId: item.channel_id,
          seriesId: key === item.series_id ? key : '',
          name: item.name,
          accountLabel,
          windowLabel,
          baseLabel: [item.name, accountLabel, windowLabel]
            .filter(Boolean)
            .join(' · '),
          available: item.status === 'success' ? item.current_available : null,
          points: (item.overview_points ?? [])
            .filter((point) => Number.isFinite(point.timestamp))
            .sort((a, b) => a.timestamp - b.timestamp),
          color: COLORS[index % COLORS.length],
          dashPattern:
            index >= COLORS.length
              ? DASH_PATTERNS[
                  (Math.floor(index / COLORS.length) - 1) % DASH_PATTERNS.length
                ]
              : undefined,
        }
      })
    const labelCounts = new Map<string, number>()
    const channelWindowCounts = new Map<string, number>()
    for (const item of candidates) {
      labelCounts.set(
        item.baseLabel,
        (labelCounts.get(item.baseLabel) ?? 0) + 1
      )
      const channelWindow = `${item.channelId}:${item.windowLabel}`
      channelWindowCounts.set(
        channelWindow,
        (channelWindowCounts.get(channelWindow) ?? 0) + 1
      )
    }
    return candidates.map((item) => {
      const disambiguation =
        (labelCounts.get(item.baseLabel) ?? 0) > 1 && item.seriesId
          ? t('Account {{id}}', { id: item.seriesId.slice(0, 8) })
          : ''
      const sameChannelWindow =
        (channelWindowCounts.get(`${item.channelId}:${item.windowLabel}`) ??
          0) > 1
      return {
        ...item,
        label: [item.baseLabel, disambiguation].filter(Boolean).join(' · '),
        legendSecondary: (sameChannelWindow
          ? [disambiguation, item.accountLabel, item.windowLabel]
          : [item.windowLabel, item.accountLabel, disambiguation]
        )
          .filter(Boolean)
          .join(' · '),
      }
    })
  }, [props.items, t])
  const availableKeys = series.map((item) => item.key)
  const defaultKeys = defaultVisibleSeriesKeys(series)
  const activeKeys = visibleSeriesKeys(
    visibilityOverrides,
    availableKeys,
    defaultKeys
  )
  const visible = series.filter((item) => activeKeys.includes(item.key))
  const previewKeys = new Set([...defaultKeys, ...activeKeys])
  const previewSeries = series.filter((item) => previewKeys.has(item.key))
  const hasExtraSeries = series.length > previewSeries.length
  const expandedLegend = allSeriesExpanded && hasExtraSeries
  const legendSeries = expandedLegend ? series : previewSeries
  const detailsVisible = series.length === 0 || detailsExpanded
  const timestamps = visible.flatMap((item) =>
    item.points.map((point) => point.timestamp)
  )
  const rangeTimestamps = series.flatMap((item) =>
    item.points.map((point) => point.timestamp)
  )
  const start = rangeTimestamps.length > 0 ? Math.min(...rangeTimestamps) : 0
  const end = rangeTimestamps.length > 0 ? Math.max(...rangeTimestamps) : 0
  const selectedTime =
    cursorTimestamp === null || timestamps.length === 0
      ? null
      : Math.max(start, Math.min(end, cursorTimestamp))
  const x = (timestamp: number) =>
    LEFT +
    (end > start
      ? ((timestamp - start) / (end - start)) * (plotWidth - LEFT - 12)
      : (plotWidth - LEFT - 12) / 2)
  const y = (available: number) =>
    TOP + ((100 - Math.max(0, Math.min(100, available))) / 100) * PLOT_HEIGHT
  const timeLabel = (timestamp: number) =>
    new Intl.DateTimeFormat(toIntlLocale(i18n.language), {
      month: 'numeric',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    }).format(timestamp * 1000)
  const comparison =
    selectedTime === null
      ? []
      : visible.map((item) => {
          let observed: ChannelQuotaOverviewPoint | null = null
          for (const point of item.points) {
            if (point.timestamp > selectedTime) break
            if (point.continuity_break) observed = null
            observed =
              typeof point.available === 'number' &&
              Number.isFinite(point.available)
                ? point
                : null
          }
          return { ...item, observed }
        })
  const detailEntries: { key: string; item: ChannelQuotaChangeItem }[] = []
  const seenDetailKeys = new Set<string>()
  for (const item of props.items.slice(0, 4)) {
    if (
      item.unit === 'percent' &&
      (!item.series_id || quotaSeriesKey(item) !== item.series_id)
    ) {
      continue
    }
    const key =
      item.series_id && quotaSeriesKey(item) === item.series_id
        ? item.series_id
        : [
            quotaSeriesKey(item),
            item.observed_at ?? '',
            item.current_available ?? '',
            item.overview_points
              ?.map((point) => `${point.timestamp}:${point.available ?? ''}`)
              .join(',') ?? '',
          ].join(':')
    if (seenDetailKeys.has(key)) continue
    seenDetailKeys.add(key)
    detailEntries.push({ key, item })
  }
  const selectPointerTime = (event: PointerEvent<SVGSVGElement>) => {
    const bounds = event.currentTarget.getBoundingClientRect()
    if (bounds.width <= 0 || timestamps.length === 0) return
    const pointerX = ((event.clientX - bounds.left) / bounds.width) * plotWidth
    const fraction = Math.max(
      0,
      Math.min(1, (pointerX - LEFT) / (plotWidth - LEFT - 12))
    )
    setCursorTimestamp(Math.round(start + fraction * (end - start)))
  }

  if (props.items.length === 0) return null

  return (
    <div className='grid min-w-0 gap-4' data-testid='codex-account-quota-chart'>
      {hasUnidentifiedHistory && (
        <Alert>
          <AlertTitle>{t('Unidentified quota history')}</AlertTitle>
          <AlertDescription>
            {t(
              'Some observations have no stable account identity and are not connected into a trend.'
            )}
          </AlertDescription>
        </Alert>
      )}
      {series.length > 0 && (
        <section
          className='min-w-0 rounded-xl border p-3 sm:p-4'
          aria-label={t('Remaining quota')}
        >
          <div className='mb-3 flex flex-wrap items-center justify-between gap-2'>
            <h3 className='text-sm font-semibold'>{t('Remaining quota')}</h3>
            <span className='text-muted-foreground text-xs'>
              {t('Last 24 hours')} · %
            </span>
          </div>
          <div ref={plotContainerRef} className='min-w-0'>
            <svg
              role='img'
              aria-label={t('Remaining quota')}
              tabIndex={0}
              viewBox={`0 0 ${plotWidth} ${HEIGHT}`}
              className='h-56 w-full'
              preserveAspectRatio='none'
              data-testid='quota-comparison-chart'
              onFocus={() => setCursorTimestamp((current) => current ?? end)}
              onPointerMove={selectPointerTime}
              onPointerDown={selectPointerTime}
              onKeyDown={(event) => {
                if (timestamps.length === 0) return
                const step = Math.max(1, Math.round((end - start) / 24))
                if (event.key === 'Home') {
                  event.preventDefault()
                  setCursorTimestamp(start)
                } else if (event.key === 'End') {
                  event.preventDefault()
                  setCursorTimestamp(end)
                } else if (event.key === 'ArrowLeft') {
                  event.preventDefault()
                  setCursorTimestamp((current) =>
                    Math.max(start, (current ?? end) - step)
                  )
                } else if (event.key === 'ArrowRight') {
                  event.preventDefault()
                  setCursorTimestamp((current) =>
                    Math.min(end, (current ?? start) + step)
                  )
                }
              }}
            >
              {[0, 25, 50, 75, 100].map((tick) => (
                <g key={tick}>
                  <line
                    x1={LEFT}
                    x2={plotWidth - 12}
                    y1={y(tick)}
                    y2={y(tick)}
                    stroke='var(--border)'
                    strokeDasharray='3 4'
                    vectorEffect='non-scaling-stroke'
                  />
                  <text
                    x='2'
                    y={y(tick) + 4}
                    fill='var(--muted-foreground)'
                    fontSize='11'
                  >
                    {tick}%
                  </text>
                </g>
              ))}
              {timestamps.length > 0 && (
                <>
                  <text
                    x={LEFT}
                    y={HEIGHT - 5}
                    fill='var(--muted-foreground)'
                    fontSize='11'
                  >
                    {timeLabel(start)}
                  </text>
                  <text
                    x={plotWidth - 12}
                    y={HEIGHT - 5}
                    textAnchor='end'
                    fill='var(--muted-foreground)'
                    fontSize='11'
                  >
                    {timeLabel(end)}
                  </text>
                </>
              )}
              {selectedTime !== null && (
                <line
                  data-testid='quota-comparison-cursor'
                  x1={x(selectedTime)}
                  x2={x(selectedTime)}
                  y1={TOP}
                  y2={TOP + PLOT_HEIGHT}
                  stroke='var(--foreground)'
                  strokeDasharray='4 4'
                  vectorEffect='non-scaling-stroke'
                />
              )}
              {visible.flatMap((item) =>
                continuousSegments(item.points).map((segment) => {
                  const segmentKey = `${item.key}:${segment.map((point) => `${point.timestamp}:${point.available}`).join('|')}`
                  if (segment.length === 1) {
                    return (
                      <circle
                        key={segmentKey}
                        data-testid='quota-comparison-point'
                        data-series-key={item.key}
                        cx={x(segment[0].timestamp)}
                        cy={y(segment[0].available ?? 0)}
                        r='3'
                        fill={item.color}
                      />
                    )
                  }
                  return (
                    <polyline
                      key={segmentKey}
                      data-testid='quota-comparison-line'
                      data-series-key={item.key}
                      points={segment
                        .map(
                          (point) =>
                            `${x(point.timestamp)},${y(point.available ?? 0)}`
                        )
                        .join(' ')}
                      fill='none'
                      stroke={item.color}
                      strokeDasharray={item.dashPattern}
                      strokeWidth='1.5'
                      strokeLinejoin='round'
                      strokeLinecap='round'
                      vectorEffect='non-scaling-stroke'
                    />
                  )
                })
              )}
            </svg>
          </div>
          <p className='text-muted-foreground mt-2 text-xs'>
            {t('Move across the chart or use arrow keys to compare quota.')}
          </p>
          {selectedTime !== null && comparison.length > 0 && (
            <div
              className='bg-muted/40 mt-2 rounded-lg p-3 text-xs'
              data-testid='quota-comparison-readout'
            >
              <p className='font-medium'>
                {t('Compare quota at {{time}}', {
                  time: timeLabel(selectedTime),
                })}
              </p>
              <ul className='mt-2 grid gap-2 sm:grid-cols-2'>
                {comparison.map((item) => (
                  <li key={item.key} className='flex min-w-0 gap-2'>
                    <QuotaSeriesSwatch
                      color={item.color}
                      dashPattern={item.dashPattern}
                    />
                    <span className='min-w-0'>
                      <span className='block font-medium break-words'>
                        {item.label}
                      </span>
                      {item.observed ? (
                        <span className='text-muted-foreground'>
                          <span>{item.observed.available?.toFixed(1)}%</span> ·{' '}
                          <span>
                            {t('Observed at {{time}}', {
                              time: timeLabel(item.observed.timestamp),
                            })}
                          </span>
                        </span>
                      ) : (
                        <span className='text-muted-foreground'>
                          {t('No data')}
                        </span>
                      )}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          )}
          <div
            id='quota-comparison-legend'
            className='mt-2 grid min-w-0 grid-cols-1 gap-1.5 lg:grid-cols-2'
            data-testid='quota-comparison-legend'
          >
            {legendSeries.map((item) => {
              const shown = activeKeys.includes(item.key)
              return (
                <Button
                  key={item.key}
                  type='button'
                  variant={shown ? 'outline' : 'ghost'}
                  size='xs'
                  className='h-auto w-full min-w-0 items-center justify-start px-2 py-1.5 text-left'
                  aria-pressed={shown}
                  aria-label={
                    typeof item.available === 'number' &&
                    Number.isFinite(item.available)
                      ? `${item.label} · ${item.available.toFixed(1)}%`
                      : item.label
                  }
                  title={item.label}
                  onClick={() =>
                    setVisibilityOverrides((current) => {
                      const next = new Map(current)
                      const defaultVisible = defaultKeys.includes(item.key)
                      next.set(
                        item.key,
                        !(current.get(item.key) ?? defaultVisible)
                      )
                      return next
                    })
                  }
                >
                  <QuotaSeriesSwatch
                    color={item.color}
                    dashPattern={item.dashPattern}
                  />
                  <span className='min-w-0 flex-1'>
                    <span className='block truncate font-medium'>
                      {item.name}
                    </span>
                    <span className='text-muted-foreground block truncate text-xs'>
                      {item.legendSecondary}
                    </span>
                  </span>
                  {typeof item.available === 'number' &&
                  Number.isFinite(item.available) ? (
                    <span className='shrink-0 tabular-nums'>
                      {item.available.toFixed(1)}%
                    </span>
                  ) : null}
                </Button>
              )
            })}
          </div>
          {hasExtraSeries && (
            <div className='mt-1 flex justify-end'>
              <Button
                type='button'
                variant='ghost'
                size='sm'
                aria-controls='quota-comparison-legend'
                aria-expanded={expandedLegend}
                onClick={() => setAllSeriesExpanded((current) => !current)}
              >
                {expandedLegend
                  ? t('Show fewer quota series')
                  : t('Show all quota series')}
              </Button>
            </div>
          )}
        </section>
      )}
      {series.length > 0 && (
        <div className='flex justify-end'>
          <Button
            type='button'
            variant='ghost'
            size='sm'
            aria-expanded={detailsExpanded}
            aria-controls='quota-overview-details'
            onClick={() => setDetailsExpanded((current) => !current)}
          >
            {detailsExpanded ? t('Hide details') : t('Show details')}
            <ChevronDown
              className={cn(
                'size-4 transition-transform',
                detailsExpanded && 'rotate-180'
              )}
              aria-hidden='true'
            />
          </Button>
        </div>
      )}
      <div
        id='quota-overview-details'
        className={cn(
          'grid min-w-0 grid-cols-1 gap-3 lg:grid-cols-2',
          !detailsVisible && 'hidden'
        )}
      >
        {detailsVisible &&
          detailEntries.map((entry) => (
            <QuotaOverviewCard key={entry.key} item={entry.item} />
          ))}
      </div>
    </div>
  )
}
