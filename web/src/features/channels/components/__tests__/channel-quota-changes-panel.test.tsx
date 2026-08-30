/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { describe, expect, test, vi } from 'vitest'

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
})
