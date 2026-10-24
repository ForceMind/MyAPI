/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
export type KeyCondition =
  | 'available'
  | 'disabled'
  | 'expired'
  | 'exhausted'
  | 'unknown'

type KeyStateFields = {
  status: number
  expired_time: number
  unlimited_quota: boolean
  remain_quota: number
}

/**
 * Whether a Key can serve a request right now, from its server fields only.
 * Malformed numbers are `unknown`, never `available`, so a client action
 * (such as opening Playground with this Key) is not widened by bad data.
 */
export function getKeyCondition(
  key: KeyStateFields,
  nowSeconds = Date.now() / 1000
): KeyCondition {
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
