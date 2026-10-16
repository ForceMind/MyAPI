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
import { useCallback, useEffect, useMemo, useRef } from 'react'

import { useAuthStore } from '@/stores/auth-store'

type CurrentResult<T> = {
  current: true
  value: T
}

type StaleResult = {
  current: false
}

export type SecurityRequestResult<T> = CurrentResult<T> | StaleResult

type SecurityIdentity = {
  userId: number | null
  role: number | null
  sid: string | null
}

function getSecurityIdentity(): SecurityIdentity {
  const { user, session } = useAuthStore.getState().auth
  return {
    userId: user?.id ?? null,
    role: user?.role ?? null,
    sid: session?.sid ?? null,
  }
}

function isSameSecurityIdentity(expected: SecurityIdentity): boolean {
  const current = getSecurityIdentity()
  return (
    current.userId === expected.userId &&
    current.role === expected.role &&
    current.sid === expected.sid
  )
}

export function useSecurityRequestScope(scopeKey: string) {
  const active = useRef(true)
  const generation = useRef(0)
  const pending = useRef<AbortController | null>(null)
  const identity = useRef(getSecurityIdentity())

  const cancel = useCallback(() => {
    generation.current += 1
    pending.current?.abort()
    pending.current = null
  }, [])

  useEffect(() => {
    identity.current = getSecurityIdentity()
    active.current = true
    return () => {
      active.current = false
      cancel()
    }
  }, [cancel, scopeKey])

  const run = useCallback(
    async <T>(request: (signal: AbortSignal) => Promise<T>) => {
      cancel()
      const expectedIdentity = identity.current
      if (!active.current || !isSameSecurityIdentity(expectedIdentity)) {
        return { current: false } as StaleResult
      }

      const controller = new AbortController()
      const requestGeneration = generation.current
      pending.current = controller

      try {
        const value = await request(controller.signal)
        if (
          !active.current ||
          controller.signal.aborted ||
          generation.current !== requestGeneration ||
          !isSameSecurityIdentity(expectedIdentity)
        ) {
          return { current: false } as StaleResult
        }
        return { current: true, value } as CurrentResult<T>
      } catch (error) {
        if (
          !active.current ||
          controller.signal.aborted ||
          generation.current !== requestGeneration ||
          !isSameSecurityIdentity(expectedIdentity)
        ) {
          return { current: false } as StaleResult
        }
        throw error
      } finally {
        if (pending.current === controller) pending.current = null
      }
    },
    [cancel]
  )

  return useMemo(() => ({ run, cancel }), [cancel, run])
}
