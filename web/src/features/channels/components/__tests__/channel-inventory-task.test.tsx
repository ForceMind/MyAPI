import { QueryClient } from '@tanstack/react-query'
import { act, cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import type { ChannelTaskReadState } from '../../channel-task-state'
import { channelSchema, type ChannelQuotaChangeItem } from '../../types'
import { ChannelsPrimaryButtons } from '../channels-primary-buttons'
import { ChannelsProvider } from '../channels-provider'
import { ChannelsTable } from '../channels-table'

const originalAuth = useAuthStore.getState()
const originalConfig = useSystemConfigStore.getState()
const item = channelSchema.parse({ id: 23, type: 1, name: 'Synthetic inventory channel', key: '', status: 1, group: 'default', tag: 'synthetic-tag', priority: 0, weight: 0, created_time: 1, test_time: 1700000000, response_time: 250, balance_updated_time: 0, models: 'alpha,beta' })
const clients: QueryClient[] = []
let outcome: 'ready' | 'error'
let items: typeof item[]
let savedEvidence: ChannelQuotaChangeItem[]
let evidenceMetadata: Record<string, unknown>

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('channels:view-mode', 'table')
  outcome = 'ready'
  items = [item]
  savedEvidence = []
  evidenceMetadata = {}
  useAuthStore.getState().auth.setUser({ id: 1, role: 100, username: 'synthetic-root' })
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
    if (url === '/api/status') return { data: { success: true, data: {} } }
    if (url === '/api/group/') return { data: { success: true, data: ['default', 'other'] } }
    if (url === '/api/channel/quota/changes') return { data: { success: true, data: {
      items: savedEvidence,
      source_complete: true,
      items_complete: true,
      total_items: savedEvidence.length,
      returned_items: savedEvidence.length,
      ...evidenceMetadata,
    } } }
    if (url === '/api/channel' || url === '/api/channel/search') {
      if (outcome === 'error') throw new Error('Synthetic inventory read failure')
      return { data: { success: true, data: { items, total: config?.params?.tag_mode ? 1 : 100, type_counts: { 1: 100, 57: 1 } } } }
    }
    throw new Error(`Unexpected synthetic inventory GET ${url}`)
  })
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected synthetic inventory write'))
  vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected synthetic inventory write'))
  vi.spyOn(api, 'delete').mockRejectedValue(new Error('Unexpected synthetic inventory deletion'))
})
afterEach(() => {
  cleanup()
  clients.splice(0).forEach((value) => value.clear())
  useAuthStore.setState(originalAuth)
  useSystemConfigStore.setState(originalConfig)
  localStorage.clear()
  vi.useRealTimers()
  vi.restoreAllMocks()
})
async function mount(options: { entry?: string; task?: boolean; report?: (state: ChannelTaskReadState) => void } = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  clients.push(client)
  function Task() {
    const [active, setActive] = useState(false)
    return <>
      <button type='button' onClick={() => setActive(true)}>Open synthetic inventory</button>
      <button type='button' onClick={() => setActive(false)}>Close synthetic inventory</button>
      <ChannelsPrimaryButtons />
      <section hidden={!active}><ChannelsTable active={active} onReadStateChange={options.report} /></section>
    </>
  }
  const route = await renderListRoute({ client, path: '/channels/', entry: options.entry, element: <ChannelsProvider>{options.task ? <Task /> : <><ChannelsPrimaryButtons /><ChannelsTable /></>}</ChannelsProvider> })
  return { client, ...route }
}
function inventoryReads() {
  return vi.mocked(api.get).mock.calls.filter(([url]) => url === '/api/channel' || url === '/api/channel/search')
}

it('monitoring and routing profiles change only information while custom restores legacy column and width preferences', async () => {
  const legacy = { models: true, tag: true, priority: true, weight: true, id: true, group: true }
  localStorage.setItem('channels:column-visibility', JSON.stringify(legacy))
  localStorage.setItem('channels:column-sizing', JSON.stringify({ name: 333 }))
  const user = userEvent.setup()
  await mount()
  await screen.findByRole('link', { name: item.name })
  expect(screen.getAllByRole('columnheader')).toHaveLength(6)
  expect(screen.getByRole('columnheader', { name: /Last saved test/ })).toBeVisible()
  expect(screen.queryByRole('columnheader', { name: /Priority/ })).not.toBeInTheDocument()
  expect(localStorage.getItem('channels:column-visibility')).toBe(JSON.stringify(legacy))
  expect(localStorage.getItem('channels:column-sizing')).toBe(JSON.stringify({ name: 333 }))
  const reads = inventoryReads().length
  await user.selectOptions(screen.getByRole('combobox', { name: 'Inventory view' }), 'routing')
  expect(await screen.findByRole('columnheader', { name: /Priority/ })).toBeVisible()
  expect(screen.getByRole('columnheader', { name: /Weight/ })).toBeVisible()
  expect(screen.getByRole('columnheader', { name: /Models/ })).toBeVisible()
  expect(localStorage.getItem('channels:column-visibility')).toBe(JSON.stringify(legacy))
  await user.selectOptions(screen.getByRole('combobox', { name: 'Inventory view' }), 'custom')
  expect(await screen.findByRole('columnheader', { name: /^ID/ })).toBeVisible()
  expect(screen.getByRole('columnheader', { name: /Models/ })).toBeVisible()
  expect(JSON.parse(localStorage.getItem('channels:column-sizing') ?? '{}')).toMatchObject({ name: 333 })
  expect(inventoryReads()).toHaveLength(reads)
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
})
it('legacy joint filters retain their first-value contract and model composition while collapsed controls stay local', async () => {
  const user = userEvent.setup()
  const { router } = await mount({ entry: '/channels/?model=alpha&type=%5B%221%22%2C%2257%22%5D&group=%5B%22default%22%2C%22other%22%5D' })
  await screen.findByRole('link', { name: item.name })
  expect(screen.getByText(/All takes precedence/)).toBeVisible()
  const read = inventoryReads()[0]
  expect(read?.[1]?.params).toMatchObject({ model: 'alpha', type: 1, group: 'default' })
  const before = inventoryReads().length
  await user.click(screen.getByRole('button', { name: 'More filters (3)' }))
  const model = screen.getByRole('textbox', { name: 'Model' })
  expect(model).toHaveValue('alpha')
  expect(screen.getByRole('combobox', { name: 'Group' })).toHaveValue('default')
  expect(inventoryReads()).toHaveLength(before)
  fireEvent.compositionStart(model)
  fireEvent.change(model, { target: { value: '合成模型' } })
  expect(inventoryReads()).toHaveLength(before)
  fireEvent.compositionEnd(model)
  await waitFor(() => expect(inventoryReads().at(-1)?.[1]?.params).toMatchObject({ model: '合成模型', type: 1, group: 'default' }))
  expect(router.state.location.search).toMatchObject({ type: ['1', '57'], group: ['default', 'other'] })
  await user.selectOptions(screen.getByRole('combobox', { name: 'Group' }), 'other')
  await waitFor(() => expect(inventoryReads().at(-1)?.[1]?.params).toMatchObject({ model: '合成模型', type: 1, group: 'other' }))
  expect(router.state.location.search).toMatchObject({ type: ['1', '57'], group: ['other'] })
})
it('inactive inventory performs no list or group read and hides portaled pagination and bulk controls without losing selection', async () => {
  const report = vi.fn()
  const user = userEvent.setup()
  const { client } = await mount({ task: true, report })
  expect(inventoryReads()).toHaveLength(0)
  expect(vi.mocked(api.get).mock.calls.filter(([url]) => url === '/api/group/')).toHaveLength(0)
  expect(vi.mocked(api.get).mock.calls.filter(([url]) => url === '/api/channel/quota/changes')).toHaveLength(0)
  expect(screen.queryByRole('navigation', { name: 'Connections' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Open synthetic inventory' }))
  await screen.findByRole('link', { name: item.name })
  await user.click(screen.getByRole('button', { name: 'Select channels' }))
  await user.click(screen.getByRole('checkbox', { name: 'Select row' }))
  expect(screen.getByText('Selected channel IDs (1)')).toBeVisible()
  await waitFor(() => expect(report).toHaveBeenLastCalledWith(expect.objectContaining({ phase: 'ready', readAt: expect.any(Number) })))
  await user.click(screen.getByRole('button', { name: 'Close synthetic inventory' }))
  expect(screen.queryByRole('navigation', { name: 'Connections' })).not.toBeInTheDocument()
  expect(screen.queryByText('Selected channel IDs (1)')).not.toBeInTheDocument()
  const before = inventoryReads().length
  await act(async () => { await client.invalidateQueries({ queryKey: ['channels'] }); await client.invalidateQueries({ queryKey: ['groups'] }) })
  expect(inventoryReads()).toHaveLength(before)
  await user.click(screen.getByRole('button', { name: 'Open synthetic inventory' }))
  expect(await screen.findByText('Selected channel IDs (1)')).toBeVisible()
})

it.each(['table', 'card'])('the %s inventory batches saved evidence and keeps balance capability separate from actual failure', async (view) => {
  localStorage.setItem('channels:view-mode', view)
  if (view === 'card') {
    const original = window.matchMedia
    vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({ ...original(query), matches: query === '(max-width: 640px)' }))
  }
  items = [item, { ...item, id: 24, name: 'Synthetic failed balance' }]
  savedEvidence = [
    { channel_id: 23, name: item.name, metric_type: 'balance', source: 'channel_type_1', unit: 'usd', status: 'unsupported', observed_at: Math.floor(Date.now() / 1000), series_id: 'a'.repeat(64) },
    { channel_id: 24, name: 'Synthetic failed balance', metric_type: 'balance', source: 'channel_type_1', unit: 'usd', status: 'error', observed_at: Math.floor(Date.now() / 1000), series_id: 'b'.repeat(64) },
  ]
  await mount()
  await waitFor(() => expect(screen.getAllByText(/Balance query: Balance query unsupported/)[0]).toBeVisible())
  expect(screen.getAllByText(/Balance query: Balance sample failed/)[0]).toBeVisible()
  expect(screen.getAllByText('Window query capability unknown')).toHaveLength(2)
  const reads = vi.mocked(api.get).mock.calls.filter(([url]) => url === '/api/channel/quota/changes')
  expect(reads).toHaveLength(1)
  expect(reads[0]?.[1]?.params).toMatchObject({ channel_ids: '23,24' })
  expect(vi.mocked(api.get).mock.calls.some(([url]) => /test|update_balance|codex\/usage/.test(String(url)))).toBe(false)
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
})

it('changing the inventory actor discards saved evidence before reading that actor scope', async () => {
  savedEvidence = [{ channel_id: 23, name: item.name, metric_type: 'balance', source: 'channel_type_1', unit: 'usd', status: 'unsupported', series_id: 'a'.repeat(64) }]
  evidenceMetadata = { total_items: 101, items_complete: false }
  await mount()
  await waitFor(() => expect(screen.getAllByText(/Balance query: Balance query unsupported/)[0]).toBeVisible())
  expect(screen.getAllByText('Saved quota evidence is incomplete. Missing series and earlier failures may be omitted.')[0]).toBeVisible()
  savedEvidence = []
  evidenceMetadata = {}
  act(() => useAuthStore.getState().auth.setUser({ id: 2, role: 100, username: 'synthetic-new-actor' }))
  expect(screen.queryAllByText(/Balance query: Balance query unsupported/)).toHaveLength(0)
  expect(await screen.findByText('No saved window sample')).toBeVisible()
  expect(screen.queryByText('Saved quota evidence is incomplete. Missing series and earlier failures may be omitted.')).not.toBeInTheDocument()
  const reads = vi.mocked(api.get).mock.calls.filter(([url]) => url === '/api/channel/quota/changes')
  expect(reads).toHaveLength(2)
})

it('truncated table evidence keeps a returned failure and does not label an omitted connection as having no samples', async () => {
  items = [item, { ...item, id: 24, name: 'Synthetic omitted evidence' }]
  savedEvidence = [{ channel_id: 23, name: item.name, metric_type: 'balance', source: 'channel_type_1', unit: 'usd', status: 'error', series_id: 'a'.repeat(64) }]
  evidenceMetadata = { total_items: 101, items_complete: false }
  await mount()
  await waitFor(() => expect(screen.getAllByText(/Balance query: Balance sample failed/)[0]).toBeVisible())
  expect(screen.queryByText('No saved window sample')).not.toBeInTheDocument()
  const omitted = screen.getByRole('link', { name: 'Synthetic omitted evidence' }).closest('tr')
  if (!omitted) throw new Error('Expected omitted evidence channel row')
  expect(within(omitted).getByText('Saved quota evidence is incomplete. Missing series and earlier failures may be omitted.')).toBeVisible()
  expect(vi.mocked(api.get).mock.calls.filter(([url]) => url === '/api/channel/quota/changes')).toHaveLength(1)
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
})
it('a hidden known inventory error uses its own precise read time instead of the previous successful read or unread state', async () => {
  const now = vi.spyOn(Date, 'now').mockReturnValue(1700000000123)
  const report = vi.fn()
  const user = userEvent.setup()
  const { client } = await mount({ task: true, report })
  await user.click(screen.getByRole('button', { name: 'Open synthetic inventory' }))
  await screen.findByRole('link', { name: item.name })
  await waitFor(() => expect(report).toHaveBeenLastCalledWith({ phase: 'ready', readAt: 1700000000.123 }))
  now.mockReturnValue(1700000100456)
  outcome = 'error'
  await act(async () => { await client.invalidateQueries({ queryKey: ['channels'] }) })
  expect(await screen.findByRole('alert')).toHaveTextContent('Failed to load channels')
  await waitFor(() => expect(report).toHaveBeenLastCalledWith({ phase: 'error', readAt: 1700000100.456 }))
  await user.click(screen.getByRole('button', { name: 'Close synthetic inventory' }))
  expect(report).toHaveBeenLastCalledWith({ phase: 'error', readAt: 1700000100.456 })
  expect(screen.queryByRole('navigation', { name: 'Connections' })).not.toBeInTheDocument()
})
it('auto-disabled evidence stays visible and expands completely while menu and display actions never probe a provider', async () => {
  const reason = 'Synthetic failure ' + 'complete-long-reason-'.repeat(12)
  items = [{ ...item, status: 3, other_info: JSON.stringify({ status_reason: reason, status_time: 1700000000 }) }]
  const user = userEvent.setup()
  await mount()
  await screen.findByRole('link', { name: item.name })
  const synopsis = screen.getAllByText(/^Synthetic failure/).filter((element) => !element.closest('[hidden]'))
  expect(synopsis).toHaveLength(1)
  expect(synopsis[0]).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Disabled channel details' }))
  expect(screen.getByText(reason, { exact: true })).toBeVisible()
  const before = inventoryReads().length
  await user.click(screen.getByRole('button', { name: 'Open menu' }))
  expect(await screen.findByRole('menuitem', { name: 'Advanced test' })).toBeVisible()
  expect(screen.getByText('Diagnostics')).toBeVisible()
  expect(screen.getByText('Model configuration')).toBeVisible()
  expect(screen.getByText('Credential maintenance')).toBeVisible()
  expect(screen.getByText('Dangerous operations')).toBeVisible()
  expect(inventoryReads()).toHaveLength(before)
  expect(vi.mocked(api.get).mock.calls.some(([url]) => /test|update_balance|fetch_models/.test(String(url)))).toBe(false)
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
  expect(api.delete).not.toHaveBeenCalled()
})
it('global maintenance and display settings are explicit scopes while read-only action permissions remain disabled', async () => {
  useAuthStore.getState().auth.setUser({ id: 2, role: 10, username: 'synthetic-reader', permissions: { admin_permissions: { channel: { read: true, write: false, operate: false, sensitive_write: false } } } })
  const user = userEvent.setup()
  await mount()
  await screen.findByRole('link', { name: item.name })
  expect(screen.getByRole('button', { name: 'Create Channel' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Edit' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Test Connection' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Disable' })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Global maintenance' }))
  expect(screen.getByText('All channels')).toBeVisible()
  expect(screen.getByRole('menuitem', { name: 'Test All Channels' })).toHaveAttribute('aria-disabled', 'true')
  expect(screen.getByRole('menuitem', { name: 'Delete All Disabled' })).toHaveAttribute('aria-disabled', 'true')
  await user.keyboard('{Escape}')
  await user.click(screen.getByRole('button', { name: 'Display settings' }))
  expect(screen.getByRole('menuitemcheckbox', { name: 'Tag Mode' })).toHaveAttribute('aria-checked', 'false')
  expect(screen.getByRole('menuitemcheckbox', { name: 'Sort by ID' })).toHaveAttribute('aria-checked', 'false')
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
  expect(api.delete).not.toHaveBeenCalled()
})

it('mobile monitoring keeps operational status visible and exposes every supplementary original field through an explicit local action', async () => {
  localStorage.setItem('channels:view-mode', 'card')
  const original = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({ ...original(query), matches: query === '(max-width: 640px)' }))
  const user = userEvent.setup()
  const copied = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined)
  await mount()
  await screen.findByRole('link', { name: item.name })
  expect(screen.getByRole('button', { name: 'Disable' })).toBeVisible()
  expect(screen.getByRole('button', { name: 'Edit' })).toBeVisible()
  expect(screen.getByRole('button', { name: 'Test Connection' })).toBeVisible()
  expect(screen.getByText('Priority')).not.toBeVisible()
  const reads = inventoryReads().length
  await user.click(screen.getByRole('button', { name: 'More channel fields' }))
  expect(screen.getByText('Priority')).toBeVisible()
  expect(screen.getByText('Weight')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Models (2)' }))
  expect(screen.getByText('alpha')).toBeVisible()
  expect(screen.getByText('beta')).toBeVisible()
  const model = screen.getByRole('button', { name: 'alpha' })
  act(() => model.focus())
  await user.keyboard('{Enter}')
  expect(copied).toHaveBeenCalledExactlyOnceWith('alpha')
  expect(inventoryReads()).toHaveLength(reads)
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
})
it('tag aggregation stays distinct from selected numerical IDs and preserves the exact batch status payload', async () => {
  items = [item, { ...item, id: 24, name: 'Second synthetic channel' }]
  const write = vi.mocked(api.post).mockResolvedValue({ data: { success: true, data: 1 } })
  const user = userEvent.setup()
  const { router } = await mount()
  await screen.findByRole('link', { name: item.name })
  await user.click(screen.getByRole('button', { name: 'Display settings' }))
  await user.click(screen.getByRole('menuitemcheckbox', { name: 'Tag Mode' }))
  await user.keyboard('{Escape}')
  await user.click(await screen.findByRole('button', { name: 'Expand tag synthetic-tag' }))
  await user.click(screen.getByRole('button', { name: 'Select channels' }))
  const name = screen.getByRole('link', { name: item.name })
  const row = name.closest('tr')
  if (!row) throw new Error('Expected numerical child channel row')
  await user.click(within(row).getByRole('checkbox', { name: 'Select row' }))
  expect(screen.getByText('Selected channel IDs (1)')).toBeVisible()
  expect(router.state.location.pathname).toBe('/channels/')
  expect(write).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Disable selected channels' }))
  await waitFor(() => expect(write).toHaveBeenCalledExactlyOnceWith('/api/channel/status/batch', { ids: [23], status: 2 }, expect.objectContaining({ skipAuthRefresh: true })))
  await waitFor(() => expect(screen.queryByText('Selected channel IDs (1)')).not.toBeInTheDocument())
})

it('legacy All sentinels remain authoritative without silently rewriting the retained URL or counting inactive filters', async () => {
  const user = userEvent.setup()
  const { router } = await mount({ entry: '/channels/?type=%5B%221%22%2C%22all%22%5D&group=%5B%22default%22%2C%22all%22%5D' })
  await screen.findByRole('link', { name: item.name })
  expect(inventoryReads()[0]?.[1]?.params).toMatchObject({ type: undefined, group: undefined })
  expect(screen.queryByText(/Active inventory filters/)).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'More filters (0)' }))
  expect(screen.getByRole('combobox', { name: 'Type' })).toHaveValue('all')
  expect(screen.getByRole('combobox', { name: 'Group' })).toHaveValue('all')
  expect(router.state.location.search).toMatchObject({ type: ['1', 'all'], group: ['default', 'all'] })
  expect(inventoryReads()).toHaveLength(1)
})

async function pendingPriorityChange() {
  const fixture = await mount()
  const initialUser = userEvent.setup()
  await screen.findByRole('link', { name: item.name })
  await initialUser.selectOptions(screen.getByRole('combobox', { name: 'Inventory view' }), 'routing')
  const priority = await screen.findByRole('group', { name: 'Priority' })
  vi.useFakeTimers({ shouldClearNativeTimers: true })
  const increment = within(priority).getByRole('button', { name: 'Increment' })
  act(() => { increment.focus(); fireEvent.click(increment) })
  expect(api.put).not.toHaveBeenCalled()
  return fixture
}

it.each(['SID', 'role'] as const)('an old pending inline edit cannot PUT during %s-change cleanup', async (change) => {
  const fixture = await pendingPriorityChange()
  act(() => {
    if (change === 'SID') {
      useAuthStore.setState((state) => ({ auth: { ...state.auth, session: { sid: 'synthetic-replacement', current: true, login_method: 'password', ip: '192.0.2.1', user_agent: 'Synthetic browser', created_at: 1, last_active_at: 1, expires_at: 4000000000 } } }))
    } else {
      useAuthStore.getState().auth.setUser({ id: 1, role: 10, username: 'synthetic-reader', permissions: { admin_permissions: { channel: { read: true, write: false, operate: false } } } })
    }
  })
  fixture.unmount()
  act(() => { vi.runOnlyPendingTimers() })
  await Promise.resolve()
  expect(api.put).not.toHaveBeenCalled()
  expect(api.post).not.toHaveBeenCalled()
})
it('same-actor write revocation prevents a pending inline cleanup PUT without changing the retained read scope', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, role: 10, username: 'synthetic-editor', permissions: { admin_permissions: { channel: { read: true, write: true, operate: true } } } })
  const fixture = await pendingPriorityChange()
  act(() => {
    useAuthStore.getState().auth.setUser({ id: 1, role: 10, username: 'synthetic-editor', permissions: { admin_permissions: { channel: { read: true, write: false, operate: true } } } })
  })
  fixture.unmount()
  act(() => { vi.runOnlyPendingTimers() })
  await Promise.resolve()
  expect(api.put).not.toHaveBeenCalled()
})
it.each(['blur', 'cleanup'] as const)('same-actor %s still commits exactly one original inline command', async (trigger) => {
  const put = vi.mocked(api.put).mockResolvedValue({ data: { success: true } })
  const fixture = await pendingPriorityChange()
  if (trigger === 'blur') {
    act(() => screen.getByPlaceholderText('Filter by name, ID, or key...').focus())
  } else {
    fixture.unmount()
  }
  act(() => { vi.runOnlyPendingTimers() })
  await Promise.resolve()
  expect(put).toHaveBeenCalledExactlyOnceWith('/api/channel/', { id: 23, priority: 1 }, expect.objectContaining({ skipAuthRefresh: true, skipErrorHandler: true, skipBusinessError: true }))
  if (trigger === 'blur') fixture.unmount()
  act(() => { vi.runOnlyPendingTimers() })
  await Promise.resolve()
  expect(put).toHaveBeenCalledTimes(1)
})
