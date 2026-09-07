/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  formatQuotaAmount,
  formatQuotaDuration,
  formatQuotaRate,
  quotaETAStatus,
  quotaPredictionTiming,
  quotaWindowLabel,
} from '@/features/channels/lib/quota-history'
import type {
  ChannelQuotaChangeItem,
  ChannelQuotaOverviewPoint,
} from '@/features/channels/types'
import { toIntlLocale } from '@/i18n/languages'

function finite(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}

function quotaStatusLabel(
  status: ChannelQuotaChangeItem['status'],
  t: (key: string) => string
): string | null {
  if (status === 'success') return t('Success')
  if (status === 'error') return t('Error')
  if (status === 'unsupported') return t('Unsupported')
  if (status === 'unavailable') return t('Unavailable')
  return null
}

function formatTimestamp(
  timestamp: number | undefined,
  locale: string
): string {
  if (!finite(timestamp)) return '—'
  return new Intl.DateTimeFormat(toIntlLocale(locale), {
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(timestamp * 1000)
}

function RemainingSparkline(props: { item: ChannelQuotaChangeItem }) {
  const { i18n, t } = useTranslation()
  const points = props.item.overview_points ?? []
  const values = points.map((point) => point.available).filter(finite)
  if (values.length === 0) return null
  const segments: ChannelQuotaOverviewPoint[][] = []
  let currentSegment: ChannelQuotaOverviewPoint[] = []
  for (const point of points) {
    if (point.continuity_break || !finite(point.available)) {
      if (currentSegment.length > 0) segments.push(currentSegment)
      currentSegment = []
    }
    if (finite(point.available)) currentSegment.push(point)
  }
  if (currentSegment.length > 0) segments.push(currentSegment)
  const firstPoint = segments[0]?.[0]
  const lastSegment = segments.at(-1)
  const lastPoint = lastSegment?.at(-1)
  if (!firstPoint || !lastPoint) return null
  const minimum = Math.min(...values)
  const maximum = Math.max(...values)
  const spread = maximum - minimum
  const firstTimestamp = firstPoint.timestamp
  const lastTimestamp = lastPoint.timestamp
  const timestampSpan = lastTimestamp - firstTimestamp
  const coordinates = (point: ChannelQuotaOverviewPoint) => {
    const x =
      timestampSpan === 0
        ? 50
        : ((point.timestamp - firstTimestamp) / timestampSpan) * 100
    const available = point.available ?? 0
    const y = spread === 0 ? 50 : 90 - ((available - minimum) / spread) * 80
    return { x, y }
  }
  const descriptionValues = {
    start: formatQuotaAmount(
      firstPoint.available,
      props.item,
      'available',
      t,
      i18n.language
    ),
    end: formatQuotaAmount(
      lastPoint.available,
      props.item,
      'available',
      t,
      i18n.language
    ),
    startTime: formatTimestamp(firstTimestamp, i18n.language),
    endTime: formatTimestamp(lastTimestamp, i18n.language),
    count: segments.length,
    interpolation: { escapeValue: false },
  }
  const description =
    segments.length === 1
      ? t(
          'Remaining quota trend from {{start}} to {{end}}, {{startTime}} to {{endTime}}, across 1 continuous segment',
          descriptionValues
        )
      : t(
          'Remaining quota trend from {{start}} to {{end}}, {{startTime}} to {{endTime}}, across {{count}} continuous segments',
          descriptionValues
        )
  return (
    <svg
      viewBox='0 0 100 100'
      role='img'
      aria-label={description}
      className='text-primary h-10 w-full'
      preserveAspectRatio='none'
      data-testid='quota-overview-sparkline'
    >
      {segments.map((segment) => {
        const line = segment
          .map((point) => {
            const coordinate = coordinates(point)
            return `${coordinate.x},${coordinate.y}`
          })
          .join(' ')
        const singlePoint =
          segment.length === 1 ? coordinates(segment[0]) : null
        return (
          <g
            key={`${segment[0].timestamp}:${segment.at(-1)?.timestamp ?? segment[0].timestamp}`}
            data-testid='quota-overview-sparkline-segment'
          >
            <polyline
              points={line}
              fill='none'
              stroke='currentColor'
              strokeWidth='6'
              strokeLinecap='round'
              strokeLinejoin='round'
              vectorEffect='non-scaling-stroke'
            />
            {singlePoint ? (
              <circle
                cx={singlePoint.x}
                cy={singlePoint.y}
                r='3'
                fill='currentColor'
                vectorEffect='non-scaling-stroke'
              />
            ) : null}
          </g>
        )
      })}
    </svg>
  )
}

export function QuotaOverviewCard(props: { item: ChannelQuotaChangeItem }) {
  const { i18n, t } = useTranslation()
  const item = props.item
  const analysis = item.analysis
  const latest = analysis?.methods.latest_interval
  const observed = analysis?.methods.observed_window
  const details = [
    item.account_label && item.account_label !== item.name ? item.name : null,
    item.window_type ? quotaWindowLabel(item.window_type, t) : null,
  ].filter(Boolean)
  const etaStatus = quotaETAStatus(observed?.eta)
  const statusLabel = quotaStatusLabel(item.status, t)
  let etaLabel = t('Insufficient data')
  let etaValue = t('No depletion forecast available')
  let etaTime: number | undefined
  if (etaStatus === 'depletion') {
    etaLabel = t('Estimated depletion')
    etaTime = observed?.eta.estimated_depletion_at
  } else if (etaStatus === 'reset') {
    etaLabel = t('Reset before depletion')
    etaTime = observed?.eta.reset_at
  } else if (etaStatus === 'stable') {
    etaLabel = t('Stable or no observed consumption')
    etaValue = t('Stable')
  }
  if (etaStatus === 'depletion' || etaStatus === 'reset') {
    const timing = quotaPredictionTiming(etaTime, analysis?.as_of)
    if (timing.status === 'future') {
      etaValue = `${t('Estimated time from analysis point')}: ${formatQuotaDuration(
        timing.seconds,
        t
      )}`
    } else if (timing.status === 'passed') {
      etaValue = t('Prediction time has passed')
    }
  }

  return (
    <Card size='sm' className='min-w-0' data-testid='quota-overview-card'>
      <CardHeader>
        <CardTitle className='truncate' title={item.account_label || item.name}>
          {item.account_label || item.name}
        </CardTitle>
        {details.length > 0 ? (
          <CardDescription className='truncate'>
            {details.join(' · ')}
          </CardDescription>
        ) : null}
        {item.plan_type || statusLabel ? (
          <CardAction>
            <div className='flex items-center gap-1'>
              {item.plan_type ? (
                <Badge variant='secondary'>{item.plan_type}</Badge>
              ) : null}
              {statusLabel ? (
                <Badge
                  variant={
                    item.status === 'error' ? 'destructive' : 'secondary'
                  }
                >
                  {statusLabel}
                </Badge>
              ) : null}
            </div>
          </CardAction>
        ) : null}
      </CardHeader>
      <CardContent className='grid gap-3'>
        <div className='grid grid-cols-2 gap-2'>
          <div className='min-w-0'>
            <div className='text-muted-foreground text-[11px]'>
              {t('Remaining quota')}
            </div>
            <div className='truncate font-semibold tabular-nums'>
              {formatQuotaAmount(item.current_available, item, 'available', t)}
            </div>
          </div>
          <div className='min-w-0'>
            <div className='text-muted-foreground text-[11px]'>
              {t('Latest observed interval')}
            </div>
            <div className='truncate font-semibold tabular-nums'>
              {formatQuotaRate(latest?.rate_per_minute, item, 'minute', t)}
            </div>
          </div>
          <div className='min-w-0'>
            <div className='text-muted-foreground text-[11px]'>
              {t('Average consumption per minute')}
            </div>
            <div className='truncate font-semibold tabular-nums'>
              {formatQuotaRate(observed?.rate_per_minute, item, 'minute', t)}
            </div>
          </div>
          <div className='min-w-0'>
            <div className='text-muted-foreground text-[11px]'>
              {t('Estimated consumption per hour')}
            </div>
            <div className='truncate font-semibold tabular-nums'>
              {formatQuotaRate(observed?.rate_per_hour, item, 'hour', t)}
            </div>
          </div>
        </div>
        <RemainingSparkline item={item} />
        <div className='text-muted-foreground flex items-center justify-between gap-2 text-xs'>
          <span>{t('Observed coverage')}</span>
          <span className='text-foreground font-medium tabular-nums'>
            {observed ? `${Math.round(observed.coverage * 100)}%` : '—'}
          </span>
        </div>
      </CardContent>
      <CardFooter className='justify-between gap-2'>
        <div className='min-w-0 text-xs'>
          <div className='text-muted-foreground'>{etaLabel}</div>
          <div className='font-medium'>{etaValue}</div>
          {etaTime != null ? (
            <div className='text-muted-foreground truncate tabular-nums'>
              {formatTimestamp(etaTime, i18n.language)}
            </div>
          ) : null}
        </div>
        <Button
          variant='ghost'
          size='xs'
          render={
            <Link
              to='/channels'
              search={{ filter: item.name, quotaChannelId: item.channel_id }}
            />
          }
          nativeButton={false}
        >
          {t('View details')}
        </Button>
      </CardFooter>
    </Card>
  )
}
