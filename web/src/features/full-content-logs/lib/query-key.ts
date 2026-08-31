/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import type { FullContentLogFilters } from '../types'

/**
 * Keep sensitive full-content log caches isolated across users and sessions.
 * A logout/login in the same tab must never reuse another identity's rows.
 */
export function getFullContentLogsQueryKey(
  page: number,
  filters: FullContentLogFilters,
  userId: number | null,
  sessionId: string | null
) {
  return ['full-content-logs', 'list', userId, sessionId, page, filters] as const
}
