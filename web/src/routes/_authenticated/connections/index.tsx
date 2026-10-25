/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { createFileRoute, redirect } from '@tanstack/react-router'

import {
  connectionsSearchSchema,
  legacyConnectionLinkTarget,
} from '@/features/connections/connection-state'
import { Connections } from '@/features/connections/connections'
import { hasPermission } from '@/lib/admin-permissions'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

export const Route = createFileRoute('/_authenticated/connections/')({
  beforeLoad: ({ search }) => {
    const user = useAuthStore.getState().auth.user
    if (
      !user ||
      user.role < ROLE.ADMIN ||
      !hasPermission(user, 'channel', 'read')
    ) {
      throw redirect({ to: '/403' })
    }
    // Earlier inspector links now open the connection's page.
    const target = legacyConnectionLinkTarget(search)
    if (target) {
      throw redirect({
        to: '/connections/$connectionId',
        params: { connectionId: String(target.connectionId) },
        search: target.search,
        replace: true,
      })
    }
  },
  validateSearch: connectionsSearchSchema,
  component: Connections,
})
