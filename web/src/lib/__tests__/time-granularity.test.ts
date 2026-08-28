/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { describe, expect, it } from 'vitest'

import { formatChartTime, getChartBucketTimestamp } from '@/lib/time'

describe('dashboard time granularity', () => {
  it('formats minute buckets with the exact local minute', () => {
    const timestamp = Math.floor(
      new Date(2026, 7, 28, 12, 34, 56).getTime() / 1000
    )

    expect(formatChartTime(timestamp, 'minute')).toBe('08-28 12:34')
  })

  it('normalizes each supported bucket to its local boundary', () => {
    const timestamp = Math.floor(
      new Date(2026, 7, 25, 12, 34, 56).getTime() / 1000
    )

    const minute = new Date(getChartBucketTimestamp(timestamp, 'minute') * 1000)
    expect([
      minute.getHours(),
      minute.getMinutes(),
      minute.getSeconds(),
    ]).toEqual([12, 34, 0])

    const hour = new Date(getChartBucketTimestamp(timestamp, 'hour') * 1000)
    expect([hour.getHours(), hour.getMinutes(), hour.getSeconds()]).toEqual([
      12, 0, 0,
    ])

    const day = new Date(getChartBucketTimestamp(timestamp, 'day') * 1000)
    expect([day.getHours(), day.getMinutes(), day.getSeconds()]).toEqual([
      0, 0, 0,
    ])

    const week = new Date(getChartBucketTimestamp(timestamp, 'week') * 1000)
    expect([week.getDay(), week.getHours(), week.getMinutes()]).toEqual([
      1, 0, 0,
    ])
    expect(formatChartTime(timestamp, 'week')).toBe('08-24 - 08-30')
  })

  it('uses the API fixed offset for both bucketing and labels', () => {
    const timestamp = Math.floor(
      new Date('2026-01-01T00:45:00-05:00').getTime() / 1000
    )
    const bucket = getChartBucketTimestamp(timestamp, 'day', -300)

    expect(bucket).toBe(
      Math.floor(new Date('2026-01-01T00:00:00-05:00').getTime() / 1000)
    )
    expect(formatChartTime(bucket, 'day', -300)).toBe('01-01')
  })
})
