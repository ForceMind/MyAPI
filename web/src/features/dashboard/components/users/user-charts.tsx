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
import { CanceledError } from 'axios'
import { VChart } from '@visactor/react-vchart'
import { Loader2 } from 'lucide-react'
import {
  useEffect,
  useMemo,
  useState,
  useRef,
  useCallback,
  type ReactNode,
} from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useThemeCustomization } from '@/context/theme-customization-provider'
import { useTheme } from '@/context/theme-provider'
import { getUserQuotaDataByUsers } from '@/features/dashboard/api'
import {
  TIME_GRANULARITY_OPTIONS,
  TIME_RANGE_PRESETS,
} from '@/features/dashboard/constants'
import {
  buildQueryParams,
  getDefaultDays,
  isQuotaRangeSupported,
  saveGranularity,
  processUserChartData,
} from '@/features/dashboard/lib'
import { placeLegendForPhone } from '@/features/dashboard/lib/charts'
import type {
  ProcessedUserChartData,
  UserChartsFilters,
} from '@/features/dashboard/types'
import { useMediaQuery } from '@/hooks'
import { useChartPalette } from '@/lib/chart-palette'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'
import { getRollingDateRange, type TimeGranularity } from '@/lib/time'
import { VCHART_OPTION } from '@/lib/vchart'
import { useAuthStore } from '@/stores/auth-store'

let themeManagerPromise: Promise<
  (typeof import('@visactor/vchart'))['ThemeManager']
> | null = null

const USER_CHARTS: {
  value: string
  labelKey: string
  specKey: keyof ProcessedUserChartData
}[] = [
  {
    value: 'rank',
    labelKey: 'User Consumption Ranking',
    specKey: 'spec_user_rank',
  },
  {
    value: 'trend',
    labelKey: 'User Consumption Trend',
    specKey: 'spec_user_trend',
  },
]

const TOP_USER_LIMIT_OPTIONS = [5, 10, 20, 50]

interface UserChartsProps {
  filters: UserChartsFilters
  onFiltersChange: (filters: UserChartsFilters) => void
}

export function UserCharts(props: UserChartsProps) {
  const { t } = useTranslation()
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)
  const { resolvedTheme } = useTheme()
  const { customization } = useThemeCustomization()
  const palette = useChartPalette(`${resolvedTheme}:${customization.preset}`)
  const isMobile = useMediaQuery(CONSOLE_MOBILE_QUERY)
  const [themeReady, setThemeReady] = useState(false)
  const themeManagerRef = useRef<
    (typeof import('@visactor/vchart'))['ThemeManager'] | null
  >(null)

  // The selection is owned by the dashboard parent so it persists across
  // sub-section switches; the rolling window is derived from the chosen range.
  const timeGranularity = props.filters.timeGranularity
  const selectedRange = props.filters.selectedRange
  const topUserLimit = props.filters.topUserLimit
  const onFiltersChange = props.onFiltersChange

  const timeRange = useMemo(() => {
    const { start, end } = getRollingDateRange(selectedRange)
    return {
      start_timestamp: Math.floor(start.getTime() / 1000),
      end_timestamp: Math.floor(end.getTime() / 1000),
    }
  }, [selectedRange])
  const queryParams = useMemo(
    () =>
      buildQueryParams(timeRange, {
        time_granularity: timeGranularity,
      }),
    [timeGranularity, timeRange]
  )

  const handleRangeChange = useCallback(
    (days: number) => {
      if (timeGranularity === 'minute' && days > 1) {
        toast.info(t('Minute granularity is limited to the last 24 hours.'))
        return
      }
      onFiltersChange({ ...props.filters, selectedRange: days })
    },
    [onFiltersChange, props.filters, t, timeGranularity]
  )

  const handleGranularityChange = useCallback(
    (g: TimeGranularity) => {
      saveGranularity(g)
      const currentRange = getRollingDateRange(selectedRange)
      const keepRange = isQuotaRangeSupported(
        currentRange.start,
        currentRange.end,
        g
      )
      if (!keepRange) {
        toast.info(
          g === 'minute'
            ? t('Minute granularity is limited to the last 24 hours.')
            : t('The selected range is too large for this granularity.')
        )
      }
      onFiltersChange({
        ...props.filters,
        timeGranularity: g,
        selectedRange: keepRange ? selectedRange : getDefaultDays(g),
      })
    },
    [onFiltersChange, props.filters, selectedRange, t]
  )

  const handleTopUserLimitChange = useCallback(
    (limit: number) => {
      onFiltersChange({ ...props.filters, topUserLimit: limit })
    },
    [onFiltersChange, props.filters]
  )

  useEffect(() => {
    const updateTheme = async () => {
      setThemeReady(false)
      if (!themeManagerPromise) {
        themeManagerPromise = import('@visactor/vchart').then(
          (m) => m.ThemeManager
        )
      }
      const ThemeManager = await themeManagerPromise
      themeManagerRef.current = ThemeManager
      ThemeManager.setCurrentTheme(resolvedTheme === 'dark' ? 'dark' : 'light')
      setThemeReady(true)
    }
    updateTheme()
  }, [resolvedTheme])

  const {
    data: userData,
    isError,
    error,
    isLoading,
    isFetching,
    refetch,
  } = useQuery({
    queryKey: ['dashboard', 'user-quota', queryParams, identity],
    queryFn: async ({ signal }) => {
      const isCurrent = () => {
        const { auth } = useAuthStore.getState()
        return !signal.aborted && identity === `${auth.user?.id ?? null}:${auth.user?.role ?? 0}:${auth.session?.sid ?? null}`
      }
      if (!isCurrent()) throw new CanceledError('Analytics request scope changed')
      const result = await getUserQuotaDataByUsers(queryParams)
      if (!isCurrent()) throw new CanceledError('Analytics request scope changed')
      return result
    },
    select: (res) => {
      if (!Array.isArray(res.data)) throw new Error(t('Failed to load'))
      return res.data
    },
    staleTime: 60_000,
  })

  const chartData = useMemo(
    () =>
      processUserChartData(
        isLoading ? [] : (userData ?? []),
        timeGranularity,
        t,
        topUserLimit,
        queryParams.timezone_offset,
        palette
      ),
    [
      userData,
      isLoading,
      timeGranularity,
      t,
      topUserLimit,
      queryParams.timezone_offset,
      palette,
    ]
  )

  return (
    <div className='space-y-3'>
      <div className='flex flex-wrap items-center gap-1.5 pb-1 sm:gap-2 [&_[role=tab]]:min-h-11 [&_[role=tab]]:min-w-11 [&_[role=tablist]]:h-auto [&_[role=tablist]]:flex-wrap [&_[role=tablist]]:max-w-full'>
        <Tabs
          value={String(selectedRange)}
          onValueChange={(value) => handleRangeChange(Number(value))}
          className='min-w-0 max-w-full'
        >
          <TabsList
            className='h-auto min-h-12 max-w-full flex-wrap'
            aria-label={t('Time range')}
          >
            {TIME_RANGE_PRESETS.map((preset) => (
              <TabsTrigger
                key={preset.days}
                value={String(preset.days)}
                disabled={timeGranularity === 'minute' && preset.days > 1}
                className='px-2.5 text-xs'
              >
                {t(preset.label)}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>

        <Tabs
          value={timeGranularity}
          onValueChange={(value) =>
            handleGranularityChange(value as TimeGranularity)
          }
          className='min-w-0 max-w-full'
        >
          <TabsList
            className='h-auto min-h-12 max-w-full flex-wrap'
            aria-label={t('Time Granularity')}
          >
            {TIME_GRANULARITY_OPTIONS.map((opt) => (
              <TabsTrigger
                key={opt.value}
                value={opt.value}
                className='px-2.5 text-xs'
              >
                {t(opt.label)}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>

        <Tabs
          value={String(topUserLimit)}
          onValueChange={(value) => handleTopUserLimitChange(Number(value))}
          className='min-w-0 max-w-full'
        >
          <TabsList
            className='h-auto min-h-12 max-w-full flex-wrap'
            aria-label={t('Top Users')}
          >
            <span className='text-muted-foreground px-2 text-xs font-medium whitespace-nowrap'>
              {t('Top Users')}
            </span>
            {TOP_USER_LIMIT_OPTIONS.map((limit) => (
              <TabsTrigger
                key={limit}
                value={String(limit)}
                className='px-2.5 text-xs'
              >
                {t('Top {{count}}', { count: limit })}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>

        {isLoading && (
          <Loader2 className='text-muted-foreground size-4 animate-spin' />
        )}
      </div>

      <div className='grid gap-3'>
        {USER_CHARTS.map((chart) => {
          const spec = chartData[chart.specKey]
          let chartContent: ReactNode = <Skeleton className='h-full w-full' />
          if (!isLoading) {
            if (isError) {
              chartContent = (
                <div
                  role='alert'
                  className='text-muted-foreground flex h-full flex-col items-center justify-center gap-3 px-4 text-center text-sm [overflow-wrap:anywhere]'
                >
                  {error instanceof Error && error.message
                    ? error.message
                    : t('Failed to load')}
                  <Button
                    type='button'
                    variant='outline'
                    className='min-h-11 min-w-11'
                    disabled={isFetching}
                    onClick={() => void refetch()}
                  >
                    {t('Retry')}
                  </Button>
                </div>
              )
            } else if (themeReady && spec) {
              chartContent = (
                <VChart
                  key={`user-${chart.value}-${topUserLimit}-${resolvedTheme}-${palette ? 'tokens' : 'default'}-${isMobile ? 'phone' : 'wide'}`}
                  spec={{
                    ...placeLegendForPhone(spec, isMobile),
                    theme: resolvedTheme === 'dark' ? 'dark' : 'light',
                    background: 'transparent',
                  }}
                  option={VCHART_OPTION}
                />
              )
            } else {
              chartContent = null
            }
          }

          return (
            <section
              key={chart.value}
              className='bg-card overflow-hidden rounded-xl border'
            >
              <div className='flex w-full items-center gap-2 border-b px-4 py-3'>
                <h2 className='text-sm font-semibold'>{t(chart.labelKey)}</h2>
              </div>

              <div className='h-[340px] p-1.5 sm:h-96 sm:p-2'>
                {chartContent}
              </div>
            </section>
          )
        })}
      </div>
    </div>
  )
}
