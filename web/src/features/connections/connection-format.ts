/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { TFunction } from 'i18next'

import {
  formatQuotaDuration,
  quotaWindowLabel,
} from '@/features/channels/lib/quota-history'
import type { ChannelQuotaChangeItem } from '@/features/channels/types'

/** A saved test's response time, as the connection test reports it. */
export function formatTestDuration(milliseconds: number) {
  return milliseconds >= 1000
    ? `${(milliseconds / 1000).toFixed(2)} s`
    : `${Math.max(1, Math.round(milliseconds))} ms`
}

/** The account window's name: its length when known, else its type. */
export function windowName(item: ChannelQuotaChangeItem, t: TFunction) {
  return item.window_seconds && item.window_seconds > 0
    ? formatQuotaDuration(item.window_seconds, t)
    : quotaWindowLabel(item.window_type, t)
}
