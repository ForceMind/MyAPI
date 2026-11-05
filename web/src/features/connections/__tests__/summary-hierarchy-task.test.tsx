import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { ChannelModelDiscovery } from '@/features/channels/components/channel-model-discovery'
import { ApiKeysProvider } from '@/features/keys/components/api-keys-provider'
import { KeySharesPanel } from '@/features/keys/components/inspector/key-shares-panel'
import { ModelScopeDialog } from '@/features/keys/components/model-scope-dialog'
import { apiKeySchema } from '@/features/keys/types'
import { PoolKeyAllocations } from '@/features/personal-upstream/components/pool-key-allocations'
import { poolUsageFixture } from '@/features/personal-upstream/__tests__/pool-usage-fixture'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { ConnectionRouting } from '../connection-sections'

const originalAuth = useAuthStore.getState()
const clients: QueryClient[] = []
const modelName = `synthetic-complete-model-${'long-segment-'.repeat(12)}`
const key = apiKeySchema.parse({ id: 21, name: 'Synthetic Key', key: 'masked', status: 1, remain_quota: 100, used_quota: 0, unlimited_quota: false, expired_time: -1, created_time: 1, accessed_time: 1, group: 'default', model_limits_enabled: true })

function client() {
  const value = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  clients.push(value)
  return value
}
async function renderRoute(element: ReactNode) {
  const root = createRootRoute({ component: () => element })
  const router = createRouter({ routeTree: root, history: createMemoryHistory({ initialEntries: ['/'] }) })
  await router.load()
  render(<QueryClientProvider client={client()}><RouterProvider router={router} /></QueryClientProvider>)
}
function scopeResponse(models: string[], revision = 'a'.repeat(64)) {
  return { data: { success: true, data: { token_id: 21, token_name: 'Synthetic Key', model_limits_enabled: true, models, revision } } }
}
beforeEach(() => {
  useAuthStore.getState().auth.setUser({ id: 1, role: 100, username: 'synthetic-owner' })
  vi.spyOn(api, 'get').mockRejectedValue(new Error('Unexpected synthetic read'))
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected synthetic write'))
  vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected synthetic write'))
})
afterEach(() => {
  cleanup()
  clients.splice(0).forEach((value) => value.clear())
  useAuthStore.setState(originalAuth)
  vi.restoreAllMocks()
})

it('routing counts expand complete loaded models and groups through explicit keyboard and click actions without any request', async () => {
  const user = userEvent.setup()
  render(<QueryClientProvider client={client()}><ConnectionRouting item={{ id: 7, type: 1, name: 'Synthetic connection', status: 1, models: `alpha,${modelName}`, group: 'default,synthetic-team', priority: 0, weight: 0 }} showPreview={false} canWrite={false} /></QueryClientProvider>)
  const models = screen.getByRole('button', { name: 'Models (2)' })
  const groups = screen.getByRole('button', { name: 'Groups (2)' })
  expect(models).toHaveAttribute('aria-expanded', 'false')
  expect(groups).toHaveAttribute('aria-expanded', 'false')
  expect(screen.getByText(modelName)).not.toBeVisible()
  expect(screen.getByText('synthetic-team')).not.toBeVisible()
  act(() => models.focus())
  await user.keyboard('{Enter}')
  expect(models).toHaveAttribute('aria-expanded', 'true')
  expect(screen.getByText(modelName)).toBeVisible()
  await user.click(groups)
  expect(screen.getByText('default')).toBeVisible()
  expect(screen.getByText('synthetic-team')).toBeVisible()
  expect(api.get).not.toHaveBeenCalled()
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
})
it('missing routing metadata remains Unknown while an explicit zero weight is retained', () => {
  render(<QueryClientProvider client={client()}><ConnectionRouting item={{ id: 7, type: 1, name: 'Synthetic connection', status: 1, models: null, group: null, priority: null, weight: 0 }} showPreview={false} canWrite={false} /></QueryClientProvider>)
  expect(screen.getAllByText('Unknown')).toHaveLength(3)
  expect(screen.getByText('0')).toBeVisible()
  expect(screen.queryByRole('button', { name: 'Models (0)' })).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Groups (0)' })).not.toBeInTheDocument()
})
it('discovery expands saved evidence without refreshing upstream and keeps failed or stale status outside', async () => {
  const evidence = { models: ['alpha', modelName], source: 'openai_models', status: 'failed', fetched_at: 1728000000, checked_at: 1729000000, stale: true }
  vi.mocked(api.get).mockResolvedValue({ data: { success: true, data: evidence } })
  const user = userEvent.setup()
  render(<QueryClientProvider client={client()}><ChannelModelDiscovery channelId={7} /></QueryClientProvider>)
  const models = await screen.findByRole('button', { name: 'Discovered models (2)' })
  expect(models).toHaveAttribute('aria-expanded', 'false')
  expect(screen.getByText('Model discovery failed; previous evidence is retained')).toBeVisible()
  expect(screen.getByText('Stale discovery evidence')).toBeVisible()
  expect(screen.getByText(modelName)).not.toBeVisible()
  act(() => models.focus())
  await user.keyboard(' ')
  expect(screen.getByText(modelName)).toBeVisible()
  expect(api.get).toHaveBeenCalledTimes(1)
  expect(api.get).toHaveBeenCalledWith('/api/channel/model-discovery/7', expect.any(Object))
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
})
it('allocation totals, management and warnings precede a complete local list with accurate Key return context', async () => {
  const usage = poolUsageFixture({ decomposition_state: 'inconsistent', state: 'discontinuity', estimated_used_micro: null })
  const allocation = usage.allocations[0]
  if (!allocation) {
    throw new Error('Expected synthetic allocation')
  }
  allocation.estimated_used_micro = null
  const user = userEvent.setup()
  await renderRoute(<PoolKeyAllocations usage={usage} />)
  const toggle = screen.getByRole('button', { name: 'Key allocations (3)' })
  expect(toggle).toHaveAttribute('aria-expanded', 'false')
  expect(screen.getByText('Total allocated')).toBeVisible()
  expect(screen.getByText('Total allocated').nextElementSibling).toHaveTextContent('60%')
  expect(screen.getByText('Unallocated').nextElementSibling).toHaveTextContent('40%')
  expect(screen.getByText('Manage Key shares')).toBeVisible()
  expect(screen.getByText('Paused: 1')).toBeVisible()
  expect(screen.getByText('Unknown usage: 1 Key(s)')).toBeVisible()
  expect(screen.getByText('Desktop app')).not.toBeVisible()
  expect(screen.getByText('Total allocated').compareDocumentPosition(toggle) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  await user.click(toggle)
  expect(screen.getByText('Desktop app')).toBeVisible()
  const row = screen.getByTestId('pool-key-21')
  expect(within(row).getByTestId('allocation-estimated-usage')).toHaveTextContent('Unknown')
  const link = within(row).getByRole('link', { name: 'Key #21' })
  const href = link.getAttribute('href')
  if (!href) {
    throw new Error('Expected Key return link')
  }
  const context = new URL(href, 'https://synthetic.invalid').searchParams.get('connectionContext')
  if (!context) {
    throw new Error('Expected preserved connection context')
  }
  expect(JSON.parse(context)).toEqual({ connection: { connection: 1 }, pool: 7, key: 21 })
  expect(api.get).not.toHaveBeenCalled()
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
})
it('collapsed allocations expose over-share and attribution warnings while expanded values retain the original units and progress bounds', async () => {
  const remove = vi.spyOn(api, 'delete').mockRejectedValue(new Error('Unexpected synthetic deletion'))
  const usage = poolUsageFixture({
    allocated_bps: 3000,
    estimated_used_micro: 17500000,
    decomposition_state: 'inconsistent',
    state: 'discontinuity',
    allocations: [{
      token_id: 21,
      token_name: 'Synthetic over-share Key',
      token_state: 'enabled',
      share_bps: 1000,
      estimated_used_micro: 12500000,
      pending_attempts: 2,
      unsampled_attempts: 0,
      uncertain_attempts: 0,
    }, {
      token_id: 22,
      token_name: 'Synthetic unsampled Key',
      token_state: 'enabled',
      share_bps: 1000,
      estimated_used_micro: 5000000,
      pending_attempts: 0,
      unsampled_attempts: 3,
      uncertain_attempts: 0,
    }, {
      token_id: 23,
      token_name: 'Synthetic uncertain Key',
      token_state: 'enabled',
      share_bps: 1000,
      estimated_used_micro: 0,
      pending_attempts: 0,
      unsampled_attempts: 0,
      uncertain_attempts: 1,
    }],
  })
  const user = userEvent.setup()
  await renderRoute(<PoolKeyAllocations usage={usage} />)
  const toggle = screen.getByRole('button', { name: 'Key allocations (3)' })
  expect(toggle).toHaveAttribute('aria-expanded', 'false')
  expect(screen.getByText('Over-share Key allocations: 1')).toBeVisible()
  expect(screen.getByText('Key allocations awaiting attribution: 3')).toBeVisible()
  const row = screen.getByTestId('pool-key-21')
  expect(within(row).getByText('Over the allocated share')).not.toBeVisible()
  await user.click(toggle)
  expect(within(row).getByText('Over the allocated share')).toBeVisible()
  expect(within(row).getByTestId('allocation-estimated-usage')).toHaveTextContent('12.5 percentage points')
  expect(within(row).getByTestId('allocation-used-within-share')).toHaveTextContent('125%')
  expect(within(row).getByText('Pending requests').nextElementSibling).toHaveTextContent('2')
  expect(within(row).getByText('Unsampled requests').nextElementSibling).toHaveTextContent('0')
  expect(within(row).getByText('Uncertain requests').nextElementSibling).toHaveTextContent('0')
  expect(within(screen.getByTestId('pool-key-22')).getByText('Unsampled requests').nextElementSibling).toHaveTextContent('3')
  expect(within(screen.getByTestId('pool-key-23')).getByText('Uncertain requests').nextElementSibling).toHaveTextContent('1')
  const bars = within(row).getAllByRole('progressbar')
  expect(bars).toHaveLength(2)
  expect(bars[0]).toHaveAttribute('aria-valuenow', '10')
  expect(bars[1]).toHaveAttribute('aria-valuenow', '100')
  expect(api.get).not.toHaveBeenCalled()
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
  expect(remove).not.toHaveBeenCalled()
})
it('window shares keep paused, unknown and attribution warnings visible while complete per-window observations stay local', async () => {
  const base = poolUsageFixture()
  vi.mocked(api.get).mockResolvedValue({ data: { success: true, data: { token_id: 21, pools: [
    { ...base, channel_name: 'Unknown synthetic window', share_bps: 0, estimated_used_micro: 0, baseline_time: 0, state: 'discontinuity' },
    { ...base, pool_id: 8, snapshot_id: 10, channel_name: 'Zero synthetic window', share_bps: 1000, estimated_used_micro: 0, pending_attempts: 0, unsampled_attempts: 0 },
  ] } } })
  const user = userEvent.setup()
  render(<QueryClientProvider client={client()}><KeySharesPanel apiKey={key} /></QueryClientProvider>)
  const unknown = await screen.findByRole('region', { name: /Unknown synthetic window/ })
  const details = within(unknown).getByRole('button', { name: 'Observation details' })
  expect(details).toHaveAttribute('aria-expanded', 'false')
  expect(within(unknown).getByText('Requests paused')).toBeVisible()
  expect(within(unknown).getByText('Estimated usage of share: Unknown')).toBeVisible()
  expect(within(unknown).getByText('Usage is still awaiting attribution.')).toBeVisible()
  expect(within(unknown).getByText('Window unavailable or stale; later requests are paused.')).toBeVisible()
  expect(within(unknown).getByText(/Observed at/)).not.toBeVisible()
  await user.click(details)
  expect(within(unknown).getByText('Unknown')).toBeVisible()
  expect(within(unknown).getByText(/Observed at/)).toBeVisible()
  const zero = screen.getByRole('region', { name: /Zero synthetic window/ })
  await user.click(within(zero).getByRole('button', { name: 'Observation details' }))
  expect(within(zero).getByText('0%')).toBeVisible()
  expect(api.get).toHaveBeenCalledTimes(1)
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
})
it('a share read error remains visible without advertising an empty or zero window count', async () => {
  render(<QueryClientProvider client={client()}><KeySharesPanel apiKey={key} /></QueryClientProvider>)
  expect(await screen.findByRole('alert')).toHaveTextContent('Could not load subscription shares.')
  expect(screen.getByRole('button', { name: 'Retry' })).toBeVisible()
  expect(screen.queryByText('No subscription share is configured for this Key.')).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Observation details' })).not.toBeInTheDocument()
})
it('saved model IDs retain old, empty and duplicate entries through collapse, independent removal and exact save', async () => {
  let current = scopeResponse(['retired-model', '', 'retired-model'])
  const get = vi.mocked(api.get).mockImplementation(async (url) => {
    if (url === '/api/token/21/model-limits') return current
    if (url === '/api/user/models') return { data: { success: true, data: [] } }
    throw new Error(`Unexpected synthetic scope GET ${url}`)
  })
  const put = vi.mocked(api.put).mockImplementation(async (_url, body) => {
    const command = body as { models: string[] }
    current = scopeResponse(command.models, 'b'.repeat(64))
    return current
  })
  const user = userEvent.setup()
  const close = vi.fn()
  render(<QueryClientProvider client={client()}><ApiKeysProvider><ModelScopeDialog apiKey={key} onClose={close} /></ApiKeysProvider></QueryClientProvider>)
  const saved = await screen.findByRole('button', { name: 'Saved model IDs (3)' })
  expect(saved).toHaveAttribute('aria-expanded', 'false')
  expect(screen.getByRole('textbox', { name: 'Selected model IDs' })).toBeVisible()
  expect(screen.getByText('Empty model ID')).not.toBeVisible()
  const reads = get.mock.calls.length
  act(() => saved.focus())
  await user.keyboard('{Enter}')
  expect(screen.getByText('Empty model ID')).toBeVisible()
  expect(screen.getAllByText('retired-model')).toHaveLength(2)
  const remove = screen.getAllByRole('button', { name: 'Remove retired-model' })[0]
  if (!remove) {
    throw new Error('Expected independent model removal')
  }
  await user.click(remove)
  expect(saved).toHaveAccessibleName('Saved model IDs (2)')
  await user.click(saved)
  expect(screen.getByText('Empty model ID')).not.toBeVisible()
  await user.click(saved)
  expect(screen.getByText('Empty model ID')).toBeVisible()
  expect(screen.getByText('retired-model')).toBeVisible()
  expect(get).toHaveBeenCalledTimes(reads)
  expect(put).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Save model scope' }))
  await waitFor(() => expect(close).toHaveBeenCalledOnce())
  expect(put).toHaveBeenCalledExactlyOnceWith('/api/token/21/model-limits', {
    expected_revision: 'a'.repeat(64), model_limits_enabled: true, models: ['', 'retired-model'],
  }, expect.objectContaining({ skipAuthRefresh: true }))
  expect(api.post).not.toHaveBeenCalled()
})
