/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { ChannelQuotaChangeItem } from '@/features/channels/types'
import {
  describeNativeWindowState,
  nativeWindowFreshness,
  nativeWindowState,
} from '@/features/personal-upstream/window-state'

/** A provider balance can be negative; finite saved amounts preserve debt. */
export function savedBalanceAmount(item: ChannelQuotaChangeItem): number | null {
  if (item.metric_type !== 'balance' || item.status !== 'success' ||
    typeof item.current_available !== 'number' || !Number.isFinite(item.current_available)) return null
  return item.current_available
}

/** A later connection observation does not prove this older identity recovered. */
export function isHistoricalConnectionWindow(
  item: ChannelQuotaChangeItem,
  windows: ChannelQuotaChangeItem[],
  nowSeconds: number
): boolean {
  return item.metric_type === 'codex_rate_limit' && item.status === 'error' && item.source !== 'codex_relay_usage_limit' &&
    Number.isSafeInteger(item.observed_at) && (item.observed_at ?? 0) > 0 &&
    (item.observed_at ?? 0) <= nowSeconds && windows.some((other) =>
      other.channel_id === item.channel_id &&
      nativeWindowState(other, nowSeconds) === 'available' &&
      other.source?.startsWith('codex_wham_usage_') &&
      Number.isSafeInteger(other.observed_at) &&
      (other.observed_at ?? 0) > (item.observed_at ?? 0) &&
      (other.observed_at ?? 0) <= nowSeconds
    )
}

/** Classify saved evidence without merging series or claiming an error resolved. */
export function describeConnectionWindow(
  item: ChannelQuotaChangeItem,
  windows: ChannelQuotaChangeItem[],
  nowSeconds: number,
  t: (key: string) => string
): ReturnType<typeof describeNativeWindowState> {
  // Balance-route capability belongs to this saved balance series only.
  // A failed balance query never establishes native-window capability.
  if (item.metric_type === 'balance') {
    if (item.status === 'unsupported') {
      if (item.error_code === 'quota_not_configured') {
        return {
          label: t('Balance query not configured'),
          description: t('No balance query route was configured for this saved result. Account-window support is separate.'),
          variant: 'neutral',
        }
      }
      return {
        label: t('Balance query unsupported'),
        description: t('This saved result reports no balance query support for this series. It does not determine account-window support.'),
        variant: 'neutral',
      }
    }
    if (item.status === 'error') {
      return {
        label: t('Balance sample failed'),
        description: t('This saved balance observation failed. Its timestamp applies only to this series.'),
        variant: 'danger',
      }
    }
    if (savedBalanceAmount(item) !== null) {
      const freshness = nativeWindowFreshness(item, nowSeconds)
      if (freshness === 'fresh') return describeNativeWindowState('available', t)
      return describeNativeWindowState(freshness === 'stale' ? 'stale' : 'unverified', t)
    }
  }
  if (item.source === 'codex_relay_usage_limit') {
    return {
      label: t('Relay usage-limit event'),
      description: t(
        'A model request reported a usage limit. This event is not a WHAM sampling failure.'
      ),
      variant: 'warning',
    }
  }
  if (item.status === 'error') {
    const historical = isHistoricalConnectionWindow(item, windows, nowSeconds)
    if (historical) {
      return {
        label: t('Historical saved sample error'),
        description: t(
          'A newer WHAM observation exists for this connection. This saved error may belong to a different identity and is not marked resolved.'
        ),
        variant: 'warning',
      }
    }
    if (
      item.source === 'codex_wham_usage' ||
      item.source?.startsWith('codex_wham_usage_')
    ) {
      return {
        label: t('WHAM usage sample failed'),
        description: t(
          'This saved WHAM observation failed. It does not describe other window series.'
        ),
        variant: 'danger',
      }
    }
    return {
      label: t('Usage sample failed'),
      description: t(
        'This saved observation failed. Its timestamp applies only to this series.'
      ),
      variant: 'danger',
    }
  }
  return describeNativeWindowState(nativeWindowState(item, nowSeconds), t)
}
