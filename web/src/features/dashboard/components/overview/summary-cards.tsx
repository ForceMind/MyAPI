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
import { ArrowRight, Flame, ShieldCheck, TrendingDown } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { StaggerContainer, StaggerItem } from '@/components/page-transition'
import { Button } from '@/components/ui/button'
import { getChannelRouting } from '@/features/channel-routing/api'
import {
  getRecordedRequestSummary,
  getUserQuotaDates,
} from '@/features/dashboard/api'
import { useSummaryCardsConfig } from '@/features/dashboard/hooks/use-dashboard-config'
import { buildQueryParams } from '@/features/dashboard/lib/filters'
import type { QuotaDataItem } from '@/features/dashboard/types'
import { useStatus } from '@/hooks/use-status'
import { hasPermission } from '@/lib/admin-permissions'
import { getCurrencyLabel, isCurrencyDisplayEnabled } from '@/lib/currency'
import { formatNumber, formatQuota } from '@/lib/format'
import { ROLE } from '@/lib/roles'
import { SELF_USE_MINIMAL } from '@/lib/self-use-build'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { useRollingDayWindow } from '../../hooks/use-rolling-day-window'
import { StatCard } from '../ui/stat-card'

const SUMMARY_SPARKLINE_BUCKETS = 12

type SummarySparklineKey = 'balance' | 'usage' | 'requests'

function getBucketIndex(
  timestamp: number,
  start: number,
  end: number,
  bucketCount: number
): number {
  if (end <= start) return 0
  const ratio = (timestamp - start) / (end - start)
  return Math.min(bucketCount - 1, Math.max(0, Math.floor(ratio * bucketCount)))
}

function buildSummarySparklines(
  data: QuotaDataItem[],
  currentBalance: number,
  start: number,
  end: number
): Record<SummarySparklineKey, number[]> {
  const usage = Array.from({ length: SUMMARY_SPARKLINE_BUCKETS }, () => 0)
  const requests = Array.from({ length: SUMMARY_SPARKLINE_BUCKETS }, () => 0)

  for (const item of data) {
    const timestamp = Number(item.created_at) || start
    const index = getBucketIndex(
      timestamp,
      start,
      end,
      SUMMARY_SPARKLINE_BUCKETS
    )
    usage[index] += Number(item.quota) || 0
    requests[index] += Number(item.count) || 0
  }

  let balance = currentBalance
  const balanceTrend = Array.from(
    { length: SUMMARY_SPARKLINE_BUCKETS },
    () => 0
  )

  for (let index = SUMMARY_SPARKLINE_BUCKETS - 1; index >= 0; index--) {
    balanceTrend[index] = Math.max(0, balance)
    balance += usage[index]
  }

  return {
    balance: balanceTrend,
    usage,
    requests,
  }
}

function getRunwayDays(
  remainQuota: number,
  recentUsage: number
): number | null {
  if (remainQuota <= 0 || recentUsage <= 0) return null
  const days = remainQuota / recentUsage
  if (!Number.isFinite(days)) return null
  return days
}

type HealthLevel = 'healthy' | 'caution' | 'critical'

function getHealthLevel(remainQuota: number, recentUsage: number): HealthLevel {
  if (remainQuota <= 0) return 'critical'
  const days = getRunwayDays(remainQuota, recentUsage)
  if (days !== null && days < 3) return 'caution'
  return 'healthy'
}

const HEALTH_CONFIG: Record<
  HealthLevel,
  { dotClass: string; labelKey: string }
> = {
  healthy: {
    dotClass: 'bg-success',
    labelKey: 'Healthy',
  },
  caution: {
    dotClass: 'bg-warning',
    labelKey: 'Low balance',
  },
  critical: {
    dotClass: 'bg-destructive',
    labelKey: 'Balance depleted',
  },
}

export function SummaryCards() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const { status, loading } = useStatus()

  const summaryTimeRange = useRollingDayWindow()
  const summaryQueryParams = useMemo(
    () =>
      buildQueryParams(summaryTimeRange, {
        time_granularity: 'hour',
      }),
    [summaryTimeRange]
  )
  const remainQuota = Number(user?.quota ?? 0)
  const isAdmin = Boolean(user?.role && user.role >= ROLE.ADMIN)
  const canReadChannels = hasPermission(user, 'channel', 'read')
  const canReadOperationalChannels = isAdmin && canReadChannels

  const usageTrendQuery = useQuery({
    queryKey: [
      'dashboard',
      'overview',
      'summary-sparklines',
      user?.id ?? null,
      sessionId,
      summaryTimeRange.start_timestamp,
      summaryTimeRange.end_timestamp,
      summaryQueryParams.timezone_offset,
    ],
    queryFn: async () => getUserQuotaDates(summaryQueryParams),
    enabled: Boolean(user) && !isAdmin,
    staleTime: 60 * 1000,
  })

  const requestSummaryQuery = useQuery({
    queryKey: [
      'dashboard',
      'overview',
      'recorded-request-summary',
      user?.id ?? null,
      sessionId,
      isAdmin,
      summaryTimeRange.start_timestamp,
      summaryTimeRange.end_timestamp,
    ],
    queryFn: () => getRecordedRequestSummary(summaryTimeRange, isAdmin),
    enabled: Boolean(user),
    staleTime: 60 * 1000,
    retry: false,
  })
  const routingQuery = useQuery({
    queryKey: [
      'dashboard',
      'overview',
      'routing-policy',
      user?.id ?? null,
      sessionId,
    ],
    queryFn: getChannelRouting,
    enabled: canReadOperationalChannels,
    staleTime: 60 * 1000,
    retry: false,
  })

  const summaryValues = useMemo(() => {
    const requestSummaryUnavailable =
      requestSummaryQuery.isError || !requestSummaryQuery.data
    const routingAvailable =
      canReadOperationalChannels &&
      !routingQuery.isError &&
      routingQuery.data?.success === true
    const routingChannels = routingQuery.data?.data?.channels ?? []
    let routingModeDisplay = t('Unavailable')
    if (routingAvailable) {
      routingModeDisplay = routingQuery.data?.data?.policy.enabled
        ? t('Enabled')
        : t('Disabled')
    }

    let recordedSuccessRateDisplay = t('Unavailable')
    if (!requestSummaryUnavailable) {
      recordedSuccessRateDisplay =
        requestSummaryQuery.data?.success_rate == null
          ? t('No data')
          : `${formatNumber(requestSummaryQuery.data.success_rate)}%`
    }

    return {
      recordedRequestsDisplay: requestSummaryUnavailable
        ? t('Unavailable')
        : formatNumber(requestSummaryQuery.data?.total_requests ?? 0),
      recordedSuccessRateDisplay,
      enabledChannelsDisplay: !routingAvailable
        ? t('Unavailable')
        : formatNumber(
            routingChannels.filter((channel) => channel.status === 1).length
          ),
      routingModeDisplay,
      isAdmin,
    }
  }, [
    canReadOperationalChannels,
    isAdmin,
    requestSummaryQuery.data,
    requestSummaryQuery.isError,
    routingQuery.data?.success,
    routingQuery.data?.data?.channels,
    routingQuery.data?.data?.policy.enabled,
    routingQuery.isError,
    t,
  ])

  const currencyEnabledFromStore = isCurrencyDisplayEnabled()
  const statusCurrencyFlag =
    typeof status?.display_in_currency === 'boolean'
      ? Boolean(status.display_in_currency)
      : undefined
  const currencyEnabled =
    statusCurrencyFlag !== undefined
      ? statusCurrencyFlag
      : currencyEnabledFromStore
  const currencyLabel = currencyEnabled ? getCurrencyLabel() : 'Tokens'
  const usageTrendUnavailable = usageTrendQuery.isError

  const sparklineData = useMemo(
    () =>
      buildSummarySparklines(
        usageTrendQuery.data?.data ?? [],
        remainQuota,
        summaryTimeRange.start_timestamp,
        summaryTimeRange.end_timestamp
      ),
    [
      remainQuota,
      summaryTimeRange.end_timestamp,
      summaryTimeRange.start_timestamp,
      usageTrendQuery.data?.data,
    ]
  )

  const recentUsage = useMemo(
    () =>
      (usageTrendQuery.data?.data ?? []).reduce(
        (total, item) => total + (Number(item.quota) || 0),
        0
      ),
    [usageTrendQuery.data?.data]
  )

  const healthLevel = getHealthLevel(remainQuota, recentUsage)
  const healthCfg = HEALTH_CONFIG[healthLevel]
  const runwayDays = getRunwayDays(remainQuota, recentUsage)

  const todayUsageDisplay = usageTrendUnavailable
    ? t('Unavailable')
    : formatQuota(recentUsage)
  let runwayDisplay: string
  if (usageTrendUnavailable) {
    runwayDisplay = t('Unavailable')
  } else if (runwayDays !== null) {
    if (runwayDays < 1) {
      runwayDisplay = t('Less than 1 day left')
    } else if (runwayDays > 999) {
      runwayDisplay = `999+ ${t('days')}`
    } else {
      runwayDisplay = `~${formatNumber(Math.floor(runwayDays))} ${t('days')}`
    }
  } else if (remainQuota <= 0) {
    runwayDisplay = t('Balance depleted')
  } else {
    runwayDisplay = t('No recent usage')
  }

  const items = useSummaryCardsConfig({
    ...summaryValues,
    todayUsageDisplay,
    currencyEnabled,
    currencyLabel,
  }).map((config, index) => {
    const tones = ['accent-1', 'accent-2', 'accent-3'] as const
    let sparkline: number[] | undefined
    if (config.key === 'todayUsage') sparkline = sparklineData.usage
    else if (config.key === 'requests') sparkline = sparklineData.requests

    let itemLoading = false
    let updatedAt = 0
    if (config.key === 'requests' || config.key === 'successRate') {
      itemLoading = requestSummaryQuery.isLoading
      if (requestSummaryQuery.isSuccess) {
        updatedAt = requestSummaryQuery.dataUpdatedAt
      }
    } else if (
      canReadOperationalChannels &&
      routingQuery.isSuccess &&
      routingQuery.data?.success
    ) {
      updatedAt = routingQuery.dataUpdatedAt
    }

    if (config.key === 'channels' || config.key === 'routing') {
      itemLoading = canReadOperationalChannels && routingQuery.isLoading
    }

    return {
      itemLoading,
      updatedAt,
      key: config.key,
      title: config.title,
      value: config.value,
      desc: config.description,
      icon: config.icon,
      tone: tones[index] ?? 'accent-3',
      sparkline,
      sparklineVariant: 'line' as const,
    }
  })

  if (isAdmin) {
    return (
      <section
        aria-label={t('Usage at a glance')}
        className='bg-card overflow-hidden rounded-xl border'
      >
        <header className='flex flex-wrap items-center justify-between gap-x-4 gap-y-1 border-b px-4 py-2'>
          <h2 className='text-sm font-semibold'>{t('Overview')}</h2>
          <p className='text-muted-foreground text-xs tabular-nums'>
            {t('Time range')}:{' '}
            {new Date(summaryTimeRange.start_timestamp * 1000).toLocaleString()}{' '}
            – {new Date(summaryTimeRange.end_timestamp * 1000).toLocaleString()}
          </p>
        </header>
        <dl className='grid grid-cols-2 lg:grid-cols-4'>
          {items.map((item) => (
            <div
              key={item.key}
              className='min-w-0 space-y-2 border-b p-4 odd:border-r lg:border-r lg:border-b-0 lg:last:border-r-0'
            >
              <dt className='text-muted-foreground text-xs'>{item.title}</dt>
              <dd className='text-xl font-semibold tabular-nums sm:text-2xl'>
                {item.itemLoading ? t('Loading') : item.value}
              </dd>
              <p className='text-muted-foreground text-xs'>{item.desc}</p>
              <p className='text-muted-foreground text-xs tabular-nums'>
                {t('Last updated:')}{' '}
                {item.updatedAt
                  ? new Date(item.updatedAt).toLocaleTimeString()
                  : t('Unavailable')}
              </p>
            </div>
          ))}
        </dl>
        {requestSummaryQuery.data?.coverage?.complete === false && (
          <p className='text-muted-foreground border-t px-4 py-2 text-xs'>
            {t('Recorded request outcomes are incomplete for this period.')}
          </p>
        )}
      </section>
    )
  }

  return (
    <div className='bg-card overflow-hidden rounded-2xl border shadow-xs'>
      <div className='grid xl:grid-cols-[minmax(0,1fr)_19rem]'>
        <div className='flex flex-col gap-2.5 p-3 sm:gap-3 sm:p-5'>
          <div className='flex flex-wrap items-start justify-between gap-3'>
            <div className='flex flex-col gap-1'>
              <h3 className='text-sm font-semibold sm:text-base'>
                {t('Usage at a glance')}
              </h3>
              <p className='text-muted-foreground text-xs sm:text-sm'>
                {t('Monitor balance, usage, and request volume')}
              </p>
              {usageTrendUnavailable ? (
                <p className='text-destructive text-xs' role='alert'>
                  {t('Usage data is temporarily unavailable.')}
                </p>
              ) : null}
              {requestSummaryQuery.data?.coverage?.complete === false ? (
                <p className='text-muted-foreground text-xs'>
                  {t(
                    'Recorded request outcomes are incomplete for this period.'
                  )}
                </p>
              ) : null}
            </div>
          </div>
          <StaggerContainer className='grid grid-cols-2 gap-1.5 sm:gap-3 lg:grid-cols-4'>
            {items.map((it) => (
              <StaggerItem
                key={it.key}
                className='bg-background/60 rounded-lg border px-2 py-1.5 sm:rounded-xl sm:p-3'
              >
                <StatCard
                  title={it.title}
                  value={it.value}
                  description={it.desc}
                  icon={it.icon}
                  tone={it.tone}
                  sparkline={it.sparkline}
                  sparklineVariant={it.sparklineVariant}
                  loading={loading}
                  compactMobile
                />
              </StaggerItem>
            ))}
          </StaggerContainer>
        </div>

        <div className='flex flex-col justify-between gap-3 border-t bg-[linear-gradient(135deg,color-mix(in_oklch,var(--overview-accent-2)_12%,var(--background))_0%,color-mix(in_oklch,oklch(0.82_0.04_155)_8%,var(--background))_48%,color-mix(in_oklch,var(--overview-accent-1)_7%,var(--background))_100%)] p-3 sm:gap-4 sm:p-5 xl:border-t-0 xl:border-l'>
          <div className='flex flex-col gap-2 sm:gap-3'>
            <div className='flex items-center justify-between'>
              <span className='text-muted-foreground text-xs font-medium'>
                {t('Credit remaining')}
              </span>
              <span className='flex items-center gap-1.5'>
                <span
                  className={cn(
                    'size-1.5 rounded-full',
                    usageTrendUnavailable
                      ? 'bg-muted-foreground'
                      : healthCfg.dotClass
                  )}
                  aria-hidden='true'
                />
                <span className='text-muted-foreground text-[11px] font-medium'>
                  {usageTrendUnavailable
                    ? t('Usage unavailable')
                    : t(healthCfg.labelKey)}
                </span>
              </span>
            </div>

            <div className='font-mono text-xl font-semibold tracking-tight sm:text-2xl'>
              {formatQuota(remainQuota)}
            </div>

            <div className='grid grid-cols-2 gap-2'>
              <div className='bg-background/60 rounded-lg px-2.5 py-2'>
                <div className='text-muted-foreground flex items-center gap-1 text-[11px] leading-none font-medium'>
                  <Flame className='size-3 shrink-0' aria-hidden='true' />
                  <span className='truncate'>{t('Last 24h usage')}</span>
                </div>
                <div className='text-foreground mt-1.5 truncate text-xs font-semibold tabular-nums'>
                  {usageTrendUnavailable
                    ? t('Unavailable')
                    : formatQuota(recentUsage)}
                </div>
              </div>
              <div className='bg-background/60 rounded-lg px-2.5 py-2'>
                <div className='text-muted-foreground flex items-center gap-1 text-[11px] leading-none font-medium'>
                  {runwayDays !== null && runwayDays < 3 ? (
                    <TrendingDown
                      className='size-3 shrink-0'
                      aria-hidden='true'
                    />
                  ) : (
                    <ShieldCheck
                      className='size-3 shrink-0'
                      aria-hidden='true'
                    />
                  )}
                  <span className='truncate'>{t('Runway')}</span>
                </div>
                <div
                  className={cn(
                    'mt-1.5 truncate text-xs font-semibold tabular-nums',
                    healthLevel === 'critical' && 'text-destructive',
                    healthLevel === 'caution' && 'text-warning'
                  )}
                >
                  {runwayDisplay}
                </div>
              </div>
            </div>
          </div>

          <Button
            className='justify-between'
            render={
              SELF_USE_MINIMAL ? <Link to='/keys' /> : <Link to='/wallet' />
            }
          >
            <span>{SELF_USE_MINIMAL ? t('API Keys') : t('Wallet')}</span>
            <ArrowRight data-icon='inline-end' />
          </Button>
        </div>
      </div>
    </div>
  )
}
