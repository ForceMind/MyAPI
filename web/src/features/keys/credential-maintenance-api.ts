/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import axios from 'axios'
import { z } from 'zod'

import { api } from '@/lib/api'
import { createOperationId } from '@/lib/operation-id'

const hexDigest = z.string().regex(/^[0-9a-f]{64}$/)
const status = z.enum(['ready', 'blocked', 'unknown'])
const blockerSchema = z.object({
  code: z.string().min(1),
  state: z.enum(['blocked', 'unknown']),
})
const sourceWindowSchema = z.object({
  pool_id: z.number().int().positive(),
  window_slot: z.string().min(1),
  revision: z.number().int().min(0),
  share_bps: z.number().int().min(0).max(10000),
  observed_at: z.number().int().min(0),
  reset_at: z.number().int().min(0),
})
const targetWindowSchema = sourceWindowSchema.extend({
  available_bps: z.number().int().min(0).max(10000),
})
const changesSchema = z.object({
  policy: z.object({
    changed: z.boolean(),
    before_revision: z.number().int().min(0),
    after_revision: z.number().int().min(0),
    source_channel_id: z.number().int().positive(),
    target_channel_id: z.number().int().positive(),
  }),
  allocations: z.array(
    z.object({
      source_pool_id: z.number().int().positive(),
      target_pool_id: z.number().int().positive(),
      window_slot: z.string().min(1),
      share_bps: z.number().int().min(0).max(10000),
      target_existing: z.boolean(),
    })
  ),
})
const diagnosticChangesSchema = changesSchema.extend({
  policy: changesSchema.shape.policy.extend({
    source_channel_id: z.number().int().min(0),
    target_channel_id: z.number().int().min(0),
  }),
})

const maintenanceSchema = z.object({
  token: z.object({
    id: z.number().int().positive(),
    name: z.string(),
    masked_key: z.string(),
    owner_user_id: z.number().int().positive(),
  }),
  permissions: z.object({
    can_migrate: z.boolean(),
    reason: z.string(),
  }),
  rotation: z.object({
    commit_supported: z.boolean(),
    status,
    blockers: z.array(blockerSchema),
  }),
  migration: z.object({
    status,
    source: z
      .object({
        channel_id: z.number().int().positive(),
        channel_name: z.string(),
        subject_hint: z.string(),
        windows: z.array(sourceWindowSchema),
      })
      .nullable(),
    candidates: z.array(
      z.object({
        channel_id: z.number().int().positive(),
        channel_name: z.string(),
        subject_hint: z.string(),
        credential_mode: z.literal('single'),
        status,
        blockers: z.array(blockerSchema),
        windows: z.array(targetWindowSchema),
      })
    ),
    blockers: z.array(blockerSchema),
  }),
})

const previewBaseSchema = z.object({
  expires_at: z.number().int().min(0),
  blockers: z.array(blockerSchema),
})
const previewSchema = z.discriminatedUnion('status', [
  previewBaseSchema.extend({
    status: z.literal('ready'),
    preview_revision: hexDigest,
    changes: changesSchema,
  }),
  previewBaseSchema.extend({
    status: z.literal('blocked'),
    preview_revision: z.literal(''),
    changes: diagnosticChangesSchema,
  }),
  previewBaseSchema.extend({
    status: z.literal('unknown'),
    preview_revision: z.literal(''),
    changes: diagnosticChangesSchema,
  }),
])

const receiptSchema = z.object({
  operation_id: hexDigest,
  state: z.literal('committed'),
  replayed: z.boolean().optional().default(false),
  token_id: z.number().int().positive(),
  source_subject_hint: z.string(),
  target_subject_hint: z.string(),
  source_channel_id: z.number().int().positive(),
  target_channel_id: z.number().int().positive(),
  policy_before_revision: z.number().int().min(0),
  policy_after_revision: z.number().int().min(0),
  created_at: z.number().int().min(0),
  changes: changesSchema,
})

const response = <T extends z.ZodType>(schema: T) =>
  z.object({ success: z.literal(true), data: schema })

export type CredentialMaintenance = z.infer<typeof maintenanceSchema>
export type CredentialMigrationPreview = z.infer<typeof previewSchema>
export type CredentialMigrationReceipt = z.infer<typeof receiptSchema>
export type CredentialMigrationBlocker = z.infer<typeof blockerSchema>
export type CredentialTargetShare = {
  source_pool_id: number
  target_pool_id: number
  share_bps: number
}
export type CredentialMigrationInput = {
  target_channel_id: number
  target_shares: CredentialTargetShare[]
}

export const createCredentialMigrationOperationId = createOperationId

const requestConfig = (signal?: AbortSignal) => ({
  signal,
  timeout: 15000,
  disableDuplicate: true,
  skipAuthRefresh: true,
  skipBusinessError: true,
  skipErrorHandler: true,
})

export async function getCredentialMaintenance(
  tokenId: number,
  signal?: AbortSignal
): Promise<CredentialMaintenance> {
  const result = await api.get(
    `/api/token/${tokenId}/maintenance`,
    requestConfig(signal)
  )
  const data = response(maintenanceSchema).parse(result.data).data
  if (data.token.id !== tokenId) {
    throw new Error('Credential maintenance identity mismatch')
  }
  return data
}

export async function previewCredentialMigration(
  tokenId: number,
  input: CredentialMigrationInput,
  signal?: AbortSignal
): Promise<CredentialMigrationPreview> {
  const result = await api.post(
    `/api/token/${tokenId}/account-migration/preview`,
    input,
    requestConfig(signal)
  )
  return response(previewSchema).parse(result.data).data
}

export async function commitCredentialMigration(
  tokenId: number,
  input: CredentialMigrationInput & {
    operation_id: string
    preview_revision: string
    confirmed: true
  },
  proofToken: string,
  signal?: AbortSignal
): Promise<CredentialMigrationReceipt> {
  const result = await api.post(
    `/api/token/${tokenId}/account-migration`,
    input,
    {
      ...requestConfig(signal),
      headers: { 'X-Security-Proof': proofToken },
    }
  )
  const data = response(receiptSchema).parse(result.data).data
  if (data.token_id !== tokenId || data.operation_id !== input.operation_id) {
    throw new Error('Credential migration receipt identity mismatch')
  }
  return data
}

export async function getCredentialMigrationReceipt(
  tokenId: number,
  operationId: string,
  signal?: AbortSignal
): Promise<CredentialMigrationReceipt> {
  const result = await api.get(
    `/api/token/${tokenId}/maintenance/operations/${operationId}`,
    requestConfig(signal)
  )
  const data = response(receiptSchema).parse(result.data).data
  if (data.token_id !== tokenId || data.operation_id !== operationId) {
    throw new Error('Credential migration receipt identity mismatch')
  }
  return data
}

export function isUncertainCredentialCommit(error: unknown): boolean {
  if (!axios.isAxiosError(error)) return true
  const statusCode = error.response?.status
  return (
    statusCode === undefined ||
    statusCode < 400 ||
    statusCode === 408 ||
    statusCode === 499 ||
    statusCode >= 500
  )
}
