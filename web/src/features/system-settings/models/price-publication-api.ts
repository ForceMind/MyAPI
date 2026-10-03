import { isAxiosError } from 'axios'
import { z } from 'zod'

import { api } from '@/lib/api'

const digest = z.string().regex(/^[0-9a-f]{64}$/)
const receipt = z.object({
  id: digest,
  actor_id: z.number().int().positive(),
  action: z.enum(['publish', 'lock', 'rollback']),
  created_at: z.number().int().positive(),
  revision: z.number().int().nonnegative(),
})
const preview = z.object({
  source_sha256: digest,
  expected_digest: digest,
  revision: z.number().int().nonnegative(),
  rows: z.array(
    z.object({
      model: z.string().min(1),
      current_mode: z.string(),
      current_expression: z.string(),
      locked: z.boolean(),
      eligible: z.boolean(),
      candidate: z
        .object({
          model: z.string().min(1),
          expression: z.string().min(1),
          expression_sha256: digest,
          source_sha256: digest,
        })
        .nullable(),
    })
  ),
})
const state = z.object({
  expected_digest: digest,
  runtime_ready: z.boolean(),
  snapshot: z.object({
    state: z.object({ revision: z.number().int().nonnegative() }),
  }),
  receipts: z.array(receipt),
})

export type PricePublicationRequest = {
  id: string
  expected_digest: string
  action: 'publish' | 'lock' | 'rollback'
  source_sha256?: string
  rollback_of?: string
  models?: { model: string; locked: boolean }[]
  confirmed: boolean
}

export async function fetchPricePublicationPreview(
  source: string,
  signal?: AbortSignal
) {
  digest.parse(source)
  const response = await api.get<{ success: boolean; data?: unknown }>(
    `/api/ratio_sync/openai/versions/${source}/publication-preview`,
    { signal, skipErrorHandler: true }
  )
  if (!response.data.success) {
    throw new Error('price_publication_preview_failed')
  }
  const parsed = preview.parse(response.data.data)
  if (parsed.source_sha256 !== source) {
    throw new Error('price_publication_source_mismatch')
  }
  if (
    new Set(parsed.rows.map((row) => row.model)).size !== parsed.rows.length ||
    parsed.rows.some(
      (row) =>
        (row.eligible && (row.locked || !row.candidate)) ||
        (row.candidate &&
          (row.candidate.model !== row.model ||
            row.candidate.source_sha256 !== source))
    )
  ) {
    throw new Error('price_publication_source_mismatch')
  }
  return parsed
}

export async function fetchPricePublicationState(signal?: AbortSignal) {
  const response = await api.get<{ success: boolean; data?: unknown }>(
    '/api/ratio_sync/openai/publications',
    { signal, skipErrorHandler: true }
  )
  if (!response.data.success) throw new Error('price_publication_state_failed')
  return state.parse(response.data.data)
}

export async function applyPricePublication(request: PricePublicationRequest) {
  try {
    const response = await api.post<{
      success: boolean
      data?: unknown
      code?: string
    }>('/api/ratio_sync/openai/publications', request, {
      skipErrorHandler: true,
    })
    if (!response.data.success) {
      throw new Error(response.data.code ?? 'price_publication_failed')
    }
    const result = z
      .object({ receipt, runtime_ready: z.literal(true) })
      .parse(response.data.data)
    if (result.receipt.id !== request.id) {
      throw new Error('price_publication_receipt_mismatch')
    }
    return result
  } catch (error) {
    if (isAxiosError(error)) {
      const body = z
        .object({ code: z.string() })
        .safeParse(error.response?.data)
      if (body.success) throw new Error(body.data.code)
    }
    throw error
  }
}

export function newPricePublicationID() {
  return `${crypto.randomUUID()}${crypto.randomUUID()}`.replaceAll('-', '')
}
