/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery } from '@tanstack/react-query'

import { useAuthStore } from '@/stores/auth-store'

import { getSubscriptionShare } from '../subscription-share-api'

/** The native-window shares one Key holds, as saved. */
export function useKeySubscriptionShare(keyId: number) {
  const userId = useAuthStore((state) => state.auth.user?.id)
  const role = useAuthStore((state) => state.auth.user?.role)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  return useQuery({
    queryKey: ['subscription-share', userId, role, sessionId, keyId],
    queryFn: ({ signal }) => getSubscriptionShare(keyId, signal),
    retry: false,
  })
}
