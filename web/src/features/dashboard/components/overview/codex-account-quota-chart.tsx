/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQuery } from '@tanstack/react-query'
import { ChartNoAxesCombined, CircleAlert } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CartesianGrid, Line, LineChart, Tooltip, XAxis, YAxis } from 'recharts'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { ChartContainer } from '@/components/ui/chart'
import { Skeleton } from '@/components/ui/skeleton'
import { getChannelQuotaHistory } from '@/features/channels/api'
import type {
  ChannelQuotaChangeItem,
  ChannelQuotaHistoryPoint,
} from '@/features/channels/types'
import { formatTimestampToDate } from '@/lib/format'

function seriesKey(item: ChannelQuotaChangeItem): string {
  return [item.channel_id, item.source, item.window_type, item.plan_type]
    .map((value) => value || '')
    .join('|')
}

function formatPercent(value: number | null | undefined): string {
  return typeof value === 'number' && Number.isFinite(value)
    ? `${value.toFixed(1)}%`
    : '-'
}

function isCodexSeries(item: ChannelQuotaChangeItem): boolean {
  return (
    item.metric_type === 'codex_rate_limit' &&
    item.status === 'success' &&
    Boolean(item.source && item.window_type)
  )
}

export function CodexAccountQuotaChart(props: {
  items: ChannelQuotaChangeItem[]
}) {
  const { t } = useTranslation()
  const series = useMemo(() => props.items.filter(isCodexSeries), [props.items])
  const [selectedKey, setSelectedKey] = useState('')
  const selected =
    series.find((item) => seriesKey(item) === selectedKey) ?? series[0]

  useEffect(() => {
    if (selected && !series.some((item) => seriesKey(item) === selectedKey)) {
      setSelectedKey(seriesKey(selected))
    }
    if (!selected && selectedKey) setSelectedKey('')
  }, [selected, selectedKey, series])

  const query = useQuery({
    queryKey: [
      'dashboard',
      'codex-account-quota-history',
      selected?.channel_id ?? null,
      selected?.source ?? null,
      selected?.window_type ?? null,
      selected?.plan_type ?? null,
    ],
    queryFn: () =>
      getChannelQuotaHistory(selected!.channel_id, {
        range: '24h',
        metric_type: 'codex_rate_limit',
        source: selected!.source,
        window_type: selected!.window_type,
        plan_type: selected!.plan_type,
        unit: 'percent',
        granularity: 'raw',
        limit: 500,
      }),
    enabled: Boolean(selected),
    retry: false,
    staleTime: 60 * 1000,
    refetchInterval: 60 * 1000,
  })

  const data = query.data?.data
  const points = (data?.points ?? []).filter(
    (point): point is ChannelQuotaHistoryPoint & { available: number } =>
      point.status === 'success' &&
      typeof point.available === 'number' &&
      Number.isFinite(point.available)
  )
  const chartPoints = points.map((point) => ({
    ...point,
    label: formatTimestampToDate(point.timestamp),
    value: point.available,
  }))
  const latest = points.at(-1)?.available ?? data?.current?.available

  return (
    <Card size='sm' className='min-w-0' data-testid='codex-account-quota-chart'>
      <CardHeader className='gap-2 p-4 pb-2'>
        <div className='flex min-w-0 flex-wrap items-center justify-between gap-2'>
          <CardTitle className='flex min-w-0 items-center gap-2 text-sm'>
            <ChartNoAxesCombined
              className='text-primary size-4 shrink-0'
              aria-hidden='true'
            />
            <span className='truncate'>Codex · {t('Usage trend')}</span>
          </CardTitle>
          {series.length > 1 ? (
            <label className='text-muted-foreground flex min-w-0 items-center gap-2 text-xs'>
              <span className='shrink-0'>Codex</span>
              <select
                aria-label='Codex account'
                className='text-foreground h-8 max-w-full min-w-0 rounded-lg border bg-transparent px-2 text-sm'
                value={selected ? seriesKey(selected) : ''}
                onChange={(event) => setSelectedKey(event.target.value)}
              >
                {series.map((item) => (
                  <option key={seriesKey(item)} value={seriesKey(item)}>
                    {item.account_label || item.name} · {item.window_type}
                    {item.plan_type ? ` · ${item.plan_type}` : ''}
                  </option>
                ))}
              </select>
            </label>
          ) : null}
        </div>
        <CardDescription className='text-xs leading-5'>
          {t('Historical Codex rate-limit usage. Missing samples remain gaps.')}
        </CardDescription>
      </CardHeader>
      <CardContent className='min-w-0 space-y-3 p-4 pt-2'>
        {series.length === 0 ? (
          <div className='text-muted-foreground rounded-lg border border-dashed p-5 text-center text-sm'>
            {t('No Codex usage history yet')}
          </div>
        ) : query.isLoading ? (
          <Skeleton className='h-56 w-full' aria-label={t('Loading')} />
        ) : query.isError || query.data?.success === false ? (
          <Alert variant='destructive'>
            <CircleAlert />
            <AlertTitle>{t('Unable to load Codex usage history')}</AlertTitle>
            <AlertDescription>
              {query.error instanceof Error
                ? query.error.message
                : query.data?.message || t('Please try again later.')}
            </AlertDescription>
          </Alert>
        ) : points.length === 0 ? (
          <div className='text-muted-foreground rounded-lg border border-dashed p-5 text-center text-sm'>
            {t('No Codex usage history yet')}
          </div>
        ) : (
          <>
            <div
              className='h-56 w-full min-w-0 touch-pan-y'
              aria-label={t('Codex usage history chart')}
            >
              <ChartContainer
                className='aspect-auto h-full w-full'
                config={{
                  value: {
                    label: t('Available quota'),
                    color: 'var(--chart-1)',
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
                    domain={[0, 100]}
                    tickLine={false}
                    axisLine={false}
                    width={42}
                    tick={{ fontSize: 10 }}
                    tickFormatter={(value: number) => `${value}%`}
                  />
                  <Tooltip
                    formatter={(value) => [
                      formatPercent(Number(value)),
                      t('Available quota'),
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
              </ChartContainer>
            </div>
            <div className='text-muted-foreground flex flex-wrap justify-between gap-x-3 gap-y-1 text-xs'>
              <span>
                {t('Latest')}: {formatPercent(latest)}
              </span>
              <span>
                {t('Samples')}: {points.length}
              </span>
              <span>
                {t('Last sample')}:{' '}
                {formatTimestampToDate(points.at(-1)?.timestamp ?? 0)}
              </span>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}
