import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { ChannelRoutingFieldEditor } from '../channel-routing-field-editor'

const originalAuth = useAuthStore.getState()
let client: QueryClient

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 7, role: 100, username: 'synthetic-owner' })
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
  vi.spyOn(api, 'post').mockRejectedValue(new Error('No provider request is allowed'))
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: { id: 23, weight: 0 } } })
})
afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.setState(originalAuth)
  vi.useRealTimers()
  vi.restoreAllMocks()
})

function editor(value: number | null, canWrite = true, readVersion = 1) {
  return <QueryClientProvider client={client}><ChannelRoutingFieldEditor channelId={23} field='weight' value={value} canWrite={canWrite} readVersion={readVersion} /></QueryClientProvider>
}

test('an unknown saved value stays unknown without a fabricated zero or implicit update', () => {
  render(editor(null))
  expect(screen.getByText('Unknown')).toBeVisible()
  expect(screen.queryByRole('button', { name: '0' })).not.toBeInTheDocument()
  cleanup()
  expect(api.put).not.toHaveBeenCalled()
})

test('a legal zero edits through the existing blur flush and sends exactly the original field command', async () => {
  render(editor(0))
  const group = screen.getByRole('group', { name: 'Weight' })
  act(() => fireEvent.click(within(group).getByRole('button', { name: 'Increment' })))
  expect(api.put).not.toHaveBeenCalled()
  expect(screen.getByText('Pending save')).toBeVisible()
  await act(async () => fireEvent.blur(within(group).getByRole('button', { name: '1' }), { relatedTarget: document.body }))
  expect(api.put).toHaveBeenCalledExactlyOnceWith('/api/channel/', { id: 23, weight: 1 }, expect.objectContaining({ skipAuthRefresh: true }))
  expect(screen.getByText('Saved')).toBeVisible()
  expect(api.post).not.toHaveBeenCalled()
})

test('fresh write revocation cancels a pending cleanup commit while preserving the saved zero', () => {
  const view = render(editor(0))
  fireEvent.click(screen.getByRole('button', { name: 'Increment' }))
  view.rerender(editor(0, false))
  view.unmount()
  expect(api.put).not.toHaveBeenCalled()
})

test('an actor switch cannot flush an old pending command during cleanup', () => {
  const view = render(editor(0))
  fireEvent.click(screen.getByRole('button', { name: 'Increment' }))
  act(() => useAuthStore.getState().auth.setUser({ id: 8, role: 100, username: 'synthetic-next-owner' }))
  view.unmount()
  expect(api.put).not.toHaveBeenCalled()
})

test.each([undefined, 500, 408])('an uncertain response %s locks editing until an explicit saved-value read without another PUT', async (status) => {
  vi.mocked(api.put).mockRejectedValue(Object.assign(new Error('Synthetic response lost'), { isAxiosError: true, response: status ? { status } : undefined }))
  render(editor(0))
  fireEvent.click(screen.getByRole('button', { name: 'Increment' }))
  await act(async () => fireEvent.blur(screen.getByRole('button', { name: '1' }), { relatedTarget: document.body }))
  expect(screen.getByRole('alert')).toHaveTextContent('Save result unknown. Read the saved value before trying again.')
  expect(screen.getByRole('button', { name: 'Increment' })).toBeDisabled()
  fireEvent.click(screen.getByRole('button', { name: 'Increment' }))
  expect(api.put).toHaveBeenCalledTimes(1)
  expect(api.get).not.toHaveBeenCalled()
  await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Read saved routing value' })))
  expect(api.get).toHaveBeenCalledExactlyOnceWith('/api/channel/23', expect.objectContaining({ disableDuplicate: true }))
  expect(screen.getByRole('button', { name: 'Increment' })).toBeEnabled()
  expect(screen.getByRole('button', { name: '0' })).toBeVisible()
  expect(screen.getByText('Previous draft: 1')).toBeVisible()
  expect(api.put).toHaveBeenCalledTimes(1)
})

test('a failed explicit saved-value read keeps the unconfirmed draft locked and does not PUT again', async () => {
  vi.mocked(api.put).mockRejectedValue(new Error('Synthetic response lost'))
  vi.mocked(api.get).mockRejectedValue(new Error('Synthetic read unavailable'))
  render(editor(0))
  fireEvent.click(screen.getByRole('button', { name: 'Increment' }))
  await act(async () => fireEvent.blur(screen.getByRole('button', { name: '1' }), { relatedTarget: document.body }))
  await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Read saved routing value' })))
  expect(screen.getByText('Unable to read the saved routing value. Editing remains locked.')).toBeVisible()
  expect(screen.getByRole('button', { name: '1' })).toBeDisabled()
  expect(api.put).toHaveBeenCalledTimes(1)
})

test('a saved-value read for an old actor cannot restore or unlock the next actor field', async () => {
  vi.mocked(api.put).mockRejectedValue(new Error('Synthetic response lost'))
  let release = () => {}
  const pending = new Promise<void>((resolve) => { release = resolve })
  vi.mocked(api.get).mockImplementation(async () => { await pending; return { data: { success: true, data: { id: 23, weight: 5 } } } })
  const view = render(editor(0))
  fireEvent.click(screen.getByRole('button', { name: 'Increment' }))
  await act(async () => fireEvent.blur(screen.getByRole('button', { name: '1' }), { relatedTarget: document.body }))
  fireEvent.click(screen.getByRole('button', { name: 'Read saved routing value' }))
  act(() => useAuthStore.getState().auth.setUser({ id: 8, role: 100, username: 'synthetic-next-owner' }))
  view.rerender(editor(null))
  await act(async () => release())
  expect(screen.getByText('Unknown')).toBeVisible()
  expect(screen.queryByRole('button', { name: '5' })).not.toBeInTheDocument()
  expect(api.put).toHaveBeenCalledTimes(1)
})

test('a rejected field update stays failed instead of reporting saved', async () => {
  vi.mocked(api.put).mockResolvedValue({ data: { success: false, message: 'Synthetic rejected update' } })
  render(editor(0))
  fireEvent.click(screen.getByRole('button', { name: 'Increment' }))
  await act(async () => fireEvent.blur(screen.getByRole('button', { name: '1' }), { relatedTarget: document.body }))
  expect(screen.getByRole('alert')).toHaveTextContent('Synthetic rejected update')
  expect(screen.queryByText('Saved')).not.toBeInTheDocument()
})

test('an in-flight save disables the field and a committed pending cache publication remains explicit', async () => {
  let release = () => {}
  const pending = new Promise<void>((resolve) => { release = resolve })
  vi.mocked(api.put).mockImplementation(async () => {
    await pending
    return { data: { success: true, cache_pending: true } }
  })
  render(editor(0))
  fireEvent.click(screen.getByRole('button', { name: 'Increment' }))
  fireEvent.blur(screen.getByRole('button', { name: '1' }), { relatedTarget: document.body })
  expect(screen.getByText('Saving...')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Increment' })).toBeDisabled()
  await act(async () => release())
  expect(screen.getByText('Saved; routing cache publication pending')).toBeVisible()
  expect(api.put).toHaveBeenCalledTimes(1)
})

test('priority retains its existing negative boundary while a zero weight cannot decrement', () => {
  render(<QueryClientProvider client={client}>
    <ChannelRoutingFieldEditor channelId={23} field='priority' value={-999} canWrite />
    <ChannelRoutingFieldEditor channelId={23} field='weight' value={0} canWrite />
  </QueryClientProvider>)
  expect(within(screen.getByRole('group', { name: 'Priority' })).getByRole('button', { name: 'Decrement' })).toBeDisabled()
  expect(within(screen.getByRole('group', { name: 'Weight' })).getByRole('button', { name: 'Decrement' })).toBeDisabled()
  expect(screen.getByRole('button', { name: '-999' })).toBeVisible()
  expect(api.put).not.toHaveBeenCalled()
})

test('a pending draft and its acknowledged value survive stale props while a later saved read can update the field', async () => {
  const view = render(editor(1, true, 1))
  fireEvent.click(screen.getByRole('button', { name: 'Increment' }))
  view.rerender(editor(1, true, 2))
  expect(screen.getByRole('button', { name: '2' })).toBeVisible()
  await act(async () => fireEvent.blur(screen.getByRole('button', { name: '2' }), { relatedTarget: document.body }))
  expect(api.put).toHaveBeenCalledExactlyOnceWith('/api/channel/', { id: 23, weight: 2 }, expect.objectContaining({ skipAuthRefresh: true }))
  view.rerender(editor(1, true, 2))
  expect(screen.getByRole('button', { name: '2' })).toBeVisible()
  view.rerender(editor(3, true, 3))
  expect(screen.getByRole('button', { name: '3' })).toBeVisible()
})

test('a write cancels the pre-write channel read so its delayed old response cannot republish stale cache data', async () => {
  const queryKey = ['channels', 'list', 'synthetic-routing']
  client.setQueryData(queryKey, { weight: 1 })
  let release = () => {}
  const pending = new Promise<void>((resolve) => { release = resolve })
  const oldRead = client.fetchQuery({ queryKey, queryFn: async () => { await pending; return { weight: 0 } } }).catch(() => undefined)
  const view = render(editor(1, true, client.getQueryState(queryKey)?.dataUpdatedAt))
  fireEvent.click(screen.getByRole('button', { name: 'Increment' }))
  await act(async () => fireEvent.blur(screen.getByRole('button', { name: '2' }), { relatedTarget: document.body }))
  expect(screen.getByText('Saved')).toBeVisible()
  await act(async () => { release(); await oldRead })
  expect(client.getQueryData(queryKey)).toEqual({ weight: 1 })
  view.rerender(editor(1, true, client.getQueryState(queryKey)?.dataUpdatedAt))
  expect(screen.getByRole('button', { name: '2' })).toBeVisible()
})
