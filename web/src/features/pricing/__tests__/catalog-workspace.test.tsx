import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { Pricing } from '..'
import type { PricingData } from '../types'

vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))
const clients: QueryClient[] = []
const initialConfig = useSystemConfigStore.getState()
const emptyPricing: PricingData = { success: true, data: [], vendors: [], group_ratio: {}, usable_group: {}, supported_endpoint: {}, auto_groups: [] }
let pricingRequest: () => Promise<{ data: PricingData }>

async function renderCatalog() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  clients.push(client)
  const root = createRootRoute()
  const catalog = createRoute({ getParentRoute: () => root, path: '/pricing/', validateSearch: (search: Record<string, unknown>) => search, component: Pricing })
  const router = createRouter({ routeTree: root.addChildren([catalog]), history: createMemoryHistory({ initialEntries: ['/pricing'] }) })
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
