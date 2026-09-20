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
  QuotaWriterApiEnvelope,
  QuotaWriterApplyRequest,
  QuotaWriterDrainReport,
  QuotaWriterMode,
  QuotaWriterStatus,
  QuotaWriterTransition,
  QuotaWriterTransitionPage,
  QuotaWriterTransitionPlan,
} from './types'

export async function getQuotaWriterStatus() {
  const res = await api.get<QuotaWriterApiEnvelope<QuotaWriterStatus>>(
    '/api/quota-writer/status'
  )
  return res.data
}

export async function getQuotaWriterTransitionPlan(target: QuotaWriterMode) {
  const res = await api.get<QuotaWriterApiEnvelope<QuotaWriterTransitionPlan>>(
    '/api/quota-writer/plan',
    { params: { target } }
  )
  return res.data
}

/**
 * Apply is error-handled by the caller: a 409 response carries the missing
 * audit checks (and possibly the persisted failed evidence row) that must be
 * rendered inline instead of a generic toast.
 */
export async function applyQuotaWriterTransition(
  request: QuotaWriterApplyRequest
) {
  const res = await api.post<QuotaWriterApiEnvelope<QuotaWriterTransition>>(
    '/api/quota-writer/apply',
    request,
    { skipErrorHandler: true }
  )
  return res.data
}

export async function getQuotaWriterTransitions(
  page: number,
  pageSize: number
) {
  const res = await api.get<QuotaWriterApiEnvelope<QuotaWriterTransitionPage>>(
    '/api/quota-writer/transitions',
    { params: { p: page, page_size: pageSize } }
  )
  return res.data
}

export async function driveQuotaWriterDrains(budget: number) {
  const res = await api.post<QuotaWriterApiEnvelope<QuotaWriterDrainReport>>(
    '/api/quota-writer/drain',
    null,
    { params: { budget }, skipErrorHandler: true }
  )
  return res.data
}
