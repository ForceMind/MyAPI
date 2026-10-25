/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useQuery } from '@tanstack/react-query'

import { useAuthStore } from '@/stores/auth-store'

import { getApiKey } from '../api'

/**
 * One saved Key of the signed-in user, read by ID. A Key that is missing or
 * belongs to another account is an error, never another Key's data.
 * `refreshTrigger` rereads it after an action on the Keys page.
 */
export function useKeyDetail(keyId: number, refreshTrigger = 0) {
  const userId = useAuthStore((state) => state.auth.user?.id ?? null)
  const role = useAuthStore((state) => state.auth.user?.role ?? 0)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  return useQuery({
    queryKey: ['api-key', userId, role, sessionId, keyId, refreshTrigger],
    queryFn: async ({ signal }) => {
      const response = await getApiKey(keyId, signal)
      if (!response.success || response.data?.id !== keyId) {
        throw new Error('Key unavailable')
      }
      return response.data
    },
    enabled: userId !== null,
    retry: false,
    placeholderData: (previous) =>
      previous?.id === keyId ? previous : undefined,
  })
}
