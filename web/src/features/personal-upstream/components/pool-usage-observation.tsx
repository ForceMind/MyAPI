/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  formatQuotaDuration,
  quotaWindowLabel,
} from '@/features/channels/lib/quota-history'
import type { ConnectionKeyContext } from '@/features/connections/key-context'
import type { SubscriptionSharePool } from '@/features/keys/subscription-share-api'
import { toIntlLocale } from '@/i18n/languages'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getPoolUsage } from '../pool-usage-api'
import { PoolKeyAllocations } from './pool-key-allocations'
import { PoolUsageSummary } from './pool-usage-summary'
import { WindowPoints } from './window-points'

export function PoolUsageObservation(props: {
  pool: SubscriptionSharePool
  channelId: number
  canRead: boolean
  selectedKeyId?: number
  connectionContext?: ConnectionKeyContext
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
      props.selectedKeyId,
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
  const selectedAllocation = usage?.allocations.find(
    (allocation) => allocation.token_id === props.selectedKeyId
  )
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
          {t('Refresh saved observations')}
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
          {query.error instanceof Error ? (
            <AlertDescription className='[overflow-wrap:anywhere]'>
              {query.error.message}
            </AlertDescription>
          ) : null}
          <Button variant='outline' onClick={() => void query.refetch()}>
            {t('Retry')}
          </Button>
        </Alert>
      ) : null}
      {usage ? (
        <>
          <dl className='text-muted-foreground grid min-w-0 grid-cols-1 gap-x-6 gap-y-2 rounded-lg border p-4 text-xs sm:grid-cols-2 [overflow-wrap:anywhere]'>
            <div
              className='flex flex-wrap gap-x-2'
              data-testid='pool-usage-freshness'
            >
              <dt>{t('Sample age')}:</dt>
              <dd>
                {t('{{count}} seconds', { count: usage.sample_age_seconds })} ·{' '}
                {t('Approximate')}
              </dd>
            </div>
            <div>
              <dt className='sr-only'>{t('Observation time')}</dt>
              <dd>
                {usage.observed_at > 0
                  ? t('Last processed observation {{time}}', {
                      time: new Intl.DateTimeFormat(toIntlLocale(i18n.language), {
                        dateStyle: 'short',
                        timeStyle: 'short',
                      }).format(new Date(usage.observed_at * 1000)),
                    })
                  : t('No timestamp')}
              </dd>
            </div>
            <div className='flex flex-wrap gap-x-2'>
              <dt>{t('Status')}:</dt>
              <dd><code>{usage.state}</code></dd>
            </div>
            {usage.reason ? (
              <div className='flex flex-wrap gap-x-2'>
                <dt>{t('Reason')}:</dt>
                <dd><code>{usage.reason}</code></dd>
              </div>
            ) : null}
            <div className='flex flex-wrap gap-x-2'>
              <dt>{t('Plan')}:</dt>
              <dd>{usage.plan_type}</dd>
            </div>
            <div className='flex flex-wrap gap-x-2'>
              <dt>{t('Reset time')}:</dt>
              <dd>
                {usage.reset_at > 0
                  ? new Intl.DateTimeFormat(toIntlLocale(i18n.language), {
                      dateStyle: 'short',
                      timeStyle: 'short',
                    }).format(new Date(usage.reset_at * 1000))
                  : t('Unknown')}
              </dd>
            </div>
          </dl>
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
          {props.selectedKeyId ? (
            <Card className='pc-card min-w-0' data-testid='pool-current-key'>
              <CardHeader>
                <CardTitle>
                  <h2>{t('Current Key share')}</h2>
                </CardTitle>
              </CardHeader>
              <CardContent className='flex min-w-0 flex-col gap-3 [overflow-wrap:anywhere]'>
                {selectedAllocation ? (
                  <>
                    <p className='font-medium'>
                      {selectedAllocation.token_name ||
                        t('Key #{{id}}', { id: selectedAllocation.token_id })}
                    </p>
                    <dl className='grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] gap-3 text-sm [&_dd]:text-right'>
                      <dt>{t('This Key allocation')}</dt>
                      <dd>
                        {new Intl.NumberFormat(toIntlLocale(i18n.language), {
                          maximumFractionDigits: 2,
                        }).format(selectedAllocation.share_bps / 100)}%
                      </dd>
                      <dt>{t('Local Key estimate')}</dt>
                      <dd>
                        <WindowPoints
                          value={selectedAllocation.estimated_used_micro}
                        />
                      </dd>
                    </dl>
                  </>
                ) : (
                  <p>{t('This Key has no share for the selected account.')}</p>
                )}
              </CardContent>
            </Card>
          ) : null}
          <div className='grid min-w-0 items-start gap-5 lg:grid-cols-2'>
            <PoolUsageSummary usage={usage} />
            <PoolKeyAllocations usage={usage} returnContext={props.connectionContext ? { ...props.connectionContext, pool: usage.pool_id } : undefined} />
          </div>
        </>
      ) : null}
    </section>
  )
}
