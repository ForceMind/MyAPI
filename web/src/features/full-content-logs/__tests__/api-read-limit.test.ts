import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { getFullContentLogDetail } from '../api'

afterEach(() => vi.restoreAllMocks())

test.each([
  ['default preview', undefined, 1024 * 1024],
  ['explicit larger read', 16 * 1024 * 1024, 16 * 1024 * 1024],
] as const)('%s retains a finite same-request body read budget', async (_name, requested, expected) => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: {} } })
  await getFullContentLogDetail('saved/request-id', undefined, requested)
  expect(get).toHaveBeenCalledExactlyOnceWith('/api/full-content-logs/saved%2Frequest-id', expect.objectContaining({ params: { max_response_bytes: expected } }))
})
