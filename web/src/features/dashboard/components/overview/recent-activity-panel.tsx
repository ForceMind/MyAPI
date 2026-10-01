/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Activity, TriangleAlert } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button, buttonVariants } from '@/components/ui/button'
import { IconBadge } from '@/components/ui/icon-badge'
import { getRecentLogOverview } from '@/features/dashboard/api'
import type { RecentLogOverviewItem } from '@/features/dashboard/types'
import { toIntlLocale } from '@/i18n/languages'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { PanelWrapper } from '../ui/panel-wrapper'

const ACTIVITY_STALE_MS = 2 * 60 * 1000

function RecentLogList(props: {
  rows: RecentLogOverviewItem[] | undefined
  emptyKey: string
  isAdmin: boolean
  dateFormatter: Intl.DateTimeFormat
}) {
  const { t } = useTranslation()
  if (!props.rows?.length) {
    return (
      <p className='text-muted-foreground py-5 text-center text-xs'>
        {t(props.emptyKey)}
      </p>
    )
  }
  return (
    <ul className='divide-border divide-y'>
      {props.rows.map((row) => {
        const observedAt = Number.isFinite(row.created_at)
          ? new Date(row.created_at * 1000)
          : null
        const validDate =
          observedAt && Number.isFinite(observedAt.getTime())
            ? observedAt
            : null
        return (
          <li
            key={row.id}
            className='flex min-w-0 items-center justify-between gap-3 py-2.5 text-xs'
          >
            <span className='min-w-0 flex-1 truncate font-medium'>
              {row.model_name?.trim() || t('Unknown')}
              {props.isAdmin && row.username ? (
                <span className='text-muted-foreground ml-1 font-normal'>
                  · {row.username}
                </span>
              ) : null}
            </span>
            {validDate ? (
              <time
                className='text-muted-foreground shrink-0 tabular-nums'
                dateTime={validDate.toISOString()}
              >
                {props.dateFormatter.format(validDate)}
              </time>
            ) : (
              <span className='text-muted-foreground shrink-0'>
                {t('Unknown')}
              </span>
            )}
          </li>
        )
      })}
    </ul>
  )
}

export function RecentActivityPanel() {
  const { i18n, t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const isAdmin = Boolean(user && user.role >= ROLE.ADMIN)
  const dateFormatter = useMemo(
    () =>
      new Intl.DateTimeFormat(toIntlLocale(i18n.language), {
        month: 'numeric',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      }),
    [i18n.language]
  )
  const query = useQuery({
    queryKey: [
      'dashboard',
      'recent-log-overview',
      user?.id ?? null,
      sessionId,
      isAdmin,
    ],
    queryFn: async () => {
      const response = await getRecentLogOverview(isAdmin)
      if (!response.success || !response.data) {
        throw new Error('recent log overview unavailable')
      }
      return response.data
    },
    enabled: user !== null,
    staleTime: ACTIVITY_STALE_MS,
    refetchInterval: ACTIVITY_STALE_MS,
    retry: false,
  })
  const oldestErrorAt = query.data?.errors.at(-1)?.created_at
  const errorStartTime =
    oldestErrorAt &&
    Number.isFinite(oldestErrorAt) &&
    oldestErrorAt > 0 &&
    oldestErrorAt <= Number.MAX_SAFE_INTEGER / 1000
      ? Math.max(1, Math.floor(oldestErrorAt * 1000) - 60_000)
      : undefined

  if (!user) return null

  return (
    <PanelWrapper
      title={
        <span className='flex items-center gap-2'>
          <IconBadge tone='info' size='sm'>
            <Activity />
          </IconBadge>
          {t('Recent activity')}
        </span>
      }
      description={t('Recent requests and errors without message contents.')}
      loading={query.isLoading}
      height='h-40'
      headerActions={
        <Link
          to='/usage-logs/$section'
          params={{ section: 'common' }}
          className={buttonVariants({ variant: 'ghost', size: 'sm' })}
        >
          {t('View logs')}
        </Link>
      }
    >
      {query.isError ? (
        <Alert variant='destructive'>
          <AlertTitle>{t('Unable to load recent activity')}</AlertTitle>
          <AlertDescription>
            <Button
              variant='outline'
              size='sm'
              onClick={() => void query.refetch()}
            >
              {t('Refresh')}
            </Button>
          </AlertDescription>
        </Alert>
      ) : (
        <div className='grid gap-4 md:grid-cols-2'>
          <section aria-label={t('Recent requests')} className='min-w-0'>
            <h3 className='flex items-center gap-2 text-sm font-semibold'>
              <Activity className='text-info size-4' aria-hidden='true' />
              {t('Recent requests')}
            </h3>
            <RecentLogList
              rows={query.data?.requests}
              emptyKey='No recent requests'
              isAdmin={isAdmin}
              dateFormatter={dateFormatter}
            />
          </section>
          <section aria-label={t('Recent errors')} className='min-w-0'>
            <div className='flex items-center justify-between gap-2'>
              <h3 className='flex min-w-0 items-center gap-2 text-sm font-semibold'>
                <TriangleAlert
                  className='text-destructive size-4 shrink-0'
                  aria-hidden='true'
                />
                {t('Recent errors')}
              </h3>
              <Link
                to='/usage-logs/$section'
                params={{ section: 'common' }}
                search={{ type: ['5'], startTime: errorStartTime }}
                className={buttonVariants({ variant: 'ghost', size: 'xs' })}
                aria-label={`${t('Recent errors')} · ${t('View logs')}`}
              >
                {t('View logs')}
              </Link>
            </div>
            <RecentLogList
              rows={query.data?.errors}
              emptyKey='No recent errors'
              isAdmin={isAdmin}
              dateFormatter={dateFormatter}
            />
          </section>
        </div>
      )}
    </PanelWrapper>
  )
}
