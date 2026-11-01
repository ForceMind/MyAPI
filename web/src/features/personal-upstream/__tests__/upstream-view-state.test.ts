import { expect, test } from 'vitest'

import type { ChannelQuotaChangeItem } from '@/features/channels/types'
import { connectionPageSearchSchema, connectionListContext } from '@/features/connections/connection-state'
import { connectionKeyContextSchema } from '@/features/connections/key-context'

import { filterUpstreamWindows, upstreamSearchSchema, upstreamWindowPeriod } from '../upstream-view-state'

const rows: ChannelQuotaChangeItem[] = [
  { channel_id: 7, name: 'Connection A', account_label: 'Account A', metric_type: 'codex_rate_limit', unit: 'percent', status: 'success', current_available: 0, observed_at: 1000, window_seconds: 18000 },
  { channel_id: 8, name: 'Connection B', metric_type: 'codex_rate_limit', unit: 'percent', status: 'error', current_available: null, observed_at: 1000, window_type: 'weekly' },
]

test('saved filters keep reported zero distinct from a missing value and only match returned records', () => {
  expect(filterUpstreamWindows(rows, { status: 'available' }, 1100)).toEqual([rows[0]])
  expect(filterUpstreamWindows(rows, { filter: 'account a', period: 'duration:18000' }, 1100)).toEqual([rows[0]])
  expect(filterUpstreamWindows(rows, { filter: '8', status: 'error' }, 1100)).toEqual([rows[1]])
  expect(filterUpstreamWindows(rows, { filter: 'not returned' }, 1100)).toEqual([])
  expect(upstreamWindowPeriod(rows[1])).toBe('type:weekly')
  expect(upstreamWindowPeriod({ ...rows[0], window_seconds: Number.NaN })).toBe('type:')
})

test('window return context survives connection and Key links while arbitrary URLs and credential fields are stripped', () => {
  const upstreamReturn = { channelId: 7, keyId: 21, filter: 'A', period: 'duration:18000', status: 'available' as const, changes: true, token: 'DO-NOT-STORE', url: 'https://invalid.test' }
  const parsed = connectionPageSearchSchema.parse({ connectionReturn: { upstreamReturn } }).connectionReturn
  const keys = connectionKeyContextSchema.parse({ connection: { ...connectionListContext(parsed ?? {}), connection: 7 } })
  expect(keys.connection.upstreamReturn).toEqual({ channelId: 7, keyId: 21, filter: 'A', period: 'duration:18000', status: 'available', changes: true })
  expect(JSON.stringify(keys)).not.toContain('DO-NOT-STORE')
  expect(JSON.stringify(keys)).not.toContain('invalid.test')
})

test('invalid account, Key, focus and state values cannot become a misleading selected observation', () => {
  expect(upstreamSearchSchema.parse({ channelId: -1, keyId: 0, focusChannelId: 1.5, status: 'healthy', changes: 'yes' })).toEqual({ channelId: undefined, keyId: undefined, focusChannelId: undefined, status: undefined, changes: undefined })
})
