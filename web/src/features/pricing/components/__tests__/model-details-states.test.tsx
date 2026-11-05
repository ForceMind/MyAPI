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
import { DEFAULT_CURRENCY_CONFIG, useSystemConfigStore } from '@/stores/system-config-store'

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
  useSystemConfigStore.getState().setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
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

it.each([undefined, null, Number.NaN, Number.POSITIVE_INFINITY])('detail keeps model facts and marks unknown per-request price %s in the base and group price cells', async (price) => {
  pricingRequest = async () => ({ data: {
    ...populatedPricing,
    data: [{ ...populatedPricing.data[0], quota_type: 1, model_price: price, enable_groups: ['default'] }],
    group_ratio: { default: 2 },
    usable_group: { default: { desc: 'Synthetic group', ratio: 2 } },
  } })
  await renderDetails()
  expect(await screen.findByRole('heading', { name: 'synthetic-text-model' })).toBeVisible()
  expect(screen.getByRole('button', { name: 'Copy model name' })).toBeVisible()
  const base = screen.getByRole('heading', { name: 'Base Price' }).closest('section')
  const groups = screen.getByRole('heading', { name: 'Pricing by Group' }).closest('section')
  if (!base || !groups) throw new Error('Expected base and group pricing sections')
  expect(within(base).getByText('Unknown')).toBeVisible()
  expect(within(groups).getByText('Unknown')).toBeVisible()
  expect(base).not.toHaveTextContent('$0')
  expect(groups).not.toHaveTextContent('$0')
})

it('a zero group ratio keeps its 0x label and original zero computed price while the base price stays nonzero', async () => {
  pricingRequest = async () => ({ data: {
    ...populatedPricing,
    data: [{ ...populatedPricing.data[0], quota_type: 1, model_price: 0.25, enable_groups: ['free', 'default'] }],
    group_ratio: { free: 0, default: 2 },
    usable_group: { free: { desc: 'Synthetic free group', ratio: 0 }, default: { desc: 'Synthetic paid group', ratio: 2 } },
  } })
  await renderDetails()
  await screen.findByRole('heading', { name: 'synthetic-text-model' })
  const groups = screen.getByRole('heading', { name: 'Pricing by Group' }).closest('section')
  const base = screen.getByRole('heading', { name: 'Base Price' }).closest('section')
  if (!base || !groups) throw new Error('Expected base and group pricing sections')
  const zeroRatio = within(groups).getByText('0x')
  const freeRow = zeroRatio.closest('tr')
  if (!freeRow) throw new Error('Expected free group pricing row')
  expect(within(freeRow).getByText('$0')).toBeVisible()
  expect(within(base).getByText('$0.25')).toBeVisible()
})

it('dynamic group presentation preserves a configured zero ratio alongside a nonzero group without changing the expression', async () => {
  const expression = 'tier("base", p * 2.5 + c * 15)'
  pricingRequest = async () => ({ data: {
    ...populatedPricing,
    data: [{ ...populatedPricing.data[0], billing_mode: 'tiered_expr', billing_expr: expression, enable_groups: ['free', 'default'] }],
    group_ratio: { free: 0, default: 2 },
    usable_group: { free: { desc: 'Synthetic free group', ratio: 0 }, default: { desc: 'Synthetic paid group', ratio: 2 } },
  } })
  const { router } = await renderDetails()
  await screen.findByRole('heading', { name: 'synthetic-text-model' })
  const groups = screen.getByRole('heading', { name: 'Pricing by Group' }).closest('section')
  if (!groups) throw new Error('Expected dynamic group prices')
  expect(router.state.location.search.tokenUnit).toBe('K')
  expect(within(groups).getByText('Prices shown per 1K tokens')).toBeVisible()
  const free = within(groups).getByRole('group', { name: 'Pricing for group free' })
  const paid = within(groups).getByRole('group', { name: 'Pricing for group default' })
  expect(within(free).getByText('0x')).toBeVisible()
  expect(within(paid).getByText('2x')).toBeVisible()
  expect(within(free).getAllByText('$0')).toHaveLength(2)
  // Expression coefficients are USD/1M tokens; this route displays USD/1K.
  expect(within(paid).getByText('$0.005')).toBeVisible()
  expect(within(paid).getByText('$0.03')).toBeVisible()
  expect(groups).not.toHaveTextContent('1x')
})

it.each([{ groups: [] }, { groups: ['saved-group'] }])('API rate limits with configured groups $groups expose unknown values without generating limits or inventing a default group', async ({ groups }) => {
  pricingRequest = async () => ({ data: {
    ...populatedPricing,
    data: [{ ...populatedPricing.data[0], enable_groups: groups }],
  } })
  await renderDetails()
  await screen.findByRole('heading', { name: 'synthetic-text-model' })
  const reads = vi.mocked(api.get).mock.calls.map(([url]) => url)
  await userEvent.setup().click(screen.getByRole('tab', { name: 'API' }))
  const section = (await screen.findByRole('heading', { name: 'Rate limits' })).closest('section')
  if (!section) throw new Error('Expected rate limit information')
  expect(within(section).getByText('This catalog does not provide actual rate-limit data.')).toBeVisible()
  if (groups.length > 0) {
    expect(within(section).getByText('saved-group')).toBeVisible()
    expect(within(section).getByRole('columnheader', { name: 'RPM' })).toBeVisible()
    expect(within(section).getByRole('columnheader', { name: 'TPM' })).toBeVisible()
    expect(within(section).getByRole('columnheader', { name: 'RPD' })).toBeVisible()
    expect(within(section).getAllByText('Unknown')).toHaveLength(3)
  } else {
    expect(within(section).getByText('Unknown')).toBeVisible()
    expect(within(section).queryByRole('table')).not.toBeInTheDocument()
    expect(within(section).queryByText('default')).not.toBeInTheDocument()
  }
  expect(section.textContent).not.toMatch(/\d/)
  expect(vi.mocked(api.get).mock.calls.map(([url]) => url)).toEqual(reads)
})

it('API examples use the same normalized installation address as Key delivery without a duplicated version path', async () => {
  const post = vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected model request from a saved example'))
  pricingRequest = async () => ({ data: {
    ...populatedPricing,
    data: [{ ...populatedPricing.data[0], supported_endpoint_types: ['openai'] }],
    supported_endpoint: { openai: { path: '/v1/chat/completions', method: 'POST' } },
  } })
  const { client } = await renderDetails()
  await screen.findByRole('heading', { name: 'synthetic-text-model' })
  act(() => { client.setQueryData(['status'], { server_address: 'https://gateway.invalid/root/v1///' }) })
  const user = userEvent.setup()
  await user.click(screen.getByRole('tab', { name: 'API' }))
  const section = (await screen.findByRole('heading', { name: 'Code samples' })).closest('section')
  if (!section) throw new Error('Expected model client examples')
  await waitFor(() => expect(section).toHaveTextContent('curl https://gateway.invalid/root/v1/chat/completions'))
  expect(section).not.toHaveTextContent('/v1/v1')
  await user.click(within(section).getByRole('tab', { name: 'Python' }))
  await waitFor(() => expect(section).toHaveTextContent('base_url="https://gateway.invalid/root/v1"'))
  expect(section).not.toHaveTextContent('/v1/v1')
  expect(post).not.toHaveBeenCalled()
})
