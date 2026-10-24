/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery } from '@tanstack/react-query'

import {
  getChannel,
  getChannels,
  searchChannels,
} from '@/features/channels/api'
import { channelsQueryKeys } from '@/features/channels/lib/channel-actions'
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

export function useConnections(search: ConnectionsSearch) {
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
  // Testing contacts the provider; it needs the operate grant both in the
  // signed-in identity and in the fresh self read.
  const canOperate = Boolean(
    canRead &&
    hasPermission(user, 'channel', 'operate') &&
    hasPermission(self.data, 'channel', 'operate')
  )
  const params = {
    p: search.page ?? 1,
    page_size: CONNECTION_PAGE_SIZE,
    status: search.status,
    sort_by: 'id' as const,
    sort_order: 'desc' as const,
  }
  const list = useQuery({
    queryKey: [
      ...channelsQueryKeys.lists(),
      'workspace',
      ...identity,
      canRead,
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
    enabled: canRead,
    retry: false,
    refetchOnWindowFocus: false,
  })
  const selectedId = search.connection
  const detail = useQuery({
    queryKey: [
      ...channelsQueryKeys.detail(selectedId ?? -1),
      'workspace',
      ...identity,
      canRead,
    ],
    queryFn: async ({ signal }) => {
      if (selectedId === undefined) throw new Error('Connection not selected')
      return parseConnection(
        await getChannel(selectedId, {
          signal,
          disableDuplicate: true,
          skipErrorHandler: true,
        }),
        selectedId
      )
    },
    enabled: canRead && selectedId !== undefined,
    retry: false,
    refetchOnWindowFocus: false,
  })
  return {
    self,
    list,
    detail,
    canRead,
    canWrite,
    canOperate,
    identity: identity.join(':'),
  }
}
