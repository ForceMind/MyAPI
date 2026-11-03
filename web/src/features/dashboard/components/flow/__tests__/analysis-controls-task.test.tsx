import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { LogStatCards } from '../../models/log-stat-cards'
import { UserCharts } from '../../users/user-charts'
import { FlowNodeFilterControl } from '../flow-node-filter'

vi.mock('@visactor/react-vchart', () => ({ VChart: () => <div data-testid='analysis-chart' /> }))
vi.mock('@visactor/vchart', () => ({ ThemeManager: { setCurrentTheme: vi.fn() } }))

const clients: QueryClient[] = []

const longName = `synthetic/model-${'long-segment-'.repeat(16)}`
const stageLabels = {
  user: 'Users', token: 'API keys', node: 'Nodes', group: 'Groups',
  channel: 'Channels', model: 'Models',
}

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  vi.restoreAllMocks()
})

describe('analysis control tasks', () => {
  it('keeps the entire selected node name readable and removes exactly that node', async () => {
    const user = userEvent.setup()
    const remove = vi.fn()
    render(
      <FlowNodeFilterControl
        stages={['model']}
        stageLabels={stageLabels}
        metricLabel='Requests'
        formatMetricValue={(value) => String(value)}
        options={[]}
        selectedNodes={[{ kind: 'model', id: longName }]}
        onToggleNode={vi.fn()}
        onRemoveNode={remove}
        onClearNodes={vi.fn()}
      />
    )
    await user.click(screen.getByRole('button', { name: 'Filter by node' }))
    const label = screen.getByText(`Models: ${longName}`)
    expect(label).toBeVisible()
    expect(label).not.toHaveClass('truncate')
    const button = screen.getByRole('button', { name: 'Remove node filter' })
    expect(button).toHaveClass('min-h-11', 'min-w-11')
    expect(button).toHaveAttribute('aria-description', `Models: ${longName}`)
    await user.click(button)
    expect(remove).toHaveBeenCalledExactlyOnceWith({ kind: 'model', id: longName })
  })

  it('keeps selected node names inside the count popover and opening or closing it never changes filters', async () => {
    const toggle = vi.fn()
    const remove = vi.fn()
    const clear = vi.fn()
    render(<FlowNodeFilterControl stages={['model']} stageLabels={stageLabels} metricLabel='Requests' formatMetricValue={String} options={[]} selectedNodes={[{ kind: 'model', id: longName }, { kind: 'model', id: 'second-synthetic-model' }]} onToggleNode={toggle} onRemoveNode={remove} onClearNodes={clear} />)
    const user = userEvent.setup()
    const trigger = screen.getByRole('button', { name: 'Filter by node' })
    expect(trigger).toHaveTextContent('2')
    expect(screen.queryByText(`Models: ${longName}`)).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Clear' })).toBeVisible()
    trigger.focus()
    await user.keyboard('{Enter}')
    expect(screen.getByText(`Models: ${longName}`)).toBeVisible()
    expect(screen.getAllByRole('button', { name: 'Remove node filter' })).toHaveLength(2)
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByText(`Models: ${longName}`)).not.toBeInTheDocument())
    expect(trigger).toHaveFocus()
    expect(toggle).not.toHaveBeenCalled()
    expect(remove).not.toHaveBeenCalled()
    expect(clear).not.toHaveBeenCalled()
  })

  it.each(['transport', 'missing'])('keeps %s statistics unknown, reports graph failure, and explicitly retries', async (failure) => {
    const get = vi.spyOn(api, 'get')
    if (failure === 'transport') get.mockRejectedValueOnce(new Error('synthetic offline'))
    else get.mockResolvedValueOnce({ data: { success: true } })
    get.mockResolvedValue({ data: { success: true, data: [] } })
    const update = vi.fn()
    const user = userEvent.setup()
    render(
      <LogStatCards
        filters={{
          time_granularity: 'hour',
          start_timestamp: new Date('2026-01-01T00:00:00Z'),
          end_timestamp: new Date('2026-01-02T00:00:00Z'),
        }}
        onDataUpdate={update}
      />
    )
    expect(await screen.findByRole('alert')).toHaveTextContent('Failed to load model analytics')
    await waitFor(() => expect(screen.getAllByText('--')).toHaveLength(5))
    await waitFor(() => expect(update).toHaveBeenCalledWith([], false, expect.any(Number), true))
    expect(get).toHaveBeenCalledTimes(1)
    await user.click(screen.getByRole('button', { name: 'Retry' }))
    await waitFor(() => expect(get).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument())
    await waitFor(() => expect(screen.queryAllByText('--')).toHaveLength(0))
    await waitFor(() => expect(update).toHaveBeenLastCalledWith([], false, expect.any(Number)))
    expect(get.mock.calls[0]?.[1]).toEqual(get.mock.calls[1]?.[1])
  })

  it('shows a missing user-analysis response as an error and retries instead of plotting confirmed zero', async () => {
    const get = vi.spyOn(api, 'get').mockResolvedValueOnce({ data: { success: true } })
    get.mockResolvedValue({ data: { success: true, data: [] } })
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    clients.push(client)
    const user = userEvent.setup()
    render(
      <QueryClientProvider client={client}>
        <UserCharts filters={{ timeGranularity: 'hour', selectedRange: 1, topUserLimit: 10 }} onFiltersChange={vi.fn()} />
      </QueryClientProvider>
    )
    const alerts = await screen.findAllByRole('alert')
    const alert = alerts[0]
    expect(alert).toBeDefined()
    if (!alert) throw new Error('Expected user analysis error')
    expect(alert).toHaveTextContent('Failed to load')
    expect(screen.queryByTestId('analysis-chart')).not.toBeInTheDocument()
    await user.click(within(alert).getByRole('button', { name: 'Retry' }))
    await waitFor(() => expect(screen.queryAllByRole('alert')).toHaveLength(0))
    await waitFor(() => expect(get).toHaveBeenCalledTimes(2))
    expect(get.mock.calls[0]?.[1]).toEqual(get.mock.calls[1]?.[1])
  })
})
