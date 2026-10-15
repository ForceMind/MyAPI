/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AnchorHTMLAttributes } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import type { SubscriptionSharePool } from '@/features/keys/subscription-share-api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { PoolUsageObservation } from '../components/pool-usage-observation'
import { getPoolUsage, type PoolUsage } from '../pool-usage-api'
import { poolUsageFixture } from './pool-usage-fixture'

vi.mock('../pool-usage-api', () => ({ getPoolUsage: vi.fn() }))
vi.mock('@tanstack/react-router', () => ({
  Link: (props: AnchorHTMLAttributes<HTMLAnchorElement> & { to?: string }) => {
    const { to, children, ...attributes } = props
    return (
      <a href={to} {...attributes}>
        {children}
      </a>
    )
  },
}))

afterEach(() => {
  useAuthStore.getState().auth.reset('idle')
  vi.resetAllMocks()
})

function authenticate(role: number = ROLE.SUPER_ADMIN, sid = 'session-a') {
  useAuthStore.getState().auth.setBundle({
    access_token: 'synthetic-access',
    token_type: 'Bearer',
    access_expires_at: 4_000_000_000,
    user: { id: 7, username: 'owner', role },
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
  })
}

function observe(
  usage = poolUsageFixture(),
  overrides: {
    poolId?: number
    channelId?: number
    closed?: boolean
    canRead?: boolean
  } = {}
) {
  const pool: SubscriptionSharePool = {
    ...usage,
    pool_id: overrides.poolId ?? usage.pool_id,
    state: overrides.closed ? 'closed' : usage.state,
    share_bps: 1000,
    estimated_used_micro: 5000000,
    unattributed_micro: 40000000,
  }
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <PoolUsageObservation
        pool={pool}
        channelId={overrides.channelId ?? 1}
        canRead={overrides.canRead ?? true}
      />
    </QueryClientProvider>
  )
}

test('shows all Key allocations and the initial cycle baseline separately from a later recovery reference', async () => {
  authenticate()
  vi.mocked(getPoolUsage).mockResolvedValue(poolUsageFixture())
  observe()
  await screen.findByRole('heading', { name: 'All Key allocations' })
  expect(screen.getByTestId('pool-estimated-total')).toHaveTextContent(
    '15 percentage points'
  )
  expect(screen.getByTestId('pool-initial-baseline')).toHaveTextContent(
    '35 percentage points'
  )
  expect(screen.getByTestId('pool-unattributed-growth')).toHaveTextContent(
    '5 percentage points'
  )
  expect(screen.getByTestId('pool-recovery-reference')).toHaveTextContent(
    '43 percentage points'
  )
  expect(
    [...screen.getByTestId('pool-decomposition-bar').children].map(
      (element) => (element as HTMLElement).style.width
    )
  ).toEqual(['15%', '35%', '5%'])
  for (const name of ['Desktop app', 'Shared device', 'Mobile app'])
    expect(screen.getByText(name)).toBeVisible()
  expect(
    within(screen.getByTestId('pool-key-21')).getByTestId(
      'allocation-used-within-share'
    )
  ).toHaveTextContent('50%')
})

test('retains zero-share and deleted Key usage without division by zero', async () => {
  authenticate()
  const usage = poolUsageFixture({ allocated_bps: 5000 })
  usage.allocations[2] = {
    ...usage.allocations[2],
    token_name: null,
    token_state: 'deleted',
    share_bps: 0,
  }
  vi.mocked(getPoolUsage).mockResolvedValue(usage)
  observe(usage)
  const row = await screen.findByTestId('pool-key-23')
  expect(within(row).getByText('Key #23')).toBeVisible()
  expect(
    within(row).getByTestId('allocation-estimated-usage')
  ).toHaveTextContent('3 percentage points')
  expect(
    within(row).getByTestId('allocation-used-within-share')
  ).toHaveTextContent('Unknown')
  expect(row).not.toHaveTextContent('Infinity')
  expect(
    within(row).getByText('Share stopped; prior usage is retained.')
  ).toBeVisible()
})

test('unknown evidence keeps counts and prior values without drawing a false decomposition', async () => {
  authenticate()
  const usage = poolUsageFixture({
    decomposition_state: 'inconsistent',
    state: 'discontinuity',
    estimated_used_micro: null,
    pending_attempts: 3,
  })
  usage.allocations[0].estimated_used_micro = null
  vi.mocked(getPoolUsage).mockResolvedValue(usage)
  observe(usage)
  await screen.findByTestId('pool-decomposition-unavailable')
  expect(screen.getByTestId('pool-estimated-total')).toHaveTextContent(
    'Unknown'
  )
  expect(screen.queryByTestId('pool-decomposition-bar')).not.toBeInTheDocument()
  expect(screen.getByTestId('pool-preserved-estimates')).toBeVisible()
  expect(screen.getByText(/Last processed observation/)).toBeVisible()
  expect(
    within(screen.getByTestId('pool-key-22')).getByTestId(
      'allocation-estimated-usage'
    )
  ).toHaveTextContent('7 percentage points')
})

test('a failed read offers bounded explicit recovery without stale successful charts', async () => {
  authenticate()
  vi.mocked(getPoolUsage)
    .mockRejectedValueOnce(new Error('Synthetic outage'))
    .mockResolvedValueOnce(poolUsageFixture())
  observe()
  const error = await screen.findByTestId('pool-usage-error')
  expect(screen.queryByTestId('pool-decomposition-bar')).not.toBeInTheDocument()
  await userEvent
    .setup()
    .click(within(error).getByRole('button', { name: 'Retry' }))
  await screen.findByRole('heading', { name: 'All Key allocations' })
  expect(getPoolUsage).toHaveBeenCalledTimes(2)
})

for (const forbidden of [
  'admin',
  'user',
  'candidate',
  'other-channel',
  'closed',
  'fresh-permission-denied',
] as const) {
  test(`does not request the root projection for ${forbidden}`, () => {
    let role: number = ROLE.SUPER_ADMIN
    if (forbidden === 'admin') role = ROLE.ADMIN
    if (forbidden === 'user') role = ROLE.USER
    authenticate(role)
    observe(poolUsageFixture(), {
      poolId: forbidden === 'candidate' ? 0 : 7,
      channelId: forbidden === 'other-channel' ? 2 : 1,
      closed: forbidden === 'closed',
      canRead: forbidden !== 'fresh-permission-denied',
    })
    expect(getPoolUsage).not.toHaveBeenCalled()
    expect(screen.queryByTestId('pool-usage-7')).not.toBeInTheDocument()
  })
}

test('a previous session response cannot populate the new session window', async () => {
  authenticate()
  let finishPrevious!: (value: PoolUsage) => void
  const previous = new Promise<PoolUsage>((resolve) => {
    finishPrevious = resolve
  })
  const current = poolUsageFixture()
  current.allocations[0].token_name = 'Current session Key'
  vi.mocked(getPoolUsage)
    .mockReturnValueOnce(previous)
    .mockResolvedValueOnce(current)
  observe()
  await waitFor(() => expect(getPoolUsage).toHaveBeenCalledTimes(1))
  act(() => authenticate(ROLE.SUPER_ADMIN, 'session-b'))
  await screen.findByText('Current session Key')
  const obsolete = poolUsageFixture()
  obsolete.allocations[0].token_name = 'Obsolete session Key'
  await act(async () => {
    finishPrevious(obsolete)
    await previous
  })
  expect(screen.queryByText('Obsolete session Key')).not.toBeInTheDocument()
  expect(screen.getByText('Current session Key')).toBeVisible()
  expect(getPoolUsage).toHaveBeenCalledTimes(2)
})

test('a replaced pool displays its returned window identity instead of the older selected-Key snapshot', async () => {
  authenticate()
  vi.mocked(getPoolUsage).mockResolvedValue(
    poolUsageFixture({
      window_type: 'weekly',
      window_seconds: 604800,
      window_slot: 'secondary',
      generation: 2,
    })
  )
  observe()
  await screen.findByRole('heading', { name: 'All Key allocations' })
  expect(screen.getByTestId('pool-window-descriptor')).toHaveTextContent(
    '7 days'
  )
  expect(screen.getByTestId('pool-window-descriptor')).toHaveTextContent(
    'Secondary window'
  )
  expect(screen.getByTestId('pool-window-descriptor')).not.toHaveTextContent(
    '5 hours'
  )
})
