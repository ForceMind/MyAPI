/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import {
  formatQuotaDuration,
  quotaWindowLabel,
} from '@/features/channels/lib/quota-history'
import type { ChannelQuotaChangeItem } from '@/features/channels/types'
import { toIntlLocale } from '@/i18n/languages'

import {
  describeNativeWindowState,
  nativeWindowState,
  nativeWindowUsage,
} from '../window-state'

export function WindowCard(props: {
  item: ChannelQuotaChangeItem
  compact?: boolean
  showHistory?: boolean
}) {
  const { t, i18n } = useTranslation()
  const used = nativeWindowUsage(props.item)
  const state = nativeWindowState(props.item, Date.now() / 1000)
  const observed = props.item.observed_at
  const observedValid = Number.isSafeInteger(observed) && (observed ?? 0) > 0
  const label =
    props.item.window_seconds && props.item.window_seconds > 0
      ? formatQuotaDuration(props.item.window_seconds, t)
      : quotaWindowLabel(props.item.window_type, t)
  let slotLabel = ''
  if (props.item.source === 'codex_wham_usage_primary') {
    slotLabel = t('Primary window')
  } else if (props.item.source === 'codex_wham_usage_secondary') {
    slotLabel = t('Secondary window')
  }
  const {
    label: statusLabel,
    description,
    variant: status,
  } = describeNativeWindowState(state, t)
  return (
    <Card className='pc-card min-w-0' data-testid='native-window-card'>
      <CardHeader>
        <div className='flex min-w-0 flex-wrap items-center justify-between gap-3'>
          <CardTitle className='min-w-0 break-words'>
            <h2>
              {props.compact
                ? props.item.account_label || props.item.name
                : label}
            </h2>
          </CardTitle>
          <StatusBadge
            className='h-auto min-h-5 py-0.5 text-left whitespace-normal'
            variant={status}
            copyable={false}
          >
            {statusLabel}
          </StatusBadge>
        </div>
        {props.compact ? (
          <p className='text-muted-foreground'>
            {label}
            {slotLabel ? ` · ${slotLabel}` : ''}
          </p>
        ) : null}
        {!props.compact && slotLabel ? (
          <p className='text-muted-foreground text-xs'>{slotLabel}</p>
        ) : null}
      </CardHeader>
      <CardContent className='flex flex-col gap-3'>
        {used === null ? (
          <p className='text-muted-foreground text-base font-medium'>
            {t('Usage unavailable')}
          </p>
        ) : (
          <div className='flex flex-wrap items-baseline gap-x-3 gap-y-1'>
            <p className='text-foreground text-3xl font-semibold tracking-tight tabular-nums'>
              {`${new Intl.NumberFormat(toIntlLocale(i18n.language), { maximumFractionDigits: 2 }).format(used)}%`}
            </p>
            <p className='text-muted-foreground text-sm'>
              {state === 'available'
                ? t('Account window used')
                : t('Last reported usage')}
            </p>
          </div>
        )}
        {used !== null && state === 'available' ? (
          <Progress
            value={used}
            aria-label={t('Account window used')}
            className='[&_[data-slot=progress-track]]:h-2.5'
          />
        ) : null}
        <p className='text-muted-foreground text-xs'>
          {observedValid
            ? t('Observed {{time}}', {
                time: new Intl.DateTimeFormat(toIntlLocale(i18n.language), {
                  month: 'short',
                  day: 'numeric',
                  hour: '2-digit',
                  minute: '2-digit',
                }).format(new Date((observed as number) * 1000)),
              })
            : t('No timestamp')}
        </p>
        <p className='text-muted-foreground text-xs leading-relaxed'>
          {description}
        </p>
        {props.showHistory ? (
          <Link
            to='/channels'
            search={{ quotaChannelId: props.item.channel_id }}
            className='text-primary self-start text-xs font-medium underline-offset-4 hover:underline focus-visible:underline'
          >
            {t('View channel history')}
          </Link>
        ) : null}
      </CardContent>
    </Card>
  )
}
