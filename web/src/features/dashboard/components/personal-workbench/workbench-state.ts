import type { PlaygroundKey } from '@/features/playground/types'
import type { UserUsagePolicy } from '@/features/users/user-usage-policy-api'
import { ROLE } from '@/lib/roles'
import type { AuthUser } from '@/stores/auth-store'

export type PlaygroundKeyCondition =
  | 'available'
  | 'disabled'
  | 'expired'
  | 'exhausted'
  | 'unknown'

export function getPlaygroundKeyCondition(
  key: PlaygroundKey,
  nowSeconds = Date.now() / 1000
): PlaygroundKeyCondition {
  if (key.status === 2) return 'disabled'
  if (key.status === 3) return 'expired'
  if (key.status === 4) return 'exhausted'
  if (key.status !== 1) return 'unknown'
  if (!Number.isSafeInteger(key.expired_time)) return 'unknown'
  if (!key.unlimited_quota && !Number.isSafeInteger(key.remain_quota)) {
    return 'unknown'
  }
  if (key.expired_time !== -1 && key.expired_time <= nowSeconds) {
    return 'expired'
  }
  if (!key.unlimited_quota && key.remain_quota <= 0) return 'exhausted'
  return 'available'
}

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

export function getFreshUsagePolicy(
  user: AuthUser | undefined
): UserUsagePolicy | undefined {
  if (
    !user ||
    typeof user.self_use_no_balance !== 'boolean' ||
    !Number.isSafeInteger(user.usage_policy_revision) ||
    (user.usage_policy_revision ?? -1) < 0 ||
    !Number.isSafeInteger(user.quota)
  ) {
    return undefined
  }
  return {
    user_id: user.id,
    no_balance: user.self_use_no_balance,
    revision: user.usage_policy_revision as number,
    legacy_remaining_quota: user.quota as number,
  }
}
