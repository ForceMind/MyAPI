import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useSystemConfigStore } from '@/stores/system-config-store'

import type { PricingData } from '../../types'
import { ModelDetails } from '../model-details'

const emptyPricing: PricingData = {
  success: true,
  data: [],
  vendors: [],
  group_ratio: {},
  usable_group: {},
  supported_endpoint: {},
  auto_groups: [],
}
const populatedPricing: PricingData = {
  ...emptyPricing,
  data: [
    {
      id: 1,
      model_name: 'synthetic-text-model',
      quota_type: 0,
      model_ratio: 1,
      completion_ratio: 1,
      enable_groups: [],
    },
  ],
}
// Canvas rendering is an external browser boundary; these tests exercise request states.
vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))

const clients: QueryClient[] = []
const initialConfig = useSystemConfigStore.getState()
let pricingRequest: () => Promise<{ data: PricingData }>

async function renderDetails() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const root = createRootRoute()
  const details = createRoute({
    getParentRoute: () => root,
    path: '/pricing/$modelId/',
    validateSearch: (search: Record<string, unknown>) => search,
    component: ModelDetails,
  })
  const catalog = createRoute({
    getParentRoute: () => root,
    path: '/pricing',
    validateSearch: (search: Record<string, unknown>) => search,
    component: () => <h1>Model catalog</h1>,
  })
  const router = createRouter({
    routeTree: root.addChildren([details, catalog]),
    history: createMemoryHistory({
      initialEntries: [
        '/pricing/synthetic-text-model?search=synthetic&vendor=8&group=default&tokenUnit=K&view=table&rechargePrice=true',
      ],
    }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return { client, router }
}

beforeEach(() => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  pricingRequest = async () => ({ data: emptyPricing })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/pricing') return pricingRequest()
    if (url === '/api/status') {
      return { data: { success: true, data: { system_name: 'MyAPI' } } }
    }
    if (url === '/api/notice') return { data: { success: true, data: '' } }
    if (url === '/api/perf-metrics') {
      return {
        data: {
          success: true,
          data: { model_name: 'synthetic-text-model', groups: [] },
        },
      }
    }
    throw new Error(`Unexpected API endpoint: ${url}`)
  })
})

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  useSystemConfigStore.setState(initialConfig)
  localStorage.clear()
})

describe('model details request recovery', () => {
  it('shows a retryable pricing failure rather than claiming the model does not exist', async () => {
    pricingRequest = async () => {
      throw new Error('pricing unavailable')
    }
    await renderDetails()

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Failed to load models'
    )
    expect(screen.queryByText('Model not found')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Back to Models' })).toBeEnabled()
  })

  it('retries a failed pricing read and renders the recovered model without navigating away', async () => {
    pricingRequest = async () => {
      throw new Error('pricing unavailable')
    }
    const user = userEvent.setup()
    const { router } = await renderDetails()
    await screen.findByRole('alert')
    const originalSearch = router.state.location.search
    pricingRequest = async () => ({ data: populatedPricing })
    await user.click(screen.getByRole('button', { name: 'Retry' }))

    expect(
      await screen.findByRole('heading', { name: 'synthetic-text-model' })
    ).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/pricing/synthetic-text-model')
    expect(router.state.location.search).toEqual(originalSearch)
  })

  it('preserves pricing filters when returning from a failed detail request', async () => {
    pricingRequest = async () => {
      throw new Error('pricing unavailable')
    }
    const user = userEvent.setup()
    const { router } = await renderDetails()
    const originalSearch = router.state.location.search
    const alert = await screen.findByRole('alert')
    await user.click(
      within(alert).getByRole('button', { name: 'Back to Models' })
    )

    expect(
      await screen.findByRole('heading', { name: 'Model catalog' })
    ).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/pricing')
    expect(router.state.location.search).toEqual(originalSearch)
  })

  it('keeps successful missing models distinct from failed requests', async () => {
    await renderDetails()

    expect(await screen.findByText('Model not found')).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Retry' })
    ).not.toBeInTheDocument()
  })

  it('replaces stale model content with recovery controls after a pricing refresh fails', async () => {
    pricingRequest = async () => ({ data: populatedPricing })
    const { client } = await renderDetails()
    await screen.findByRole('heading', { name: 'synthetic-text-model' })
    pricingRequest = async () => {
      throw new Error('pricing unavailable')
    }
    await act(async () => client.invalidateQueries({ queryKey: ['pricing'] }))

    await waitFor(() =>
      expect(screen.getByRole('alert')).toHaveTextContent(
        'Failed to load models'
      )
    )
    expect(
      screen.queryByRole('heading', { name: 'synthetic-text-model' })
    ).not.toBeInTheDocument()
    expect(screen.queryByText('Model not found')).not.toBeInTheDocument()
  })
})
