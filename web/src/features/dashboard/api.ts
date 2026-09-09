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
import type { TimeGranularity } from '@/lib/time'

import type {
  FlowQuotaDataItem,
  QuotaDataItem,
  UptimeGroupResult,
} from './types'

// ============================================================================
// Dashboard APIs
// ============================================================================

// ----------------------------------------------------------------------------
// Quota & Usage Data
// ----------------------------------------------------------------------------

// Get user quota data within a time range
// Admin users get all users' data by default.
export async function getUserQuotaDates(
  params: {
    start_timestamp: number
    end_timestamp: number
    granularity?: TimeGranularity
    default_time?: TimeGranularity
    timezone_offset?: number
    username?: string
  },
  isAdmin = false
) {
  const endpoint = isAdmin ? '/api/data' : '/api/data/self'
  const res = await api.get<{
    success: boolean
    data?: QuotaDataItem[]
    message?: string
  }>(endpoint, { params })
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to load dashboard data')
  }
  return res.data
}

export interface RecordedRequestSummary {
  start_timestamp: number
  end_timestamp: number
  total_requests: number
  successful_requests: number
  failed_requests: number
  /** Percentage in the inclusive 0–100 range, or null when there are no requests. */
  success_rate: number | null
  coverage?: {
    complete: boolean
    consume_logs_enabled: boolean
    error_logs_enabled: boolean
    reason?: string
    identified_requests: number
    unidentified_log_rows: number
    window_semantics?: string
    deduplication?: string
  }
}

/**
 * Returns deduplicated, recorded API request outcomes for the active scope.
 * Administrators receive the global aggregate; other users receive their own.
 */
export async function getRecordedRequestSummary(
  params: { start_timestamp: number; end_timestamp: number },
  isAdmin = false
): Promise<RecordedRequestSummary> {
  const endpoint = isAdmin
    ? '/api/log/request-summary'
    : '/api/log/self/request-summary'
  const res = await api.get<{
    success: boolean
    data?: RecordedRequestSummary
    message?: string
  }>(endpoint, { params })
  if (!res.data.success || !res.data.data) {
    throw new Error(res.data.message || 'Failed to load request summary')
  }
  return res.data.data
}

// ----------------------------------------------------------------------------
// System Monitoring
// ----------------------------------------------------------------------------

export async function getUserQuotaDataByUsers(params: {
  start_timestamp: number
  end_timestamp: number
  granularity?: TimeGranularity
  default_time?: TimeGranularity
  timezone_offset?: number
}) {
  const res = await api.get<{
    success: boolean
    data?: QuotaDataItem[]
    message?: string
  }>('/api/data/users', { params })
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to load dashboard data')
  }
  return res.data
}

export async function getFlowQuotaDates(
  params: {
    start_timestamp: number
    end_timestamp: number
    granularity?: TimeGranularity
    default_time?: TimeGranularity
    timezone_offset?: number
    username?: string
  },
  isAdmin = false
) {
  const endpoint = isAdmin ? '/api/data/flow' : '/api/data/flow/self'
  const res = await api.get<{
    success: boolean
    data?: FlowQuotaDataItem[]
    message?: string
  }>(endpoint, { params })
  return res.data
}

// Get uptime monitoring status for all services
export async function getUptimeStatus() {
  const res = await api.get<{ success: boolean; data: UptimeGroupResult[] }>(
    '/api/uptime/status'
  )
  return res.data
}
