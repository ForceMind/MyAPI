import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { DirectionProvider } from '@/context/direction-provider'
import { ThemeProvider } from '@/context/theme-provider'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { ModelLeaderboard } from '../components/model-leaderboard'
import { MarketShareSection } from '../components/market-share-section'
import { Rankings } from '../index'
import type { ModelRanking, VendorRanking } from '../types'

// Isolate the third-party chart renderer; ranking components and data stay real.
vi.mock('@visactor/react-vchart', () => ({ VChart: () => <div data-testid='ranking-chart' /> }))
vi.mock('@visactor/vchart', () => ({ ThemeManager: { setCurrentTheme: vi.fn() } }))

const originalAuth = useAuthStore.getState()
let client: QueryClient
const empty = { models: [], vendors: [], top_movers: [], top_droppers: [], models_history: { points: [], models: [], buckets: 0 }, vendor_share_history: { points: [], vendors: [], buckets: 0 } }
beforeEach(() => {
  useAuthStore.getState().auth.reset('idle')
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  client.setQueryData(['status'], {})
  client.setQueryData(['notice'], { success: true, data: '' })
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })
async function renderRankings(element: ReactNode) {
  const root = createRootRoute()
  const page = createRoute({ getParentRoute: () => root, path: '/rankings/', validateSearch: (search: Record<string, unknown>) => search, component: () => element })
  const router = createRouter({ routeTree: root.addChildren([page]), history: createMemoryHistory({ initialEntries: ['/rankings/?period=month'] }) })
  await router.load()
  render(<QueryClientProvider client={client}><DirectionProvider><ThemeProvider><TooltipProvider><RouterProvider router={router} /></TooltipProvider></ThemeProvider></DirectionProvider></QueryClientProvider>)
}

it('rejects a business-failed snapshot and retries the same selected period as a read before showing a confirmed empty ranking', async () => {
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url !== '/api/rankings') throw new Error(`Unexpected GET ${url}`)
    return { data: { success: false, message: 'Synthetic ranking read rejected', data: { ...empty, models: [{ model_name: 'Unconfirmed model', vendor: 'Synthetic vendor', rank: 1, category: 'all', total_tokens: 0, share: 0, growth_pct: 0 }] } } }
  })
  await renderRankings(<Rankings />)
  expect(await screen.findByRole('alert')).toHaveTextContent('Synthetic ranking read rejected')
  expect(screen.queryByText('Unconfirmed model')).not.toBeInTheDocument()
  get.mockResolvedValue({ data: { success: true, data: empty } })
  await userEvent.setup().click(screen.getByRole('button', { name: 'Retry' }))
  await screen.findByText('No models match the selected filters')
  expect(get.mock.calls.filter(([url]) => url === '/api/rankings').map((call) => call[1])).toEqual([{ params: { period: 'month' } }, { params: { period: 'month' } }])
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

it('keeps full model and vendor names readable and missing usage or growth unknown while retaining the exact zero share and supplemental fields', async () => {
  const row: ModelRanking = { rank: 6, previous_rank: 8, model_name: 'synthetic-model-' + 'long-name-'.repeat(12), vendor: 'synthetic-vendor-' + 'long-name-'.repeat(10), category: 'programming', total_tokens: null as unknown as number, share: 0, growth_pct: Number.NaN }
  await renderRankings(<ModelLeaderboard rows={[row]} />)
  const name = screen.getByRole('link', { name: row.model_name })
  expect(name).toHaveTextContent(row.model_name)
  expect(name).not.toHaveClass('truncate')
  expect(screen.getByRole('link', { name: row.vendor.toLowerCase() }).closest('p')).not.toHaveClass('truncate')
  expect(screen.getByText('0%')).not.toBeVisible()
  expect(screen.getAllByText('Unknown')).toHaveLength(2)
  await userEvent.setup().click(screen.getByText('Ranking details'))
  const details = screen.getByText('Ranking details').closest('details')
  if (!details) {
    throw new Error('Expected ranking details element')
  }
  expect(within(details).getByText('programming')).toBeVisible()
  expect(within(details).getByText('8')).toBeVisible()
  expect(within(details).getByText('0%')).toBeVisible()
})

it('lets readers expand beyond the original twelve vendors and reach existing supplemental vendor fields', async () => {
  const rows: VendorRanking[] = Array.from({ length: 13 }, (_, index) => ({ rank: index + 1, vendor: `Synthetic vendor ${index + 1}`, total_tokens: 0, share: 0, growth_pct: 0, models_count: index + 2, top_model: `synthetic-top-model-${index + 1}` }))
  await renderRankings(<MarketShareSection rows={rows} history={{ points: [], vendors: [], buckets: 0 }} period='month' />)
  expect(screen.queryByRole('link', { name: 'Synthetic vendor 13' })).not.toBeInTheDocument()
  await userEvent.setup().click(screen.getByRole('button', { name: 'Show all vendors' }))
  const last = screen.getByRole('link', { name: 'Synthetic vendor 13' }).closest('li')
  if (!last) {
    throw new Error('Expected final vendor list item')
  }
  await userEvent.setup().click(within(last).getByText('Ranking details'))
  expect(within(last).getByRole('link', { name: 'synthetic-top-model-13' })).toBeVisible()
  expect(within(last).getByText('14')).toBeVisible()
  await waitFor(() => expect(screen.getByRole('button', { name: 'Show fewer vendors' })).toHaveAttribute('aria-expanded', 'true'))
})
