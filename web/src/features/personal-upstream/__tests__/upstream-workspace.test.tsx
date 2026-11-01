import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AxiosAdapter } from 'axios'
import { afterEach, expect, test, vi } from 'vitest'

import { mountConnections } from '@/features/connections/__tests__/connections-page-fixture'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useInspectorStore } from '@/stores/inspector-store'

vi.mock('@/hooks/use-sidebar-config', () => ({ useIsSidebarModuleVisible: () => true }))
const originalAdapter = api.defaults.adapter
const originalMedia = Object.getOwnPropertyDescriptor(window, 'matchMedia')
let teardown: (() => void) | undefined
const now = Math.floor(Date.now() / 1000)
const sample = { channel_id: 7, name: 'Synthetic account', metric_type: 'codex_rate_limit', unit: 'percent', source: 'codex_wham_usage_primary', window_type: 'five_hour', window_seconds: 18000, status: 'success', current_available: 0, observed_at: now - 20, series_id: 'a'.repeat(64) }

afterEach(() => {
  teardown?.()
  teardown = undefined
  api.defaults.adapter = originalAdapter
  if (originalMedia) Object.defineProperty(window, 'matchMedia', originalMedia)
  useAuthStore.getState().auth.reset('idle')
  useInspectorStore.setState({ current: null })
  vi.restoreAllMocks()
})
async function mount(options: Parameters<typeof mountConnections>[0]) {
  const fixture = await mountConnections(options)
  teardown = fixture.teardown
  return fixture
}

test('overview filters saved observations without another read and retains historical error context outside the filter', async () => {
  const fixture = await mount({ entry: '/upstream-accounts', windows: [sample, { ...sample, series_id: 'b'.repeat(64), status: 'error', source: 'codex_wham_usage', observed_at: now - 120, current_available: null, window_type: 'weekly', window_seconds: 604800 }, { ...sample, series_id: 'c'.repeat(64), channel_id: 8, name: 'Other account', current_available: 50 }] })
  await screen.findByRole('heading', { name: 'Synthetic account' })
  expect(screen.getAllByTestId('native-window-card')).toHaveLength(2)
  expect(screen.getByText('Statistics and filters cover returned observations only.')).toBeVisible()
  expect(screen.getByText('Returned connections').closest('div')).toHaveTextContent('2')
  const reads = fixture.requests.filter((request) => request.path === '/api/channel/quota/changes').length
  await userEvent.setup().selectOptions(screen.getByRole('combobox', { name: 'Snapshot state' }), 'error')
  await waitFor(() => expect(screen.getAllByTestId('native-window-card')).toHaveLength(1))
  expect(screen.getByText('Historical saved sample error')).toBeVisible()
  expect(screen.getByText(/not marked resolved/)).toBeVisible()
  expect(fixture.requests.filter((request) => request.path === '/api/channel/quota/changes')).toHaveLength(reads)
  expect(fixture.requests.some((request) => request.path.includes('/test/') || request.path.includes('/codex/usage'))).toBe(false)
})

test('an account window opens its connection and Back restores filters, account and focus', async () => {
  const fixture = await mount({ entry: '/upstream-accounts?channelId=7&filter=Synthetic&period=duration%3A18000', windows: [sample] })
  await screen.findByRole('heading', { name: 'Synthetic account' })
  const link = screen.getByText('Manage account connection').closest('a')
  expect(link).not.toBeNull()
  await userEvent.setup().click(link as HTMLAnchorElement)
  await screen.findByRole('heading', { level: 1, name: 'Synthetic connection' })
  expect(fixture.router.state.location.search).toMatchObject({ connectionReturn: { upstreamReturn: { channelId: 7, filter: 'Synthetic', period: 'duration:18000' } } })
  await act(async () => useInspectorStore.getState().current?.close())
  await waitFor(() => expect(fixture.router.state.location.pathname).toBe('/upstream-accounts'))
  expect(fixture.router.state.location.search).toMatchObject({ channelId: 7, filter: 'Synthetic', period: 'duration:18000' })
  const aside = await screen.findByRole('complementary', { name: 'Select upstream account' })
  await waitFor(() => expect(within(aside).getByRole('button', { name: 'Synthetic account' })).toHaveFocus())
  expect(fixture.requests.every((request) => request.method === 'get')).toBe(true)
})

test('a saved window failure reports unknown statistics and never an empty successful count', async () => {
  await mount({ entry: '/upstream-accounts', windowsFail: true })
  expect(await screen.findByText('Unable to load upstream account windows')).toBeVisible()
  expect(screen.getByText('Returned connections').closest('div')).toHaveTextContent('Unknown')
  expect(screen.queryByText('No upstream account windows were reported')).not.toBeInTheDocument()
})

test('a direct connection return restores window context after refresh without testing a provider', async () => {
  const context = encodeURIComponent(JSON.stringify({ upstreamReturn: { channelId: 7, filter: 'Synthetic' } }))
  const fixture = await mount({ entry: `/connections/7?connectionReturn=${context}`, windows: [sample] })
  await screen.findByRole('heading', { level: 1, name: 'Synthetic connection' })
  await act(async () => useInspectorStore.getState().current?.close())
  await waitFor(() => expect(fixture.router.state.location.pathname).toBe('/upstream-accounts'))
  expect(fixture.router.state.location.search).toMatchObject({ channelId: 7, filter: 'Synthetic', focusChannelId: 7 })
  expect(fixture.requests.some((request) => request.path.includes('/test/'))).toBe(false)
})


test('selecting the already active account preserves its confirmed Key and does not reread snapshots', async () => {
  const fixture = await mount({ entry: '/upstream-accounts?channelId=7&keyId=21', windows: [sample], extraReads: {
    '/pg/keys': { items: [{ id: 21, name: 'Selected Key', status: 1, group: 'default', remain_quota: 0, unlimited_quota: false, expired_time: -1 }], total: 1 },
    '/api/token/21/subscription-share': { token_id: 21, pools: [] },
  } })
  const key = await screen.findByRole('combobox', { name: 'Select an API Key' })
  await waitFor(() => expect(key).toHaveValue('21'))
  const reads = fixture.requests.filter((request) => request.path === '/api/channel/quota/changes').length
  const aside = screen.getByRole('complementary', { name: 'Select upstream account' })
  await userEvent.setup().click(within(aside).getByRole('button', { name: 'Synthetic account' }))
  expect(fixture.router.state.location.search).toMatchObject({ channelId: 7, keyId: 21 })
  expect(key).toHaveValue('21')
  expect(fixture.requests.filter((request) => request.path === '/api/channel/quota/changes')).toHaveLength(reads)
})

test('a connection read failure returns to the account inventory with its origin focus', async () => {
  const context = encodeURIComponent(JSON.stringify({ upstreamReturn: { channelId: 7, filter: 'Synthetic' } }))
  const fixture = await mount({ entry: `/connections/7?connectionReturn=${context}`, windows: [sample], failDetail: true })
  const back = await screen.findByRole('link', { name: 'Back to account windows' })
  await userEvent.setup().click(back)
  await waitFor(() => expect(fixture.router.state.location.pathname).toBe('/upstream-accounts'))
  const aside = await screen.findByRole('complementary', { name: 'Select upstream account' })
  await waitFor(() => expect(within(aside).getByRole('button', { name: 'Synthetic account' })).toHaveFocus())
})


test('a newly returned observation is classified using render time rather than the time the read started', async () => {
  const clock = vi.spyOn(Date, 'now').mockReturnValue(1000_000)
  await mount({ entry: '/upstream-accounts', windows: [{ ...sample, observed_at: 1000 }] })
  await screen.findByRole('heading', { name: 'Synthetic account' })
  const adapter = api.defaults.adapter as AxiosAdapter
  api.defaults.adapter = async (config) => {
    const response = await adapter(config)
    if (config.url === '/api/channel/quota/changes') {
      clock.mockReturnValue(1100_000)
      response.data = { success: true, data: { items: [{ ...sample, observed_at: 1100, source: 'new-saved-response' }] } }
    }
    return response
  }
  await userEvent.setup().click(screen.getByRole('button', { name: 'Refresh account windows' }))
  await within(screen.getByTestId('native-window-card')).findByText('new-saved-response')
  expect(within(screen.getByTestId('native-window-card')).getByText('Snapshot available')).toBeVisible()
  expect(within(screen.getByTestId('native-window-card')).queryByText('Sample time unverified')).not.toBeInTheDocument()
  expect(screen.getByText('Fresh window observations').closest('div')).toHaveTextContent('1')
})


test('explicitly choosing the initially displayed account anchors that selection in the return context', async () => {
  const fixture = await mount({ entry: '/upstream-accounts', windows: [sample] })
  const aside = await screen.findByRole('complementary', { name: 'Select upstream account' })
  await userEvent.setup().click(within(aside).getByRole('button', { name: 'Synthetic account' }))
  await waitFor(() => expect(fixture.router.state.location.search).toMatchObject({ channelId: 7 }))
})


test('different saved identities on one connection remain separate and the newest returned identity labels the picker', async () => {
  await mount({ entry: '/upstream-accounts', windows: [
    { ...sample, account_label: 'Current saved identity' },
    { ...sample, series_id: 'd'.repeat(64), account_label: 'Historical saved identity', source: 'codex_wham_usage', status: 'error', observed_at: now - 120, current_available: null },
  ] })
  const aside = await screen.findByRole('complementary', { name: 'Select upstream account' })
  expect(within(aside).getByRole('button', { name: 'Current saved identity' })).toHaveAttribute('aria-pressed', 'true')
  const cards = screen.getAllByTestId('native-window-card')
  expect(cards).toHaveLength(2)
  expect(within(cards[0]).getByText('Current saved identity')).toBeVisible()
  expect(within(cards[1]).getByText('Historical saved identity')).toBeVisible()
  expect(within(cards[1]).getByText('Historical saved sample error')).toBeVisible()
})
