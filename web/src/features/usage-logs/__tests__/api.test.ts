/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { getAllLogs, getUserLogs } from '../api'

vi.mock('@/lib/api', () => ({
  api: {
    get: vi.fn(),
  },
}))

const successResponse = {
  success: true,
  data: { items: [], total: 0, page: 1, page_size: 20 },
}

describe('usage log API scope', () => {
  test('requests self-scoped logs for a signed-in non-admin user', async () => {
    vi.mocked(api.get).mockResolvedValueOnce({
      data: successResponse,
    } as never)

    await getUserLogs({ p: 2, page_size: 20, request_id: 'req-mobile' })

    expect(api.get).toHaveBeenCalledWith(
      '/api/log/self?p=2&page_size=20&request_id=req-mobile'
    )
  })

  test('requests all logs only through the admin endpoint', async () => {
    vi.mocked(api.get).mockResolvedValueOnce({
      data: successResponse,
    } as never)

    await getAllLogs({ p: 1, page_size: 20, model_name: 'gpt-5' })

    expect(api.get).toHaveBeenCalledWith(
      '/api/log?p=1&page_size=20&model_name=gpt-5'
    )
  })
})
