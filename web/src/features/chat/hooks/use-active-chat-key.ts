/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from '@tanstack/react-query'
import { CanceledError } from 'axios'

import { fetchTokenKey, getApiKeys } from '@/features/keys/api'
import { API_KEY_STATUS } from '@/features/keys/constants'
import { useAuthStore } from '@/stores/auth-store'

export async function fetchActiveChatKey(signal?: AbortSignal) {
  const initial = useAuthStore.getState().auth
  const identity = `${initial.user?.id ?? null}:${initial.user?.role ?? 0}:${initial.session?.sid ?? null}`
  const assertScope = () => {
    const { auth } = useAuthStore.getState()
    if (signal?.aborted || identity !== `${auth.user?.id ?? null}:${auth.user?.role ?? 0}:${auth.session?.sid ?? null}`) throw new CanceledError('Chat key request scope changed')
  }
  assertScope()
  const result = await getApiKeys({ p: 1, size: 50 })
  assertScope()
  if (!result.success) {
    throw new Error(result.message || 'Failed to load API keys')
  }

  const items = result.data?.items ?? []
  const active = items.find((item) => item.status === API_KEY_STATUS.ENABLED)
  if (!active) {
    throw new Error('No enabled API keys found. Create or enable one first.')
  }

  const keyResult = await fetchTokenKey(active.id, { signal, skipAuthRefresh: true })
  assertScope()
  if (!keyResult.success || !keyResult.data?.key) {
    throw new Error(keyResult.message || 'Failed to load API key')
  }

  return `sk-${keyResult.data.key}`
}

/**
 * Get the currently active API key for chat links
 */
export function useActiveChatKey(enabled: boolean) {
  const userId = useAuthStore((state) => state.auth.user?.id)
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)

  return useQuery({
    queryKey: ['chat-active-key', identity],
    queryFn: ({ signal }) => fetchActiveChatKey(signal),
    enabled: enabled && Boolean(userId),
    staleTime: 5 * 60 * 1000,
    gcTime: 10 * 60 * 1000,
  })
}
