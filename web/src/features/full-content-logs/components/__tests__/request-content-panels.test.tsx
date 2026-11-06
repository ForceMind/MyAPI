/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'
import { api } from '@/lib/api'

import * as savedApi from '../../api'
import type { FullContentLogDetail } from '../../types'
import { RequestBodyPanel, RequestOriginalPanel } from '../request-content-panels'
import { FullContentLogDetailsDialog } from '../full-content-log-details-dialog'

const DETAIL: FullContentLogDetail = {
  timestamp: '2026-10-30T01:02:03Z', request_id: 'saved-body-1',
  method: 'POST', path: '/v1/responses', model: 'saved-model',
  status: 200, duration_ms: 10, request_bytes: 64, response_bytes: 32,
  chunk_count: 1, request_encoding: 'json', response_encoding: 'json',
  request_body: '{ "messages": [{"role":"user","content":"A saved question"}], "temperature": 0 }',
  response_body: '{"output_text":"A saved answer"}',
  request_headers: { 'X-Saved-Header': ['request evidence'] },
  response_headers: { 'X-Saved-Header': ['response evidence'] },
  query: { saved: ['parameter evidence'] },
}
const clients: QueryClient[] = []
const originalAuth = useAuthStore.getState()

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.setState(originalAuth)
  vi.restoreAllMocks()
})

function setup() {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'saved-viewer', role: 100 })
  const read = vi.spyOn(savedApi, 'getFullContentLogDetail').mockResolvedValue({ success: true, data: DETAIL })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: Infinity } } })
  clients.push(client)
  return { read, client }
}

function panel(client: QueryClient, active: boolean, requestId: string | null = DETAIL.request_id) {
  return <QueryClientProvider client={client}><RequestBodyPanel requestId={requestId} active={active} /></QueryClientProvider>
}

describe('saved request content panels', () => {
  test('an explicit saved-response segment reader reaches bytes beyond 16 MiB without eager reads, accumulation or writes', async () => {
    useAuthStore.getState().auth.setUser({ id: 1, username: 'saved-viewer', role: 100 })
    const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: Infinity } } })
    clients.push(client)
    const detailPath = `/api/full-content-logs/${DETAIL.request_id}`
    const segmentsPath = `${detailPath}/segments`
    const snapshot = 'a'.repeat(64)
    const limit = 1024 * 1024
    const tailOffset = '16777216'
    const totalBytes = '16777220'
    const get = vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
      if (url === detailPath) return { data: { success: true, data: { ...DETAIL, response_body: 'HEAD', response_encoding: 'utf-8', response_content_type: 'text/plain', response_body_truncated: true } } }
      if (url !== segmentsPath) throw new Error(`Unexpected saved-content GET ${url}`)
      const params = config?.params as Record<string, unknown> | undefined
      if (params?.part !== 'response' || params.limit !== limit || typeof params.offset !== 'string') throw new Error('Invalid saved response segment request')
      if (params.offset !== '0' && params.snapshot !== snapshot) throw new Error('Missing saved response snapshot')
      const pages: Record<string, { body: string; next: string | null }> = {
        '0': { body: 'HEAD', next: '4' },
        '4': { body: 'NEXT', next: '8' },
        [tailOffset]: { body: 'TAIL', next: null },
      }
      const page = pages[params.offset]
      if (!page) throw new Error('Unexpected saved response byte position')
      return { data: { success: true, data: {
        request_id: DETAIL.request_id, part: 'response', requested_offset: params.offset,
        offset: params.offset, next_offset: page.next, total_bytes: totalBytes,
        expected_bytes: null, has_more: page.next !== null, snapshot,
        encoding: 'utf-8', content_type: 'text/plain', body: page.body, source_complete: null,
      } } }
    })
    const post = vi.spyOn(api, 'post').mockRejectedValue(new Error('Saved-content reads must not POST'))
    const put = vi.spyOn(api, 'put').mockRejectedValue(new Error('Saved-content reads must not PUT'))
    const remove = vi.spyOn(api, 'delete').mockRejectedValue(new Error('Saved-content reads must not DELETE'))
    render(<QueryClientProvider client={client}><RequestOriginalPanel requestId={DETAIL.request_id} active /></QueryClientProvider>)
    const preview = await screen.findByRole('region', { name: 'Raw response body' })
    expect(preview.querySelector('pre')?.textContent).toBe('HEAD')
    expect(get).toHaveBeenCalledExactlyOnceWith(detailPath, expect.objectContaining({ params: { max_response_bytes: limit } }))
    expect(post).not.toHaveBeenCalled()
    expect(put).not.toHaveBeenCalled()
    expect(remove).not.toHaveBeenCalled()

    // The old source must fail here on the missing user-visible entry, not an import.
    await userEvent.setup().click(await screen.findByRole('button', { name: 'Read saved response in segments' }))
    await screen.findByRole('region', { name: 'Saved response segments' })
    const viewer = () => screen.getByRole('region', { name: 'Saved response segments' })
    await waitFor(() => expect(viewer().querySelector('pre')?.textContent).toBe('HEAD'))
    expect(preview).not.toBeVisible()
    expect(within(viewer()).getByRole('button', { name: 'Previous segment' })).toBeDisabled()
    for (const label of ['Expected bytes', 'Capture completeness']) {
      const term = within(viewer()).getByText(label, { selector: 'dt' })
      const value = term.nextElementSibling
      if (!value) throw new Error(`Expected saved-content metadata for ${label}`)
      expect(value).toHaveTextContent('Unknown')
      expect(value).not.toHaveTextContent(/^0$/)
    }
    const user = userEvent.setup()
    await user.click(within(viewer()).getByRole('button', { name: 'Next segment' }))
    await waitFor(() => expect(viewer().querySelector('pre')?.textContent).toBe('NEXT'))
    expect(viewer().querySelectorAll('pre')).toHaveLength(1)
    await user.click(within(viewer()).getByRole('button', { name: 'Previous segment' }))
    await waitFor(() => expect(viewer().querySelector('pre')?.textContent).toBe('HEAD'))
    const position = within(viewer()).getByRole('textbox', { name: 'Saved byte position' })
    await user.clear(position)
    await user.type(position, tailOffset)
    expect(get.mock.calls.filter(([url]) => url === segmentsPath)).toHaveLength(3)
    await user.click(within(viewer()).getByRole('button', { name: 'Go to saved byte' }))
    await waitFor(() => expect(viewer().querySelector('pre')?.textContent).toBe('TAIL'))
    expect(within(viewer()).getByRole('textbox', { name: 'Saved byte position' })).toHaveValue(tailOffset)
    expect(within(viewer()).getByRole('button', { name: 'Next segment' })).toBeDisabled()
    expect(viewer().querySelectorAll('pre')).toHaveLength(1)
    await user.click(within(viewer()).getByRole('button', { name: 'First segment' }))
    await waitFor(() => expect(viewer().querySelector('pre')?.textContent).toBe('HEAD'))
    expect(get.mock.calls.filter(([url]) => url === segmentsPath).map(([, config]) => config?.params)).toEqual([
      { part: 'response', offset: '0', limit },
      { part: 'response', offset: '4', limit, snapshot },
      { part: 'response', offset: '0', limit, snapshot },
      { part: 'response', offset: tailOffset, limit, snapshot },
      { part: 'response', offset: '0', limit, snapshot },
    ])
    expect(post).not.toHaveBeenCalled()
    expect(put).not.toHaveBeenCalled()
    expect(remove).not.toHaveBeenCalled()
    await user.click(within(viewer()).getByRole('button', { name: 'Close segment reader' }))
    expect(preview).toBeVisible()
    expect(screen.getByRole('button', { name: 'Read saved response in segments' })).toHaveFocus()
  })

  test('a truncated saved response is expanded only by an explicit bounded read while the existing preview stays visible', async () => {
    const { read, client } = setup()
    read.mockResolvedValueOnce({ success: true, data: { ...DETAIL, response_body_truncated: true, response_body_total_bytes: 2 * 1024 * 1024 } })
    let release = () => {}
    const pending = new Promise<void>((resolve) => { release = resolve })
    read.mockImplementation(async () => { await pending; return { success: true, data: DETAIL } })
    render(panel(client, true))
    await screen.findByText('A saved answer')
    expect(read).toHaveBeenCalledTimes(1)
    await userEvent.setup().click(screen.getByRole('button', { name: 'Read more saved content (up to 16 MiB)' }))
    expect(screen.getByText('A saved answer')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Read more saved content (up to 16 MiB)' })).toBeDisabled()
    await act(async () => release())
    await waitFor(() => expect(read).toHaveBeenCalledTimes(2))
    expect(read.mock.calls[1]).toEqual([DETAIL.request_id, expect.any(AbortSignal), 16 * 1024 * 1024])
  })

  test('a failed expanded read keeps its explicit error and the previously read preview without pretending content is complete', async () => {
    const { read, client } = setup()
    read.mockResolvedValueOnce({ success: true, data: { ...DETAIL, response_body_truncated: true } })
    read.mockRejectedValue(new Error('Synthetic expanded read failed'))
    render(panel(client, true))
    await screen.findByText('A saved answer')
    await userEvent.setup().click(screen.getByRole('button', { name: 'Read more saved content (up to 16 MiB)' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Synthetic expanded read failed')
    expect(screen.getByText('A saved answer')).toBeVisible()
  })

  test('changing request identity cancels an expanded read and its late body never replaces the next request', async () => {
    const { read, client } = setup()
    read.mockResolvedValueOnce({ success: true, data: { ...DETAIL, response_body_truncated: true } })
    let release = () => {}
    let signal: AbortSignal | undefined
    const pending = new Promise<void>((resolve) => { release = resolve })
    read.mockImplementation(async (_id, readSignal, bytes) => {
      if (bytes) { signal = readSignal; await pending; return { success: true, data: { ...DETAIL, response_body: 'LATE OLD SAVED BODY' } } }
      return { success: true, data: { ...DETAIL, request_id: 'next-saved-request', response_body: '{"output_text":"Next saved answer"}' } }
    })
    const view = render(panel(client, true))
    await screen.findByText('A saved answer')
    await userEvent.setup().click(screen.getByRole('button', { name: 'Read more saved content (up to 16 MiB)' }))
    view.rerender(panel(client, true, 'next-saved-request'))
    await screen.findByText('Next saved answer')
    expect(signal?.aborted).toBe(true)
    await act(async () => release())
    expect(screen.queryByText('LATE OLD SAVED BODY')).not.toBeInTheDocument()
  })

  test('changing actors cancels the expanded read and does not grant the next actor an automatic expanded read', async () => {
    const { read, client } = setup()
    read.mockResolvedValueOnce({ success: true, data: { ...DETAIL, response_body_truncated: true } })
    let release = () => {}
    let signal: AbortSignal | undefined
    const pending = new Promise<void>((resolve) => { release = resolve })
    read.mockImplementation(async (_id, readSignal, bytes) => {
      if (bytes) { signal = readSignal; await pending; return { success: true, data: { ...DETAIL, response_body: 'OLD ACTOR EXPANDED BODY' } } }
      return { success: true, data: { ...DETAIL, response_body: '{"output_text":"Next actor saved answer"}' } }
    })
    render(panel(client, true))
    await screen.findByText('A saved answer')
    await userEvent.setup().click(screen.getByRole('button', { name: 'Read more saved content (up to 16 MiB)' }))
    act(() => useAuthStore.getState().auth.setUser({ id: 2, username: 'next-saved-viewer', role: 100 }))
    await screen.findByText('Next actor saved answer')
    expect(signal?.aborted).toBe(true)
    await act(async () => release())
    expect(screen.queryByText('OLD ACTOR EXPANDED BODY')).not.toBeInTheDocument()
    expect(read.mock.calls.filter((call) => call[2] !== undefined)).toHaveLength(1)
  })

  test('the server truncation marker stays visible after the larger finite read reaches its limit', async () => {
    const { read, client } = setup()
    read.mockResolvedValue({ success: true, data: { ...DETAIL, response_body_truncated: true } })
    render(panel(client, true))
    await screen.findByText('A saved answer')
    await userEvent.setup().click(screen.getByRole('button', { name: 'Read more saved content (up to 16 MiB)' }))
    expect(await screen.findByText('Saved content is still truncated at the 16 MiB read limit. The remainder is unavailable in this view.')).toBeVisible()
    expect(read).toHaveBeenCalledTimes(2)
    expect(screen.queryByRole('button', { name: 'Read more saved content (up to 16 MiB)' })).not.toBeInTheDocument()
  })

  test('body reads start only when active and the readable/original switch preserves all saved fields', async () => {
    const { read, client } = setup()
    const user = userEvent.setup()
    const view = render(panel(client, false))
    expect(read).not.toHaveBeenCalled()
    view.rerender(panel(client, true))
    await screen.findByText('A saved answer')
    expect(screen.getByText('USER A saved question')).toBeInTheDocument()
    expect(screen.queryByText('request evidence', { exact: false })).not.toBeInTheDocument()
    await user.click(screen.getByText('Request headers'))
    await screen.findByText(/request evidence/)

    await user.click(screen.getByRole('button', { name: 'Raw data' }))
    expect(screen.getByRole('button', { name: 'Raw data' })).toHaveAttribute('aria-pressed', 'true')
    const raw = screen.getByRole('region', { name: 'Raw request body' })
    expect(raw.querySelector('pre')?.textContent).toBe(DETAIL.request_body)
    expect(screen.getByText('Query parameters')).toBeInTheDocument()
    expect(read).toHaveBeenCalledTimes(1)
  })

  test('the legacy dialog retains Request/Response tabs with one shared raw-mode control', async () => {
    const { read, client } = setup()
    const user = userEvent.setup()
    render(
      <QueryClientProvider client={client}>
        <FullContentLogDetailsDialog requestId={DETAIL.request_id} open onOpenChange={() => undefined} />
      </QueryClientProvider>
    )
    await screen.findByRole('tab', { name: 'Request' })
    expect(screen.getAllByRole('button', { name: 'Raw data' })).toHaveLength(1)
    await user.click(screen.getByRole('button', { name: 'Raw data' }))
    expect(screen.getByRole('region', { name: 'Raw request body' }).querySelector('pre')?.textContent).toBe(DETAIL.request_body)
    await user.click(screen.getByRole('tab', { name: /^Response/ }))

    const response = await screen.findByRole('region', { name: 'Raw response body' })
    expect(response.querySelector('pre')?.textContent).toBe(DETAIL.response_body)
    expect(screen.getByRole('button', { name: 'Raw data' })).toHaveAttribute('aria-pressed', 'true')
    expect(read).toHaveBeenCalledTimes(1)
  })

  test('the original tab shares the administrator gate and hides a cached body after role loss', async () => {
    const { read, client } = setup()
    render(<QueryClientProvider client={client}><RequestOriginalPanel requestId={DETAIL.request_id} active /></QueryClientProvider>)
    const original = await screen.findByRole('region', { name: 'Raw request body' })
    expect(original.querySelector('pre')?.textContent).toBe(DETAIL.request_body)
    await act(async () => {
      useAuthStore.getState().auth.setUser({ id: 1, username: 'saved-viewer', role: 1 })
    })

    expect(screen.getByText('Administrator access required')).toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'Raw request body' })).not.toBeInTheDocument()
    expect(read).toHaveBeenCalledTimes(1)
  })

  test('a failed saved-body refetch shows the error and removes previously visible content', async () => {
    const { read, client } = setup()
    render(panel(client, true))
    await screen.findByText('A saved answer')
    read.mockRejectedValue(new Error('saved evidence forbidden'))
    await act(async () => { await client.invalidateQueries({ queryKey: ['full-content-logs', 'detail'] }) })

    await screen.findByText('saved evidence forbidden')
    expect(screen.queryByText('A saved answer')).not.toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'Request text' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeEnabled()
  })

  test('a legacy row without a Request ID never fetches body content', () => {
    const { read, client } = setup()
    render(panel(client, true, null))

    expect(screen.getByText('No Request ID on this record')).toBeInTheDocument()
    expect(read).not.toHaveBeenCalled()
  })

  test('missing saved content preserves the capture-off explanation instead of showing an empty body', async () => {
    const { read, client } = setup()
    read.mockRejectedValue(Object.assign(new Error('missing'), { isAxiosError: true, response: { status: 404 } }))
    vi.spyOn(savedApi, 'getFullContentLogs').mockResolvedValue({ success: true, data: {
      enabled: false, page: 1, page_size: 1, total: 0, items: [],
      files: { count: 0, total_size: 0, files: [] }, facets: { models: [], tokens: [] },
    } })
    render(panel(client, true))

    await screen.findByText('No saved body for this request')
    await waitFor(() => expect(screen.getByText('Body capture is off (FULL_CONTENT_LOG_ENABLED). Existing files remain available.')).toBeInTheDocument())
    expect(screen.queryByRole('region', { name: 'Request text' })).not.toBeInTheDocument()
  })
})
