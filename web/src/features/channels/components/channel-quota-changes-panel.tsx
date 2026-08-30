/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQuery } from '@tanstack/react-query'
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
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { formatNumber, formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import { getChannelQuotaChanges } from '../api'
import type { ChannelQuotaChangeItem } from '../types'

type Range = '24h' | '7d' | '30d' | '90d'
type StatusFilter = 'all' | 'success' | 'unavailable' | 'error'

const rangeOptions: Range[] = ['24h', '7d', '30d', '90d']

function finite(value: number | null | undefined): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}

function formatMetric(value: number | null | undefined, item?: ChannelQuotaChangeItem) {
  if (!finite(value)) return '-'
  if (item?.unit === 'percent') return `${value.toFixed(2)}%/min`
  if (item?.unit === 'usd' || item?.currency) {
    return `${formatCurrencyFromUSD(value, { digitsLarge: 2, digitsSmall: 4, abbreviate: false })}/min`
  }
  const unit = item?.unit ? ` ${item.unit}` : ''
  return `${formatNumber(value)}${unit}/min`
}

function formatAvailable(value: number | null | undefined, item: ChannelQuotaChangeItem) {
  if (!finite(value)) return '-'
  if (item.unit === 'percent') return `${value.toFixed(1)}%`
  if (item.unit === 'usd' || item.currency) {
    return formatCurrencyFromUSD(value, { digitsLarge: 2, digitsSmall: 4, abbreviate: false })
  }
  return `${formatNumber(value)}${item.unit ? ` ${item.unit}` : ''}`
}

function directionLabel(direction: ChannelQuotaChangeItem['direction'], t: (key: string) => string) {
  if (direction === 'increase') return t('Increasing')
  if (direction === 'decrease') return t('Decreasing')
  if (direction === 'stable') return t('Stable')
  return t('Unknown')
}

function statusVariant(status: string | undefined) {
  if (status === 'error') return 'destructive' as const
  if (status === 'unavailable') return 'warning' as const
  return 'outline' as const
}

function DirectionIcon({ direction }: { direction: ChannelQuotaChangeItem['direction'] }) {
  if (direction === 'increase') return <ArrowUpRight className='size-4 shrink-0 text-success' aria-hidden='true' />
  if (direction === 'decrease') return <ArrowDownRight className='size-4 shrink-0 text-destructive' aria-hidden='true' />
  if (direction === 'stable') return <Minus className='size-4 shrink-0 text-muted-foreground' aria-hidden='true' />
  return <CircleAlert className='size-4 shrink-0 text-muted-foreground' aria-hidden='true' />
}

function ChangeRow({ item, t }: { item: ChannelQuotaChangeItem; t: (key: string) => string }) {
  const direction = item.direction ?? 'unknown'
  const current = formatAvailable(item.current_available, item)
  const previous = formatAvailable(item.previous_available, item)
  return (
    <div className='grid min-w-0 gap-2 rounded-lg border bg-card/60 p-3 sm:grid-cols-[minmax(0,1.5fr)_minmax(7rem,0.8fr)_minmax(8rem,1fr)_auto] sm:items-center'>
      <div className='min-w-0'>
        <div className='flex min-w-0 items-center gap-2'>
          <Activity className='size-4 shrink-0 text-muted-foreground' aria-hidden='true' />
          <span className='truncate font-medium'>{item.account_label || item.name}</span>
        </div>
        <div className='mt-1 flex min-w-0 flex-wrap items-center gap-1.5 text-xs text-muted-foreground'>
          {item.account_label && item.name !== item.account_label ? <span className='truncate'>{item.name}</span> : null}
          {item.source ? <Badge variant='outline'>{item.source}</Badge> : null}
          {item.window_type ? <Badge variant='secondary'>{item.window_type}</Badge> : null}
          {item.metric_type ? <span>{item.metric_type}</span> : null}
        </div>
      </div>
      <div className='text-xs text-muted-foreground'>
        <div>{t('Current')}</div>
        <div className='font-medium text-foreground'>{current}</div>
        {previous !== '-' ? <div className='mt-0.5'>{t('Previous')}: {previous}</div> : null}
      </div>
      <div className='flex min-w-0 items-center gap-2'>
        <DirectionIcon direction={direction} />
        <div className='min-w-0'>
          <div className={cn('truncate font-semibold', direction === 'decrease' && 'text-destructive', direction === 'increase' && 'text-success')}>
            {formatMetric(item.change_per_minute, item)}
          </div>
          <div className='text-xs text-muted-foreground'>{directionLabel(direction, t)}</div>
        </div>
      </div>
      <div className='text-left text-xs text-muted-foreground sm:text-right'>
        {item.status ? <Badge variant={statusVariant(item.status)}>{item.status}</Badge> : null}
        {item.observed_at ? <div className='mt-1'>{formatTimestampToDate(item.observed_at)}</div> : null}
      </div>
    </div>
  )
}

export function ChannelQuotaChangesPanel() {
  const { t } = useTranslation()
  const [range, setRange] = useState<Range>('24h')
  const [windowFilter, setWindowFilter] = useState('all')
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all')
  const query = useQuery({
    queryKey: ['channel-quota-changes', range],
    queryFn: () => getChannelQuotaChanges({ range, limit: 2000, sort: 'abs_change_per_minute' }),
    retry: false,
    staleTime: 60 * 1000,
  })

  const items = useMemo(
    () => query.data?.data?.items ?? [],
    [query.data?.data?.items]
  )
  const windows = useMemo(
    () => [...new Set(items.map((item) => item.window_type).filter(Boolean) as string[])].sort(),
    [items]
  )
  const filteredItems = useMemo(
    () => items
      .filter((item) => windowFilter === 'all' || item.window_type === windowFilter)
      .filter((item) => statusFilter === 'all' || item.status === statusFilter)
      .sort((a, b) => (b.abs_change_per_minute ?? Math.abs(b.change_per_minute ?? 0)) - (a.abs_change_per_minute ?? Math.abs(a.change_per_minute ?? 0))),
    [items, statusFilter, windowFilter]
  )
  const maxItem = filteredItems.find((item) => finite(item.abs_change_per_minute) || finite(item.change_per_minute))
  const maxDrop = filteredItems.find((item) => item.direction === 'decrease')

  return (
    <Card className='mb-3 min-w-0' data-testid='channel-quota-changes-panel'>
      <CardHeader className='gap-3 pb-3'>
        <div className='flex min-w-0 flex-wrap items-start justify-between gap-3'>
          <div className='min-w-0'>
            <CardTitle className='flex items-center gap-2 text-base'>
              <Activity className='size-4 shrink-0 text-primary' aria-hidden='true' />
              <span>{t('Account quota changes')}</span>
            </CardTitle>
            <CardDescription className='mt-1 max-w-2xl text-xs leading-5'>
              {t('Upstream account quota only; MyAPI user balances are not included. Sorted by the largest absolute change per minute.')}
            </CardDescription>
          </div>
          <Button type='button' variant='ghost' size='icon-xs' onClick={() => void query.refetch()} disabled={query.isFetching} aria-label={t('Refresh')}>
            <RefreshCw className={cn('size-4', query.isFetching && 'animate-spin')} aria-hidden='true' />
          </Button>
        </div>
        <div className='grid min-w-0 grid-cols-1 gap-2 sm:flex sm:flex-wrap sm:items-end'>
          <div className='grid min-w-0 gap-1'>
            <Label htmlFor='quota-change-range' className='text-xs text-muted-foreground'>{t('Window')}</Label>
            <select id='quota-change-range' value={range} onChange={(event) => setRange(event.target.value as Range)} className='h-8 min-w-0 rounded-lg border bg-transparent px-2 text-sm sm:w-28'>
              {rangeOptions.map((option) => <option key={option} value={option}>{option}</option>)}
            </select>
          </div>
          <div className='grid min-w-0 gap-1'>
            <Label htmlFor='quota-change-window-filter' className='text-xs text-muted-foreground'>{t('Quota window type')}</Label>
            <select id='quota-change-window-filter' value={windowFilter} onChange={(event) => setWindowFilter(event.target.value)} className='h-8 min-w-0 rounded-lg border bg-transparent px-2 text-sm sm:min-w-36'>
              <option value='all'>{t('All windows')}</option>
              {windows.map((option) => <option key={option} value={option}>{option}</option>)}
            </select>
          </div>
          <div className='grid min-w-0 gap-1'>
            <Label htmlFor='quota-change-status-filter' className='text-xs text-muted-foreground'>{t('Status')}</Label>
            <select id='quota-change-status-filter' value={statusFilter} onChange={(event) => setStatusFilter(event.target.value as StatusFilter)} className='h-8 min-w-0 rounded-lg border bg-transparent px-2 text-sm sm:min-w-32'>
              <option value='all'>{t('All statuses')}</option>
              <option value='success'>{t('Success')}</option>
              <option value='unavailable'>{t('Unavailable')}</option>
              <option value='error'>{t('Error')}</option>
            </select>
          </div>
        </div>
      </CardHeader>
      <CardContent className='min-w-0 space-y-3 pt-0'>
        {query.isLoading ? <div className='space-y-2'><Skeleton className='h-14 w-full' /><Skeleton className='h-14 w-full' /></div> : null}
        {query.isError || query.data?.success === false ? (
          <Alert variant='destructive'>
            <CircleAlert />
            <AlertTitle>{t('Unable to load account quota changes')}</AlertTitle>
            <AlertDescription>{query.error instanceof Error ? query.error.message : query.data?.message || t('Please try again later.')}</AlertDescription>
          </Alert>
        ) : null}
        {!query.isLoading && !query.isError && query.data?.success !== false && filteredItems.length === 0 ? (
          <div className='rounded-lg border border-dashed p-5 text-center text-sm text-muted-foreground'>{t('No account quota changes yet')}</div>
        ) : null}
        {!query.isLoading && !query.isError && query.data?.success !== false && filteredItems.length > 0 ? (
          <>
            <div className='grid min-w-0 grid-cols-1 gap-2 sm:grid-cols-3'>
              <div className='rounded-lg border bg-muted/20 p-3'><div className='text-xs text-muted-foreground'>{t('Largest change per minute')}</div><div className='mt-1 font-semibold'>{formatMetric(maxItem?.abs_change_per_minute ?? (maxItem ? Math.abs(maxItem.change_per_minute ?? 0) : null), maxItem)}</div></div>
              <div className='rounded-lg border bg-muted/20 p-3'><div className='text-xs text-muted-foreground'>{t('Largest decrease per minute')}</div><div className='mt-1 font-semibold text-destructive'>{formatMetric(maxDrop?.change_per_minute != null ? Math.abs(maxDrop.change_per_minute) : null, maxDrop)}</div></div>
              <div className='rounded-lg border bg-muted/20 p-3'><div className='text-xs text-muted-foreground'>{t('Accounts tracked')}</div><div className='mt-1 font-semibold'>{formatNumber(filteredItems.length)}</div></div>
            </div>
            <div className='space-y-2'>{filteredItems.map((item) => <ChangeRow key={`${item.channel_id}:${item.metric_type ?? ''}:${item.window_type ?? ''}:${item.source ?? ''}`} item={item} t={t} />)}</div>
          </>
        ) : null}
      </CardContent>
    </Card>
  )
}
