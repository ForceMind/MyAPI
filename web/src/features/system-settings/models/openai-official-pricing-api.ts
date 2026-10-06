import { z } from 'zod'

import { api } from '@/lib/api'

const amount = z.string().regex(/^(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$/)
const rates = z.object({
  input_usd_per_million: amount,
  cached_input_usd_per_million: amount.nullable(),
  cache_write_usd_per_million: amount.nullable(),
  output_usd_per_million: amount,
})
const snapshot = z.object({
  source_url: z.literal('https://developers.openai.com/api/docs/pricing.md'),
  fetched_at: z.number().int().positive(),
  content_sha256: z.string().regex(/^[0-9a-f]{64}$/),
  currency: z.literal('USD'),
  unit_tokens: z.literal(1_000_000),
  service_tier: z.literal('standard'),
  scope: z.literal('text-token-price-source-not-published'),
  models: z
    .array(
      z.object({
        model: z.string().min(1),
        source_label: z.string().min(1),
        short_context: rates,
        long_context: rates.nullable(),
      })
    )
    .min(1)
    .refine(
      (models) =>
        new Set(models.map((model) => model.model)).size === models.length,
      'duplicate_pricing_model'
    ),
})

export type OfficialOpenAIPriceSnapshot = z.infer<typeof snapshot>

export async function fetchOfficialOpenAIPricing(
  signal?: AbortSignal
): Promise<OfficialOpenAIPriceSnapshot> {
  const response = await api.get<{ success: boolean; data?: unknown }>(
    '/api/ratio_sync/openai',
    {
      signal,
      skipErrorHandler: true,
    }
  )
  if (!response.data.success) throw new Error('official_pricing_unavailable')
  return snapshot.parse(response.data.data)
}

export async function saveOfficialOpenAIPriceSource(
  signal?: AbortSignal
): Promise<OfficialOpenAIPriceSnapshot> {
  const response = await api.post<{ success: boolean; data?: unknown }>(
    '/api/ratio_sync/openai/versions',
    undefined,
    { signal, skipErrorHandler: true }
  )
  if (!response.data.success) {
    throw new Error('official_pricing_source_save_failed')
  }
  return snapshot.parse(response.data.data)
}

export async function fetchFrozenOpenAIPriceSource(
  digest: string,
  signal?: AbortSignal
): Promise<OfficialOpenAIPriceSnapshot> {
  if (!/^[0-9a-f]{64}$/.test(digest)) {
    throw new Error('official_pricing_invalid_version')
  }
  const response = await api.get<{ success: boolean; data?: unknown }>(
    `/api/ratio_sync/openai/versions/${digest}`,
    { signal, skipErrorHandler: true }
  )
  if (!response.data.success) {
    throw new Error('official_pricing_source_load_failed')
  }
  const parsed = snapshot.parse(response.data.data)
  if (parsed.content_sha256 !== digest) {
    throw new Error('official_pricing_version_mismatch')
  }
  return parsed
}
