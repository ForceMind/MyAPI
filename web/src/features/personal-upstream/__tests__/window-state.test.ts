/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { describe, expect, test } from 'vitest'

import type { ChannelQuotaChangeItem } from '@/features/channels/types'

import { nativeWindowFreshness, nativeWindowUsage } from '../window-state'

const window: ChannelQuotaChangeItem = {
  channel_id: 7,
  name: 'Subscription account',
  metric_type: 'codex_rate_limit',
  unit: 'percent',
  status: 'success',
  current_available: 45,
  observed_at: 1000,
}

describe('native upstream window evidence', () => {
  test('expresses account use from the native remaining percentage', () => {
    expect(nativeWindowUsage(window)).toBe(55)
  })
  test.each([null, undefined, Number.NaN, -1, 101])(
    'keeps invalid remaining evidence %s unknown',
    (remaining) => {
      expect(
        nativeWindowUsage({ ...window, current_available: remaining })
      ).toBeNull()
    }
  )
  test('does not convert a monetary balance into window usage', () => {
    expect(
      nativeWindowUsage({ ...window, metric_type: 'balance', unit: 'usd' })
    ).toBeNull()
  })
  test('does not treat an errored snapshot as current usage', () => {
    expect(nativeWindowUsage({ ...window, status: 'error' })).toBeNull()
  })
  test('marks evidence stale only after its sampling validity expires', () => {
    expect(nativeWindowFreshness(window, 1300)).toBe('fresh')
    expect(nativeWindowFreshness(window, 1301)).toBe('stale')
  })
  test('keeps future and missing sample times unknown', () => {
    expect(nativeWindowFreshness(window, 999)).toBe('unknown')
    expect(
      nativeWindowFreshness({ ...window, observed_at: undefined }, 1300)
    ).toBe('unknown')
  })
})
