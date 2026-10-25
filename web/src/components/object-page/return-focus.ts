/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import {
  useLocation,
  useRouter,
  type HistoryLocation,
} from '@tanstack/react-router'
import { useCallback, useEffect, type RefObject } from 'react'

// The row that opened an object page, per history entry of the page it was
// opened from. Back (in-app or the browser's) shows that entry again and
// focus returns there; the page itself is remounted, so this lives outside
// it. The row attribute names the list, so the page being left never takes
// a return meant for the page being shown.
const pendingFocus = new Map<string, { attribute: string; id: string }>()

function entryKey(state: HistoryLocation['state']) {
  return state.__TSR_key ?? state.key
}

/**
 * Remembers, for the current history entry, the row (`[attribute="id"]`)
 * that is opening an object page.
 */
export function useRememberReturnFocus() {
  const router = useRouter()
  return useCallback(
    (attribute: string, id: string | number) => {
      const key = entryKey(router.state.location.state)
      if (key) pendingFocus.set(key, { attribute, id: String(id) })
    },
    [router]
  )
}

/**
 * Returns focus to the row of this list that opened an object page, once
 * its history entry is shown again. While the list is loading it waits for
 * the row, but any newer pointer, key or focus action from the user cancels
 * the return; once the list has settled without that row, focus goes to the
 * fallback when there is one, else it gives up when other rows are shown.
 */
export function useReturnFocus(options: {
  attribute: string
  settled: boolean
  rows: unknown
  /**
   * Where focus goes when the list has settled without that row (removed,
   * filtered out, empty or unreadable).
   */
  fallback?: RefObject<HTMLElement | null>
}) {
  const key = useLocation({ select: (location) => entryKey(location.state) })
  useEffect(() => {
    if (!key) return
    const pending = pendingFocus.get(key)
    if (pending?.attribute !== options.attribute) return
    const target = document.querySelector<HTMLElement>(
      `[${pending.attribute}="${pending.id.replaceAll(/["\\]/g, '\\$&')}"]`
    )
    const listShown =
      options.settled &&
      document.querySelector(`[${options.attribute}]`) !== null
    const fallback = options.settled ? options.fallback?.current : null
    if (target || listShown || fallback) {
      pendingFocus.delete(key)
      ;(target ?? fallback)?.focus()
      return
    }
    const cancel = () => pendingFocus.delete(key)
    document.addEventListener('pointerdown', cancel, true)
    document.addEventListener('keydown', cancel, true)
    document.addEventListener('focusin', cancel, true)
    return () => {
      document.removeEventListener('pointerdown', cancel, true)
      document.removeEventListener('keydown', cancel, true)
      document.removeEventListener('focusin', cancel, true)
    }
  }, [key, options.attribute, options.settled, options.rows, options.fallback])
}
