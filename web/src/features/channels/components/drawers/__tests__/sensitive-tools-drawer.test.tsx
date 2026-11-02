import { QueryClient } from '@tanstack/react-query'
import { act, cleanup, fireEvent, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { channelSchema } from '../../../types'
import { ChannelsProvider } from '../../channels-provider'
import { ChannelMutateDrawer } from '../channel-mutate-drawer'

const originalAuth = useAuthStore.getState()
const channel = channelSchema.parse({ id: 23, type: 1, name: 'synthetic-channel', key: '', status: 1, models: 'synthetic-model', group: 'default', created_time: 1, test_time: 0, response_time: 0, balance_updated_time: 0 })
let client: QueryClient

beforeEach(() => {
  localStorage.clear()
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.SUPER_ADMIN, username: 'synthetic-root' })
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/status') return { data: { success: true, data: {} } }
    if (url === '/api/channel/23') return { data: { success: true, data: channel } }
    if (url === '/api/group/') return { data: { success: true, data: ['default'] } }
    if (url === '/api/channel/models') return { data: { success: true, data: [{ id: 'synthetic-model' }] } }
    if (url === '/api/prefill_group') return { data: { success: true, data: [] } }
    throw new Error(`Unexpected GET ${url}`)
  })
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected channel write'))
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); localStorage.clear(); vi.restoreAllMocks() })

function Drawer(props: { onSaved?: (ids: number[]) => void }) {
  const [open, setOpen] = useState(true)
  return <ChannelMutateDrawer open={open} onOpenChange={setOpen} currentRow={channel} onSaved={props.onSaved} />
}
async function renderDrawer(onSaved?: (ids: number[]) => void) {
  await renderListRoute({ client, path: '/channels/', element: <ChannelsProvider><Drawer onSaved={onSaved} /></ChannelsProvider> })
  const input = await screen.findByDisplayValue('synthetic-channel')
  const form = input.closest('form')
  if (!form) throw new Error('Channel editor form is missing')
  return form
}

it('keeps the original channel editor open when Escape is pressed during a pending save', async () => {
  let complete: ((value: { data: { success: boolean } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean } }>((resolve) => { complete = resolve })
  const put = vi.spyOn(api, 'put').mockReturnValue(pending)
  const form = await renderDrawer()
  fireEvent.submit(form)
  await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
  try {
    await userEvent.setup().keyboard('{Escape}')
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByLabelText('Name *')).toHaveValue('synthetic-channel')
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
  } finally {
    await act(async () => { complete?.({ data: { success: true } }); await pending })
  }
})

it('admits one channel save when the same valid editor form is submitted twice before the write resolves', async () => {
  let complete: ((value: { data: { success: boolean } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean } }>((resolve) => { complete = resolve })
  const put = vi.spyOn(api, 'put').mockReturnValue(pending)
  const form = await renderDrawer()
  await act(async () => { fireEvent.submit(form); fireEvent.submit(form); await Promise.resolve() })
  await waitFor(() => expect(put).toHaveBeenCalled())
  try {
    expect(put).toHaveBeenCalledTimes(1)
    expect(put.mock.calls[0]?.[0]).toBe('/api/channel/')
    expect(put.mock.calls[0]?.[1]).toMatchObject({ id: 23, name: 'synthetic-channel', models: 'synthetic-model', group: 'default' })
  } finally {
    await act(async () => { complete?.({ data: { success: true } }); await pending })
  }
})

it('keeps a new identity editor draft open when an old identity save for the same channel resolves', async () => {
  let complete: ((value: { data: { success: boolean } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean } }>((resolve) => { complete = resolve })
  const put = vi.spyOn(api, 'put').mockReturnValue(pending)
  const saved = vi.fn()
  const form = await renderDrawer(saved)
  fireEvent.submit(form)
  await waitFor(() => expect(put).toHaveBeenCalledTimes(1))
  await act(async () => { useAuthStore.getState().auth.setUser({ id: 2, role: ROLE.SUPER_ADMIN, username: 'synthetic-new-root' }) })
  await screen.findByDisplayValue('synthetic-channel')
  fireEvent.change(screen.getByLabelText('Name *'), { target: { value: 'new-identity-draft' } })
  await act(async () => { complete?.({ data: { success: true } }); await pending })
  expect(screen.getByRole('dialog')).toBeInTheDocument()
  expect(screen.getByLabelText('Name *')).toHaveValue('new-identity-draft')
  expect(saved).not.toHaveBeenCalled()
})
