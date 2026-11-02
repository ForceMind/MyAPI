import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { Pricing } from '..'
import { pricingSearchSchema } from '../catalog-state'
import { ModelDetails } from '../components/model-details'
import type { PricingData } from '../types'

vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))
const clients: QueryClient[] = []
const initialConfig = useSystemConfigStore.getState()
const emptyPricing: PricingData = { success: true, data: [], vendors: [], group_ratio: {}, usable_group: {}, supported_endpoint: {}, auto_groups: [] }
let pricingRequest: () => Promise<{ data: PricingData }>

async function renderCatalog(entry = '/pricing') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  clients.push(client)
  const root = createRootRoute()
  const catalog = createRoute({ getParentRoute: () => root, path: '/pricing/', validateSearch: pricingSearchSchema, component: Pricing })
  const details = createRoute({ getParentRoute: () => root, path: '/pricing/$modelId/', validateSearch: pricingSearchSchema, component: ModelDetails })
  const router = createRouter({ routeTree: root.addChildren([catalog, details]), history: createMemoryHistory({ initialEntries: [entry] }) })
  await router.load()
  render(<QueryClientProvider client={client}><RouterProvider router={router} /></QueryClientProvider>)
  return { client, router }
}

beforeEach(() => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  pricingRequest = async () => ({ data: emptyPricing })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/pricing') return pricingRequest()
    if (url === '/api/status') return { data: { success: true, data: { system_name: 'MyAPI' } } }
    if (url === '/api/perf-metrics/summary') return { data: { success: true, data: { models: [] } } }
    if (url === '/api/perf-metrics') return { data: { success: true, data: { groups: [] } } }
    if (url === '/api/notice') return { data: { success: true, data: '' } }
    throw new Error(`Unexpected API endpoint: ${url}`)
  })
})

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  useSystemConfigStore.setState(initialConfig)
  localStorage.clear()
})

describe('model catalog workspace', () => {
  it('shows a retryable catalog failure without claiming zero enabled models', async () => {
    pricingRequest = async () => { throw new Error('pricing unavailable') }
    await renderCatalog()
    expect(await screen.findByRole('alert')).toHaveTextContent('Failed to load models')
    expect(screen.getByRole('button', { name: 'Retry' })).toBeEnabled()
    expect(screen.queryByText('This site currently has 0 models enabled')).not.toBeInTheDocument()
    expect(screen.queryByText('No Models Found')).not.toBeInTheDocument()
  })
})

const populatedPricing: PricingData = {
  ...emptyPricing,
  vendors: [{ id: 8, name: 'Synthetic provider', icon: 'OpenAI' }],
  usable_group: { default: { desc: 'Synthetic group', ratio: 2 } },
  group_ratio: { default: 2 },
  data: Array.from({ length: 45 }, (_, index) => ({ id: index + 1, model_name: `synthetic-model-${String(index + 1).padStart(2, '0')}`, vendor_id: 8, quota_type: 0, model_ratio: 1, completion_ratio: 2, cache_ratio: 0, enable_groups: ['default'], supported_endpoint_types: ['openai'], tags: 'synthetic' })),
}

describe('model catalog shared navigation', () => {
  beforeEach(() => { pricingRequest = async () => ({ data: populatedPricing }) })

  it('keeps the current page across card and table views and changes page size through one paginator', async () => {
    const user = userEvent.setup()
    const { router } = await renderCatalog('/pricing?page=2')
    const firstVisibleModel = await screen.findByRole('button', { name: 'View model synthetic-model-21' })
    await waitFor(() => expect(firstVisibleModel).toBeVisible())
    await user.click(screen.getByRole('button', { name: 'Table view' }))
    await waitFor(() => expect(router.state.location.search).toMatchObject({ page: 2, view: 'table' }))
    expect(screen.getByRole('button', { name: 'View model synthetic-model-21' })).toBeVisible()
    expect(screen.queryByRole('button', { name: 'View model synthetic-model-01' })).not.toBeInTheDocument()
    const pagination = screen.getByRole('navigation', { name: 'Model catalog pagination' })
    await user.selectOptions(within(pagination).getByRole('combobox'), '50')
    await waitFor(() => expect(router.state.location.search).toMatchObject({ pageSize: 50 }))
    expect(router.state.location.search.page).toBeUndefined()
    expect(within(pagination).getByRole('button', { name: 'First page' })).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Card view' }))
    expect(await screen.findByRole('button', { name: 'View model synthetic-model-45' })).toBeVisible()
  })

  it('filters locally, resets the page and clears search with filters without an extra pricing read', async () => {
    const user = userEvent.setup()
    const { router } = await renderCatalog('/pricing?page=3&search=synthetic&vendor=Synthetic%20provider')
    await screen.findByRole('button', { name: 'View model synthetic-model-41' })
    await user.clear(screen.getByRole('textbox', { name: 'Search models' }))
    await user.type(screen.getByRole('textbox', { name: 'Search models' }), 'missing-model')
    expect(await screen.findByRole('heading', { name: 'No models found' })).toBeVisible()
    expect(router.state.location.search.page).toBeUndefined()
    await user.click(screen.getByRole('button', { name: 'Clear all filters' }))
    expect(await screen.findByRole('button', { name: 'View model synthetic-model-01' })).toBeVisible()
    expect(router.state.location.search.search).toBeUndefined()
    expect(router.state.location.search.vendor).toBeUndefined()
    expect(vi.mocked(api.get).mock.calls.filter(([url]) => url === '/api/pricing')).toHaveLength(1)
  })

  it('retains directory context through the drawer and standalone model page and returns focus once', async () => {
    const user = userEvent.setup()
    const { router, client } = await renderCatalog('/pricing?page=2&pageSize=20&group=default&sort=name&tokenUnit=K&rechargePrice=true')
    const opener = await screen.findByRole('button', { name: 'View model synthetic-model-21' })
    await user.click(opener)
    const dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('link', { name: 'Open model page' }))
    await screen.findByRole('heading', { name: 'synthetic-model-21' })
    expect(router.state.location.pathname).toBe('/pricing/synthetic-model-21')
    expect(router.state.location.search).toMatchObject({ page: 2, pageSize: 20, group: 'default', tokenUnit: 'K', rechargePrice: true })
    await user.click(screen.getByRole('button', { name: 'Back' }))
    const restored = await screen.findByRole('button', { name: 'View model synthetic-model-21' })
    await waitFor(() => expect(restored).toHaveFocus())
    await user.click(screen.getByRole('textbox', { name: 'Search models' }))
    await act(async () => { await client.invalidateQueries({ queryKey: ['pricing'] }) })
    expect(screen.getByRole('textbox', { name: 'Search models' })).toHaveFocus()
    expect(router.state.location.search).toMatchObject({ page: 2, pageSize: 20, group: 'default', tokenUnit: 'K', rechargePrice: true })
  })

  it('closing the drawer keeps page, price controls and the opening action', async () => {
    const user = userEvent.setup()
    const { router } = await renderCatalog('/pricing?page=2&tokenUnit=K')
    const opener = await screen.findByRole('button', { name: 'View model synthetic-model-21' })
    await user.click(opener)
    await screen.findByRole('dialog')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(opener).toHaveFocus()
    expect(router.state.location.search).toMatchObject({ page: 2, tokenUnit: 'K' })
  })

  it('retries a failed read and distinguishes an empty catalog from filtered results', async () => {
    pricingRequest = async () => { throw new Error('unavailable') }
    const user = userEvent.setup()
    await renderCatalog()
    await screen.findByRole('alert')
    pricingRequest = async () => ({ data: emptyPricing })
    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByRole('heading', { name: 'No models are available in this catalog.' })).toBeVisible()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('keeps loading distinct from a confirmed empty response', async () => {
    let resolve!: (value: { data: PricingData }) => void
    pricingRequest = () => new Promise((complete) => { resolve = complete })
    await renderCatalog()
    expect(screen.queryByText('Returned models')).not.toBeInTheDocument()
    expect(screen.queryByText('This site currently has 0 models enabled')).not.toBeInTheDocument()
    await act(async () => { resolve({ data: emptyPricing }) })
    expect(await screen.findByRole('heading', { name: 'No models are available in this catalog.' })).toBeVisible()
  })

  it('shows application-level read failures as retryable errors', async () => {
    pricingRequest = async () => ({ data: { ...emptyPricing, success: false } })
    await renderCatalog()
    expect(await screen.findByRole('alert')).toHaveTextContent('Failed to load models')
    expect(screen.queryByText('Returned models')).not.toBeInTheDocument()
  })
})

describe('model catalog mobile table', () => {
  it('mounts one reachable model action and preserves all table fields on small screens', async () => {
    const original = window.matchMedia
    vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({ ...original(query), matches: query === '(max-width: 767px)' || query === '(prefers-reduced-motion: reduce)' }))
    pricingRequest = async () => ({ data: { ...populatedPricing, data: [populatedPricing.data[0]] } })
    await renderCatalog('/pricing?view=table')
    const action = await screen.findByRole('button', { name: 'View model synthetic-model-01' })
    await waitFor(() => expect(action).toBeVisible())
    expect(screen.getAllByRole('button', { name: 'View model synthetic-model-01' })).toHaveLength(1)
    expect(screen.getByText('Cached')).toBeVisible()
    expect(screen.getByText('Tags')).toBeVisible()
    expect(screen.getByText('Endpoints')).toBeVisible()
    expect(screen.getAllByText('Synthetic provider').length).toBeGreaterThan(0)
    const user = userEvent.setup()
    action.focus()
    await user.keyboard('{Enter}')
    expect(await screen.findByRole('dialog')).toBeVisible()
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(action).toHaveFocus()
  })
})

describe('model drawer saved-read updates', () => {
  it('keeps the active tab and focus during a saved catalog refresh and returns to the live model action', async () => {
    pricingRequest = async () => ({ data: populatedPricing })
    const user = userEvent.setup()
    const { client } = await renderCatalog()
    await user.click(await screen.findByRole('button', { name: 'View model synthetic-model-01' }))
    const dialog = await screen.findByRole('dialog')
    const performance = within(dialog).getByRole('tab', { name: 'Performance' })
    await user.click(performance)
    expect(performance).toHaveFocus()
    pricingRequest = async () => ({ data: { ...populatedPricing, data: populatedPricing.data.map((model) => ({ ...model, description: 'Updated saved catalog description' })) } })
    await act(async () => { await client.invalidateQueries({ queryKey: ['pricing'] }) })
    expect(performance).toHaveAttribute('aria-selected', 'true')
    expect(performance).toHaveFocus()
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    await waitFor(() => expect(screen.getByRole('button', { name: 'View model synthetic-model-01' })).toHaveFocus())
  })
})

describe('model catalog wire identity', () => {
  it('returns focus to the chosen model when the pricing response contains model names without numeric IDs', async () => {
    // model.Pricing serializes model_name and does not serialize a model id.
    const wire = { ...populatedPricing, data: populatedPricing.data.slice(0, 2).map(({ id: _id, ...item }) => item) }
    pricingRequest = async () => ({ data: wire })
    const user = userEvent.setup()
    const { router } = await renderCatalog()
    const action = await screen.findByRole('button', { name: 'View model synthetic-model-02' })
    await user.click(action)
    await screen.findByRole('dialog')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    await waitFor(() => expect(screen.getByRole('button', { name: 'View model synthetic-model-02' })).toHaveFocus())
    await user.click(screen.getByRole('button', { name: 'View model synthetic-model-02' }))
    const dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('link', { name: 'Open model page' }))
    await screen.findByRole('heading', { name: 'synthetic-model-02' })
    expect(router.state.location.search.focusModel).toBe('synthetic-model-02')
    await user.click(screen.getByRole('button', { name: 'Back' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'View model synthetic-model-02' })).toHaveFocus())
  })
})
