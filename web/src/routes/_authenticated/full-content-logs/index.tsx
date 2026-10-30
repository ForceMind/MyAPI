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

import { createFileRoute, redirect, useLocation } from '@tanstack/react-router'
import { useCallback } from 'react'

import { FullContentLogs } from '@/features/full-content-logs'
import {
  createFullContentLogListStateCache,
  type FullContentLogListState,
} from '@/features/full-content-logs/lib/list-state'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

export const Route = createFileRoute('/_authenticated/full-content-logs/')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()
    if ((auth.user?.role ?? ROLE.GUEST) < ROLE.ADMIN) {
      throw redirect({ to: '/403' })
    }
  },
  component: FullContentLogsRoute,
})

// History keys select bounded, memory-only control snapshots. No list controls
// or captured content are written to route URLs, history state, or browser storage.
const listStates = createFullContentLogListStateCache()
useAuthStore.subscribe((state, previous) => {
  if (
    state.auth.user?.id !== previous.auth.user?.id ||
    state.auth.session?.sid !== previous.auth.session?.sid ||
    (state.auth.user?.role ?? ROLE.GUEST) < ROLE.ADMIN
  ) {
    listStates.clear()
  }
})

function FullContentLogsRoute() {
  const entryKey = useLocation({
    select: (location) =>
      location.state.__TSR_key ?? location.state.key ?? 'initial',
  })
  const userId = useAuthStore((state) => state.auth.user?.id ?? null)
  const sessionId = useAuthStore((state) => state.auth.session?.sid ?? null)
  const canRead = useAuthStore((state) =>
    state.auth.user !== null && state.auth.user.role >= ROLE.ADMIN
  )
  const saveState = useCallback(
    (state: FullContentLogListState) => {
      listStates.write(entryKey, userId, sessionId, state)
    },
    [entryKey, userId, sessionId]
  )
  return (
    <FullContentLogs
      key={JSON.stringify([entryKey, userId, sessionId, canRead])}
      initialState={listStates.read(entryKey, userId, sessionId)}
      onStateChange={saveState}
    />
  )
}
