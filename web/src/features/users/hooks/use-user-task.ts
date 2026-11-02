import { useCallback, useLayoutEffect, useMemo, useRef } from 'react'

import { useAuthStore } from '@/stores/auth-store'

/** Bind one user-management task to its actor, session and selected user. */
export function useUserTask(open: boolean, userId?: number | null) {
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)
  const generation = useRef<object>({})
  const active = useRef(false)
  const pending = useRef(false)
  useLayoutEffect(() => {
    generation.current = {}
    active.current = open
    pending.current = false
    return () => { generation.current = {}; active.current = false; pending.current = false }
  }, [open, userId, identity])
  const capture = useCallback(() => {
    const token = generation.current
    return {
      isCurrent: () => {
        const { auth } = useAuthStore.getState()
        return active.current && token === generation.current && identity === `${auth.user?.id ?? null}:${auth.user?.role ?? 0}:${auth.session?.sid ?? null}`
      },
      finish: () => { if (token === generation.current) pending.current = false },
    }
  }, [identity])
  const begin = useCallback(() => {
    const operation = capture()
    if (pending.current || !operation.isCurrent()) return null
    pending.current = true
    return operation
  }, [capture])
  const isPending = useCallback(() => pending.current, [])
  return useMemo(() => ({ identity, capture, begin, isPending }), [identity, capture, begin, isPending])
}
