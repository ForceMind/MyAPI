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
import { getRouteApi } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { StatTile } from '@/components/stat-tile'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useMediaQuery } from '@/hooks'
import { toIntlLocale } from '@/i18n/languages'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'
import { formatLogQuota } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'

import { getLogStats, getUserLogStats } from '../api'
import { tokenStatisticsSchema } from '../lib/token-statistics'
import { buildApiParams } from '../lib/utils'
import {
  useLogMoneyVisibility,
  useLogsViewScope,
  useUsageLogsContext,
} from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')

function RateFigure(props: { label: string; value: string | number }) {
  return (
    <span className='bg-muted/40 inline-flex h-7 items-center gap-2 rounded-md border px-2.5 text-xs'>
      <span className='text-muted-foreground'>{props.label}</span>
      <span className='font-mono font-semibold tabular-nums'>
        {props.value}
      </span>
    </span>
  )
}

/**
 * Token usage of the selected range. A PC shows the four figures as tiles
 * with the evidence below; a phone keeps them on one wrapping line and moves
 * the evidence and the per-minute rates under "Statistics basis", so the
 * first screen still reaches the records. Missing usage is never zero.
 */
export function CommonLogsStats() {
  const { t, i18n } = useTranslation()
  const showCosts = useLogMoneyVisibility()
  const { isAdminView: isAdmin } = useLogsViewScope()
  const searchParams = route.useSearch()
  const { sensitiveVisible } = useUsageLogsContext()
  const isMobile = useMediaQuery(CONSOLE_MOBILE_QUERY)
  const userId = useAuthStore((state) => state.auth.user?.id ?? null)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const role = useAuthStore((state) => state.auth.user?.role ?? null)

  const {
    data: stats,
    isLoading,
    isError,
    refetch,
    isFetching,
  } = useQuery({
    queryKey: [
      'usage-logs-stats',
      isAdmin,
      userId,
      sessionId,
      role,
      searchParams,
    ],
    enabled: userId !== null,
    retry: false,
    queryFn: async ({ signal }) => {
      const params = buildApiParams({
        page: 1,
        pageSize: 1,
        searchParams,
        columnFilters: [],
        isAdmin,
      })

      const result = isAdmin
        ? await getLogStats(params, signal)
        : await getUserLogStats(params, signal)
      if (!result.success || !result.data) {
        throw new Error(t('Failed to load usage statistics'))
      }
      if (result.data.token_usage !== undefined) {
        const parsed = tokenStatisticsSchema.safeParse(result.data.token_usage)
        if (!parsed.success) {
          throw new Error(t('Failed to load usage statistics'))
        }
        result.data.token_usage = parsed.data
      }
      return result.data
    },
  })

  if (userId === null) return null
  if (isError) {
    return (
      <div role='alert' className='flex flex-wrap items-center gap-2 text-sm'>
        <span>{t('Failed to load usage statistics')}</span>
        <Button
          variant='outline'
          size='sm'
          disabled={isFetching}
          onClick={() => void refetch()}
        >
          {t('Retry')}
        </Button>
      </div>
    )
  }
  if (isLoading) {
    return isMobile ? (
      <Skeleton className='h-5 w-full max-w-80' />
    ) : (
      <div className='grid grid-cols-2 gap-2 sm:gap-3 lg:grid-cols-4'>
        {['input', 'output', 'total', 'cache'].map((key) => (
          <Skeleton key={key} className='h-20 rounded-lg' />
        ))}
      </div>
    )
  }

  const usage = stats?.token_usage
  const locale = toIntlLocale(i18n.resolvedLanguage ?? i18n.language)
  const number = new Intl.NumberFormat(locale)
  const hasKnown =
    usage !== undefined && (usage.known_records > 0 || usage.records === 0)
  const formatTokens = (value: string) =>
    sensitiveVisible ? number.format(BigInt(value)) : '••••'
  let cacheValue = t('Unknown')
  if (usage?.cache_hit_rate != null) {
    cacheValue = sensitiveVisible
      ? new Intl.NumberFormat(locale, {
          style: 'percent',
          maximumFractionDigits: 2,
        }).format(usage.cache_hit_rate)
      : '••••'
  }
  const partial =
    usage !== undefined && usage.unknown_records > 0 && usage.known_records > 0
  const values = usage
    ? [
        {
          label: partial ? t('Known input Tokens') : t('Input Tokens'),
          value: hasKnown
            ? formatTokens(usage.known_input_tokens)
            : t('Unknown'),
        },
        {
          label: partial ? t('Known output Tokens') : t('Output Tokens'),
          value: hasKnown
            ? formatTokens(usage.known_output_tokens)
            : t('Unknown'),
        },
        {
          label: partial ? t('Known Token subtotal') : t('Total Tokens'),
          value: hasKnown
            ? formatTokens(usage.known_total_tokens)
            : t('Unknown'),
        },
        { label: t('Cache hit rate'), value: cacheValue },
      ]
    : []

  const rates = (
    <div
      className='flex flex-wrap items-center gap-2'
      title={t(
        'RPM and TPM cover consumption logs from the last 60 seconds, not all requests or complete token usage.'
      )}
    >
      {showCosts && (
        <RateFigure
          label={t('Usage')}
          value={sensitiveVisible ? formatLogQuota(stats?.quota || 0) : '••••'}
        />
      )}
      <span className='text-muted-foreground text-[11px]'>
        {t('Recorded usage · last 60s')}
      </span>
      <RateFigure label={t('RPM')} value={stats?.rpm || 0} />
      <RateFigure label={t('TPM')} value={stats?.tpm || 0} />
    </div>
  )
  const evidence = usage ? (
    <>
      {usage.records === 0 ? (
        <p>{t('No call records match this range.')}</p>
      ) : (
        <p>
          {t('Usage evidence: {{known}} / {{total}} records', {
            known: number.format(usage.known_records),
            total: number.format(usage.records),
          })}
        </p>
      )}
      {usage.unknown_records > 0 && (
        <p>
          {usage.known_records > 0 && (
            <strong className='font-medium'>
              {t('Known subtotal')}
              {' · '}
            </strong>
          )}
          {usage.unknown_records === 1
            ? t('1 record with unknown usage')
            : t('{{count}} records with unknown usage', {
                count: usage.unknown_records,
              })}
        </p>
      )}
      <p>
        {t('Cache evidence: {{known}} / {{total}} records', {
          known: number.format(usage.cache_known_records),
          total: number.format(usage.records),
        })}
      </p>
      {usage.cache_known_records > 0 && (
        <p>
          {t('Cached input: {{cached}} / {{input}} Tokens', {
            cached: formatTokens(usage.cache_read_tokens),
            input: formatTokens(usage.cache_input_tokens),
          })}
        </p>
      )}
    </>
  ) : null
  const method = (
    <p>
      {t(
        'Cache hit rate uses summed cached input divided by summed input for records with cache evidence. Missing usage is excluded, not zero.'
      )}
    </p>
  )

  if (isMobile) {
    return (
      <section
        aria-label={t('Token usage statistics')}
        className='bg-card min-w-0 rounded-xl border px-3 pt-2'
        data-testid={usage ? 'log-token-statistics' : undefined}
      >
        {usage ? (
          <dl className='flex min-w-0 flex-wrap gap-x-4 gap-y-1 text-xs'>
            {values.map((item) => (
              <div
                key={item.label}
                className='flex min-w-0 items-baseline gap-1.5'
              >
                <dt className='text-muted-foreground'>{item.label}</dt>
                <dd className='font-mono font-semibold break-all tabular-nums'>
                  {item.value}
                </dd>
              </div>
            ))}
          </dl>
        ) : (
          <p className='text-muted-foreground text-sm'>
            {t('Token statistics unavailable')}
          </p>
        )}
        <details className='text-muted-foreground text-xs'>
          <summary className='min-h-11 w-fit cursor-pointer py-3'>
            {t('Statistics basis')}
          </summary>
          <div className='flex flex-col gap-1 pb-3'>
            {evidence}
            {method}
            {rates}
          </div>
        </details>
      </section>
    )
  }

  return (
    <div className='flex min-w-0 flex-col gap-2'>
      {usage ? (
        <section
          aria-label={t('Token usage statistics')}
          className='flex min-w-0 flex-col gap-2'
          data-testid='log-token-statistics'
        >
          <p className='text-sm font-medium'>
            {t('Token usage · selected range')}
          </p>
          <dl className='grid min-w-0 grid-cols-2 gap-2 sm:gap-3 lg:grid-cols-4'>
            {values.map((item) => (
              <StatTile key={item.label} label={item.label} value={item.value} />
            ))}
          </dl>
          <div className='text-muted-foreground flex flex-wrap items-center gap-x-4 gap-y-1 text-xs'>
            {evidence}
            <details className='basis-full'>
              <summary className='min-h-11 w-fit cursor-pointer py-3'>
                {t('Statistics basis')}
              </summary>
              <div className='mt-1'>{method}</div>
            </details>
          </div>
        </section>
      ) : (
        <p className='text-muted-foreground text-sm'>
          {t('Token statistics unavailable')}
        </p>
      )}
      {rates}
    </div>
  )
}
