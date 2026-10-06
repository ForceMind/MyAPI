import { z } from 'zod'

export const userUsagePolicySchema = z.object({
  noBalance: z.boolean(),
  confirmed: z.boolean().refine((value) => value),
})
export type UserUsagePolicyValues = z.infer<typeof userUsagePolicySchema>
