/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { getAllLogs } from '@/features/usage-logs/api'
import type { LogOtherData } from '@/features/usage-logs/types'
import { hasPermission } from '@/lib/admin-permissions'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { useRollingDayWindow } from '../../hooks/use-rolling-day-window'

export function RoutingSwitchSummary() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const allowed =
    Boolean(user && user.role >= ROLE.ADMIN) &&
    hasPermission(user, 'channel', 'read')
  const { end_timestamp: end } = useRollingDayWindow()
  const query = useQuery({
    queryKey: [
      'dashboard',
      'overview',
      'recent-routing-switches',
      user?.id ?? null,
      sessionId,
      end,
    ],
    queryFn: () =>
      getAllLogs({
        p: 1,
        page_size: 20,
        start_timestamp: end - 86400,
        end_timestamp: end,
      }),
    enabled: allowed,
    staleTime: 60000,
    retry: false,
    meta: { errorHandledLocally: true },
  })
  if (!allowed) return null
  const failed = query.isError || query.data?.success === false
  const switches = (query.data?.data?.items ?? [])
    .flatMap((log) => {
      if (!('other' in log) || !log.other) return []
      try {
        const other: LogOtherData = JSON.parse(log.other)
        const routing = other.admin_info?.channel_routing
        if (
          !routing ||
          typeof routing.switch_count !== 'number' ||
          !Number.isSafeInteger(routing.switch_count) ||
          routing.switch_count <= 0
        ) {
          return []
        }
        return [
          {
            id: log.id,
            channel:
              typeof routing.channel_id === 'number'
                ? routing.channel_id
                : undefined,
            count: routing.switch_count,
            reason:
              typeof routing.switch_reason === 'string'
                ? routing.switch_reason
                : undefined,
          },
        ]
      } catch {
        return []
      }
    })
    .slice(0, 3)
  return (
    <section
      aria-label={t('Recent channel switches')}
      className='bg-card min-w-0 space-y-3 rounded-xl border p-4'
    >
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <h3 className='text-sm font-semibold'>
          {t('Recent channel switches')}
        </h3>
        <Link
          to='/usage-logs/$section'
          params={{ section: 'common' }}
          className='text-primary text-xs'
        >
          {t('Usage Logs')}
        </Link>
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Switches found in the latest 20 log entries from the last 24 hours.'
        )}
      </p>
      {query.isLoading && (
        <p role='status' className='text-sm'>
          {t('Loading')}
        </p>
      )}
      {failed && (
        <p role='alert' className='text-destructive text-sm'>
          {t('Please try again.')}
        </p>
      )}
      {!query.isLoading && !failed && !switches.length && (
        <p className='text-muted-foreground text-sm'>
          {t('No channel switches in the loaded logs.')}
        </p>
      )}
      <ul className='space-y-2 text-xs'>
        {switches.map((item) => (
          <li
            key={item.id}
            className='flex flex-wrap justify-between gap-2 border-t pt-2'
          >
            <span>
              {t('Channel ID')}: {item.channel ?? '—'} · {t('Channel switches')}
              : {item.count}
            </span>
            <span className='text-muted-foreground font-mono break-all'>
              {item.reason || '—'}
            </span>
          </li>
        ))}
      </ul>
    </section>
  )
}
