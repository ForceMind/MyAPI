/* Copyright (C) 2026 ForceMind. Licensed under AGPL-3.0-or-later. */
import { isAxiosError } from 'axios'
import { z } from 'zod'

import type { VerificationMethod, VerificationMethods } from '@/features/auth/secure-verification/types'
import { api } from '@/lib/api'
import { buildAssertionResult, isPasskeySupported, prepareCredentialRequestOptions } from '@/lib/passkey'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

export function operationsProofScope(plan: Pick<OperationsPlan, 'id' | 'digest'>): `operations.upgrade.approve:${string}:${string}` {
  return `operations.upgrade.approve:${plan.id}:${plan.digest}`
}
export const operationStates = ['prepared', 'approved', 'queued', 'verifying_artifact', 'stopping', 'backing_up', 'creating', 'starting', 'verifying', 'succeeded', 'failed_before_start', 'recovery_required', 'interrupted'] as const
const identity = z.string().min(1)
const epoch = z.number().int().positive()
const planSchema = z.object({
  id: z.string().regex(/^[a-f0-9]{32,64}$/),
  digest: z.string().regex(/^[a-f0-9]{64}$/),
  instance_id: identity,
  owner_id: z.number().int().positive(),
  key_id: z.number().int().positive(),
  state: z.enum(operationStates),
  created_at: epoch,
  expires_at: epoch,
  approved_at: z.number().int().nonnegative().optional(),
  approval_expires_at: z.number().int().nonnegative().optional(),
  source_image_id: z.string().regex(/^sha256:[a-f0-9]{64}$/),
  target_image_id: z.string().regex(/^sha256:[a-f0-9]{64}$/),
  source_container_id: identity,
  target_container_id: z.string().optional(),
  source_config_digest: z.string().regex(/^[a-f0-9]{64}$/),
  source_sha: z.string().regex(/^[a-f0-9]{40}$/),
  tree_sha: z.string().regex(/^[a-f0-9]{40}$/),
  run_id: z.string().regex(/^[1-9]\d*$/),
  producer_attempt: z.string().regex(/^[1-9]\d*$/),
  artifact_id: z.string().regex(/^[1-9]\d*$/),
  manifest_sha256: z.string().regex(/^[a-f0-9]{64}$/),
  version: identity,
  source_version: identity.optional(),
  platform: z.literal('linux/amd64'),
  backup_created: z.boolean(),
  target_start_attempted: z.boolean(),
  source_mutation_attempted: z.boolean().optional(),
  source_restart_disabled: z.boolean().optional(),
  code: z.string().regex(/^[A-Z][A-Z0-9_]{0,79}$/).optional(),
  checks: z.record(z.string(), z.boolean().nullable()).optional(),
})
const instanceSchema = z.object({ id: identity, state: identity, current_image_id: identity.optional(), current_container_id: identity.optional() })
export type OperationsPlan = z.infer<typeof planSchema>
export type OperationsInstance = z.infer<typeof instanceSchema>

const requestOptions = {
  timeout: 15000,
  disableDuplicate: true,
  skipAuthRefresh: true,
  skipBusinessError: true,
  skipErrorHandler: true,
} as const

// Backend text and validation payloads must never reach UI/toast error messages.
export class OperationsError extends Error {
  constructor(public readonly kind: 'unavailable' | 'rejected' | 'unknown' | 'invalid', public readonly code?: string) {
    super('Operations request failed')
  }
}

function operationError(error: unknown): OperationsError {
  if (error instanceof OperationsError) return error
  if (error instanceof z.ZodError) return new OperationsError('invalid')
  if (isAxiosError(error)) {
    if (error.response?.status === 503 && error.response.data?.code === 'OPERATIONS_UNAVAILABLE') return new OperationsError('unavailable', 'OPERATIONS_UNAVAILABLE')
    const status = error.response?.status
    if (status && status >= 400 && status < 500 && status !== 408) return new OperationsError('rejected')
  }
  return new OperationsError('unknown')
}

export function operationsSessionKey(): string | null {
  const { user, session } = useAuthStore.getState().auth
  if (user?.role !== ROLE.SUPER_ADMIN || !session?.sid) return null
  return JSON.stringify([user.id, user.role, session.sid, user.permissions ?? null])
}

type ApprovalAttempt = { plan: OperationsPlan; outcome: 'pending' | 'unknown' | 'settled' }
let attemptScope: string | null = null
let approvalAttempts = new Map<string, ApprovalAttempt>()

function sessionApprovalAttempts(scopeKey: string): Map<string, ApprovalAttempt> {
  if (scopeKey !== operationsSessionKey()) return new Map()
  if (attemptScope !== scopeKey) {
    attemptScope = scopeKey
    approvalAttempts = new Map()
  }
  return approvalAttempts
}

export function unconfirmedOperationsApprovals(scopeKey: string): OperationsPlan[] {
  return [...sessionApprovalAttempts(scopeKey).values()].filter((attempt) => attempt.outcome !== 'settled').map((attempt) => attempt.plan)
}

export function wasOperationsApprovalSubmitted(scopeKey: string, id: string): boolean {
  return sessionApprovalAttempts(scopeKey).has(id)
}

export function confirmOperationsApprovalStatus(scopeKey: string, id: string): void {
  const attempt = sessionApprovalAttempts(scopeKey).get(id)
  if (attempt) attempt.outcome = 'settled'
}

export async function listOperationsPlans(signal: AbortSignal): Promise<OperationsPlan[]> {
  try {
    const response = await api.get('/api/operations/plans', { ...requestOptions, signal })
    if (response.data?.code === 'OPERATIONS_UNAVAILABLE') throw new OperationsError('unavailable')
    return z.object({ success: z.literal(true), data: z.array(planSchema) }).parse(response.data).data
  } catch (error) { throw operationError(error) }
}

export async function listOperationsInstances(signal: AbortSignal): Promise<OperationsInstance[]> {
  try {
    const response = await api.get('/api/operations/instances', { ...requestOptions, signal })
    return z.object({ success: z.literal(true), data: z.array(instanceSchema) }).parse(response.data).data
  } catch (error) { throw operationError(error) }
}

export async function getOperationsPlan(id: string, signal: AbortSignal): Promise<OperationsPlan> {
  try {
    const response = await api.get(`/api/operations/plans/${encodeURIComponent(id)}`, { ...requestOptions, signal })
    return z.object({ success: z.literal(true), data: planSchema }).parse(response.data).data
  } catch (error) { throw operationError(error) }
}

export async function approveOperationsPlan(plan: OperationsPlan, proof: string, signal: AbortSignal): Promise<OperationsPlan> {
  const scopeKey = operationsSessionKey()
  if (!proof || !scopeKey || signal.aborted) throw new OperationsError('rejected')
  const attempts = sessionApprovalAttempts(scopeKey)
  if (attempts.has(plan.id)) throw new OperationsError('rejected')
  const attempt: ApprovalAttempt = { plan, outcome: 'pending' }
  attempts.set(plan.id, attempt)
  try {
    const response = await api.post(`/api/operations/plans/${encodeURIComponent(plan.id)}/approve`, { digest: plan.digest }, { ...requestOptions, signal, headers: { 'X-Security-Proof': proof } })
    if (response.data?.success === false) throw new OperationsError('rejected')
    const approved = z.object({ success: z.literal(true), data: planSchema }).parse(response.data).data
    if (operationsApprovalBlock(plan, approved) !== 'approved') throw new OperationsError('unknown')
    attempt.outcome = 'settled'
    return approved
  } catch (error) {
    const safeError = operationError(error)
    if (attempt.outcome !== 'settled') attempt.outcome = safeError.kind === 'rejected' ? 'settled' : 'unknown'
    throw safeError
  }
}

export type ApprovalBlock = 'changed' | 'expired' | 'approved' | 'state'
export function operationsApprovalBlock(expected: OperationsPlan, current: OperationsPlan): ApprovalBlock | null {
  const immutableFields = ['id', 'digest', 'instance_id', 'owner_id', 'key_id', 'created_at', 'expires_at', 'source_image_id', 'target_image_id', 'source_container_id', 'source_config_digest', 'source_sha', 'tree_sha', 'run_id', 'producer_attempt', 'artifact_id', 'manifest_sha256', 'version', 'source_version', 'platform'] as const
  if (immutableFields.some((field) => expected[field] !== current[field])) return 'changed'
  if (current.state === 'approved') return 'approved'
  if (current.state !== 'prepared') return 'state'
  if (current.expires_at <= Date.now() / 1000) return 'expired'
  return null
}

// Same account verification endpoints and WebAuthn serialization as the existing
// security flow, with safe errors and no authentication replay on this boundary.
export async function operationsVerificationMethods(signal: AbortSignal): Promise<VerificationMethods> {
  try {
    const [twoFA, passkey, supported] = await Promise.all([
      api.get('/api/user/2fa/status', { ...requestOptions, signal }),
      api.get('/api/user/passkey', { ...requestOptions, signal }),
      isPasskeySupported(),
    ])
    const statusSchema = z.object({ success: z.literal(true), data: z.object({ enabled: z.boolean() }) })
    return { has2FA: statusSchema.parse(twoFA.data).data.enabled, hasPasskey: statusSchema.parse(passkey.data).data.enabled, passkeySupported: supported }
  } catch (error) { throw operationError(error) }
}

export async function verifyOperationsApproval(plan: OperationsPlan, method: VerificationMethod, code: string | undefined, signal: AbortSignal, isCurrent: () => boolean): Promise<string> {
  try {
    const scope = operationsProofScope(plan)
    let proofData: unknown
    if (method === '2fa') {
      const response = await api.post('/api/verify', { method: '2fa', code: code?.trim(), scope }, { ...requestOptions, signal })
      proofData = response.data
    } else {
      const begin = await api.post('/api/user/passkey/verify/begin', { scope }, { ...requestOptions, signal })
      if (!isCurrent() || signal.aborted) throw new OperationsError('rejected')
      const beginData = z.object({ success: z.literal(true), data: z.object({ flow_token: identity, options: z.unknown() }) }).parse(begin.data).data
      const publicKey = prepareCredentialRequestOptions(beginData.options)
      const credential = await navigator.credentials.get({ publicKey, signal }) as PublicKeyCredential | null
      if (!credential || !isCurrent() || signal.aborted) throw new OperationsError('rejected')
      const assertion = buildAssertionResult(credential)
      if (!assertion) throw new OperationsError('rejected')
      const finish = await api.post('/api/user/passkey/verify/finish', { flow_token: beginData.flow_token, credential: assertion }, { ...requestOptions, signal })
      proofData = finish.data
    }
    if (!isCurrent() || signal.aborted) throw new OperationsError('rejected')
    const proof = z.object({ success: z.literal(true), data: z.object({ proof_token: identity, scope: z.literal(scope), method: z.enum(['2fa', 'passkey']), expires_at: epoch }) }).parse(proofData).data
    if (proof.method !== method || proof.expires_at <= Date.now() / 1000) throw new OperationsError('rejected')
    return proof.proof_token
  } catch (error) { throw operationError(error) }
}
