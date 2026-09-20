import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { getAffinityUsageCache } from './api'
import { CacheStatsDialog } from './cache-stats-dialog'

vi.mock('./api', () => ({ getAffinityUsageCache: vi.fn() }))
vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))

const target = {
  rule_name: 'codex cache',
  using_group: 'default',
  key_hint: 'key-…abcd',
  key_fp: 'abcd',
}

describe('CacheStatsDialog', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  test('shows a retryable error and clears stale stats when the cache query fails', async () => {
    vi.mocked(getAffinityUsageCache)
      .mockResolvedValueOnce({
        success: true,
        data: { total: 4, hit: 3 },
      })
      .mockRejectedValueOnce(new Error('network unavailable'))
      .mockResolvedValueOnce({
        success: true,
        data: { total: 8, hit: 6 },
      })

    const view = render(
      <CacheStatsDialog open onOpenChange={vi.fn()} target={target} />
    )
    expect(await screen.findByText('3/4 (75.00%)')).toBeInTheDocument()

    view.rerender(
      <CacheStatsDialog open={false} onOpenChange={vi.fn()} target={target} />
    )
    view.rerender(
      <CacheStatsDialog open onOpenChange={vi.fn()} target={target} />
    )

    expect(await screen.findByText('Request failed')).toBeInTheDocument()
    expect(screen.queryByText('3/4 (75.00%)')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('6/8 (75.00%)')).toBeInTheDocument()
    await waitFor(() => {
      expect(getAffinityUsageCache).toHaveBeenCalledTimes(3)
    })
  })
})
