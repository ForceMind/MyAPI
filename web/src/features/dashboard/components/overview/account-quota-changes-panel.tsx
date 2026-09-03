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
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertTitle, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { IconBadge } from '@/components/ui/icon-badge'
import {
  getChannelQuotaChanges,
  getChannelQuotaSamplingStatus,
  getCodexQuotaSeries,
} from '@/features/channels/api'
import { QuotaCustomRangeControls } from '@/features/channels/components/quota-history-trend'
import { useQuotaHistoryTime } from '@/features/channels/hooks/use-quota-history-time'
import {
  quotaWindowLabel,
  quotaSeriesKey,
  quotaHistoryRangeOptions,
  formatQuotaAmount,
} from '@/features/channels/lib/quota-history'
import type {
  ChannelQuotaChangeItem,
  ChannelQuotaHistoryRange,
} from '@/features/channels/types'
import { hasPermission } from '@/lib/admin-permissions'
import { getSelf } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { PanelWrapper } from '../ui/panel-wrapper'
import { CodexAccountQuotaChart } from './codex-account-quota-chart'

const LIMIT = 5
const DATA_LIMIT = 50
type Range = ChannelQuotaHistoryRange

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

function formatSignedAmount(
  value: number | null | undefined,
  unit: string | undefined,
  currency: string | undefined,
  t: (key: string, options?: Record<string, unknown>) => string
): string {
  if (!finite(value)) return '—'
  const sign = value > 0 ? '+' : ''
  return `${sign}${formatQuotaAmount(value, { unit, currency }, 'rate_per_minute', t)}`
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

function MovementRow(props: {
  item: ChannelQuotaChangeItem
  maxMovement: number
}) {
  const { t } = useTranslation()
  const item = props.item
  const tone = movementTone(item)
  const Icon = tone.icon
  const value = finite(item.change_per_minute)
    ? Math.abs(item.change_per_minute)
    : item.abs_change_per_minute
  const width =
    finite(value) && props.maxMovement > 0
      ? Math.max(8, Math.min(100, (value / props.maxMovement) * 100))
      : 0
  let movementColor =
    item.direction === 'decrease' ? 'bg-destructive/70' : 'bg-warning/70'
  if (item.status === 'unsupported' || item.status === 'unavailable') {
    movementColor = 'bg-muted-foreground/40'
  }

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
          <Badge
            variant={tone.badge}
            className='hidden shrink-0 sm:inline-flex'
          >
            {t(tone.label)}
          </Badge>
        </div>
        <div className='text-muted-foreground mt-1 flex min-w-0 items-center gap-2 text-[11px]'>
          <span className='truncate'>
            {item.account_label || t('Provider account')}
          </span>
          {item.window_type && (
            <span className='shrink-0'>
              · {quotaWindowLabel(item.window_type, t)}
            </span>
          )}
          {item.plan_type && (
            <span className='shrink-0'>· {item.plan_type}</span>
          )}
        </div>
        <div className='bg-muted/50 mt-2 h-1.5 overflow-hidden rounded-full'>
          <div
            className={cn(
              'h-full rounded-full transition-[width]',
              movementColor
            )}
            style={{ width: `${width}%` }}
            aria-hidden='true'
          />
        </div>
      </div>
      <div className='flex max-w-[48%] min-w-0 shrink-0 flex-col items-end gap-1 text-right'>
        <span
          className={cn(
            'max-w-full break-words font-mono text-sm font-semibold tabular-nums',
            tone.className
          )}
        >
          {formatSignedAmount(
            item.change_per_minute,
            item.unit,
            item.currency,
            t
          )}
        </span>
        <span className='text-muted-foreground max-w-full font-mono text-[11px] break-words tabular-nums'>
          {formatQuotaAmount(item.current_available, item, 'available', t)}
        </span>
      </div>
    </li>
  )
}

export function AccountQuotaChangesPanel() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const setUser = useAuthStore((state) => state.auth.setUser)
  // Keep dashboard query caches isolated across login sessions. Without the
  // identity in the key, a successful admin response could remain in the
  // TanStack cache and be rendered immediately when another session with the
  // same permission is opened in the same tab.
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const capabilityRefreshKey = useRef<string | null>(null)
  const time = useQuotaHistoryTime()
  const { range, setRange } = time
  const [refreshEpoch, setRefreshEpoch] = useState(0)

  // A non-empty permission matrix can still be stale when an administrator's
  // role policy changes while the SPA remains open. Refresh the self profile
  // once per user/session when this panel is mounted; the backend remains the
  // authority and an explicit deny is preserved in the returned matrix.
  useEffect(() => {
    if (
      !user ||
      !sessionId ||
      user.role < ROLE.ADMIN ||
      user.role >= ROLE.SUPER_ADMIN
    ) {
      return
    }
    const refreshKey = `${user.id}:${sessionId}`
    if (capabilityRefreshKey.current === refreshKey) return
    capabilityRefreshKey.current = refreshKey
    let active = true
    void getSelf()
      .then((response) => {
        const candidate = response?.data
        if (
          !active ||
          !response?.success ||
          !candidate ||
          typeof candidate !== 'object' ||
          candidate.id !== user.id ||
          typeof candidate.role !== 'number'
        ) {
          return
        }
        setUser(candidate as typeof user)
      })
      .catch(() => {
        // The quota query below still reports the current permission/session
        // error; metadata refresh is best-effort and must not hide the panel.
      })
    return () => {
      active = false
    }
  }, [sessionId, setUser, user])
  // The API route is protected by the same resolved permission matrix as the
  // rest of the admin channel surface. Using the capability payload here
  // avoids showing a panel to users who would be rejected by an explicit
  // Casbin deny or by the outer AdminAuth middleware.
  const canReadChannels = hasPermission(user, 'channel', 'read')

  const query = useQuery({
    queryKey: [
      'dashboard',
      'account-quota-changes',
      time.params,
      DATA_LIMIT,
      user?.id ?? null,
      sessionId,
      canReadChannels,
    ],
    queryFn: () =>
      getChannelQuotaChanges({
        ...time.params,
        // This bounded list is only the compact latest-movement list.
        limit: DATA_LIMIT,
        sort: 'abs_change_per_minute',
      }),
    enabled: canReadChannels,
    staleTime: 60 * 1000,
    placeholderData: (previous, previousQuery) =>
      previousQuery?.queryKey[4] === (user?.id ?? null) &&
      previousQuery?.queryKey[5] === sessionId
        ? previous
        : undefined,
    // Keep the overview useful while it remains open after a background
    // sampler run. TanStack Query pauses interval work in hidden tabs by
    // default, so this does not create background polling for idle clients.
    refetchInterval: 60 * 1000,
    retry: false,
  })
  const codexQuery = useQuery({
    queryKey: [
      'dashboard',
      'codex-quota-series',
      user?.id ?? null,
      sessionId,
      time.params,
    ],
    queryFn: () => getCodexQuotaSeries({ ...time.params, limit: 2000 }),
    enabled: canReadChannels,
    staleTime: 60 * 1000,
    refetchInterval: 60 * 1000,
    retry: false,
    placeholderData: (previous, previousQuery) =>
      previousQuery?.queryKey[2] === (user?.id ?? null) &&
      previousQuery?.queryKey[3] === sessionId
        ? previous
        : undefined,
  })
  const samplingStatusQuery = useQuery({
    queryKey: [
      'dashboard',
      'channel-quota-sampling-status',
      user?.id ?? null,
      sessionId,
      canReadChannels,
    ],
    queryFn: () => getChannelQuotaSamplingStatus(),
    enabled: canReadChannels,
    retry: false,
    staleTime: 5 * 60 * 1000,
  })
  const refreshAll = () => {
    // Child quota history queries include refreshEpoch. A dashboard refresh
    // therefore refreshes the list, sampler state, and visible chart together.
    setRefreshEpoch((value) => value + 1)
    void query.refetch()
    void codexQuery.refetch()
    void samplingStatusQuery.refetch()
  }

  const allItems = useMemo(
    () => query.data?.data?.items ?? [],
    [query.data?.data?.items]
  )
  const items = useMemo(() => allItems.slice(0, LIMIT), [allItems])
  const codexItems = codexQuery.data?.data?.items ?? []
  const maxMovement = items.reduce((max, item) => {
    if (finite(item.abs_change_per_minute)) {
      return Math.max(max, item.abs_change_per_minute)
    }
    if (finite(item.change_per_minute)) {
      return Math.max(max, Math.abs(item.change_per_minute))
    }
    return max
  }, 0)

  if (!canReadChannels) {
    // Keep ordinary users completely unaware of upstream quota data, while
    // giving an administrator with a custom/partial permission matrix a
    // visible explanation instead of an unexplained blank dashboard region.
    if (!user || user.role < ROLE.ADMIN) return null
    return (
      <PanelWrapper
        title={
          <span className='flex items-center gap-2'>
            <IconBadge tone='warning' size='sm'>
              <Activity />
            </IconBadge>
            {t('Quota consumption')}
          </span>
        }
        description={t(
          'Observed provider account consumption and latest quota status'
        )}
        empty
        emptyMessage={t(
          'Administrator permission required to view account quota changes'
        )}
      />
    )
  }

  const queryFailed = query.isError || query.data?.success === false
  const codexFailed = codexQuery.isError || codexQuery.data?.success === false
  const status = getHttpStatus(query.error) ?? getHttpStatus(codexQuery.error)
  let errorMessage = t('Unable to load account quota changes')
  if (status === 401) {
    errorMessage = t(
      'Your session is missing or expired. Sign in again to load provider account quota.'
    )
  }
  if (status === 403) {
    errorMessage = t('Your account does not have permission to read channels.')
  }
  const incomplete =
    codexQuery.data?.data?.items_complete === false ||
    codexQuery.data?.data?.source_complete === false

  return (
    <PanelWrapper
      title={
        <span className='flex items-center gap-2'>
          <IconBadge tone='info' size='sm'>
            <Activity />
          </IconBadge>
          {t('Quota consumption')}
        </span>
      }
      description={t(
        'Observed provider account consumption and latest quota status'
      )}
      loading={query.isLoading && codexQuery.isLoading}
      height='h-64'
      contentClassName='space-y-3'
      headerActions={
        <div className='flex items-center gap-1'>
          <select
            aria-label={t('Time range')}
            className='text-foreground h-7 min-w-0 rounded-md border bg-transparent px-1.5 text-xs'
            value={range}
            onChange={(event) => setRange(event.target.value as Range)}
          >
            {quotaHistoryRangeOptions.map((option) => (
              <option key={option} value={option}>
                {option === 'custom' ? t('Custom') : option}
              </option>
            ))}
          </select>
          <Button
            variant='ghost'
            size='sm'
            className='size-7 p-0'
            onClick={refreshAll}
            disabled={
              query.isFetching ||
              codexQuery.isFetching ||
              samplingStatusQuery.isFetching
            }
            aria-label={t('Refresh')}
          >
            <RotateCw
              className={cn('size-3.5', query.isFetching && 'animate-spin')}
            />
          </Button>
          <Button
            variant='ghost'
            size='sm'
            className='h-7 gap-1 px-2 text-xs'
            render={<Link to='/channels' />}
          >
            {t('Channels')}
            <ExternalLink data-icon='inline-end' />
          </Button>
        </div>
      }
    >
      {queryFailed || codexFailed ? (
        <Alert variant='destructive'>
          <AlertTitle>{errorMessage}</AlertTitle>
          <AlertDescription>
            <p>
              {t(
                'Try a shorter time range or refresh to retry. Existing history is retained.'
              )}
            </p>
            {status === 401 ? (
              <Button
                variant='outline'
                size='sm'
                render={<Link to='/sign-in' />}
              >
                {t('Sign in again')}
              </Button>
            ) : null}
          </AlertDescription>
        </Alert>
      ) : null}
      {incomplete ? (
        <Alert>
          <AlertTitle>{t('Incomplete quota history')}</AlertTitle>
          <AlertDescription>
            {t(
              'Some quota series are missing. Narrow the time range to load complete history.'
            )}
          </AlertDescription>
        </Alert>
      ) : null}
      {query.isPlaceholderData || codexQuery.isPlaceholderData ? (
        <p className='text-muted-foreground text-xs' role='status'>
          {t('Loading')}
        </p>
      ) : null}
      {range === 'custom' &&
      !codexItems.some((item) => item.metric_type === 'codex_rate_limit') ? (
        <QuotaCustomRangeControls
          range={time.customRange}
          onApply={time.setCustomRange}
        />
      ) : null}
      <CodexAccountQuotaChart
        items={codexItems}
        range={range}
        onRangeChange={setRange}
        refreshEpoch={refreshEpoch}
        customRange={time.customRange}
        onCustomRangeChange={time.setCustomRange}
      />
      {!queryFailed &&
        (items.length === 0 ? (
          <div className='text-muted-foreground rounded-xl border border-dashed p-4 text-center text-sm'>
            {samplingStatusQuery.data?.data?.enabled
              ? t(
                  'No account quota changes recorded yet. Background sampling is enabled and will populate this panel after the next interval.'
                )
              : t(
                  'No account quota changes recorded yet. Enable quota sampling or query a provider account to start history.'
                )}
          </div>
        ) : (
          <ul
            className='max-h-64 min-w-0 space-y-2 overflow-y-auto pr-1'
            aria-label={t('Account quota changes')}
          >
            {items.map((item) => (
              <MovementRow
                key={quotaSeriesKey(item)}
                item={item}
                maxMovement={maxMovement}
              />
            ))}
          </ul>
        ))}
    </PanelWrapper>
  )
}
