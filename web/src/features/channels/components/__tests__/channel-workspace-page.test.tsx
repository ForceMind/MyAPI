import { QueryClient } from '@tanstack/react-query'
import { act, cleanup, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { Channels } from '../../index'
import { channelSchema } from '../../types'

vi.mock('@/hooks/use-sidebar-config', () => ({ useIsSidebarModuleVisible: () => true }))

const originalAuth = useAuthStore.getState()
const item = channelSchema.parse({ id: 23, type: 1, name: 'needle-channel', key: 'masked', status: 1, created_time: 1, test_time: 0, response_time: 0, balance_updated_time: 0, models: 'alpha', group: 'default', priority: 9, weight: 25 })
const observer = { channel_id: 23, name: item.name, series_id: 'a'.repeat(64), account_label: 'Synthetic account window', metric_type: 'codex_rate_limit', source: 'codex_wham_usage_primary', window_type: 'five_hour', window_seconds: 18000, unit: 'percent', status: 'success', current_available: 82, observed_at: 1_793_000_000 }
let client: QueryClient
let failEvidence: boolean
let ignoreRequestedChannel: boolean
let evidence: typeof observer[]
let requests: { path: string; params: Record<string, unknown> }[]

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('channels:view-mode', 'table')
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  failEvidence = false
  ignoreRequestedChannel = false
  evidence = []
  requests = []
  useAuthStore.getState().auth.setBundle({ access_token: 'synthetic-workspace-access', token_type: 'Bearer', access_expires_at: 4_000_000_000, user: { id: 1, role: 100, username: 'synthetic-root' }, session: { sid: 'channel-workspace-session', current: true, login_method: 'password', ip: '192.0.2.1', user_agent: 'Synthetic fixture', created_at: 1, last_active_at: 1, expires_at: 4_000_000_000 } })
  vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
    const path = url.split('?')[0]
    const params = (config?.params ?? {}) as Record<string, unknown>
    requests.push({ path, params })
    if (path === '/api/status') return { data: { success: true, data: {} } }
    if (path === '/api/user/2fa/status' || path === '/api/user/passkey') return { data: { success: true, data: { enabled: false } } }
    if (path === '/api/user/self') return { data: { success: true, data: useAuthStore.getState().auth.user } }
    if (path === '/api/channel/ops') return { data: { success: true, data: { retry_times: 3 } } }
    if (path === '/api/group/') return { data: { success: true, data: ['default'] } }
    if (path === '/api/channel' || path === '/api/channel/search') return { data: { success: true, data: { items: [item], total: 220, type_counts: { 1: 220 } } } }
    if (path === '/api/channel/quota/changes') {
      if (failEvidence) throw new Error('Synthetic saved evidence read failed')
      const items = params.channel_ids === '999' && !ignoreRequestedChannel ? [] : evidence
      return { data: { success: true, data: { items, source_complete: true, items_complete: true } } }
    }
    if (path === '/api/channel/quota/status') return { data: { success: true, data: { enabled: false, interval_seconds: 60, max_channels: 100 } } }
    if (/^\/api\/channel\/\d+\/quota\/history$/.test(path)) return { data: { success: true, data: { channel_id: 23, series_id: observer.series_id, start: 100, end: 200, limit: 5000, points: [] } } }
    if (path === '/api/channel/quota/events') return { data: { success: true, data: { items: [], total: 0, page: 1, page_size: 10 } } }
    if (path === '/api/channel/routing-preview') return { data: { success: true, data: { group: String(params.group), model: String(params.model), request_path: params.request_path, source: 'database', generation: 1, data_generation: 1, published_generation: 1, cluster_committed_epoch: 1, local_published_epoch: 1, cache_enabled: false, cache_pending: false, tiers: [], rejected: [] } } }
    throw new Error(`Unexpected workspace GET ${path}`)
  })
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected workspace POST'))
  vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected workspace PUT'))
  vi.spyOn(api, 'delete').mockRejectedValue(new Error('Unexpected workspace DELETE'))
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); localStorage.clear(); vi.restoreAllMocks() })

async function page(entry = '/channels/#channel-inventory') {
  return renderListRoute({ client, path: '/channels/', entry, element: <Channels /> })
}
function heavyReads() {
  return requests.filter((request) => /quota\/changes|quota\/status|quota\/history|quota\/events|routing-preview/.test(request.path))
}

test('inventory does not read heavy evidence and returning restores the complete search, scroll and origin focus', async () => {
  const params = new URLSearchParams({ filter: 'needle', model: 'alpha', status: JSON.stringify(['enabled']), type: JSON.stringify(['1']), group: JSON.stringify(['default']), page: '2', pageSize: '50', quotaChannelId: '23' })
  const rendered = await page(`/channels/?${params}#channel-inventory`)
  await screen.findByText(item.name)
  expect(heavyReads()).toEqual([])
  const inventory = screen.getByRole('region', { name: 'Channel inventory' })
  const scroller = inventory.closest<HTMLElement>('[data-page-scroll]')
  if (!scroller) throw new Error('Expected the inventory page scroll container')
  scroller.scrollTop = 321
  const nav = screen.getByRole('navigation', { name: 'Channel workspace' })
  const entry = within(nav).getByRole('link', { name: 'Quota evidence' })
  const before = new URLSearchParams(rendered.router.state.location.searchStr)
  const user = userEvent.setup()
  await user.click(entry)
  await waitFor(() => expect(requests.some((request) => request.path === '/api/channel/quota/changes')).toBe(true))
  expect(scroller.querySelector('#channel-inventory')).not.toBeVisible()
  expect(screen.queryByRole('navigation', { name: 'Connections' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Back to inventory' }))
  await waitFor(() => expect(inventory).toBeVisible())
  await waitFor(() => expect(rendered.router.state.location.hash).toBe('channel-inventory'))
  expect([...new URLSearchParams(rendered.router.state.location.searchStr).entries()]).toEqual([...before.entries()])
  expect(scroller.scrollTop).toBe(321)
  expect(entry).toHaveFocus()
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
  expect(api.delete).not.toHaveBeenCalled()
})

test('a deep link for a missing channel never selects another channel or starts its history read', async () => {
  evidence = [observer]
  ignoreRequestedChannel = true
  await page('/channels/?quotaChannelId=999#channel-evidence')
  await screen.findByText('No saved evidence for the requested channel in this range.')
  expect(requests.find((request) => request.path === '/api/channel/quota/changes')?.params.channel_ids).toBe('999')
  expect(requests.some((request) => request.path === '/api/channel/23/quota/history')).toBe(false)
  expect(requests.some((request) => request.path === '/api/channel')).toBe(false)
  expect(screen.queryByRole('button', { name: /Synthetic account window/ })).not.toBeInTheDocument()
  expect(within(screen.getByRole('region', { name: 'Quota evidence' })).queryByText(observer.account_label)).not.toBeInTheDocument()
})

test('routing hash preserves the original draft and only an explicit submit reads a snapshot', async () => {
  await page('/channels/#channel-routing')
  const task = screen.getByRole('region', { name: 'Routing Preview' })
  const user = userEvent.setup()
  const input = within(task).getByLabelText('Routing preview model')
  await user.type(input, 'synthetic-model')
  expect(heavyReads()).toEqual([])
  await user.click(screen.getByRole('button', { name: 'Back to inventory' }))
  await waitFor(() => expect(screen.getByRole('region', { name: 'Channel inventory' })).toHaveFocus())
  const nav = screen.getByRole('navigation', { name: 'Channel workspace' })
  await user.click(within(nav).getByRole('link', { name: 'Routing Preview' }))
  expect(within(task).getByLabelText('Routing preview model')).toHaveValue('synthetic-model')
  expect(requests.some((request) => request.path === '/api/channel/routing-preview')).toBe(false)
  await user.click(within(task).getByRole('button', { name: 'Preview routing' }))
  await waitFor(() => expect(requests.filter((request) => request.path === '/api/channel/routing-preview')).toHaveLength(1))
  const before = requests.filter((request) => request.path === '/api/channel/routing-preview').length
  await user.click(screen.getByRole('button', { name: 'Back to inventory' }))
  await user.click(within(nav).getByRole('link', { name: 'Routing Preview' }))
  expect(requests.filter((request) => request.path === '/api/channel/routing-preview')).toHaveLength(before)
})

test('a known task read failure remains visible outside the closed task without being presented as channel health', async () => {
  failEvidence = true
  await page()
  await screen.findByText(item.name)
  const nav = screen.getByRole('navigation', { name: 'Channel workspace' })
  const user = userEvent.setup()
  await user.click(within(nav).getByRole('link', { name: 'Quota evidence' }))
  await waitFor(() => expect(within(nav).getByRole('link', { name: 'Quota evidence' })).toHaveTextContent('Read failed'))
  await user.click(screen.getByRole('button', { name: 'Back to inventory' }))
  expect(within(nav).getByRole('link', { name: 'Quota evidence' })).toHaveTextContent('Read failed')
  expect(screen.getByRole('button', { name: 'Open task' })).toBeVisible()
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
  expect(api.delete).not.toHaveBeenCalled()
})

test('a same-user session change discards private task drafts and previous read failures', async () => {
  failEvidence = true
  await page('/channels/#channel-evidence')
  const nav = screen.getByRole('navigation', { name: 'Channel workspace' })
  await waitFor(() => expect(within(nav).getByRole('link', { name: 'Quota evidence' })).toHaveTextContent('Read failed'))
  await userEvent.setup().click(within(nav).getByRole('link', { name: 'Routing Preview' }))
  const task = screen.getByRole('region', { name: 'Routing Preview' })
  await userEvent.setup().type(within(task).getByLabelText('Routing preview model'), 'previous-session-private-draft')
  const auth = useAuthStore.getState().auth
  const user = auth.user
  const session = auth.session
  if (!session || !user) throw new Error('Expected a synthetic authenticated session')
  await act(async () => { useAuthStore.getState().auth.setBundle({ access_token: 'synthetic-replacement-access', token_type: 'Bearer', access_expires_at: 4_000_000_000, user, session: { ...session, sid: 'replacement-session' } }) })
  await waitFor(() => expect(within(screen.getByRole('region', { name: 'Routing Preview' })).getByLabelText('Routing preview model')).toHaveValue(''))
  expect(screen.queryByDisplayValue('previous-session-private-draft')).not.toBeInTheDocument()
  expect(within(screen.getByRole('navigation', { name: 'Channel workspace' })).getByRole('link', { name: 'Quota evidence' })).not.toHaveTextContent('Read failed')
  expect(requests.some((request) => request.path === '/api/channel/routing-preview')).toBe(false)
})

test('the advanced table shares the Connections identity and does not label unopened tasks as unloaded failures', async () => {
  await page()
  await screen.findByRole('link', { name: item.name })
  expect(screen.getByRole('heading', { level: 1, name: 'Connections' })).toBeVisible()
  const nav = screen.getByRole('navigation', { name: 'Channel workspace' })
  expect(within(nav).queryByText('Not loaded')).not.toBeInTheDocument()
  expect(within(nav).queryByText('Not submitted')).not.toBeInTheDocument()
  expect(screen.getByRole('table')).toBeVisible()
  expect(heavyReads()).toEqual([])
})
