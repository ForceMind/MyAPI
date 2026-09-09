/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { getChannelRouting } from '@/features/channel-routing/api'
import {
  getRecordedRequestSummary,
  getUserQuotaDates,
} from '@/features/dashboard/api'
import { useAuthStore } from '@/stores/auth-store'

import { SummaryCards } from '../summary-cards'

vi.mock('@/features/channel-routing/api', () => ({
  getChannelRouting: vi.fn(),
}))
vi.mock('@/features/dashboard/api', () => ({
  getRecordedRequestSummary: vi.fn(),
  getUserQuotaDates: vi.fn(),
}))
vi.mock('@/lib/api', () => ({ getStatus: vi.fn().mockResolvedValue({}) }))

function mountSummary() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <SummaryCards />
    </QueryClientProvider>
  )
  return client
}

beforeEach(() => {
  vi.clearAllMocks()
  useAuthStore.getState().auth.setUser({ id: 1, username: 'admin', role: 100 })
  vi.mocked(getRecordedRequestSummary).mockResolvedValue({
    start_timestamp: 1,
    end_timestamp: 2,
    total_requests: 100,
    successful_requests: 92,
    failed_requests: 8,
    success_rate: 92,
  })
  vi.mocked(getUserQuotaDates).mockResolvedValue({ success: true, data: [] })
  vi.mocked(getChannelRouting).mockResolvedValue({
    success: true,
    data: {
      policy: {
        enabled: false,
        sticky_enabled: true,
        session_ttl_seconds: 86400,
        quota_max_age_seconds: 600,
      },
      channels: [
        {
          id: 1,
          name: 'enabled',
          type: 1,
          status: 1,
          priority: 0,
          weight: 1,
          quota: { state: 'fresh' },
        },
        {
          id: 2,
          name: 'disabled',
          type: 1,
          status: 2,
          priority: 0,
          weight: 1,
          quota: { state: 'unknown' },
        },
      ],
    },
  })
})

describe('administrator overview indicators', () => {
  test('shows recorded success and counts enabled channels without claiming health', async () => {
    mountSummary()
    expect(await screen.findByText('92%')).toBeInTheDocument()
    const title = screen.getByText('Enabled channels')
    expect(title.nextElementSibling).toHaveTextContent('1')
    expect(
      screen.getByText('Configured channels, not a health check')
    ).toBeInTheDocument()
    expect(getUserQuotaDates).not.toHaveBeenCalled()
  })
  test('failed refresh replaces the previously successful request rate', async () => {
    const client = mountSummary()
    await screen.findByText('92%')
    vi.mocked(getRecordedRequestSummary).mockRejectedValue(new Error('fixture'))
    await client.invalidateQueries({
      queryKey: ['dashboard', 'overview', 'recorded-request-summary'],
    })
    await waitFor(() =>
      expect(screen.queryByText('92%')).not.toBeInTheDocument()
    )
    const title = screen.getByText('Recorded request success rate')
    expect(title.nextElementSibling).toHaveTextContent('Unavailable')
  })
})
