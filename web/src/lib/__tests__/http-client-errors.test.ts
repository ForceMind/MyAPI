/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import axios, { AxiosError, CanceledError } from 'axios'
import { toast } from 'sonner'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/http-client'
import { useAuthStore } from '@/stores/auth-store'

const originalAuth = useAuthStore.getState()

afterEach(() => {
  useAuthStore.setState(originalAuth)
})

describe('HTTP error notifications', () => {
  test('an already aborted read rejects without a service-error notification', async () => {
    const showError = vi.spyOn(toast, 'error')
    const adapter = vi.fn()
    const controller = new AbortController()
    controller.abort()

    await expect(
      api.get('/api/log/stat', { signal: controller.signal, adapter })
    ).rejects.toMatchObject({ code: 'ERR_CANCELED' })

    expect(adapter).not.toHaveBeenCalled()
    expect(showError).not.toHaveBeenCalled()
  })

  test('a CancelToken rejection retains its identity without notifying', async () => {
    const showError = vi.spyOn(toast, 'error')
    const source = axios.CancelToken.source()
    source.cancel('The previous read is no longer needed')

    await expect(
      api.get('/api/log/stat', { cancelToken: source.token })
    ).rejects.toBe(source.token.reason)

    expect(showError).not.toHaveBeenCalled()
  })

  test('an adapter cancellation retains the original rejection without notifying', async () => {
    const showError = vi.spyOn(toast, 'error')
    const error = new CanceledError('The previous observer was removed')

    await expect(
      api.get('/api/log/stat', { adapter: () => Promise.reject(error) })
    ).rejects.toBe(error)

    expect(showError).not.toHaveBeenCalled()
  })

  test.each([
    [AxiosError.ECONNABORTED, 'timeout of 15000ms exceeded'],
    [AxiosError.ETIMEDOUT, 'request deadline exceeded'],
    [AxiosError.ERR_NETWORK, 'Network Error'],
  ])(
    '%s remains visible and rejects with the original error',
    async (code, message) => {
      const showError = vi.spyOn(toast, 'error')
      const error = new AxiosError(message, code)

      await expect(
        api.get('/api/log/stat', { adapter: () => Promise.reject(error) })
      ).rejects.toBe(error)

      expect(showError).toHaveBeenCalledExactlyOnceWith(message)
    }
  )

  test.each([
    [401, 'Session expired!'],
    [403, 'Access denied'],
    [503, 'Service unavailable'],
  ])(
    'HTTP %s remains visible and preserves the failure',
    async (status, message) => {
      const showError = vi.spyOn(toast, 'error')
      let failure: AxiosError | undefined

      const request = api.get('/api/log/stat', {
        skipAuthRefresh: true,
        adapter: (config) => {
          failure = new AxiosError(
            'Request failed',
            undefined,
            config,
            undefined,
            {
              status,
              statusText: 'Error',
              headers: {},
              config,
              data: { success: false, message },
            }
          )
          return Promise.reject(failure)
        },
      })

      await expect(request).rejects.toMatchObject({ response: { status } })
      await expect(request).rejects.toBe(failure)
      expect(showError).toHaveBeenCalledExactlyOnceWith(message)
    }
  )

  test('a non-cancellation error named canceled is not hidden', async () => {
    const showError = vi.spyOn(toast, 'error')
    const error = new Error('canceled')

    await expect(
      api.get('/api/log/stat', { adapter: () => Promise.reject(error) })
    ).rejects.toBe(error)

    expect(showError).toHaveBeenCalledExactlyOnceWith('canceled')
  })
})
