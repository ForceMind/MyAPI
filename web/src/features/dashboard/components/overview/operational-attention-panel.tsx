/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { getChannelQuotaChanges } from '@/features/channels/api'
import { quotaSeriesKey } from '@/features/channels/lib/quota-history'
import type { ChannelQuotaChangeItem } from '@/features/channels/types'
import { hasPermission } from '@/lib/admin-permissions'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

function attentionLabel(
  item: ChannelQuotaChangeItem,
  t: (key: string) => string
): string {
  if (item.alert?.status === 'critical' || item.alert?.status === 'warning') {
    return t('Quota alert')
  }
  if (item.status === 'error') return t('Error')
  if (item.status === 'unsupported') return t('Unsupported')
  return t('Unavailable')
}

export function OperationalAttentionPanel() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const allowed =
    Boolean(user && user.role >= ROLE.ADMIN) &&
    hasPermission(user, 'channel', 'read')
  const quotaQuery = useQuery({
    queryKey: [
      'dashboard',
      'overview',
      'quota-attention',
      user?.id ?? null,
      sessionId,
    ],
    queryFn: () =>
      getChannelQuotaChanges({
        range: '24h',
        limit: 20,
        sort: 'observed_desc',
      }),
    enabled: allowed,
    staleTime: 60 * 1000,
    retry: false,
    meta: { errorHandledLocally: true },
  })
  const concerns = useMemo(
    () =>
      (quotaQuery.data?.data?.items ?? []).filter(
        (item) =>
          item.status !== 'success' ||
          item.alert?.status === 'warning' ||
          item.alert?.status === 'critical'
      ),
    [quotaQuery.data?.data?.items]
  )

  if (!allowed) return null

  return (
    <section
      aria-label={t('Needs attention')}
      className='bg-card min-w-0 space-y-3 rounded-xl border p-4'
    >
      <h3 className='text-sm font-semibold'>{t('Needs attention')}</h3>
      {quotaQuery.isLoading ? (
        <p role='status' className='text-muted-foreground text-sm'>
          {t('Loading')}
        </p>
      ) : null}
      {quotaQuery.isError || quotaQuery.data?.success === false ? (
        <p role='alert' className='text-destructive text-sm'>
          {t('Unable to load account quota changes')}
        </p>
      ) : null}
      <ul className='space-y-3 text-sm'>
        {concerns.slice(0, 4).map((item) => (
          <li key={quotaSeriesKey(item)} className='space-y-1'>
            <Link
              to='/channels'
              search={{ tab: 'quota', quotaChannelId: item.channel_id }}
              className='text-primary block font-medium break-words'
            >
              {item.account_label || item.name}
            </Link>
            <p className='text-muted-foreground text-xs'>
              {attentionLabel(item, t)}
            </p>
          </li>
        ))}
      </ul>
      {!quotaQuery.isLoading &&
      !quotaQuery.isError &&
      quotaQuery.data?.success !== false &&
      concerns.length === 0 ? (
        <p className='text-muted-foreground text-xs'>
          {t('No quota alerts in the loaded samples.')}
        </p>
      ) : null}
    </section>
  )
}
