import type { UserWalletData } from '../types'

export type WalletAccountSnapshot = Pick<UserWalletData, 'id' | 'username' | 'group'> &
  Partial<Pick<UserWalletData, 'quota' | 'used_quota' | 'request_count' | 'aff_quota' | 'aff_history_quota' | 'aff_count'>>

function walletAmount(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isSafeInteger(value) ? value : undefined
}

function walletCount(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0 ? value : undefined
}

/** Identity is required; one missing amount never erases other known observations. */
export function readWalletAccount(value: unknown, expectedId: number | undefined): WalletAccountSnapshot | null {
  if (!value || typeof value !== 'object' || !('id' in value) || value.id !== expectedId || expectedId === undefined) return null
  const record = value as Record<string, unknown>
  return {
    id: expectedId,
    username: typeof record.username === 'string' ? record.username : '',
    group: typeof record.group === 'string' ? record.group : '',
    quota: walletAmount(record.quota),
    used_quota: walletAmount(record.used_quota),
    request_count: walletCount(record.request_count),
    aff_quota: walletAmount(record.aff_quota),
    aff_history_quota: walletAmount(record.aff_history_quota),
    aff_count: walletCount(record.aff_count),
  }
}
