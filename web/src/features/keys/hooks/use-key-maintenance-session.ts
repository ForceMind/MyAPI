/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useCallback, useEffect, useRef } from 'react'

import { useAuthStore } from '@/stores/auth-store'

export function useKeyMaintenanceSession(): () => boolean {
  const auth = useAuthStore.getState().auth
  const scope = useRef({
    userId: auth.user?.id,
    role: auth.user?.role,
    sid: auth.session?.sid ?? null,
  })
  const mounted = useRef(true)
  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])
  return useCallback(() => {
    const current = useAuthStore.getState().auth
    return (
      mounted.current &&
      current.user?.id === scope.current.userId &&
      current.user?.role === scope.current.role &&
      (current.session?.sid ?? null) === scope.current.sid
    )
  }, [])
}
