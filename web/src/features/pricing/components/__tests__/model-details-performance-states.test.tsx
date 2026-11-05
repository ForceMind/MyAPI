import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, within } from '@testing-library/react'
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
  it.each([
    { success: false, message: 'Synthetic metrics rejected' },
    { success: true },
    { success: true, data: { model_name: model.model_name } },
    { success: true, data: { model_name: model.model_name, groups: [null] } },
    { success: true, data: { model_name: model.model_name, groups: [{ group: 'actual', series: null }] } },
    { success: true, data: { model_name: model.model_name, groups: [{ group: 'actual', series: [null] }] } },
    { success: true, data: { model_name: model.model_name, groups: [{ group: 'actual', series: [{ ts: 'unknown' }] }] } },
    { success: true, data: { model_name: model.model_name, groups: [{ group: 'actual', series: [{ ts: Number.MAX_VALUE }] }] } },
  ])('a rejected or malformed metrics response is a visible retryable failure: %j', async (response) => {
    vi.spyOn(api, 'get').mockResolvedValue({ data: response })
    renderPerformance()
    expect(await screen.findByRole('alert')).toHaveTextContent('Failed to load performance data')
    expect(screen.getByRole('button', { name: 'Retry' })).toBeVisible()
    expect(screen.queryByText('Performance data is not yet available for this model.')).not.toBeInTheDocument()
  })

  it('a partial success-rate series does not claim that the unobserved intervals had no incidents', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ data: {
      success: true,
      data: { model_name: model.model_name, groups: [{
        group: 'actual-group', avg_ttft_ms: 15, avg_latency_ms: 20, avg_tps: 10, success_rate: 100,
        series: [
          { ts: 1700000000, avg_ttft_ms: 15, avg_latency_ms: 20, avg_tps: 10, success_rate: 100 },
          { ts: 1700003600, avg_ttft_ms: 15, avg_latency_ms: 20, avg_tps: 10, success_rate: null },
        ],
      }] },
    } })
    renderPerformance()
    await screen.findByText('Per-group performance')
    expect(screen.queryByText('No incidents in the last 24 hours')).not.toBeInTheDocument()
    expect(screen.queryByText('0.00%')).not.toBeInTheDocument()
    const knownRates = screen.getAllByText('100.00%')
    expect(knownRates).toHaveLength(2)
    for (const rate of knownRates) expect(rate).toBeVisible()
    expect(api.get).toHaveBeenCalledOnce()
  })

  it.each([
    { successRate: null, expected: '—' },
    { successRate: 0, expected: '0.00%' },
  ])('missing success rate is unknown while explicit zero is preserved: $successRate', async ({ successRate, expected }) => {
    vi.spyOn(api, 'get').mockResolvedValue({ data: {
      success: true,
      data: { model_name: model.model_name, groups: [{
        group: 'actual-group', avg_ttft_ms: 15, avg_latency_ms: 20, avg_tps: 10, success_rate: successRate,
        series: [{ ts: 1700000000, avg_ttft_ms: 15, avg_latency_ms: 20, avg_tps: 10, success_rate: successRate }],
      }] },
    } })
    renderPerformance()
    await screen.findByText('Per-group performance')
    const label = screen.getByText('Average group success rate')
    const summary = label?.parentElement
    if (!summary) throw new Error('Expected the success-rate summary')
    expect(within(summary).getByText(expected)).toBeVisible()
    if (successRate === null) {
      expect(screen.queryByText('No incidents in the last 24 hours')).not.toBeInTheDocument()
      expect(screen.queryByText('0.00%')).not.toBeInTheDocument()
    }
    expect(api.get).toHaveBeenCalledOnce()
  })

  it('the group row preserves its reported success rate instead of substituting an unweighted trend average', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({ data: {
      success: true,
      data: { model_name: model.model_name, groups: [{
        group: 'actual-group', avg_ttft_ms: 15, avg_latency_ms: 20, avg_tps: 10, success_rate: 99.01,
        series: [
          { ts: 1700000000, avg_ttft_ms: 15, avg_latency_ms: 20, avg_tps: 10, success_rate: 100 },
          { ts: 1700003600, avg_ttft_ms: 15, avg_latency_ms: 20, avg_tps: 10, success_rate: 0 },
        ],
      }] },
    } })
    renderPerformance()
    await screen.findByText('Per-group performance')
    expect(screen.getAllByText('99.01%')).toHaveLength(2)
    expect(screen.queryByText('50.0%')).not.toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'Success-rate trend' })).toBeVisible()
  })

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
