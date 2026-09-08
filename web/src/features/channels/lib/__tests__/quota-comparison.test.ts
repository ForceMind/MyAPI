/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { describe, expect, test } from 'vitest'

import {
  nearestQuotaPoint,
  quotaComparisonGroup,
  weeklyQuotaCycle,
  zoomQuotaTime,
} from '../quota-comparison'

describe('quota comparison time contract', () => {
  test('zoom retains the pointer anchor and clamps expansion to now', () => {
    expect(zoomQuotaTime({ start: 1000, end: 2000 }, 0.5, 0.25, 3000)).toEqual({
      start: 1125,
      end: 1625,
    })
    expect(zoomQuotaTime({ start: 1000, end: 2000 }, 2, 0.5, 2000)).toEqual({
      start: 0,
      end: 2000,
    })
  })
  test('zoom stops at one minute and the supported 180 day range', () => {
    expect(
      zoomQuotaTime({ start: 1000, end: 1060 }, 0.5, 0.5, 2000).end -
        zoomQuotaTime({ start: 1000, end: 1060 }, 0.5, 0.5, 2000).start
    ).toBe(60)
    const result = zoomQuotaTime(
      { start: 1, end: 180 * 86400 },
      10,
      0.5,
      200 * 86400
    )
    expect(result.end - result.start).toBe(180 * 86400)
  })
  test('weekly range follows a Thursday reset and previous cycle is complete', () => {
    const reset = Date.parse('2026-09-10T10:30:00Z') / 1000
    const now = Date.parse('2026-09-08T12:00:00Z') / 1000
    expect(weeklyQuotaCycle(reset, now)).toEqual({
      start: reset - 604800,
      end: now,
    })
    expect(weeklyQuotaCycle(reset, now, -1)).toEqual({
      start: reset - 1209600,
      end: reset - 604800,
    })
  })
  test('a reset at now opens the next cycle, and missing anchors are not guessed', () => {
    expect(weeklyQuotaCycle(2000000, 2000000)).toBeNull()
    expect(weeklyQuotaCycle(2000000, 2000000, -1)).toEqual({
      start: 1395200,
      end: 2000000,
    })
    expect(weeklyQuotaCycle(Number.NaN, 2000000)).toBeNull()
    expect(weeklyQuotaCycle(0, 2000000)).toBeNull()
  })
  test('different measurement units and windows cannot share a comparison group', () => {
    const base = {
      channel_id: 1,
      name: 'A',
      unit: 'percent',
      window_type: 'weekly',
    }
    expect(quotaComparisonGroup(base)).toBe(
      quotaComparisonGroup({ ...base, channel_id: 2 })
    )
    expect(quotaComparisonGroup(base)).not.toBe(
      quotaComparisonGroup({ ...base, unit: 'USD' })
    )
    expect(quotaComparisonGroup(base)).not.toBe(
      quotaComparisonGroup({ ...base, window_type: 'five_hour' })
    )
  })
  test('nearest point returns an actual sample including its missing value', () => {
    const points = [
      { timestamp: 100, value: 1.234567 },
      { timestamp: 160, value: null },
      { timestamp: 220, value: 1 },
    ]
    expect(nearestQuotaPoint(points, 159)).toEqual(points[1])
    expect(nearestQuotaPoint(points, 10)).toEqual(points[0])
    expect(nearestQuotaPoint(points, 300)).toEqual(points[2])
    expect(nearestQuotaPoint([], 300)).toBeUndefined()
  })
})
