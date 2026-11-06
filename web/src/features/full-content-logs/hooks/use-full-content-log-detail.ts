/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { CanceledError } from 'axios'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getFullContentLogDetail } from '../api'
import { CONTENT_EXPANDED_LIMIT } from '../lib/content-preview'
import { denySavedContent, savedContentStatus } from '../lib/saved-content-access'

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
  const client = useQueryClient()
  const expandedKey = useMemo(() => ['full-content-logs', 'detail', requestId, userId, sessionId, role, 'expanded'] as const, [requestId, userId, sessionId, role])
  const scope = JSON.stringify(expandedKey)
  const [expandedScope, setExpandedScope] = useState<string | null>(null)
  const isCurrent = () => {
    const current = useAuthStore.getState().auth
    return current.user?.id === userId && current.user?.role === role &&
      (current.session?.sid ?? null) === sessionId && role >= ROLE.ADMIN
  }
  const readSavedDetail = async (signal: AbortSignal, bytes?: number) => {
    try {
      if (!isCurrent() || !requestId) throw new CanceledError('Saved-content scope changed')
      const response = bytes === undefined
        ? await getFullContentLogDetail(requestId, signal)
        : await getFullContentLogDetail(requestId, signal, bytes)
      if (!isCurrent() || signal.aborted) throw new CanceledError('Saved-content scope changed')
      if (!response.success || !response.data || response.data.request_id !== requestId ||
        typeof response.data.request_body !== 'string' || typeof response.data.response_body !== 'string') {
        throw new Error(response.message || t('Failed to load log detail'))
      }
      return response.data
    } catch (error) {
      const status = savedContentStatus(error)
      if (requestId && userId !== null && isCurrent() && !signal.aborted && (status === 401 || status === 403)) {
        await denySavedContent(client, requestId, { id: userId, role, sid: sessionId }, status)
      }
      throw error
    }
  }
  const previewQuery = useQuery({
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
    queryFn: ({ signal }) => readSavedDetail(signal),
    enabled: enabled && Boolean(requestId) && canRead,
    retry: false,
  })
  const expandedQuery = useQuery({
    queryKey: expandedKey,
    enabled: false,
    queryFn: ({ signal }) => readSavedDetail(signal, CONTENT_EXPANDED_LIMIT),
    retry: false,
    refetchOnWindowFocus: false,
  })
  useEffect(() => {
    // An earlier actor/request's explicit expansion is not a standing read grant.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setExpandedScope(null)
    return () => { void client.cancelQueries({ queryKey: expandedKey, exact: true }) }
  }, [client, expandedKey])
  const expansionRequested = expandedScope === scope
  const expandedStatus = savedContentStatus(expandedQuery.error)
  const expansionDenied = expandedStatus === 401 || expandedStatus === 403
  let detailQuery = previewQuery
  if (!previewQuery.isError && expansionRequested && (expandedQuery.isSuccess || expansionDenied)) detailQuery = expandedQuery
  const detail = canRead && !detailQuery.isError ? detailQuery.data : undefined
  const status = savedContentStatus(detailQuery.error)
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
    expanded: expansionRequested && expandedQuery.isSuccess && !previewQuery.isError,
    expanding: expansionRequested && expandedQuery.isFetching,
    expansionError: expansionRequested && expandedQuery.isError ? expandedQuery.error : null,
    readExpanded: () => {
      if (!enabled || !canRead || !requestId || !isCurrent()) return
      setExpandedScope(scope)
      void expandedQuery.refetch()
    },
  }
}
