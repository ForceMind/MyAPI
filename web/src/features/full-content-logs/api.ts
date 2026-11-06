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
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import type {
  ApiEnvelope,
  FullContentLogDetail,
  FullContentLogFileStats,
  FullContentLogList,
  FullContentLogListParams,
  SavedContentSegment,
  SavedContentSegmentRequest,
} from './types'
import { parseSavedSegment, SAVED_SEGMENT_LIMIT } from './lib/saved-segment'

function localTimeToUnix(value: string): number | undefined {
  if (!value) return undefined
  const timestamp = new Date(value).getTime()
  if (!Number.isFinite(timestamp)) return undefined
  return Math.floor(timestamp / 1000)
}

export async function getFullContentLogs(
  params: FullContentLogListParams,
  signal?: AbortSignal
): Promise<ApiEnvelope<FullContentLogList>> {
  const response = await api.get('/api/full-content-logs', {
    signal,
    params: {
      p: params.page,
      page_size: params.pageSize,
      model: params.filters.model || undefined,
      token: params.filters.token || undefined,
      request_id: params.filters.requestId || undefined,
      start_timestamp: localTimeToUnix(params.filters.startTime),
      end_timestamp: localTimeToUnix(params.filters.endTime),
    },
  })
  return response.data as ApiEnvelope<FullContentLogList>
}

export async function getFullContentLogDetail(
  requestId: string,
  signal?: AbortSignal,
  maxResponseBytes = 1024 * 1024
): Promise<ApiEnvelope<FullContentLogDetail>> {
  const response = await api.get(
    `/api/full-content-logs/${encodeURIComponent(requestId)}`,
    {
      signal,
      skipErrorHandler: true,
      skipBusinessError: true,
      params: { max_response_bytes: maxResponseBytes },
    }
  )
  return response.data as ApiEnvelope<FullContentLogDetail>
}

export async function getFullContentLogFiles(): Promise<
  ApiEnvelope<FullContentLogFileStats>
> {
  const response = await api.get('/api/full-content-logs/files')
  return response.data as ApiEnvelope<FullContentLogFileStats>
}

export async function getFullContentLogSegment(
  requestId: string,
  request: SavedContentSegmentRequest,
  signal: AbortSignal
): Promise<SavedContentSegment> {
  const auth = useAuthStore.getState().auth
  const actor = JSON.stringify([auth.user?.id, auth.user?.role, auth.session?.sid ?? null])
  const isCurrentScope = () => {
    const current = useAuthStore.getState().auth
    return !signal.aborted && actor === JSON.stringify([current.user?.id, current.user?.role, current.session?.sid ?? null])
  }
  const response = await api.get(`/api/full-content-logs/${encodeURIComponent(requestId)}/segments`, {
    signal,
    isCurrentScope,
    disableDuplicate: true,
    skipBusinessError: true,
    skipErrorHandler: true,
    params: {
      part: request.part,
      offset: request.offset,
      limit: request.limit ?? SAVED_SEGMENT_LIMIT,
      ...(request.snapshot ? { snapshot: request.snapshot } : {}),
    },
  })
  return parseSavedSegment(response.data, requestId, request)
}

export async function downloadFullContentLogFile(
  filename: string
): Promise<void> {
  const response = await api.get(
    `/api/full-content-logs/files/${encodeURIComponent(filename)}`,
    { responseType: 'blob' }
  )
  const url = URL.createObjectURL(response.data as Blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  URL.revokeObjectURL(url)
}

export async function deleteFullContentLogFile(
  filename: string
): Promise<ApiEnvelope<{ deleted_bytes: number }>> {
  const response = await api.delete(
    `/api/full-content-logs/files/${encodeURIComponent(filename)}`,
    { skipAuthRefresh: true }
  )
  return response.data as ApiEnvelope<{ deleted_bytes: number }>
}

export async function deleteAllFullContentLogFiles(): Promise<
  ApiEnvelope<{ deleted_count: number; freed_bytes: number }>
> {
  const response = await api.delete('/api/full-content-logs/files', { skipAuthRefresh: true })
  return response.data as ApiEnvelope<{
    deleted_count: number
    freed_bytes: number
  }>
}
