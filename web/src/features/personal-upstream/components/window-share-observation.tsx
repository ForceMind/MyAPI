/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { quotaWindowLabel } from '@/features/channels/lib/quota-history'
import { getSubscriptionShare } from '@/features/keys/subscription-share-api'
import { getPlaygroundKeys } from '@/features/playground/api'
import type { ConnectionKeyContext } from '@/features/connections/key-context'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { PoolUsageObservation } from './pool-usage-observation'
import { WindowPoints } from './window-points'

export function WindowShareObservation(props: {
  channelId: number
  connectionContext?: ConnectionKeyContext
  canReadPoolUsage?: boolean
  selectedKeyId?: string
  onKeyChange?: (keyId: string) => void
}) {
  const { t } = useTranslation()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const role = useAuthStore((state) => state.auth.user?.role)
  const sessionId = useAuthStore((state) => state.auth.session?.sid)
  const [uncontrolledKeyId, setUncontrolledKeyId] = useState('')
  const keyId = props.selectedKeyId ?? uncontrolledKeyId
  const keysQuery = useQuery({
    queryKey: ['personal-console', 'keys', userId, role, sessionId],
    queryFn: getPlaygroundKeys,
    retry: false,
    refetchOnWindowFocus: false,
  })
  const confirmedKey = !keysQuery.isError
    ? keysQuery.data?.find((key) => String(key.id) === keyId)
    : undefined
  const shareQuery = useQuery({
    queryKey: [
      'personal-console',
      'upstream-share',
      userId,
      role,
      sessionId,
      confirmedKey?.id,
      props.channelId,
    ],
    queryFn: ({ signal }) =>
      getSubscriptionShare(confirmedKey?.id ?? -1, signal),
    enabled: Boolean(confirmedKey),
    retry: false,
    refetchOnWindowFocus: false,
  })
  const pools =
    !shareQuery.isError && confirmedKey
      ? shareQuery.data?.pools.filter(
          (pool) =>
            pool.channel_id === props.channelId &&
            pool.pool_id > 0 &&
            pool.state !== 'closed'
        )
      : undefined
  return (
    <section
      className='flex min-w-0 flex-col gap-5'
      data-testid='upstream-share-observation'
    >
      <Card className='pc-card min-w-0'>
        <CardHeader>
          <CardTitle>
            <h2>{t('Key share settings')}</h2>
          </CardTitle>
        </CardHeader>
        <CardContent className='flex min-w-0 flex-col gap-4'>
          <label htmlFor='upstream-share-key' className='font-medium'>
            {t('Select an API Key')}
          </label>
          <select
            id='upstream-share-key'
            value={keyId}
            aria-describedby={
              confirmedKey ? 'upstream-share-key-name' : undefined
            }
            disabled={keysQuery.isLoading || keysQuery.isError}
            onChange={(event) => {
              setUncontrolledKeyId(event.target.value)
              props.onKeyChange?.(event.target.value)
            }}
            className='bg-background h-11 w-full min-w-0 rounded-md border px-2'
          >
            <option value=''>{t('Choose a Key')}</option>
            {!keysQuery.isError
              ? keysQuery.data?.map((key) => (
                  <option key={key.id} value={key.id}>
                    {key.name}
                  </option>
                ))
              : null}
          </select>
          {confirmedKey ? (
            <p
              id='upstream-share-key-name'
              data-testid='upstream-share-selected-key'
              className='text-muted-foreground text-xs [overflow-wrap:anywhere]'
            >
              {confirmedKey.name}
            </p>
          ) : null}
          {keysQuery.isError ? (
            <Alert variant='destructive'>
              <AlertTitle>{t('Unable to load Keys')}</AlertTitle>
              {keysQuery.error instanceof Error ? (
                <AlertDescription className='[overflow-wrap:anywhere]'>
                  {keysQuery.error.message}
                </AlertDescription>
              ) : null}
              <Button
                variant='outline'
                onClick={() => void keysQuery.refetch()}
              >
                {t('Retry')}
              </Button>
            </Alert>
          ) : null}
          {shareQuery.isLoading ? <Skeleton className='h-24 w-full' /> : null}
          {shareQuery.isError ? (
            <Alert variant='destructive'>
              <AlertTitle>{t('Unable to load Key shares')}</AlertTitle>
              {shareQuery.error instanceof Error ? (
                <AlertDescription className='[overflow-wrap:anywhere]'>
                  {shareQuery.error.message}
                </AlertDescription>
              ) : null}
              <Button
                variant='outline'
                onClick={() => void shareQuery.refetch()}
              >
                {t('Retry')}
              </Button>
            </Alert>
          ) : null}
          {!confirmedKey && !keysQuery.isError ? (
            <p className='text-muted-foreground'>
              {t('Choose a Key to observe its native window share.')}
            </p>
          ) : null}
          {pools?.length === 0 ? (
            <p className='text-muted-foreground'>
              {role === ROLE.SUPER_ADMIN
                ? t('This Key has no share for the selected account.')
                : t(
                    'Root access is required to match Key shares to this account.'
                  )}
            </p>
          ) : null}
          {!props.canReadPoolUsage
            ? pools?.map((pool) => (
                <div
                  key={`${pool.pool_id}:${pool.snapshot_id}`}
                  className='flex min-w-0 flex-col gap-3 rounded-lg border p-4'
                >
                  <div className='flex min-w-0 flex-wrap justify-between gap-2'>
                    <p className='font-medium break-words'>
                      {pool.channel_name} ·{' '}
                      {quotaWindowLabel(pool.window_type, t)} ·{' '}
                      {quotaWindowLabel(pool.window_slot, t)}
                    </p>
                    <span>{t('Approximate')}</span>
                  </div>
                  <dl className='grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] gap-x-3 gap-y-3 text-sm [overflow-wrap:anywhere] [&_dd]:text-right'>
                    <dt>{t('This Key allocation')}</dt>
                    <dd>{pool.share_bps / 100}%</dd>
                    <dt>{t('Total allocated')}</dt>
                    <dd>{pool.allocated_bps / 100}%</dd>
                    <dt>{t('Unallocated')}</dt>
                    <dd>{(10000 - pool.allocated_bps) / 100}%</dd>
                    <dt>{t('Local Key estimate')}</dt>
                    <dd>
                      {pool.baseline_time > 0 &&
                      pool.state !== 'baseline_pending' ? (
                        <WindowPoints value={pool.estimated_used_micro} />
                      ) : (
                        t('Unknown')
                      )}
                    </dd>
                    <dt>{t('Unattributed usage (including initial baseline)')}</dt>
                    <dd>
                      {pool.baseline_time > 0 &&
                      pool.state !== 'baseline_pending' ? (
                        <WindowPoints value={pool.unattributed_micro} />
                      ) : (
                        t('Unknown')
                      )}
                    </dd>
                    <dt>{t('Pending requests')}</dt>
                    <dd>{pool.pending_attempts}</dd>
                    <dt>{t('Unsampled requests')}</dt>
                    <dd>{pool.unsampled_attempts}</dd>
                    <dt>{t('Uncertain requests')}</dt>
                    <dd>{pool.uncertain_attempts ?? t('Unknown')}</dd>
                    <dt>{t('Sample age')}</dt>
                    <dd>
                      {t('{{count}} seconds', { count: pool.sample_age_seconds })}
                    </dd>
                  </dl>
                  <Progress
                    value={pool.allocated_bps / 100}
                    aria-label={t('Total allocated')}
                    className='[&_[data-slot=progress-track]]:h-2.5'
                  />
                  <dl className='text-muted-foreground grid min-w-0 grid-cols-[minmax(0,1fr)_minmax(0,1fr)] gap-2 border-t pt-3 text-xs [overflow-wrap:anywhere] [&_dd]:text-right'>
                    <dt>{t('Source')}</dt>
                    <dd className='font-mono'>
                      {pool.window_source || t('Unknown')}
                    </dd>
                    <dt>{t('Status')}</dt>
                    <dd><code>{pool.state}</code></dd>
                    {pool.reason ? (
                      <>
                        <dt>{t('Reason')}</dt>
                        <dd><code>{pool.reason}</code></dd>
                      </>
                    ) : null}
                  </dl>
                  {pool.sample_age_seconds > pool.max_age_seconds ? (
                    <p className='text-muted-foreground'>{t('Stale sample')}</p>
                  ) : null}
                  {pool.state !== 'active' ? (
                    <p className='text-muted-foreground'>
                      {t(
                        'Window observations are unavailable; existing estimates are preserved.'
                      )}
                    </p>
                  ) : null}
                  {pool.replacement_required || pool.rebaseline_required ? (
                    <p className='text-muted-foreground'>
                      {t(
                        'This window needs new upstream evidence before it can be used.'
                      )}
                    </p>
                  ) : null}
                </div>
              ))
            : null}
          {!props.canReadPoolUsage || !pools?.length ? (
            <>
              <p className='bg-muted rounded-lg p-3 text-xs leading-relaxed'>
                {t(
                  'Shares are approximate and do not reserve upstream capacity. Concurrent requests and sampling delays can exceed the share.'
                )}
              </p>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Upstream account usage is not exact per-Key attribution. Windows are observed separately.'
                )}
              </p>
              <Button
                nativeButton={false}
                render={
                  <Link
                    to='/keys'
                    search={{
                      connectionContext:
                        role === ROLE.SUPER_ADMIN
                          ? (props.connectionContext ?? {
                              connection: { connection: props.channelId },
                            })
                          : undefined,
                    }}
                  />
                }
              >
                {t('Manage Key shares')}
              </Button>
            </>
          ) : null}
        </CardContent>
      </Card>
      {props.canReadPoolUsage
        ? pools?.map((pool) => (
            <PoolUsageObservation
              key={`${userId}:${sessionId}:${pool.pool_id}`}
              pool={pool}
              channelId={props.channelId}
              canRead={props.canReadPoolUsage === true}
              selectedKeyId={confirmedKey?.id}
              connectionContext={props.connectionContext}
            />
          ))
        : null}
    </section>
  )
}
