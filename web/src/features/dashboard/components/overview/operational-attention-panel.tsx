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

import { getChannelRouting } from '@/features/channel-routing/api'
import { getQuotaStateLabel } from '@/features/channel-routing/labels'
import { getChannelQuotaChanges } from '@/features/channels/api'
import { quotaSeriesKey } from '@/features/channels/lib/quota-history'
import { hasPermission } from '@/lib/admin-permissions'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

export function OperationalAttentionPanel() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const allowed =
    Boolean(user && user.role >= ROLE.ADMIN) &&
    hasPermission(user, 'channel', 'read')
  const routing = useQuery({
    queryKey: [
      'dashboard',
      'overview',
      'routing-policy',
      user?.id ?? null,
      sessionId,
    ],
    queryFn: getChannelRouting,
    enabled: allowed,
    staleTime: 60000,
    retry: false,
    meta: { errorHandledLocally: true },
  })
  const quota = useQuery({
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
    staleTime: 60000,
    retry: false,
    meta: { errorHandledLocally: true },
  })
  if (!allowed) return null
  const failed =
    routing.isError ||
    quota.isError ||
    routing.data?.success === false ||
    quota.data?.success === false
  const loading = routing.isLoading || quota.isLoading
  const channels = routing.data?.data?.channels ?? []
  const concerns = channels.filter(
    (channel) => channel.status === 1 && channel.quota.state !== 'fresh'
  )
  const lowQuota = (quota.data?.data?.items ?? []).filter(
    (item) =>
      item.alert?.enabled &&
      ['warning', 'critical'].includes(item.alert.status) &&
      !concerns.some((channel) => channel.id === item.channel_id)
  )
  const disabled = routing.data?.data?.policy.enabled === false
  return (
    <section
      aria-label={t('Needs attention')}
      className='bg-card min-w-0 space-y-3 rounded-xl border p-4'
    >
      <h3 className='text-sm font-semibold'>{t('Needs attention')}</h3>
      {loading && (
        <p role='status' className='text-muted-foreground text-sm'>
          {t('Loading')}
        </p>
      )}
      {failed && (
        <p role='alert' className='text-destructive text-sm'>
          {t('Unable to load account quota changes')}
        </p>
      )}
      <ul className='space-y-3 text-sm'>
        {concerns.slice(0, 4).map((channel) => (
          <li key={channel.id} className='space-y-1'>
            <Link
              to='/channels'
              search={{ tab: 'quota', quotaChannelId: channel.id }}
              className='text-primary block font-medium break-words'
            >
              {channel.name}
            </Link>
            <p className='text-muted-foreground text-xs'>
              {getQuotaStateLabel(t, channel.quota.state)}
            </p>
          </li>
        ))}
        {lowQuota.slice(0, Math.max(0, 4 - concerns.length)).map((item) => (
          <li key={quotaSeriesKey(item)} className='space-y-1'>
            <Link
              to='/channels'
              search={{ tab: 'quota', quotaChannelId: item.channel_id }}
              className='text-primary block font-medium break-words'
            >
              {item.name}
            </Link>
            <p className='text-muted-foreground text-xs'>{t('Quota alert')}</p>
          </li>
        ))}
      </ul>
      {!loading && !failed && !concerns.length && !lowQuota.length && (
        <p className='text-muted-foreground text-xs'>
          {t('No quota alerts in the loaded samples.')}
        </p>
      )}
      {disabled && (
        <div className='space-y-1 border-t pt-3'>
          <p className='text-sm'>{t('Intelligent allocation is disabled.')}</p>
          <Link
            to='/channels'
            search={{ tab: 'routing' }}
            className='text-primary text-xs'
          >
            {t('Preview routing')}
          </Link>
        </div>
      )}
      <Link
        to='/channels'
        search={{ tab: 'quota' }}
        className='text-primary inline-block text-xs'
      >
        {t('Quota analysis')}
      </Link>
    </section>
  )
}
