import axios, {
  AxiosError,
  type AxiosAdapter,
  type AxiosResponse,
  type InternalAxiosRequestConfig,
} from 'axios'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import type { AuthBundle } from '@/stores/auth-store'

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}))

let transport: AxiosAdapter
const adapter: AxiosAdapter = (config) => transport(config)
const defaultAdapter = axios.defaults.adapter
axios.defaults.adapter = adapter
const { useAuthStore } = await import('@/stores/auth-store')
const {
  getAssignedPolicy,
  getOwnerAccess,
  previewAssignedPolicy,
  removeAssignedPolicy,
  saveAssignedPolicy,
} = await import('../api')
axios.defaults.adapter = defaultAdapter

const originalAuth = useAuthStore.getState()
const candidate = {
  enabled: true,
  public_models: null,
  upstream_models: [],
  channel_ids: [1],
}
const tokenTarget = {
  subject: 'token' as const,
  id: 7,
  ownerUserId: 1,
}
const userTarget = {
  subject: 'user' as const,
  id: 2,
  ownerUserId: 2,
}

function bundle(sid: string): AuthBundle {
  return {
    access_token: `synthetic-access-${sid}`,
    token_type: 'Bearer',
    access_expires_at: 4_000_000_000,
    user: { id: 1, role: 100, username: 'synthetic-root' },
    session: {
      sid,
      current: true,
      login_method: 'password',
      ip: '127.0.0.1',
      user_agent: 'test',
      created_at: 1,
      last_active_at: 1,
      expires_at: 4_000_000_000,
    },
  }
}

function response(
  config: InternalAxiosRequestConfig,
  data: unknown
): AxiosResponse {
  return { config, data, status: 200, statusText: 'OK', headers: {} }
}

function unauthorized(config: InternalAxiosRequestConfig): AxiosError {
  return new AxiosError(
    'Synthetic expired authentication',
    AxiosError.ERR_BAD_REQUEST,
    config,
    undefined,
    {
      config,
      data: { success: false },
      status: 401,
      statusText: 'Unauthorized',
      headers: {},
    }
  )
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

beforeEach(() => {
  useAuthStore.getState().auth.setBundle(bundle('session-a'))
})

afterEach(() => {
  useAuthStore.setState(originalAuth)
  vi.restoreAllMocks()
})

test.each(['read', 'save', 'remove', 'preview', 'owner'] as const)(
  'a token policy %s returning 401 never refreshes or replays',
  async (action) => {
    const requests: InternalAxiosRequestConfig[] = []
    const refreshes: InternalAxiosRequestConfig[] = []
    const pending = deferred<AxiosResponse>()
    transport = (config) => {
      if (config.url === '/api/user/auth/refresh') {
        refreshes.push(config)
        return Promise.resolve(
          response(config, { success: true, data: bundle('refreshed') })
        )
      }
      requests.push(config)
      return pending.promise
    }
    let request: Promise<unknown>
    if (action === 'read') request = getAssignedPolicy(tokenTarget)
    else if (action === 'save') {
      request = saveAssignedPolicy(tokenTarget, {
        ...candidate,
        expected_revision: 0,
      })
    } else if (action === 'remove') {
      request = removeAssignedPolicy(tokenTarget, 0)
    } else if (action === 'preview') {
      request = previewAssignedPolicy(tokenTarget, candidate)
    } else request = getOwnerAccess(7)
    await vi.waitFor(() => expect(requests).toHaveLength(1))
    useAuthStore.getState().auth.setBundle(bundle('replacement'))
    pending.reject(unauthorized(requests[0]))
    await expect(request).rejects.toThrow('Synthetic expired authentication')
    expect(requests).toHaveLength(1)
    expect(refreshes).toHaveLength(0)
    expect(useAuthStore.getState().auth.session?.sid).toBe('replacement')
  }
)

test('a user policy request retains normal authentication refresh and replay', async () => {
  const requests: InternalAxiosRequestConfig[] = []
  const refreshes: InternalAxiosRequestConfig[] = []
  transport = (config) => {
    if (config.url === '/api/user/auth/refresh') {
      refreshes.push(config)
      return Promise.resolve(
        response(config, { success: true, data: bundle('refreshed') })
      )
    }
    requests.push(config)
    if (requests.length === 1) return Promise.reject(unauthorized(config))
    return Promise.resolve(
      response(config, {
        success: true,
        data: {
          subject: 'user',
          subject_id: 2,
          owner_user_id: 2,
          revision: 1,
          assigned: true,
          ...candidate,
        },
      })
    )
  }
  await expect(
    saveAssignedPolicy(userTarget, { ...candidate, expected_revision: 0 })
  ).resolves.toMatchObject({ subject: 'user', subject_id: 2, revision: 1 })
  expect(refreshes).toHaveLength(1)
  expect(requests).toHaveLength(2)
  expect(requests[1].headers.get('Authorization')).toBe(
    'Bearer synthetic-access-refreshed'
  )
})
