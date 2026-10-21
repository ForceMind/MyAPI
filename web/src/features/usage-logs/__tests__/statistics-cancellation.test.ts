/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { CanceledError } from 'axios'
import { toast } from 'sonner'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { getUserLogStats } from '../api'

const originalAdapter = api.defaults.adapter
const originalAuth = useAuthStore.getState()
afterEach(() => {
  api.defaults.adapter = originalAdapter
  useAuthStore.setState(originalAuth)
})

test('a new statistics observer does not reuse its cancelled predecessor in the same session', async () => {
  const showError = vi.spyOn(toast, 'error')
  let firstSeen = () => {}
  const started = new Promise<void>((resolve) => {
    firstSeen = resolve
  })
  let dispatches = 0
  api.defaults.adapter = (config) => {
    dispatches++
    if (dispatches === 1) {
      const pending = new Promise<never>((_resolve, reject) => {
        config.signal?.addEventListener?.(
          'abort',
          () => reject(new CanceledError('synthetic cancellation', config)),
          { once: true }
        )
      })
      firstSeen()
      return pending
    }
    return Promise.resolve({
      data: { success: true, data: { quota: 0, rpm: 0, tpm: 7 } },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    })
  }
  const previous = new AbortController()
  const first = getUserLogStats({}, previous.signal).catch(
    (error: unknown) => error
  )
  await started
  const next = getUserLogStats({}, new AbortController().signal)
  previous.abort()
  const response = await next
  expect(response.success).toBe(true)
  expect(response.data?.tpm).toBe(7)
  expect(await first).toHaveProperty('code', 'ERR_CANCELED')
  expect(dispatches).toBe(2)
  expect(showError).not.toHaveBeenCalled()
})
