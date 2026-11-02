import z from 'zod'

// The same public, non-sensitive view state is accepted by both catalog routes.
export const pricingSearchSchema = z.object({
  search: z.string().optional(),
  sort: z.string().optional(),
  vendor: z.string().optional(),
  group: z.string().optional(),
  quotaType: z.string().optional(),
  endpointType: z.string().optional(),
  tag: z.string().optional(),
  tokenUnit: z.enum(['M', 'K']).optional(),
  view: z.enum(['card', 'table']).optional().catch(undefined),
  rechargePrice: z.boolean().optional(),
  focusModel: z.string().min(1).max(1024).optional().catch(undefined),
  page: z.number().int().positive().max(2147483647).optional().catch(undefined),
  pageSize: z.number().int().positive().max(100).optional().catch(undefined),
})

export type PricingSearch = z.infer<typeof pricingSearchSchema>
