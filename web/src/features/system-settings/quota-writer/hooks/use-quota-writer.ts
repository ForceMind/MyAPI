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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import {
  applyQuotaWriterTransition,
  driveQuotaWriterDrains,
  getQuotaWriterStatus,
  getQuotaWriterTransitionPlan,
  getQuotaWriterTransitions,
} from '../api'
import type { QuotaWriterApplyRequest, QuotaWriterMode } from '../types'

const QUOTA_WRITER_QUERY_KEY = 'quota-writer'

export function useQuotaWriterStatus() {
  return useQuery({
    queryKey: [QUOTA_WRITER_QUERY_KEY, 'status'],
    queryFn: async () => {
      const res = await getQuotaWriterStatus()
      return res.data ?? null
    },
  })
}

export function useQuotaWriterTransitionPlan(target: QuotaWriterMode | null) {
  return useQuery({
    queryKey: [QUOTA_WRITER_QUERY_KEY, 'plan', target],
    enabled: target != null,
    queryFn: async () => {
      const res = await getQuotaWriterTransitionPlan(target as QuotaWriterMode)
      return res.data ?? null
    },
  })
}

export function useQuotaWriterTransitions(page: number, pageSize: number) {
  return useQuery({
    queryKey: [QUOTA_WRITER_QUERY_KEY, 'transitions', page, pageSize],
    queryFn: async () => {
      const res = await getQuotaWriterTransitions(page, pageSize)
      return res.data ?? null
    },
  })
}

/**
 * The mutations deliberately carry no toast side effects: callers receive the
 * raw result or axios error through per-call `mutate` callbacks so 409
 * conflicts (missing audit checks) and drain reports can be rendered inline.
 * Every quota-writer query is invalidated on success.
 */
export function useApplyQuotaWriterTransition() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (request: QuotaWriterApplyRequest) =>
      applyQuotaWriterTransition(request),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QUOTA_WRITER_QUERY_KEY] })
    },
  })
}

export function useDriveQuotaWriterDrains() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (budget: number) => driveQuotaWriterDrains(budget),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QUOTA_WRITER_QUERY_KEY] })
    },
  })
}
