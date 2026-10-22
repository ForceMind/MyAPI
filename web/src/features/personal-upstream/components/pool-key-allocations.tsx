/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import type { ConnectionKeyContext } from '@/features/connections/key-context'
import { toIntlLocale } from '@/i18n/languages'

import type { PoolKeyAllocation, PoolUsage } from '../pool-usage-api'
import { WindowPoints } from './window-points'

function AllocationRow(props: {
  allocation: PoolKeyAllocation
  context: ConnectionKeyContext
}) {
  const { t, i18n } = useTranslation()
  const item = props.allocation
  const percent = new Intl.NumberFormat(toIntlLocale(i18n.language), {
    maximumFractionDigits: 2,
  })
  const withinShare =
    item.share_bps > 0 && item.estimated_used_micro !== null
      ? (item.estimated_used_micro / (item.share_bps * 10000)) * 100
      : null
  let status = t('Unknown')
  if (item.token_state === 'enabled') status = t('Enabled')
  if (item.token_state === 'paused') status = t('Paused')
  if (item.token_state === 'expired') status = t('Expired')
  if (item.token_state === 'exhausted') status = t('Exhausted')
  if (item.token_state === 'deleted') status = t('Deleted')
  if (item.token_state === 'missing') status = t('Key no longer exists')
  return (
    <li
      className='flex min-w-0 flex-col gap-3 border-b py-4 last:border-0'
      data-testid={`pool-key-${item.token_id}`}
    >
      <div className='flex min-w-0 flex-wrap items-start justify-between gap-2'>
        <div className='min-w-0 flex-1 [overflow-wrap:anywhere]'>
          <p className='font-medium'>
            {item.token_name || t('Key #{{id}}', { id: item.token_id })}
          </p>
          <p className='text-muted-foreground mt-1 text-xs'>{status}</p>
        </div>
        <span className='tabular-nums'>
          {percent.format(item.share_bps / 100)}%
        </span>
      </div>
      <Link
        to='/keys'
        search={{ connectionContext: { ...props.context, key: item.token_id } }}
        className='text-primary self-start underline'
      >
        {t('Key #{{id}}', { id: item.token_id })}
      </Link>
      <Progress
        value={item.share_bps / 100}
        aria-label={t('Allocated native window share')}
        className='[&_[data-slot=progress-track]]:h-2.5'
      />
      <dl className='grid min-w-0 grid-cols-[minmax(0,1fr)_minmax(0,1fr)] gap-2 text-sm [&_dd]:text-right'>
        <dt>{t('Local Key estimate')}</dt>
        <dd data-testid='allocation-estimated-usage'>
          <WindowPoints value={item.estimated_used_micro} />
        </dd>
        <dt>{t('Used within this share')}</dt>
        <dd data-testid='allocation-used-within-share'>
          {withinShare === null
            ? t('Unknown')
            : `${percent.format(withinShare)}%`}
        </dd>
      </dl>
      {withinShare !== null ? (
        <Progress
          value={Math.min(100, withinShare)}
          aria-label={t('Used within this share')}
          className='[&_[data-slot=progress-track]]:h-1.5'
        />
      ) : null}
      {withinShare !== null && withinShare > 100 ? (
        <p className='text-warning text-sm'>{t('Over the allocated share')}</p>
      ) : null}
      {item.share_bps === 0 ? (
        <p className='text-muted-foreground text-xs'>
          {t('Share stopped; prior usage is retained.')}
        </p>
      ) : null}
      <dl className='text-muted-foreground grid grid-cols-[minmax(0,1fr)_auto] gap-x-2 gap-y-1 text-xs [&_dd]:text-right'>
        <dt>{t('Pending requests')}</dt>
        <dd>{item.pending_attempts}</dd>
        <dt>{t('Unsampled requests')}</dt>
        <dd>{item.unsampled_attempts}</dd>
        <dt>{t('Uncertain requests')}</dt>
        <dd>{item.uncertain_attempts}</dd>
      </dl>
    </li>
  )
}

export function PoolKeyAllocations(props: {
  usage: PoolUsage
  connectionContext?: ConnectionKeyContext
}) {
  const { t, i18n } = useTranslation()
  const percent = new Intl.NumberFormat(toIntlLocale(i18n.language), {
    maximumFractionDigits: 2,
  })
  return (
    <Card className='pc-card min-w-0' data-testid='pool-key-allocations'>
      <CardHeader>
        <CardTitle>
          <h2>{t('All Key allocations')}</h2>
        </CardTitle>
      </CardHeader>
      <CardContent className='flex min-w-0 flex-col gap-4 [overflow-wrap:anywhere]'>
        {props.usage.allocations.length === 0 ? (
          <p>{t('No Key allocations in this window.')}</p>
        ) : null}
        <ul className='min-w-0'>
          {props.usage.allocations.map((allocation) => (
            <AllocationRow
              key={allocation.token_id}
              allocation={allocation}
              context={
                props.connectionContext ?? {
                  connection: { connection: props.usage.channel_id },
                  pool: props.usage.pool_id,
                }
              }
            />
          ))}
        </ul>
        <dl className='grid grid-cols-[minmax(0,1fr)_auto] gap-3 text-sm [&_dd]:text-right'>
          <dt>{t('Total allocated')}</dt>
          <dd>{percent.format(props.usage.allocated_bps / 100)}%</dd>
          <dt>{t('Unallocated')}</dt>
          <dd>{percent.format((10000 - props.usage.allocated_bps) / 100)}%</dd>
        </dl>
        <p className='text-muted-foreground text-xs leading-relaxed'>
          {t(
            'Shares are approximate and do not reserve upstream capacity. Concurrent requests and sampling delays can exceed the share.'
          )}
        </p>
        {!props.connectionContext && (
          <Button
            nativeButton={false}
            render={
              <Link
                to='/keys'
                search={{
                  connectionContext: props.connectionContext ?? {
                    connection: { connection: props.usage.channel_id },
                    pool: props.usage.pool_id,
                  },
                }}
              />
            }
            className='h-auto min-h-11 max-w-full whitespace-normal'
          >
            {t('Manage Key shares')}
          </Button>
        )}
      </CardContent>
    </Card>
  )
}
