import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { getChannelQuotaChanges } from '@/features/channels/api'
import { getRecentLogOverview } from '@/features/dashboard/api'
import { getTokenBudget } from '@/features/keys/token-budget-api'
import { getPlaygroundKeys } from '@/features/playground/api'
import { getSelf } from '@/lib/api'
import { ROLE } from '@/lib/roles'

import { usePersonalWorkbenchData } from '../use-personal-workbench-data'

vi.mock('@/features/channels/api', () => ({
  getChannelQuotaChanges: vi.fn(),
}))
vi.mock('@/features/dashboard/api', () => ({
  getRecentLogOverview: vi.fn(),
}))
vi.mock('@/features/keys/token-budget-api', () => ({
  getTokenBudget: vi.fn(),
}))
vi.mock('@/features/playground/api', () => ({
  getPlaygroundKeys: vi.fn(),
}))
vi.mock('@/lib/api', () => ({ getSelf: vi.fn() }))
vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({
    status: {
      user_funding_mode: 'disabled',
      user_funding_capabilities: {
        mode: 'disabled',
        epoch: 1,
        ready: true,
      },
    },
    loading: false,
    error: null,
    confirmed: true,
  }),
}))

const currentUser = {
  id: 7,
  username: 'owner',
  role: ROLE.SUPER_ADMIN,
}
const availableKey = {
  id: 31,
  name: 'chosen-key',
  status: 1,
  group: 'default',
  remain_quota: 100,
  unlimited_quota: false,
  expired_time: -1,
}

function wrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return function QueryWrapper(props: { children: ReactNode }) {
    return (
      <QueryClientProvider client={client}>
        {props.children}
      </QueryClientProvider>
    )
  }
}

function freshSelf(role: number, canReadChannels: boolean) {
  return {
    success: true,
    data: {
      id: 7,
      username: 'owner',
      role,
      quota: 100,
      self_use_no_balance: true,
      usage_policy_revision: 1,
      permissions: {
        admin_permissions: {
          channel: { read: canReadChannels },
        },
      },
    },
  }
}

beforeEach(() => {
  vi.mocked(getSelf).mockResolvedValue(freshSelf(ROLE.SUPER_ADMIN, true))
  vi.mocked(getPlaygroundKeys).mockResolvedValue([availableKey])
  vi.mocked(getRecentLogOverview).mockResolvedValue({
    success: true,
    data: { requests: [], errors: [] },
  })
  vi.mocked(getChannelQuotaChanges).mockResolvedValue({
    success: true,
    data: { items: [] },
  })
})

describe('personal workbench data boundaries', () => {
  test('starts with no selected Key and always reads the self log overview', async () => {
    const { result } = renderHook(
      () =>
        usePersonalWorkbenchData({
          user: currentUser,
          sessionId: 'session-a',
          selectedKeyId: null,
        }),
      { wrapper: wrapper() }
    )

    await waitFor(() => expect(result.current.recentQuery.isSuccess).toBe(true))
    expect(getRecentLogOverview).toHaveBeenCalledWith(false)
    expect(getTokenBudget).not.toHaveBeenCalled()
  })

  test('does not request account windows for a fresh owner without channel permission', async () => {
    vi.mocked(getSelf).mockResolvedValue(freshSelf(ROLE.USER, false))
    const { result } = renderHook(
      () =>
        usePersonalWorkbenchData({
          user: currentUser,
          sessionId: 'session-a',
          selectedKeyId: null,
        }),
      { wrapper: wrapper() }
    )

    await waitFor(() => expect(result.current.identityConfirmed).toBe(true))
    expect(result.current.canReadChannels).toBe(false)
    expect(getChannelQuotaChanges).not.toHaveBeenCalled()
  })

  test('revokes residual channel permission after a fresh self retry fails', async () => {
    const { result } = renderHook(
      () =>
        usePersonalWorkbenchData({
          user: currentUser,
          sessionId: 'session-a',
          selectedKeyId: null,
        }),
      { wrapper: wrapper() }
    )
    await waitFor(() => expect(getChannelQuotaChanges).toHaveBeenCalledOnce())

    vi.mocked(getSelf).mockRejectedValueOnce(new Error('offline'))
    await act(async () => {
      await result.current.selfQuery.refetch()
    })

    expect(result.current.identityError).toBe(true)
    expect(result.current.identityConfirmed).toBe(false)
    expect(result.current.freshUser).toBeUndefined()
    expect(result.current.canReadChannels).toBe(false)
    expect(getChannelQuotaChanges).toHaveBeenCalledOnce()
  })

  test('stops treating a selected Key as confirmed after its safe list refresh fails', async () => {
    vi.mocked(getTokenBudget).mockResolvedValue({
      policy: {
        token_id: 31,
        user_id: 7,
        enabled: false,
        account_threshold_enabled: false,
        account_min_remaining_bps: 0,
        account_max_age_seconds: 300,
        fee_enabled: false,
        fee_limit_usd: '0.000000',
        fee_used_usd: '0.000000',
        fee_reserved_usd: '0.000000',
        limit: 0,
        used: 0,
        reserved: 0,
        pending_request_id: '',
        revision: 1,
      },
      pending: null,
    })
    const { result } = renderHook(
      () =>
        usePersonalWorkbenchData({
          user: currentUser,
          sessionId: 'session-a',
          selectedKeyId: 31,
        }),
      { wrapper: wrapper() }
    )
    await waitFor(() => expect(getTokenBudget).toHaveBeenCalledOnce())

    vi.mocked(getPlaygroundKeys).mockRejectedValueOnce(new Error('offline'))
    await act(async () => {
      await result.current.keysQuery.refetch()
    })

    expect(result.current.keysError).toBe(true)
    expect(result.current.confirmedKeys).toBeUndefined()
    expect(result.current.selectedKey).toBeUndefined()
  })
})
