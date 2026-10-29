/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery } from '@tanstack/react-query'
import { useCallback, useEffect, useState } from 'react'
import z from 'zod'

import {
  getChannel,
  getChannelQuotaChanges,
  getChannels,
  searchChannels,
} from '@/features/channels/api'
import { channelsQueryKeys } from '@/features/channels/lib/channel-actions'
import { trafficBy } from '@/features/dashboard/lib/overview-flow'
import { getAllLogs } from '@/features/usage-logs/api'
import { usageLogSchema } from '@/features/usage-logs/data/schema'
import { isRequestLog } from '@/features/usage-logs/lib/request-facts'
import { hasPermission } from '@/lib/admin-permissions'
import { api } from '@/lib/api'
import { parseFreshSelfUser } from '@/lib/fresh-self-user'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  CONNECTION_PAGE_SIZE,
  parseConnection,
  parseConnections,
  type ConnectionsSearch,
} from './connection-state'

/** Latest call records read to tell which connections are in use. */
const TRAFFIC_SAMPLE_SIZE = 100
const CONNECTION_RECENT_REQUEST_LIMIT = 20
const DAY_SECONDS = 24 * 60 * 60

/**
 * Who may read, edit and operate connections, from the signed-in identity
 * and a fresh `/api/user/self` read: a stale grant never starts channel
 * reads. `identity` keys every query, so another account's data is never
 * shown after a switch.
 */
export function useConnectionAccess() {
  const user = useAuthStore((state) => state.auth.user)
  const sid = useAuthStore((state) => state.auth.session?.sid)
  const declaredRead = Boolean(
    user && user.role >= ROLE.ADMIN && hasPermission(user, 'channel', 'read')
  )
  const declaredWrite = Boolean(
    declaredRead && hasPermission(user, 'channel', 'write')
  )
  const identity = [user?.id, sid, user?.role, declaredRead, declaredWrite]
  const self = useQuery({
    queryKey: ['connections-self', ...identity],
    queryFn: async ({ signal }) => {
      const response = await api.get('/api/user/self', {
        signal,
        disableDuplicate: true,
        skipErrorHandler: true,
      })
      return parseFreshSelfUser(response.data, user?.id ?? -1)
    },
    enabled: declaredRead,
    staleTime: 0,
    refetchOnMount: 'always',
    retry: false,
    refetchOnWindowFocus: false,
  })
  const canRead = Boolean(
    declaredRead &&
    self.isSuccess &&
    !self.isFetching &&
    !self.isError &&
    self.data.role >= ROLE.ADMIN &&
    hasPermission(self.data, 'channel', 'read')
  )
  const canWrite = Boolean(
    canRead && declaredWrite && hasPermission(self.data, 'channel', 'write')
  )
  // Testing contacts the provider and switching changes routing; both need
  // the operate grant in the signed-in identity and in the fresh self read.
  const canOperate = Boolean(
    canRead &&
    hasPermission(user, 'channel', 'operate') &&
    hasPermission(self.data, 'channel', 'operate')
  )
  return {
    self,
    canRead,
    canWrite,
    canOperate,
    isRoot: canRead && self.data?.role === ROLE.SUPER_ADMIN,
    identity: identity.join(':'),
  }
}

export type ConnectionAccess = ReturnType<typeof useConnectionAccess>

/** One page of connections (saved configuration metadata only). */
export function useConnectionList(
  access: ConnectionAccess,
  search: ConnectionsSearch
) {
  const params = {
    p: search.page ?? 1,
    page_size: CONNECTION_PAGE_SIZE,
    status: search.status,
    sort_by: 'id' as const,
    sort_order: 'desc' as const,
  }
  return useQuery({
    queryKey: [
      ...channelsQueryKeys.lists(),
      'workspace',
      access.identity,
      access.canRead,
      params,
      search.filter,
    ],
    queryFn: async ({ signal }) => {
      const config = { signal, disableDuplicate: true, skipErrorHandler: true }
      return parseConnections(
        search.filter
          ? await searchChannels({ ...params, keyword: search.filter }, config)
          : await getChannels(params, config)
      )
    },
    enabled: access.canRead,
    retry: false,
    refetchOnWindowFocus: false,
  })
}

/** One connection's saved configuration; another ID in the reply is an error. */
export function useConnectionDetail(
  access: ConnectionAccess,
  connectionId: number
) {
  return useQuery({
    queryKey: [
      ...channelsQueryKeys.detail(connectionId),
      'workspace',
      access.identity,
      access.canRead,
    ],
    queryFn: async ({ signal }) =>
      parseConnection(
        await getChannel(connectionId, {
          signal,
          disableDuplicate: true,
          skipErrorHandler: true,
        }),
        connectionId
      ),
    enabled: access.canRead,
    retry: false,
    refetchOnWindowFocus: false,
  })
}

/**
 * Saved account window observations of the given connections (the native
 * percent windows of subscription accounts), in one read. Only stored
 * samples are read; no provider is contacted. A reply naming a connection
 * that was not asked for is an error, never a healthy empty result.
 */
export function useConnectionWindows(
  access: ConnectionAccess,
  connectionIds: number[]
) {
  const ids = [...new Set(connectionIds)].sort((a, b) => a - b)
  const [now, setNow] = useState(() => Date.now() / 1000)
  const enabled = access.canRead && ids.length > 0
  const query = useQuery({
    queryKey: ['connections-windows', access.identity, ids.join(',')],
    queryFn: async ({ signal }) => {
      const response = await getChannelQuotaChanges(
        {
          channel_ids: ids.join(','),
          range: '24h',
          limit: 100,
          sort: 'observed_desc',
        },
        { signal, disableDuplicate: true }
      )
      if (
        !response.success ||
        !response.data ||
        !Array.isArray(response.data.items) ||
        response.data.items.some(
          (item) =>
            !ids.includes(item.channel_id) || typeof item.name !== 'string'
        )
      ) {
        throw new Error('Connection windows unavailable')
      }
      return response.data.items.filter(
        (item) =>
          item.metric_type === 'codex_rate_limit' && item.unit === 'percent'
      )
    },
    enabled,
    retry: false,
    refetchOnWindowFocus: false,
  })
  const readSaved = query.refetch
  const refetch = useCallback(() => {
    setNow(Date.now() / 1000)
    return readSaved()
  }, [readSaved])
  useEffect(() => {
    if (!enabled) return
    const refreshVisible = () => {
      if (document.visibilityState !== 'visible') return
      // This query reads saved rows only; never refresh provider credentials,
      // run WHAM sampling or substitute the connection's Test action.
      void refetch()
    }
    const timer = window.setInterval(refreshVisible, 60_000)
    document.addEventListener('visibilitychange', refreshVisible)
    return () => {
      window.clearInterval(timer)
      document.removeEventListener('visibilitychange', refreshVisible)
    }
  }, [enabled, refetch])
  return { ...query, refetch, now }
}

/**
 * Which connections carried requests lately: the latest call records of
 * the last 24 hours (one bounded read of saved records), counted per
 * connection ID. `truncated` when more records exist than were read.
 */
export function useConnectionTraffic(access: ConnectionAccess) {
  // Fixed per visit so the query key stays stable while the page is open.
  const [since] = useState(() => Math.floor(Date.now() / 1000) - DAY_SECONDS)
  return useQuery({
    queryKey: ['logs', 'connection-traffic', access.identity, since],
    queryFn: async () => {
      const response = await getAllLogs({
        p: 1,
        page_size: TRAFFIC_SAMPLE_SIZE,
        start_timestamp: since,
      })
      if (!response?.success || !Array.isArray(response.data?.items)) {
        throw new Error('Connection traffic unavailable')
      }
      const rows = usageLogSchema
        .array()
        .parse(response.data.items)
        .filter(isRequestLog)
      return {
        traffic: trafficBy(rows, (row) => row.channel),
        truncated: response.data.total > response.data.items.length,
      }
    },
    enabled: access.canRead,
    retry: false,
    refetchOnWindowFocus: false,
  })
}

const totalSchema = z.number().int().nonnegative()

/**
 * Exact 24-hour figures of one connection from saved call records: request
 * records (consume and error rows) and request failures (`result=failed`:
 * no pending reviews, no rejections before sending). Each is a one-row
 * read of the list total; an unreadable total is unknown, never zero.
 */
export function useConnectionStats(
  access: ConnectionAccess,
  connectionId: number
) {
  const [since] = useState(() => Math.floor(Date.now() / 1000) - DAY_SECONDS)
  return useQuery({
    queryKey: [
      'logs',
      'connection-stats',
      access.identity,
      connectionId,
      since,
    ],
    queryFn: async () => {
      const base = {
        p: 1,
        page_size: 1,
        channel: connectionId,
        start_timestamp: since,
      }
      const responses = await Promise.all([
        getAllLogs({ ...base, type: 2 }),
        getAllLogs({ ...base, type: 5 }),
        getAllLogs({ ...base, result: 'failed' }),
      ])
      const [consumed, errors, failed] = responses.map((response) => {
        const total = totalSchema.safeParse(response?.data?.total)
        return response?.success && total.success ? total.data : null
      })
      return {
        requests:
          consumed === null || errors === null ? null : consumed + errors,
        failed,
        /** Start of the counted period, in seconds. */
        since,
      }
    },
    enabled: access.canRead,
    retry: false,
    refetchOnWindowFocus: false,
  })
}

/**
 * The latest request records routed to one connection (no time range, so a
 * quiet connection still shows its last requests).
 */
export function useConnectionRequests(
  access: ConnectionAccess,
  connectionId: number
) {
  return useQuery({
    queryKey: ['logs', 'connection-recent', access.identity, connectionId],
    queryFn: async () => {
      const response = await getAllLogs({
        p: 1,
        page_size: CONNECTION_RECENT_REQUEST_LIMIT,
        channel: connectionId,
      })
      if (!response?.success || !Array.isArray(response.data?.items)) {
        throw new Error('Connection requests unavailable')
      }
      // A row routed elsewhere is never shown as this connection's request.
      return usageLogSchema
        .array()
        .parse(response.data.items)
        .filter((row) => row.channel === connectionId && isRequestLog(row))
    },
    enabled: access.canRead,
    retry: false,
    refetchOnWindowFocus: false,
  })
}
