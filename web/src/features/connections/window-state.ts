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
  nativeWindowState,
} from '@/features/personal-upstream/window-state'

/** Classify saved evidence without merging series or claiming an error resolved. */
export function describeConnectionWindow(
  item: ChannelQuotaChangeItem,
  windows: ChannelQuotaChangeItem[],
  nowSeconds: number,
  t: (key: string) => string
): ReturnType<typeof describeNativeWindowState> {
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
    const historical =
      Number.isSafeInteger(item.observed_at) &&
      (item.observed_at ?? 0) > 0 &&
      (item.observed_at ?? 0) <= nowSeconds &&
      windows.some(
        (other) =>
          other.channel_id === item.channel_id &&
          nativeWindowState(other, nowSeconds) === 'available' &&
          other.source?.startsWith('codex_wham_usage_') &&
          Number.isSafeInteger(other.observed_at) &&
          (other.observed_at ?? 0) > (item.observed_at ?? 0) &&
          (other.observed_at ?? 0) <= nowSeconds
      )
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
