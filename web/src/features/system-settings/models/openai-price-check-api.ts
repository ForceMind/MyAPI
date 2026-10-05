import { z } from 'zod'

import { api } from '@/lib/api'

const digest = z.string().regex(/^[0-9a-f]{64}$/)
const optionalDigest = z.union([digest, z.literal('')])
const count = z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER)
const change = z.enum([
  'addition',
  'change',
  'removal',
  'unqualified',
  'unchanged',
])
const checkState = z.object({
  enabled: z.boolean(),
  interval_seconds: z.literal(86400),
  stale_after_seconds: z.literal(259200),
  stale: z.boolean(),
  last_attempt_at: count,
  last_attempt_status: z.string(),
  last_task_id: z.string(),
  last_error_code: z.string(),
  last_success_at: count,
  next_check_at: count,
  source_sha256: optionalDigest,
  source_fetched_at: count,
  pending_source_sha256: optionalDigest,
  expected_digest: digest,
  revision: count,
  diff_total: count,
  diff_truncated: z.boolean(),
  diff_review_required: z.boolean(),
  diff_counts: z.object({
    addition: count,
    change: count,
    removal: count,
    unqualified: count,
    unchanged: count,
  }),
  diff: z
    .array(
      z.object({
        model: z.string().min(1).max(1024),
        model_sha256: digest.optional(),
        model_name_truncated: z.boolean(),
        change,
        current_mode: z.string(),
        current_expression_sha256: optionalDigest,
        candidate: z
          .object({
            model: z.string(),
            expression: z.string(),
            expression_sha256: digest,
            source_sha256: digest,
          })
          .nullable(),
        locked: z.boolean(),
        eligible: z.boolean(),
        pending_review: z.boolean(),
      })
    )
    .max(128),
})
export type OpenAIPriceCheckState = z.infer<typeof checkState>

export async function fetchOpenAIPriceCheck(
  signal?: AbortSignal
): Promise<OpenAIPriceCheckState> {
  const response = await api.get('/api/ratio_sync/openai/check', {
    signal,
    skipErrorHandler: true,
  })
  return z
    .object({ success: z.literal(true), data: checkState })
    .parse(response.data).data
}

export async function startOpenAIPriceCheck() {
  const response = await api.post(
    '/api/ratio_sync/openai/check',
    {},
    { skipErrorHandler: true }
  )
  return z
    .object({
      success: z.literal(true),
      data: z.object({
        task: z.object({
          task_id: z.string().min(1),
          status: z.enum(['pending', 'running', 'succeeded', 'failed']),
        }),
        created: z.boolean(),
      }),
    })
    .parse(response.data).data
}

export async function setOpenAIPriceCheckEnabled(enabled: boolean) {
  const response = await api.put(
    '/api/option/',
    { key: 'OpenAIOfficialPriceCheckEnabled', value: String(enabled) },
    { skipErrorHandler: true }
  )
  z.object({ success: z.literal(true) }).parse(response.data)
}
