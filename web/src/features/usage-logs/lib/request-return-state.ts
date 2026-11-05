/* Copyright (C) 2026 ForceMind. AGPL-3.0-or-later. */
import { useLocation, useRouter } from '@tanstack/react-router'
import { useCallback, useEffect } from 'react'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import type { LogsViewScope } from '../components/usage-logs-provider'

type ReturnPoint = {
  top: number
  left: number
  logId: number
  kind: 'table' | 'page'
  viewScope: LogsViewScope
}

export function requestReturnActorKey(auth: ReturnType<typeof useAuthStore.getState>['auth']): string {
  return JSON.stringify([auth.user?.id ?? null, auth.user?.role ?? 0, auth.session?.sid ?? null])
}

/** Only coordinates, a record ID and an enum are retained; never filters or content. */
export function createRequestReturnStateCache() {
  let identity: string | null = null
  const points = new Map<string, ReturnPoint>()
  const read = (entry: string, actor: string) => {
    if (identity !== actor) {
      points.clear()
      identity = actor
    }
    return points.get(entry)
  }
  return {
    read,
    write(entry: string, actor: string, point: ReturnPoint) {
      read(entry, actor)
      points.delete(entry)
      points.set(entry, { ...point })
      if (points.size > 20) {
        const oldest = points.keys().next().value
        if (oldest !== undefined) points.delete(oldest)
      }
    },
    take(entry: string, actor: string) {
      const point = read(entry, actor)
      points.delete(entry)
      return point
    },
    clear() { points.clear(); identity = null },
  }
}

const returnPoints = createRequestReturnStateCache()
useAuthStore.subscribe((state, previous) => {
  if (requestReturnActorKey(state.auth) !== requestReturnActorKey(previous.auth)) returnPoints.clear()
})

/** Existing request-link callbacks already restrict this to an unmodified same-tab click. */
export function useCaptureRequestReturn(actor: string) {
  const router = useRouter()
  const entry = useLocation({ select: (location) => location.state.__TSR_key ?? location.state.key })
  const capture = useCallback((logId: number, viewScope: LogsViewScope = 'all') => {
    const current = useAuthStore.getState().auth
    if (requestReturnActorKey(current) !== actor || !Number.isSafeInteger(logId) || logId <= 0) return
    const key = router.state.location.state.__TSR_key ?? router.state.location.state.key
    if (!key) return
    const target = document.querySelector<HTMLElement>(`[data-usage-request-link="${logId}"]`)
    const table = target?.closest('table')
    const scroller = table?.parentElement ?? target?.closest<HTMLElement>('[data-page-scroll]')
    if (!scroller || !Number.isFinite(scroller.scrollTop) || !Number.isFinite(scroller.scrollLeft)) return
    returnPoints.write(key, actor, {
      top: scroller.scrollTop, left: scroller.scrollLeft, logId,
      kind: table ? 'table' : 'page',
      viewScope: (current.user?.role ?? 0) >= ROLE.ADMIN ? viewScope : 'self',
    })
  }, [router, actor])
  return { capture, initialViewScope: entry ? returnPoints.read(entry, actor)?.viewScope : undefined }
}

/** Called after the existing return-focus hook, so final focus cannot move the restored scroll. */
export function useRestoreRequestReturn(options: { ready: boolean; rows: unknown }) {
  const actor = useAuthStore((state) => requestReturnActorKey(state.auth))
  const entry = useLocation({ select: (location) => location.state.__TSR_key ?? location.state.key })
  useEffect(() => {
    if (!entry || requestReturnActorKey(useAuthStore.getState().auth) !== actor) return
    const point = returnPoints.read(entry, actor)
    if (!point) return
    if (!options.ready) {
      const cancel = () => {
        if (requestReturnActorKey(useAuthStore.getState().auth) === actor) returnPoints.take(entry, actor)
      }
      document.addEventListener('pointerdown', cancel, true)
      document.addEventListener('keydown', cancel, true)
      document.addEventListener('focusin', cancel, true)
      return () => {
        document.removeEventListener('pointerdown', cancel, true)
        document.removeEventListener('keydown', cancel, true)
        document.removeEventListener('focusin', cancel, true)
      }
    }
    returnPoints.take(entry, actor)
    const target = document.querySelector<HTMLElement>(`[data-usage-request-link="${point.logId}"]`)
    if (!target) return
    const scroller = point.kind === 'table' ? target.closest('table')?.parentElement : target.closest<HTMLElement>('[data-page-scroll]')
    if (!scroller) return
    scroller.scrollTop = point.top
    scroller.scrollLeft = point.left
    target.focus({ preventScroll: true })
  }, [entry, actor, options.ready, options.rows])
}
