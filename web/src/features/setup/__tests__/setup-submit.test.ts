import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SetupSubmissionUnknownError, submitSetup } from '../api'

afterEach(() => {
  vi.useRealTimers()
})

describe('installation submission', () => {
  test('rejects a stalled public request within the total deadline even when the adapter ignores cancellation', async () => {
    vi.useFakeTimers()
    let aborted = false
    let rejectAdapter!: (error: Error) => void
    vi.spyOn(api, 'post').mockImplementation((_url, _payload, config) => {
      config?.signal?.addEventListener?.('abort', () => {
        aborted = true
      })
      return new Promise((_resolve, reject) => {
        rejectAdapter = reject
      })
    })
    let outcome = 'pending'
    void submitSetup({ username: 'root' }).then(
      () => {
        outcome = 'success'
      },
      () => {
        outcome = 'unknown'
      }
    )

    await vi.advanceTimersByTimeAsync(15000)

    expect(outcome).toBe('unknown')
    expect(aborted).toBe(true)
    expect(api.post).toHaveBeenCalledWith(
      '/api/setup',
      { username: 'root' },
      expect.objectContaining({
        timeout: 15000,
        skipAuthRefresh: true,
        skipBusinessError: true,
        skipErrorHandler: true,
      })
    )

    rejectAdapter(new Error('late adapter failure'))
    await vi.advanceTimersByTimeAsync(0)
    expect(outcome).toBe('unknown')
  })

  test('preserves an immediate business failure as a known result', async () => {
    const response = { success: false, message: 'Invalid configuration' }
    vi.spyOn(api, 'post').mockResolvedValue({ data: response })

    await expect(submitSetup({})).resolves.toEqual(response)
  })

  test.each([
    ['an HTTP failure', new Error('Request failed with status code 503')],
    ['a transport failure', new Error('Network Error')],
  ])('treats %s as an unknown result', async (_name, error) => {
    vi.spyOn(api, 'post').mockRejectedValue(error)

    await expect(submitSetup({})).rejects.toBeInstanceOf(
      SetupSubmissionUnknownError
    )
  })

  test('treats an invalid response body as an unknown result', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({ data: { message: 'missing result' } })

    await expect(submitSetup({})).rejects.toBeInstanceOf(
      SetupSubmissionUnknownError
    )
  })
})
