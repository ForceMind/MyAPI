/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ChannelQuotaDetailChart } from '@/features/channels/components/channel-quota-detail-chart'
import type { QuotaCustomRange } from '@/features/channels/hooks/use-quota-history-time'
import {
  quotaSeriesKey,
  quotaWindowLabel,
} from '@/features/channels/lib/quota-history'
import type {
  ChannelQuotaChangeItem,
  ChannelQuotaHistoryRange,
} from '@/features/channels/types'

export function CodexAccountQuotaChart(props: {
  items: ChannelQuotaChangeItem[]
  range: ChannelQuotaHistoryRange
  onRangeChange: (range: ChannelQuotaHistoryRange) => void
  refreshEpoch?: number
  customRange?: QuotaCustomRange
  onCustomRangeChange?: (range: QuotaCustomRange) => void
}) {
  const { t } = useTranslation()
  const [selectedKey, setSelectedKey] = useState('')
  const series = props.items.filter(
    (item) => item.metric_type === 'codex_rate_limit'
  )
  const selected =
    series.find((item) => quotaSeriesKey(item) === selectedKey) ?? series[0]
  if (!selected) return null
  return (
    <div className='min-w-0 space-y-2' data-testid='codex-account-quota-chart'>
      {series.length > 1 ? (
        <label className='grid min-w-0 gap-1 text-xs sm:max-w-md'>
          <span className='text-muted-foreground'>{t('Account')}</span>
          <select
            aria-label={t('Codex account')}
            className='text-foreground h-8 min-w-0 rounded-lg border bg-transparent px-2 text-sm'
            value={quotaSeriesKey(selected)}
            onChange={(event) => setSelectedKey(event.target.value)}
          >
            {series.map((item) => (
              <option key={quotaSeriesKey(item)} value={quotaSeriesKey(item)}>
                {item.account_label || item.name} ·{' '}
                {quotaWindowLabel(item.window_type, t)}
                {item.plan_type ? ` · ${item.plan_type}` : ''}
              </option>
            ))}
          </select>
        </label>
      ) : null}
      <ChannelQuotaDetailChart
        item={selected}
        title={`Codex · ${t('Usage trend')}`}
        range={props.range}
        onRangeChange={props.onRangeChange}
        refreshEpoch={props.refreshEpoch}
        customRange={props.customRange}
        onCustomRangeChange={props.onCustomRangeChange}
      />
    </div>
  )
}
