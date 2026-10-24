/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import {
  formatQuotaDuration,
  quotaSeriesKey,
  quotaWindowLabel,
} from '@/features/channels/lib/quota-history'
import type { ChannelQuotaChangeItem } from '@/features/channels/types'
import { AccountsState } from '@/features/personal-upstream/components/accounts-state'
import {
  describeNativeWindowState,
  nativeWindowState,
  nativeWindowUsage,
} from '@/features/personal-upstream/window-state'
import { useMediaQuery } from '@/hooks/use-media-query'
import { useIsSidebarModuleVisible } from '@/hooks/use-sidebar-config'
import { toIntlLocale } from '@/i18n/languages'
import { CONSOLE_MOBILE_QUERY } from '@/lib/console-layout'
import { linkButtonClassName } from '@/lib/link-button'

import type { OverviewData } from './use-overview-data'

const DESKTOP_WINDOWS = 3
const MOBILE_WINDOWS = 1

/**
 * The most used account windows first (design 8.1). Windows without a
 * trustworthy sample sort last and show their state instead of a number.
 */
function tightestWindows(items: ChannelQuotaChangeItem[], count: number) {
  return items
    .filter(
      (item) =>
        item.metric_type === 'codex_rate_limit' && item.unit === 'percent'
    )
    .map((item) => ({ item, used: nativeWindowUsage(item) }))
    .sort((a, b) => (b.used ?? -1) - (a.used ?? -1))
    .slice(0, count)
}

export function OverviewWindows(props: { data: OverviewData }) {
  const { t, i18n } = useTranslation()
  const mobile = useMediaQuery(CONSOLE_MOBILE_QUERY)
  const showAccounts = useIsSidebarModuleVisible('/upstream-accounts')
  const upstream = props.data.upstream
  const query = upstream.accountsQuery
  const windows =
    upstream.canRead && !query.isError
      ? tightestWindows(
          query.data?.items ?? [],
          mobile ? MOBILE_WINDOWS : DESKTOP_WINDOWS
        )
      : []
  const now = Date.now() / 1000
  const number = new Intl.NumberFormat(toIntlLocale(i18n.language), {
    maximumFractionDigits: 1,
  })
  const time = new Intl.DateTimeFormat(toIntlLocale(i18n.language), {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })

  let body
  if (upstream.canRead && query.isPending) {
    body = (
      <div className='flex flex-col gap-3' aria-busy='true'>
        <span className='sr-only'>{t('Loading...')}</span>
        <Skeleton className='h-12 w-full' />
        <Skeleton className='h-12 w-full' />
      </div>
    )
  } else if (windows.length === 0) {
    body = <AccountsState data={upstream} />
  } else {
    body = (
      <ul className='flex flex-col gap-3'>
        {windows.map(({ item, used }) => {
          const state = nativeWindowState(item, now)
          const status = describeNativeWindowState(state, t)
          const windowLabel =
            item.window_seconds && item.window_seconds > 0
              ? formatQuotaDuration(item.window_seconds, t)
              : quotaWindowLabel(item.window_type, t)
          const observed = item.observed_at
          return (
            <li
              key={quotaSeriesKey(item)}
              className='flex min-w-0 flex-col gap-1'
            >
              <div className='flex min-w-0 items-baseline justify-between gap-3'>
                {/* A long account name stays on one line; the full name is
                    in the title and on the account windows page. */}
                <span
                  className='text-data min-w-0 truncate font-medium'
                  title={`${item.account_label || item.name} · ${windowLabel}`}
                >
                  {item.account_label || item.name} · {windowLabel}
                </span>
                <span className='text-data shrink-0 tabular-nums'>
                  {used === null
                    ? t('Usage unavailable')
                    : t('{{value}}% used', { value: number.format(used) })}
                </span>
              </div>
              <div className='text-caption text-muted-foreground flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1'>
                {used !== null && (
                  // Telemetry fill always comes with the number beside it.
                  <Progress
                    value={used}
                    aria-label={t('Account window used')}
                    className='[&_[data-slot=progress-indicator]]:bg-telemetry min-w-24 flex-1 [&_[data-slot=progress-track]]:h-1.5'
                  />
                )}
                {/* A trustworthy sample needs no badge; any other state
                    says why the number may not be current. */}
                {state !== 'available' && (
                  <StatusBadge
                    variant={status.variant}
                    showDot
                    copyable={false}
                    className='h-auto min-h-5 whitespace-normal'
                  >
                    {status.label}
                  </StatusBadge>
                )}
                <span className='shrink-0'>
                  {Number.isSafeInteger(observed) && (observed ?? 0) > 0
                    ? t('Observed {{time}}', {
                        time: time.format(
                          new Date((observed as number) * 1000)
                        ),
                      })
                    : t('No timestamp')}
                </span>
              </div>
            </li>
          )
        })}
      </ul>
    )
  }

  return (
    <section
      aria-labelledby='overview-windows-title'
      className='bg-card p-card flex min-w-0 flex-col gap-3 rounded-xl border'
    >
      <div className='flex min-w-0 flex-wrap items-center justify-between gap-2'>
        <h2 id='overview-windows-title' className='text-title'>
          {t('Most used account windows')}
        </h2>
        {upstream.canRead && showAccounts && (
          <Link
            to='/upstream-accounts'
            data-myapi-button=''
            className={linkButtonClassName({ variant: 'ghost' })}
          >
            {t('View all account windows')}
          </Link>
        )}
      </div>
      {body}
    </section>
  )
}
