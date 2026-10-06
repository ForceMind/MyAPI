import { z } from 'zod'

const identifier = z.number().int().positive().max(Number.MAX_SAFE_INTEGER)
const revision = z.number().int().min(0).max(Number.MAX_SAFE_INTEGER)

export const assignedPolicySchema = z.object({
  subject: z.enum(['user', 'token']),
  subject_id: identifier,
  owner_user_id: identifier,
  revision,
  assigned: z.boolean(),
  enabled: z.boolean(),
  public_models: z
    .array(z.string())
    .nullish()
    .transform((value) => value ?? null),
  upstream_models: z
    .array(z.string())
    .nullish()
    .transform((value) => value ?? null),
  channel_ids: z
    .array(identifier)
    .nullish()
    .transform((value) => value ?? null),
})
export const modelAvailabilitySchema = z.object({
  model: z.string().min(1),
  allowed: z.boolean(),
  reasons: z.array(z.string()),
})
export const ownerAccessSchema = z.object({
  token_id: identifier,
  assigned: z.boolean(),
  user_revision: revision,
  token_revision: revision,
  models: z.array(modelAvailabilitySchema),
})
export type AssignedPolicy = z.infer<typeof assignedPolicySchema>
export type ModelAvailability = z.infer<typeof modelAvailabilitySchema>
export type OwnerAccess = z.infer<typeof ownerAccessSchema>
export type PolicySubject = AssignedPolicy['subject']
export type PolicyTarget = {
  subject: PolicySubject
  id: number
  ownerUserId: number
}
export type PolicyCandidate = Pick<
  AssignedPolicy,
  'enabled' | 'public_models' | 'upstream_models' | 'channel_ids'
>
export type PolicyCommand = PolicyCandidate & { expected_revision: number }
export type PolicyPreview = {
  policy: AssignedPolicy
  models: ModelAvailability[]
}
