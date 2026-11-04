import { AxiosError, type AxiosAdapter, type InternalAxiosRequestConfig } from 'axios'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { approveOperationsPlan, listOperationsPlans, OperationsError, operationsApprovalBlock, operationsProofScope, verifyOperationsApproval } from '../operations-api'
import { now, operationResponse, operationsSignIn, plan } from './operations-fixtures'

const previousAdapter = api.defaults.adapter
const previousAuth = useAuthStore.getState()
beforeEach(() => { operationsSignIn(); vi.spyOn(Date, 'now').mockReturnValue(now * 1000) })
afterEach(() => { api.defaults.adapter = previousAdapter; useAuthStore.setState(previousAuth); vi.restoreAllMocks(); vi.unstubAllGlobals() })

it('sends the frozen digest and proof exactly once after a 401 without auth refresh or replay', async () => {
  const requests: InternalAxiosRequestConfig[] = []
  api.defaults.adapter = (async (config) => {
    requests.push(config)
    throw new AxiosError('private backend text', AxiosError.ERR_BAD_REQUEST, config, undefined, { config, data: { success: false, message: 'private backend text' }, status: 401, statusText: 'Unauthorized', headers: {} })
  }) as AxiosAdapter
  await expect(approveOperationsPlan(plan, 'synthetic-proof', new AbortController().signal)).rejects.toMatchObject({ kind: 'rejected', message: 'Operations request failed' })
  expect(requests).toHaveLength(1)
  expect(requests[0]).toMatchObject({ url: `/api/operations/plans/${plan.id}/approve`, method: 'post', timeout: 15000, skipAuthRefresh: true, skipBusinessError: true, skipErrorHandler: true })
  expect(requests[0]?.headers.get('X-Security-Proof')).toBe('synthetic-proof')
  expect(JSON.parse(requests[0]?.data as string)).toEqual({ digest: plan.digest })
})

it('rejects a malformed plan instead of inventing missing version, timestamp or backup values', async () => {
  api.defaults.adapter = async (config) => operationResponse(config, [{ ...plan, backup_created: undefined, expires_at: undefined }])
  await expect(listOperationsPlans(new AbortController().signal)).rejects.toMatchObject({ kind: 'invalid' })
})

it('preserves unconfigured status separately from transport unknown', async () => {
  api.defaults.adapter = (async (config) => {
    throw new AxiosError('private config', AxiosError.ERR_BAD_RESPONSE, config, undefined, { config, data: { code: 'OPERATIONS_UNAVAILABLE' }, status: 503, statusText: 'Unavailable', headers: {} })
  }) as AxiosAdapter
  await expect(listOperationsPlans(new AbortController().signal)).rejects.toMatchObject({ kind: 'unavailable' })
  api.defaults.adapter = async () => { throw new Error('private transport') }
  await expect(listOperationsPlans(new AbortController().signal)).rejects.toMatchObject({ kind: 'unknown' })
})

it('requests proof bound to the exact plan ID and digest and rejects proof for another plan', async () => {
  const requests: InternalAxiosRequestConfig[] = []
  api.defaults.adapter = async (config) => {
    requests.push(config)
    return operationResponse(config, { proof_token: 'synthetic-proof', method: '2fa', scope: operationsProofScope({ ...plan, digest: 'c'.repeat(64) }), expires_at: now + 60 })
  }
  await expect(verifyOperationsApproval(plan, '2fa', '123456', new AbortController().signal, () => true)).rejects.toBeInstanceOf(OperationsError)
  expect(requests).toHaveLength(1)
  expect(JSON.parse(requests[0]?.data as string)).toEqual({ method: '2fa', code: '123456', scope: operationsProofScope(plan) })
  expect(requests[0]).toMatchObject({ skipAuthRefresh: true, skipBusinessError: true, skipErrorHandler: true })
})

it.each([
  [{ ...plan, expires_at: now }, 'changed'],
  [{ ...plan, digest: 'c'.repeat(64) }, 'changed'],
  [{ ...plan, target_image_id: `sha256:${'7'.repeat(64)}` }, 'changed'],
  [{ ...plan, state: 'approved' as const }, 'approved'],
  [{ ...plan, state: 'recovery_required' as const }, 'state'],
] as const)('blocks changed immutable fields or non-prepared state: %s', (current, expected) => {
  expect(operationsApprovalBlock(plan, current)).toBe(expected)
})
it('blocks an expired frozen plan even when every immutable field still matches', () => {
  const expired = { ...plan, expires_at: now }
  expect(operationsApprovalBlock(expired, expired)).toBe('expired')
})


it('does not replay a network-unknown approval even if the caller requests it again', async () => {
  const requests: InternalAxiosRequestConfig[] = []
  api.defaults.adapter = async (config) => { requests.push(config); throw new AxiosError('private network failure', AxiosError.ERR_NETWORK, config) }
  await expect(approveOperationsPlan(plan, 'first-proof', new AbortController().signal)).rejects.toMatchObject({ kind: 'unknown' })
  await expect(approveOperationsPlan(plan, 'second-proof', new AbortController().signal)).rejects.toMatchObject({ kind: 'rejected' })
  expect(requests).toHaveLength(1)
})

it('uses the existing WebAuthn assertion shape and a plan-bound scope for Passkey proof', async () => {
  const requests: InternalAxiosRequestConfig[] = []
  const bytes = new Uint8Array([1, 2]).buffer
  const credential = { id: 'synthetic-credential', type: 'public-key', rawId: bytes, response: { authenticatorData: bytes, clientDataJSON: bytes, signature: bytes, userHandle: null }, authenticatorAttachment: null, getClientExtensionResults: () => ({}) }
  const getCredential = vi.fn().mockResolvedValue(credential)
  vi.stubGlobal('navigator', { credentials: { get: getCredential } })
  api.defaults.adapter = async (config) => {
    requests.push(config)
    if (config.url?.endsWith('/begin')) return operationResponse(config, { flow_token: 'synthetic-flow', options: { publicKey: { challenge: 'AQI', allowCredentials: [] } } })
    return operationResponse(config, { proof_token: 'synthetic-proof', expires_at: now + 60, scope: operationsProofScope(plan), method: 'passkey' })
  }
  const signal = new AbortController().signal
  await expect(verifyOperationsApproval(plan, 'passkey', undefined, signal, () => true)).resolves.toBe('synthetic-proof')
  expect(requests.map((request) => request.url)).toEqual(['/api/user/passkey/verify/begin', '/api/user/passkey/verify/finish'])
  expect(JSON.parse(requests[0]?.data as string)).toEqual({ scope: operationsProofScope(plan) })
  expect(JSON.parse(requests[1]?.data as string)).toEqual({ flow_token: 'synthetic-flow', credential: { id: 'synthetic-credential', type: 'public-key', rawId: 'AQI', authenticatorAttachment: null, response: { authenticatorData: 'AQI', clientDataJSON: 'AQI', signature: 'AQI', userHandle: null }, clientExtensionResults: {} } })
  expect(getCredential).toHaveBeenCalledWith({ publicKey: { challenge: bytes, allowCredentials: [] }, signal })
  expect(requests.every((request) => request.skipAuthRefresh && request.skipBusinessError && request.skipErrorHandler)).toBe(true)
})


it.each([undefined, null, 0])('rejects a plan with missing, null or zero management key ID: %s', async (keyID) => {
  api.defaults.adapter = async (config) => operationResponse(config, [{ ...plan, key_id: keyID }])
  await expect(listOperationsPlans(new AbortController().signal)).rejects.toMatchObject({ kind: 'invalid' })
})

it('preserves optional source mutation markers without inventing false for absent fields', async () => {
  api.defaults.adapter = async (config) => operationResponse(config, [plan, { ...plan, source_mutation_attempted: true, source_restart_disabled: false }])
  const plans = await listOperationsPlans(new AbortController().signal)
  expect(plans[0]).not.toHaveProperty('source_mutation_attempted')
  expect(plans[0]).not.toHaveProperty('source_restart_disabled')
  expect(plans[1]).toMatchObject({ source_mutation_attempted: true, source_restart_disabled: false })
})
