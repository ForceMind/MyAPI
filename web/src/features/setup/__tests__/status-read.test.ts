import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { getSetupStatus } from '../api'

afterEach(() => {
  vi.useRealTimers()
})

describe('installation status reads', () => {
  test('rejects a stalled read within its deadline instead of keeping startup pending', async () => {
    vi.useFakeTimers()
    let aborted = false
    vi.spyOn(api, 'get').mockImplementation((_url, config) => {
      config?.signal?.addEventListener?.('abort', () => {
        aborted = true
      })
      return new Promise(() => {})
    })
    let outcome = 'pending'
    void getSetupStatus().then(
      () => {
        outcome = 'success'
      },
      () => {
        outcome = 'failed'
      }
    )

    await vi.advanceTimersByTimeAsync(15000)

    expect(outcome).toBe('failed')
    expect(aborted).toBe(true)
  })

  test.each([
    { success: false, message: 'Status unavailable' },
    { success: true },
    {
      success: true,
      data: { status: 'false', root_init: false, database_type: 'sqlite' },
    },
    {
      success: true,
      data: { status: false, root_init: 'false', database_type: 'sqlite' },
    },
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

  test('cancels an abandoned read without waiting for the adapter to settle', async () => {
    const controller = new AbortController()
    vi.spyOn(api, 'get').mockImplementation(() => new Promise(() => {}))
    const read = getSetupStatus(controller.signal)

    controller.abort()

    await expect(read).rejects.toThrow('Failed to load setup status')
  })

  test('does not send a request when navigation has already been cancelled', async () => {
    const controller = new AbortController()
    controller.abort()
    const get = vi.spyOn(api, 'get')

    await expect(getSetupStatus(controller.signal)).rejects.toThrow()

    expect(get).not.toHaveBeenCalled()
  })
})
