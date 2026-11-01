/* Copyright (C) 2026 ForceMind. AGPL-3.0-or-later. */
import z from 'zod'

import type { ChannelQuotaChangeItem } from '@/features/channels/types'

import { nativeWindowState } from './window-state'

const id = z.number().int().positive().max(2147483647).optional().catch(undefined)
export const upstreamViewSchema = z.object({
  channelId: id,
  keyId: id,
  filter: z.string().trim().max(200).optional().catch(undefined),
  period: z.string().max(120).optional().catch(undefined),
  status: z.enum(['available', 'stale', 'unverified', 'error', 'unsupported', 'unavailable']).optional().catch(undefined),
  changes: z.boolean().optional().catch(undefined),
})
export type UpstreamViewState = z.infer<typeof upstreamViewSchema>
export const upstreamSearchSchema = upstreamViewSchema.extend({ focusChannelId: id })

/** A filter key is a recorded duration, or the provider's recorded window type. */
export function upstreamWindowPeriod(item: ChannelQuotaChangeItem): string {
  if (Number.isSafeInteger(item.window_seconds) && (item.window_seconds ?? 0) > 0) return `duration:${item.window_seconds}`
  return `type:${item.window_type ?? ''}`
}

export function filterUpstreamWindows(items: ChannelQuotaChangeItem[], view: UpstreamViewState, now: number): ChannelQuotaChangeItem[] {
  const text = view.filter?.toLowerCase()
  return items.filter((item) => {
    if (text && !`${item.channel_id} ${item.name} ${item.account_label ?? ''}`.toLowerCase().includes(text)) return false
    if (view.period && upstreamWindowPeriod(item) !== view.period) return false
    return !view.status || nativeWindowState(item, now) === view.status
  })
}
