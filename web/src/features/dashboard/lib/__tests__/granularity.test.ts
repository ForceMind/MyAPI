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

import { processChartData } from '@/features/dashboard/lib/charts'
import {
  buildQueryParams,
  isQuotaRangeSupported,
} from '@/features/dashboard/lib/filters'
import type { QuotaDataItem } from '@/features/dashboard/types'

function positiveBarValues(
  data: QuotaDataItem[],
  granularity: 'minute' | 'hour'
) {
  const chart = processChartData(data, granularity)
  return chart.spec_line.data[0].values.filter(
    (item: { rawQuota: number }) => item.rawQuota > 0
  )
}

describe('dashboard aggregation requests', () => {
  it('sends the selected granularity and browser timezone with the legacy alias', () => {
    const params = buildQueryParams(
      { start_timestamp: 100, end_timestamp: 200 },
      { time_granularity: 'minute' }
    )

    expect(params).toMatchObject({
      start_timestamp: 100,
      end_timestamp: 200,
      granularity: 'minute',
      default_time: 'minute',
      timezone_offset: -new Date(200 * 1000).getTimezoneOffset(),
    })
  })

  it('accepts one day of minute buckets and rejects longer minute ranges', () => {
    const end = new Date('2026-08-28T12:00:00Z')
    const oneDayStart = new Date(end.getTime() - 24 * 60 * 60 * 1000)
    const sevenDayStart = new Date(end.getTime() - 7 * 24 * 60 * 60 * 1000)

    expect(isQuotaRangeSupported(oneDayStart, end, 'minute')).toBe(true)
    expect(isQuotaRangeSupported(sevenDayStart, end, 'minute')).toBe(false)
    expect(isQuotaRangeSupported(sevenDayStart, end, 'hour')).toBe(true)
  })

  it('applies the 1500 bucket limit to every granularity', () => {
    const end = new Date('2026-08-28T12:00:00Z')
    const acceptedHourStart = new Date(end.getTime() - 1499 * 60 * 60 * 1000)
    const rejectedHourStart = new Date(end.getTime() - 1500 * 60 * 60 * 1000)

    expect(isQuotaRangeSupported(acceptedHourStart, end, 'hour')).toBe(true)
    expect(isQuotaRangeSupported(rejectedHourStart, end, 'hour')).toBe(false)
  })

  it('keeps minute buckets separate and combines them at hour granularity', () => {
    const firstMinute = Math.floor(
      new Date(2026, 7, 28, 12, 1, 0).getTime() / 1000
    )
    const data: QuotaDataItem[] = [
      {
        model_name: 'gpt-a',
        created_at: firstMinute,
        quota: 10,
        count: 1,
      },
      {
        model_name: 'gpt-a',
        created_at: firstMinute + 60,
        quota: 20,
        count: 2,
      },
    ]

    const minuteValues = positiveBarValues(data, 'minute')
    expect(minuteValues).toHaveLength(2)
    expect(
      minuteValues.map((item: { rawQuota: number }) => item.rawQuota)
    ).toEqual([10, 20])

    const hourValues = positiveBarValues(data, 'hour')
    expect(hourValues).toHaveLength(1)
    expect(hourValues[0].rawQuota).toBe(30)
  })
})
