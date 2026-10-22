/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { createFileRoute, redirect } from '@tanstack/react-router'

import { connectionsSearchSchema } from '@/features/connections/connection-state'
import { Connections } from '@/features/connections/connections'
import { hasPermission } from '@/lib/admin-permissions'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

export const Route = createFileRoute('/_authenticated/connections/')({
  beforeLoad: () => {
    const user = useAuthStore.getState().auth.user
    if (
      !user ||
      user.role < ROLE.ADMIN ||
      !hasPermission(user, 'channel', 'read')
    ) {
      throw redirect({ to: '/403' })
    }
  },
  validateSearch: connectionsSearchSchema,
  component: Connections,
})
