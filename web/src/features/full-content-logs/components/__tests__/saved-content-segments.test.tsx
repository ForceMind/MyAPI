import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import * as clipboard from '@/lib/copy-to-clipboard'
import { useAuthStore } from '@/stores/auth-store'

import { SAVED_SEGMENT_LIMIT } from '../../lib/saved-segment'
import type { FullContentLogDetail, SavedContentPart, SavedContentSegment } from '../../types'
import { RequestOriginalPanel, RequestStreamPanel } from '../request-content-panels'
import { SavedContentSegments } from '../saved-content-segments'
import { SavedStreamEvents } from '../saved-stream-events'

const originalAuth = useAuthStore.getState()
const clients: QueryClient[] = []
const snapshot = 'a'.repeat(64)
let session = 0
beforeEach(() => {
  session += 1
  useAuthStore.setState((state) => ({ auth: { ...state.auth,
    user: { id: 1, username: 'segment-viewer', role: 100 },
    session: { sid: `segment-${session}`, current: true, login_method: 'password', ip: '', user_agent: '', created_at: 1, last_active_at: 1, expires_at: 9999999999 },
  } }))
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected saved-content write'))
  vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected saved-content write'))
  vi.spyOn(api, 'delete').mockRejectedValue(new Error('Unexpected saved-content write'))
})
afterEach(() => { cleanup(); clients.splice(0).forEach((client) => client.clear()); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })

function savedPage(override: Partial<SavedContentSegment> = {}): SavedContentSegment {
  return { request_id: 'segment-ui', part: 'response', requested_offset: '0', offset: '0', next_offset: '4', total_bytes: '12', expected_bytes: null, has_more: true, snapshot, encoding: 'utf-8', content_type: 'text/plain', body: 'HEAD', source_complete: null, ...override }
}
function mount(part: SavedContentPart = 'response', requestId = 'segment-ui') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  clients.push(client)
  const element = (nextPart = part, nextId = requestId) => <QueryClientProvider client={client}><SavedContentSegments requestId={nextId} part={nextPart} /></QueryClientProvider>
  return { ...render(element()), client, element }
}
function viewer(part: SavedContentPart = 'response') {
  return screen.getByRole('region', { name: part === 'response' ? 'Saved response segments' : 'Saved request segments' })
}
async function open(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'Read saved response in segments' }))
  await waitFor(() => expect(viewer().querySelector('pre')?.textContent).toBe('HEAD'))
}

test('short pages retain the precise last-viewed position for Previous rather than subtracting the page limit', async () => {
  const get = vi.spyOn(api, 'get').mockImplementation(async (_url, config) => {
    const offset = String(config?.params?.offset)
    if (offset === '0') return { data: { success: true, data: savedPage() } }
    return { data: { success: true, data: savedPage({ requested_offset: offset, offset, body: offset === '4' ? 'NEXT' : 'LAST', next_offset: offset === '4' ? '8' : null, has_more: offset === '4' }) } }
  })
  mount()
  const user = userEvent.setup()
  expect(get).not.toHaveBeenCalled()
  await open(user)
  await user.click(within(viewer()).getByRole('button', { name: 'Next segment' }))
  await waitFor(() => expect(viewer().querySelector('pre')?.textContent).toBe('NEXT'))
  await user.click(within(viewer()).getByRole('button', { name: 'Next segment' }))
  await waitFor(() => expect(viewer().querySelector('pre')?.textContent).toBe('LAST'))
  await user.click(within(viewer()).getByRole('button', { name: 'Previous segment' }))
  await waitFor(() => expect(viewer().querySelector('pre')?.textContent).toBe('NEXT'))
  expect(get.mock.calls.map(([, config]) => config?.params?.offset)).toEqual(['0', '4', '8', '4'])
  expect(api.post).not.toHaveBeenCalled()
})

test('large decimal jumps and close/reopen preserve only cursor metadata and do not read before an explicit action', async () => {
  const tail = '9007199254740993'
  const get = vi.spyOn(api, 'get').mockImplementation(async (_url, config) => {
    const offset = config?.params?.offset
    return { data: { success: true, data: savedPage({ requested_offset: offset, offset, body: offset === '0' ? 'HEAD' : 'TAIL', total_bytes: '9007199254740997', has_more: offset === '0', next_offset: offset === '0' ? '4' : null }) } }
  })
  mount()
  const user = userEvent.setup()
  await open(user)
  const input = within(viewer()).getByRole('textbox', { name: 'Saved byte position' })
  await user.clear(input)
  await user.type(input, tail)
  expect(get).toHaveBeenCalledTimes(1)
  await user.click(within(viewer()).getByRole('button', { name: 'Go to saved byte' }))
  await waitFor(() => expect(viewer().querySelector('pre')?.textContent).toBe('TAIL'))
  expect(get.mock.calls[1]?.[1]?.params).toEqual({ part: 'response', offset: tail, limit: SAVED_SEGMENT_LIMIT, snapshot })
  await user.click(within(viewer()).getByRole('button', { name: 'Close segment reader' }))
  expect(screen.queryByRole('region', { name: 'Saved response segments' })).not.toBeInTheDocument()
  expect(get).toHaveBeenCalledTimes(2)
  await user.click(screen.getByRole('button', { name: 'Read saved response in segments' }))
  await waitFor(() => expect(viewer().querySelector('pre')?.textContent).toBe('TAIL'))
  expect(get.mock.calls[2]?.[1]?.params).toEqual({ part: 'response', offset: tail, limit: SAVED_SEGMENT_LIMIT, snapshot })
  expect(within(viewer()).getByRole('button', { name: 'Previous segment' })).toBeDisabled()
  expect(api.post).not.toHaveBeenCalled()
})

test('a full decoded MiB base64 page retains its larger encoded tail and copies the exact current segment', async () => {
  const body = btoa('x'.repeat(SAVED_SEGMENT_LIMIT - 4) + 'TAIL')
  const get = vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: savedPage({ body, encoding: 'base64', total_bytes: String(SAVED_SEGMENT_LIMIT), expected_bytes: String(SAVED_SEGMENT_LIMIT), source_complete: true, has_more: false, next_offset: null }) } })
  mount()
  const user = userEvent.setup()
  const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined)
  await user.click(screen.getByRole('button', { name: 'Read saved response in segments' }))
  await waitFor(() => expect(viewer().querySelector('pre')?.textContent).toBe(body))
  expect(body.length).toBeGreaterThan(SAVED_SEGMENT_LIMIT)
  await user.click(within(viewer()).getByRole('button', { name: 'Copy' }))
  expect(copy).toHaveBeenCalledExactlyOnceWith(body)
  expect(get).toHaveBeenCalledTimes(1)
})

test('the segmented SSE mode hides structured/raw previews and closing returns their search and focus without another read', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: savedPage({ next_offset: null, has_more: false, total_bytes: '4', body: 'TAIL' }) } })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  clients.push(client)
  const detail: FullContentLogDetail = { request_id: 'segment-ui', timestamp: 'saved', method: 'POST', path: '/v1/responses', status: 200, duration_ms: 1, request_bytes: 0, response_bytes: 20, chunk_count: 1, request_body: '', response_body: 'data: [DONE]\n\n', response_body_truncated: true, request_headers: {}, response_headers: {}, query: {} }
  render(<QueryClientProvider client={client}><SavedStreamEvents detail={detail} /></QueryClientProvider>)
  const structured = screen.getByRole('list', { name: 'Saved SSE event preview' })
  const raw = screen.getByRole('region', { name: 'Raw response body' })
  const user = userEvent.setup()
  await user.type(screen.getByRole('searchbox', { name: 'Search saved event preview' }), 'DONE')
  await user.click(screen.getByRole('button', { name: 'Read saved response in segments' }))
  await waitFor(() => expect(viewer().querySelector('pre')?.textContent).toBe('TAIL'))
  expect(structured).not.toBeVisible()
  expect(raw).not.toBeVisible()
  await user.click(within(viewer()).getByRole('button', { name: 'Close segment reader' }))
  expect(structured).toBeVisible()
  expect(raw).toBeVisible()
  expect(screen.getByRole('searchbox', { name: 'Search saved event preview' })).toHaveValue('DONE')
  expect(screen.getByRole('button', { name: 'Read saved response in segments' })).toHaveFocus()
  expect(get).toHaveBeenCalledTimes(1)
})

test.each([
  ['FULL_CONTENT_SEGMENT_CHANGED', 'Saved content changed. Reload to start a new read.'],
  ['FULL_CONTENT_SEGMENT_AMBIGUOUS', 'Saved request identity is ambiguous. Reload to try again.'],
  ['FULL_CONTENT_SEGMENT_CORRUPT', 'Saved content is corrupt. Reload to try again.'],
])('a %s conflict clears the page and reloads a fresh snapshot only after an explicit action', async (code, message) => {
  const get = vi.spyOn(api, 'get').mockResolvedValueOnce({ data: { success: true, data: savedPage() } }).mockRejectedValueOnce(Object.assign(new Error('UNSAFE path/body'), { isAxiosError: true, response: { status: 409, data: { code } } })).mockResolvedValue({ data: { success: true, data: savedPage({ body: 'NEW', total_bytes: '3', next_offset: null, has_more: false, snapshot: 'b'.repeat(64) }) } })
  mount()
  const user = userEvent.setup()
  await open(user)
  await user.click(within(viewer()).getByRole('button', { name: 'Next segment' }))
  expect(await within(viewer()).findByRole('alert')).toHaveTextContent(message)
  expect(viewer().querySelector('pre')).toBeNull()
  expect(viewer()).not.toHaveTextContent('UNSAFE')
  expect(get).toHaveBeenCalledTimes(2)
  await user.click(within(viewer()).getByRole('button', { name: 'Reload saved content' }))
  await waitFor(() => expect(viewer().querySelector('pre')?.textContent).toBe('NEW'))
  expect(get.mock.calls[2]?.[1]?.params).toEqual({ part: 'response', offset: '0', limit: SAVED_SEGMENT_LIMIT })
})

test('a late old-page clipboard failure cannot appear on the next page', async () => {
  vi.spyOn(api, 'get').mockResolvedValueOnce({ data: { success: true, data: savedPage() } }).mockResolvedValue({ data: { success: true, data: savedPage({ requested_offset: '4', offset: '4', body: 'NEXT', next_offset: null, total_bytes: '8', has_more: false }) } })
  let release = () => {}
  const pending = new Promise<void>((resolve) => { release = resolve })
  vi.spyOn(clipboard, 'copyToClipboard').mockImplementation(async () => { await pending; return false })
  mount()
  const user = userEvent.setup()
  await open(user)
  await user.click(within(viewer()).getByRole('button', { name: 'Copy' }))
  await user.click(within(viewer()).getByRole('button', { name: 'Next segment' }))
  await waitFor(() => expect(viewer().querySelector('pre')?.textContent).toBe('NEXT'))
  await act(async () => release())
  expect(screen.queryByText('Failed to copy to clipboard')).not.toBeInTheDocument()
})

test.each(['request', 'part', 'actor', 'role', 'session', 'close', 'inactive', 'unmount'] as const)('changing %s cancels the current read in layout cleanup and drops its late payload', async (change) => {
  let release = () => {}
  let signal: AbortSignal | undefined
  const pending = new Promise<void>((resolve) => { release = resolve })
  const get = vi.spyOn(api, 'get').mockImplementation(async (_url, config) => { signal = config?.signal as AbortSignal; await pending; return { data: { success: true, data: savedPage({ body: 'LATE', next_offset: null, total_bytes: '4', has_more: false }) } } })
  const view = mount()
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Read saved response in segments' }))
  await waitFor(() => expect(get).toHaveBeenCalledTimes(1))
  if (change === 'request') view.rerender(view.element('response', 'next-request'))
  else if (change === 'part') view.rerender(view.element('request'))
  else if (change === 'actor' || change === 'role') act(() => useAuthStore.getState().auth.setUser({ id: change === 'actor' ? 2 : 1, username: 'next-scope', role: change === 'role' ? 1 : 100 }))
  else if (change === 'session') act(() => useAuthStore.setState((state) => ({ auth: { ...state.auth, session: state.auth.session && { ...state.auth.session, sid: 'next-session' } } })))
  else if (change === 'close') await user.click(within(viewer()).getByRole('button', { name: 'Close segment reader' }))
  else if (change === 'inactive') view.rerender(<QueryClientProvider client={view.client}><SavedContentSegments requestId='segment-ui' part='response' active={false} /></QueryClientProvider>)
  else view.unmount()
  expect(signal?.aborted).toBe(true)
  await act(async () => release())
  expect(screen.queryByText('LATE')).not.toBeInTheDocument()
  expect(get).toHaveBeenCalledTimes(1)
  expect(api.post).not.toHaveBeenCalled()
})

test.each([401, 403])('a current %s clears original and expanded saved bodies without restoring them on another tab', async (status) => {
  const auth = useAuthStore.getState().auth
  const key = ['full-content-logs', 'detail', 'denied-request', 1, auth.session?.sid ?? null, 100]
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  clients.push(client)
  const detail: FullContentLogDetail = { request_id: 'denied-request', timestamp: 'saved', method: 'POST', path: '/v1/responses', status: 200, duration_ms: 1, request_bytes: 1, response_bytes: 4, chunk_count: 1, request_body: 'SECRET REQUEST', response_body: 'SECRET RESPONSE', response_body_truncated: true, request_headers: {}, response_headers: {}, query: {} }
  client.setQueryData(key, detail)
  client.setQueryData([...key, 'expanded'], detail)
  const get = vi.spyOn(api, 'get').mockRejectedValue(Object.assign(new Error('UNSAFE server output'), { isAxiosError: true, response: { status } }))
  const view = render(<QueryClientProvider client={client}><RequestOriginalPanel requestId='denied-request' active /></QueryClientProvider>)
  await screen.findByRole('region', { name: 'Raw response body' })
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Read saved response in segments' }))
  await waitFor(() => expect(client.getQueryState(key)?.data).toBeUndefined())
  expect(client.getQueryState([...key, 'expanded'])?.data).toBeUndefined()
  expect(client.getQueryState(key)?.status).toBe('error')
  expect(screen.queryByText('SECRET RESPONSE')).not.toBeInTheDocument()
  get.mockImplementation(async () => new Promise(() => {}))
  view.rerender(<QueryClientProvider client={client}><RequestStreamPanel requestId='denied-request' active /></QueryClientProvider>)
  expect(screen.queryByText('SECRET RESPONSE')).not.toBeInTheDocument()
  expect(screen.queryByText('SECRET REQUEST')).not.toBeInTheDocument()
  expect(api.post).not.toHaveBeenCalled()
})
