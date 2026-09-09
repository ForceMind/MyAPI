/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQueries, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ChartNoAxesCombined } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { IconBadge } from '@/components/ui/icon-badge'
import {
  getChannelQuotaChanges,
  getChannelQuotaHistory,
} from '@/features/channels/api'
import {
  QuotaComparisonChart,
  type ComparisonSeries,
} from '@/features/channels/components/quota-comparison-chart'
import type { QuotaTimeBounds } from '@/features/channels/lib/quota-comparison'
import {
  getChannelQuotaSeriesColor,
  quotaSeriesKey,
} from '@/features/channels/lib/quota-history'
import type { ChannelQuotaHistoryRange } from '@/features/channels/types'
import { useAuthStore } from '@/stores/auth-store'

import { PanelWrapper } from '../ui/panel-wrapper'
import { selectComparableQuotaSeries } from './channel-quota-overview-data'

const RANGE_SECONDS: Record<ChannelQuotaHistoryRange, number> = {
  '1h': 60 * 60,
  '6h': 6 * 60 * 60,
  '24h': 24 * 60 * 60,
  '7d': 7 * 24 * 60 * 60,
  '30d': 30 * 24 * 60 * 60,
  '90d': 90 * 24 * 60 * 60,
  custom: 24 * 60 * 60,
}

function currentBounds(range: ChannelQuotaHistoryRange): QuotaTimeBounds {
  const end = Math.floor(Date.now() / 1000)
  return { start: end - RANGE_SECONDS[range], end }
}

/**
 * A compact, read-only quota main chart. It keeps only directly comparable
 * provider series together and delegates chart rendering, gaps and zoom
 * behavior to the channel feature's shared chart.
 */
export function ChannelQuotaOverview() {
  const { t } = useTranslation()
  const userId = useAuthStore((state) => state.auth.user?.id ?? null)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const [range, setRange] = useState<ChannelQuotaHistoryRange>('24h')
  const [bounds, setBounds] = useState<QuotaTimeBounds>(() =>
    currentBounds('24h')
  )
  const [refresh, setRefresh] = useState(0)
  const catalogue = useQuery({
    queryKey: [
      'dashboard',
      'channel-quota-catalogue',
      userId,
      sessionId,
      refresh,
    ],
    queryFn: () => getChannelQuotaChanges({ catalogue: true, limit: 2000 }),
    staleTime: 60 * 1000,
    retry: false,
  })
  const items = useMemo(
    () => selectComparableQuotaSeries(catalogue.data?.data?.items ?? []),
    [catalogue.data?.data?.items]
  )
  const histories = useQueries({
    queries: items.map((item) => ({
      queryKey: [
        'dashboard',
        'channel-quota-history',
        userId,
        sessionId,
        quotaSeriesKey(item),
        bounds.start,
        bounds.end,
        refresh,
      ],
      queryFn: () =>
        getChannelQuotaHistory(item.channel_id, {
          range: 'custom',
          start: new Date(bounds.start * 1000).toISOString(),
          end: new Date(bounds.end * 1000).toISOString(),
          exact_identity: true,
          consumption_basis: 'available',
          metric_type: item.metric_type,
          source: item.source,
          window_type: item.window_type,
          plan_type: item.plan_type,
          unit: item.unit,
          currency: item.currency,
          window_seconds: item.window_seconds,
          granularity: 'auto',
          timezone_offset: -new Date().getTimezoneOffset(),
          limit: 5000,
        }),
      staleTime: 60 * 1000,
      retry: false,
    })),
  })
  const series = useMemo<ComparisonSeries[]>(
    () =>
      items.flatMap((item, index) => {
        const result = histories[index]?.data
        if (!result?.success || !result.data) return []
        return [
          {
            key: quotaSeriesKey(item),
            label: `${item.account_label || item.name} · #${item.channel_id}`,
            data: result.data,
            color: getChannelQuotaSeriesColor(item.channel_id),
          },
        ]
      }),
    [histories, items]
  )
  const loading =
    catalogue.isLoading || histories.some((query) => query.isLoading)
  const failed =
    catalogue.isError ||
    catalogue.data?.success === false ||
    histories.some((query) => query.isError || query.data?.success === false)
  const incomplete =
    catalogue.data?.data?.source_complete === false ||
    catalogue.data?.data?.items_complete === false ||
    histories.some((query) => {
      const data = query.data?.data
      return (
        data?.complete === false ||
        data?.truncated === true ||
        data?.source_complete === false ||
        data?.points_complete === false
      )
    })
  const hasSamples = series.some((item) =>
    item.data.points.some((point) => Number.isFinite(point.available))
  )

  const handleRangeChange = (nextRange: ChannelQuotaHistoryRange) => {
    if (nextRange === 'custom') return
    setRange(nextRange)
    setBounds(currentBounds(nextRange))
  }

  const handleZoom = (nextBounds: QuotaTimeBounds) => {
    setRange('custom')
    setBounds(nextBounds)
  }

  return (
    <PanelWrapper
      title={
        <span className='flex items-center gap-2'>
          <IconBadge tone='info' size='sm'>
            <ChartNoAxesCombined />
          </IconBadge>
          {t('Quota history')}
        </span>
      }
      description={t(
        'Compare recorded quota on one timeline. Missing samples remain gaps.'
      )}
      headerActions={
        <div className='flex items-center gap-1'>
          <Button
            variant='ghost'
            size='sm'
            onClick={() => {
              if (range !== 'custom') setBounds(currentBounds(range))
              setRefresh((value) => value + 1)
            }}
            disabled={loading}
          >
            {t('Refresh')}
          </Button>
          <Button
            variant='ghost'
            size='sm'
            render={
              <Link
                to='/channels'
                search={{
                  tab: 'quota',
                  quotaChannelId: items[0]?.channel_id,
                  quotaStart: bounds.start,
                  quotaEnd: bounds.end,
                }}
              />
            }
            nativeButton={false}
          >
            {t('Quota analysis')}
          </Button>
        </div>
      }
      contentClassName='min-w-0'
    >
      <div className='grid min-w-0 gap-3'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <label className='text-muted-foreground flex items-center gap-2 text-xs'>
            {t('Time range')}
            <select
              value={range}
              className='bg-background h-8 rounded-lg border px-2 text-sm'
              onChange={(event) =>
                handleRangeChange(
                  event.target.value as ChannelQuotaHistoryRange
                )
              }
            >
              <option value='1h'>{t('Last hour')}</option>
              <option value='6h'>{t('Last 6 hours')}</option>
              <option value='24h'>{t('Last 24 hours')}</option>
              <option value='7d'>{t('Last 7 days')}</option>
              <option value='30d'>{t('Last 30 days')}</option>
              <option value='custom'>{t('Custom')}</option>
            </select>
          </label>
          <span className='text-muted-foreground text-xs'>
            {t(
              'Scroll over the chart to zoom. Zooming reloads the selected time range; tooltips show recorded timestamps.'
            )}
          </span>
        </div>
        <div className='relative min-w-0 rounded-xl border p-2 sm:p-3'>
          <QuotaComparisonChart
            series={series}
            metric='available'
            style='line'
            bounds={bounds}
            onZoom={handleZoom}
            className='h-[240px]'
          />
          {loading ? (
            <p
              className='bg-background/90 absolute inset-x-0 top-3 mx-auto w-fit rounded px-3 py-1 text-xs'
              role='status'
            >
              {t('Loading')}
            </p>
          ) : null}
          {!loading && !hasSamples ? (
            <p className='text-muted-foreground bg-background/90 absolute inset-0 flex items-center justify-center px-4 text-center text-sm'>
              {failed ? t('Please try again.') : t('No quota history data yet')}
            </p>
          ) : null}
        </div>
        {series.length > 0 ? (
          <div className='flex flex-wrap gap-x-4 gap-y-1 text-xs'>
            {series.map((item) => (
              <Link
                key={item.key}
                to='/channels'
                search={{
                  tab: 'quota',
                  quotaChannelId: item.data.channel_id,
                  quotaStart: bounds.start,
                  quotaEnd: bounds.end,
                }}
                className='hover:underline'
                style={{ color: item.color }}
              >
                ● {item.label}
              </Link>
            ))}
          </div>
        ) : null}
        {failed ? (
          <p role='alert' className='text-destructive text-sm'>
            {t(
              'Some quota histories could not be loaded. Retry to recover missing series.'
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
      </div>
    </PanelWrapper>
  )
}
