/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { expect, test } from 'vitest'

import type { ChannelQuotaChangeItem } from '@/features/channels/types'

import { describeConnectionWindow } from '../window-state'

const now = 1_800_000_000
const t = (key: string) => key
const success: ChannelQuotaChangeItem = {
  channel_id: 7,
  series_id: 'a'.repeat(64),
  name: 'Subscription',
  metric_type: 'codex_rate_limit',
  unit: 'percent',
  source: 'codex_wham_usage_primary',
  window_type: 'five_hour',
  status: 'success',
  current_available: 18,
  observed_at: now - 30,
}
const failed: ChannelQuotaChangeItem = {
  ...success,
  source: 'codex_wham_usage',
  series_id: 'b'.repeat(64),
  status: 'error',
  current_available: null,
  observed_at: now - 3600,
}

test('relay limit evidence is a forwarding event even when a newer WHAM observation exists', () => {
  const event = {
    ...failed,
    source: 'codex_relay_usage_limit',
    window_type: 'none',
  }
  expect(describeConnectionWindow(event, [event, success], now, t)).toEqual({
    label: 'Relay usage-limit event',
    description:
      'A model request reported a usage limit. This event is not a WHAM sampling failure.',
    variant: 'warning',
  })
})

test('a newer valid WHAM window labels an older independent error as historical without claiming resolution', () => {
  const windows = [failed, success]
  expect(describeConnectionWindow(failed, windows, now, t)).toEqual({
    label: 'Historical saved sample error',
    description:
      'A newer WHAM observation exists for this connection. This saved error may belong to a different identity and is not marked resolved.',
    variant: 'warning',
  })
  expect(windows).toEqual([failed, success])
  expect(windows.map((item) => item.series_id)).toEqual([
    'b'.repeat(64),
    'a'.repeat(64),
  ])
})

test.each([
  { ...success, channel_id: 8 },
  { ...success, observed_at: now - 301 },
  { ...success, observed_at: now + 1 },
  { ...success, observed_at: undefined },
  { ...success, observed_at: failed.observed_at },
  { ...success, source: 'codex_relay_usage_limit' },
  { ...success, current_available: null },
  { ...success, current_available: Number.NaN },
])(
  'unrelated or untrustworthy success cannot turn WHAM failure into historical evidence: %j',
  (other) => {
    expect(describeConnectionWindow(failed, [failed, other], now, t).label).toBe(
      'WHAM usage sample failed'
    )
  }
)

test('a fresh zero-remaining window stays measured and becomes stale after its validity boundary', () => {
  const item = { ...success, current_available: 0, observed_at: now - 300 }
  expect(describeConnectionWindow(item, [item], now, t).label).toBe(
    'Snapshot available'
  )
  expect(describeConnectionWindow(item, [item], now + 1, t).label).toBe(
    'Stale sample'
  )
})

test('a generic saved error describes its own series rather than claiming the latest WHAM attempt failed', () => {
  const item = { ...failed, source: undefined }
  expect(describeConnectionWindow(item, [item], now, t)).toEqual({
    label: 'Usage sample failed',
    description:
      'This saved observation failed. Its timestamp applies only to this series.',
    variant: 'danger',
  })
})

test.each([undefined, 0, now + 1, Number.NaN])(
  'an unverified error timestamp %s cannot establish chronological history',
  (observed_at) => {
    const item = { ...failed, observed_at }
    expect(describeConnectionWindow(item, [item, success], now, t).label).toBe(
      'WHAM usage sample failed'
    )
  }
)
