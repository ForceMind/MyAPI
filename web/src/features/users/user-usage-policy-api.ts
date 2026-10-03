import { z } from 'zod'

import { api } from '@/lib/api'

const policySchema = z.object({
  user_id: z.number().int().positive(),
  no_balance: z.boolean(),
  revision: z.number().int().min(0).max(Number.MAX_SAFE_INTEGER),
  legacy_remaining_quota: z
    .number()
    .int()
    .min(Number.MIN_SAFE_INTEGER)
    .max(Number.MAX_SAFE_INTEGER),
})
export type UserUsagePolicy = z.infer<typeof policySchema>
export type UserUsagePolicyCommand = {
  id: string
  expected_revision: number
  no_balance: boolean
  confirmed: true
}
function parsePolicy(payload: unknown, userId: number): UserUsagePolicy {
  const policy = z
    .object({ success: z.literal(true), data: policySchema })
    .parse(payload).data
  if (policy.user_id !== userId) {
    throw new Error('User usage policy identity mismatch')
  }
  return policy
}
export async function getUserUsagePolicy(
  userId: number,
  signal?: AbortSignal
): Promise<UserUsagePolicy> {
  const response = await api.get(`/api/user/${userId}/usage-policy`, {
    signal,
    skipErrorHandler: true,
  })
  return parsePolicy(response.data, userId)
}
export async function writeUserUsagePolicy(
  userId: number,
  body: UserUsagePolicyCommand
): Promise<UserUsagePolicy> {
  const response = await api.put(`/api/user/${userId}/usage-policy`, body, {
    skipErrorHandler: true,
  })
  return parsePolicy(response.data, userId)
}
