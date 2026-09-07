/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQuery } from '@tanstack/react-query'
import { getRouteApi, Link } from '@tanstack/react-router'
import {
  Activity,
  ArrowDownRight,
  ArrowUpRight,
  CircleAlert,
  Minus,
  RefreshCw,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { formatNumber, formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { getChannelQuotaChanges, getChannelQuotaSamplingStatus } from '../api'
import { useQuotaHistoryTime } from '../hooks/use-quota-history-time'
import {
  quotaWindowLabel,
  formatQuotaAmount,
  quotaHistoryRangeOptions,
} from '../lib/quota-history'
import type { ChannelQuotaChangeItem, ChannelQuotaHistoryRange } from '../types'
import { ChannelQuotaDetailChart } from './channel-quota-detail-chart'
import { QuotaCustomRangeControls } from './quota-history-trend'

const route = getRouteApi('/_authenticated/channels/')

type Range = ChannelQuotaHistoryRange
type StatusFilter = 'all' | 'success' | 'unavailable' | 'unsupported' | 'error'

function getHttpStatus(error: unknown): number | undefined {
  if (!error || typeof error !== 'object') return undefined
  const response = (error as { response?: unknown }).response
  if (!response || typeof response !== 'object') return undefined
  const status = (response as { status?: unknown }).status
  return typeof status === 'number' ? status : undefined
}

const rangeOptions = quotaHistoryRangeOptions

function detailSeriesKey(item: ChannelQuotaChangeItem): string {
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
    .join(':')
}

function formatMetric(
  value: number | null | undefined,
  item: ChannelQuotaChangeItem | undefined,
  t: (key: string, options?: Record<string, unknown>) => string
) {
  return formatQuotaAmount(value, item, 'rate_per_minute', t)
}

function formatAvailable(
  value: number | null | undefined,
  item: ChannelQuotaChangeItem,
  t: (key: string) => string
) {
  return formatQuotaAmount(value, item, 'available', t)
}

function directionLabel(
  direction: ChannelQuotaChangeItem['direction'],
  t: (key: string) => string
) {
  if (direction === 'increase') return t('Increasing')
  if (direction === 'decrease') return t('Decreasing')
  if (direction === 'stable') return t('Stable')
  return t('Unknown')
}

function statusVariant(status: string | undefined) {
  if (status === 'error' || status === 'critical') return 'destructive' as const
  if (
    status === 'unavailable' ||
    status === 'unsupported' ||
    status === 'warning'
  ) {
    return 'warning' as const
  }
  return 'outline' as const
}

function alertLabel(status: string, t: (key: string) => string) {
  if (status === 'critical') return t('Critical')
  if (status === 'warning') return t('Warning')
  if (status === 'healthy') return t('Healthy')
  if (status === 'unavailable') return t('Unavailable')
  return t('Unknown')
}

function quotaStatusLabel(
  status: string | undefined,
  t: (key: string) => string
) {
  if (status === 'success') return t('Success')
  if (status === 'error') return t('Error')
  if (status === 'unsupported') return t('Unsupported')
  if (status === 'unavailable') return t('Unavailable')
  return status || t('Unknown')
}

function DirectionIcon({
  direction,
}: {
  direction: ChannelQuotaChangeItem['direction']
}) {
  if (direction === 'increase') {
    return (
      <ArrowUpRight
        className='text-success size-4 shrink-0'
        aria-hidden='true'
      />
    )
  }
  if (direction === 'decrease') {
    return (
      <ArrowDownRight
        className='text-destructive size-4 shrink-0'
        aria-hidden='true'
      />
    )
  }
  if (direction === 'stable') {
    return (
      <Minus
        className='text-muted-foreground size-4 shrink-0'
        aria-hidden='true'
      />
    )
  }
  return (
    <CircleAlert
      className='text-muted-foreground size-4 shrink-0'
      aria-hidden='true'
    />
  )
}

function ChangeRow({
  item,
  t,
}: {
  item: ChannelQuotaChangeItem
  t: (key: string) => string
}) {
  const direction = item.direction ?? 'unknown'
  const current = formatAvailable(item.current_available, item, t)
  const previous = formatAvailable(item.previous_available, item, t)
  return (
    <div className='bg-card/60 grid min-w-0 gap-2 rounded-lg border p-3 sm:grid-cols-[minmax(0,1.5fr)_minmax(7rem,0.8fr)_minmax(8rem,1fr)_auto] sm:items-center'>
      <div className='min-w-0'>
        <div className='flex min-w-0 items-center gap-2'>
          <Activity
            className='text-muted-foreground size-4 shrink-0'
            aria-hidden='true'
          />
          <span className='truncate font-medium'>
            {item.account_label || item.name}
          </span>
        </div>
        <div className='text-muted-foreground mt-1 flex min-w-0 flex-wrap items-center gap-1.5 text-xs'>
          {item.account_label && item.name !== item.account_label ? (
            <span className='truncate'>{item.name}</span>
          ) : null}
          {item.window_type ? (
            <Badge variant='secondary'>
              {quotaWindowLabel(item.window_type, t)}
            </Badge>
          ) : null}
          {item.plan_type ? (
            <Badge variant='secondary'>{item.plan_type}</Badge>
          ) : null}
        </div>
      </div>
      <div className='text-muted-foreground text-xs'>
        <div>{t('Current')}</div>
        <div className='text-foreground font-medium'>{current}</div>
        {previous !== '-' ? (
          <div className='mt-0.5'>
            {t('Previous')}: {previous}
          </div>
        ) : null}
      </div>
      <div className='flex min-w-0 items-center gap-2'>
        <DirectionIcon direction={direction} />
        <div className='min-w-0'>
          <div
            className={cn(
              'truncate font-semibold',
              direction === 'decrease' && 'text-destructive',
              direction === 'increase' && 'text-success'
            )}
          >
            {formatMetric(item.change_per_minute, item, t)}
          </div>
          <div className='text-muted-foreground text-xs'>
            {directionLabel(direction, t)}
            {' · '}
            {t('Latest observed interval')}
          </div>
        </div>
      </div>
      <div className='text-muted-foreground text-left text-xs sm:text-right'>
        {item.status ? (
          <Badge variant={statusVariant(item.status)}>
            {quotaStatusLabel(item.status, t)}
          </Badge>
        ) : null}
        {item.alert?.status && item.alert.status !== 'disabled' ? (
          <Badge
            className='mt-1 sm:ml-1'
            variant={statusVariant(item.alert.status)}
          >
            {t('Quota alert')}: {alertLabel(item.alert.status, t)}
          </Badge>
        ) : null}
        {item.observed_at ? (
          <div className='mt-1'>{formatTimestampToDate(item.observed_at)}</div>
        ) : null}
      </div>
    </div>
  )
}

export function ChannelQuotaChangesPanel() {
  const { t } = useTranslation()
  // Avoid carrying a previous administrator's cached account movements into
  // a different login session when the route remains mounted during logout.
  const userId = useAuthStore((state) => state.auth.user?.id ?? null)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const time = useQuotaHistoryTime()
  const { range, setRange } = time
  const { quotaChannelId } = route.useSearch()
  const [windowFilter, setWindowFilter] = useState('all')
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all')
  const [detailKey, setDetailKey] = useState('')
  const [refreshEpoch, setRefreshEpoch] = useState(0)
  const query = useQuery({
    queryKey: ['channel-quota-changes', userId, sessionId, time.params],
    queryFn: () =>
      getChannelQuotaChanges({
        ...time.params,
        limit: 2000,
        sort: 'abs_change_per_minute',
      }),
    retry: false,
    staleTime: 60 * 1000,
    refetchInterval: 60 * 1000,
    placeholderData: (previous, previousQuery) =>
      previousQuery?.queryKey[1] === userId &&
      previousQuery?.queryKey[2] === sessionId
        ? previous
        : undefined,
  })
  const samplingStatusQuery = useQuery({
    queryKey: ['channel-quota-sampling-status', userId, sessionId],
    queryFn: () => getChannelQuotaSamplingStatus(),
    retry: false,
    staleTime: 5 * 60 * 1000,
  })
  const refreshAll = () => {
    // The detail query carries refreshEpoch in its key, so a visible graph is
    // refreshed alongside the change list rather than displaying stale data.
    setRefreshEpoch((value) => value + 1)
    void query.refetch()
    void samplingStatusQuery.refetch()
  }

  const items = useMemo(
    () => query.data?.data?.items ?? [],
    [query.data?.data?.items]
  )
  const windows = useMemo(
    () =>
      [
        ...new Set(
          items.map((item) => item.window_type).filter(Boolean) as string[]
        ),
      ].sort(),
    [items]
  )
  const filteredItems = useMemo(
    () =>
      items
        .filter(
          (item) => windowFilter === 'all' || item.window_type === windowFilter
        )
        .filter(
          (item) => statusFilter === 'all' || item.status === statusFilter
        )
        .sort(
          (a, b) =>
            (b.abs_change_per_minute ?? Math.abs(b.change_per_minute ?? 0)) -
            (a.abs_change_per_minute ?? Math.abs(a.change_per_minute ?? 0))
        ),
    [items, statusFilter, windowFilter]
  )
  // Keep a current upstream error selectable. The detailed endpoint preserves
  // the error as a chart gap and shows it as the latest raw status, which is
  // more useful than hiding the account behind the last successful value.
  const detailItems = filteredItems
  const detailItem =
    detailItems.find((item) => detailSeriesKey(item) === detailKey) ??
    (detailKey === '' && quotaChannelId != null
      ? detailItems.find((item) => item.channel_id === quotaChannelId)
      : undefined) ??
    detailItems[0]
  const httpStatus = getHttpStatus(query.error)
  let errorTitle = t('Unable to load account quota changes')
  let errorDescription =
    query.error instanceof Error
      ? query.error.message
      : query.data?.message || t('Please try again later.')
  if (httpStatus === 401) {
    errorTitle = t('Sign in again to view account quota changes')
    errorDescription = t(
      'Your session is missing or expired. Sign in again to load provider account quota.'
    )
  } else if (httpStatus === 403) {
    errorTitle = t(
      'Administrator permission required to view account quota changes'
    )
    errorDescription = t(
      'Your account does not have permission to read channels.'
    )
  }

  return (
    <Card className='mb-3 min-w-0' data-testid='channel-quota-changes-panel'>
      <CardHeader className='gap-3 pb-3'>
        <div className='flex min-w-0 flex-wrap items-start justify-between gap-3'>
          <div className='min-w-0'>
            <CardTitle className='flex items-center gap-2 text-base'>
              <Activity
                className='text-primary size-4 shrink-0'
                aria-hidden='true'
              />
              <span>{t('Quota consumption')}</span>
            </CardTitle>
            <CardDescription className='mt-1 max-w-2xl text-xs leading-5'>
              {t(
                'Upstream account quota only; MyAPI user balances are not included. Sorted by the largest absolute change per minute.'
              )}
            </CardDescription>
          </div>
          <Button
            type='button'
            variant='ghost'
            size='icon-xs'
            onClick={refreshAll}
            disabled={query.isFetching || samplingStatusQuery.isFetching}
            aria-label={t('Refresh')}
          >
            <RefreshCw
              className={cn('size-4', query.isFetching && 'animate-spin')}
              aria-hidden='true'
            />
          </Button>
        </div>
        <div className='grid min-w-0 grid-cols-1 gap-2 sm:flex sm:flex-wrap sm:items-end'>
          <div className='grid min-w-0 gap-1'>
            <Label
              htmlFor='quota-change-range'
              className='text-muted-foreground text-xs'
            >
              {t('Window')}
            </Label>
            <select
              id='quota-change-range'
              value={range}
              onChange={(event) => setRange(event.target.value as Range)}
              className='h-8 min-w-0 rounded-lg border bg-transparent px-2 text-sm sm:w-28'
            >
              {rangeOptions.map((option) => (
                <option key={option} value={option}>
                  {option === 'custom' ? t('Custom') : option}
                </option>
              ))}
            </select>
          </div>
          {detailItems.length > 0 ? (
            <div className='grid min-w-0 gap-1 sm:max-w-[28rem]'>
              <Label
                htmlFor='quota-detail-series'
                className='text-muted-foreground text-xs'
              >
                {t('Detailed series')}
              </Label>
              <select
                id='quota-detail-series'
                value={detailItem ? detailSeriesKey(detailItem) : ''}
                onChange={(event) => setDetailKey(event.target.value)}
                className='h-8 min-w-0 rounded-lg border bg-transparent px-2 text-sm'
              >
                {detailItems.map((item) => {
                  const key = detailSeriesKey(item)
                  return (
                    <option key={key} value={key}>
                      {item.account_label || item.name} ·{' '}
                      {quotaWindowLabel(item.window_type, t)}
                    </option>
                  )
                })}
              </select>
            </div>
          ) : null}
          <div className='grid min-w-0 gap-1'>
            <Label
              htmlFor='quota-change-window-filter'
              className='text-muted-foreground text-xs'
            >
              {t('Quota window type')}
            </Label>
            <select
              id='quota-change-window-filter'
              value={windowFilter}
              onChange={(event) => setWindowFilter(event.target.value)}
              className='h-8 min-w-0 rounded-lg border bg-transparent px-2 text-sm sm:min-w-36'
            >
              <option value='all'>{t('All windows')}</option>
              {windows.map((option) => (
                <option key={option} value={option}>
                  {quotaWindowLabel(option, t)}
                </option>
              ))}
            </select>
          </div>
          <div className='grid min-w-0 gap-1'>
            <Label
              htmlFor='quota-change-status-filter'
              className='text-muted-foreground text-xs'
            >
              {t('Status')}
            </Label>
            <select
              id='quota-change-status-filter'
              value={statusFilter}
              onChange={(event) =>
                setStatusFilter(event.target.value as StatusFilter)
              }
              className='h-8 min-w-0 rounded-lg border bg-transparent px-2 text-sm sm:min-w-32'
            >
              <option value='all'>{t('All statuses')}</option>
              <option value='success'>{t('Success')}</option>
              <option value='unavailable'>{t('Unavailable')}</option>
              <option value='unsupported'>{t('Unsupported')}</option>
              <option value='error'>{t('Error')}</option>
            </select>
          </div>
        </div>
        {range === 'custom' && !detailItem ? (
          <QuotaCustomRangeControls
            range={time.customRange}
            onApply={time.setCustomRange}
          />
        ) : null}
      </CardHeader>
      <CardContent className='min-w-0 space-y-3 pt-0'>
        {query.data?.data?.source_complete === false ||
        query.data?.data?.items_complete === false ? (
          <Alert>
            <AlertTitle>{t('Incomplete quota history')}</AlertTitle>
            <AlertDescription>
              {t(
                'Some quota series are missing. Narrow the time range to load complete history.'
              )}
            </AlertDescription>
          </Alert>
        ) : null}
        {query.isPlaceholderData ? (
          <p className='text-muted-foreground text-xs' role='status'>
            {t('Loading')}
          </p>
        ) : null}
        {query.isLoading ? (
          <div className='space-y-2'>
            <Skeleton className='h-14 w-full' />
            <Skeleton className='h-14 w-full' />
          </div>
        ) : null}
        {query.isError || query.data?.success === false ? (
          <Alert variant='destructive'>
            <CircleAlert />
            <AlertTitle>{errorTitle}</AlertTitle>
            <AlertDescription className='flex flex-wrap items-center gap-2'>
              <span>{errorDescription}</span>
              {getHttpStatus(query.error) === 401 ? (
                <Button
                  variant='outline'
                  size='sm'
                  className='h-7 px-2 text-xs'
                  render={<Link to='/sign-in' />}
                >
                  {t('Sign in again')}
                </Button>
              ) : null}
            </AlertDescription>
          </Alert>
        ) : null}
        {!query.isLoading &&
        !query.isError &&
        query.data?.success !== false &&
        filteredItems.length === 0 ? (
          <div className='text-muted-foreground space-y-2 rounded-lg border border-dashed p-5 text-center text-sm'>
            <div>
              {t(
                'No account quota changes recorded yet. Enable quota sampling or query a provider account to start history.'
              )}
            </div>
            {samplingStatusQuery.data?.data ? (
              <div className='text-xs'>
                {samplingStatusQuery.data.data.enabled
                  ? t(
                      'Background quota sampling is enabled (every {{minutes}} minutes).',
                      {
                        minutes: Math.max(
                          1,
                          Math.round(
                            samplingStatusQuery.data.data.interval_seconds / 60
                          )
                        ),
                      }
                    )
                  : t(
                      'Background quota sampling is disabled; query a provider account to create the first sample.'
                    )}
              </div>
            ) : null}
          </div>
        ) : null}
        {!query.isLoading &&
        !query.isError &&
        query.data?.success !== false &&
        filteredItems.length > 0 ? (
          <>
            <div className='grid min-w-0 grid-cols-1 gap-2 sm:grid-cols-3'>
              <div className='bg-muted/20 rounded-lg border p-3'>
                <div className='text-muted-foreground text-xs'>
                  {t('Peak consumption per minute')}
                </div>
                <div className='mt-1 font-semibold'>
                  {formatMetric(
                    detailItem?.consumption?.peak_rate_per_minute,
                    detailItem,
                    t
                  )}
                </div>
              </div>
              <div className='bg-muted/20 rounded-lg border p-3'>
                <div className='text-muted-foreground text-xs'>
                  {t('Average consumption per minute')}
                </div>
                <div className='text-destructive mt-1 font-semibold'>
                  {formatMetric(
                    detailItem?.consumption?.average_rate_per_minute,
                    detailItem,
                    t
                  )}
                </div>
              </div>
              <div className='bg-muted/20 rounded-lg border p-3'>
                <div className='text-muted-foreground text-xs'>
                  {t('Quota series tracked')}
                </div>
                <div className='mt-1 font-semibold'>
                  {formatNumber(filteredItems.length)}
                </div>
              </div>
            </div>
            {detailItem ? (
              <ChannelQuotaDetailChart
                item={detailItem}
                range={range}
                onRangeChange={setRange}
                refreshEpoch={refreshEpoch}
                customRange={time.customRange}
                onCustomRangeChange={time.setCustomRange}
              />
            ) : null}
            <div className='space-y-2'>
              {filteredItems.map((item) => (
                <ChangeRow key={detailSeriesKey(item)} item={item} t={t} />
              ))}
            </div>
          </>
        ) : null}
      </CardContent>
    </Card>
  )
}
