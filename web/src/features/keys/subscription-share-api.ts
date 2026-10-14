/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { z } from 'zod'

import { api } from '@/lib/api'
import { createOperationId } from '@/lib/operation-id'

const safeCount = z.number().int().min(0).max(Number.MAX_SAFE_INTEGER)
const positiveSafeCount = z
  .number()
  .int()
  .positive()
  .max(Number.MAX_SAFE_INTEGER)

const subscriptionSharePoolSchema = z.object({
  pool_id: safeCount,
  snapshot_id: positiveSafeCount,
  revision: safeCount,
  channel_name: z.string(),
  plan_type: z.string().min(1),
  window_slot: z.string().min(1),
  window_type: z.string().min(1),
  window_seconds: positiveSafeCount,
  reset_at: positiveSafeCount,
  observed_at: positiveSafeCount,
  max_age_seconds: positiveSafeCount,
  sample_age_seconds: safeCount,
  state: z.string(),
  reason: z.string().optional(),
  allocated_bps: z.number().int().min(0).max(10000),
  share_bps: z.number().int().min(0).max(10000),
  estimated_used_micro: safeCount,
  unattributed_micro: safeCount,
  pending_attempts: safeCount,
  unsampled_attempts: safeCount,
  uncertain_attempts: safeCount.optional(),
  approximate: z.literal(true),
  baseline_time: safeCount,
  replacement_required: z.boolean().optional(),
  rebaseline_required: z.boolean().optional(),
})

const subscriptionShareViewSchema = z.object({
  token_id: z.number().int().positive(),
  pools: z.array(subscriptionSharePoolSchema),
})

export type SubscriptionSharePool = z.infer<
  typeof subscriptionSharePoolSchema
>
export type SubscriptionShareView = z.infer<
  typeof subscriptionShareViewSchema
>

export type SubscriptionShareCommand = {
  id: string
  pool_id: number
  snapshot_id: number
  expected_revision: number
  share_bps: number
  confirmed_approximate: true
  replace_window?: true
}

export type SubscriptionShareCloseCommand = {
  id: string
  expected_revision: number
  confirmed_close: true
}

export const createSubscriptionShareOperationId = createOperationId

function parseSubscriptionShare(
  payload: unknown,
  tokenId: number
): SubscriptionShareView {
  const view = z
    .object({ success: z.literal(true), data: subscriptionShareViewSchema })
    .parse(payload).data

  if (view.token_id !== tokenId) {
    throw new Error('Subscription share identity mismatch')
  }
  if (
    new Set(
      view.pools.map((pool) => `${pool.pool_id}:${pool.snapshot_id}`)
    ).size !== view.pools.length
  ) {
    throw new Error('Subscription share pool identity mismatch')
  }
  return view
}

export async function getSubscriptionShare(
  tokenId: number,
  signal?: AbortSignal
): Promise<SubscriptionShareView> {
  const response = await api.get(`/api/token/${tokenId}/subscription-share`, {
    signal,
    disableDuplicate: true,
    skipBusinessError: true,
    skipErrorHandler: true,
  })
  return parseSubscriptionShare(response.data, tokenId)
}

export async function writeSubscriptionShare(
  tokenId: number,
  command: SubscriptionShareCommand
): Promise<SubscriptionShareView> {
  const response = await api.put(
    `/api/token/${tokenId}/subscription-share`,
    command,
    {
      skipBusinessError: true,
      skipErrorHandler: true,
    }
  )
  return parseSubscriptionShare(response.data, tokenId)
}

export async function closeSubscriptionSharePool(
  poolId: number,
  command: SubscriptionShareCloseCommand
): Promise<void> {
  const response = await api.post(
    `/api/subscription-share/pools/${poolId}/close`,
    command,
    {
      skipBusinessError: true,
      skipErrorHandler: true,
    }
  )
  z.object({ success: z.literal(true) }).parse(response.data)
}
