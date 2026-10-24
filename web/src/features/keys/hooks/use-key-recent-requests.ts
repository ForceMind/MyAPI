/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery } from '@tanstack/react-query'

import { getAllLogs, getUserLogs } from '@/features/usage-logs/api'
import { usageLogSchema } from '@/features/usage-logs/data/schema'
import { isRequestLog } from '@/features/usage-logs/lib/request-facts'
import { useAuthStore } from '@/stores/auth-store'

export const KEY_RECENT_REQUEST_LIMIT = 20

/**
 * The latest request records of one Key, matched by its saved ID (a rename
 * keeps its history). No time range applies, so a quiet Key still shows its
 * last requests. Administrators read the admin projection so the diagnosis
 * tab can show routing attempts.
 */
export function useKeyRecentRequests(keyId: number, isAdmin: boolean) {
  const userId = useAuthStore((state) => state.auth.user?.id ?? null)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  return useQuery({
    queryKey: ['logs', 'key-recent', isAdmin, userId, sessionId, keyId],
    queryFn: async () => {
      const params = {
        p: 1,
        page_size: KEY_RECENT_REQUEST_LIMIT,
        token_id: keyId,
      }
      const response = isAdmin
        ? await getAllLogs(params)
        : await getUserLogs(params)
      if (!response?.success || !Array.isArray(response.data?.items)) {
        throw new Error('Key requests unavailable')
      }
      // Keys belong to the signed-in user: a row for another user or Key in
      // the response is never shown as this Key's request.
      return usageLogSchema
        .array()
        .parse(response.data.items)
        .filter(
          (row) =>
            row.user_id === userId &&
            row.token_id === keyId &&
            isRequestLog(row)
        )
    },
    enabled: userId !== null,
    retry: false,
    refetchOnWindowFocus: false,
  })
}
