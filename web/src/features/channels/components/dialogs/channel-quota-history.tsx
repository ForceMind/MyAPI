/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
/* eslint-disable no-nested-ternary */
import { useQuery } from '@tanstack/react-query'
import { AlertCircle, ChartNoAxesCombined, Info } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CartesianGrid,
  Line,
  LineChart,
  ReferenceLine,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ChartContainer } from '@/components/ui/chart'
import { Skeleton } from '@/components/ui/skeleton'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import { getChannelQuotaHistory } from '../../api'
import { isMultiKeyChannel } from '../../lib'
import type {
  Channel,
  ChannelQuotaHistoryData,
  ChannelQuotaHistoryPoint,
} from '../../types'

type Range = '24h' | '7d' | '30d' | '90d' | 'custom'
type Granularity = 'auto' | 'raw' | 'hour' | 'day' | 'week'

const ranges: Range[] = ['24h', '7d', '30d', '90d', 'custom']
const granularities: Granularity[] = ['auto', 'raw', 'hour', 'day', 'week']

function formatValue(
  value: number | undefined,
  data?: ChannelQuotaHistoryData
) {
  if (value == null || !Number.isFinite(value)) return '-'
  if (data?.unit === 'percent') return `${value.toFixed(1)}%`
  if (data?.unit === 'usd' || (!data?.unit && data?.currency)) {
    return formatCurrencyFromUSD(value, {
      digitsLarge: 2,
      digitsSmall: 4,
      abbreviate: false,
    })
  }
  return `${new Intl.NumberFormat(undefined, { maximumFractionDigits: 4 }).format(value)}${data?.unit ? ` ${data.unit}` : ''}`
}

function pointLabel(timestamp: number) {
  return formatTimestampToDate(timestamp)
}

export function ChannelQuotaHistory({
  channel,
  open,
}: {
  channel: Channel
  open: boolean
}) {
  const { t } = useTranslation()
  const [range, setRange] = useState<Range>('30d')
  const [customStart, setCustomStart] = useState('')
  const [customEnd, setCustomEnd] = useState('')
  const [appliedCustomRange, setAppliedCustomRange] = useState<{
    start: string
    end: string
  } | null>(null)
  const [granularity, setGranularity] = useState<Granularity>('auto')
  const multiKey = isMultiKeyChannel(channel)
  const timezoneOffset = -new Date().getTimezoneOffset()
  const query = useQuery({
    queryKey: [
      'channel-quota-history',
      channel.id,
      range,
      appliedCustomRange?.start,
      appliedCustomRange?.end,
      granularity,
      timezoneOffset,
    ],
    queryFn: () =>
      getChannelQuotaHistory(channel.id, {
        ...(range === 'custom' && appliedCustomRange
          ? {
              start: new Date(`${appliedCustomRange.start}T00:00:00`).toISOString(),
              end: new Date(`${appliedCustomRange.end}T23:59:59.999`).toISOString(),
            }
          : { range: range as Exclude<Range, 'custom'> }),
        granularity,
        timezone_offset: timezoneOffset,
        limit: 500,
      }),
    enabled: open && !multiKey && (range !== 'custom' || appliedCustomRange !== null),
    retry: false,
    staleTime: 60 * 1000,
  })

  if (multiKey) {
    return (
      <Alert>
        <Info />
        <AlertTitle>{t('Quota history unavailable')}</AlertTitle>
        <AlertDescription>
          {t('Multi-key channels do not expose one combined account quota.')}
        </AlertDescription>
      </Alert>
    )
  }

  const data = query.data?.data
  const points = data?.points ?? []
  const successfulPoints = points.filter(
    (point): point is ChannelQuotaHistoryPoint & { available: number } =>
      point.status === 'success' && typeof point.available === 'number'
  )
  const latest = data?.current?.available ?? successfulPoints.at(-1)?.available
  const chartPoints = points.map((point) => ({
    ...point,
    label: pointLabel(point.timestamp),
    value: point.status === 'success' ? point.available : undefined,
  }))

  return (
    <Card size='sm' className='min-w-0'>
      <CardHeader className='gap-2'>
        <div className='flex items-center justify-between gap-2'>
          <CardTitle className='flex min-w-0 items-center gap-2 text-sm'>
            <ChartNoAxesCombined className='size-4 shrink-0' />
            <span className='truncate'>{t('Quota history')}</span>
          </CardTitle>
          <div className='flex max-w-full shrink-0 gap-1 overflow-x-auto pb-0.5'>
            {ranges.map((item) => (
              <Button
                key={item}
                type='button'
                variant={range === item ? 'secondary' : 'ghost'}
                size='xs'
                className='h-7 shrink-0 px-2 text-xs'
                onClick={() => setRange(item)}
              >
                {item === 'custom' ? t('Custom') : item}
              </Button>
            ))}
          </div>
        </div>
        <div
          className='flex flex-wrap items-center gap-1'
          aria-label={t('Chart granularity')}
        >
          <span className='text-muted-foreground mr-1 text-xs'>
            {t('Bucket')}
          </span>
          {granularities.map((item) => (
            <Button
              key={item}
              type='button'
              variant={granularity === item ? 'secondary' : 'ghost'}
              size='xs'
              className='h-7 px-2 text-xs'
              onClick={() => setGranularity(item)}
            >
              {t(
                item === 'auto' ? 'Auto' : item[0].toUpperCase() + item.slice(1)
              )}
            </Button>
          ))}
        </div>
        {range === 'custom' && (
          <div className='flex flex-wrap items-end gap-2 rounded-md border p-2'>
            <label className='grid min-w-[9rem] flex-1 gap-1 text-xs'>
              <span className='text-muted-foreground'>{t('Start')}</span>
              <input
                type='date'
                value={customStart}
                max={customEnd || undefined}
                onChange={(event) => setCustomStart(event.target.value)}
                className='bg-background h-8 rounded-md border px-2 text-sm'
              />
            </label>
            <label className='grid min-w-[9rem] flex-1 gap-1 text-xs'>
              <span className='text-muted-foreground'>{t('End')}</span>
              <input
                type='date'
                value={customEnd}
                min={customStart || undefined}
                onChange={(event) => setCustomEnd(event.target.value)}
                className='bg-background h-8 rounded-md border px-2 text-sm'
              />
            </label>
            <Button
              type='button'
              size='sm'
              className='h-8'
              disabled={!customStart || !customEnd || customStart > customEnd}
              onClick={() =>
                setAppliedCustomRange({ start: customStart, end: customEnd })
              }
            >
              {t('Apply Filters')}
            </Button>
            {customStart && customEnd && customStart > customEnd && (
              <p className='basis-full text-xs text-destructive'>
                {t('Start date must be before end date')}
              </p>
            )}
          </div>
        )}
      </CardHeader>
      <CardContent className='min-w-0 space-y-3'>
        {query.isLoading ? (
          <div className='space-y-3' aria-label={t('Loading')}>
            <Skeleton className='h-56 w-full' />
            <Skeleton className='h-4 w-2/3' />
          </div>
        ) : query.isError || query.data?.success === false ? (
          <Alert variant='destructive'>
            <AlertCircle />
            <AlertTitle>{t('Unable to load quota history')}</AlertTitle>
            <AlertDescription>
              {query.error instanceof Error
                ? query.error.message
                : query.data?.message || t('Please try again later.')}
            </AlertDescription>
          </Alert>
        ) : points.length === 0 ? (
          <div className='text-muted-foreground rounded-lg border border-dashed p-5 text-center text-sm'>
            {t('No quota history data yet')}
          </div>
        ) : successfulPoints.length === 0 ? (
          <Alert>
            <Info />
            <AlertTitle>{t('Quota history is unavailable')}</AlertTitle>
            <AlertDescription>
              {t('The upstream did not return a usable quota value.')}
            </AlertDescription>
          </Alert>
        ) : (
          <>
            {data?.summary && (
              <div className='grid grid-cols-2 gap-2 sm:grid-cols-4'>
                {[
                  [t('Start'), data.summary.start_available],
                  [t('Current'), data.summary.end_available],
                  [t('Change'), data.summary.change],
                  [t('Change %'), `${data.summary.change_percent.toFixed(1)}%`],
                ].map(([label, value]) => (
                  <div
                    key={String(label)}
                    className='bg-muted/50 min-w-0 rounded-md p-2'
                  >
                    <div className='text-muted-foreground truncate text-[11px]'>
                      {label}
                    </div>
                    <div className='truncate text-sm font-semibold'>
                      {label === t('Change %')
                        ? value
                        : formatValue(value as number, data)}
                    </div>
                  </div>
                ))}
              </div>
            )}
            <div
              className='h-56 w-full min-w-0 touch-pan-y'
              aria-label={t('Quota history chart')}
            >
              <ChartContainer
                className='aspect-auto h-full w-full'
                config={{
                  value: {
                    label: t('Available quota'),
                    color: 'var(--primary)',
                  },
                }}
                initialDimension={{ width: 320, height: 224 }}
              >
                <LineChart
                  data={chartPoints}
                  margin={{ top: 8, right: 8, left: 0, bottom: 4 }}
                >
                  <CartesianGrid vertical={false} strokeDasharray='3 3' />
                  <XAxis
                    dataKey='label'
                    tickLine={false}
                    axisLine={false}
                    minTickGap={32}
                    tick={{ fontSize: 10 }}
                  />
                  <YAxis
                    tickLine={false}
                    axisLine={false}
                    width={48}
                    tick={{ fontSize: 10 }}
                    tickFormatter={(value: number) => formatValue(value, data)}
                  />
                  <Tooltip
                    formatter={(value) => [
                      formatValue(Number(value), data),
                      t('Available quota'),
                    ]}
                    labelFormatter={(label) => String(label)}
                  />
                  {chartPoints.some((point) => point.reset_at) && (
                    <ReferenceLine
                      x={chartPoints.find((point) => point.reset_at)?.label}
                      stroke='var(--muted-foreground)'
                      strokeDasharray='4 4'
                    />
                  )}
                  <Line
                    type='monotone'
                    dataKey='value'
                    connectNulls={false}
                    stroke='var(--color-value)'
                    strokeWidth={2}
                    dot={successfulPoints.length < 80}
                    activeDot={{ r: 5 }}
                    isAnimationActive={false}
                  />
                </LineChart>
              </ChartContainer>
            </div>
            <div className='text-muted-foreground space-y-1 text-xs'>
              <div className='flex flex-wrap justify-between gap-x-3 gap-y-1'>
                <span>
                  {t('Latest')}: {formatValue(latest, data)}
                </span>
                <span>
                  {t('Samples')}: {successfulPoints.length}
                </span>
              </div>
              <p className='break-words'>
                {t('Last sample')}:{' '}
                {pointLabel(successfulPoints.at(-1)?.timestamp ?? 0)}
              </p>
              <div
                className={cn(
                  'border-t pt-2',
                  points.length !== successfulPoints.length &&
                    'text-amber-600 dark:text-amber-400'
                )}
              >
                {points.length !== successfulPoints.length
                  ? t('Some samples failed and are shown as gaps.')
                  : t('Failed samples are never treated as zero.')}
              </div>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}
