/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { useTranslation } from 'react-i18next'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getFullContentLogDetail } from '../api'

/**
 * Reads one saved, redacted request body. The query key carries the signed-in
 * identity so a tab that switches accounts never shows the previous body, and
 * nothing is read for non-administrators or before `enabled`. Reading a saved
 * body never resends the request.
 */
export function useFullContentLogDetail(
  requestId: string | null,
  enabled: boolean
) {
  const { t } = useTranslation()
  const userId = useAuthStore((state) => state.auth.user?.id ?? null)
  const role = useAuthStore((state) => state.auth.user?.role ?? 0)
  const canRead = userId !== null && role >= ROLE.ADMIN
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const detailQuery = useQuery({
    // Keep sensitive request/response bodies isolated when a tab switches
    // authenticated identities without a full page reload.
    queryKey: [
      'full-content-logs',
      'detail',
      requestId,
      userId,
      sessionId,
      role,
    ],
    queryFn: async ({ signal }) => {
      const response = await getFullContentLogDetail(requestId || '', signal)
      if (
        !response.success ||
        !response.data ||
        response.data.request_id !== requestId
      ) {
        throw new Error(response.message || t('Failed to load log detail'))
      }
      return response.data
    },
    enabled: enabled && Boolean(requestId) && canRead,
    retry: false,
  })
  const detail = canRead && !detailQuery.isError ? detailQuery.data : undefined
  const status = isAxiosError(detailQuery.error)
    ? detailQuery.error.response?.status
    : undefined
  let contentError =
    detailQuery.error instanceof Error
      ? detailQuery.error.message
      : t('Failed to load log detail')
  if (status === 404) {
    contentError = t(
      'No saved request content. It may not have been recorded or may have expired.'
    )
  } else if (status === 401 || status === 403) {
    contentError = t('Request content is available to administrators only.')
  }
  return {
    detailQuery,
    detail,
    contentError,
    canRead,
    missing: status === 404,
  }
}
