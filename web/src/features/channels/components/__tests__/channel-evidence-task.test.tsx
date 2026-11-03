import { QueryClient } from '@tanstack/react-query'
import { act, cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import type { ChannelEvidenceTask, ChannelTaskReadState } from '../../channel-task-state'
import { ChannelQuotaChangesPanel } from '../channel-quota-changes-panel'
import { ChannelQuotaEvents } from '../channel-quota-events'
import { ChannelRoutingPreview } from '../channel-routing-preview'

const originalAuth = useAuthStore.getState()
const series = { channel_id: 7, name: 'Synthetic channel', account_label: 'Synthetic quota account', source: 'codex_wham_usage_primary', window_type: 'five_hour', window_seconds: 18000, metric_type: 'codex_rate_limit', unit: 'percent', status: 'error', current_available: null, series_id: 'synthetic-series-one', observed_at: 100 }
const savedList = { success: true, data: { items: [series], source_complete: false, items_complete: true } }
const history = { success: true, data: { channel_id: 7, start: 100, end: 200, limit: 5000, unit: 'percent', source_complete: true, points_complete: true, complete: true, points: [{ timestamp: 100, status: 'error', error_code: 'synthetic-upstream-error' }] } }
const event = { id: 1, event_key: 'synthetic-event-key', snapshot_id: 1, channel_id: 7, series_ref: 'synthetic-series-one', status: 'exhausted', kind: 'threshold', state: 'retryable', attempt_count: 0, observed_at: 100, created_at: 100, updated_at: 100, last_error_code: 'synthetic-delivery-error', evidence: { channel_id: 7, observed_at: 100, available: 0, total: 100, unit: 'percent', metric_type: 'codex_rate_limit', window_type: 'five_hour' } }
const snapshot = { success: true, data: { group: 'default', model: 'synthetic-model', source: 'database', generation: 7, cache_enabled: false, cache_pending: false, tiers: [], rejected: [] } }
let client: QueryClient
function setActor(sid: string) {
  useAuthStore.setState((state) => ({ auth: { ...state.auth, user: { id: 17, role: 100, username: 'synthetic-root' }, session: { sid, current: true, login_method: 'password', ip: '192.0.2.1', user_agent: 'Synthetic browser', created_at: 1, last_active_at: 1, expires_at: 9999999999 } } }))
}
beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  setActor('synthetic-task-session')
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/channel/quota/changes') return { data: savedList }
    if (url === '/api/channel/quota/status') return { data: { success: true, data: { enabled: false, interval_seconds: 300, max_channels: 20 } } }
    if (url === '/api/channel/7/quota/history') return { data: history }
    if (url === '/api/channel/quota/events') return { data: { success: true, data: { items: [event], page: 1, page_size: 10 } } }
    if (url === '/api/channel/routing-preview') return { data: snapshot }
    throw new Error(`Unexpected synthetic evidence GET ${url}`)
  })
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected synthetic evidence POST'))
  vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected synthetic evidence PUT'))
})
afterEach(() => { vi.useRealTimers(); cleanup(); client.clear(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })
function TaskFrame(props: { kind: ChannelEvidenceTask; initial?: boolean; report: (state: ChannelTaskReadState) => void }) {
  const [active, setActive] = useState(props.initial ?? false)
  let panel = <ChannelQuotaChangesPanel active={active} onReadStateChange={props.report} />
  if (props.kind === 'events') panel = <ChannelQuotaEvents active={active} onReadStateChange={props.report} />
  if (props.kind === 'routing') panel = <ChannelRoutingPreview active={active} onReadStateChange={props.report} />
  return <><button type='button' onClick={() => setActive(true)}>Enter task</button><button type='button' onClick={() => setActive(false)}>Leave task</button><div hidden={!active}>{panel}</div></>
}
async function renderTask(kind: ChannelEvidenceTask, report = vi.fn(), initial = false, entry = '/channels/') {
  await renderListRoute({ client, path: '/channels/', entry, element: <TaskFrame kind={kind} report={report} initial={initial} /> })
  return report
}
function calls(url: string) { return vi.mocked(api.get).mock.calls.filter(([path]) => path === url) }

it('reads quota only after entry, leaves the series unselected, and preserves failed and incomplete observations outside selection', async () => {
  const report = await renderTask('quota')
  expect(api.get).not.toHaveBeenCalled()
  expect(report).toHaveBeenLastCalledWith(expect.objectContaining({ phase: 'unread' }))
  await userEvent.setup().click(screen.getByRole('button', { name: 'Enter task' }))
  await screen.findByText('Saved quota evidence includes failed observations.')
  expect(screen.getByText('Incomplete quota history')).toBeVisible()
  expect(calls('/api/channel/7/quota/history')).toHaveLength(0)
  expect(screen.getByLabelText('Detailed series')).toHaveValue('')
  await waitFor(() => expect(report).toHaveBeenLastCalledWith(expect.objectContaining({ phase: 'ready', incomplete: true, samplingEnabled: false })))
  await userEvent.setup().selectOptions(screen.getByLabelText('Detailed series'), screen.getByRole('option', { name: /Synthetic quota account/ }))
  await waitFor(() => expect(calls('/api/channel/7/quota/history')).toHaveLength(1))
  expect(calls('/api/channel/7/quota/history')[0]?.[1]).toEqual(expect.objectContaining({ signal: expect.any(AbortSignal), params: expect.objectContaining({ limit: 5000, source: series.source, window_seconds: 18000, unit: 'percent' }) }))
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
})

it('keeps failed-observation feedback separate from a ready HTTP read even when the visible status filter excludes those rows', async () => {
  const report = await renderTask('quota', vi.fn(), true)
  await waitFor(() => expect(report).toHaveBeenLastCalledWith(expect.objectContaining({ phase: 'ready', failedObservations: true })))
  fireEvent.change(screen.getByLabelText('Status'), { target: { value: 'success' } })
  expect(screen.getByText('Saved quota evidence includes failed observations.')).toBeVisible()
  expect(report).toHaveBeenLastCalledWith(expect.objectContaining({ phase: 'ready', failedObservations: true, incomplete: true }))
  expect(calls('/api/channel/quota/changes')).toHaveLength(1)
  expect(calls('/api/channel/7/quota/history')).toHaveLength(0)
})

it('does not substitute another channel when the old quotaChannelId has no saved evidence in the requested range', async () => {
  await renderTask('quota', vi.fn(), true, '/channels/?quotaChannelId=99')
  expect(await screen.findByText('No saved evidence for the requested channel in this range.')).toBeVisible()
  expect(calls('/api/channel/7/quota/history')).toHaveLength(0)
  expect(calls('/api/channel/quota/changes')[0]?.[1]).toEqual(expect.objectContaining({ params: expect.objectContaining({ channel_ids: '99', limit: 2000, sort: 'abs_change_per_minute' }) }))
  expect(screen.queryByRole('option', { name: /Synthetic quota account/ })).not.toBeInTheDocument()
})

it('stops the quota page polling clock while its retained task is inactive', async () => {
  vi.useFakeTimers()
  await renderTask('quota', vi.fn(), true)
  await act(async () => { await vi.advanceTimersByTimeAsync(0) })
  expect(calls('/api/channel/quota/changes')).toHaveLength(1)
  fireEvent.click(screen.getByRole('button', { name: 'Leave task' }))
  await act(async () => { await vi.advanceTimersByTimeAsync(65000) })
  expect(calls('/api/channel/quota/changes')).toHaveLength(1)
  expect(calls('/api/channel/quota/status')).toHaveLength(1)
  expect(calls('/api/channel/7/quota/history')).toHaveLength(0)
})

it('keeps a selected detail failure in the task state when only the list is successfully refreshed again', async () => {
  const original = vi.mocked(api.get).getMockImplementation()
  if (!original) throw new Error('Expected synthetic evidence transport')
  vi.mocked(api.get).mockImplementation((url, ...args) => url === '/api/channel/7/quota/history' ? Promise.reject(new Error('Synthetic detail read failed')) : original(url, ...args))
  const report = await renderTask('quota', vi.fn(), true)
  await userEvent.setup().selectOptions(await screen.findByLabelText('Detailed series'), screen.getByRole('option', { name: /Synthetic quota account/ }))
  await waitFor(() => expect(report).toHaveBeenLastCalledWith(expect.objectContaining({ phase: 'error', incomplete: true })))
  await act(async () => { await client.refetchQueries({ queryKey: ['channel-quota-changes'] }) })
  await waitFor(() => expect(calls('/api/channel/quota/changes')).toHaveLength(2))
  expect(report).toHaveBeenLastCalledWith(expect.objectContaining({ phase: 'error', incomplete: true }))
})

it('keeps event scope global and unknown totals unknown while the original zero evidence and delivery error remain readable', async () => {
  const report = await renderTask('events')
  expect(api.get).not.toHaveBeenCalled()
  await userEvent.setup().click(screen.getByRole('button', { name: 'Enter task' }))
  expect(await screen.findByText('Observed remaining: 0 / 100 percent')).toBeVisible()
  expect(screen.getByText('All channel events')).toBeVisible()
  expect(screen.getByText('Page 1 · total unavailable')).toBeVisible()
  expect(screen.getByText('Delivery error: synthetic-delivery-error')).toBeVisible()
  expect(calls('/api/channel/quota/events')[0]?.[1]).toEqual(expect.objectContaining({ params: { p: 1, page_size: 10, status: undefined } }))
  await waitFor(() => expect(report).toHaveBeenLastCalledWith(expect.objectContaining({ phase: 'ready' })))
})

it('lets an in-flight read report its real error after the same actor leaves the retained event task', async () => {
  let fail: ((reason: Error) => void) | undefined
  const pending = new Promise<never>((_resolve, reject) => { fail = reject })
  vi.mocked(api.get).mockReturnValue(pending)
  const report = await renderTask('events', vi.fn(), true)
  await waitFor(() => expect(api.get).toHaveBeenCalledTimes(1))
  await userEvent.setup().click(screen.getByRole('button', { name: 'Leave task' }))
  await act(async () => { fail?.(new Error('Synthetic late read failure')); await pending.catch(() => undefined) })
  await waitFor(() => expect(report).toHaveBeenLastCalledWith(expect.objectContaining({ phase: 'error', readAt: expect.any(Number) })))
})

it('preserves routing inputs and the last snapshot across task visits without replaying the saved preview', async () => {
  const report = await renderTask('routing')
  expect(api.get).not.toHaveBeenCalled()
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Enter task' }))
  await user.type(screen.getByLabelText('Routing preview model'), 'synthetic-model')
  await user.type(screen.getByLabelText('Routing preview request path'), '/v1/responses')
  expect(api.get).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Preview routing' }))
  expect(await screen.findByText('Snapshot epoch 7')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Leave task' }))
  await act(async () => { await client.invalidateQueries({ queryKey: ['channels', 'list'] }) })
  await user.click(screen.getByRole('button', { name: 'Enter task' }))
  expect(screen.getByLabelText('Routing preview model')).toHaveValue('synthetic-model')
  expect(screen.getByLabelText('Routing preview request path')).toHaveValue('/v1/responses')
  expect(screen.getByText('Snapshot epoch 7')).toBeVisible()
  expect(calls('/api/channel/routing-preview')).toHaveLength(1)
  await user.click(screen.getByRole('button', { name: 'Preview routing' }))
  await waitFor(() => expect(calls('/api/channel/routing-preview')).toHaveLength(2))
  expect(calls('/api/channel/routing-preview')[0]?.[1]).toEqual(expect.objectContaining({ params: { group: 'default', model: 'synthetic-model', request_path: '/v1/responses' } }))
  await waitFor(() => expect(report).toHaveBeenLastCalledWith(expect.objectContaining({ phase: 'ready' })))
  expect(api.post).not.toHaveBeenCalled()
})

it('does not publish old-session quota evidence after the same user replaces the session', async () => {
  let finish: ((value: { data: typeof savedList }) => void) | undefined
  const pending = new Promise<{ data: typeof savedList }>((resolve) => { finish = resolve })
  const original = vi.mocked(api.get).getMockImplementation()
  if (!original) throw new Error('Expected synthetic evidence transport')
  let reads = 0
  vi.mocked(api.get).mockImplementation((url, ...args) => {
    if (url === '/api/channel/quota/changes') { reads += 1; return reads === 1 ? pending : Promise.resolve({ data: { success: true, data: { items: [], source_complete: true, items_complete: true } } }) }
    return original(url, ...args)
  })
  const report = await renderTask('quota', vi.fn(), true)
  await waitFor(() => expect(reads).toBe(1))
  await act(async () => { setActor('synthetic-new-task-session') })
  await waitFor(() => expect(reads).toBe(2))
  await screen.findByText('No account quota changes recorded yet. Enable quota sampling or query a provider account to start history.')
  await act(async () => { finish?.({ data: savedList }); await pending })
  expect(screen.queryByRole('option', { name: /Synthetic quota account/ })).not.toBeInTheDocument()
  await waitFor(() => expect(report).toHaveBeenLastCalledWith(expect.objectContaining({ phase: 'ready', incomplete: false })))
})


it('does not display another known account-window identity under the selected series title', async () => {
  const original = vi.mocked(api.get).getMockImplementation()
  if (!original) throw new Error('Expected synthetic evidence transport')
  vi.mocked(api.get).mockImplementation((url, ...args) => url === '/api/channel/7/quota/history' ? Promise.resolve({ data: { ...history, data: { ...history.data, series_id: 'different-synthetic-series' } } }) : original(url, ...args))
  const report = await renderTask('quota', vi.fn(), true)
  await userEvent.setup().selectOptions(await screen.findByLabelText('Detailed series'), screen.getByRole('option', { name: /Synthetic quota account/ }))
  const title = await screen.findByText('Unable to load quota history')
  const error = title.closest<HTMLElement>('[role=alert]')
  if (!error) throw new Error('Expected the original history error boundary')
  await userEvent.setup().click(within(error).getByText('Details'))
  expect(screen.getByText('This history belongs to a different account window. Refresh the directory before continuing.')).toBeVisible()
  await waitFor(() => expect(report).toHaveBeenLastCalledWith(expect.objectContaining({ phase: 'error' })))
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
})

it('reports failed observations from the selected saved history without calling a successful HTTP read an error', async () => {
  const original = vi.mocked(api.get).getMockImplementation()
  if (!original) throw new Error('Expected synthetic evidence transport')
  vi.mocked(api.get).mockImplementation((url, ...args) => url === '/api/channel/quota/changes' ? Promise.resolve({ data: { success: true, data: { items: [{ ...series, status: 'success', current_available: 0 }], source_complete: true, items_complete: true } } }) : original(url, ...args))
  const report = await renderTask('quota', vi.fn(), true)
  await waitFor(() => expect(report).toHaveBeenLastCalledWith(expect.objectContaining({ phase: 'ready', failedObservations: false })))
  await userEvent.setup().selectOptions(screen.getByLabelText('Detailed series'), screen.getByRole('option', { name: /Synthetic quota account/ }))
  await waitFor(() => expect(report).toHaveBeenLastCalledWith(expect.objectContaining({ phase: 'ready', failedObservations: true, incomplete: false })))
  expect(screen.getByText('Saved quota evidence includes failed observations.')).toBeVisible()
  expect(calls('/api/channel/7/quota/history')).toHaveLength(1)
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
})
