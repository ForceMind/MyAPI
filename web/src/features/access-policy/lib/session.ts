import { useCallback, useEffect, useRef } from 'react'

import { useAuthStore } from '@/stores/auth-store'

export type AccessSession = { userId: number; role: number; sid: string | null }

export function useAccessSession(scope: AccessSession) {
  const mounted = useRef(true)
  const requests = useRef(new Set<AbortController>())
  useEffect(() => {
    mounted.current = true
    const activeRequests = requests.current
    return () => {
      mounted.current = false
      for (const request of activeRequests) request.abort()
      activeRequests.clear()
    }
  }, [])
  const isCurrent = useCallback((): boolean => {
    const auth = useAuthStore.getState().auth
    return (
      mounted.current &&
      auth.user?.id === scope.userId &&
      auth.user.role === scope.role &&
      (auth.session?.sid ?? null) === scope.sid
    )
  }, [scope.userId, scope.role, scope.sid])
  const run = useCallback(
    async <T>(request: (signal: AbortSignal) => Promise<T>): Promise<T> => {
      if (!isCurrent()) throw new Error('Session changed')
      const controller = new AbortController()
      requests.current.add(controller)
      try {
        const result = await request(controller.signal)
        if (!isCurrent() || controller.signal.aborted) {
          throw new Error('Session changed')
        }
        return result
      } finally {
        requests.current.delete(controller)
      }
    },
    [isCurrent]
  )
  return { isCurrent, run }
}
