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

import type {
  ApiEnvelope,
  FullContentLogDetail,
  FullContentLogFileStats,
  FullContentLogList,
  FullContentLogListParams,
} from './types'

function localTimeToUnix(value: string): number | undefined {
  if (!value) return undefined
  const timestamp = new Date(value).getTime()
  if (!Number.isFinite(timestamp)) return undefined
  return Math.floor(timestamp / 1000)
}

export async function getFullContentLogs(
  params: FullContentLogListParams
): Promise<ApiEnvelope<FullContentLogList>> {
  const response = await api.get('/api/full-content-logs', {
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
  requestId: string
): Promise<ApiEnvelope<FullContentLogDetail>> {
  const response = await api.get(
    `/api/full-content-logs/${encodeURIComponent(requestId)}`,
    { params: { max_response_bytes: 1024 * 1024 } }
  )
  return response.data as ApiEnvelope<FullContentLogDetail>
}

export async function getFullContentLogFiles(): Promise<
  ApiEnvelope<FullContentLogFileStats>
> {
  const response = await api.get('/api/full-content-logs/files')
  return response.data as ApiEnvelope<FullContentLogFileStats>
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
    `/api/full-content-logs/files/${encodeURIComponent(filename)}`
  )
  return response.data as ApiEnvelope<{ deleted_bytes: number }>
}

export async function deleteAllFullContentLogFiles(): Promise<
  ApiEnvelope<{ deleted_count: number; freed_bytes: number }>
> {
  const response = await api.delete('/api/full-content-logs/files')
  return response.data as ApiEnvelope<{
    deleted_count: number
    freed_bytes: number
  }>
}
