import { z } from 'zod'

import { api } from '@/lib/api'
import { usdAmountSchema } from '@/lib/exact-usd'

const quota = z.number().int().min(0).max(2147483647)
export const usageReviewSchema = z.object({
  review_metadata: z.string().optional(),
  text_dispatch_pending: z.boolean().optional(),
  can_recover_text_dispatch: z.boolean().optional(),
  request_id: z.string(),
  user_id: z.number().int(),
  token_id: z.number().int(),
  state: z.string(),
  reserved_quota: quota,
  actual_quota: quota.nullable(),
  token_budget: z
    .object({
      request_id: z.string(),
      token_id: z.number().int(),
      user_id: z.number().int(),
      state: z.string(),
      actual_input: quota.nullable(),
      actual_output: quota.nullable(),
      fee_enabled: z.boolean(),
      actual_fee_usd: usdAmountSchema.nullish(),
    })
    .optional(),
  decision: z
    .object({
      id: z.number().int(),
      actual_quota: quota,
      evidence_reference: z.string(),
      actual_input_tokens: quota.optional(),
      actual_output_tokens: quota.optional(),
      actual_fee_usd: usdAmountSchema.optional(),
    })
    .optional(),
})
export type UsageReview = z.infer<typeof usageReviewSchema>

function parseReview(payload: unknown, requestId: string): UsageReview {
  const result = z
    .object({ success: z.literal(true), data: usageReviewSchema })
    .parse(payload)
  if (result.data.request_id !== requestId) {
    throw new Error('Usage review identity mismatch')
  }
  const budget = result.data.token_budget
  if (
    budget &&
    (budget.request_id !== requestId ||
      budget.token_id !== result.data.token_id ||
      budget.user_id !== result.data.user_id)
  ) {
    throw new Error('Token budget review identity mismatch')
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
  evidence: string,
  tokens?: { input: number; output: number; feeUSD?: string }
): Promise<UsageReview> {
  const response = await api.post(
    `/api/usage-review/${encodeURIComponent(requestId)}/reconcile`,
    {
      actual_quota: actualQuota,
      evidence_reference: evidence,
      confirmed_reliable_evidence: true,
      ...(tokens
        ? {
            actual_input_tokens: tokens.input,
            actual_output_tokens: tokens.output,
            ...(tokens.feeUSD !== undefined
              ? { actual_fee_usd: tokens.feeUSD }
              : {}),
          }
        : {}),
    },
    { skipErrorHandler: true }
  )
  return parseReview(response.data, requestId)
}

export async function recoverTextDispatchUsage(
  requestId: string,
  actualQuota: number,
  evidence: string
): Promise<UsageReview> {
  const response = await api.post(
    `/api/usage-review/${encodeURIComponent(requestId)}/recover-dispatch`,
    {
      actual_quota: actualQuota,
      evidence_reference: evidence,
      confirmed_reliable_evidence: true,
      confirmed_request_finished: true,
    },
    { skipErrorHandler: true }
  )
  return parseReview(response.data, requestId)
}

export async function getPendingUsageReviews(
  writer: 'authoritative' | 'legacy',
  after: string,
  signal?: AbortSignal
) {
  const response = await api.get('/api/usage-reviews/pending', {
    params: { writer, after },
    signal,
    skipErrorHandler: true,
  })
  return z
    .object({
      success: z.literal(true),
      data: z.object({
        items: z.array(usageReviewSchema),
        next_after: z.string().regex(/^\d*$/),
      }),
    })
    .parse(response.data).data
}
