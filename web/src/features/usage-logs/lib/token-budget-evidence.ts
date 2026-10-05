import { z } from 'zod'

import { usdAmountSchema } from '@/lib/exact-usd'

const count = z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER)
export const tokenBudgetEvidenceSchema = z.object({
  bound_source: z.string().optional(),
  input_tokens_bound: count.optional(),
  max_output_tokens: count.optional(),
  reserved: count.optional(),
  fee_reserved_usd: usdAmountSchema.optional(),
  actual_input: count.nullish(),
  actual_output: count.nullish(),
  actual_fee_usd: usdAmountSchema.nullish(),
})
export type TokenBudgetEvidenceData = z.infer<typeof tokenBudgetEvidenceSchema>
