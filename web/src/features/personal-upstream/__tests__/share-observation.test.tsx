/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AnchorHTMLAttributes } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { getSubscriptionShare, type SubscriptionSharePool } from '@/features/keys/subscription-share-api'
import { getPlaygroundKeys } from '@/features/playground/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { WindowShareObservation } from '../components/window-share-observation'

vi.mock('@/features/playground/api', () => ({ getPlaygroundKeys: vi.fn() }))
vi.mock('@/features/keys/subscription-share-api', () => ({ getSubscriptionShare: vi.fn() }))
vi.mock('@tanstack/react-router', () => ({ Link: (props: AnchorHTMLAttributes<HTMLAnchorElement> & { to?: string }) => {
  const { to, children, ...attributes } = props
  return <a href={to} {...attributes}>{children}</a>
} }))
afterEach(() => { useAuthStore.getState().auth.reset('idle') })

const pool: SubscriptionSharePool = {
  pool_id: 1, snapshot_id: 9, revision: 1, channel_id: 7, channel_name: 'Subscription account', plan_type: 'pro', window_slot: 'primary', window_type: 'five_hour', window_seconds: 18000, reset_at: 2000, observed_at: 1000, max_age_seconds: 300, sample_age_seconds: 60, state: 'baseline_pending', allocated_bps: 6000, share_bps: 2000, estimated_used_micro: 0, unattributed_micro: 0, pending_attempts: 0, unsampled_attempts: 0, approximate: true, baseline_time: 0,
}

async function observePool(evidence: SubscriptionSharePool) {
  useAuthStore.getState().auth.setBundle({ access_token: 'access-token', token_type: 'Bearer', access_expires_at: 4_000_000_000, user: { id: 7, username: 'root', role: ROLE.SUPER_ADMIN }, session: { sid: 'session-a', current: true, login_method: 'password', ip: '127.0.0.1', user_agent: 'test', created_at: 1, last_active_at: 1, expires_at: 4_000_000_000 } })
  vi.mocked(getPlaygroundKeys).mockResolvedValue([{ id: 31, name: 'Desktop app', status: 1, group: 'default', remain_quota: 100, unlimited_quota: false, expired_time: -1 }])
  vi.mocked(getSubscriptionShare).mockResolvedValue({ token_id: 31, pools: [evidence] })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={client}><WindowShareObservation channelId={7} /></QueryClientProvider>)
  const user = userEvent.setup()
  await screen.findByRole('option', { name: 'Desktop app' })
  await user.selectOptions(screen.getByRole('combobox', { name: 'Select an API Key' }), '31')
  await screen.findByText('This Key allocation')
}

test('keeps usage and unattributed growth unknown until a trustworthy baseline exists', async () => {
  await observePool(pool)
  expect(screen.getAllByText('Unknown', { exact: true })).toHaveLength(2)
  expect(screen.queryByText('0 percentage points')).not.toBeInTheDocument()
  expect(screen.getByText('20%', { exact: true })).toBeInTheDocument()
})

test('shows native percentage points after a trustworthy baseline without converting them to Tokens', async () => {
  await observePool({ ...pool, state: 'active', baseline_time: 900, estimated_used_micro: 5_000_000, unattributed_micro: 3_000_000 })
  expect(screen.getByText('5 percentage points')).toBeInTheDocument()
  expect(screen.getByText('3 percentage points')).toBeInTheDocument()
  expect(screen.queryByText('Unknown', { exact: true })).not.toBeInTheDocument()
})
