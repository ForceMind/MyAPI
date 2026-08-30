/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  Activity,
  ArrowDownRight,
  ArrowUpRight,
  CircleAlert,
  ExternalLink,
  Minus,
  RotateCw,
} from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { IconBadge } from '@/components/ui/icon-badge'
import { getChannelQuotaChanges, getChannelQuotaSamplingStatus } from '@/features/channels/api'
import type { ChannelQuotaChangeItem } from '@/features/channels/types'
import { hasPermission } from '@/lib/admin-permissions'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { PanelWrapper } from '../ui/panel-wrapper'

const RANGE = '24h' as const
const LIMIT = 5

function getHttpStatus(error: unknown): number | undefined {
  if (!error || typeof error !== 'object') return undefined
  const response = (error as { response?: unknown }).response
  if (!response || typeof response !== 'object') return undefined
  const status = (response as { status?: unknown }).status
  return typeof status === 'number' ? status : undefined
}

function finite(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}

function metricIdentity(item: ChannelQuotaChangeItem): string {
  return [item.metric_type, item.unit, item.currency, item.window_type]
    .map((value) => value || '')
    .join('|')
}

function formatAmount(
  value: number | null | undefined,
  unit?: string,
  currency?: string
): string {
  if (!finite(value)) return '—'
  const formatted = new Intl.NumberFormat(undefined, {
    maximumFractionDigits: 2,
  }).format(value)
  const suffix = currency || unit
  return suffix ? `${formatted} ${suffix}` : formatted
}

function formatSignedAmount(
  value: number | null | undefined,
  unit?: string,
  currency?: string
): string {
  if (!finite(value)) return '—'
  const sign = value > 0 ? '+' : ''
  return `${sign}${formatAmount(value, unit, currency)}`
}

function movementTone(item: ChannelQuotaChangeItem): {
  icon: typeof ArrowDownRight
  badge: 'destructive' | 'warning' | 'secondary'
  className: string
  label: string
} {
  if (item.status === 'error') {
    return {
      icon: CircleAlert,
      badge: 'destructive',
      className: 'text-destructive',
      label: 'Error',
    }
  }
  if (item.status === 'unsupported') {
    return {
      icon: Minus,
      badge: 'warning',
      className: 'text-warning',
      label: 'Unsupported',
    }
  }
  if (item.status === 'unavailable') {
    return {
      icon: Minus,
      badge: 'secondary',
      className: 'text-muted-foreground',
      label: 'Unavailable',
    }
  }
  if (item.direction === 'decrease') {
    return {
      icon: ArrowDownRight,
      badge: 'destructive',
      className: 'text-destructive',
      label: 'Decreasing',
    }
  }
  if (item.direction === 'increase') {
    return {
      icon: ArrowUpRight,
      badge: 'warning',
      className: 'text-warning',
      label: 'Increasing',
    }
  }
  return {
    icon: Minus,
    badge: 'secondary',
    className: 'text-muted-foreground',
    label: 'Stable',
  }
}

function MovementRow(props: { item: ChannelQuotaChangeItem; maxMovement: number }) {
  const { t } = useTranslation()
  const item = props.item
  const tone = movementTone(item)
  const Icon = tone.icon
  const value = finite(item.change_per_minute)
    ? Math.abs(item.change_per_minute)
    : item.abs_change_per_minute
  const width = finite(value) && props.maxMovement > 0
    ? Math.max(8, Math.min(100, (value / props.maxMovement) * 100))
    : 0

  return (
    <li className='group flex min-w-0 items-center gap-3 rounded-xl border px-3 py-2.5 sm:px-4'>
      <span
        className={cn(
          'flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted/60',
          tone.className
        )}
      >
        <Icon className='size-4' aria-hidden='true' />
      </span>
      <div className='min-w-0 flex-1'>
        <div className='flex min-w-0 items-center gap-2'>
          <Link
            to='/channels'
            search={{ filter: item.name }}
            className='min-w-0 truncate text-sm font-medium hover:underline'
            title={item.name}
          >
            {item.name}
          </Link>
          <Badge variant={tone.badge} className='hidden shrink-0 sm:inline-flex'>
            {t(tone.label)}
          </Badge>
        </div>
        <div className='text-muted-foreground mt-1 flex min-w-0 items-center gap-2 text-[11px]'>
          <span className='truncate'>{item.account_label || t('Provider account')}</span>
          {item.window_type && <span className='shrink-0'>· {item.window_type}</span>}
          {item.plan_type && <span className='shrink-0'>· {item.plan_type}</span>}
        </div>
        <div className='bg-muted/50 mt-2 h-1.5 overflow-hidden rounded-full'>
          <div
            className={cn(
              'h-full rounded-full transition-[width]',
              item.status === 'unsupported' || item.status === 'unavailable'
                ? 'bg-muted-foreground/40'
                : item.direction === 'decrease'
                  ? 'bg-destructive/70'
                  : 'bg-warning/70'
            )}
            style={{ width: `${width}%` }}
            aria-hidden='true'
          />
        </div>
      </div>
      <div className='flex min-w-0 max-w-[48%] shrink-0 flex-col items-end gap-1 text-right'>
        <span className={cn('max-w-full break-words font-mono text-sm font-semibold tabular-nums', tone.className)}>
          {formatSignedAmount(item.change_per_minute, item.unit, item.currency)}
          <span className='text-muted-foreground ml-1 text-[10px] font-normal'>/min</span>
        </span>
        <span className='text-muted-foreground max-w-full break-words font-mono text-[11px] tabular-nums'>
          {formatAmount(item.current_available, item.unit, item.currency)}
        </span>
      </div>
    </li>
  )
}

export function AccountQuotaChangesPanel() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  // The API route is protected by the same resolved permission matrix as the
  // rest of the admin channel surface. Using the capability payload here
  // avoids showing a panel to users who would be rejected by an explicit
  // Casbin deny or by the outer AdminAuth middleware.
  const canReadChannels = hasPermission(user, 'channel', 'read')

  const query = useQuery({
    queryKey: ['dashboard', 'account-quota-changes', RANGE, LIMIT],
    queryFn: () =>
      getChannelQuotaChanges({
        range: RANGE,
        limit: LIMIT,
        sort: 'abs_change_per_minute',
      }, { skipAuthRefresh: true }),
    enabled: canReadChannels,
    staleTime: 60 * 1000,
    // Keep the overview useful while it remains open after a background
    // sampler run. TanStack Query pauses interval work in hidden tabs by
    // default, so this does not create background polling for idle clients.
    refetchInterval: 60 * 1000,
    retry: false,
  })
  const samplingStatusQuery = useQuery({
    queryKey: ['dashboard', 'channel-quota-sampling-status'],
    queryFn: () => getChannelQuotaSamplingStatus({ skipAuthRefresh: true }),
    enabled: canReadChannels,
    retry: false,
    staleTime: 5 * 60 * 1000,
  })

  const items = useMemo(
    () => query.data?.data?.items ?? [],
    [query.data?.data?.items]
  )
  const firstMetric = items.find((item) => finite(item.change_per_minute))
  // Do not compare raw numbers from different units (for example USD and
  // percent). The movement list remains complete; summary cards use the
  // first metric's homogeneous series only, preventing a wrong suffix/value.
  const summaryItems = useMemo(
    () =>
      firstMetric
        ? items.filter(
            (item) => metricIdentity(item) === metricIdentity(firstMetric)
          )
        : [],
    [items, firstMetric]
  )
  const maxDrop = useMemo(
    () =>
      summaryItems.reduce<number | null>((max, item) => {
        if (!finite(item.change_per_minute) || item.change_per_minute >= 0) return max
        const value = Math.abs(item.change_per_minute)
        return max == null ? value : Math.max(max, value)
      }, null),
    [summaryItems]
  )
  const maxIncrease = useMemo(
    () =>
      summaryItems.reduce<number | null>((max, item) => {
        if (!finite(item.change_per_minute) || item.change_per_minute <= 0) return max
        return max == null ? item.change_per_minute : Math.max(max, item.change_per_minute)
      }, null),
    [summaryItems]
  )
  const maxMovement = items.reduce((max, item) => {
    if (finite(item.abs_change_per_minute)) {
      return Math.max(max, item.abs_change_per_minute)
    }
    if (finite(item.change_per_minute)) {
      return Math.max(max, Math.abs(item.change_per_minute))
    }
    return max
  }, 0)

  if (!canReadChannels) return null

  if (query.isError || query.data?.success === false) {
    const status = getHttpStatus(query.error)
    const isSessionError = status === 401
    const isPermissionError = status === 403
    const errorMessage = isSessionError
      ? t('Your session is missing or expired. Sign in again to load provider account quota.')
      : isPermissionError
        ? t('Your account does not have permission to read channels.')
        : t('Unable to load account quota changes')
    return (
      <PanelWrapper
        title={
          <span className='flex items-center gap-2'>
            <IconBadge tone='warning' size='sm'><Activity /></IconBadge>
            {t('Account quota changes')}
          </span>
        }
        description={t('Largest provider account quota movements per minute')}
        empty
        emptyMessage={errorMessage}
        headerActions={
          isSessionError ? (
            <Button variant='outline' size='sm' className='h-7 px-2 text-xs' render={<Link to='/sign-in' />}>
              {t('Sign in again')}
            </Button>
          ) : (
            <Button variant='ghost' size='sm' className='size-7 p-0' onClick={() => void query.refetch()} aria-label={t('Retry')}>
              <RotateCw className='size-3.5' />
            </Button>
          )
        }
      />
    )
  }

  return (
    <PanelWrapper
      title={
        <span className='flex items-center gap-2'>
          <IconBadge tone='info' size='sm'><Activity /></IconBadge>
          {t('Account quota changes')}
        </span>
      }
      description={t('Largest provider account quota movements per minute')}
      loading={query.isLoading}
      empty={!query.isLoading && items.length === 0}
      emptyMessage={samplingStatusQuery.data?.data?.enabled
        ? t('No account quota changes recorded yet. Background sampling is enabled and will populate this panel after the next interval.')
        : t('No account quota changes recorded yet. Enable quota sampling or query a provider account to start history.')}
      height='h-64'
      contentClassName='space-y-3'
      headerActions={
        <div className='flex items-center gap-1'>
          <Button variant='ghost' size='sm' className='size-7 p-0' onClick={() => void query.refetch()} disabled={query.isFetching} aria-label={t('Refresh')}>
            <RotateCw className={cn('size-3.5', query.isFetching && 'animate-spin')} />
          </Button>
          <Button variant='ghost' size='sm' className='h-7 gap-1 px-2 text-xs' render={<Link to='/channels' />}>
            {t('Channels')}<ExternalLink data-icon='inline-end' />
          </Button>
        </div>
      }
    >
      <div className='grid grid-cols-2 gap-2'>
        <div className='bg-destructive/5 rounded-xl border border-destructive/15 px-3 py-2'>
          <div className='text-muted-foreground text-[11px]'>{t('Max drop / minute')}</div>
          <div className='text-destructive mt-1 font-mono text-sm font-semibold tabular-nums'>
            {maxDrop == null ? '—' : `-${formatAmount(maxDrop, firstMetric?.unit, firstMetric?.currency)}`}
          </div>
        </div>
        <div className='bg-warning/5 rounded-xl border border-warning/15 px-3 py-2'>
          <div className='text-muted-foreground text-[11px]'>{t('Max increase / minute')}</div>
          <div className='text-warning mt-1 font-mono text-sm font-semibold tabular-nums'>
            {formatAmount(maxIncrease, firstMetric?.unit, firstMetric?.currency)}
          </div>
        </div>
      </div>
      <ul
        className='max-h-64 min-w-0 space-y-2 overflow-y-auto pr-1'
        aria-label={t('Account quota changes')}
      >
        {items.map((item) => <MovementRow key={`${item.channel_id}-${item.window_type ?? ''}-${item.metric_type ?? ''}`} item={item} maxMovement={maxMovement} />)}
      </ul>
    </PanelWrapper>
  )
}
