import { z } from 'zod'

import { usageReviewSchema } from '@/features/usage-logs/usage-review-api'
import { api } from '@/lib/api'
import { usdAmountSchema } from '@/lib/exact-usd'

const count = z.number().int().min(0).max(Number.MAX_SAFE_INTEGER)
const budgetViewSchema = z.object({
  policy: z.object({
    token_id: z.number().int().positive(),
    user_id: z.number().int().positive(),
    enabled: z.boolean(),
    account_threshold_enabled: z.boolean(),
    account_min_remaining_bps: z.number().int().min(0).max(10000),
    account_max_age_seconds: z.number().int().min(30).max(3600),
    fee_enabled: z.boolean(),
    fee_limit_usd: usdAmountSchema,
    fee_used_usd: usdAmountSchema,
    fee_reserved_usd: usdAmountSchema,
    limit: count,
    used: count,
    reserved: count,
    pending_request_id: z.string(),
    revision: count,
  }),
  pending: z
    .object({
      request_id: z.string(),
      token_id: z.number().int().positive(),
      user_id: z.number().int().positive(),
      state: z.enum(['prepared', 'sent', 'usage_unknown']),
      reserved: count,
      model_name: z.string(),
      fee_enabled: z.boolean(),
      fee_reserved_usd: usdAmountSchema,
      actual_fee_usd: usdAmountSchema.nullish(),
    })
    .nullable(),
  review: usageReviewSchema.optional(),
})
export type TokenBudgetView = z.infer<typeof budgetViewSchema>
export type TokenBudgetCommand =
  | {
      kind: 'policy'
      body: {
        id: string
        expected_revision: number
        enabled: boolean
        limit: number
        confirmed: true
        fee: { enabled: boolean; limit_usd: string }
        account_threshold: {
          enabled: boolean
          minimum_remaining_bps: number
          max_age_seconds: number
        }
      }
    }
  | {
      kind: 'recover'
      body: {
        request_id: string
        action: 'cancel_not_sent' | 'reconcile'
        evidence_reference: string
        confirmed_reliable_evidence: true
        actual_quota?: number
        actual_input_tokens?: number
        actual_output_tokens?: number
        actual_fee_usd?: string
      }
    }

export function createTokenBudgetOperationId(): string {
  return Array.from(crypto.getRandomValues(new Uint8Array(32)), (value) =>
    value.toString(16).padStart(2, '0')
  ).join('')
}

function parseBudget(payload: unknown, tokenId: number): TokenBudgetView {
  const view = z
    .object({ success: z.literal(true), data: budgetViewSchema })
    .parse(payload).data
  if (
    view.policy.token_id !== tokenId ||
    !!view.pending !== !!view.policy.pending_request_id
  ) {
    throw new Error('Token budget identity mismatch')
  }
  if (
    view.pending &&
    (view.pending.request_id !== view.policy.pending_request_id ||
      view.pending.token_id !== tokenId ||
      view.pending.user_id !== view.policy.user_id ||
      view.pending.reserved !== view.policy.reserved ||
      view.pending.fee_enabled !== view.policy.fee_enabled ||
      view.pending.fee_reserved_usd !== view.policy.fee_reserved_usd)
  ) {
    throw new Error('Token budget reservation mismatch')
  }
  if (
    view.review &&
    (!view.pending ||
      view.review.request_id !== view.pending.request_id ||
      view.review.token_id !== tokenId ||
      view.review.user_id !== view.policy.user_id)
  ) {
    throw new Error('Token budget review mismatch')
  }
  if (view.pending && view.pending.state !== 'prepared' && !view.review) {
    throw new Error('Token budget review is missing')
  }
  return view
}

export async function getTokenBudget(
  tokenId: number,
  signal?: AbortSignal
): Promise<TokenBudgetView> {
  const response = await api.get(`/api/token/${tokenId}/budget`, {
    signal,
    skipErrorHandler: true,
  })
  return parseBudget(response.data, tokenId)
}

export async function writeTokenBudget(
  tokenId: number,
  command: TokenBudgetCommand
): Promise<TokenBudgetView> {
  const response =
    command.kind === 'policy'
      ? await api.put(`/api/token/${tokenId}/budget`, command.body, {
          skipErrorHandler: true,
        })
      : await api.post(`/api/token/${tokenId}/budget/recover`, command.body, {
          skipErrorHandler: true,
        })
  return parseBudget(response.data, tokenId)
}
