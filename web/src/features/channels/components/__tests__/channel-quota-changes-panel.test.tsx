/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  getChannelQuotaChanges,
  getChannelQuotaSamplingStatus,
} from '../../api'
import { ChannelQuotaChangesPanel } from '../channel-quota-changes-panel'

vi.mock('../../api', () => ({
  getChannelQuotaChanges: vi.fn(),
  getChannelQuotaSamplingStatus: vi.fn(),
}))

// The panel is rendered in the channels route in production. Keep this unit
// test focused on its responsive content and query states without requiring a
// full router tree.
vi.mock('@tanstack/react-router', () => ({
  Link: (props: { children?: ReactNode; to?: string; [key: string]: unknown }) => (
    <a href={props.to} {...props}>{props.children}</a>
  ),
}))

function renderPanel() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ChannelQuotaChangesPanel />
    </QueryClientProvider>
  )
}

describe('channel quota changes panel', () => {
  afterEach(() => {
    useAuthStore.getState().auth.reset()
  })

  test('does not reuse quota rows after the authenticated user changes', async () => {
    useAuthStore.getState().auth.setUser({ id: 101, username: 'first-admin', role: ROLE.ADMIN })
    vi.mocked(getChannelQuotaChanges)
      .mockResolvedValueOnce({
        success: true,
        data: {
          items: [{ channel_id: 1, name: 'First account', status: 'success', direction: 'stable', current_available: 90 }],
        },
      })
      .mockResolvedValueOnce({
        success: true,
        data: {
          items: [{ channel_id: 2, name: 'Second account', status: 'success', direction: 'stable', current_available: 80 }],
        },
      })
    vi.mocked(getChannelQuotaSamplingStatus).mockResolvedValue({
      success: true,
      data: { enabled: false, interval_seconds: 300, max_channels: 20 },
    })

    renderPanel()
    expect(await screen.findByText('First account')).toBeInTheDocument()

    useAuthStore.getState().auth.setUser({ id: 202, username: 'second-admin', role: ROLE.ADMIN })

    expect(await screen.findByText('Second account')).toBeInTheDocument()
    expect(screen.queryByText('First account')).not.toBeInTheDocument()
    expect(getChannelQuotaChanges).toHaveBeenCalledTimes(2)
  })

  test('shows mobile-safe rows and the largest movement summary', async () => {
    vi.mocked(getChannelQuotaChanges).mockResolvedValueOnce({
      success: true,
      data: {
        items: [
          {
            channel_id: 12,
            name: 'Codex',
            account_label: 'Codex team account',
            current_available: 72,
            previous_available: 80,
            change_per_minute: -8,
            abs_change_per_minute: 8,
            direction: 'decrease',
            status: 'success',
            alert: {
              enabled: true,
              status: 'warning',
              ratio_percent: 15,
              warning_percent: 20,
              critical_percent: 10,
            },
            unit: 'percent',
            window_type: 'primary',
            observed_at: 1_700_000_000,
          },
        ],
      },
    })
    vi.mocked(getChannelQuotaSamplingStatus).mockResolvedValueOnce({
      success: true,
      data: { enabled: true, interval_seconds: 300, max_channels: 20 },
    })

    renderPanel()

    expect(await screen.findByText('Codex team account')).toBeInTheDocument()
    expect(screen.getByText('Largest change per minute')).toBeInTheDocument()
    expect(screen.getByText('Accounts tracked')).toBeInTheDocument()
    expect(screen.getByLabelText('Refresh')).toBeInTheDocument()
    expect(screen.getByLabelText('Quota window type')).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Unsupported' })).toBeInTheDocument()
    expect(screen.getByText('Quota alert: Warning')).toBeInTheDocument()
  })

  test('exposes unsupported provider quota samples as a distinct status', async () => {
    vi.mocked(getChannelQuotaChanges).mockResolvedValueOnce({
      success: true,
      data: {
        items: [{
          channel_id: 14,
          name: 'Claude',
          status: 'unsupported',
          direction: 'unknown',
          observed_at: 1_700_000_100,
        }],
      },
    })
    vi.mocked(getChannelQuotaSamplingStatus).mockResolvedValueOnce({
      success: true,
      data: { enabled: false, interval_seconds: 900, max_channels: 100 },
    })

    renderPanel()

    expect(await screen.findByText('unsupported')).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Unsupported' })).toBeInTheDocument()
  })

  test('shows sampling guidance in the empty state', async () => {
    vi.mocked(getChannelQuotaChanges).mockResolvedValueOnce({
      success: true,
      data: { items: [] },
    })
    vi.mocked(getChannelQuotaSamplingStatus).mockResolvedValueOnce({
      success: true,
      data: { enabled: true, interval_seconds: 300, max_channels: 20 },
    })

    renderPanel()

    expect(
      await screen.findByText('No account quota changes recorded yet. Enable quota sampling or query a provider account to start history.')
    ).toBeInTheDocument()
    expect(
      screen.getByText('Background quota sampling is enabled (every 5 minutes).')
    ).toBeInTheDocument()
  })

  test('keeps loading state visible while the request is pending', () => {
    vi.mocked(getChannelQuotaChanges).mockImplementationOnce(
      () => new Promise(() => undefined)
    )
    vi.mocked(getChannelQuotaSamplingStatus).mockImplementationOnce(
      () => new Promise(() => undefined)
    )

    renderPanel()

    expect(document.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThan(0)
    expect(screen.getByText('Account quota changes')).toBeInTheDocument()
  })

  test('renders a retryable error instead of an empty table', async () => {
    vi.mocked(getChannelQuotaChanges).mockRejectedValueOnce(
      new Error('quota endpoint unavailable')
    )
    vi.mocked(getChannelQuotaSamplingStatus).mockResolvedValueOnce({
      success: true,
      data: { enabled: false, interval_seconds: 300, max_channels: 20 },
    })

    renderPanel()

    expect(await screen.findByText('Unable to load account quota changes')).toBeInTheDocument()
    expect(screen.getByText('quota endpoint unavailable')).toBeInTheDocument()
  })

  test('filters rows by quota status without refetching', async () => {
    vi.mocked(getChannelQuotaChanges).mockResolvedValueOnce({
      success: true,
      data: {
        items: [
          { channel_id: 1, name: 'Codex', account_label: 'Codex account', status: 'success', direction: 'stable', current_available: 80, unit: 'percent' },
          { channel_id: 2, name: 'Claude', account_label: 'Claude account', status: 'unsupported', direction: 'unknown' },
        ],
      },
    })
    vi.mocked(getChannelQuotaSamplingStatus).mockResolvedValueOnce({
      success: true,
      data: { enabled: false, interval_seconds: 900, max_channels: 20 },
    })

    renderPanel()
    expect(await screen.findByText('Codex account')).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Status'), { target: { value: 'unsupported' } })
    expect(screen.queryByText('Codex account')).not.toBeInTheDocument()
    expect(screen.getByText('Claude account')).toBeInTheDocument()
    expect(getChannelQuotaChanges).toHaveBeenCalledTimes(1)
  })

  test('refreshes the quota query on demand', async () => {
    vi.mocked(getChannelQuotaChanges)
      .mockResolvedValueOnce({ success: true, data: { items: [] } })
      .mockResolvedValueOnce({ success: true, data: { items: [] } })
    vi.mocked(getChannelQuotaSamplingStatus).mockResolvedValueOnce({
      success: true,
      data: { enabled: false, interval_seconds: 900, max_channels: 20 },
    })

    renderPanel()
    await screen.findByText('No account quota changes recorded yet. Enable quota sampling or query a provider account to start history.')
    fireEvent.click(screen.getByLabelText('Refresh'))
    await waitFor(() => expect(getChannelQuotaChanges).toHaveBeenCalledTimes(2))
  })
})
