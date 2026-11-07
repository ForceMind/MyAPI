/* Copyright (C) 2026 ForceMind. AGPL-3.0-or-later. */
import { useLocation, useRouter } from '@tanstack/react-router'
import { useCallback, useEffect, type RefObject } from 'react'

import { useAuthStore } from '@/stores/auth-store'

type RoutingReturnPoint = {
  actor: string
  connectionId: number
  top: number
  left: number
  departed: boolean
}

function routingActor(
  auth: ReturnType<typeof useAuthStore.getState>['auth']
): string {
  return JSON.stringify([
    auth.user?.id ?? null,
    auth.user?.role ?? null,
    auth.session?.sid ?? null,
  ])
}

// Per history entry, bounded and memory-only. No address, filter, model list or
// credential is retained; the router already owns the original search state.
const returnPoints = new Map<string, RoutingReturnPoint>()
useAuthStore.subscribe((state, previous) => {
  if (routingActor(state.auth) !== routingActor(previous.auth)) {
    returnPoints.clear()
  }
})

/** The node calls capture only for an unmodified same-tab routing Link. */
export function useOverviewRoutingReturn(options: {
  enabled: boolean
  pending: boolean
  failed: boolean
  items: readonly { id: number }[] | undefined
  root: RefObject<HTMLElement | null>
}): (connectionId: number) => void {
  const router = useRouter()
  const actor = useAuthStore((state) => routingActor(state.auth))
  const entry = useLocation({
    select: (location) => location.state.__TSR_key ?? location.state.key,
  })

  useEffect(() => {
    return () => {
      if (!entry || routingActor(useAuthStore.getState().auth) !== actor) {
        return
      }
      const point = returnPoints.get(entry)
      if (point?.actor === actor) {
        point.departed = true
      }
    }
  }, [entry, actor])

  useEffect(() => {
    if (!entry || routingActor(useAuthStore.getState().auth) !== actor) {
      return
    }
    const point = returnPoints.get(entry)
    if (!point || point.actor !== actor || !point.departed) {
      return
    }
    if (options.pending) {
      const cancel = () => returnPoints.delete(entry)
      document.addEventListener('pointerdown', cancel, true)
      document.addEventListener('keydown', cancel, true)
      document.addEventListener('focusin', cancel, true)
      document.addEventListener('wheel', cancel, {
        capture: true,
        passive: true,
      })
      return () => {
        document.removeEventListener('pointerdown', cancel, true)
        document.removeEventListener('keydown', cancel, true)
        document.removeEventListener('focusin', cancel, true)
        document.removeEventListener('wheel', cancel, true)
      }
    }
    returnPoints.delete(entry)
    if (
      !options.enabled ||
      options.failed ||
      !options.items?.some((item) => item.id === point.connectionId)
    ) {
      return
    }
    const target = options.root.current?.querySelector<HTMLElement>(
      `[data-overview-routing-link="${point.connectionId}"]`
    )
    const scroller = target?.closest<HTMLElement>('[data-page-scroll]')
    if (!target || !scroller) {
      return
    }
    target.focus({ preventScroll: true })
    scroller.scrollTop = point.top
    scroller.scrollLeft = point.left
  }, [
    actor,
    entry,
    options.enabled,
    options.pending,
    options.failed,
    options.items,
    options.root,
  ])

  return useCallback(
    (connectionId: number) => {
      if (
        !options.enabled ||
        routingActor(useAuthStore.getState().auth) !== actor ||
        !Number.isSafeInteger(connectionId) ||
        connectionId <= 0
      ) {
        return
      }
      const key =
        router.state.location.state.__TSR_key ?? router.state.location.state.key
      const target = options.root.current?.querySelector<HTMLElement>(
        `[data-overview-routing-link="${connectionId}"]`
      )
      const scroller = target?.closest<HTMLElement>('[data-page-scroll]')
      if (
        !key ||
        !scroller ||
        !Number.isFinite(scroller.scrollTop) ||
        !Number.isFinite(scroller.scrollLeft)
      ) {
        return
      }
      returnPoints.delete(key)
      returnPoints.set(key, {
        actor,
        connectionId,
        top: Math.max(0, scroller.scrollTop),
        left: scroller.scrollLeft,
        departed: false,
      })
      if (returnPoints.size > 20) {
        const oldest = returnPoints.keys().next().value
        if (oldest !== undefined) {
          returnPoints.delete(oldest)
        }
      }
    },
    [actor, router, options.enabled, options.root]
  )
}
