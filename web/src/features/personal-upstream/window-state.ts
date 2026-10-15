/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { ChannelQuotaChangeItem } from '@/features/channels/types'

export function nativeWindowUsage(item: ChannelQuotaChangeItem): number | null {
  if (item.unit !== 'percent' || item.metric_type !== 'codex_rate_limit' || item.status !== 'success') return null
  const remaining = item.current_available
  if (typeof remaining !== 'number' || !Number.isFinite(remaining) || remaining < 0 || remaining > 100) return null
  return 100 - remaining
}

export function nativeWindowFreshness(item: ChannelQuotaChangeItem, nowSeconds: number): 'fresh' | 'stale' | 'unknown' {
  if (!Number.isSafeInteger(item.observed_at) || (item.observed_at ?? 0) <= 0 || (item.observed_at ?? 0) > nowSeconds) return 'unknown'
  // Match the existing subscription-share default sampling validity. A pool can impose a stricter bound.
  return nowSeconds - (item.observed_at as number) > 300 ? 'stale' : 'fresh'
}
