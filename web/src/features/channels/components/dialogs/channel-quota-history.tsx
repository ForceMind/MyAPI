/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQuery } from '@tanstack/react-query'
import { Info } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { useAuthStore } from '@/stores/auth-store'

import { getChannelQuotaHistory } from '../../api'
import { isMultiKeyChannel } from '../../lib'
import type { QuotaHistoryChartStyle } from '../../lib/quota-history'
import type {
  Channel,
  ChannelQuotaHistoryGranularity,
  ChannelQuotaHistoryMetric,
} from '../../types'
import { QuotaHistoryTrend } from '../quota-history-trend'

const HISTORY_RANGES = ['1h', '6h', '24h', '7d', '30d', '90d'] as const
const RANGE_OPTIONS = [...HISTORY_RANGES, 'custom'] as const
const GRANULARITY_OPTIONS = [
  'auto',
  'raw',
  'minute',
  '5m',
  '15m',
  'hour',
  'day',
  'week',
] as const satisfies readonly ChannelQuotaHistoryGranularity[]
const HISTORY_POINT_LIMIT = 5000

type Range = (typeof RANGE_OPTIONS)[number]

function alertVariant(status: string) {
  if (status === 'critical') return 'destructive' as const
  if (status === 'warning') return 'secondary' as const
  return 'outline' as const
}

function QuotaAlertStatus({
  alert,
}: {
  alert:
    | {
        enabled: boolean
        status: 'disabled' | 'unavailable' | 'healthy' | 'warning' | 'critical'
        ratio_percent?: number
        warning_percent: number
        critical_percent: number
      }
    | undefined
}) {
  const { t } = useTranslation()

  if (!alert?.enabled) return null

  return (
    <Alert>
      <AlertTitle className='flex flex-wrap items-center gap-2'>
        <span>{t('Quota alert')}</span>
        <Badge variant={alertVariant(alert.status)}>
          {t(alert.status[0].toUpperCase() + alert.status.slice(1))}
        </Badge>
      </AlertTitle>
      <AlertDescription className='flex flex-wrap gap-x-3 gap-y-1'>
        {alert.ratio_percent != null ? (
          <span>
            {t('Remaining')}: {alert.ratio_percent.toFixed(1)}%
          </span>
        ) : null}
        <span>
          {t('Warning')}: {alert.warning_percent}%
        </span>
        <span>
          {t('Critical')}: {alert.critical_percent}%
        </span>
      </AlertDescription>
    </Alert>
  )
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
  const userId = useAuthStore((state) => state.auth.user?.id ?? null)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const [customStart, setCustomStart] = useState('')
  const [customEnd, setCustomEnd] = useState('')
  const [appliedCustomRange, setAppliedCustomRange] = useState<{
    start: string
    end: string
  } | null>(null)
  const [granularity, setGranularity] =
    useState<ChannelQuotaHistoryGranularity>('auto')
  const [metric, setMetric] = useState<ChannelQuotaHistoryMetric>('available')
  const [manualChartStyle, setChartStyle] =
    useState<QuotaHistoryChartStyle | null>(null)
  const chartStyle =
    manualChartStyle ?? (metric === 'consumption' ? 'bar' : 'line')
  const multiKey = isMultiKeyChannel(channel)
  const timezoneOffset = -new Date().getTimezoneOffset()
  const customRangeReady = range !== 'custom' || appliedCustomRange !== null
  const customStartAt =
    range === 'custom' && appliedCustomRange
      ? new Date(`${appliedCustomRange.start}T00:00:00`).toISOString()
      : undefined
  const customEndAt =
    range === 'custom' && appliedCustomRange
      ? new Date(`${appliedCustomRange.end}T23:59:59.999`).toISOString()
      : undefined
  const requestRange = range === 'custom' ? undefined : range
  const requestIdentity = {
    range: requestRange,
    start: customStartAt,
    end: customEndAt,
    granularity,
    timezone_offset: timezoneOffset,
    limit: HISTORY_POINT_LIMIT,
  }
  const query = useQuery({
    queryKey: [
      'channel-quota-history',
      userId,
      sessionId,
      channel.id,
      requestIdentity.range,
      requestIdentity.start,
      requestIdentity.end,
      requestIdentity.granularity,
      requestIdentity.timezone_offset,
      requestIdentity.limit,
    ],
    queryFn: () => getChannelQuotaHistory(channel.id, requestIdentity),
    enabled: open && !multiKey && customRangeReady,
    retry: false,
    staleTime: 60 * 1000,
    refetchInterval: 60 * 1000,
  })
  const data = query.data?.success === false ? undefined : query.data?.data
  const errorMessage =
    query.data?.success === false
      ? query.data.message || t('Please try again later.')
      : undefined
  const invalidCustomRange = Boolean(
    customStart && customEnd && customStart > customEnd
  )

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

  return (
    <div className='min-w-0 space-y-3'>
      {range === 'custom' ? (
        <Card size='sm' className='min-w-0'>
          <CardContent className='space-y-3 pt-4'>
            {!appliedCustomRange ? (
              <label className='grid gap-1 text-xs'>
                <span className='text-muted-foreground'>{t('Time range')}</span>
                <select
                  aria-label={t('Time range')}
                  value={range}
                  onChange={(event) => setRange(event.target.value as Range)}
                  className='h-8 rounded-lg border bg-transparent px-2 text-sm'
                >
                  {RANGE_OPTIONS.map((option) => (
                    <option key={option} value={option}>
                      {option === 'custom' ? t('Custom') : option}
                    </option>
                  ))}
                </select>
              </label>
            ) : null}
            <div className='flex flex-wrap items-end gap-2'>
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
                disabled={!customStart || !customEnd || invalidCustomRange}
                onClick={() =>
                  setAppliedCustomRange({ start: customStart, end: customEnd })
                }
              >
                {t('Apply Filters')}
              </Button>
              {invalidCustomRange ? (
                <p className='text-destructive basis-full text-xs'>
                  {t('Invalid time range')}
                </p>
              ) : null}
            </div>
          </CardContent>
        </Card>
      ) : null}
      {customRangeReady ? (
        <>
          <QuotaAlertStatus alert={data?.alert} />
          <QuotaHistoryTrend
            data={data}
            title={t('Quota history')}
            range={range}
            granularity={granularity}
            metric={metric}
            chartStyle={chartStyle}
            rangeOptions={RANGE_OPTIONS}
            rangeOptionLabel={(value) =>
              value === 'custom' ? t('Custom') : value
            }
            granularityOptions={GRANULARITY_OPTIONS}
            isLoading={query.isLoading}
            error={query.isError ? query.error : undefined}
            errorMessage={errorMessage}
            onRangeChange={(value) => setRange(value as Range)}
            onGranularityChange={(value) =>
              setGranularity(value as ChannelQuotaHistoryGranularity)
            }
            onMetricChange={setMetric}
            onChartStyleChange={setChartStyle}
            onRefresh={() => {
              void query.refetch()
            }}
          />
        </>
      ) : null}
    </div>
  )
}
