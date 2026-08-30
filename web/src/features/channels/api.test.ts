/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { getChannelQuotaChanges, getChannelQuotaSamplingStatus } from './api'

vi.mock('@/lib/api', () => ({
  api: {
    get: vi.fn(),
  },
}))

describe('channel quota API authentication behavior', () => {
  test('keeps the standard auth refresh path for quota changes', async () => {
    vi.mocked(api.get).mockResolvedValueOnce({ data: { success: true, data: { items: [] } } } as never)

    await getChannelQuotaChanges({ range: '24h', limit: 5 })

    const config = vi.mocked(api.get).mock.calls[0]?.[1] as Record<string, unknown>
    expect(config.skipAuthRefresh).toBeUndefined()
    expect(config.skipErrorHandler).toBe(true)
    expect(config.skipBusinessError).toBe(true)
  })

  test('keeps the standard auth refresh path for sampler status', async () => {
    vi.mocked(api.get).mockResolvedValueOnce({ data: { success: true, data: { enabled: false } } } as never)

    await getChannelQuotaSamplingStatus()

    const config = vi.mocked(api.get).mock.calls[0]?.[1] as Record<string, unknown>
    expect(config.skipAuthRefresh).toBeUndefined()
    expect(config.skipErrorHandler).toBe(true)
    expect(config.skipBusinessError).toBe(true)
  })
})
