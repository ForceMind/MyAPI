/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQueries, useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { useAuthStore } from '@/stores/auth-store'

import { getChannelQuotaChanges, getChannelQuotaHistory } from '../api'
import {
  quotaComparisonGroup,
  weeklyQuotaCycle,
  zoomQuotaTime,
  type QuotaTimeBounds,
} from '../lib/quota-comparison'
import {
  getChannelQuotaSeriesColor,
  quotaSeriesKey,
  quotaWindowLabel,
} from '../lib/quota-history'
import {
  QuotaComparisonChart,
  type ComparisonChartStyle,
  type ComparisonMetric,
  type ComparisonSeries,
} from './quota-comparison-chart'
import { QuotaCustomRangeControls } from './quota-history-trend'

const selectClass = 'h-9 min-w-0 rounded-lg border bg-background px-2 text-sm'

export type ChannelQuotaComparisonProps = {
  initialChannelId?: number
  initialBounds?: QuotaTimeBounds
}

function isInitialQuotaBounds(
  bounds: QuotaTimeBounds | undefined
): bounds is QuotaTimeBounds {
  return (
    !!bounds &&
    Number.isSafeInteger(bounds.start) &&
    Number.isSafeInteger(bounds.end) &&
    bounds.start > 0 &&
    bounds.end > bounds.start
  )
}

export function ChannelQuotaComparison(props: ChannelQuotaComparisonProps) {
  const { t } = useTranslation()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const sessionId = useAuthStore((state) => state.auth.session?.sid)
  const initialBounds = isInitialQuotaBounds(props.initialBounds)
    ? props.initialBounds
    : undefined
  const [bounds, setBounds] = useState<QuotaTimeBounds>(() => {
    if (initialBounds) return initialBounds
    const end = Math.floor(Date.now() / 1000)
    return { start: end - 86400, end }
  })
  const [mode, setMode] = useState(initialBounds ? 'custom' : '24h')
  const [metric, setMetric] = useState<ComparisonMetric>('available')
  const [style, setStyle] = useState<ComparisonChartStyle>('line')
  const [group, setGroup] = useState('')
  const [selection, setSelection] = useState<string[] | null>(null)
  const [granularity, setGranularity] = useState<'raw' | 'auto'>('raw')
  const [cycleAnchor, setCycleAnchor] = useState('')
  const [manualAnchor, setManualAnchor] = useState(false)
  const [cycleOffset, setCycleOffset] = useState(0)
  const [cycleError, setCycleError] = useState(false)
  const [refresh, setRefresh] = useState(0)
  const params = useMemo(
    () => ({
      range: 'custom' as const,
      start: new Date(bounds.start * 1000).toISOString(),
      end: new Date(bounds.end * 1000).toISOString(),
    }),
    [bounds]
  )
  const catalogue = useQuery({
    queryKey: ['quota-comparison-catalogue', userId, sessionId, refresh],
    queryFn: () => getChannelQuotaChanges({ catalogue: true, limit: 2000 }),
    staleTime: 60000,
    retry: false,
  })
  const items = useMemo(
    () => catalogue.data?.data?.items ?? [],
    [catalogue.data?.data?.items]
  )
  const groups = useMemo(
    () => [
      ...new Map(
        items.map((item) => [quotaComparisonGroup(item), item])
      ).entries(),
    ],
    [items]
  )
  const focused = items.find(
    (item) => item.channel_id === props.initialChannelId
  )
  let activeGroup =
    groups.find(([, item]) => item.window_type === 'weekly')?.[0] ??
    groups[0]?.[0]
  if (focused) activeGroup = quotaComparisonGroup(focused)
  if (groups.some(([key]) => key === group)) activeGroup = group
  const candidates = items
    .filter((item) => quotaComparisonGroup(item) === activeGroup)
    .sort(
      (a, b) =>
        a.channel_id - b.channel_id ||
        quotaSeriesKey(a).localeCompare(quotaSeriesKey(b))
    )
  const selected =
    selection ??
    [...candidates]
      .sort(
        (a, b) =>
          Number(b.channel_id === props.initialChannelId) -
          Number(a.channel_id === props.initialChannelId)
      )
      .slice(0, 8)
      .map(quotaSeriesKey)
  const chosen = candidates.filter((item) =>
    selected.includes(quotaSeriesKey(item))
  )
  const queries = useQueries({
    queries: chosen.map((item) => ({
      queryKey: [
        'quota-comparison-history',
        userId,
        sessionId,
        quotaSeriesKey(item),
        params,
        granularity,
        refresh,
      ],
      queryFn: () =>
        getChannelQuotaHistory(item.channel_id, {
          ...params,
          consumption_basis: 'available',
          exact_identity: true,
          metric_type: item.metric_type,
          source: item.source,
          window_type: item.window_type,
          plan_type: item.plan_type,
          unit: item.unit,
          currency: item.currency,
          window_seconds: item.window_seconds,
          granularity,
          timezone_offset: -new Date().getTimezoneOffset(),
          limit: 5000,
        }),
      staleTime: 60000,
      retry: false,
    })),
  })
  const chartSeries: ComparisonSeries[] = chosen.flatMap((item, index) => {
    const result = queries[index].data
    if (!result?.success || !result.data) return []
    return [
      {
        key: quotaSeriesKey(item),
        label: `${item.account_label || item.name} · #${item.channel_id}${candidates.filter((peer) => peer.channel_id === item.channel_id).length > 1 ? ` · ${item.plan_type || ''} / ${item.source || ''}` : ''}`,
        data: result.data,
        color: getChannelQuotaSeriesColor(item.channel_id),
      },
    ]
  })
  const firstHistory = queries[0]?.data
  const firstIsWeekly =
    chosen[0]?.window_type === 'weekly' || chosen[0]?.window_seconds === 604800
  const sourceReset =
    firstIsWeekly && firstHistory?.success
      ? firstHistory.data?.current?.reset_at
      : undefined
  const anchor = cycleAnchor
    ? new Date(cycleAnchor).getTime() / 1000
    : sourceReset
  const applyCycle = (offset: number) => {
    const next = weeklyQuotaCycle(
      anchor ?? Number.NaN,
      Math.floor(Date.now() / 1000),
      offset
    )
    setMode('cycle')
    setCycleError(!next)
    if (next) {
      if (!cycleAnchor && anchor) {
        const date = new Date(anchor * 1000)
        setCycleAnchor(
          new Date(date.getTime() - date.getTimezoneOffset() * 60000)
            .toISOString()
            .slice(0, 19)
        )
      }
      setBounds(next)
      setCycleOffset(offset)
    }
  }
  const zoom = (next: QuotaTimeBounds) => {
    setBounds(next)
    setMode('custom')
    if (!manualAnchor) setCycleAnchor('')
    setCycleError(false)
  }
  const catalogueIncomplete =
    catalogue.data?.data?.source_complete === false ||
    catalogue.data?.data?.items_complete === false
  const incomplete = chartSeries.some(
    (item) =>
      item.data.complete === false ||
      item.data.truncated ||
      item.data.source_complete === false ||
      item.data.points_complete === false
  )
  const loading =
    catalogue.isFetching || queries.some((query) => query.isFetching)
  const failed =
    catalogue.isError ||
    catalogue.data?.success === false ||
    queries.some((query) => query.isError || query.data?.success === false)
  const hasSamples = chartSeries.some((item) =>
    item.data.points.some((point) =>
      Number.isFinite(
        metric === 'available' ? point.available : point.consumption
      )
    )
  )
  let emptyLabel = t('No quota samples in this time range.')
  if (!items.length) emptyLabel = t('No quota history data yet')
  else if (!chosen.length) emptyLabel = t('Select channels to compare.')
  else if (failed) emptyLabel = t('Please try again.')
  const resetView = () => {
    const end = Math.floor(Date.now() / 1000)
    setBounds({ start: end - 86400, end })
    setMode('24h')
    setCycleError(false)
    if (!manualAnchor) setCycleAnchor('')
    setRefresh((value) => value + 1)
  }
  const timeLabel = (value: number) =>
    new Date(value * 1000).toLocaleString(undefined, {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })

  return (
    <section className='grid min-w-0 gap-3' aria-label={t('Quota analysis')}>
      <div className='grid grid-cols-2 gap-x-3 gap-y-2 sm:grid-cols-4'>
        <Label className='text-muted-foreground grid gap-1 text-xs'>
          {t('Metric')}
          <select
            aria-label={t('Metric')}
            className={selectClass}
            value={metric}
            onChange={(event) =>
              setMetric(event.target.value as ComparisonMetric)
            }
          >
            <option value='available'>{t('Available quota')}</option>
            <option value='consumption'>{t('Estimated usage')}</option>
          </select>
        </Label>
        <Label className='text-muted-foreground grid gap-1 text-xs'>
          {t('Chart type')}
          <select
            aria-label={t('Chart type')}
            className={selectClass}
            value={style}
            onChange={(event) =>
              setStyle(event.target.value as ComparisonChartStyle)
            }
          >
            <option value='line'>{t('Line')}</option>
            <option value='bar'>{t('Bar')}</option>
            <option value='scatter'>{t('Scatter')}</option>
            <option value='area'>{t('Area')}</option>
          </select>
        </Label>
        <Label className='text-muted-foreground grid gap-1 text-xs'>
          {t('Time range')}
          <select
            aria-label={t('Time range')}
            className={selectClass}
            value={mode}
            onChange={(event) => {
              const value = event.target.value
              if (value === 'cycle') {
                applyCycle(0)
                return
              }
              setMode(value)
              if (!manualAnchor) setCycleAnchor('')
              setCycleError(false)
              if (value !== 'custom') {
                const end = Math.floor(Date.now() / 1000)
                setBounds({
                  start:
                    end -
                    ({ '1h': 3600, '24h': 86400, '7d': 604800, '30d': 2592000 }[
                      value
                    ] ?? 86400),
                  end,
                })
              }
            }}
          >
            <option value='1h'>1h</option>
            <option value='24h'>24h</option>
            <option value='7d'>7d</option>
            <option value='30d'>30d</option>
            <option value='cycle' disabled={!firstIsWeekly}>
              {t('Weekly reset cycle')}
            </option>
            <option value='custom'>{t('Custom')}</option>
          </select>
        </Label>
        <Label className='text-muted-foreground grid gap-1 text-xs'>
          {t('Quota window type')}
          <select
            aria-label={t('Quota window type')}
            className={selectClass}
            value={activeGroup ?? ''}
            onChange={(event) => {
              setGroup(event.target.value)
              setSelection(null)
              setCycleAnchor('')
              setManualAnchor(false)
              setCycleError(false)
              if (mode === 'cycle') setMode('custom')
            }}
          >
            {groups.map(([key, item]) => (
              <option key={key} value={key}>
                {quotaWindowLabel(item.window_type, t)} ·{' '}
                {item.unit === 'percent'
                  ? '%'
                  : item.currency || item.unit || t('Unknown')}
              </option>
            ))}
          </select>
        </Label>
      </div>
      <fieldset className='min-w-0'>
        <legend className='sr-only'>{t('Channels to compare')}</legend>
        <div className='flex max-h-20 flex-wrap gap-2 overflow-y-auto'>
          {candidates.map((item) => {
            const key = quotaSeriesKey(item)
            const checked = selected.includes(key)
            return (
              <label
                key={key}
                className='has-[:checked]:border-primary/40 has-[:checked]:bg-primary/5 flex max-w-full min-w-0 items-center gap-2 rounded-md border px-2 py-1 text-xs'
              >
                <input
                  type='checkbox'
                  checked={checked}
                  disabled={!checked && chosen.length >= 8}
                  onChange={() => {
                    const next = checked
                      ? selected.filter((value) => value !== key)
                      : [...selected, key]
                    const first = candidates.find((candidate) =>
                      next.includes(quotaSeriesKey(candidate))
                    )
                    if (
                      (first ? quotaSeriesKey(first) : undefined) !==
                      (chosen[0] ? quotaSeriesKey(chosen[0]) : undefined)
                    ) {
                      setCycleAnchor('')
                      setManualAnchor(false)
                      if (mode === 'cycle') setMode('custom')
                    }
                    setSelection(next)
                  }}
                />
                <span className='truncate'>
                  {item.account_label || item.name} · #{item.channel_id}
                  {candidates.filter(
                    (peer) => peer.channel_id === item.channel_id
                  ).length > 1
                    ? ` · ${item.plan_type || ''} / ${item.source || ''}`
                    : ''}
                </span>
              </label>
            )
          })}
        </div>
      </fieldset>
      {failed ? (
        <p role='alert' className='text-destructive text-sm'>
          {t(
            'Some quota histories could not be loaded. Retry to recover missing series.'
          )}
        </p>
      ) : null}
      {catalogueIncomplete ? (
        <p role='alert' className='text-sm text-amber-700'>
          {t(
            'The channel list is partial because its scan or item limit was reached. Some historical series may be missing; changing the chart time range does not expand this list.'
          )}
        </p>
      ) : null}
      {incomplete ? (
        <p role='alert' className='text-sm text-amber-700'>
          {t(
            'History is incomplete. Zoom in or shorten the time range to load finer samples.'
          )}
        </p>
      ) : null}
      <div className='min-w-0 rounded-lg border p-2 sm:p-3'>
        <div className='flex flex-wrap items-center justify-between gap-2 text-xs'>
          <span>
            {timeLabel(bounds.start)} – {timeLabel(bounds.end)}
          </span>
          <div className='flex shrink-0 gap-1'>
            <Button
              variant='ghost'
              size='sm'
              onClick={() => {
                const duration = (
                  {
                    '1h': 3600,
                    '24h': 86400,
                    '7d': 604800,
                    '30d': 2592000,
                  } as Record<string, number>
                )[mode]
                if (duration) {
                  const end = Math.floor(Date.now() / 1000)
                  setBounds({ start: end - duration, end })
                }
                setRefresh((value) => value + 1)
              }}
              disabled={loading}
            >
              {t('Refresh')}
            </Button>
            <Button size='sm' variant='ghost' onClick={resetView}>
              {t('Reset view')}
            </Button>
            <Button
              size='sm'
              variant='outline'
              aria-label={t('Zoom in')}
              onClick={() =>
                zoom(
                  zoomQuotaTime(bounds, 0.5, 0.5, Math.floor(Date.now() / 1000))
                )
              }
            >
              +
            </Button>
            <Button
              size='sm'
              variant='outline'
              aria-label={t('Zoom out')}
              onClick={() =>
                zoom(
                  zoomQuotaTime(bounds, 2, 0.5, Math.floor(Date.now() / 1000))
                )
              }
            >
              −
            </Button>
          </div>
        </div>
        <div className='relative'>
          <QuotaComparisonChart
            series={chartSeries}
            metric={metric}
            style={style}
            bounds={bounds}
            onZoom={zoom}
          />
          {loading ? (
            <p
              role='status'
              className='bg-background/90 absolute inset-x-0 top-2 mx-auto w-fit rounded px-3 py-1 text-xs'
            >
              {t('Loading')}
            </p>
          ) : null}
          {!loading && !hasSamples ? (
            <div className='bg-background/90 absolute inset-0 flex flex-col items-center justify-center gap-3 px-4 text-center text-sm'>
              <p>{emptyLabel}</p>
            </div>
          ) : null}
        </div>
        <div className='flex flex-wrap gap-x-4 gap-y-1 text-xs'>
          {chartSeries.map((item) => (
            <span key={item.key} style={{ color: item.color }}>
              ● {item.label}
            </span>
          ))}
        </div>
      </div>
      {mode === 'custom' ? (
        <QuotaCustomRangeControls
          key={`${bounds.start}:${bounds.end}`}
          seconds
          range={{ start: params.start, end: params.end }}
          onApply={(value) =>
            zoom({
              start: Math.floor(new Date(value.start).getTime() / 1000),
              end: Math.floor(new Date(value.end).getTime() / 1000),
            })
          }
        />
      ) : null}
      {mode === 'cycle' ? (
        <div className='grid gap-2 rounded-lg border p-3'>
          <p className='text-sm'>
            {t(
              'Cycle anchor uses the first selected channel. Other channels keep their own reset boundaries.'
            )}
          </p>
          {sourceReset ? (
            <p className='text-muted-foreground text-xs'>
              {chosen[0]?.account_label || chosen[0]?.name} · {t('Reset time')}:{' '}
              {timeLabel(sourceReset)}
            </p>
          ) : null}
          <div className='flex flex-wrap items-end gap-2'>
            <Label className='grid gap-1'>
              {t('Reset anchor override')}
              <input
                type='datetime-local'
                step='1'
                className={selectClass}
                value={cycleAnchor}
                onChange={(event) => {
                  setCycleAnchor(event.target.value)
                  setManualAnchor(!!event.target.value)
                }}
              />
            </Label>
            <Button variant='outline' onClick={() => applyCycle(0)}>
              {t('Current cycle')}
            </Button>
            <Button
              variant='outline'
              disabled={cycleOffset <= -24}
              onClick={() => applyCycle(cycleOffset - 1)}
            >
              {t('Previous cycle')}
            </Button>
          </div>
          {cycleError ? (
            <p role='alert' className='text-destructive text-sm'>
              {t(
                'Choose a reset anchor because no provider reset time is available.'
              )}
            </p>
          ) : null}
        </div>
      ) : null}
      {metric === 'consumption' ? (
        <p className='text-muted-foreground text-sm'>
          {t(
            'Estimated usage is the drop between valid remaining-quota samples. Resets, refills and missing intervals are excluded; this is not billed usage.'
          )}
        </p>
      ) : null}
      <details className='text-muted-foreground border-t pt-2 text-xs'>
        <summary className='w-fit cursor-pointer py-1'>
          {t('Advanced settings')}
        </summary>
        <div className='mt-2 grid gap-3'>
          <Label className='grid gap-1'>
            {t('Resolution')}
            <select
              aria-label={t('Resolution')}
              className={selectClass}
              value={granularity}
              onChange={(event) =>
                setGranularity(event.target.value as 'raw' | 'auto')
              }
            >
              <option value='raw'>{t('Raw samples')}</option>
              <option value='auto'>{t('Auto')}</option>
            </select>
          </Label>{' '}
          <p className='text-muted-foreground mt-3 text-xs'>
            {t(
              'Scroll over the chart to zoom. Zooming reloads the selected time range; tooltips show recorded timestamps.'
            )}
          </p>{' '}
          <p className='text-muted-foreground mt-2 text-xs'>
            {t('Compare up to 8 series with the same unit and quota window.')}
          </p>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Display precision does not increase sampling frequency. No unobserved samples are reconstructed.'
            )}
          </p>
        </div>
      </details>
    </section>
  )
}
