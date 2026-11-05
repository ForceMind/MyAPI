import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'
import { api } from '@/lib/api'

import { CONNECTION, mountConnections } from './connections-page-fixture'

vi.mock('@/hooks/use-sidebar-config', () => ({ useIsSidebarModuleVisible: () => true }))

const originalAuth = useAuthStore.getState()
const originalAdapter = api.defaults.adapter
const originalMatchMedia = Object.getOwnPropertyDescriptor(window, 'matchMedia')
let dispose: (() => void) | undefined
afterEach(() => {
  dispose?.()
  dispose = undefined
  api.defaults.adapter = originalAdapter
  if (originalMatchMedia) Object.defineProperty(window, 'matchMedia', originalMatchMedia)
  else Reflect.deleteProperty(window, 'matchMedia')
  useAuthStore.setState(originalAuth)
})

test('a card defaults to model and group counts and keyboard expansion keeps all values without navigation or network work', async () => {
  const model = 'synthetic/model-with-an-extremely-long-complete-identifier'
  const fixture = await mountConnections({ items: [{ ...CONNECTION, priority: null, weight: 0, group: 'default,premium', models: `alpha,${model}` }] })
  dispose = fixture.teardown
  const card = within(await screen.findByRole('article', { name: CONNECTION.name }))
  const models = card.getByRole('button', { name: 'Models (2)' })
  const groups = card.getByRole('button', { name: 'Groups (2)' })
  expect(models).toHaveAttribute('aria-expanded', 'false')
  expect(card.getByText(`alpha, ${model}`)).not.toBeVisible()
  expect(card.getByText('default, premium')).not.toBeVisible()
  expect(card.getByText('Priority').parentElement).toHaveTextContent('Unknown')
  expect(card.getByText('Weight').parentElement).toHaveTextContent('0')
  await waitFor(() => expect(card.getByText('No saved window sample')).toBeVisible())
  const before = fixture.requests.length
  const user = userEvent.setup()
  models.focus()
  await user.keyboard('{Enter}')
  expect(card.getByText(`alpha, ${model}`)).toBeVisible()
  await user.click(groups)
  expect(card.getByText('default, premium')).toBeVisible()
  models.focus()
  await user.keyboard(' ')
  expect(card.getByText(`alpha, ${model}`)).not.toBeVisible()
  expect(models).toHaveFocus()
  expect(fixture.router.state.location.pathname).toBe('/connections')
  expect(fixture.requests.slice(before)).toEqual([])
  expect(screen.getByTestId('connection-grid')).toHaveClass('items-start')
})

test('current quota is visible while historical details retain each observation and original uncertainty after expansion', async () => {
  const now = Math.floor(Date.now() / 1000)
  const saved = { channel_id: 7, name: CONNECTION.name, metric_type: 'codex_rate_limit', unit: 'percent', window_type: '5h', window_seconds: 18000, status: 'success', source: 'codex_wham_usage_primary', current_available: 18, current_used: 82, observed_at: now - 5, reset_at: now + 3600, series_id: 'a'.repeat(64) }
  const fixture = await mountConnections({ windows: [saved, { ...saved, source: 'codex_wham_usage', status: 'error', current_available: null, current_used: null, observed_at: now - 100, series_id: 'b'.repeat(64) }] })
  dispose = fixture.teardown
  const card = within(await screen.findByRole('article', { name: CONNECTION.name }))
  const details = await card.findByRole('button', { name: 'Window details (2), historical records (1)' })
  const current = card.getByRole('list', { name: 'Current account windows' })
  expect(within(current).getByText(/18% left/)).toBeVisible()
  expect(card.getByText('Used')).toBeVisible()
  const history = card.getByText('A newer WHAM observation exists for this connection. This saved error may belong to a different identity and is not marked resolved.')
  expect(history).not.toBeVisible()
  const before = fixture.requests.length
  await userEvent.setup().click(details)
  expect(history).toBeVisible()
  const full = card.getByRole('list', { name: 'Saved account windows' })
  expect(within(full).getAllByRole('listitem')).toHaveLength(2)
  expect(full.textContent?.match(/Observed /g)).toHaveLength(2)
  expect(card.getByRole('img', { name: /82% used, 18% left/ })).toBeVisible()
  expect(fixture.requests.slice(before)).toEqual([])
})

test('a current failed observation stays visible and cannot turn its old numeric value into a quota ring', async () => {
  const fixture = await mountConnections({ windows: [{ channel_id: 7, name: CONNECTION.name, metric_type: 'codex_rate_limit', unit: 'percent', window_type: '5h', status: 'error', source: 'codex_wham_usage', current_available: 45, observed_at: Math.floor(Date.now() / 1000), series_id: 'c'.repeat(64) }] })
  dispose = fixture.teardown
  const card = within(await screen.findByRole('article', { name: CONNECTION.name }))
  await waitFor(() => expect(within(card.getByRole('list', { name: 'Current account windows' })).getByText(/WHAM usage sample failed/)).toBeVisible())
  expect(card.queryByRole('img')).not.toBeInTheDocument()
  expect(card.queryByText(/55%|100% left/)).not.toBeInTheDocument()
  const before = fixture.requests.length
  await userEvent.setup().click(card.getByRole('button', { name: 'Window details (1)' }))
  expect(card.getByText('This saved WHAM observation failed. It does not describe other window series.')).toBeVisible()
  expect(fixture.requests.slice(before)).toEqual([])
})

test('a healthy different account window never hides the failed weekly series while details are collapsed', async () => {
  const now = Math.floor(Date.now() / 1000)
  const fixture = await mountConnections({ windows: [
    { channel_id: 7, name: CONNECTION.name, metric_type: 'codex_rate_limit', unit: 'percent', window_type: 'weekly', window_seconds: 604800, status: 'error', source: 'codex_wham_usage_secondary', current_available: null, observed_at: now - 20, account_label: 'Synthetic account A', account_ref: 'account-a', series_id: 'd'.repeat(64) },
    { channel_id: 7, name: CONNECTION.name, metric_type: 'codex_rate_limit', unit: 'percent', window_type: 'five_hour', window_seconds: 18000, status: 'success', source: 'codex_wham_usage_primary', current_available: 82, observed_at: now - 5, account_label: 'Synthetic account B', account_ref: 'account-b', series_id: 'e'.repeat(64) },
  ] })
  dispose = fixture.teardown
  const card = within(await screen.findByRole('article', { name: CONNECTION.name }))
  const current = await card.findByRole('list', { name: 'Current account windows' })
  expect(within(current).getByText(/7 days.*Saved sample error; recovery unconfirmed/)).toBeVisible()
  expect(within(current).getByText(/82% left/)).toBeVisible()
  const toggle = card.getByRole('button', { name: 'Window details (2), historical records (1)' })
  expect(toggle).toHaveAttribute('aria-expanded', 'false')
  const before = fixture.requests.length
  await userEvent.setup().click(toggle)
  const evidence = card.getByRole('list', { name: 'Saved account windows' })
  expect(within(evidence).getAllByRole('listitem')).toHaveLength(2)
  expect(within(evidence).getByText(/7 days.*Historical saved sample error/)).toBeVisible()
  expect(within(evidence).getByText(/not marked resolved/)).toBeVisible()
  expect(fixture.requests.slice(before)).toEqual([])
})

test.each([4, 1, 57])('empty saved records for type %i keep query capability unknown', async (type) => {
  const fixture = await mountConnections({ items: [{ ...CONNECTION, type }] })
  dispose = fixture.teardown
  const card = within(await screen.findByRole('article', { name: CONNECTION.name }))
  await waitFor(() => expect(card.getByText('No saved window sample')).toBeVisible())
  expect(card.getByText('Window query capability unknown')).toBeVisible()
  expect(card.queryByText('Metered, no subscription window')).not.toBeInTheDocument()
  expect(card.queryByText('Local server, no quota window')).not.toBeInTheDocument()
  expect(card.queryByRole('img')).not.toBeInTheDocument()
})

test('expanded card evidence retains account identity and source for each independently saved series', async () => {
  const now = Math.floor(Date.now() / 1000)
  const fixture = await mountConnections({ windows: [
    { channel_id: 7, name: CONNECTION.name, metric_type: 'codex_rate_limit', unit: 'percent', window_type: 'weekly', status: 'unsupported', source: 'codex_wham_usage_secondary', account_label: 'Synthetic weekly account', observed_at: now - 20, series_id: 'f'.repeat(64) },
    { channel_id: 7, name: CONNECTION.name, metric_type: 'codex_rate_limit', unit: 'percent', window_type: 'five_hour', status: 'success', source: 'codex_wham_usage_primary', account_label: 'Synthetic primary account', current_available: 82, observed_at: now - 5, series_id: 'a'.repeat(64) },
  ] })
  dispose = fixture.teardown
  const card = within(await screen.findByRole('article', { name: CONNECTION.name }))
  const current = await card.findByRole('list', { name: 'Current account windows' })
  expect(within(current).getByText(/Window reporting unsupported/)).toBeVisible()
  const before = fixture.requests.length
  await userEvent.setup().click(card.getByRole('button', { name: 'Window details (2)' }))
  const rows = within(card.getByRole('list', { name: 'Saved account windows' })).getAllByRole('listitem')
  expect(within(rows[0]).getByText('Synthetic weekly account')).toBeVisible()
  expect(within(rows[0]).getByText(/codex_wham_usage_secondary/)).toBeVisible()
  expect(within(rows[1]).getByText('Synthetic primary account')).toBeVisible()
  expect(within(rows[1]).getByText(/codex_wham_usage_primary/)).toBeVisible()
  expect(fixture.requests.slice(before)).toEqual([])
})
