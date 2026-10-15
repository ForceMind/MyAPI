/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery } from '@tanstack/react-query'

import { getChannelQuotaChanges } from '@/features/channels/api'
import { parseFreshSelfUser } from '@/features/dashboard/components/personal-workbench/workbench-state'
import { hasPermission } from '@/lib/admin-permissions'
import { getSelf } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

export function useUpstreamAccounts() {
  const user = useAuthStore((state) => state.auth.user)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const identity = [user?.id, sessionId]
  const selfQuery = useQuery({
    queryKey: ['personal-console', 'self', ...identity],
    queryFn: async () => parseFreshSelfUser(await getSelf(), user?.id ?? -1),
    enabled: Boolean(user),
    retry: false,
    refetchOnWindowFocus: false,
  })
  const canRead = Boolean(
    selfQuery.isSuccess &&
    !selfQuery.isError &&
    selfQuery.data.role >= ROLE.ADMIN &&
    hasPermission(selfQuery.data, 'channel', 'read')
  )
  const accountsQuery = useQuery({
    queryKey: ['personal-console', 'upstream-windows', ...identity, canRead],
    queryFn: async ({ signal }) => {
      const response = await getChannelQuotaChanges(
        { range: '24h', limit: 100, sort: 'observed_desc' },
        { signal }
      )
      if (
        !response.success ||
        !response.data ||
        !Array.isArray(response.data.items)
      ) {
        throw new Error('Account windows unavailable')
      }
      return response.data
    },
    enabled: canRead,
    retry: false,
    refetchOnWindowFocus: false,
    refetchInterval: 60_000,
  })
  return { selfQuery, accountsQuery, canRead }
}
