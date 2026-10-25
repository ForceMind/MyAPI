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
import { parseConnections } from '@/features/connections/connection-state'
import { useBodyCaptureStatus } from '@/features/full-content-logs/hooks/use-body-capture-status'
import { useUpstreamAccounts } from '@/features/personal-upstream/use-upstream-accounts'
import { getPlaygroundKeys } from '@/features/playground/api'
import { getAllLogs, getUserLogs } from '@/features/usage-logs/api'
import { usageLogSchema } from '@/features/usage-logs/data/schema'
import { isRequestLog } from '@/features/usage-logs/lib/request-facts'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { FLOW_NODE_LIMIT } from '../../lib/overview-flow'

/** Rows read for the flow board, the attention strip and the request table. */
export const OVERVIEW_SAMPLE_SIZE = 100
const DAY_SECONDS = 24 * 60 * 60
const totalSchema = z.number().int().nonnegative()

/**
 * Everything the overview reads, with the existing list endpoints only:
 * the latest call records of the last 24 hours (one bounded read, aggregated
 * per Key ID and connection ID in the browser), exact 24-hour request counts
 * (two one-row reads), the owner-scoped safe Key list, and for channel
 * readers the first connections by priority, the enabled total and the saved
 * account windows. Nothing here contacts a provider. Query keys carry the
 * signed-in identity, so another account's data is never shown after a
 * switch.
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
      const total = totalSchema.safeParse(response.data.total)
      const fetched = response.data.items.length
      const rows = usageLogSchema
        .array()
        .parse(response.data.items)
        .filter(
          (row) => isRequestLog(row) && (isAdmin || row.user_id === user?.id)
        )
      // More records exist in the 24 hours than this one page holds: the
      // sample then covers only the time since its oldest row.
      const truncated = total.success ? total.data > fetched : true
      return { rows, truncated }
    },
    enabled: Boolean(user),
    retry: false,
    refetchOnWindowFocus: false,
  })

  const counts = useQuery({
    queryKey: ['overview', 'request-counts', isAdmin, ...identity, since],
    queryFn: async () => {
      const base = { p: 1, page_size: 1, start_timestamp: since }
      const responses = await Promise.all([
        fetchLogs({ ...base, type: 2 }),
        fetchLogs({ ...base, type: 5 }),
        // The same rule as the call records' "request failures only" link.
        fetchLogs({ ...base, result: 'failed' }),
      ])
      const [consumed, errors, failed] = responses.map((response) =>
        response?.success ? totalSchema.safeParse(response.data?.total) : null
      )
      if (!consumed?.success || !errors?.success) {
        throw new Error('Request counts unavailable')
      }
      return {
        requests: consumed.data + errors.data,
        /** Request failures; null when that total could not be read. */
        failed: failed?.success ? failed.data : null,
      }
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
      const [first, enabled] = await Promise.all([
        getChannels(
          {
            p: 1,
            page_size: FLOW_NODE_LIMIT,
            sort_by: 'priority',
            sort_order: 'desc',
          },
          config
        ),
        getChannels({ p: 1, page_size: 1, status: 'enabled' }, config),
      ])
      // Saved configuration metadata only; a malformed list or total is
      // unavailable, never a fabricated zero.
      const list = parseConnections(first)
      const available = totalSchema.safeParse(enabled.data?.total)
      if (
        !enabled.success ||
        !available.success ||
        available.data > list.total
      ) {
        throw new Error('Connection totals unavailable')
      }
      return {
        items: list.items,
        total: list.total,
        enabled: available.data,
      }
    },
    enabled: canReadChannels,
    retry: false,
    refetchOnWindowFocus: false,
  })

  // The owner-scoped safe Key list (no Key objects or secrets), as the
  // Playground reads it.
  const keys = useQuery({
    queryKey: ['overview', 'keys', ...identity],
    queryFn: async ({ signal }) =>
      (await getPlaygroundKeys({ signal })).map((key) => ({
        id: key.id,
        name: key.name,
        status: key.status,
      })),
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
