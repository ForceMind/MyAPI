/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { useEffect } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import {
  getChannelQuotaHistory,
  getCodexQuotaSeries,
  getCodexUsage,
} from '../../../api'
import type { Channel } from '../../../types'
import { ChannelsProvider, useChannels } from '../../channels-provider'
import { BalanceQueryDialog } from '../balance-query-dialog'

vi.mock('../../../api', () => ({
  getChannelQuotaHistory: vi.fn(),
  getCodexQuotaSeries: vi.fn(),
  getCodexUsage: vi.fn(),
  updateChannelBalance: vi.fn(),
}))

vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))

function CodexBalanceQueryHarness() {
  const { setCurrentRow } = useChannels()
  useEffect(() => {
    setCurrentRow({
      id: 57,
      name: 'Codex',
      type: 57,
      balance: 0,
    } as Channel)
  }, [setCurrentRow])

  return <BalanceQueryDialog open onOpenChange={vi.fn()} />
}

function renderBalanceQueryDialog() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ChannelsProvider>
        <CodexBalanceQueryHarness />
      </ChannelsProvider>
    </QueryClientProvider>
  )
}

describe('Codex balance query refresh', () => {
  beforeEach(() => {
    vi.stubGlobal('localStorage', {
      getItem: vi.fn(() => null),
      setItem: vi.fn(),
      removeItem: vi.fn(),
    })
  })

  afterEach(() => {
    vi.clearAllMocks()
    vi.unstubAllGlobals()
  })

  test('clears current windows after the production refresh callback fails and retains the history tab', async () => {
    vi.mocked(getCodexUsage)
      .mockResolvedValueOnce({
        success: true,
        data: {
          rate_limit: {
            primary_window: {
              used_percent: 20,
              limit_window_seconds: 18000,
            },
          },
        },
      })
      .mockRejectedValueOnce(new Error('Codex refresh failed'))
    vi.mocked(getCodexQuotaSeries).mockResolvedValue({
      success: true,
      data: { items: [] },
    })
    vi.mocked(getChannelQuotaHistory).mockResolvedValue({
      success: true,
      data: { channel_id: 57, start: 1, end: 2, limit: 5000, points: [] },
    })

    renderBalanceQueryDialog()
    expect(await screen.findByText('5-Hour Window')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    expect(await screen.findByText('Codex refresh failed')).toBeInTheDocument()
    expect(screen.queryByText('5-Hour Window')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('tab', { name: 'History trend' }))
    expect(screen.getByTestId('codex-usage-history-panel')).toBeInTheDocument()
  })
})
