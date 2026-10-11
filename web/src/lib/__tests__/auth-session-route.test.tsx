/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import type { AuthBundle } from '../../stores/auth-store'

const axiosHarness = vi.hoisted(() => ({
  post: vi.fn(),
}))

vi.mock('axios', () => ({
  default: {
    create: () => ({
      post: axiosHarness.post,
    }),
    isAxiosError: (error: unknown) =>
      Boolean(
        error &&
        typeof error === 'object' &&
        'isAxiosError' in error &&
        error.isAxiosError
      ),
  },
}))

const originalLocksDescriptor = Object.getOwnPropertyDescriptor(
  navigator,
  'locks'
)

const makeBundle = (sid: string, accessToken: string): AuthBundle => ({
  access_token: accessToken,
  token_type: 'Bearer',
  access_expires_at: 2_100_000_000,
  user: {
    id: 42,
    username: `user-${sid}`,
    display_name: `User ${sid}`,
    role: 1,
    status: 1,
    group: 'default',
  },
  session: {
    sid,
    current: true,
    login_method: 'password',
    ip: '127.0.0.1',
    user_agent: 'vitest',
    created_at: 1_900_000_000,
    last_active_at: 1_900_000_000,
    expires_at: 2_100_003_600,
  },
})

const makeRefreshResponse = (bundle: AuthBundle) => ({
  status: 200,
  data: {
    success: true,
    data: bundle,
  },
})

const expiredResponseCases = [
  {
    name: 'success',
    response: makeRefreshResponse(makeBundle('sid-late', 'token-late')),
  },
  {
    name: 'refresh race',
    response: {
      status: 409,
      data: { success: false, code: 'AUTH_REFRESH_RACE' },
    },
  },
  {
    name: 'unauthorized',
    response: { status: 401, data: { success: false } },
  },
] as const

const makeDeferred = <T,>() => {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

const makeAbortablePendingRequest = (signal?: AbortSignal) =>
  new Promise<never>((_resolve, reject) => {
    const rejectAsCanceled = () =>
      reject({
        isAxiosError: true,
        message: 'request canceled',
      })

    if (signal?.aborted) {
      rejectAsCanceled()
      return
    }
    signal?.addEventListener('abort', rejectAsCanceled, { once: true })
  })

const setNavigatorLocks = (locks: unknown) => {
  Object.defineProperty(navigator, 'locks', {
    configurable: true,
    value: locks,
  })
}

const loadAuthRuntime = async () => {
  vi.resetModules()
  const authSession = await import('../auth-session')
  const { useAuthStore } = await import('../../stores/auth-store')
  return { authSession, useAuthStore }
}

describe('auth refresh route deadline', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2030-01-01T00:00:00.000Z'))
    axiosHarness.post.mockReset()
    setNavigatorLocks(undefined)
  })

  afterEach(() => {
    vi.clearAllTimers()
    vi.useRealTimers()
    axiosHarness.post.mockReset()
    if (originalLocksDescriptor) {
      Object.defineProperty(navigator, 'locks', originalLocksDescriptor)
    } else {
      Reflect.deleteProperty(navigator, 'locks')
    }
  })

  test('bounds the initial bootstrap refresh when the HTTP request hangs', async () => {
    axiosHarness.post.mockImplementation(
      (_url: string, _body: unknown, config?: { signal?: AbortSignal }) =>
        makeAbortablePendingRequest(config?.signal)
    )
    const { authSession, useAuthStore } = await loadAuthRuntime()

    let outcome: Awaited<
      ReturnType<typeof authSession.bootstrapAuthentication>
    > | null = null
    void authSession.bootstrapAuthentication().then((result) => {
      outcome = result
    })
    await Promise.resolve()

    expect(useAuthStore.getState().auth.bootstrapState).toBe('checking')
    expect(axiosHarness.post).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(10_001)

    expect(outcome).toMatchObject({ kind: 'transient_error' })
    expect(useAuthStore.getState().auth.bootstrapState).toBe('idle')
  })

  test('bounds bootstrap while queued for the Web Lock without sending HTTP', async () => {
    const lockRequest = vi.fn(
      (
        _name: string,
        options: { signal?: AbortSignal },
        _callback: () => unknown
      ) =>
        new Promise<never>((_resolve, reject) => {
          const rejectAsCanceled = () =>
            reject(
              options.signal?.reason ??
                new DOMException('Aborted', 'AbortError')
            )
          if (options.signal?.aborted) {
            rejectAsCanceled()
            return
          }
          options.signal?.addEventListener('abort', rejectAsCanceled, {
            once: true,
          })
        })
    )
    setNavigatorLocks({ request: lockRequest })
    const { authSession, useAuthStore } = await loadAuthRuntime()

    let outcome: Awaited<
      ReturnType<typeof authSession.bootstrapAuthentication>
    > | null = null
    void authSession.bootstrapAuthentication().then((result) => {
      outcome = result
    })
    await Promise.resolve()

    expect(lockRequest).toHaveBeenCalledTimes(1)
    expect(axiosHarness.post).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(10_001)

    expect(outcome).toMatchObject({ kind: 'transient_error' })
    expect(useAuthStore.getState().auth.bootstrapState).toBe('idle')
    expect(axiosHarness.post).not.toHaveBeenCalled()
  })

  test('starts a new refresh after a stalled 401 refresh reaches its deadline', async () => {
    axiosHarness.post
      .mockImplementationOnce(
        (_url: string, _body: unknown, config?: { signal?: AbortSignal }) =>
          makeAbortablePendingRequest(config?.signal)
      )
      .mockResolvedValueOnce(
        makeRefreshResponse(makeBundle('sid-2', 'token-2'))
      )
    const { authSession, useAuthStore } = await loadAuthRuntime()

    let firstOutcome: Awaited<
      ReturnType<typeof authSession.refreshAuthentication>
    > | null = null
    void authSession.refreshAuthentication().then((result) => {
      firstOutcome = result
    })
    await Promise.resolve()

    const firstConfig = axiosHarness.post.mock.calls[0]?.[2] as
      | { signal?: AbortSignal; timeout?: number }
      | undefined
    expect(firstConfig?.signal).toBeInstanceOf(AbortSignal)
    expect(firstConfig?.timeout).toBeGreaterThan(0)
    expect(firstConfig?.timeout).toBeLessThanOrEqual(10_000)

    await vi.advanceTimersByTimeAsync(10_001)

    expect(firstOutcome).toMatchObject({ kind: 'transient_error' })

    const retryOutcome = await authSession.refreshAuthentication()

    expect(axiosHarness.post).toHaveBeenCalledTimes(2)
    expect(retryOutcome).toMatchObject({ kind: 'authenticated' })
    expect(useAuthStore.getState().auth.session?.sid).toBe('sid-2')
  })

  test('ignores a timed-out HTTP result and keeps the newer refresh singleton', async () => {
    const staleRequest = makeDeferred<ReturnType<typeof makeRefreshResponse>>()
    const currentRequest =
      makeDeferred<ReturnType<typeof makeRefreshResponse>>()
    axiosHarness.post
      .mockImplementationOnce(() => staleRequest.promise)
      .mockImplementationOnce(() => currentRequest.promise)
    const { authSession, useAuthStore } = await loadAuthRuntime()

    let staleOutcome: Awaited<
      ReturnType<typeof authSession.refreshAuthentication>
    > | null = null
    void authSession.refreshAuthentication().then((result) => {
      staleOutcome = result
    })
    await vi.advanceTimersByTimeAsync(9_000)

    const newerLogin = makeBundle('sid-new-login', 'token-new-login')
    authSession.applyAuthBundle(newerLogin, false)
    await vi.advanceTimersByTimeAsync(1_001)

    expect(staleOutcome).toMatchObject({ kind: 'transient_error' })
    expect(useAuthStore.getState().auth).toMatchObject({
      accessToken: 'token-new-login',
      bootstrapState: 'complete',
      session: { sid: 'sid-new-login' },
    })

    const currentPromise = authSession.refreshAuthentication()
    expect(axiosHarness.post).toHaveBeenCalledTimes(2)

    staleRequest.resolve(
      makeRefreshResponse(makeBundle('sid-stale', 'token-stale'))
    )
    await Promise.resolve()
    await Promise.resolve()

    expect(useAuthStore.getState().auth).toMatchObject({
      accessToken: 'token-new-login',
      session: { sid: 'sid-new-login' },
    })
    expect(authSession.refreshAuthentication()).toBe(currentPromise)
    expect(axiosHarness.post).toHaveBeenCalledTimes(2)

    currentRequest.resolve(
      makeRefreshResponse(makeBundle('sid-current', 'token-current'))
    )
    await expect(currentPromise).resolves.toMatchObject({
      kind: 'authenticated',
    })
    expect(useAuthStore.getState().auth).toMatchObject({
      accessToken: 'token-current',
      session: { sid: 'sid-current' },
    })
  })

  test('keeps an acquired Web Lock until an abort-ignoring HTTP request settles', async () => {
    let lockTail: Promise<unknown> = Promise.resolve()
    const lockRequest = vi.fn(
      (
        _name: string,
        _options: { signal?: AbortSignal },
        callback: () => unknown
      ) => {
        const previousLock = lockTail
        const result = (async () => {
          await previousLock
          return callback()
        })()
        lockTail = result.then(
          () => undefined,
          () => undefined
        )
        return result
      }
    )
    setNavigatorLocks({ request: lockRequest })
    const staleRequest = makeDeferred<ReturnType<typeof makeRefreshResponse>>()
    axiosHarness.post
      .mockImplementationOnce(() => staleRequest.promise)
      .mockResolvedValueOnce(
        makeRefreshResponse(makeBundle('sid-current', 'token-current'))
      )
    const { authSession, useAuthStore } = await loadAuthRuntime()

    let firstOutcome: Awaited<
      ReturnType<typeof authSession.bootstrapAuthentication>
    > | null = null
    void authSession.bootstrapAuthentication().then((result) => {
      firstOutcome = result
    })
    await Promise.resolve()
    await Promise.resolve()
    expect(axiosHarness.post).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(10_001)

    expect(firstOutcome).toMatchObject({ kind: 'transient_error' })
    let secondOutcome: Awaited<
      ReturnType<typeof authSession.refreshAuthentication>
    > | null = null
    void authSession.refreshAuthentication().then((result) => {
      secondOutcome = result
    })
    for (let index = 0; index < 8; index += 1) {
      await Promise.resolve()
    }

    expect(lockRequest).toHaveBeenCalledTimes(2)
    expect(axiosHarness.post).toHaveBeenCalledTimes(1)
    expect(secondOutcome).toBe(null)

    staleRequest.resolve(
      makeRefreshResponse(makeBundle('sid-stale', 'token-stale'))
    )
    for (let index = 0; index < 20; index += 1) {
      await Promise.resolve()
    }

    expect(axiosHarness.post).toHaveBeenCalledTimes(2)
    expect(secondOutcome).toMatchObject({ kind: 'authenticated' })
    expect(useAuthStore.getState().auth).toMatchObject({
      accessToken: 'token-current',
      session: { sid: 'sid-current' },
    })
  })

  test.each(expiredResponseCases)(
    'rejects an expired $name response before the deadline timer runs',
    async ({ response }) => {
      const request =
        makeDeferred<(typeof expiredResponseCases)[number]['response']>()
      axiosHarness.post.mockImplementationOnce(() => request.promise)
      const { authSession, useAuthStore } = await loadAuthRuntime()
      const existingBundle = {
        ...makeBundle('sid-existing', 'token-existing'),
        access_expires_at: Math.floor(Date.now() / 1000) - 1,
      }
      authSession.applyAuthBundle(existingBundle, false)

      let outcome: Awaited<
        ReturnType<typeof authSession.bootstrapAuthentication>
      > | null = null
      void authSession.bootstrapAuthentication().then((result) => {
        outcome = result
      })
      await Promise.resolve()

      const signal = (
        axiosHarness.post.mock.calls[0]?.[2] as
          | { signal?: AbortSignal }
          | undefined
      )?.signal
      expect(useAuthStore.getState().auth.bootstrapState).toBe('checking')
      expect(signal?.aborted).toBe(false)

      vi.setSystemTime(new Date(Date.now() + 10_001))
      request.resolve(response)
      for (let index = 0; index < 12; index += 1) {
        await Promise.resolve()
      }

      expect(outcome).toMatchObject({ kind: 'transient_error' })
      expect(useAuthStore.getState().auth).toMatchObject({
        accessToken: 'token-existing',
        bootstrapState: 'idle',
        session: { sid: 'sid-existing' },
      })
      expect(signal?.aborted).toBe(true)
      expect(axiosHarness.post).toHaveBeenCalledTimes(1)
      expect(vi.getTimerCount()).toBe(0)
    }
  )
})
