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
const blockerSchema = z
  .object({
    code: z.string().min(1),
    state: z.enum(['blocked', 'unknown']),
  })
  .strict()
const effectsSchema = z
  .object({
    token_id_preserved: z.literal(true),
    old_secret_invalidation: z.literal('immediate'),
    accepted_settlement: z.literal('continues'),
    writer_changed: z.literal(false),
  })
  .strict()
const previewBaseSchema = z
  .object({
    expires_at: z.number().int().min(0),
    credential_version_before: z.number().int().min(0),
    credential_version_after: z.number().int().min(0),
    blockers: z.array(blockerSchema),
  })
  .strict()
const previewSchema = z
  .discriminatedUnion('status', [
    previewBaseSchema.extend({
      status: z.literal('ready'),
      preview_revision: hexDigest,
      effects: effectsSchema,
    }),
    previewBaseSchema.extend({
      status: z.literal('blocked'),
      preview_revision: z.literal(''),
    }),
    previewBaseSchema.extend({
      status: z.literal('unknown'),
      preview_revision: z.literal(''),
    }),
  ])
  .superRefine((preview, context) => {
    if (preview.status !== 'ready') return
    if (
      !Number.isSafeInteger(preview.credential_version_before) ||
      !Number.isSafeInteger(preview.credential_version_after) ||
      preview.credential_version_before < 1 ||
      preview.credential_version_after !== preview.credential_version_before + 1
    ) {
      context.addIssue({
        code: 'custom',
        message: 'Ready credential rotation has invalid versions',
      })
    }
  })
const receiptSchema = z
  .object({
    operation_id: hexDigest,
    state: z.literal('committed'),
    token_id: z.number().int().positive(),
    credential_version_before: z.number().int().min(0),
    credential_version_after: z.number().int().positive(),
    masked_key: z.string().min(1),
    created_at: z.number().int().min(0),
    old_secret_invalidated_at: z.number().int().min(0),
    settlement_mode: z.literal('authoritative_persistent'),
    writer_changed: z.literal(false),
  })
  .strict()
  .superRefine((receipt, context) => {
    if (
      !Number.isSafeInteger(receipt.credential_version_before) ||
      !Number.isSafeInteger(receipt.credential_version_after) ||
      receipt.credential_version_before < 1 ||
      receipt.credential_version_after !== receipt.credential_version_before + 1
    ) {
      context.addIssue({
        code: 'custom',
        message: 'Credential rotation receipt has invalid versions',
      })
    }
  })
const revealSchema = z
  .object({ secret: z.string().regex(/^sk-[A-Za-z0-9_-]{48}$/) })
  .strict()
const commitResultSchema = z.discriminatedUnion('replayed', [
  z
    .object({
      receipt: receiptSchema,
      replayed: z.literal(false),
      reveal: revealSchema,
    })
    .strict(),
  z
    .object({
      receipt: receiptSchema,
      replayed: z.literal(true),
      reveal: z.null(),
    })
    .strict(),
])
const successResponse = <T extends z.ZodType>(schema: T) =>
  z
    .object({
      success: z.literal(true),
      message: z.string().optional(),
      data: schema,
    })
    .strict()
const dispositionResponse = z
  .object({
    data: z
      .object({
        commit_disposition: z.enum(['not_accepted', 'unknown']),
      })
      .passthrough(),
  })
  .passthrough()

export type CredentialRotationPreview = z.infer<typeof previewSchema>
export type CredentialRotationReceipt = z.infer<typeof receiptSchema>
export type CredentialRotationCommitResult = z.infer<typeof commitResultSchema>
export type CredentialRotationBlocker = z.infer<typeof blockerSchema>
export type CredentialRotationCommitDisposition = 'not_accepted' | 'unknown'

export const createCredentialRotationOperationId = createOperationId

const requestConfig = (signal?: AbortSignal) => ({
  signal,
  timeout: 15000,
  disableDuplicate: true,
  skipAuthRefresh: true,
  skipBusinessError: true,
  skipErrorHandler: true,
})

export async function previewCredentialRotation(
  tokenId: number,
  signal?: AbortSignal
): Promise<CredentialRotationPreview> {
  const result = await api.post(
    `/api/token/${tokenId}/credential-rotation/preview`,
    {},
    requestConfig(signal)
  )
  return successResponse(previewSchema).parse(result.data).data
}

export async function commitCredentialRotation(
  tokenId: number,
  input: {
    operation_id: string
    preview_revision: string
    confirmed: true
  },
  proofToken: string,
  signal?: AbortSignal
): Promise<CredentialRotationCommitResult> {
  const result = await api.post(
    `/api/token/${tokenId}/credential-rotation`,
    input,
    {
      ...requestConfig(signal),
      headers: { 'X-Security-Proof': proofToken },
    }
  )
  const data = successResponse(commitResultSchema).parse(result.data).data
  if (
    data.receipt.token_id !== tokenId ||
    data.receipt.operation_id !== input.operation_id
  ) {
    throw new Error('Credential rotation receipt identity mismatch')
  }
  return data
}

export async function getCredentialRotationReceipt(
  tokenId: number,
  operationId: string,
  signal?: AbortSignal
): Promise<CredentialRotationReceipt> {
  const result = await api.get(
    `/api/token/${tokenId}/credential-rotation/operations/${operationId}`,
    requestConfig(signal)
  )
  const data = successResponse(receiptSchema).parse(result.data).data
  if (data.token_id !== tokenId || data.operation_id !== operationId) {
    throw new Error('Credential rotation receipt identity mismatch')
  }
  return data
}

export function getCredentialRotationCommitDisposition(
  error: unknown
): CredentialRotationCommitDisposition {
  if (!axios.isAxiosError(error)) return 'unknown'
  const status = error.response?.status
  if (
    !status ||
    status < 400 ||
    status === 408 ||
    status === 499 ||
    status >= 500
  ) {
    return 'unknown'
  }
  const parsed = dispositionResponse.safeParse(error.response?.data)
  if (parsed.success) return parsed.data.data.commit_disposition
  const headerDisposition =
    error.response?.headers?.['x-myapi-credential-rotation-disposition']
  if (headerDisposition === 'not_accepted') {
    return 'not_accepted'
  }
  return 'unknown'
}
