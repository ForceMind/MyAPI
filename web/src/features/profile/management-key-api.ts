/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { isAxiosError } from 'axios'
import { z } from 'zod'

import { api } from '@/lib/api'

export const managementKeyScopes = [
  'channels.read',
  'channels.operate',
  'tokens.read',
  'logs.read',
  'system.read',
] as const

export type ManagementKeyScope = (typeof managementKeyScopes)[number]

const metadataSchema = z.object({
  id: z.number().int().positive(),
  name: z.string(),
  prefix: z.string(),
  scopes: z.array(z.enum(managementKeyScopes)),
  channel_ids: z.array(z.number().int().positive()),
  token_ids: z.array(z.number().int().positive()),
  created_at: z.number().int().nonnegative(),
  expires_at: z.number().int().nonnegative(),
  revoked_at: z.number().int().nonnegative(),
})

export type ManagementKeyMetadata = z.infer<typeof metadataSchema>
export type CreateManagementKey = {
  name: string
  scopes: ManagementKeyScope[]
  channel_ids: number[]
  token_ids: number[]
  expires_at: number
}

// Do not expose backend messages: an unexpected response may contain a secret.
export class ManagementKeyRejected extends Error {}

export function isManagementKeyRejection(error: unknown): boolean {
  if (error instanceof ManagementKeyRejected) return true
  const status = isAxiosError(error) ? error.response?.status : undefined
  return status !== undefined && status >= 400 && status < 500 && status !== 408
}

export async function listManagementKeys(signal: AbortSignal) {
  const response = await api.get('/api/management/keys', {
    signal,
    timeout: 15000,
    disableDuplicate: true,
    skipBusinessError: true,
    skipErrorHandler: true,
  })
  return z.object({ success: z.literal(true), data: z.array(metadataSchema) })
    .parse(response.data).data
}

export async function createManagementKey(payload: CreateManagementKey, signal: AbortSignal) {
  const response = await api.post('/api/management/keys', payload, {
    signal,
    timeout: 15000,
    skipAuthRefresh: true,
    skipBusinessError: true,
    skipErrorHandler: true,
  })
  if (response.data?.success === false) throw new ManagementKeyRejected()
  return z.object({
    success: z.literal(true),
    data: z.object({ key: metadataSchema, secret: z.string().min(1) }),
  }).parse(response.data).data
}

export async function revokeManagementKey(id: number, signal: AbortSignal) {
  const response = await api.delete(`/api/management/keys/${id}`, {
    signal,
    timeout: 15000,
    skipAuthRefresh: true,
    skipBusinessError: true,
    skipErrorHandler: true,
  })
  if (response.data?.success === false) throw new ManagementKeyRejected()
  z.object({ success: z.literal(true) }).parse(response.data)
}
