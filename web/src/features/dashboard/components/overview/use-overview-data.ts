/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import z from 'zod'

import { getChannels } from '@/features/channels/api'
import { useBodyCaptureStatus } from '@/features/full-content-logs/hooks/use-body-capture-status'
import { useUpstreamAccounts } from '@/features/personal-upstream/use-upstream-accounts'
import { getPlaygroundKeys } from '@/features/playground/api'
import { getAllLogs, getUserLogs } from '@/features/usage-logs/api'
import { usageLogSchema } from '@/features/usage-logs/data/schema'
import { isRequestLog } from '@/features/usage-logs/lib/request-facts'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

/** Rows read for the health strip, attention list and recent requests. */
export const OVERVIEW_SAMPLE_SIZE = 100
const DAY_SECONDS = 24 * 60 * 60
const totalSchema = z.number().int().nonnegative()

/**
 * Everything the overview reads, with the existing list endpoints only:
 * the latest request rows of the last 24 hours (one read), exact 24-hour
 * request counts (two one-row reads), and for channel readers the enabled /
 * total connection counts and saved account windows. Nothing here contacts
 * a provider. Query keys carry the signed-in identity, so another account's
 * data is never shown after a switch.
 */
export function useOverviewData() {
  const user = useAuthStore((state) => state.auth.user)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const isAdmin = (user?.role ?? 0) >= ROLE.ADMIN
  const identity = [user?.id, user?.role, sessionId] as const
  // Fixed per visit so the query keys stay stable while the page is open.
  const [since] = useState(() => Math.floor(Date.now() / 1000) - DAY_SECONDS)
  const fetchLogs = isAdmin ? getAllLogs : getUserLogs

  const sample = useQuery({
    queryKey: ['overview', 'requests', isAdmin, ...identity, since],
    queryFn: async () => {
      const response = await fetchLogs({
        p: 1,
        page_size: OVERVIEW_SAMPLE_SIZE,
        start_timestamp: since,
      })
      if (!response?.success || !Array.isArray(response.data?.items)) {
        throw new Error('Recent requests unavailable')
      }
      return usageLogSchema
        .array()
        .parse(response.data.items)
        .filter(
          (row) => isRequestLog(row) && (isAdmin || row.user_id === user?.id)
        )
    },
    enabled: Boolean(user),
    retry: false,
    refetchOnWindowFocus: false,
  })

  const counts = useQuery({
    queryKey: ['overview', 'request-counts', isAdmin, ...identity, since],
    queryFn: async () => {
      const [consumed, failed] = await Promise.all(
        [2, 5].map((type) =>
          fetchLogs({ p: 1, page_size: 1, type, start_timestamp: since })
        )
      )
      const totals = [consumed, failed].map((response) =>
        response?.success ? totalSchema.safeParse(response.data?.total) : null
      )
      if (totals.some((total) => !total?.success)) {
        throw new Error('Request counts unavailable')
      }
      return totals.reduce((sum, total) => sum + (total?.data ?? 0), 0)
    },
    enabled: Boolean(user),
    retry: false,
    refetchOnWindowFocus: false,
  })

  const upstream = useUpstreamAccounts()
  const canReadChannels = upstream.canRead
  const connections = useQuery({
    queryKey: ['overview', 'connections', ...identity, canReadChannels],
    queryFn: async ({ signal }) => {
      const config = { signal, skipErrorHandler: true }
      const [all, enabled] = await Promise.all([
        getChannels({ p: 1, page_size: 1 }, config),
        getChannels({ p: 1, page_size: 1, status: 'enabled' }, config),
      ])
      const total = totalSchema.safeParse(all.data?.total)
      const available = totalSchema.safeParse(enabled.data?.total)
      if (
        !all.success ||
        !enabled.success ||
        !total.success ||
        !available.success ||
        available.data > total.data
      ) {
        // A malformed total is unavailable, never a fabricated zero.
        throw new Error('Connection totals unavailable')
      }
      return { total: total.data, enabled: available.data }
    },
    enabled: canReadChannels,
    retry: false,
    refetchOnWindowFocus: false,
  })

  // The owner-scoped safe Key list (no Key objects or secrets), as the
  // Playground reads it; only its length is used here.
  const keys = useQuery({
    queryKey: ['overview', 'key-count', ...identity],
    queryFn: async ({ signal }) => (await getPlaygroundKeys({ signal })).length,
    enabled: Boolean(user),
    retry: false,
    refetchOnWindowFocus: false,
  })

  const capture = useBodyCaptureStatus()

  return {
    user,
    isAdmin,
    since,
    sample,
    counts,
    upstream,
    canReadChannels,
    connections,
    keys,
    capture,
  }
}

export type OverviewData = ReturnType<typeof useOverviewData>
