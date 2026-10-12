import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { getSetupStatus } from '../api'

afterEach(() => {
  vi.useRealTimers()
})

describe('installation status reads', () => {
  test('rejects a stalled read within its deadline instead of keeping startup pending', async () => {
    vi.useFakeTimers()
    vi.spyOn(api, 'get').mockImplementation(() => new Promise(() => {}))
    let outcome = 'pending'
    void getSetupStatus().then(
      () => { outcome = 'success' },
      () => { outcome = 'failed' }
    )

    await vi.advanceTimersByTimeAsync(15000)

    expect(outcome).toBe('failed')
  })

  test.each([
    { success: false, message: 'Status unavailable' },
    { success: true },
    { success: true, data: { status: 'false', root_init: false, database_type: 'sqlite' } },
  ])('rejects an unavailable or invalid setup result: %j', async (body) => {
    vi.spyOn(api, 'get').mockResolvedValue({ data: body })

    await expect(getSetupStatus()).rejects.toThrow()
  })

  test('preserves an explicitly uninitialized installation result', async () => {
    const body = {
      success: true,
      data: { status: false, root_init: false, database_type: 'sqlite' },
    }
    vi.spyOn(api, 'get').mockResolvedValue({ data: body })

    await expect(getSetupStatus()).resolves.toEqual(body)
  })
})
