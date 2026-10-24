/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery } from '@tanstack/react-query'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getFullContentLogs } from '../api'

const NO_FILTERS = {
  model: '',
  token: '',
  requestId: '',
  startTime: '',
  endTime: '',
}

/**
 * Whether the server currently saves request bodies (FULL_CONTENT_LOG_ENABLED).
 * Only administrators can read saved bodies, so nobody else asks. The answer
 * is `undefined` until a successful read: an unknown state is never shown as
 * "off".
 */
export function useBodyCaptureStatus(enabled = true) {
  const userId = useAuthStore((state) => state.auth.user?.id ?? null)
  const role = useAuthStore((state) => state.auth.user?.role ?? 0)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const canRead = userId !== null && role >= ROLE.ADMIN
  const query = useQuery({
    queryKey: ['full-content-logs', 'capture-status', userId, sessionId, role],
    queryFn: async ({ signal }) => {
      const response = await getFullContentLogs(
        { page: 1, pageSize: 1, filters: NO_FILTERS },
        signal
      )
      if (!response.success || !response.data) {
        throw new Error('Capture status unavailable')
      }
      return response.data.enabled === true
    },
    enabled: enabled && canRead,
    retry: false,
    refetchOnWindowFocus: false,
    staleTime: 60_000,
  })
  return {
    canRead,
    capturing: query.isError ? undefined : query.data,
    query,
  }
}
