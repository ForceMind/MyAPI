/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { getChannelQuotaHistory } from '../../../api'
import type { Channel } from '../../../types'
import { ChannelQuotaHistory } from '../channel-quota-history'

vi.mock('../../../api', () => ({
  getChannelQuotaHistory: vi.fn(),
}))

const baseChannel = {
  id: 12,
  type: 1,
  key: 'masked',
  status: 1,
  name: 'Example',
  created_time: 0,
  test_time: 0,
  response_time: 0,
  balance: 10,
  balance_updated_time: 0,
  models: 'gpt-5',
  group: 'default',
  used_quota: 0,
  channel_info: {
    is_multi_key: false,
    multi_key_size: 0,
    multi_key_polling_index: 0,
    multi_key_mode: 'random' as const,
  },
} as Channel

function renderQuota(channel = baseChannel) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ChannelQuotaHistory channel={channel} open />
    </QueryClientProvider>
  )
}

describe('channel quota history', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  test('explains why multi-key channels have no combined history', () => {
    renderQuota({
      ...baseChannel,
      channel_info: { ...baseChannel.channel_info, is_multi_key: true },
    })

    expect(screen.getByText('Quota history unavailable')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Multi-key channels do not expose one combined account quota.'
      )
    ).toBeInTheDocument()
    expect(getChannelQuotaHistory).not.toHaveBeenCalled()
  })

  test('shows an empty state while the channel has no samples', async () => {
    vi.mocked(getChannelQuotaHistory).mockResolvedValueOnce({
      success: true,
      data: {
        channel_id: baseChannel.id,
        start: 0,
        end: 1,
        limit: 500,
        points: [],
      },
    })

    renderQuota()
    expect(
      await screen.findByText('No quota history data yet')
    ).toBeInTheDocument()
  })

  test('shows an error state when history cannot be loaded', async () => {
    vi.mocked(getChannelQuotaHistory).mockRejectedValueOnce(
      new Error('quota endpoint unavailable')
    )

    renderQuota()
    expect(
      await screen.findByText('Unable to load quota history')
    ).toBeInTheDocument()
    expect(screen.getByText('quota endpoint unavailable')).toBeInTheDocument()
  })

  test('does not turn unsupported samples into zero values', async () => {
    vi.mocked(getChannelQuotaHistory).mockResolvedValueOnce({
      success: true,
      data: {
        channel_id: baseChannel.id,
        start: 0,
        end: 1,
        limit: 500,
        points: [{ timestamp: 1, status: 'unsupported' }],
      },
    })

    renderQuota()
    expect(
      await screen.findByText('Quota history is unavailable')
    ).toBeInTheDocument()
    expect(
      screen.getByText('The upstream did not return a usable quota value.')
    ).toBeInTheDocument()
  })
})
