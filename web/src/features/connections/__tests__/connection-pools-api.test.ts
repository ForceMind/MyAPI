/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { getConnectionPools } from '../connection-pools-api'
import { connectionKeyContextSchema } from '../key-context'

afterEach(() => vi.restoreAllMocks())

const pool = {
  pool_id: 7,
  channel_id: 1,
  window_source: 'primary',
  window_type: 'five_hour',
  enabled: true,
}

test('reads the requested channel page with independent cancellation', async () => {
  const signal = new AbortController().signal
  const read = vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: { channel_id: 1, items: [pool], has_more: true },
    },
  })
  expect((await getConnectionPools(1, 2, signal)).has_more).toBe(true)
  expect(read).toHaveBeenCalledWith(
    '/api/subscription-share/channels/1/pools',
    expect.objectContaining({
      params: { page: 2 },
      signal,
      disableDuplicate: true,
      skipAuthRefresh: true,
    })
  )
})

for (const corruption of [
  'channel',
  'pool-channel',
  'duplicate',
  'invalid-id',
  'failed',
] as const) {
  test(`rejects ${corruption} rather than showing false associations`, async () => {
    const data = { channel_id: 1, items: [{ ...pool }], has_more: false }
    if (corruption === 'channel') data.channel_id = 2
    if (corruption === 'pool-channel') data.items[0].channel_id = 2
    if (corruption === 'duplicate') data.items.push({ ...pool })
    if (corruption === 'invalid-id') data.items[0].pool_id = 0
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: corruption !== 'failed', data },
    })
    await expect(getConnectionPools(1, 1)).rejects.toThrow()
  })
}

test('context keeps stable numeric IDs and original inventory state without arbitrary return URLs', () => {
  expect(
    connectionKeyContextSchema.parse({
      connection: {
        connection: 1,
        page: 3,
        filter: 'same name',
        status: 'disabled',
      },
      pool: 7,
      key: 21,
      returnUrl: 'https://example.invalid',
    })
  ).toEqual({
    connection: {
      connection: 1,
      page: 3,
      filter: 'same name',
      status: 'disabled',
    },
    pool: 7,
    key: 21,
  })
  expect(
    connectionKeyContextSchema.safeParse({
      connection: { connection: 'same name' },
      key: '21',
    }).success
  ).toBe(false)
})
