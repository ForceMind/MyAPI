/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
export const LOG_PAGE_SIZES = [20, 50, 100] as const
export type LogPageSize = (typeof LOG_PAGE_SIZES)[number]

export function getLogPageCount(total: number | undefined, pageSize: number): number | null {
  if (total === undefined || !Number.isSafeInteger(total) || total < 0 || !Number.isSafeInteger(pageSize) || pageSize <= 0) return null
  return Math.max(1, Math.ceil(total / pageSize))
}

/** Preserve valid old links while bounding all outgoing list requests. */
export function normalizeLogPagination(pageIndex: number, pageSize: number) {
  return {
    pageIndex: Number.isSafeInteger(pageIndex) && pageIndex >= 0 && pageIndex < Number.MAX_SAFE_INTEGER ? pageIndex : 0,
    pageSize: Number.isSafeInteger(pageSize) && pageSize > 0 ? Math.min(100, pageSize) : 20,
  }
}
