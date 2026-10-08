import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { getPerfMetricsSummary } from '@/features/performance-metrics/api'

import { PerformanceOverview } from '../performance-overview'

vi.mock('@/features/performance-metrics/api', () => ({
  getPerfMetricsSummary: vi.fn(),
}))
const clients: QueryClient[] = []
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  vi.clearAllMocks()
})

describe('performance evidence feedback', () => {
  it('shows failure rather than no data, then retries to confirmed empty evidence', async () => {
    vi.mocked(getPerfMetricsSummary).mockRejectedValueOnce(new Error('offline'))
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    clients.push(client)
    render(
      <QueryClientProvider client={client}>
        <PerformanceOverview />
      </QueryClientProvider>
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Failed to load performance data'
    )
    expect(
      screen.queryByText('No performance data available')
    ).not.toBeInTheDocument()
    vi.mocked(getPerfMetricsSummary).mockResolvedValue({
      success: true,
      data: { models: [] },
    })
    await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(
      await screen.findByText('No performance data available')
    ).toBeInTheDocument()
  })
})
