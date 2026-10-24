/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { useRouter } from '@tanstack/react-router'
import { useCallback } from 'react'

declare module '@tanstack/history' {
  interface HistoryState {
    /** The previous history entry is the view this one was opened from. */
    inspectorBack?: boolean
  }
}

/**
 * Closing an inspector that was opened from its list goes back one history
 * entry, so the browser Back button and the in-app close agree. A deep link
 * or refresh has no such entry; it falls back to replacing the URL.
 */
export function useInspectorBack() {
  const router = useRouter()
  return useCallback(
    (fallback: () => void) => {
      if (router.state.location.state.inspectorBack) {
        router.history.back()
        return
      }
      fallback()
    },
    [router]
  )
}
