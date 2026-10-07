import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { getStatus } from '@/lib/api'

import { useFundingPresentation } from '../use-funding-presentation'
import { useSidebarData } from '../use-sidebar-data'

vi.mock('@/lib/api', () => ({ getStatus: vi.fn() }))
const originalStorage = Object.getOwnPropertyDescriptor(window, 'localStorage')
const status = (mode: 'enabled' | 'disabled' | 'retirement') => ({
  user_funding_mode: mode,
  user_funding_capabilities: {
    mode,
    epoch: 1,
    ready: true,
    can_top_up: true,
    can_redeem: true,
    can_transfer_affiliate_rewards: true,
    can_purchase_subscription: true,
    can_view_funding_history: true,
  },
})
beforeEach(() => {
  vi.resetAllMocks()
  const values = new Map<string, string>()
  Object.defineProperty(window, 'localStorage', {
    configurable: true,
    value: {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => {
        values.set(key, value)
      },
      removeItem: (key: string) => {
        values.delete(key)
      },
    },
  })
})
afterEach(() => {
  if (originalStorage) {
    Object.defineProperty(window, 'localStorage', originalStorage)
  }
})
function wrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return (props: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
  )
}

test('stored enabled placeholder cannot reopen commerce and disabled server reply remains closed', async () => {
  window.localStorage.setItem('status', JSON.stringify(status('enabled')))
  let resolve = (_value: ReturnType<typeof status>) => {}
  vi.mocked(getStatus).mockImplementation(
    () =>
      new Promise((r) => {
        resolve = r
      })
  )
  const { result } = renderHook(() => useFundingPresentation(), {
    wrapper: wrapper(),
  })
  expect(result.current.commercialEnabled).toBe(false)
  expect(result.current.ready).toBe(false)
  expect(result.current.capabilities.mode).toBe('enabled')
  expect(result.current.capabilities.ready).toBe(true)
  await act(async () => {
    resolve(status('disabled'))
  })
  await waitFor(() => expect(result.current.capabilities.mode).toBe('disabled'))
  expect(result.current.commercialEnabled).toBe(false)
  expect(result.current.ready).toBe(true)
})

test('confirmed enabled state opens presentation but a failed refresh closes it', async () => {
  vi.mocked(getStatus)
    .mockResolvedValueOnce(status('enabled'))
    .mockRejectedValueOnce(new Error('synthetic status failure'))
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const { result } = renderHook(() => useFundingPresentation(), {
    wrapper: (props) => (
      <QueryClientProvider client={client}>
        {props.children}
      </QueryClientProvider>
    ),
  })
  await waitFor(() => expect(result.current.commercialEnabled).toBe(true))
  await act(async () => {
    await client.invalidateQueries({ queryKey: ['status'] })
  })
  await waitFor(() => expect(result.current.commercialEnabled).toBe(false))
})

test('closed navigation keeps operational tasks and hides optional commercial history', async () => {
  vi.mocked(getStatus).mockResolvedValue(status('disabled'))
  const { result } = renderHook(() => useSidebarData(), { wrapper: wrapper() })
  await waitFor(() => expect(getStatus).toHaveBeenCalled())
  const groups = result.current.navGroups
  const items = groups.flatMap((group) => group.items)
  expect(items.some((item) => item.title === 'Wallet')).toBe(false)
  expect(items.some((item) => item.url === '/keys')).toBe(true)
  expect(items.some((item) => item.url === '/users')).toBe(true)
  expect(items.some((item) => item.url === '/usage-logs/common')).toBe(true)
  for (const url of ['/wallet', '/subscriptions', '/redemption-codes']) {
    expect(items.some((item) => item.url === url)).toBe(false)
  }
  expect(items.some((item) => item.title === 'History and recovery')).toBe(
    false
  )
})

test('explicitly enabled commerce retains its existing navigation', async () => {
  vi.mocked(getStatus).mockResolvedValue(status('enabled'))
  const { result } = renderHook(() => useSidebarData(), { wrapper: wrapper() })
  await waitFor(() =>
    expect(
      result.current.navGroups
        .flatMap((group) => group.items)
        .some((item) => item.title === 'Wallet')
    ).toBe(true)
  )
  const items = result.current.navGroups.flatMap((group) => group.items)
  expect(items.some((item) => item.url === '/subscriptions')).toBe(true)
})
