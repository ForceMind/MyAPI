/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { z } from 'zod'

import { api } from '@/lib/api'

const safeCount = z.number().int().min(0).max(Number.MAX_SAFE_INTEGER)
const positiveCount = safeCount.refine((value) => value > 0)
const timestamp = safeCount.max(8640000000000)
const nativeMicro = safeCount.max(100000000).nullable()
const allocationSchema = z.object({
  token_id: positiveCount,
  token_name: z.string().nullable(),
  token_state: z.enum([
    'enabled',
    'paused',
    'expired',
    'exhausted',
    'deleted',
    'missing',
    'unknown',
  ]),
  share_bps: z.number().int().min(0).max(10000),
  estimated_used_micro: nativeMicro,
  pending_attempts: safeCount,
  unsampled_attempts: safeCount,
  uncertain_attempts: safeCount,
})
const poolUsageSchema = z
  .object({
    pool_id: positiveCount,
    snapshot_id: positiveCount,
    revision: safeCount,
    channel_id: positiveCount,
    channel_name: z.string(),
    window_slot: z.enum(['primary', 'secondary']),
    window_type: z.string().min(1),
    window_seconds: positiveCount,
    plan_type: z.string().min(1),
    generation: safeCount,
    state: z.string().min(1),
    reason: z.string(),
    observed_at: timestamp,
    reset_at: timestamp,
    max_age_seconds: positiveCount,
    sample_age_seconds: safeCount,
    allocated_bps: z.number().int().min(0).max(10000),
    approximate: z.literal(true),
    baseline_time: timestamp,
    baseline_used_micro: nativeMicro,
    initial_baseline_time: timestamp.nullable(),
    initial_baseline_used_micro: nativeMicro,
    observed_used_micro: nativeMicro,
    estimated_used_micro: nativeMicro,
    unattributed_micro: nativeMicro,
    unattributed_growth_micro: nativeMicro,
    decomposition_state: z.enum([
      'available',
      'baseline_pending',
      'baseline_unknown',
      'inconsistent',
    ]),
    pending_attempts: safeCount,
    unsampled_attempts: safeCount,
    uncertain_attempts: safeCount,
    allocations: z.array(allocationSchema),
  })
  .superRefine((pool, ctx) => {
    const invalid = (message: string) =>
      ctx.addIssue({ code: 'custom', message })
    if (
      new Set(pool.allocations.map((item) => item.token_id)).size !==
      pool.allocations.length
    )
      invalid('Duplicate Key allocation')
    if (
      pool.allocations.reduce(
        (sum, item) => sum + BigInt(item.share_bps),
        0n
      ) !== BigInt(pool.allocated_bps)
    )
      invalid('Allocation total mismatch')
    for (const field of [
      'pending_attempts',
      'unsampled_attempts',
      'uncertain_attempts',
    ] as const) {
      const knownCount = pool.allocations.reduce(
        (sum, item) => sum + BigInt(item[field]),
        0n
      )
      if (
        knownCount > BigInt(pool[field]) ||
        (pool.decomposition_state === 'available' &&
          knownCount !== BigInt(pool[field]))
      )
        invalid('Request count mismatch')
    }
    if (pool.decomposition_state !== 'available') return
    if (
      pool.baseline_time <= 0 ||
      !pool.initial_baseline_time ||
      pool.baseline_used_micro === null ||
      pool.initial_baseline_used_micro === null ||
      pool.observed_used_micro === null ||
      pool.estimated_used_micro === null ||
      pool.unattributed_micro === null ||
      pool.unattributed_growth_micro === null ||
      pool.allocations.some((item) => item.estimated_used_micro === null)
    ) {
      invalid('Missing decomposition evidence')
      return
    }
    const estimated = pool.allocations.reduce(
      (sum, item) => sum + BigInt(item.estimated_used_micro!),
      0n
    )
    if (
      estimated !== BigInt(pool.estimated_used_micro) ||
      estimated + BigInt(pool.unattributed_micro) !==
        BigInt(pool.observed_used_micro) ||
      pool.initial_baseline_used_micro > pool.unattributed_micro ||
      pool.unattributed_growth_micro !==
        pool.unattributed_micro - pool.initial_baseline_used_micro
    )
      invalid('Usage decomposition mismatch')
  })

export type PoolUsage = z.infer<typeof poolUsageSchema>
export type PoolKeyAllocation = z.infer<typeof allocationSchema>

export async function getPoolUsage(
  poolId: number,
  channelId: number,
  signal?: AbortSignal
): Promise<PoolUsage> {
  positiveCount.parse(poolId)
  positiveCount.parse(channelId)
  const response = await api.get(
    `/api/subscription-share/pools/${poolId}/usage`,
    {
      signal,
      disableDuplicate: true,
      skipAuthRefresh: true,
      timeout: 15000,
      skipBusinessError: true,
      skipErrorHandler: true,
    }
  )
  const pool = z
    .object({ success: z.literal(true), data: poolUsageSchema })
    .parse(response.data).data
  if (pool.pool_id !== poolId || pool.channel_id !== channelId)
    throw new Error('Pool usage identity mismatch')
  return pool
}
