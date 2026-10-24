/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery } from '@tanstack/react-query'

import { useAuthStore } from '@/stores/auth-store'

import { getAllLogs, getUserLogs } from '../api'
import { usageLogSchema, type UsageLog } from '../data/schema'
import { isRequestLog } from '../lib/request-facts'

const LOOKUP_PAGE_SIZE = 10

/**
 * Finds the call record of one Request ID with the existing log list
 * endpoint. The backend matches `request_id` exactly and, without a time
 * range, across all dates, so a pasted ID from any day resolves. One request
 * can leave more than one row (for example an error and its refund); the
 * newest request row is the one shown, and `count` reports the rest.
 */
export function useRequestLookup(options: {
  requestId: string | undefined
  isAdmin: boolean
  enabled: boolean
}) {
  const userId = useAuthStore((state) => state.auth.user?.id ?? null)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const requestId = options.requestId?.trim() || undefined
  const query = useQuery({
    queryKey: [
      'logs',
      'request-lookup',
      options.isAdmin,
      userId,
      sessionId,
      requestId,
    ],
    queryFn: async () => {
      const params = {
        p: 1,
        page_size: LOOKUP_PAGE_SIZE,
        request_id: requestId,
      }
      const response = options.isAdmin
        ? await getAllLogs(params)
        : await getUserLogs(params)
      if (!response?.success || !Array.isArray(response.data?.items)) {
        throw new Error(response?.message || 'Request lookup failed')
      }
      const rows = usageLogSchema
        .array()
        .parse(response.data.items)
        // The endpoint filters exactly; never show a row for another ID.
        .filter((row) => row.request_id === requestId)
        .sort((a, b) => b.created_at - a.created_at || b.id - a.id)
      const log: UsageLog | undefined = rows.find(isRequestLog) ?? rows[0]
      return { log, count: rows.length }
    },
    enabled: options.enabled && Boolean(requestId) && userId !== null,
    retry: false,
    refetchOnWindowFocus: false,
  })
  return query
}
