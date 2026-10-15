/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { formatQuotaDuration, quotaWindowLabel } from '@/features/channels/lib/quota-history'
import type { ChannelQuotaChangeItem } from '@/features/channels/types'
import { toIntlLocale } from '@/i18n/languages'

import { nativeWindowFreshness, nativeWindowUsage } from '../window-state'

export function WindowCard(props: { item: ChannelQuotaChangeItem; compact?: boolean }) {
  const { t, i18n } = useTranslation()
  const used = nativeWindowUsage(props.item)
  const freshness = nativeWindowFreshness(props.item, Date.now() / 1000)
  const observed = props.item.observed_at
  const observedValid = Number.isSafeInteger(observed) && (observed ?? 0) > 0
  const label = props.item.window_seconds && props.item.window_seconds > 0
    ? formatQuotaDuration(props.item.window_seconds, t)
    : quotaWindowLabel(props.item.window_type, t)
  let statusLabel = t('Snapshot unavailable')
  let status: 'neutral' | 'success' | 'warning' | 'danger' = 'neutral'
  if (props.item.status === 'error') { statusLabel = t('Error'); status = 'danger' }
  else if (freshness === 'stale') { statusLabel = t('Stale sample'); status = 'warning' }
  else if (used !== null && freshness === 'fresh') { statusLabel = t('Snapshot available'); status = 'success' }
  return (
    <Card className='pc-card min-w-0' data-testid='native-window-card'>
      <CardHeader>
        <div className='flex min-w-0 flex-wrap items-center justify-between gap-3'>
          <CardTitle className='min-w-0 break-words'><h2>{props.compact ? props.item.account_label || props.item.name : label}</h2></CardTitle>
          <StatusBadge variant={status} copyable={false}>{statusLabel}</StatusBadge>
        </div>
        {props.compact ? <p className='text-muted-foreground'>{label}</p> : null}
      </CardHeader>
      <CardContent className='flex flex-col gap-3'>
        <p className='text-foreground text-4xl font-semibold tracking-tight tabular-nums sm:text-5xl'>{used === null ? t('Unknown') : `${new Intl.NumberFormat(toIntlLocale(i18n.language), { maximumFractionDigits: 2 }).format(used)}%`}</p>
        <p className='text-muted-foreground'>{t('Account window used')}</p>
        {used !== null ? <Progress value={used} aria-label={t('Account window used')} className='[&_[data-slot=progress-track]]:h-2.5' /> : null}
        <p className='text-muted-foreground text-xs'>{observedValid ? t('Observed {{time}}', { time: new Intl.DateTimeFormat(toIntlLocale(i18n.language), { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date((observed as number) * 1000)) }) : t('No timestamp')}</p>
        <p className='bg-muted rounded-lg p-3 text-xs leading-relaxed'>{t('Window resets require a trustworthy upstream sample.')}</p>
      </CardContent>
    </Card>
  )
}
