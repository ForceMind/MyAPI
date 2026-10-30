/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import type { FullContentLogFilters } from '../types'

export const FULL_CONTENT_PAGE_SIZES = [20, 50, 100] as const
export type FullContentPageSize = (typeof FULL_CONTENT_PAGE_SIZES)[number]

export interface FullContentLogListState {
  page: number
  pageSize: FullContentPageSize
  filters: FullContentLogFilters
}

export const EMPTY_FULL_CONTENT_FILTERS: FullContentLogFilters = {
  model: '',
  token: '',
  requestId: '',
  startTime: '',
  endTime: '',
}

export function createFullContentLogListState(): FullContentLogListState {
  return {
    page: 1,
    pageSize: 20,
    filters: { ...EMPTY_FULL_CONTENT_FILTERS },
  }
}

export function getFullContentPageCount(
  total: number | undefined,
  pageSize: number
): number | null {
  if (total === undefined || !Number.isSafeInteger(total) || total < 0) return null
  return Math.max(1, Math.ceil(total / pageSize))
}

/** Only list controls live in memory. Bodies, responses and credentials never do. */
export function createFullContentLogListStateCache() {
  let identity: string | null = null
  const entries = new Map<string, FullContentLogListState>()

  return {
    read(entryKey: string, userId: number | null, sessionId: string | null) {
      const nextIdentity = JSON.stringify([userId, sessionId])
      if (identity !== nextIdentity) {
        entries.clear()
        identity = nextIdentity
      }
      const state = entries.get(entryKey)
      if (!state) return createFullContentLogListState()
      return { ...state, filters: { ...state.filters } }
    },
    write(
      entryKey: string,
      userId: number | null,
      sessionId: string | null,
      state: FullContentLogListState
    ) {
      if (identity !== JSON.stringify([userId, sessionId])) return
      entries.delete(entryKey)
      entries.set(entryKey, { ...state, filters: { ...state.filters } })
      if (entries.size > 20) {
        const oldestKey = entries.keys().next().value
        if (oldestKey !== undefined) entries.delete(oldestKey)
      }
    },
    clear() {
      entries.clear()
      identity = null
    },
  }
}
