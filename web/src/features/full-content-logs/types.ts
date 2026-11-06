/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

export interface FullContentLogSummary {
  timestamp: string
  request_id: string
  method: string
  path: string
  model?: string
  status: number
  duration_ms: number
  request_bytes: number
  response_bytes: number
  chunk_count: number
  user_id?: number
  token_id?: number
  token_name?: string
  client_ip?: string
  error?: string
}

export interface FullContentLogFile {
  name: string
  size: number
  modified_at: string
}

export interface FullContentLogFileStats {
  count: number
  total_size: number
  files: FullContentLogFile[]
}

export interface FullContentLogTokenOption {
  id: number
  name: string
}

export interface FullContentLogFacets {
  models: string[]
  tokens: FullContentLogTokenOption[]
}

export interface FullContentLogList {
  enabled: boolean
  page: number
  page_size: number
  total: number
  items: FullContentLogSummary[]
  files: FullContentLogFileStats
  facets: FullContentLogFacets
}

export interface FullContentLogDetail extends FullContentLogSummary {
  request_content_type?: string
  request_encoding?: string
  request_body: string
  response_content_type?: string
  response_encoding?: string
  response_body: string
  response_body_total_bytes?: number
  response_body_truncated?: boolean
  request_headers: Record<string, string[]>
  response_headers: Record<string, string[]>
  query: Record<string, string[]>
}

export interface FullContentLogFilters {
  model: string
  token: string
  requestId: string
  startTime: string
  endTime: string
}

export interface FullContentLogListParams {
  page: number
  pageSize: number
  filters: FullContentLogFilters
}

export interface ApiEnvelope<T> {
  success: boolean
  message?: string
  data?: T
}

export type SavedContentPart = 'request' | 'response'

export interface SavedContentSegmentRequest {
  part: SavedContentPart
  offset: string
  limit?: number
  snapshot?: string
}

export interface SavedContentSegment {
  request_id: string
  part: SavedContentPart
  requested_offset: string
  offset: string
  next_offset: string | null
  total_bytes: string
  expected_bytes: string | null
  has_more: boolean
  snapshot: string
  encoding: 'utf-8' | 'base64'
  content_type: string
  body: string
  source_complete: boolean | null
}
