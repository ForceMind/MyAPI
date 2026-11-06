/* Copyright (C) 2026 ForceMind. AGPL-3.0-or-later. */
import type { FullContentLogDetail, SavedContentPart, SavedContentSegment, SavedContentSegmentRequest } from '../types'
import { CONTENT_PREVIEW_LIMIT, previewContent } from './content-preview'

export const SAVED_SEGMENT_LIMIT = CONTENT_PREVIEW_LIMIT
const SNAPSHOT = /^[0-9a-f]{64}$/
const MAX_OFFSET = 9223372036854775807n

export function savedByteOffset(value: unknown): bigint | null {
  if (typeof value !== 'string' || value.length > 19 || !/^(0|[1-9]\d*)$/.test(value)) return null
  const parsed = BigInt(value)
  return parsed <= MAX_OFFSET ? parsed : null
}

/** Validate the exact saved representation before counting or displaying it. */
export function savedBodyBytes(body: string, encoding: 'utf-8' | 'base64', limit: number): number | null {
  if (encoding === 'base64') {
    if (body.length > Math.ceil(limit / 3) * 4 || body.length % 4 !== 0 || /[^A-Za-z0-9+/=]/.test(body)) return null
    try {
      const decoded = atob(body)
      return decoded.length <= limit && btoa(decoded) === body ? decoded.length : null
    } catch { return null }
  }
  if (body.length > limit) return null
  for (let index = 0; index < body.length; index += 1) {
    const code = body.charCodeAt(index)
    if (code >= 0xd800 && code <= 0xdbff) {
      const next = body.charCodeAt(index + 1)
      if (!(next >= 0xdc00 && next <= 0xdfff)) return null
      index += 1
    } else if (code >= 0xdc00 && code <= 0xdfff) return null
  }
  const bytes = new TextEncoder().encode(body).length
  return bytes <= limit ? bytes : null
}

export function parseSavedSegment(payload: unknown, requestId: string, request: SavedContentSegmentRequest): SavedContentSegment {
  const invalid = () => { throw new Error('Segment data is invalid.') }
  const requested = savedByteOffset(request.offset)
  const limit = request.limit ?? SAVED_SEGMENT_LIMIT
  if ((request.part !== 'request' && request.part !== 'response') || requested === null || !Number.isSafeInteger(limit) || limit < 4 || limit > SAVED_SEGMENT_LIMIT ||
    (requested > 0n && !request.snapshot) || (request.snapshot !== undefined && !SNAPSHOT.test(request.snapshot))) return invalid()
  if (!payload || typeof payload !== 'object' || !('success' in payload) || payload.success !== true || !('data' in payload) || !payload.data || typeof payload.data !== 'object') return invalid()
  const data = payload.data as Record<string, unknown>
  const offset = savedByteOffset(data.offset)
  const total = savedByteOffset(data.total_bytes)
  const next = data.next_offset === null ? null : savedByteOffset(data.next_offset)
  if (data.request_id !== requestId || data.part !== request.part || data.requested_offset !== request.offset ||
    offset === null || total === null || (data.next_offset !== null && next === null) ||
    (data.expected_bytes !== null && savedByteOffset(data.expected_bytes) === null) ||
    typeof data.has_more !== 'boolean' || typeof data.snapshot !== 'string' || !SNAPSHOT.test(data.snapshot) ||
    (request.snapshot !== undefined && data.snapshot !== request.snapshot) ||
    (data.encoding !== 'utf-8' && data.encoding !== 'base64') || typeof data.content_type !== 'string' || typeof data.body !== 'string' ||
    (data.source_complete !== null && typeof data.source_complete !== 'boolean')) return invalid()
  if (offset > requested || requested - offset > (data.encoding === 'utf-8' ? 3n : 0n) || requested > total || offset > total) return invalid()
  const bytes = savedBodyBytes(data.body, data.encoding, limit)
  if (bytes === null) return invalid()
  const end = offset + BigInt(bytes)
  if (end > total || (requested < total && end <= requested) || data.has_more !== (end < total) ||
    (data.has_more ? next !== end || next === null || next <= requested : data.next_offset !== null || end !== total)) return invalid()
  return data as unknown as SavedContentSegment
}

/** Legacy metadata is only an entry hint; segment totals remain authoritative. */
export function hasSavedSegments(detail: FullContentLogDetail, part: SavedContentPart): boolean {
  const body = part === 'request' ? detail.request_body : detail.response_body
  if (previewContent(body).limited || (part === 'response' && detail.response_body_truncated === true)) return true
  const encoding = part === 'request' ? detail.request_encoding : detail.response_encoding
  const loaded = savedBodyBytes(body, encoding === 'base64' ? 'base64' : 'utf-8', Math.max(body.length * 4, 4))
  const total = part === 'response' ? detail.response_body_total_bytes : undefined
  return loaded !== null && typeof total === 'number' && Number.isFinite(total) && total > loaded
}
