import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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
const { getTokenBudget, writeTokenBudget } =
  await import('../../token-budget-api')
const { TokenBudgetDialog } = await import('../token-budget-dialog')
const { toast } = await import('sonner')
axios.defaults.adapter = defaultAdapter

const originalAuth = useAuthStore.getState()
const confirmedPolicy =
  'I confirm all running instances support this budget and I understand its request restrictions.'
const baseView = {
  policy: {
    token_id: 3,
    user_id: 1,
    enabled: false,
    account_threshold_enabled: false,
    account_min_remaining_bps: 2000,
    account_max_age_seconds: 300,
    fee_enabled: false,
    fee_limit_usd: '0',
    fee_used_usd: '0',
    fee_reserved_usd: '0',
    limit: 100,
    used: 0,
    reserved: 0,
    pending_request_id: '',
    revision: 0,
  },
  pending: null,
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

function renderBudget(client: QueryClient, onClose = vi.fn()) {
  return {
    onClose,
    ...render(
      <QueryClientProvider client={client}>
        <TokenBudgetDialog tokenId={3} onClose={onClose} />
      </QueryClientProvider>
    ),
  }
}

beforeEach(() => {
  useAuthStore.getState().auth.setBundle(bundle('session-a'))
  vi.mocked(toast.error).mockClear()
})

afterEach(() => {
  cleanup()
  useAuthStore.setState(originalAuth)
  vi.restoreAllMocks()
})

test.each(['policy', 'recover'] as const)(
  'a budget %s write returning 401 never refreshes or replays',
  async (kind) => {
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
    const request =
      kind === 'policy'
        ? writeTokenBudget(3, {
            kind: 'policy',
            body: {
              id: 'a'.repeat(64),
              expected_revision: 0,
              enabled: false,
              limit: 100,
              confirmed: true,
              fee: { enabled: false, limit_usd: '0' },
              account_threshold: {
                enabled: false,
                minimum_remaining_bps: 2000,
                max_age_seconds: 300,
              },
            },
          })
        : writeTokenBudget(3, {
            kind: 'recover',
            body: {
              request_id: 'synthetic-request',
              action: 'cancel_not_sent',
              evidence_reference: 'synthetic evidence',
              confirmed_reliable_evidence: true,
            },
          })
    await vi.waitFor(() => expect(requests).toHaveLength(1))
    useAuthStore.getState().auth.setBundle(bundle('replacement'))
    pending.reject(unauthorized(requests[0]))
    await expect(request).rejects.toThrow('Synthetic expired authentication')
    expect(requests).toHaveLength(1)
    expect(requests[0].skipBusinessError).toBe(true)
    expect(requests[0].skipAuthRefresh).toBe(true)
    expect(refreshes).toHaveLength(0)
    expect(useAuthStore.getState().auth.session?.sid).toBe('replacement')
  }
)

test('budget reads opt out of transport deduplication and authentication replay', async () => {
  let requestConfig: InternalAxiosRequestConfig | undefined
  transport = (config) => {
    requestConfig = config
    return Promise.resolve(response(config, { success: true, data: baseView }))
  }
  await expect(getTokenBudget(3)).resolves.toEqual(baseView)
  expect(requestConfig?.disableDuplicate).toBe(true)
  expect(requestConfig?.skipBusinessError).toBe(true)
  expect(requestConfig?.skipAuthRefresh).toBe(true)
})

test('a budget business failure is handled locally without a transport toast', async () => {
  transport = (config) =>
    Promise.resolve(
      response(config, {
        success: false,
        message: 'Synthetic private budget failure',
      })
    )
  await expect(getTokenBudget(3)).rejects.toThrow()
  expect(toast.error).not.toHaveBeenCalled()
})

test.each(['success', 'unauthorized', 'business failure'] as const)(
  'a delayed old-SID budget read ending in %s never enters either session cache',
  async (outcome) => {
    const oldRead = deferred<AxiosResponse>()
    const requests: InternalAxiosRequestConfig[] = []
    transport = (config) => {
      requests.push(config)
      if (requests.length === 1) return oldRead.promise
      return Promise.resolve(
        response(config, {
          success: true,
          data: {
            ...baseView,
            policy: { ...baseView.policy, limit: 200 },
          },
        })
      )
    }
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    renderBudget(client)
    await waitFor(() => expect(requests).toHaveLength(1))
    const oldSignal = requests[0].signal
    act(() => useAuthStore.getState().auth.setBundle(bundle('session-b')))
    expect(await screen.findByDisplayValue('200')).toBeVisible()
    expect(oldSignal?.aborted).toBe(true)
    await act(async () => {
      if (outcome === 'success') {
        oldRead.resolve(
          response(requests[0], {
            success: true,
            data: {
              ...baseView,
              policy: { ...baseView.policy, limit: 999 },
            },
          })
        )
      } else if (outcome === 'unauthorized') {
        const error = unauthorized(requests[0])
        oldRead.reject(error)
      } else {
        oldRead.resolve(
          response(requests[0], {
            success: false,
            message: 'Synthetic stale business failure',
          })
        )
      }
      await oldRead.promise.catch(() => undefined)
    })
    expect(
      client.getQueryData(['token-budget', 1, 100, 'session-a', 3])
    ).toBeUndefined()
    expect(
      client.getQueryData<{
        policy: { limit: number }
      }>(['token-budget', 1, 100, 'session-b', 3])?.policy.limit
    ).toBe(200)
    expect(screen.queryByDisplayValue('999')).not.toBeInTheDocument()
    expect(toast.error).not.toHaveBeenCalled()
    client.clear()
  }
)

test('a pending old-SID write cannot update the replacement cache', async () => {
  const user = userEvent.setup()
  const oldWrite = deferred<AxiosResponse>()
  const requests: InternalAxiosRequestConfig[] = []
  transport = (config) => {
    requests.push(config)
    if (config.method === 'put') return oldWrite.promise
    const limit =
      config.headers.get('Authorization') ===
      'Bearer synthetic-access-session-b'
        ? 200
        : 100
    return Promise.resolve(
      response(config, {
        success: true,
        data: { ...baseView, policy: { ...baseView.policy, limit } },
      })
    )
  }
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const rendered = renderBudget(client)
  await user.click(
    await screen.findByRole('checkbox', { name: confirmedPolicy })
  )
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() =>
    expect(requests.filter((request) => request.method === 'put')).toHaveLength(
      1
    )
  )
  const writeConfig = requests.find((request) => request.method === 'put')
  expect(writeConfig).toBeDefined()
  if (!writeConfig) throw new Error('Expected a budget write')
  act(() => useAuthStore.getState().auth.setBundle(bundle('session-b')))
  expect(await screen.findByDisplayValue('200')).toBeVisible()
  expect(writeConfig.signal?.aborted).toBe(true)
  await act(async () => {
    oldWrite.resolve(
      response(writeConfig, {
        success: true,
        data: {
          ...baseView,
          policy: { ...baseView.policy, limit: 999, revision: 1 },
        },
      })
    )
    await oldWrite.promise
  })
  expect(
    client.getQueryData<{ policy: { limit: number } }>([
      'token-budget',
      1,
      100,
      'session-b',
      3,
    ])?.policy.limit
  ).toBe(200)
  expect(screen.queryByDisplayValue('999')).not.toBeInTheDocument()
  expect(rendered.onClose).not.toHaveBeenCalled()
  client.clear()
})
