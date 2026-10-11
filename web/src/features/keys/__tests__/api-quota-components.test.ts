import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { getApiKey, getApiKeys, searchApiKeys } from '../api'

function apiKey(overrides: Record<string, unknown> = {}) {
  return {
    id: 7,
    name: 'Accounting key',
    key: 'synthetic-masked',
    status: 1,
    remain_quota: 100,
    used_quota: 50,
    quota_version: '9',
    metadata_fingerprint: 'a'.repeat(64),
    unlimited_quota: false,
    expired_time: -1,
    created_time: 0,
    accessed_time: 0,
    group: 'default',
    model_limits_enabled: false,
    model_limits: '',
    allow_ips: '',
    ...overrides,
  }
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('API key quota component parsing', () => {
  test('accepts exact safe-integer boundaries from the detail endpoint', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: apiKey({
          remain_quota: Number.MIN_SAFE_INTEGER,
          used_quota: Number.MAX_SAFE_INTEGER,
        }),
      },
    })

    await expect(getApiKey(7)).resolves.toMatchObject({
      data: {
        remain_quota: Number.MIN_SAFE_INTEGER,
        used_quota: Number.MAX_SAFE_INTEGER,
      },
    })
  })

  test('rejects an unsafe remaining component from the detail endpoint', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: apiKey({ remain_quota: Number.MAX_SAFE_INTEGER + 1 }),
      },
    })

    await expect(getApiKey(7)).rejects.toThrow()
  })

  test('rejects a non-integer remaining component from the list endpoint', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: {
          items: [apiKey({ remain_quota: 1.5 })],
          total: 1,
          page: 1,
          page_size: 20,
        },
      },
    })

    await expect(getApiKeys()).rejects.toThrow()
  })

  test('rejects a negative used component from the search endpoint', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: {
          items: [apiKey({ used_quota: -1 })],
          total: 1,
          page: 1,
          page_size: 20,
        },
      },
    })

    await expect(searchApiKeys({ keyword: 'Accounting' })).rejects.toThrow()
  })
})
