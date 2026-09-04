/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { quotaSeriesKey } from '@/features/channels/lib/quota-history'
import type { ChannelQuotaChangeItem } from '@/features/channels/types'

import { QuotaOverviewCard } from './quota-overview-card'

/**
 * Compatibility wrapper for the dashboard quota section. It intentionally
 * renders server-provided overview data only and never starts per-series
 * history queries or exposes detailed chart controls.
 */
export function CodexAccountQuotaChart(props: {
  items: ChannelQuotaChangeItem[]
}) {
  if (props.items.length === 0) return null
  return (
    <div
      className='grid min-w-0 grid-cols-1 gap-3 lg:grid-cols-2'
      data-testid='codex-account-quota-chart'
    >
      {props.items.map((item) => (
        <QuotaOverviewCard key={quotaSeriesKey(item)} item={item} />
      ))}
    </div>
  )
}
