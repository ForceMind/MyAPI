/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useTranslation } from 'react-i18next'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { toIntlLocale } from '@/i18n/languages'

import type { PoolUsage } from '../pool-usage-api'
import { WindowPoints } from './window-points'

export function PoolUsageSummary(props: { usage: PoolUsage }) {
  const { t, i18n } = useTranslation()
  const usage = props.usage
  const available = usage.decomposition_state === 'available'
  const chartFits =
    available && (usage.observed_used_micro ?? Infinity) <= 100000000
  let unavailableMessage = t('Usage breakdown unavailable')
  if (usage.decomposition_state === 'baseline_pending')
    unavailableMessage = t('Waiting for a trustworthy baseline')
  if (usage.decomposition_state === 'baseline_unknown')
    unavailableMessage = t(
      'The first observation for this cycle is unavailable.'
    )
  if (usage.decomposition_state === 'inconsistent')
    unavailableMessage = t(
      'The window evidence is inconsistent. Values are shown separately.'
    )
  const initialTime = usage.initial_baseline_time
  const referenceTime = usage.baseline_time
  return (
    <Card className='pc-card min-w-0' data-testid='pool-usage-summary'>
      <CardHeader>
        <CardTitle>
          <h2>{t('Window usage observation')}</h2>
        </CardTitle>
      </CardHeader>
      <CardContent className='flex min-w-0 flex-col gap-4 [overflow-wrap:anywhere]'>
        {!available ? (
          <p role='status' data-testid='pool-decomposition-unavailable'>
            {unavailableMessage}
          </p>
        ) : null}
        {chartFits ? (
          <div
            className='bg-muted flex h-3 w-full overflow-hidden rounded-full'
            aria-hidden='true'
            data-testid='pool-decomposition-bar'
          >
            <span
              className='bg-primary'
              style={{ width: `${usage.estimated_used_micro! / 1000000}%` }}
            />
            <span
              className='bg-primary/40'
              style={{
                width: `${usage.initial_baseline_used_micro! / 1000000}%`,
              }}
            />
            <span
              className='bg-warning'
              style={{
                width: `${usage.unattributed_growth_micro! / 1000000}%`,
              }}
            />
          </div>
        ) : null}
        <dl className='grid min-w-0 grid-cols-[minmax(0,1fr)_minmax(0,1fr)] items-start gap-x-3 gap-y-3 text-sm [&_dd]:text-right'>
          <dt>{t('Local Key estimates')}</dt>
          <dd data-testid='pool-estimated-total'>
            <WindowPoints value={usage.estimated_used_micro} />
          </dd>
          <dt>{t('First observation in this cycle')}</dt>
          <dd data-testid='pool-initial-baseline'>
            <WindowPoints value={usage.initial_baseline_used_micro} />
          </dd>
          <dt>{t('Unattributed growth')}</dt>
          <dd data-testid='pool-unattributed-growth'>
            <WindowPoints value={usage.unattributed_growth_micro} />
          </dd>
          <dt>{t('Unattributed usage (including initial baseline)')}</dt>
          <dd>
            <WindowPoints value={usage.unattributed_micro} />
          </dd>
          <dt>{t('Projected window usage')}</dt>
          <dd>
            <WindowPoints value={usage.observed_used_micro} />
          </dd>
          <dt>{t('Pending requests')}</dt>
          <dd>{usage.pending_attempts}</dd>
          <dt>{t('Unsampled requests')}</dt>
          <dd>{usage.unsampled_attempts}</dd>
          <dt>{t('Uncertain requests')}</dt>
          <dd>{usage.uncertain_attempts}</dd>
        </dl>
        <p className='text-muted-foreground text-xs leading-relaxed'>
          {t('Pending and unsampled request counts may overlap.')}
        </p>
        {initialTime && initialTime > 0 ? (
          <p className='text-muted-foreground text-xs'>
            {t('First observed {{time}}', {
              time: new Intl.DateTimeFormat(toIntlLocale(i18n.language), {
                dateStyle: 'short',
                timeStyle: 'short',
              }).format(new Date(initialTime * 1000)),
            })}
          </p>
        ) : null}
        <div className='bg-muted rounded-lg p-3 text-sm'>
          <p>{t('Current recovery reference')}</p>
          <p className='mt-1 font-medium' data-testid='pool-recovery-reference'>
            <WindowPoints value={usage.baseline_used_micro} />
          </p>
          {referenceTime > 0 ? (
            <p className='text-muted-foreground mt-1 text-xs'>
              {t('Observed {{time}}', {
                time: new Intl.DateTimeFormat(toIntlLocale(i18n.language), {
                  dateStyle: 'short',
                  timeStyle: 'short',
                }).format(new Date(referenceTime * 1000)),
              })}
            </p>
          ) : null}
        </div>
        <p className='text-muted-foreground text-xs leading-relaxed'>
          {t(
            'The first observation is not the activation time. A recovery reference may include already attributed usage.'
          )}
        </p>
        <p className='text-muted-foreground text-xs leading-relaxed'>
          {t(
            'These are approximate native-window observations, not exact per-Key attribution.'
          )}
        </p>
      </CardContent>
    </Card>
  )
}
