/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { z } from 'zod'

import { api } from '@/lib/api'

const revisionSchema = z.string().regex(/^[0-9a-f]{64}$/)
const modelScopeSchema = z
  .object({
    token_id: z.number().int().positive(),
    token_name: z.string(),
    model_limits_enabled: z.boolean(),
    models: z.array(z.string()),
    revision: revisionSchema,
  })
  .strict()
const modelScopeEnvelopeSchema = z
  .object({
    success: z.literal(true),
    message: z.string().optional(),
    data: modelScopeSchema,
  })
  .strict()
const modelCatalogEnvelopeSchema = z.object({
  success: z.literal(true),
  message: z.string().optional(),
  data: z.array(z.string()),
})

export type ModelScope = z.infer<typeof modelScopeSchema>

export type ModelScopeCommand = {
  expected_revision: string
  model_limits_enabled: boolean
  models: string[]
}

const requestConfig = (signal?: AbortSignal) => ({
  signal,
  timeout: 15000,
  disableDuplicate: true,
  skipAuthRefresh: true,
  skipBusinessError: true,
  skipErrorHandler: true,
})

function parseModelScope(payload: unknown, tokenId: number): ModelScope {
  const scope = modelScopeEnvelopeSchema.parse(payload).data
  if (scope.token_id !== tokenId) {
    throw new Error('Model scope identity mismatch')
  }
  return scope
}

export async function getModelScope(
  tokenId: number,
  signal?: AbortSignal
): Promise<ModelScope> {
  const response = await api.get(
    `/api/token/${tokenId}/model-limits`,
    requestConfig(signal)
  )
  return parseModelScope(response.data, tokenId)
}

export async function updateModelScope(
  tokenId: number,
  command: ModelScopeCommand,
  signal?: AbortSignal
): Promise<ModelScope> {
  const response = await api.put(
    `/api/token/${tokenId}/model-limits`,
    command,
    requestConfig(signal)
  )
  return parseModelScope(response.data, tokenId)
}

export async function getModelScopeCatalog(
  group: string,
  signal?: AbortSignal
): Promise<string[]> {
  const response = await api.get('/api/user/models', {
    ...requestConfig(signal),
    params: group ? { group } : undefined,
  })
  return modelCatalogEnvelopeSchema.parse(response.data).data
}
