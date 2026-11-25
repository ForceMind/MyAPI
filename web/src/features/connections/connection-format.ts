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
  if (item.metric_type === 'balance') return t('Balance query')
  return item.window_seconds && item.window_seconds > 0
    ? formatQuotaDuration(item.window_seconds, t)
    : quotaWindowLabel(item.window_type, t)
}

/** Show two whole units without rounding up across the reported reset. */
export function formatWindowCountdown(seconds: number, t: TFunction) {
  if (seconds < 60) return formatQuotaDuration(Math.ceil(seconds), t)
  const hour = 3600
  const day = 24 * hour
  const majorUnit = seconds >= day ? day : hour
  const minorUnit = majorUnit === day ? hour : 60
  const major = Math.floor(seconds / majorUnit) * majorUnit
  const minor = Math.floor((seconds % majorUnit) / minorUnit) * minorUnit
  if (major === 0) return formatQuotaDuration(minor, t)
  if (minor === 0) return formatQuotaDuration(major, t)
  return `${formatQuotaDuration(major, t)} ${formatQuotaDuration(minor, t)}`
}
