/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { UseQueryResult } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { QuotaRing } from '@/components/quota-ring'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { quotaSeriesKey } from '@/features/channels/lib/quota-history'
import type { ChannelQuotaChangeItem } from '@/features/channels/types'
import {
  describeNativeWindowState,
  nativeWindowState,
  nativeWindowUsage,
} from '@/features/personal-upstream/window-state'
import { formatTimestampToDate } from '@/lib/format'

import { windowName } from './connection-format'

/**
 * The connection's saved account windows (design v2 upstream "windows"):
 * one large ring per window with the share used written inside and the
 * share left beside it, when the observation is trustworthy. A failed,
 * unsupported or untimed sample shows its state instead of a number.
 */
export function ConnectionWindows(props: {
  query: Pick<
    UseQueryResult<ChannelQuotaChangeItem[]>,
    'data' | 'isPending' | 'isError' | 'isFetching' | 'refetch'
  >
}) {
  const { t } = useTranslation()
  const now = Date.now() / 1000
  return (
    <section
      aria-label={t('Saved account windows')}
      className='bg-card p-card flex min-w-0 flex-col gap-4 rounded-xl border'
    >
      {props.query.isPending && <p role='status'>{t('Loading...')}</p>}
      {props.query.isError && (
        <div role='alert' className='flex flex-col items-start gap-2'>
          <p>{t('Unable to load account windows')}</p>
          <Button
            variant='outline'
            disabled={props.query.isFetching}
            onClick={() => void props.query.refetch()}
          >
            {t('Retry')}
          </Button>
        </div>
      )}
      {props.query.data?.length === 0 && !props.query.isError && (
        <p className='text-body text-muted-foreground'>
          {t(
            'No saved subscription windows for this connection. This does not mean the connection is unavailable.'
          )}
        </p>
      )}
      {!props.query.isError && (props.query.data?.length ?? 0) > 0 && (
        <ul className='flex min-w-0 flex-wrap gap-x-8 gap-y-5'>
          {props.query.data?.map((item) => {
            const used = nativeWindowUsage(item)
            const state = nativeWindowState(item, now)
            const described = describeNativeWindowState(state, t)
            const name = windowName(item, t)
            const observed = item.observed_at
            return (
              <li
                key={quotaSeriesKey(item)}
                data-testid='connection-window'
                className='flex min-w-0 items-center gap-4'
              >
                {used === null ? (
                  <span
                    aria-hidden='true'
                    className='border-meter-track size-24 shrink-0 rounded-full border-[0.6rem] sm:size-28'
                  />
                ) : (
                  <QuotaRing
                    used={used}
                    label={t('{{window}}: {{used}}% used, {{left}}% left', {
                      window: name,
                      used: Math.round(used),
                      left: Math.round(100 - used),
                    })}
                    className='size-24 sm:size-28'
                  />
                )}
                <div className='flex min-w-0 flex-col gap-0.5'>
                  <h3 className='text-caption text-muted-foreground'>{name}</h3>
                  <p className='text-lg leading-7 font-semibold tabular-nums'>
                    {used === null
                      ? t('Usage unavailable')
                      : t('{{value}}% left', {
                          value: Math.round(100 - used),
                        })}
                  </p>
                  <p className='text-caption text-muted-foreground'>
                    {Number.isSafeInteger(observed) && (observed ?? 0) > 0
                      ? t('Observed {{time}}', {
                          time: formatTimestampToDate(observed as number),
                        })
                      : t('No timestamp')}
                  </p>
                  {state !== 'available' && (
                    <StatusBadge
                      variant={described.variant}
                      label={described.label}
                      copyable={false}
                      className='mt-1 h-auto min-h-5 self-start whitespace-normal'
                    />
                  )}
                </div>
              </li>
            )
          })}
        </ul>
      )}
      <p className='text-caption text-muted-foreground'>
        {t(
          'Only saved observations are read. Opening a connection does not contact its provider.'
        )}
      </p>
    </section>
  )
}
