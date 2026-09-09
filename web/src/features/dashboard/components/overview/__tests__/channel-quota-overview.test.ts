/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { describe, expect, test } from 'vitest'

import type { ChannelQuotaChangeItem } from '@/features/channels/types'

import { selectComparableQuotaSeries } from '../channel-quota-overview-data'

describe('dashboard channel quota main chart', () => {
  test('uses a stable set of comparable weekly quota series', () => {
    const items: ChannelQuotaChangeItem[] = [
      { channel_id: 14, name: 'daily', unit: 'percent', window_type: 'daily' },
      {
        channel_id: 9,
        name: 'weekly b',
        unit: 'percent',
        window_type: 'weekly',
        window_seconds: 604800,
      },
      {
        channel_id: 2,
        name: 'weekly a',
        unit: 'percent',
        window_type: 'weekly',
        window_seconds: 604800,
      },
      {
        channel_id: 11,
        name: 'weekly c',
        unit: 'percent',
        window_type: 'weekly',
        window_seconds: 604800,
      },
      {
        channel_id: 8,
        name: 'weekly d',
        unit: 'percent',
        window_type: 'weekly',
        window_seconds: 604800,
      },
      {
        channel_id: 3,
        name: 'weekly e',
        unit: 'percent',
        window_type: 'weekly',
        window_seconds: 604800,
      },
      {
        channel_id: 5,
        name: 'weekly dollars',
        unit: 'usd',
        window_type: 'weekly',
        window_seconds: 604800,
      },
    ]

    expect(
      selectComparableQuotaSeries(items).map((item) => item.channel_id)
    ).toEqual([2, 3, 8, 9])
  })
})
