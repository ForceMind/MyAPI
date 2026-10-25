/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { FilterHorizontalIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { quotaWindowLabel } from '@/features/channels/lib/quota-history'
import { formatTimestampToDate } from '@/lib/format'

import { useKeySubscriptionShare } from '../../hooks/use-key-subscription-share'
import { heldSharePools, shareUsedPercent } from '../../lib/share-usage'
import type { ApiKey } from '../../types'
import { SubscriptionShareDialog } from '../subscription-share-dialog'

/**
 * Approximate shares of native subscription windows held by this Key. They
 * limit requests; they do not reserve upstream capacity. An unknown baseline
 * shows unknown usage, never 0%.
 */
export function KeySharesPanel(props: { apiKey: ApiKey }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [editing, setEditing] = useState(false)
  const share = useKeySubscriptionShare(props.apiKey.id)

  if (editing) {
    return (
      <SubscriptionShareDialog
        personalKey={props.apiKey}
        tokenId={props.apiKey.id}
        onClose={() => {
          setEditing(false)
          void client.invalidateQueries({ queryKey: ['subscription-share'] })
        }}
      />
    )
  }

  const pools = heldSharePools(share.data)

  return (
    <div className='flex min-w-0 flex-col gap-3'>
      <p className='text-caption text-muted-foreground'>
        {t(
          'Shares are approximate request limits, not reserved upstream capacity. Each native window is managed separately.'
        )}
      </p>
      {share.isPending && (
        <div className='flex flex-col gap-2' aria-busy='true'>
          <span className='sr-only'>
            {t('Loading subscription windows...')}
          </span>
          <Skeleton className='h-24 w-full' />
        </div>
      )}
      {share.isError && (
        <Alert variant='destructive'>
          <AlertTitle>{t('Could not load subscription shares.')}</AlertTitle>
          <AlertDescription>
            <Button
              variant='outline'
              size='sm'
              onClick={() => void share.refetch()}
            >
              {t('Retry')}
            </Button>
          </AlertDescription>
        </Alert>
      )}
      {share.data && !share.isError && pools.length === 0 && (
        <p className='text-body text-muted-foreground'>
          {t('No subscription share is configured for this Key.')}
        </p>
      )}
      {!share.isError &&
        pools.map((pool) => {
          const knownUsage = pool.baseline_time > 0
          const used = shareUsedPercent(pool)
          const paused =
            pool.share_bps === 0 ||
            pool.state !== 'active' ||
            (knownUsage && pool.estimated_used_micro >= pool.share_bps * 10_000)
          const slot = quotaWindowLabel(pool.window_slot, t)
          return (
            <section
              key={`${pool.pool_id}:${pool.snapshot_id}`}
              className='flex min-w-0 flex-col gap-3 rounded-lg border p-3'
              aria-label={`${pool.channel_name} ${slot}`}
            >
              <div className='flex min-w-0 flex-wrap items-start justify-between gap-2'>
                <span className='text-data min-w-0 font-medium wrap-anywhere'>
                  {pool.channel_name || t('Native window')} · {slot} ·{' '}
                  {quotaWindowLabel(pool.window_type, t)}
                </span>
                <Badge variant='outline'>
                  {paused ? t('Requests paused') : t('Approximate')}
                </Badge>
              </div>
              <dl className='grid grid-cols-2 gap-3'>
                <div>
                  <dt className='text-caption text-muted-foreground'>
                    {t('This Key share')}
                  </dt>
                  <dd className='text-section tabular-nums'>
                    {pool.share_bps / 100}%
                  </dd>
                </div>
                <div>
                  <dt className='text-caption text-muted-foreground'>
                    {t('Estimated usage of share')}
                  </dt>
                  <dd className='text-section tabular-nums'>
                    {used === null
                      ? t('Unknown')
                      : `${Number(used.toFixed(2))}%`}
                  </dd>
                </div>
              </dl>
              <p className='text-caption text-muted-foreground wrap-anywhere'>
                {t('Observed at')}: {formatTimestampToDate(pool.observed_at)}
              </p>
              {(pool.pending_attempts > 0 || pool.unsampled_attempts > 0) && (
                <p className='text-caption text-muted-foreground'>
                  {t('Usage is still awaiting attribution.')}
                </p>
              )}
              {pool.state !== 'active' && (
                <p className='text-caption text-warning'>
                  {t('Window unavailable or stale; later requests are paused.')}
                </p>
              )}
            </section>
          )
        })}
      <Button
        variant='outline'
        className='self-start'
        onClick={() => setEditing(true)}
      >
        <HugeiconsIcon
          icon={FilterHorizontalIcon}
          data-icon='inline-start'
          aria-hidden='true'
        />
        {t('Window share')}
      </Button>
    </div>
  )
}
