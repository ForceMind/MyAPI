import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { createApiKey } from '../api'

afterEach(() => vi.restoreAllMocks())

const payload = { name: 'Synthetic device', remain_quota: 0, unlimited_quota: false, expired_time: -1, model_limits_enabled: false, model_limits: '', allow_ips: '', group: '', access_profile_id: 'standard', auto_groups: [], cross_group_retry: false }

test('creation projects only the returned identity and never credentials into its result', async () => {
  vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true, data: { id: 31, key: 'never-retain', name: 'do-not-need-full-key' } } })
  expect(await createApiKey(payload, { skipAuthRefresh: true })).toEqual({ success: true, message: undefined, data: { id: 31 } })
  expect(api.post).toHaveBeenCalledExactlyOnceWith('/api/token/', payload, { skipAuthRefresh: true })
})

test.each([undefined, {}, { id: 0 }, { id: -1 }, { id: 1.5 }, { id: '31' }])('a business-success creation with absent or invalid identity %j remains successful and is not replayed', async (data) => {
  vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true, data } })
  expect(await createApiKey(payload)).toEqual({ success: true, message: undefined, data: undefined })
  expect(api.post).toHaveBeenCalledOnce()
})
