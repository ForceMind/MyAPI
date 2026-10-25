/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type {
  SubscriptionSharePool,
  SubscriptionShareView,
} from '../subscription-share-api'

/** Shares that currently limit this Key (not candidates or closed ones). */
export function heldSharePools(view: SubscriptionShareView | undefined) {
  return (
    view?.pools.filter(
      (pool) =>
        pool.pool_id > 0 &&
        pool.state !== 'candidate' &&
        pool.state !== 'closed'
    ) ?? []
  )
}

/**
 * Estimated use of one share, in percent of the share. `null` when the
 * window has no usage baseline yet (or the share is 0): unknown, never 0%.
 */
export function shareUsedPercent(pool: SubscriptionSharePool): number | null {
  if (pool.baseline_time <= 0 || pool.share_bps <= 0) return null
  return pool.estimated_used_micro / (pool.share_bps * 100)
}
