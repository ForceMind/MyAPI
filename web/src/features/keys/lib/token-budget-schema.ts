import { z } from 'zod'

import { usdAmountSchema } from '@/lib/exact-usd'

// Two decimal percentage places map exactly to integer basis points.
export function percentageToBasisPoints(value: string): number {
  const [whole, fraction = ''] = value.split('.')
  return Number(whole) * 100 + Number(fraction.padEnd(2, '0'))
}

export const tokenBudgetPolicySchema = z
  .object({
    enabled: z.boolean(),
    accountThresholdEnabled: z.boolean(),
    minimumRemainingPercent: z
      .string()
      .regex(/^\d{1,3}(\.\d{1,2})?$/)
      .refine((value) => percentageToBasisPoints(value) <= 10000),
    maxAgeSeconds: z
      .string()
      .regex(/^\d{1,4}$/)
      .refine((value) => Number(value) >= 30 && Number(value) <= 3600),
    feeEnabled: z.boolean(),
    feeLimit: usdAmountSchema,
    limit: z
      .string()
      .regex(/^\d+$/)
      .refine((value) => Number.isSafeInteger(Number(value))),
    confirmed: z.boolean().refine((value) => value),
  })
  .refine(
    (value) =>
      !value.accountThresholdEnabled || (!value.enabled && !value.feeEnabled),
    {
      path: ['accountThresholdEnabled'],
      message:
        'Account thresholds cannot be combined with Token or USD budgets on this key.',
    }
  )
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
