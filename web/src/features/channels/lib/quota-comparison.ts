/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import type { ChannelQuotaChangeItem } from '../types'

export interface QuotaTimeBounds {
  start: number
  end: number
}
export const quotaMaximumSpan = 180 * 86400

export function quotaComparisonGroup(item: ChannelQuotaChangeItem): string {
  return JSON.stringify([
    item.unit ?? '',
    item.currency ?? '',
    item.window_type ?? '',
    item.window_seconds ?? 0,
  ])
}

/** Zoom around the pointer without inventing samples or changing the anchor. */
export function zoomQuotaTime(
  bounds: QuotaTimeBounds,
  factor: number,
  anchor: number,
  now: number
): QuotaTimeBounds {
  const span = Math.max(
    60,
    Math.min(quotaMaximumSpan, Math.round((bounds.end - bounds.start) * factor))
  )
  const ratio = Math.max(0, Math.min(1, anchor))
  const pivot = bounds.start + (bounds.end - bounds.start) * ratio
  let start = Math.round(pivot - span * ratio)
  let end = start + span
  if (end > now) {
    end = now
    start = end - span
  }
  return { start, end }
}

/** Use a real reset anchor; this is not a calendar week or a rolling seven days. */
export function weeklyQuotaCycle(
  resetAt: number,
  now: number,
  offset = 0
): QuotaTimeBounds | null {
  if (
    !Number.isFinite(resetAt) ||
    resetAt <= 0 ||
    !Number.isInteger(offset) ||
    offset > 0
  ) {
    return null
  }
  const week = 7 * 86400
  // A reset exactly at now begins the next cycle.
  const end = resetAt + (Math.floor((now - resetAt) / week) + 1 + offset) * week
  if (end - week >= now) return null
  return { start: end - week, end: Math.min(end, now) }
}

export function nearestQuotaPoint<T extends { timestamp: number }>(
  points: readonly T[],
  time: number
): T | undefined {
  if (!points.length) return undefined
  let low = 0
  let high = points.length
  while (low < high) {
    const mid = (low + high) >>> 1
    if (points[mid].timestamp < time) low = mid + 1
    else high = mid
  }
  if (low === 0) return points[0]
  if (low === points.length) return points[low - 1]
  return time - points[low - 1].timestamp <= points[low].timestamp - time
    ? points[low - 1]
    : points[low]
}
