import { z } from 'zod'

import { usdAmountSchema } from '@/lib/exact-usd'

export const tokenBudgetPolicySchema = z.object({
  enabled: z.boolean(),
  feeEnabled: z.boolean(),
  feeLimit: usdAmountSchema,
  limit: z
    .string()
    .regex(/^\d+$/)
    .refine((value) => Number.isSafeInteger(Number(value))),
  confirmed: z.boolean().refine((value) => value),
})
export const tokenBudgetCancelSchema = z.object({
  evidence: z
    .string()
    .trim()
    .min(1)
    .max(2048)
    .refine((value) => new TextEncoder().encode(value).length <= 2048),
  confirmed: z.boolean().refine((value) => value),
})
export type TokenBudgetPolicyValues = z.infer<typeof tokenBudgetPolicySchema>
export type TokenBudgetCancelValues = z.infer<typeof tokenBudgetCancelSchema>
