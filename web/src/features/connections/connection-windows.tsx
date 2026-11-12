/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useTranslation } from 'react-i18next'

import { QuotaRing } from '@/components/quota-ring'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { quotaSeriesKey } from '@/features/channels/lib/quota-history'
import type { ChannelQuotaChangeItem } from '@/features/channels/types'
import { nativeWindowResetLabel, nativeWindowUsage } from '@/features/personal-upstream/window-state'
import { formatTimestampToDate } from '@/lib/format'

import { windowName } from './connection-format'
import { ConnectionEvidenceQuality } from './connection-evidence-summary'
import { describeConnectionWindow, savedBalanceAmount } from './window-state'

/**
 * The connection's saved account windows (design v2 upstream "windows"):
 * one large ring per window with the share used written inside and the
 * share left beside it, when the observation is trustworthy. A failed,
 * unsupported or untimed sample shows its state instead of a number.
 */
export function ConnectionWindows(props: {
  query: {
    data: ChannelQuotaChangeItem[] | undefined
    isPending: boolean
    isError: boolean
    isFetching: boolean
    complete: boolean
    refetch: () => unknown
  }
  now: number
}) {
  const { t } = useTranslation()
  const hasBalance = props.query.data?.some((item) => item.metric_type === 'balance') ?? false
  const heading = hasBalance ? t('Saved quota evidence') : t('Saved account windows')
  return (
    <section
      aria-label={heading}
      className='bg-card p-card flex min-w-0 flex-col gap-4 rounded-xl border'
    >
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <h2 className='text-body font-semibold'>{heading}</h2>
        <Button
          variant='outline'
          size='sm'
          disabled={props.query.isFetching}
          onClick={() => void props.query.refetch()}
        >
          {t('Refresh saved windows')}
        </Button>
      </div>
      {props.query.isPending && <p role='status'>{t('Loading...')}</p>}
      {props.query.isError && (
        <div role='alert' className='text-body text-destructive'>
          <p>{t('Unable to load account windows')}</p>
        </div>
      )}
      {!props.query.isPending && !props.query.isError && !props.query.complete && (
        <p role='status' className='text-body text-warning'>{t('Saved quota evidence is incomplete. Missing series and earlier failures may be omitted.')}</p>
      )}
      {props.query.data?.length === 0 && props.query.complete && !props.query.isError && (
        <p className='text-body text-muted-foreground'>
          {t(
            'No saved subscription windows for this connection. This does not mean the connection is unavailable.'
          )}
        </p>
      )}
      {!props.query.isPending && !props.query.isError && !props.query.data?.some((item) => item.metric_type === 'codex_rate_limit') && (
        <p className='text-body text-muted-foreground'>{t('Window query capability unknown')}</p>
      )}
      {!props.query.isError && (props.query.data?.length ?? 0) > 0 && (
        <ul className='flex min-w-0 flex-col gap-4'>
          {props.query.data?.map((item) => {
            const used = nativeWindowUsage(item)
            const reset = nativeWindowResetLabel(item, props.now, t)
            const described = describeConnectionWindow(
              item,
              props.query.data ?? [],
              props.now,
              t
            )
            const name = windowName(item, t)
            const observed = item.observed_at
            let value = used === null ? t('Usage unavailable') : t('{{value}}% left', { value: Math.round(100 - used) })
            const balance = savedBalanceAmount(item)
            if (balance !== null) {
              value = t('{{value}} {{unit}}', { value: balance, unit: item.currency || item.unit || t('Unknown') })
            }
            return (
              <li
                key={quotaSeriesKey(item)}
                data-testid='connection-window'
                className='grid min-w-0 gap-4 rounded-lg border p-4 lg:grid-cols-2 lg:gap-6'
              >
                <div className='flex min-w-0 items-center gap-4'>
                  {used === null && item.metric_type === 'codex_rate_limit' ? (
                    <span
                      aria-hidden='true'
                      className='border-meter-track size-24 shrink-0 rounded-full border-[0.6rem] sm:size-28'
                    />
                  ) : used !== null && (
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
                      {value}
                    </p>
                    <p className='text-caption text-muted-foreground wrap-anywhere'>
                      {item.account_label || t('Account identity unknown')}
                    </p>
                  </div>
                </div>
                <div className='flex min-w-0 flex-col items-start gap-2'>
                  <StatusBadge
                    variant={described.variant}
                    label={described.label}
                    copyable={false}
                    className='h-auto min-h-5 whitespace-normal'
                  />
                  <dl className='text-caption text-muted-foreground flex min-w-0 flex-col gap-1'>
                    {reset && (
                      <div>
                        <dt className='sr-only'>{t('Reset time')}</dt>
                        <dd>{reset}</dd>
                      </div>
                    )}
                    <div>
                      <dt className='sr-only'>{t('Observation time')}</dt>
                      <dd>
                        {Number.isSafeInteger(observed) && (observed ?? 0) > 0
                          ? t('Observed {{time}}', {
                              time: formatTimestampToDate(observed as number),
                            })
                          : t('No timestamp')}
                      </dd>
                    </div>
                    <div className='flex min-w-0 flex-wrap gap-x-1'>
                      <dt>{t('Source')}:</dt>
                      <dd className='min-w-0 font-mono wrap-anywhere'>
                        {item.source || t('Unknown')}
                      </dd>
                    </div>
                  </dl>
                  <ConnectionEvidenceQuality item={item} />
                  {(described.variant === 'warning' || described.variant === 'danger' || item.status === 'unsupported') && (
                    <p className='text-caption text-muted-foreground wrap-anywhere'>
                      {described.description}
                    </p>
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
