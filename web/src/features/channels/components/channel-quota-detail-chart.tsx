/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
/* eslint-disable no-nested-ternary */
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Line,
  LineChart,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ChartContainer } from '@/components/ui/chart'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { formatTimestampToDate } from '@/lib/format'

import { getChannelQuotaHistory } from '../api'
import type { ChannelQuotaChangeItem } from '../types'

type Range = '24h' | '7d' | '30d' | '90d'
type Granularity = 'auto' | 'raw' | 'hour' | 'day' | 'week'
type Metric = 'available' | 'used' | 'total'
type ChartStyle = 'line' | 'area' | 'bar'

function valueLabel(value: number | undefined, item: ChannelQuotaChangeItem) {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  if (item.unit === 'percent') return `${value.toFixed(1)}%`
  if (item.unit === 'usd' || item.currency) {
    return formatCurrencyFromUSD(value, {
      digitsLarge: 2,
      digitsSmall: 4,
      abbreviate: false,
    })
  }
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 4 }).format(
    value
  )
}

export function ChannelQuotaDetailChart({
  item,
}: {
  item: ChannelQuotaChangeItem
}) {
  const { t } = useTranslation()
  const [range, setRange] = useState<Range>('30d')
  const [granularity, setGranularity] = useState<Granularity>('auto')
  const [metric, setMetric] = useState<Metric>('available')
  const [chartStyle, setChartStyle] = useState<ChartStyle>('line')
  const query = useQuery({
    queryKey: [
      'channel-quota-detail-chart',
      item.channel_id,
      item.metric_type,
      item.source,
      item.window_type,
      item.plan_type,
      range,
      granularity,
      metric,
    ],
    queryFn: () =>
      getChannelQuotaHistory(item.channel_id, {
        range,
        metric_type: item.metric_type,
        source: item.source,
        window_type: item.window_type,
        plan_type: item.plan_type,
        unit: item.unit,
        currency: item.currency,
        window_seconds: item.window_seconds,
        granularity,
        timezone_offset: -new Date().getTimezoneOffset(),
        limit: 1000,
      }),
    staleTime: 60 * 1000,
    retry: false,
  })
  const data = query.data?.data
  const points = (data?.points ?? []).filter(
    (point) =>
      point.status === 'success' &&
      typeof point[metric] === 'number' &&
      Number.isFinite(point[metric])
  )
  const chartPoints = points.map((point) => ({
    ...point,
    label: formatTimestampToDate(point.timestamp),
    value: point[metric] as number,
  }))
  const latest = chartPoints.at(-1)?.value ?? data?.current?.[metric]
  const summary = data?.summary
  const title = `${item.account_label || item.name} · ${item.window_type || t('Quota')}`
  const metricLabel = t(
    metric === 'available'
      ? 'Available quota'
      : metric === 'used'
        ? 'Used quota'
        : 'Total quota'
  )

  return (
    <Card className='min-w-0' data-testid='channel-quota-detail-chart'>
      <CardHeader className='gap-3 pb-3'>
        <div className='flex min-w-0 flex-wrap items-center justify-between gap-2'>
          <CardTitle className='truncate text-sm'>
            {t('Detailed quota history')} · {title}
          </CardTitle>
          <div className='flex max-w-full flex-wrap gap-1'>
            <select
              aria-label={t('Time range')}
              className='h-8 rounded-lg border bg-transparent px-2 text-sm'
              value={range}
              onChange={(event) => setRange(event.target.value as Range)}
            >
              {(['24h', '7d', '30d', '90d'] as Range[]).map((value) => (
                <option key={value} value={value}>
                  {value}
                </option>
              ))}
            </select>
            <select
              aria-label={t('Chart granularity')}
              className='h-8 rounded-lg border bg-transparent px-2 text-sm'
              value={granularity}
              onChange={(event) =>
                setGranularity(event.target.value as Granularity)
              }
            >
              {(['auto', 'raw', 'hour', 'day', 'week'] as Granularity[]).map(
                (value) => (
                  <option key={value} value={value}>
                    {t(
                      value === 'auto'
                        ? 'Auto'
                        : value[0].toUpperCase() + value.slice(1)
                    )}
                  </option>
                )
              )}
            </select>
            <select
              aria-label={t('Metric')}
              className='h-8 rounded-lg border bg-transparent px-2 text-sm'
              value={metric}
              onChange={(event) => setMetric(event.target.value as Metric)}
            >
              <option value='available'>{t('Available quota')}</option>
              <option value='used'>{t('Used quota')}</option>
              <option value='total'>{t('Total quota')}</option>
            </select>
            <select
              aria-label={t('Chart style')}
              className='h-8 rounded-lg border bg-transparent px-2 text-sm'
              value={chartStyle}
              onChange={(event) =>
                setChartStyle(event.target.value as ChartStyle)
              }
            >
              <option value='line'>{t('Line')}</option>
              <option value='area'>{t('Area')}</option>
              <option value='bar'>{t('Bar')}</option>
            </select>
          </div>
        </div>
      </CardHeader>
      <CardContent className='space-y-3'>
        {query.isLoading ? (
          <div className='bg-muted h-56 animate-pulse rounded-lg' />
        ) : null}
        {query.isError || query.data?.success === false ? (
          <Alert variant='destructive'>
            <AlertTitle>{t('Unable to load quota history')}</AlertTitle>
            <AlertDescription>
              {query.error instanceof Error
                ? query.error.message
                : query.data?.message || t('Please try again later.')}
            </AlertDescription>
          </Alert>
        ) : null}
        {!query.isLoading && !query.isError && points.length === 0 ? (
          <div className='text-muted-foreground rounded-lg border border-dashed p-5 text-center text-sm'>
            {t('No quota history data yet')}
          </div>
        ) : null}
        {!query.isLoading && !query.isError && points.length > 0 ? (
          <>
            <div className='grid grid-cols-2 gap-2 sm:grid-cols-4'>
              <div className='bg-muted/40 rounded-lg p-2'>
                <div className='text-muted-foreground text-[11px]'>
                  {t('Latest')}
                </div>
                <div className='font-semibold'>{valueLabel(latest, item)}</div>
              </div>
              <div className='bg-muted/40 rounded-lg p-2'>
                <div className='text-muted-foreground text-[11px]'>
                  {t('Change')}
                </div>
                <div className='font-semibold'>
                  {summary ? valueLabel(summary.change, item) : '-'}
                </div>
              </div>
              <div className='bg-muted/40 rounded-lg p-2'>
                <div className='text-muted-foreground text-[11px]'>
                  {t('Minimum')}
                </div>
                <div className='font-semibold'>
                  {summary ? valueLabel(summary.minimum, item) : '-'}
                </div>
              </div>
              <div className='bg-muted/40 rounded-lg p-2'>
                <div className='text-muted-foreground text-[11px]'>
                  {t('Maximum')}
                </div>
                <div className='font-semibold'>
                  {summary ? valueLabel(summary.maximum, item) : '-'}
                </div>
              </div>
            </div>
            <div
              className='h-72 w-full min-w-0'
              aria-label={t('Quota history chart')}
            >
              <ChartContainer
                className='h-full w-full'
                config={{
                  value: { label: metricLabel, color: 'var(--chart-1)' },
                }}
                initialDimension={{ width: 640, height: 288 }}
              >
                {chartStyle === 'bar' ? (
                  <BarChart
                    data={chartPoints}
                    margin={{ top: 8, right: 8, left: 0, bottom: 4 }}
                  >
                    <CartesianGrid vertical={false} strokeDasharray='3 3' />
                    <XAxis
                      dataKey='label'
                      tickLine={false}
                      axisLine={false}
                      minTickGap={36}
                      tick={{ fontSize: 10 }}
                    />
                    <YAxis
                      tickLine={false}
                      axisLine={false}
                      width={56}
                      tick={{ fontSize: 10 }}
                    />
                    <Tooltip
                      formatter={(value) => [
                        valueLabel(Number(value), item),
                        metricLabel,
                      ]}
                      labelFormatter={(label) => String(label)}
                    />
                    <Bar
                      dataKey='value'
                      fill='var(--color-value)'
                      radius={[3, 3, 0, 0]}
                      isAnimationActive={false}
                    />
                  </BarChart>
                ) : chartStyle === 'area' ? (
                  <AreaChart
                    data={chartPoints}
                    margin={{ top: 8, right: 8, left: 0, bottom: 4 }}
                  >
                    <CartesianGrid vertical={false} strokeDasharray='3 3' />
                    <XAxis
                      dataKey='label'
                      tickLine={false}
                      axisLine={false}
                      minTickGap={36}
                      tick={{ fontSize: 10 }}
                    />
                    <YAxis
                      tickLine={false}
                      axisLine={false}
                      width={56}
                      tick={{ fontSize: 10 }}
                    />
                    <Tooltip
                      formatter={(value) => [
                        valueLabel(Number(value), item),
                        metricLabel,
                      ]}
                      labelFormatter={(label) => String(label)}
                    />
                    <Area
                      type='monotone'
                      dataKey='value'
                      connectNulls={false}
                      stroke='var(--color-value)'
                      fill='var(--color-value)'
                      fillOpacity={0.16}
                      strokeWidth={2}
                      dot={points.length < 80}
                      isAnimationActive={false}
                    />
                  </AreaChart>
                ) : (
                  <LineChart
                    data={chartPoints}
                    margin={{ top: 8, right: 8, left: 0, bottom: 4 }}
                  >
                    <CartesianGrid vertical={false} strokeDasharray='3 3' />
                    <XAxis
                      dataKey='label'
                      tickLine={false}
                      axisLine={false}
                      minTickGap={36}
                      tick={{ fontSize: 10 }}
                    />
                    <YAxis
                      tickLine={false}
                      axisLine={false}
                      width={56}
                      tick={{ fontSize: 10 }}
                    />
                    <Tooltip
                      formatter={(value) => [
                        valueLabel(Number(value), item),
                        metricLabel,
                      ]}
                      labelFormatter={(label) => String(label)}
                    />
                    <Line
                      type='monotone'
                      dataKey='value'
                      connectNulls={false}
                      stroke='var(--color-value)'
                      strokeWidth={2}
                      dot={points.length < 80}
                      activeDot={{ r: 4 }}
                      isAnimationActive={false}
                    />
                  </LineChart>
                )}
              </ChartContainer>
            </div>
            <div className='text-muted-foreground flex flex-wrap justify-between gap-2 text-xs'>
              <span>
                {t('Samples')}: {points.length}
              </span>
              <span>
                {t('Last sample')}:{' '}
                {formatTimestampToDate(points.at(-1)?.timestamp ?? 0)}
              </span>
            </div>
          </>
        ) : null}
      </CardContent>
    </Card>
  )
}
