import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { formatQuotaAmount } from '@/features/channels/lib/quota-history'
import type { ChannelQuotaChangeItem } from '@/features/channels/types'
import { toIntlLocale } from '@/i18n/languages'

export function AccountWindowList(props: { items: ChannelQuotaChangeItem[] }) {
  const { i18n, t } = useTranslation()
  const locale = toIntlLocale(i18n.language)
  const dateFormatter = useMemo(
    () =>
      new Intl.DateTimeFormat(locale, {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      }),
    [locale]
  )

  return (
    <ul className='space-y-2'>
      {props.items.map((item) => {
        const availableConfirmed =
          item.status === 'success' &&
          typeof item.current_available === 'number' &&
          Number.isFinite(item.current_available)
        let statusLabel = t('Unknown')
        let statusVariant: 'success' | 'warning' | 'destructive' | 'neutral' =
          'neutral'
        if (availableConfirmed) {
          statusLabel = t('Snapshot available')
          statusVariant = 'success'
        } else if (item.status === 'unsupported') {
          statusLabel = t('Unsupported')
          statusVariant = 'warning'
        } else if (item.status === 'error') {
          statusLabel = t('Error')
          statusVariant = 'destructive'
        } else if (item.status === 'unavailable' || item.status === 'success') {
          statusLabel = t('Snapshot unavailable')
          statusVariant = 'warning'
        }
        const observed = Number.isFinite(item.observed_at)
          ? new Date((item.observed_at as number) * 1000)
          : null
        const observedValid = observed && Number.isFinite(observed.getTime())
        const available = availableConfirmed
          ? formatQuotaAmount(
              item.current_available,
              item,
              'available',
              t,
              locale
            )
          : null
        return (
          <li
            key={`${item.channel_id}:${item.series_id ?? item.name}:${item.window_type ?? ''}`}
            className='min-w-0 rounded-lg border px-3 py-2.5'
          >
            <div className='flex min-w-0 items-start justify-between gap-3'>
              <div className='min-w-0'>
                <p className='truncate font-medium'>
                  {item.account_label?.trim() || item.name || t('Unknown')}
                </p>
                <p className='text-muted-foreground mt-0.5 truncate text-xs'>
                  {item.window_type || item.metric_type || t('Unknown')}
                </p>
              </div>
              <StatusBadge variant={statusVariant} copyable={false}>
                {statusLabel}
              </StatusBadge>
            </div>
            <div className='mt-2 flex min-w-0 flex-wrap items-baseline justify-between gap-2 border-t pt-2 text-xs'>
              <span className='text-muted-foreground'>
                {t('Current available')}
              </span>
              <span className='font-medium tabular-nums'>
                {available ?? t('Snapshot unavailable')}
              </span>
            </div>
            <p className='text-muted-foreground mt-1 text-xs'>
              {observedValid
                ? t('Observed {{time}}', {
                    time: dateFormatter.format(observed),
                  })
                : t('No timestamp')}
            </p>
          </li>
        )
      })}
    </ul>
  )
}
