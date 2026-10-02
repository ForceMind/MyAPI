import { z } from 'zod'

import { api } from '@/lib/api'

const quota = z.number().int().min(0).max(2147483647)
const reviewSchema = z.object({
  request_id: z.string(),
  user_id: z.number().int(),
  token_id: z.number().int(),
  state: z.string(),
  reserved_quota: quota,
  actual_quota: quota.nullable(),
  decision: z
    .object({
      id: z.number().int(),
      actual_quota: quota,
      evidence_reference: z.string(),
    })
    .optional(),
})
export type UsageReview = z.infer<typeof reviewSchema>

function parseReview(payload: unknown, requestId: string): UsageReview {
  const result = z
    .object({ success: z.literal(true), data: reviewSchema })
    .parse(payload)
  if (result.data.request_id !== requestId) {
    throw new Error('Usage review identity mismatch')
  }
  return result.data
}

export async function getUsageReview(
  requestId: string,
  signal?: AbortSignal
): Promise<UsageReview> {
  const response = await api.get(
    `/api/usage-review/${encodeURIComponent(requestId)}`,
    { signal, skipErrorHandler: true }
  )
  return parseReview(response.data, requestId)
}

export async function reconcileUsageReview(
  requestId: string,
  actualQuota: number,
  evidence: string
): Promise<UsageReview> {
  const response = await api.post(
    `/api/usage-review/${encodeURIComponent(requestId)}/reconcile`,
    {
      actual_quota: actualQuota,
      evidence_reference: evidence,
      confirmed_reliable_evidence: true,
    },
    { skipErrorHandler: true }
  )
  return parseReview(response.data, requestId)
}
