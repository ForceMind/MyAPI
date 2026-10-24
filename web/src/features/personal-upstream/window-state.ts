/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { ChannelQuotaChangeItem } from '@/features/channels/types'

export function nativeWindowUsage(item: ChannelQuotaChangeItem): number | null {
  if (
    item.unit !== 'percent' ||
    item.metric_type !== 'codex_rate_limit' ||
    item.status !== 'success'
  ) {
    return null
  }
  const remaining = item.current_available
  if (
    typeof remaining !== 'number' ||
    !Number.isFinite(remaining) ||
    remaining < 0 ||
    remaining > 100
  ) {
    return null
  }
  return 100 - remaining
}

export function nativeWindowFreshness(
  item: ChannelQuotaChangeItem,
  nowSeconds: number
): 'fresh' | 'stale' | 'unknown' {
  if (
    !Number.isSafeInteger(item.observed_at) ||
    (item.observed_at ?? 0) <= 0 ||
    (item.observed_at ?? 0) > nowSeconds
  ) {
    return 'unknown'
  }
  // Match the existing subscription-share default sampling validity. A pool can impose a stricter bound.
  return nowSeconds - (item.observed_at as number) > 300 ? 'stale' : 'fresh'
}

export function nativeWindowState(
  item: ChannelQuotaChangeItem,
  nowSeconds: number
):
  | 'available'
  | 'stale'
  | 'unverified'
  | 'error'
  | 'unsupported'
  | 'unavailable' {
  if (item.status === 'error') return 'error'
  if (item.status === 'unsupported') return 'unsupported'
  if (nativeWindowUsage(item) === null) return 'unavailable'
  const freshness = nativeWindowFreshness(item, nowSeconds)
  if (freshness === 'fresh') return 'available'
  return freshness === 'stale' ? 'stale' : 'unverified'
}

/** Status text of a saved window observation; never claims availability. */
export function describeNativeWindowState(
  state: ReturnType<typeof nativeWindowState>,
  t: (key: string) => string
): {
  label: string
  description: string
  variant: 'neutral' | 'success' | 'warning' | 'danger'
} {
  switch (state) {
    case 'error':
      return {
        label: t('Usage sample failed'),
        description: t(
          'The latest usage sample failed. Inspect the channel history and connection.'
        ),
        variant: 'danger',
      }
    case 'unsupported':
      return {
        label: t('Window reporting unsupported'),
        description: t('This provider does not report this window.'),
        variant: 'neutral',
      }
    case 'stale':
      return {
        label: t('Stale sample'),
        description: t(
          'This is a past observation, not a current availability check.'
        ),
        variant: 'warning',
      }
    case 'unverified':
      return {
        label: t('Sample time unverified'),
        description: t(
          'The observation time is missing or invalid. This value is not confirmed current.'
        ),
        variant: 'warning',
      }
    case 'available':
      return {
        label: t('Snapshot available'),
        description: t('Window resets require a trustworthy upstream sample.'),
        variant: 'success',
      }
    default:
      return {
        label: t('No reliable usage sample'),
        description: t(
          'No trustworthy usage observation is available for this window.'
        ),
        variant: 'neutral',
      }
  }
}
