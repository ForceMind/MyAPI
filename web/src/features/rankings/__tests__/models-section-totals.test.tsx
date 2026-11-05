import { QueryClient } from '@tanstack/react-query'
import { cleanup, screen, within } from '@testing-library/react'
import i18next from 'i18next'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { TooltipProvider } from '@/components/ui/tooltip'

import { ModelsSection } from '../components/models-section'
import type { ModelHistorySeries, ModelRanking } from '../types'

const tooltip = vi.hoisted(() => ({ values: [] as Array<{ key: string; value: unknown }> }))
vi.mock('@/lib/use-chart-theme', () => ({ useChartTheme: () => ({ resolvedTheme: 'light', themeReady: true }) }))
// Only the canvas boundary is replaced; the actual tooltip text formatter is used.
vi.mock('@visactor/react-vchart', () => ({ VChart: (props: { spec: {
  tooltip: { dimension: { updateContent: (values: Array<{ key: string; value: unknown }>) => Array<{ key: string; value: string }> } }
} }) => <ul aria-label='Saved chart tooltip'>{props.spec.tooltip.dimension.updateContent([...tooltip.values]).map((item) => <li key={item.key}>{item.key}: {item.value}</li>)}</ul> }))

const clients: QueryClient[] = []
let originalNamespaceSeparator: typeof i18next.options.nsSeparator
beforeEach(() => {
  tooltip.values = []
  originalNamespaceSeparator = i18next.options.nsSeparator
  // Match the production i18n config: English source keys can contain literal colons.
  i18next.options.nsSeparator = false
})
afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  i18next.options.nsSeparator = originalNamespaceSeparator
  vi.restoreAllMocks()
})

async function renderModels(counts: unknown[]) {
  const rows: ModelRanking[] = counts.map((count, index) => ({ rank: index + 1, model_name: `Synthetic model ${index + 1}`, vendor: 'Synthetic vendor', category: 'all', total_tokens: count as number, share: 0, growth_pct: 0 }))
  const first = rows[0]
  if (!first) throw new Error('Expected at least one explicit ranked model fixture')
  const history: ModelHistorySeries = { buckets: 1, models: [{ name: first.model_name, vendor: 'Synthetic vendor', total: 0 }], points: [{ ts: '2026-11-06T00:00:00Z', label: 'Saved bucket', model: first.model_name, vendor: 'Synthetic vendor', tokens: 0 }] }
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  clients.push(client)
  await renderListRoute({ client, path: '/rankings/', element: <TooltipProvider><ModelsSection rows={rows} history={history} period='month' /></TooltipProvider> })
  const header = screen.getByRole('heading', { name: 'Top Models' }).closest('header')
  if (!header) throw new Error('Expected model token summary header')
  return within(header)
}

it.each([
  { name: 'missing', count: undefined, expected: 'Unknown' },
  { name: 'null', count: null, expected: 'Unknown' },
  { name: 'NaN', count: Number.NaN, expected: 'Unknown' },
  { name: 'negative', count: -1, expected: 'Unknown' },
  { name: 'zero', count: 0, expected: '0' },
])('model total keeps $name distinct from a measured zero', async ({ count, expected }) => {
  const summary = await renderModels([count])
  expect(summary.getByText(expected)).toBeVisible()
  if (expected === 'Unknown') {
    expect(summary.queryByText('0')).not.toBeInTheDocument()
    const row = screen.getByRole('link', { name: 'Synthetic model 1' }).closest('li')
    if (!row) throw new Error('Expected original ranked row')
    expect(within(row).getByText('Unknown')).toBeVisible()
  }
})

it.each([
  { counts: [7, null], expected: 'Unknown' },
  { counts: [7, 5], expected: '12' },
  { counts: [Number.MAX_SAFE_INTEGER, 1], expected: 'Unknown' },
])('a mixed total $counts requires every count and a safe sum', async ({ counts, expected }) => {
  const summary = await renderModels(counts)
  expect(summary.getByText(expected)).toBeVisible()
  expect(screen.getAllByRole('link', { name: /^Synthetic model/ }).map((link) => link.textContent)).toEqual(['Synthetic model 1', 'Synthetic model 2'])
})

it('the collapsed tooltip remainder cannot turn an unknown count into a known subtotal', async () => {
  tooltip.values = [...Array.from({ length: 10 }, (_, index) => ({ key: `Known ${index}`, value: 1 })), { key: 'Zero model', value: 0 }, { key: 'Unknown model', value: null }]
  await renderModels([1, 0])
  const output = screen.getByRole('list', { name: 'Saved chart tooltip' })
  expect(within(output).getByText('Total:: Unknown')).toBeVisible()
  expect(within(output).getByText('+2 more: Unknown')).toBeVisible()
})

it('retains numeric text supplied by the chart callback while treating empty text as unknown', async () => {
  tooltip.values = [{ key: 'Zero text', value: '0' }, { key: 'Empty text', value: '' }]
  await renderModels([0])
  const output = screen.getByRole('list', { name: 'Saved chart tooltip' })
  expect(within(output).getByText('Zero text: 0')).toBeVisible()
  expect(within(output).getByText('Empty text: Unknown')).toBeVisible()
  expect(within(output).getByText('Total:: Unknown')).toBeVisible()
})

it.each([undefined, null, Number.NaN, -1, 0])('tooltip totals preserve unknown count %s without hiding a known row', async (value) => {
  tooltip.values = [{ key: 'Known model', value: 7 }, { key: 'Other model', value }]
  await renderModels([7, 0])
  const output = screen.getByRole('list', { name: 'Saved chart tooltip' })
  expect(within(output).getByText('Known model: 7')).toBeVisible()
  expect(within(output).getByText(value === 0 ? 'Total:: 7' : 'Total:: Unknown')).toBeVisible()
  expect(within(output).getByText(value === 0 ? 'Other model: 0' : 'Other model: Unknown')).toBeVisible()
})
