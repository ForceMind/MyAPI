import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import type { PricingModel } from '../../types'
import { ModelDetailsPerformance } from '../model-details-performance'

const model: PricingModel = {
  id: 1,
  model_name: 'synthetic-text-model',
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 1,
  enable_groups: [],
}
const emptyMetrics = {
  success: true,
  data: { model_name: model.model_name, groups: [] },
}
// Canvas rendering is an external browser boundary; these tests exercise request states.
vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))

const clients: QueryClient[] = []

function renderPerformance() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  render(
    <QueryClientProvider client={client}>
      <ModelDetailsPerformance model={model} />
    </QueryClientProvider>
  )
  return client
}

afterEach(() => clients.splice(0).forEach((client) => client.clear()))

describe('model performance request states', () => {
  it('shows loading until the request confirms that no metrics are available', async () => {
    let resolveMetrics!: (value: { data: typeof emptyMetrics }) => void
    vi.spyOn(api, 'get').mockReturnValue(
      new Promise((resolve) => {
        resolveMetrics = resolve
      })
    )
    renderPerformance()

    expect(screen.getByRole('status')).toHaveTextContent('Loading...')
    expect(
      screen.queryByText(
        'Performance data is not yet available for this model.'
      )
    ).not.toBeInTheDocument()
    resolveMetrics({ data: emptyMetrics })
    expect(
      await screen.findByText(
        'Performance data is not yet available for this model.'
      )
    ).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('shows a retryable failure for transport errors and only shows empty metrics after recovery', async () => {
    vi.spyOn(api, 'get')
      .mockRejectedValueOnce(new Error('unavailable'))
      .mockResolvedValue({ data: emptyMetrics })
    const user = userEvent.setup()
    renderPerformance()

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Failed to load performance data'
    )
    expect(
      screen.queryByText(
        'Performance data is not yet available for this model.'
      )
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(
      await screen.findByText(
        'Performance data is not yet available for this model.'
      )
    ).toBeInTheDocument()
    expect(api.get).toHaveBeenLastCalledWith('/api/perf-metrics', {
      params: { model: model.model_name, hours: 24 },
    })
  })

  it('does not preserve an empty-data claim when the next metrics refresh fails', async () => {
    const request = vi
      .spyOn(api, 'get')
      .mockResolvedValue({ data: emptyMetrics })
    const client = renderPerformance()
    await screen.findByText(
      'Performance data is not yet available for this model.'
    )
    request.mockRejectedValueOnce(new Error('refresh failed'))
    await act(async () =>
      client.invalidateQueries({ queryKey: ['perf-metrics', model.model_name] })
    )

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Failed to load performance data'
    )
    expect(
      screen.queryByText(
        'Performance data is not yet available for this model.'
      )
    ).not.toBeInTheDocument()
  })
})
