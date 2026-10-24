/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useEffect, useRef, type RefObject } from 'react'

/**
 * Returns focus to the list row that opened an inspector once the inspector
 * closes. Rows carry `data-inspect-id`. When the list is still loading the
 * return waits for the row, but any newer pointer, key or focus action from
 * the user cancels it; a removed row or a failed list falls back to
 * `fallbackRef`. Changing the list context (filters, page, identity) drops a
 * pending return, because the old row no longer belongs to the list.
 */
export function useInspectorFocusReturn(options: {
  selectedId: string | undefined
  listRef: RefObject<HTMLElement | null>
  fallbackRef?: RefObject<HTMLElement | null>
  context: string
  settled: boolean
  rows: unknown
}) {
  const lastSelection = useRef(options.selectedId)
  const lastContext = useRef(options.context)
  const pending = useRef<string | undefined>(undefined)

  useEffect(() => {
    if (
      options.selectedId !== undefined ||
      lastContext.current !== options.context
    ) {
      pending.current = undefined
    } else if (lastSelection.current !== undefined) {
      pending.current = lastSelection.current
    }
    lastSelection.current = options.selectedId
    lastContext.current = options.context
  }, [options.selectedId, options.context])

  useEffect(() => {
    const returnId = pending.current
    if (returnId === undefined) return
    const row = options.listRef.current?.querySelector<HTMLElement>(
      `[data-inspect-id="${returnId.replaceAll(/["\\]/g, '\\$&')}"]`
    )
    if (row || options.settled) {
      pending.current = undefined
      const target = row ?? options.fallbackRef?.current
      target?.focus()
      return
    }
    const cancel = () => {
      pending.current = undefined
    }
    document.addEventListener('pointerdown', cancel, true)
    document.addEventListener('keydown', cancel, true)
    document.addEventListener('focusin', cancel, true)
    return () => {
      document.removeEventListener('pointerdown', cancel, true)
      document.removeEventListener('keydown', cancel, true)
      document.removeEventListener('focusin', cancel, true)
    }
  }, [
    options.selectedId,
    options.context,
    options.settled,
    options.rows,
    options.listRef,
    options.fallbackRef,
  ])
}
