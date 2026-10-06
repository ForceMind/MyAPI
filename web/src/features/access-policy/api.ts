import { isAxiosError } from 'axios'
import { z } from 'zod'

import { api } from '@/lib/api'

import {
  assignedPolicySchema,
  modelAvailabilitySchema,
  ownerAccessSchema,
  type AssignedPolicy,
  type OwnerAccess,
  type PolicyCandidate,
  type PolicyCommand,
  type PolicyPreview,
  type PolicyTarget,
} from './types'

export class AccessPolicyError extends Error {
  constructor(
    readonly code: string,
    readonly status?: number
  ) {
    super(code)
  }
}

const config = { skipErrorHandler: true, skipBusinessError: true }

function responseData(payload: unknown): unknown {
  const envelope = z
    .object({
      success: z.boolean(),
      data: z.unknown().optional(),
      code: z.string().optional(),
    })
    .parse(payload)
  if (!envelope.success) {
    throw new AccessPolicyError(envelope.code ?? 'request_failed')
  }
  return envelope.data
}

function verifyTarget(
  policy: AssignedPolicy,
  target: PolicyTarget
): AssignedPolicy {
  if (
    policy.subject !== target.subject ||
    policy.subject_id !== target.id ||
    policy.owner_user_id !== target.ownerUserId
  ) {
    throw new AccessPolicyError('identity_mismatch')
  }
  return policy
}

export function accessPolicyErrorKind(
  error: unknown
): 'conflict' | 'forbidden' | 'missing' | 'identity' | 'failed' {
  let code = ''
  let status: number | undefined
  if (error instanceof AccessPolicyError) {
    code = error.code
    status = error.status
  } else if (isAxiosError(error)) {
    status = error.response?.status
    const body = z
      .object({ code: z.string().optional() })
      .safeParse(error.response?.data)
    if (body.success) code = body.data.code ?? ''
  }
  if (status === 409 || code.includes('conflict')) return 'conflict'
  if (status === 403 || code.includes('forbidden')) return 'forbidden'
  if (status === 404 || code.includes('not_found')) return 'missing'
  if (code === 'identity_mismatch') return 'identity'
  return 'failed'
}

export async function getAssignedPolicy(
  target: PolicyTarget,
  signal?: AbortSignal
): Promise<AssignedPolicy> {
  const response = await api.get(
    `/api/access-policy/${target.subject}/${target.id}`,
    {
      ...config,
      signal,
      disableDuplicate: true,
    }
  )
  return verifyTarget(
    assignedPolicySchema.parse(responseData(response.data)),
    target
  )
}

export async function saveAssignedPolicy(
  target: PolicyTarget,
  body: PolicyCommand,
  signal?: AbortSignal
): Promise<AssignedPolicy> {
  const response = await api.put(
    `/api/access-policy/${target.subject}/${target.id}`,
    body,
    { ...config, signal }
  )
  return verifyTarget(
    assignedPolicySchema.parse(responseData(response.data)),
    target
  )
}

export async function removeAssignedPolicy(
  target: PolicyTarget,
  revision: number,
  signal?: AbortSignal
): Promise<AssignedPolicy> {
  const response = await api.delete(
    `/api/access-policy/${target.subject}/${target.id}`,
    {
      ...config,
      signal,
      data: { expected_revision: revision },
    }
  )
  return verifyTarget(
    assignedPolicySchema.parse(responseData(response.data)),
    target
  )
}

export async function previewAssignedPolicy(
  target: PolicyTarget,
  body: PolicyCandidate,
  signal?: AbortSignal
): Promise<PolicyPreview> {
  const response = await api.post(
    `/api/access-policy/${target.subject}/${target.id}/preview`,
    body,
    { ...config, signal }
  )
  const preview = z
    .object({
      policy: assignedPolicySchema,
      models: z.array(modelAvailabilitySchema),
    })
    .parse(responseData(response.data))
  verifyTarget(preview.policy, target)
  return preview
}

export async function getOwnerAccess(
  tokenId: number,
  signal?: AbortSignal
): Promise<OwnerAccess> {
  const response = await api.get(`/api/token/${tokenId}/access`, {
    ...config,
    signal,
    disableDuplicate: true,
  })
  const result = ownerAccessSchema.parse(responseData(response.data))
  if (result.token_id !== tokenId) {
    throw new AccessPolicyError('identity_mismatch')
  }
  return result
}
