/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { describe, expect, test } from 'vitest'

import type { ChannelQuotaChangeItem } from '@/features/channels/types'
import type { UsageLog } from '@/features/usage-logs/data/schema'

import {
  buildKeyNodes,
  hourlyBuckets,
  medianSeconds,
  overviewSearchSchema,
  selectionFromSearch,
  tightestWindowByConnection,
  trafficBy,
  visibleNodes,
} from '../overview-flow'

const HOUR = 3600
// 2026-10-25 10:30:00 local time keeps the bucket boundaries readable.
const NOW = Math.floor(new Date(2026, 9, 25, 10, 30, 0).getTime() / 1000)

function log(overrides: Partial<UsageLog>): UsageLog {
  return {
    id: 1,
    user_id: 1,
    created_at: NOW - 60,
    type: 2,
    content: '',
    username: '',
    token_name: 'Key',
    token_id: 21,
    model_name: 'synthetic-model',
    quota: 0,
    prompt_tokens: 10,
    completion_tokens: 5,
    use_time: 2,
    is_stream: false,
    channel: 1,
    channel_name: 'Connection',
    group: '',
    ip: '',
    other: JSON.stringify({
      request_outcome: 'completed',
      usage_outcome: 'reported',
    }),
    request_id: 'REQ',
    ...overrides,
  } as UsageLog
}

const failed = JSON.stringify({ request_outcome: 'failed' })

describe('traffic per stable identity', () => {
  test('counts requests and failures per Key ID, never per Key name', () => {
    const rows = [
      log({ id: 1, token_id: 21, token_name: 'Same name' }),
      log({ id: 2, token_id: 22, token_name: 'Same name', other: failed }),
      log({ id: 3, token_id: 21, token_name: 'Renamed', other: failed }),
    ]
    const traffic = trafficBy(rows, (row) => row.token_id)
    expect(traffic.get(21)).toEqual({ requests: 2, failures: 1 })
    expect(traffic.get(22)).toEqual({ requests: 1, failures: 1 })
  })

  test('rows without a Key ID or connection are not attributed to any node', () => {
    const rows = [log({ token_id: 0, channel: 0 })]
    expect(trafficBy(rows, (row) => row.token_id).size).toBe(0)
    expect(trafficBy(rows, (row) => row.channel).size).toBe(0)
  })

  test('counts per connection use the channel ID', () => {
    const rows = [log({ channel: 3 }), log({ channel: 3, other: failed })]
    expect(trafficBy(rows, (row) => row.channel).get(3)).toEqual({
      requests: 2,
      failures: 1,
    })
  })
})

describe('Key nodes', () => {
  const keys = [
    { id: 21, name: 'Desktop', status: 1 },
    { id: 22, name: 'Paused', status: 2 },
  ]

  test('own Keys keep their saved name and status and are ordered by recent requests', () => {
    const rows = [log({ token_id: 22 }), log({ token_id: 22 })]
    const nodes = buildKeyNodes(keys, rows, 1)
    expect(nodes.map((node) => [node.id, node.requests, node.status])).toEqual([
      [22, 2, 2],
      [21, 0, 1],
    ])
  })

  test('without a readable sample the request count is unknown, not zero', () => {
    const nodes = buildKeyNodes(keys, undefined, 1)
    expect(nodes.map((node) => node.requests)).toEqual([null, null])
    expect(nodes.map((node) => node.failures)).toEqual([null, null])
  })

  test('a Key seen only in the call records keeps the logged name and has no status', () => {
    const rows = [log({ token_id: 77, token_name: 'Teammate Key', user_id: 9 })]
    const node = buildKeyNodes(keys, rows, 1).find((item) => item.id === 77)
    expect(node).toMatchObject({
      name: 'Teammate Key',
      status: null,
      own: false,
      requests: 1,
    })
  })
})

describe('folding', () => {
  const nodes = Array.from({ length: 8 }, (_, index) => ({ id: index + 1 }))

  test('six or fewer nodes are all shown', () => {
    expect(visibleNodes(nodes.slice(0, 6), undefined)).toHaveLength(6)
  })

  test('more than six nodes show the first six', () => {
    expect(visibleNodes(nodes, undefined).map((node) => node.id)).toEqual([
      1, 2, 3, 4, 5, 6,
    ])
  })

  test('a selected node beyond the first six stays visible in the last slot', () => {
    expect(visibleNodes(nodes, 8).map((node) => node.id)).toEqual([
      1, 2, 3, 4, 5, 8,
    ])
  })
})

describe('hourly buckets', () => {
  test('24 local hours ending with the current hour, counted from the rows', () => {
    const rows = [
      log({ created_at: NOW - 60 }),
      log({ created_at: NOW - 2 * HOUR }),
      log({ created_at: NOW - 2 * HOUR + 10 }),
    ]
    const buckets = hourlyBuckets(rows, NOW, NOW - 24 * HOUR)
    expect(buckets).toHaveLength(24)
    expect(new Date(buckets[23].start * 1000).getHours()).toBe(10)
    expect(buckets[23].count).toBe(1)
    expect(buckets[21].count).toBe(2)
    expect(buckets.every((bucket) => bucket.covered)).toBe(true)
  })

  test('hours before a truncated sample are not covered instead of zero', () => {
    const rows = [log({ created_at: NOW - 3 * HOUR })]
    const buckets = hourlyBuckets(rows, NOW, NOW - 3 * HOUR)
    const uncovered = buckets.filter((bucket) => !bucket.covered)
    expect(uncovered).toHaveLength(20)
    expect(uncovered.every((bucket) => bucket.count === 0)).toBe(true)
    expect(buckets[20]).toMatchObject({ covered: true, count: 1 })
  })
})

describe('median duration', () => {
  test('uses positive durations only and is unknown when none is reported', () => {
    expect(medianSeconds([3, 1, 2, 0])).toBe(2)
    expect(medianSeconds([4, 1])).toBe(2.5)
    expect(medianSeconds([0, Number.NaN])).toBeUndefined()
  })
})

describe('account windows per connection', () => {
  const window = (
    overrides: Partial<ChannelQuotaChangeItem>
  ): ChannelQuotaChangeItem => ({
    channel_id: 1,
    name: 'Account',
    metric_type: 'codex_rate_limit',
    unit: 'percent',
    status: 'success',
    window_type: 'five_hour',
    current_available: 50,
    ...overrides,
  })

  test('the most used window represents the connection', () => {
    const map = tightestWindowByConnection([
      window({ window_type: 'weekly', current_available: 70 }),
      window({ window_type: 'five_hour', current_available: 18 }),
      window({ channel_id: 2, current_available: 90 }),
    ])
    expect(map.get(1)?.window_type).toBe('five_hour')
    expect(map.get(2)?.current_available).toBe(90)
  })

  test('a window without a trustworthy sample is kept but never outranks a measured one', () => {
    const map = tightestWindowByConnection([
      window({ status: 'error', current_available: null }),
      window({ window_type: 'weekly', current_available: 95 }),
      window({ channel_id: 3, status: 'error', current_available: null }),
    ])
    expect(map.get(1)?.window_type).toBe('weekly')
    expect(map.get(3)?.status).toBe('error')
  })

  test('other metrics are ignored', () => {
    const map = tightestWindowByConnection([
      window({ metric_type: 'balance', unit: 'usd' }),
    ])
    expect(map.size).toBe(0)
  })
})

describe('selection in the URL', () => {
  const parse = (search: Record<string, unknown>) =>
    selectionFromSearch(overviewSearchSchema.parse(search))

  test('a Key ID from a shared link selects that Key', () => {
    expect(parse({ key: 21 })).toEqual({ kind: 'key', id: 21 })
    expect(parse({ key: '21' })).toEqual({ kind: 'key', id: 21 })
  })

  test('a connection ID selects that connection', () => {
    expect(parse({ connection: 3 })).toEqual({ kind: 'connection', id: 3 })
  })

  test('invalid IDs select nothing instead of failing the page', () => {
    expect(parse({ key: 'abc', connection: -1 })).toBeNull()
    expect(parse({ key: 2147483648 })).toBeNull()
  })

  test('a Key wins when both are present', () => {
    expect(parse({ key: 21, connection: 3 })).toEqual({ kind: 'key', id: 21 })
  })
})
