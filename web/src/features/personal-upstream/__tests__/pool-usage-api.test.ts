/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { getPoolUsage } from '../pool-usage-api'
import { poolUsageFixture } from './pool-usage-fixture'

afterEach(() => vi.restoreAllMocks())

test('reads the exact pool with independent cancellation and validates the complete decomposition', async () => {
  const pool = poolUsageFixture()
  const read = vi
    .spyOn(api, 'get')
    .mockResolvedValue({ data: { success: true, data: pool } } as never)
  const controller = new AbortController()
  expect(await getPoolUsage(7, 1, controller.signal)).toEqual(pool)
  expect(read).toHaveBeenCalledWith(
    '/api/subscription-share/pools/7/usage',
    expect.objectContaining({
      signal: controller.signal,
      disableDuplicate: true,
      skipAuthRefresh: true,
      timeout: 15000,
    })
  )
})

for (const corruption of [
  'pool',
  'channel',
  'duplicate-key',
  'allocation-total',
  'usage-total',
  'growth',
  'baseline',
  'missing-usage',
  'negative',
  'unsafe-count',
  'request-count',
  'timestamp',
] as const) {
  test(`rejects ${corruption} without presenting a misleading chart`, async () => {
    const pool = poolUsageFixture()
    if (corruption === 'pool') pool.pool_id = 8
    if (corruption === 'channel') pool.channel_id = 2
    if (corruption === 'duplicate-key') pool.allocations[1].token_id = 21
    if (corruption === 'allocation-total') pool.allocated_bps = 7000
    if (corruption === 'usage-total') pool.observed_used_micro = 56000000
    if (corruption === 'growth') pool.unattributed_growth_micro = 6000000
    if (corruption === 'baseline') pool.initial_baseline_used_micro = 45000000
    if (corruption === 'missing-usage')
      pool.allocations[0].estimated_used_micro = null
    if (corruption === 'negative') pool.estimated_used_micro = -1
    if (corruption === 'unsafe-count')
      pool.pending_attempts = Number.MAX_SAFE_INTEGER + 1
    if (corruption === 'request-count') pool.pending_attempts = 0
    if (corruption === 'timestamp') pool.baseline_time = Number.MAX_SAFE_INTEGER
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: pool },
    } as never)
    await expect(getPoolUsage(7, 1)).rejects.toThrow()
  })
}

test('preserves nullable evidence and orphan request counts in an inconsistent projection', async () => {
  const pool = poolUsageFixture({
    decomposition_state: 'inconsistent',
    estimated_used_micro: null,
    pending_attempts: 3,
  })
  pool.allocations[0].estimated_used_micro = null
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: pool },
  } as never)
  expect(await getPoolUsage(7, 1)).toEqual(pool)
})

test('never requests an unconfigured candidate pool', async () => {
  const read = vi.spyOn(api, 'get')
  await expect(getPoolUsage(0, 1)).rejects.toThrow()
  expect(read).not.toHaveBeenCalled()
})

for (const field of [
  'baseline_used_micro',
  'initial_baseline_used_micro',
  'observed_used_micro',
  'estimated_used_micro',
  'unattributed_micro',
  'unattributed_growth_micro',
] as const) {
  test(`rejects out-of-window ${field} even in an inconsistent projection`, async () => {
    const pool = poolUsageFixture({ decomposition_state: 'inconsistent' })
    pool[field] = 100000001
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: pool },
    } as never)
    await expect(getPoolUsage(7, 1)).rejects.toThrow()
  })
}

test('rejects per-Key usage beyond the native window instead of clipping it', async () => {
  const pool = poolUsageFixture({ decomposition_state: 'inconsistent' })
  pool.allocations[0].estimated_used_micro = 100000001
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: pool },
  } as never)
  await expect(getPoolUsage(7, 1)).rejects.toThrow()
})

test('rejects an arithmetically balanced decomposition that exceeds the native total pool', async () => {
  const pool = poolUsageFixture({
    observed_used_micro: 110000000,
    estimated_used_micro: 40000000,
    unattributed_micro: 70000000,
    unattributed_growth_micro: 35000000,
  })
  pool.allocations[0].estimated_used_micro = 20000000
  pool.allocations[1].estimated_used_micro = 15000000
  pool.allocations[2].estimated_used_micro = 5000000
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: pool },
  } as never)
  await expect(getPoolUsage(7, 1)).rejects.toThrow()
})
