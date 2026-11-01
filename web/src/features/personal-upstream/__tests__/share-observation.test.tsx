/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AnchorHTMLAttributes } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import {
  getSubscriptionShare,
  type SubscriptionSharePool,
} from '@/features/keys/subscription-share-api'
import { getPlaygroundKeys } from '@/features/playground/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { WindowShareObservation } from '../components/window-share-observation'

vi.mock('@/features/playground/api', () => ({ getPlaygroundKeys: vi.fn() }))
vi.mock('@/features/keys/subscription-share-api', () => ({
  getSubscriptionShare: vi.fn(),
}))
vi.mock('../components/pool-usage-observation', () => ({
  PoolUsageObservation: () => <div data-testid='root-pool-usage' />,
}))
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
})

const pool: SubscriptionSharePool = {
  pool_id: 1,
  snapshot_id: 9,
  revision: 1,
  channel_id: 7,
  channel_name: 'Subscription account',
  plan_type: 'pro',
  window_slot: 'primary',
  window_type: 'five_hour',
  window_source: 'codex_wham_usage_primary',
  window_seconds: 18000,
  reset_at: 2000,
  observed_at: 1000,
  max_age_seconds: 300,
  sample_age_seconds: 60,
  state: 'baseline_pending',
  allocated_bps: 6000,
  share_bps: 2000,
  estimated_used_micro: 0,
  unattributed_micro: 0,
  pending_attempts: 0,
  unsampled_attempts: 0,
  uncertain_attempts: 0,
  approximate: true,
  baseline_time: 0,
}

async function observePool(
  evidence: SubscriptionSharePool,
  canReadPoolUsage = false
) {
  useAuthStore.getState().auth.setBundle({
    access_token: 'access-token',
    token_type: 'Bearer',
    access_expires_at: 4_000_000_000,
    user: { id: 7, username: 'root', role: ROLE.SUPER_ADMIN },
    session: {
      sid: 'session-a',
      current: true,
      login_method: 'password',
      ip: '127.0.0.1',
      user_agent: 'test',
      created_at: 1,
      last_active_at: 1,
      expires_at: 4_000_000_000,
    },
  })
  vi.mocked(getPlaygroundKeys).mockResolvedValue([
    {
      id: 31,
      name: 'Desktop app',
      status: 1,
      group: 'default',
      remain_quota: 100,
      unlimited_quota: false,
      expired_time: -1,
    },
  ])
  vi.mocked(getSubscriptionShare).mockResolvedValue({
    token_id: 31,
    pools: [evidence],
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <WindowShareObservation
        channelId={7}
        canReadPoolUsage={canReadPoolUsage}
      />
    </QueryClientProvider>
  )
  const user = userEvent.setup()
  await screen.findByRole('option', { name: 'Desktop app' })
  await user.selectOptions(
    screen.getByRole('combobox', { name: 'Select an API Key' }),
    '31'
  )
  if (canReadPoolUsage) await screen.findByTestId('root-pool-usage')
  else await screen.findByText('This Key allocation')
  return client
}

test('keeps usage and unattributed growth unknown until a trustworthy baseline exists', async () => {
  await observePool(pool)
  expect(screen.getAllByText('Unknown', { exact: true })).toHaveLength(2)
  expect(screen.queryByText('0 percentage points')).not.toBeInTheDocument()
  expect(screen.getByText('20%', { exact: true })).toBeInTheDocument()
  expect(
    screen.getByRole('combobox', { name: 'Select an API Key' })
  ).toHaveAccessibleDescription('Desktop app')
  expect(screen.getByTestId('upstream-share-selected-key')).toHaveTextContent(
    'Desktop app'
  )
})

test('shows native percentage points after a trustworthy baseline without converting them to Tokens', async () => {
  await observePool({
    ...pool,
    state: 'active',
    baseline_time: 900,
    estimated_used_micro: 5_000_000,
    unattributed_micro: 3_000_000,
  })
  expect(screen.getByText('5 percentage points')).toBeInTheDocument()
  expect(screen.getByText('3 percentage points')).toBeInTheDocument()
  expect(screen.queryByText('Unknown', { exact: true })).not.toBeInTheDocument()
})

test('Root retains the selected Key identity and uses only the new pool snapshot for numerical summaries', async () => {
  await observePool({ ...pool, share_bps: 2000 }, true)
  expect(screen.getByTestId('upstream-share-selected-key')).toHaveTextContent(
    'Desktop app'
  )
  expect(screen.getByTestId('root-pool-usage')).toBeInTheDocument()
  expect(screen.queryByText('This Key allocation')).not.toBeInTheDocument()
  expect(screen.queryByText('20%', { exact: true })).not.toBeInTheDocument()
  expect(screen.queryByText('Local Key estimate')).not.toBeInTheDocument()
})

test('the selected Key survives a stored-share refresh while failed reads remove its previous allocation', async () => {
  const client = await observePool({ ...pool, state: 'active', baseline_time: 900 })
  vi.mocked(getSubscriptionShare).mockRejectedValueOnce(
    new Error('Stored share read failed')
  )
  await act(async () => {
    await client.invalidateQueries({
      queryKey: ['personal-console', 'upstream-share'],
    })
  })
  expect(await screen.findByText('Stored share read failed')).toBeVisible()
  expect(
    screen.getByRole('combobox', { name: 'Select an API Key' })
  ).toHaveValue('31')
  expect(screen.queryByText('This Key allocation')).not.toBeInTheDocument()
  expect(screen.getByTestId('upstream-share-selected-key')).toHaveTextContent(
    'Desktop app'
  )
})

test('a controlled Key reports selection to its owner and stays selected after a parent refresh', async () => {
  await observePool(pool)
  const onKeyChange = vi.fn()
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  cleanup()
  const view = render(
    <QueryClientProvider client={client}>
      <WindowShareObservation
        channelId={7}
        selectedKeyId=''
        onKeyChange={onKeyChange}
      />
    </QueryClientProvider>
  )
  await screen.findByRole('option', { name: 'Desktop app' })
  await userEvent.setup().selectOptions(
    screen.getByRole('combobox', { name: 'Select an API Key' }),
    '31'
  )
  expect(onKeyChange).toHaveBeenCalledWith('31')
  view.rerender(
    <QueryClientProvider client={client}>
      <WindowShareObservation
        channelId={7}
        selectedKeyId='31'
        onKeyChange={onKeyChange}
      />
    </QueryClientProvider>
  )
  expect(
    screen.getByRole('combobox', { name: 'Select an API Key' })
  ).toHaveValue('31')
  await screen.findByText('This Key allocation')
})


test('a same-session Root downgrade cannot reuse privileged cached Key shares when the selected Key is controlled', async () => {
  useAuthStore.getState().auth.setBundle({
    access_token: 'synthetic-role-cache', token_type: 'Bearer', access_expires_at: 4_000_000_000,
    user: { id: 7, username: 'root', role: ROLE.SUPER_ADMIN },
    session: { sid: 'same-role-change-session', current: true, login_method: 'password', ip: '127.0.0.1', user_agent: 'test', created_at: 1, last_active_at: 1, expires_at: 4_000_000_000 },
  })
  const key = { id: 31, name: 'Desktop app', status: 1, group: 'default', remain_quota: 100, unlimited_quota: false, expired_time: -1 }
  let resolveKeys: (keys: typeof key[]) => void = () => {}
  const lowerRoleKeys = new Promise<typeof key[]>((resolve) => { resolveKeys = resolve })
  vi.mocked(getPlaygroundKeys).mockReset().mockResolvedValueOnce([key]).mockReturnValue(lowerRoleKeys)
  vi.mocked(getSubscriptionShare).mockReset()
    .mockResolvedValueOnce({ token_id: 31, pools: [{ ...pool, state: 'active', baseline_time: 900 }] })
    .mockResolvedValue({ token_id: 31, pools: [{ ...pool, channel_id: undefined, state: 'active', baseline_time: 900 }] })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  const view = render(<QueryClientProvider client={client}><WindowShareObservation channelId={7} selectedKeyId='31' canReadPoolUsage /></QueryClientProvider>)
  try {
    await screen.findByTestId('root-pool-usage')
    act(() => {
      useAuthStore.getState().auth.setUser({ id: 7, username: 'admin', role: ROLE.ADMIN })
      view.rerender(<QueryClientProvider client={client}><WindowShareObservation channelId={7} selectedKeyId='31' canReadPoolUsage={false} /></QueryClientProvider>)
    })
    expect(screen.queryByText('Total allocated')).not.toBeInTheDocument()
    expect(screen.queryByTestId('upstream-share-selected-key')).not.toBeInTheDocument()
    await waitFor(() => expect(getPlaygroundKeys).toHaveBeenCalledTimes(2))
    await act(async () => resolveKeys([key]))
    await waitFor(() => expect(getSubscriptionShare).toHaveBeenCalledTimes(2))
    expect(await screen.findByText('Root access is required to match Key shares to this account.')).toBeVisible()
    expect(screen.queryByText('Total allocated')).not.toBeInTheDocument()
  } finally {
    view.unmount()
    client.clear()
  }
})
