import { beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { getCodexUsageHistory } from '../api'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), post: vi.fn() } }))
beforeEach(() => vi.resetAllMocks())

test('account history passes the selected opaque series without dropping default range and limit', async () => {
  const series = 'a'.repeat(64)
  const response = { success: true, data: { series_id: series, identity_quality: 'provider_confirmed', points: [] } }
  vi.mocked(api.get).mockResolvedValue({ data: response } as never)
  expect(await getCodexUsageHistory(7, { series_id: series })).toEqual(response)
  expect(api.get).toHaveBeenCalledWith('/api/channel/7/codex/usage/history', expect.objectContaining({
    disableDuplicate: true,
    params: { range: '30d', limit: 500, series_id: series },
  }))
})
