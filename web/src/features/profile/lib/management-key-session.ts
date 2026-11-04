/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { ROLE } from '@/lib/roles'
import { useAuthStore, type AuthUser, type LoginSession } from '@/stores/auth-store'

export function managementKeySessionKey(auth: { user: AuthUser | null; session: LoginSession | null } = useAuthStore.getState().auth): string | null {
  if (!auth.user || !auth.session?.sid || (auth.user.role !== ROLE.ADMIN && auth.user.role !== ROLE.SUPER_ADMIN)) return null
  return JSON.stringify([auth.user.id, auth.user.role, auth.session.sid, auth.user.permissions?.admin_permissions ?? null])
}
