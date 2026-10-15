/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { Alert, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  formatQuotaDuration,
  quotaWindowLabel,
} from '@/features/channels/lib/quota-history'
import type { SubscriptionSharePool } from '@/features/keys/subscription-share-api'
import { toIntlLocale } from '@/i18n/languages'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getPoolUsage } from '../pool-usage-api'
import { PoolKeyAllocations } from './pool-key-allocations'
import { PoolUsageSummary } from './pool-usage-summary'

export function PoolUsageObservation(props: {
  pool: SubscriptionSharePool
  channelId: number
  canRead: boolean
}) {
  const { t, i18n } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const sid = useAuthStore((state) => state.auth.session?.sid)
  const allowed = Boolean(
    props.canRead &&
    user?.role === ROLE.SUPER_ADMIN &&
    sid &&
    props.pool.pool_id > 0 &&
    props.pool.channel_id === props.channelId &&
    props.pool.state !== 'closed'
  )
  const query = useQuery({
    queryKey: [
      'personal-console',
      'pool-usage',
      user?.id,
      sid,
      props.channelId,
      props.pool.pool_id,
      props.pool.snapshot_id,
      props.pool.revision,
    ],
    queryFn: ({ signal }) =>
      getPoolUsage(props.pool.pool_id, props.channelId, signal),
    enabled: allowed,
    retry: false,
    refetchOnWindowFocus: false,
    refetchInterval: 60_000,
  })
  if (!allowed) return null
  const usage = query.isError ? undefined : query.data
  const descriptor = usage ?? props.pool
  return (
    <section
      className='flex min-w-0 flex-col gap-4'
      aria-label={t('Pool window usage')}
      data-testid={`pool-usage-${props.pool.pool_id}`}
    >
      <div className='flex min-w-0 flex-wrap items-center justify-between gap-2'>
        <p
          className='min-w-0 font-medium [overflow-wrap:anywhere]'
          data-testid='pool-window-descriptor'
        >
          {descriptor.channel_name} ·{' '}
          {formatQuotaDuration(descriptor.window_seconds, t)} ·{' '}
          {quotaWindowLabel(descriptor.window_slot, t)}
        </p>
        <Button
          variant='outline'
          size='sm'
          disabled={query.isFetching}
          onClick={() => void query.refetch()}
        >
          {t('Refresh')}
        </Button>
      </div>
      {query.isPending ? (
        <Skeleton
          className='h-48 w-full'
          aria-label={t('Loading window usage')}
        />
      ) : null}
      {query.isError ? (
        <Alert variant='destructive' data-testid='pool-usage-error'>
          <AlertTitle>{t('Unable to load window usage')}</AlertTitle>
          <Button variant='outline' onClick={() => void query.refetch()}>
            {t('Retry')}
          </Button>
        </Alert>
      ) : null}
      {usage ? (
        <>
          <p
            className='text-muted-foreground text-sm'
            data-testid='pool-usage-freshness'
          >
            {t('Sample age')}:{' '}
            {t('{{count}} seconds', { count: usage.sample_age_seconds })} ·{' '}
            {t('Approximate')}
          </p>
          {usage.observed_at > 0 ? (
            <p className='text-muted-foreground text-xs'>
              {t('Last processed observation {{time}}', {
                time: new Intl.DateTimeFormat(toIntlLocale(i18n.language), {
                  dateStyle: 'short',
                  timeStyle: 'short',
                }).format(new Date(usage.observed_at * 1000)),
              })}
            </p>
          ) : null}
          {usage.sample_age_seconds > usage.max_age_seconds ? (
            <p className='text-warning text-sm'>{t('Stale sample')}</p>
          ) : null}
          {usage.state !== 'active' ? (
            <p
              className='text-muted-foreground text-sm'
              data-testid='pool-preserved-estimates'
            >
              {t(
                'Preserved estimates. The last processed observation does not confirm these values.'
              )}
            </p>
          ) : null}
          <div className='grid min-w-0 items-start gap-5 lg:grid-cols-2'>
            <PoolUsageSummary usage={usage} />
            <PoolKeyAllocations usage={usage} />
          </div>
        </>
      ) : null}
    </section>
  )
}
