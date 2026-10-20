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

import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { toIntlLocale } from '@/i18n/languages'
import { formatLogQuota } from '@/lib/format'
import { cn } from '@/lib/utils'
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

function StatBadge(props: {
  label: string
  value: string | number
  accent: string
}) {
  return (
    <span className='border-border/60 bg-muted/25 inline-flex h-7 items-center gap-2 rounded-md border px-2.5 text-xs shadow-xs'>
      <span className={cn('h-3.5 w-0.5 rounded-full', props.accent)} />
      <span className='text-muted-foreground'>{props.label}</span>
      <span className='text-foreground/85 font-mono font-semibold tabular-nums'>
        {props.value}
      </span>
    </span>
  )
}

export function CommonLogsStats() {
  const { t, i18n } = useTranslation()
  const showCosts = useLogMoneyVisibility()
  const { isAdminView: isAdmin } = useLogsViewScope()
  const searchParams = route.useSearch()
  const { sensitiveVisible } = useUsageLogsContext()
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
    return (
      <div className='flex items-center gap-2'>
        <Skeleton className='h-7 w-[150px] rounded-md' />
        <Skeleton className='h-7 w-[100px] rounded-md' />
        <Skeleton className='h-7 w-[120px] rounded-md' />
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

  return (
    <div className='min-w-0 space-y-3'>
      {usage ? (
        <section
          aria-label={t('Token usage statistics')}
          className='min-w-0 space-y-2'
          data-testid='log-token-statistics'
        >
          <p className='text-sm font-medium'>
            {t('Token usage · selected range')}
          </p>
          <dl className='grid min-w-0 grid-cols-2 gap-2 lg:grid-cols-4'>
            {values.map((item) => (
              <div
                key={item.label}
                className='bg-muted/20 min-w-0 rounded-md border p-2.5'
              >
                <dt className='text-muted-foreground text-xs'>{item.label}</dt>
                <dd className='mt-1 font-mono text-base font-semibold break-all tabular-nums'>
                  {item.value}
                </dd>
              </div>
            ))}
          </dl>
          <div className='text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs'>
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
            <details className='basis-full'>
              <summary className='min-h-11 w-fit cursor-pointer py-3'>
                {t('Statistics basis')}
              </summary>
              <p className='mt-1'>
                {t(
                  'Cache hit rate uses summed cached input divided by summed input for records with cache evidence. Missing usage is excluded, not zero.'
                )}
              </p>
            </details>
          </div>
        </section>
      ) : (
        <p className='text-muted-foreground text-sm'>
          {t('Token statistics unavailable')}
        </p>
      )}
      <div
        className='flex flex-wrap items-center gap-2'
        title={t(
          'RPM and TPM cover consumption logs from the last 60 seconds, not all requests or complete token usage.'
        )}
      >
        {showCosts && (
          <StatBadge
            label={t('Usage')}
            value={
              sensitiveVisible ? formatLogQuota(stats?.quota || 0) : '••••'
            }
            accent='bg-sky-500/70'
          />
        )}
        <span className='text-muted-foreground text-[11px]'>
          {t('Recorded usage · last 60s')}
        </span>
        <StatBadge
          label={t('RPM')}
          value={stats?.rpm || 0}
          accent='bg-rose-500/65'
        />
        <StatBadge
          label={t('TPM')}
          value={stats?.tpm || 0}
          accent='bg-slate-400/70'
        />
      </div>
    </div>
  )
}
