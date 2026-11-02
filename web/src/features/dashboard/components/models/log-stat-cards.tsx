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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { StatTile } from '@/components/stat-tile'
import { getUserQuotaDates } from '@/features/dashboard/api'
import { useModelStatCardsConfig } from '@/features/dashboard/hooks/use-dashboard-config'
import {
  buildQueryParams,
  calculateDashboardStats,
  getDefaultDays,
} from '@/features/dashboard/lib'
import type {
  QuotaDataItem,
  DashboardFilters,
} from '@/features/dashboard/types'
import { toIntlLocale } from '@/i18n/languages'
import { formatCompactNumber, formatNumber, formatQuota } from '@/lib/format'
import { computeTimeRange } from '@/lib/time'
import { useAuthStore } from '@/stores/auth-store'

interface LogStatCardsProps {
  filters?: DashboardFilters
  onDataUpdate?: (
    data: QuotaDataItem[],
    loading: boolean,
    timezoneOffsetMinutes?: number,
    failed?: boolean
  ) => void
}

const MAX_INLINE_STAT_CHARS = 9

function formatStatNumber(value: number, locale: Intl.LocalesArgument) {
  const fullValue = formatNumber(value, locale)
  const displayValue =
    fullValue.length > MAX_INLINE_STAT_CHARS
      ? formatCompactNumber(value, locale)
      : fullValue

  return {
    displayValue,
    fullValue,
  }
}

export function LogStatCards(props: LogStatCardsProps) {
  const { i18n, t } = useTranslation()
  const statCardsConfig = useModelStatCardsConfig()
  const user = useAuthStore((state) => state.auth.user)
  const isAdmin = !!(user?.role && user.role >= 10)
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)
  const [stats, setStats] = useState<{
    totalQuota: number
    totalCount: number
    totalTokens: number
  } | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [retryGeneration, setRetryGeneration] = useState(0)

  const [timeRangeMinutes, setTimeRangeMinutes] = useState(0)

  const { filters, onDataUpdate } = props

  useEffect(() => {
    const abortController = new AbortController()
    const isCurrent = () => {
      const { auth } = useAuthStore.getState()
      return !abortController.signal.aborted && identity === `${auth.user?.id ?? null}:${auth.user?.role ?? 0}:${auth.session?.sid ?? null}`
    }
    if (!isCurrent()) return
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setLoading(true)

    setError(false)
    const timeRange = computeTimeRange(
      getDefaultDays(filters?.time_granularity),
      filters?.start_timestamp,
      filters?.end_timestamp
    )
    const timeDiff = (timeRange.end_timestamp - timeRange.start_timestamp) / 60
    setTimeRangeMinutes(timeDiff)
    const queryParams = buildQueryParams(timeRange, filters)
    onDataUpdate?.([], true, queryParams.timezone_offset)

    void getUserQuotaDates(queryParams, isAdmin)
      .then((res) => {
        if (!isCurrent()) return
        if (!Array.isArray(res?.data)) {
          throw new Error(t('Failed to load model analytics'))
        }
        const data = res.data
        setStats(calculateDashboardStats(data))
        onDataUpdate?.(data, false, queryParams.timezone_offset)
      })
      .catch((reason: unknown) => {
        if (!isCurrent()) return
        setStats(null)
        setError(true)
        onDataUpdate?.([], false, queryParams.timezone_offset, true)
        toast.error(
          reason instanceof Error && reason.message
            ? reason.message
            : t('Failed to load')
        )
      })
      .finally(() => {
        if (isCurrent()) {
          setLoading(false)
        }
      })

    return () => {
      abortController.abort()
    }
  }, [filters, identity, isAdmin, onDataUpdate, retryGeneration, t])

  const adaptedStats = {
    rpm: stats?.totalCount ?? 0,
    quota: stats?.totalQuota ?? 0,
    tpm: stats?.totalTokens ?? 0,
  }

  const items = statCardsConfig.map((config) => {
    const rawValue = config.getValue(adaptedStats, timeRangeMinutes)
    const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
    const formatted =
      config.key === 'quota'
        ? {
            displayValue: formatQuota(rawValue),
            fullValue: formatQuota(rawValue),
          }
        : formatStatNumber(rawValue, locale)

    return {
      title: config.title,
      value: formatted.displayValue,
      fullValue: formatted.fullValue,
      desc: config.description,
    }
  })

  // The same figures as the Key and connection pages (design v2 StatInline);
  // a failed read shows "--", never zero.
  return (
    <div className='space-y-3 [&_[data-myapi-button]]:min-h-11 [&_[data-myapi-button]]:min-w-11'>
      {error && (
        <ErrorState
          title={t('Failed to load model analytics')}
          onRetry={() => setRetryGeneration((value) => value + 1)}
        />
      )}
      <dl className='grid min-w-0 grid-cols-2 gap-2 sm:grid-cols-3 sm:gap-3 lg:grid-cols-5'>
        {items.map((it) => (
          <StatTile
            key={it.title}
            label={it.title}
            loading={loading}
            value={
              error ? (
                <span className='text-muted-foreground'>--</span>
              ) : (
                <span className='block truncate' title={it.fullValue}>
                  {it.value}
                </span>
              )
            }
            detail={<span className='max-md:hidden'>{it.desc}</span>}
          />
        ))}
      </dl>
    </div>
  )
}
