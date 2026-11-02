import { useCallback, useLayoutEffect, useRef } from 'react'

import { useAuthStore } from '@/stores/auth-store'

/** Keep each open deployment task and its admission lock inside one login scope. */
export function useDeploymentTask(open: boolean, deploymentId?: string | number | null) {
  const identity = useAuthStore((state) => `${state.auth.user?.id ?? null}:${state.auth.user?.role ?? 0}:${state.auth.session?.sid ?? null}`)
  const generation = useRef<object>({})
  const active = useRef(false)
  const pending = useRef(false)

  useLayoutEffect(() => {
    generation.current = {}
    active.current = open
    pending.current = false
    return () => {
      active.current = false
      generation.current = {}
      pending.current = false
    }
  }, [open, deploymentId, identity])

  const begin = useCallback(() => {
    if (!active.current || pending.current) return null
    const { auth } = useAuthStore.getState()
    if (identity !== `${auth.user?.id ?? null}:${auth.user?.role ?? 0}:${auth.session?.sid ?? null}`) return null
    const token = generation.current
    pending.current = true
    return {
      isCurrent: () => {
        const { auth } = useAuthStore.getState()
        const currentIdentity = `${auth.user?.id ?? null}:${auth.user?.role ?? 0}:${auth.session?.sid ?? null}`
        return active.current && token === generation.current && identity === currentIdentity
      },
      finish: () => { if (token === generation.current) pending.current = false },
    }
  }, [identity])

  const isPending = useCallback(() => pending.current, [])
  return { identity, begin, isPending }
}
