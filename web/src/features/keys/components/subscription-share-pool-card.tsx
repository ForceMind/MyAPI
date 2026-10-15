/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { RadioGroupItem } from '@/components/ui/radio-group'
import { formatTimestampToDate } from '@/lib/format'

import type { SubscriptionSharePool } from '../subscription-share-api'

const WINDOW_TOTAL_MICRO = 100_000_000

function formatBasisPoints(value: number): string {
  const whole = Math.floor(value / 100)
  const fraction = String(value % 100)
    .padStart(2, '0')
    .replace(/0+$/, '')
  return fraction ? `${whole}.${fraction}%` : `${whole}%`
}

function formatWindowMicro(value: number): string {
  const whole = Math.floor(value / 1_000_000)
  const fraction = String(value % 1_000_000)
    .padStart(6, '0')
    .replace(/0+$/, '')
  return fraction ? `${whole}.${fraction}%` : `${whole}%`
}

function getStateLabel(state: string, t: (key: string) => string): string {
  if (state === 'closed') return t('Closed — history retained')
  if (state === 'candidate') return t('Eligible window')
  if (state === 'active') return t('Active')
  if (state === 'stale') return t('Stale sample — requests are paused')
  if (state === 'baseline_pending') return t('Waiting for baseline sample')
  if (state === 'discontinuity') return t('Discontinuity — requests are paused')
  if (state === 'reset_pending') {
    return t('Waiting for reset sample — requests are paused')
  }
  return t('Unavailable — requests are paused')
}

export function SubscriptionSharePoolCard(props: {
  pool: SubscriptionSharePool
  selectable: boolean
  showAdminDetails: boolean
  closeDisabled?: boolean
  onClosePool?: () => void
}) {
  const { t } = useTranslation()
  const id = `subscription-share-pool-${props.pool.pool_id}-${props.pool.snapshot_id}`
  const stateLabel = getStateLabel(props.pool.state, t)
  const displayName = props.showAdminDetails
    ? props.pool.channel_name
    : `${t('Native window')} ${props.pool.pool_id}`
  let keyPauseLabel = ''
  if (props.pool.state === 'closed') {
    keyPauseLabel = ''
  } else if (props.pool.share_bps === 0) {
    keyPauseLabel = t('0% share — later requests are paused')
  } else if (
    props.pool.estimated_used_micro >=
    props.pool.share_bps * 10_000
  ) {
    keyPauseLabel = t(
      'Estimated share exhausted — later requests are paused'
    )
  }
  const content = (
    <div className='min-w-0 flex-1 space-y-3'>
      <div className='flex min-w-0 flex-wrap items-start justify-between gap-2'>
        <div className='min-w-0'>
          {props.selectable ? (
            <Label
              htmlFor={id}
              className='cursor-pointer font-medium break-words'
            >
              {displayName}
            </Label>
          ) : (
            <p className='font-medium break-words'>{displayName}</p>
          )}
          <p className='text-muted-foreground text-xs break-words'>
            {t('Plan')}: {props.pool.plan_type} ·{' '}
            {t('Native window')}: {props.pool.window_slot} ·{' '}
            {props.pool.window_type} ·{' '}
            {t('{{seconds}} seconds', {
              seconds: props.pool.window_seconds.toLocaleString(),
            })}
          </p>
        </div>
        <Badge
          variant={
            props.pool.state === 'active' || props.pool.state === 'candidate'
              ? 'secondary'
              : 'warning'
          }
          className='h-auto max-w-full whitespace-normal'
        >
          {stateLabel}
        </Badge>
      </div>

      {keyPauseLabel && (
        <p className='text-warning text-xs font-medium'>{keyPauseLabel}</p>
      )}

      {props.pool.state === 'closed' && (
        <p className='text-muted-foreground text-xs'>
          {t(
            'This pool is closed. Its usage and allocation history are retained.'
          )}
        </p>
      )}

      <dl className='grid min-w-0 grid-cols-1 gap-2 text-xs sm:grid-cols-2'>
        <div>
          <dt className='text-muted-foreground'>{t('Window total')}</dt>
          <dd className='font-medium'>
            {formatWindowMicro(WINDOW_TOTAL_MICRO)}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>{t('This Key share')}</dt>
          <dd className='font-medium'>
            {formatBasisPoints(props.pool.share_bps)}
          </dd>
        </div>
        {props.showAdminDetails && (
          <div>
            <dt className='text-muted-foreground'>{t('Total allocated')}</dt>
            <dd className='font-medium'>
              {formatBasisPoints(props.pool.allocated_bps)}
            </dd>
          </div>
        )}
        <div>
          <dt className='text-muted-foreground'>{t('Estimated used')}</dt>
          <dd className='font-medium'>
            {props.pool.pool_id > 0 && props.pool.baseline_time > 0
              ? formatWindowMicro(props.pool.estimated_used_micro)
              : t('Unknown')}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>{t('Unattributed increase')}</dt>
          <dd className='font-medium'>
            {props.pool.pool_id > 0 && props.pool.baseline_time > 0
              ? formatWindowMicro(props.pool.unattributed_micro)
              : t('Unknown')}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>
            {t('Attempts awaiting attribution')}
          </dt>
          <dd className='font-medium'>{props.pool.pending_attempts}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>{t('Unsampled attempts')}</dt>
          <dd className='font-medium'>{props.pool.unsampled_attempts}</dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>{t('Uncertain attempts')}</dt>
          <dd className='font-medium'>{props.pool.uncertain_attempts ?? t('Unknown')}</dd>
        </div>
      </dl>

      <div className='text-muted-foreground space-y-1 text-xs break-words'>
        <p>
          {t('Observed at')}: {formatTimestampToDate(props.pool.observed_at)}
        </p>
        <p>
          {t('Sample age')}: {t('{{seconds}} seconds', {
            seconds: props.pool.sample_age_seconds.toLocaleString(),
          })}{' '}
          · {t('Maximum age')}: {t('{{seconds}} seconds', {
            seconds: props.pool.max_age_seconds.toLocaleString(),
          })}
        </p>
        <p>
          {t('Resets at')}: {formatTimestampToDate(props.pool.reset_at)}
        </p>
        {props.pool.baseline_time > 0 && (
          <p>
            {t('Baseline recorded at')}:{' '}
            {formatTimestampToDate(props.pool.baseline_time)}
          </p>
        )}
        {props.pool.reason &&
          props.pool.state !== 'active' &&
          props.pool.state !== 'candidate' &&
          props.pool.state !== 'closed' && (
            <p>
              {t('Pause reason')}: <code>{props.pool.reason}</code>
            </p>
          )}
      </div>

      {props.pool.state === 'baseline_pending' && (
        <p className='text-warning text-xs'>
          {t(
            'A fresh sample after enabling is required before this managed pool can accept requests.'
          )}
        </p>
      )}
      {props.pool.state !== 'closed' &&
        (props.pool.pending_attempts > 0 ||
          props.pool.unsampled_attempts > 0) && (
          <p className='text-muted-foreground text-xs'>
            {t(
              'A successful zero-growth sample releases unsampled request slots, but it does not prove those requests used no subscription capacity; attribution remains pending.'
            )}
          </p>
        )}
      {props.onClosePool && (
        <Button
          type='button'
          variant='destructive'
          size='sm'
          disabled={props.closeDisabled}
          onClick={props.onClosePool}
        >
          {t('Close pool')}
        </Button>
      )}
    </div>
  )

  if (!props.selectable) {
    return (
      <article className='min-w-0 rounded-lg border p-3'>{content}</article>
    )
  }

  return (
    <div
      className='has-data-checked:border-primary has-data-checked:bg-primary/5 flex min-w-0 items-start gap-3 rounded-lg border p-3'
    >
      <RadioGroupItem
        id={id}
        value={`${props.pool.pool_id}:${props.pool.snapshot_id}`}
      />
      {content}
    </div>
  )
}
