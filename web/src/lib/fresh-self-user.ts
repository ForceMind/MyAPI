/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { AuthUser } from '@/stores/auth-store'

import { ROLE } from './roles'

/**
 * Accepts a `/api/user/self` response only when it describes the signed-in
 * user with a known role. Pages that grant upstream access re-read the user
 * through this check instead of trusting cached permissions.
 */
export function parseFreshSelfUser(
  payload: unknown,
  expectedUserId: number
): AuthUser {
  if (!payload || typeof payload !== 'object') {
    throw new Error('Self response unavailable')
  }
  const response = payload as { success?: unknown; data?: unknown }
  if (
    response.success !== true ||
    !response.data ||
    typeof response.data !== 'object'
  ) {
    throw new Error('Self response unavailable')
  }
  const candidate = response.data as Record<string, unknown>
  const validRole =
    candidate.role === ROLE.USER ||
    candidate.role === ROLE.ADMIN ||
    candidate.role === ROLE.SUPER_ADMIN
  if (
    candidate.id !== expectedUserId ||
    !Number.isSafeInteger(candidate.id) ||
    typeof candidate.username !== 'string' ||
    !validRole
  ) {
    throw new Error('Self response identity mismatch')
  }
  return candidate as unknown as AuthUser
}
