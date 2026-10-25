/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import z from 'zod'

import type { ChannelQuotaChangeItem } from '@/features/channels/types'
import { savedKeyIdSchema } from '@/features/keys/key-request-context'
import { nativeWindowUsage } from '@/features/personal-upstream/window-state'
import type { UsageLog } from '@/features/usage-logs/data/schema'
import { getRequestFacts } from '@/features/usage-logs/lib/request-facts'

/** A flow board column shows at most this many nodes before "View all". */
export const FLOW_NODE_LIMIT = 6
const HOUR_SECONDS = 60 * 60
const HOURS = 24

/**
 * `/dashboard/$section` search: the overview's selected flow node, so a
 * refresh, Back or a shared link restores the same filter. A Key wins when
 * both are present.
 */
export const overviewSearchSchema = z.object({
  key: savedKeyIdSchema.optional().catch(undefined),
  connection: savedKeyIdSchema.optional().catch(undefined),
})
export type OverviewSearch = z.infer<typeof overviewSearchSchema>

export type FlowSelection = { kind: 'key' | 'connection'; id: number }

export function selectionFromSearch(
  search: OverviewSearch
): FlowSelection | null {
  if (search.key !== undefined) return { kind: 'key', id: search.key }
  if (search.connection !== undefined) {
    return { kind: 'connection', id: search.connection }
  }
  return null
}

export type FlowTraffic = { requests: number; failures: number }

/**
 * Requests and failures per stable ID (Key ID or connection ID) among the
 * sampled call records. Rows without an ID are not attributed to any node.
 * "Failed" follows the overview's existing rule: every request that did not
 * complete, including ones rejected before sending.
 */
export function trafficBy(
  rows: UsageLog[],
  identity: (row: UsageLog) => number
): Map<number, FlowTraffic> {
  const traffic = new Map<number, FlowTraffic>()
  for (const row of rows) {
    const id = identity(row)
    if (!Number.isSafeInteger(id) || id <= 0) continue
    const entry = traffic.get(id) ?? { requests: 0, failures: 0 }
    entry.requests += 1
    if (!getRequestFacts(row).completed) entry.failures += 1
    traffic.set(id, entry)
  }
  return traffic
}

export type FlowKeyNode = {
  id: number
  name: string
  /** Saved Key status; null when the Key is not in the viewer's own list. */
  status: number | null
  /** The Key belongs to the signed-in user. */
  own: boolean
  /** Null when the call records could not be read. */
  requests: number | null
  failures: number | null
}

/**
 * The viewer's own Keys from the safe list, plus Keys that appear only in the
 * sampled call records (an administrator also sees other users' requests).
 * Busiest first; ties keep the Key ID order.
 */
export function buildKeyNodes(
  keys: { id: number; name: string; status: number }[],
  rows: UsageLog[] | undefined,
  viewerId: number | undefined
): FlowKeyNode[] {
  const traffic = rows ? trafficBy(rows, (row) => row.token_id) : undefined
  const nodes = new Map<number, FlowKeyNode>()
  for (const key of keys) {
    const counts = traffic?.get(key.id)
    nodes.set(key.id, {
      id: key.id,
      name: key.name,
      status: key.status,
      own: true,
      requests: traffic ? (counts?.requests ?? 0) : null,
      failures: traffic ? (counts?.failures ?? 0) : null,
    })
  }
  for (const row of rows ?? []) {
    const counts = traffic?.get(row.token_id)
    if (!counts || nodes.has(row.token_id)) continue
    // Rows arrive newest first, so the first one carries the latest name.
    nodes.set(row.token_id, {
      id: row.token_id,
      name: row.token_name,
      status: null,
      own: row.user_id === viewerId,
      requests: counts.requests,
      failures: counts.failures,
    })
  }
  return [...nodes.values()].sort(
    (a, b) => (b.requests ?? 0) - (a.requests ?? 0) || a.id - b.id
  )
}

/**
 * Folds a column to its first nodes. A selected node outside them takes the
 * last visible slot, so the active filter always has its node on screen.
 */
export function visibleNodes<T extends { id: number }>(
  nodes: T[],
  selectedId: number | undefined,
  limit: number = FLOW_NODE_LIMIT
): T[] {
  if (nodes.length <= limit) return nodes
  const shown = nodes.slice(0, limit)
  const selected = nodes.find((node) => node.id === selectedId)
  if (selected && !shown.includes(selected)) shown[limit - 1] = selected
  return shown
}

export type HourBucket = {
  /** Unix seconds at the start of the local hour. */
  start: number
  count: number
  /** False when the sample does not reach back into this hour. */
  covered: boolean
}

/**
 * Requests per local hour for the 24 hours ending with the current one.
 * Hours that end before `coverageStart` (the sample was truncated) are
 * reported as not covered rather than as zero.
 */
export function hourlyBuckets(
  rows: UsageLog[],
  nowSeconds: number,
  coverageStart: number
): HourBucket[] {
  const hour = new Date(nowSeconds * 1000)
  hour.setMinutes(0, 0, 0)
  const current = Math.floor(hour.getTime() / 1000)
  const buckets = Array.from({ length: HOURS }, (_, index) => {
    const start = current - (HOURS - 1 - index) * HOUR_SECONDS
    return { start, count: 0, covered: start + HOUR_SECONDS > coverageStart }
  })
  for (const row of rows) {
    const index = Math.floor((row.created_at - buckets[0].start) / HOUR_SECONDS)
    if (index >= 0 && index < HOURS) buckets[index].count += 1
  }
  return buckets
}

/** Median of positive durations; undefined when no row reports one. */
export function medianSeconds(values: number[]) {
  const known = values
    .filter((value) => Number.isFinite(value) && value > 0)
    .sort((a, b) => a - b)
  if (known.length === 0) return undefined
  const middle = Math.floor(known.length / 2)
  return known.length % 2
    ? known[middle]
    : (known[middle - 1] + known[middle]) / 2
}

/**
 * The most used saved account window per connection. Windows without a
 * trustworthy sample are kept (their state is shown) but never outrank a
 * measured one.
 */
export function tightestWindowByConnection(items: ChannelQuotaChangeItem[]) {
  const windows = new Map<number, ChannelQuotaChangeItem>()
  for (const item of items) {
    if (item.metric_type !== 'codex_rate_limit' || item.unit !== 'percent') {
      continue
    }
    const current = windows.get(item.channel_id)
    if (
      !current ||
      (nativeWindowUsage(item) ?? -1) > (nativeWindowUsage(current) ?? -1)
    ) {
      windows.set(item.channel_id, item)
    }
  }
  return windows
}
