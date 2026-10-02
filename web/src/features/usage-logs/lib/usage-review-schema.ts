import { z } from 'zod'

export const usageReviewFormSchema = z
  .object({
    amount: z
      .string()
      .regex(/^\d+$/)
      .refine((value) => Number(value) <= 2147483647),
    input: z.string(),
    output: z.string(),
    requiresTokens: z.boolean(),
    evidence: z
      .string()
      .trim()
      .min(1)
      .max(2048)
      .refine((value) => new TextEncoder().encode(value).length <= 2048),
    confirmed: z.boolean().refine((value) => value),
  })
  .superRefine((value, context) => {
    if (!value.requiresTokens) return
    for (const key of ['input', 'output'] as const) {
      if (!/^\d+$/.test(value[key]) || Number(value[key]) > 2147483647) {
        context.addIssue({
          code: 'custom',
          path: [key],
          message: 'Invalid token count',
        })
      }
    }
    if (Number(value.input) + Number(value.output) > 2147483647) {
      context.addIssue({
        code: 'custom',
        path: ['output'],
        message: 'Invalid total token count',
      })
    }
  })

export type UsageReviewFormValues = z.infer<typeof usageReviewFormSchema>
