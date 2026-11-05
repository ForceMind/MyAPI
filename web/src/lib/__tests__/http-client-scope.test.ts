import { AxiosError, AxiosHeaders, type AxiosAdapter } from 'axios'
import { toast } from 'sonner'
import { afterEach, expect, test, vi } from 'vitest'

import { handleServerError, skipGlobalErrorHandling } from '@/lib/handle-server-error'
import { api } from '@/lib/http-client'

afterEach(() => vi.restoreAllMocks())

test('an inactive opt-in scope cancels before dispatch without a service-error notification', async () => {
  const adapter = vi.fn()
  const error = vi.spyOn(toast, 'error')
  await expect(api.get('/api/scoped-read', { adapter, isCurrentScope: () => false })).rejects.toMatchObject({ code: 'ERR_CANCELED' })
  expect(adapter).not.toHaveBeenCalled()
  expect(error).not.toHaveBeenCalled()
})

test.each(['business', '401', '500'] as const)('a %s response after the UI scope ends cannot notify or refresh the current session', async (kind) => {
  let active = true
  let release!: () => void
  const pending = new Promise<void>((resolve) => { release = resolve })
  const adapter = vi.fn<AxiosAdapter>(async (config) => {
    await pending
    if (kind === 'business') return { config, status: 200, statusText: 'OK', headers: {}, data: { success: false, message: 'Synthetic old task failure' } }
    throw new AxiosError('Synthetic old task failure', 'ERR_BAD_RESPONSE', config, undefined, {
      config, status: Number(kind), statusText: 'Failed', headers: {}, data: { message: 'Synthetic old task failure' },
    })
  })
  const error = vi.spyOn(toast, 'error')
  const request = api.get('/api/scoped-read', { adapter, isCurrentScope: () => active })
  const outcome = request.catch((failure: unknown) => failure)
  active = false
  release()
  const failure = await outcome
  expect(failure).toMatchObject({ code: 'ERR_CANCELED' })
  handleServerError(failure)
  expect(adapter).toHaveBeenCalledOnce()
  expect(error).not.toHaveBeenCalled()
})

test('active opt-in scopes do not share another observer lifetime through GET deduplication', async () => {
  const adapter = vi.fn<AxiosAdapter>(async (config) => ({ config, status: 200, statusText: 'OK', headers: {}, data: { success: true } }))
  const [first, second] = await Promise.all([
    api.get('/api/scoped-read', { adapter, isCurrentScope: () => true }),
    api.get('/api/scoped-read', { adapter, isCurrentScope: () => true }),
  ])
  expect(first.data).toEqual({ success: true })
  expect(second.data).toEqual({ success: true })
  expect(adapter).toHaveBeenCalledTimes(2)
})

test('a real failure whose message says canceled is still visible in the generic UI handler', () => {
  const error = vi.spyOn(toast, 'error')
  vi.spyOn(console, 'log').mockImplementation(() => {})
  handleServerError(new Error('canceled'))
  expect(error).toHaveBeenCalledOnce()
})

test('local failure UI is not replaced by global 500 handling, while ordinary failures retain the default policy', () => {
  const localConfig = { headers: new AxiosHeaders(), skipErrorHandler: true }
  const ordinaryConfig = { headers: new AxiosHeaders() }
  const local = new AxiosError('Synthetic local failure', 'ERR_BAD_RESPONSE', localConfig, undefined, {
    config: localConfig, status: 500, statusText: 'Failed', headers: {}, data: {},
  })
  const ordinary = new AxiosError('Synthetic ordinary failure', 'ERR_BAD_RESPONSE', ordinaryConfig, undefined, {
    config: ordinaryConfig, status: 500, statusText: 'Failed', headers: {}, data: {},
  })
  const error = vi.spyOn(toast, 'error')
  expect(skipGlobalErrorHandling(local)).toBe(true)
  handleServerError(local)
  expect(error).not.toHaveBeenCalled()
  expect(skipGlobalErrorHandling(ordinary)).toBe(false)
  expect(skipGlobalErrorHandling(new Error('canceled'))).toBe(false)
})
