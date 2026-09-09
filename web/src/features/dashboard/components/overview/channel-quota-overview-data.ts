/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { quotaComparisonGroup } from '@/features/channels/lib/quota-comparison'
import { quotaSeriesKey } from '@/features/channels/lib/quota-history'
import type { ChannelQuotaChangeItem } from '@/features/channels/types'

export function selectComparableQuotaSeries(items: ChannelQuotaChangeItem[]) {
  const sorted = [...items].sort(
    (left, right) =>
      left.channel_id - right.channel_id ||
      quotaSeriesKey(left).localeCompare(quotaSeriesKey(right))
  )
  if (sorted.length === 0) return []
  const weekly = sorted.find(
    (item) => item.window_type === 'weekly' || item.window_seconds === 604800
  )
  const group = quotaComparisonGroup(weekly ?? sorted[0])
  return sorted
    .filter((item) => quotaComparisonGroup(item) === group)
    .slice(0, 4)
}
