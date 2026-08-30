/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import { getCodexUsageHistory } from '../../../api'
import { CodexUsageHistoryPanel } from '../codex-usage-dialog'

vi.mock('../../../api', () => ({
  getCodexUsageHistory: vi.fn(),
  getCodexResetCredits: vi.fn(),
  resetCodexUsage: vi.fn(),
}))

function renderHistory() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <CodexUsageHistoryPanel channelId={12} open />
    </QueryClientProvider>
  )
}

describe('Codex usage history panel', () => {
  test('shows an explicit empty state when no observations exist', async () => {
    vi.mocked(getCodexUsageHistory).mockResolvedValueOnce({
      success: true,
      data: { points: [] },
    })

    renderHistory()

    expect(
      await screen.findByText('No Codex usage history yet')
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        'Usage history appears after the first successful usage query.'
      )
    ).toBeInTheDocument()
  })

  test('shows the upstream error instead of rendering a chart', async () => {
    vi.mocked(getCodexUsageHistory).mockRejectedValueOnce(
      new Error('history endpoint unavailable')
    )

    renderHistory()

    expect(
      await screen.findByText('Unable to load Codex usage history')
    ).toBeInTheDocument()
    expect(screen.getByText('history endpoint unavailable')).toBeInTheDocument()
  })

  test('renders only real percentage series and preserves gaps', async () => {
    vi.mocked(getCodexUsageHistory).mockResolvedValueOnce({
      success: true,
      data: {
        points: [
          { timestamp: 1_700_000_000, primary_used_percent: 12 },
          { timestamp: 1_700_003_600, status: 'error' },
          { timestamp: 1_700_007_200, primary_used_percent: 19 },
        ],
      },
    })

    renderHistory()

    expect(await screen.findByText('Usage trend')).toBeInTheDocument()
    expect(screen.getByText('Samples: 2')).toBeInTheDocument()
    expect(screen.getByText('● Primary window')).toBeInTheDocument()
    expect(screen.queryByText('● Secondary window')).not.toBeInTheDocument()
  })
})
