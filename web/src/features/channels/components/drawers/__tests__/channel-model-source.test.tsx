import { QueryClient } from '@tanstack/react-query'
import { act, cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
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
const originalScroll = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'scrollIntoView')
const savedModels = 'synthetic-saved-model,synthetic-legacy/complete-id'
const channel = channelSchema.parse({ id: 23, type: 1, name: 'synthetic-source-channel', key: '', base_url: 'https://synthetic.invalid', status: 1, models: savedModels, group: 'default', created_time: 1, test_time: 0, response_time: 0, balance_updated_time: 0 })
const user = { id: 1, role: ROLE.ADMIN, username: 'synthetic-admin', permissions: { admin_permissions: { channel: { read: true, operate: true, write: true, sensitive_write: true } } } }
let client: QueryClient

beforeEach(() => {
  localStorage.clear()
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  useAuthStore.getState().auth.setUser(user)
  Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { configurable: true, value: vi.fn() })
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/channel/23') return { data: { success: true, data: channel } }
    if (url === '/api/channel/fetch_models/23') return { data: { success: true, data: ['synthetic-current-only'] } }
    if (url === '/api/channel/models') return { data: { success: true, data: [{ id: 'synthetic-unrelated-global' }] } }
    if (url === '/api/group/') return { data: { success: true, data: ['default'] } }
    if (url === '/api/prefill_group') return { data: { success: true, data: [{ id: 1, name: 'synthetic-custom-template', items: ['synthetic-preset'] }] } }
    if (url === '/api/user/2fa/status' || url === '/api/user/passkey') return { data: { success: true, data: { enabled: false } } }
    if (url === '/api/channel/model-discovery/23') return { data: { success: true, data: null } }
    if (url === '/api/status') return { data: { success: true, data: {} } }
    throw new Error(`Unexpected GET ${url}`)
  })
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected synthetic POST'))
  vi.spyOn(api, 'delete').mockRejectedValue(new Error('Unexpected synthetic DELETE'))
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: false, message: 'Synthetic save held for inspection' } })
})
afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.setState(originalAuth)
  localStorage.clear()
  vi.restoreAllMocks()
  if (originalScroll) Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', originalScroll)
  else Reflect.deleteProperty(HTMLElement.prototype, 'scrollIntoView')
})

function Editor() {
  const [open, setOpen] = useState(true)
  return <ChannelMutateDrawer open={open} onOpenChange={setOpen} currentRow={channel} />
}
async function renderEditor() {
  await renderListRoute({ client, path: '/channels/', element: <ChannelsProvider><Editor /></ChannelsProvider> })
  const input = await screen.findByDisplayValue('synthetic-source-channel')
  const form = input.closest('form')
  if (!form) throw new Error('Expected channel editor form')
  await screen.findByRole('button', { name: 'synthetic-legacy/complete-id' })
  return form
}

it('preserves complete saved IDs and the original save payload without consulting the global model directory or probing upstream on open', async () => {
  const form = await renderEditor()
  expect(screen.getByRole('button', { name: 'synthetic-legacy/complete-id' })).toBeVisible()
  expect(screen.queryByRole('button', { name: 'Fill All' })).not.toBeInTheDocument()
  expect(screen.queryByText('synthetic-unrelated-global')).not.toBeInTheDocument()
  expect(vi.mocked(api.get).mock.calls.some(([url]) => url === '/api/channel/models' || url === '/api/channel/fetch_models/23')).toBe(false)
  fireEvent.submit(form)
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1))
  expect(vi.mocked(api.put).mock.calls[0]?.[0]).toBe('/api/channel/')
  expect(vi.mocked(api.put).mock.calls[0]?.[1]).toMatchObject({ id: 23, name: 'synthetic-source-channel', models: savedModels, group: 'default' })
  expect(api.post).not.toHaveBeenCalled()
})

it('fetches only the current saved connection and applies selection without dropping IDs absent from its listing', async () => {
  const form = await renderEditor()
  const actions = userEvent.setup()
  await actions.click(screen.getByRole('button', { name: 'Fetch from Upstream' }))
  const dialog = await screen.findByRole('dialog', { name: 'Fetch Models' })
  await actions.click(await within(dialog).findByRole('checkbox', { name: 'synthetic-current-only' }))
  expect(api.get).toHaveBeenCalledWith('/api/channel/fetch_models/23', { skipAuthRefresh: true, skipBusinessError: true, skipErrorHandler: true })
  expect(vi.mocked(api.get).mock.calls.filter(([url]) => url === '/api/channel/fetch_models/23')).toHaveLength(1)
  expect(api.put).not.toHaveBeenCalled()
  expect(api.post).not.toHaveBeenCalled()
  await actions.click(within(dialog).getByRole('button', { name: 'Save Models' }))
  expect(screen.getByRole('button', { name: 'synthetic-legacy/complete-id' })).toBeVisible()
  expect(screen.getByRole('button', { name: 'synthetic-current-only' })).toBeVisible()
  fireEvent.submit(form)
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1))
  expect(vi.mocked(api.put).mock.calls[0]?.[1]).toMatchObject({ id: 23, models: `${savedModels},synthetic-current-only` })
})

it('allows an unlisted saved ID to be removed individually while keeping manual and custom preset configuration separate from discovery', async () => {
  const form = await renderEditor()
  const actions = userEvent.setup()
  const chip = screen.getByRole('button', { name: 'synthetic-legacy/complete-id' }).closest('[data-slot="combobox-chip"]')
  if (!chip) throw new Error('Expected saved model chip')
  const remove = chip.querySelector('[data-slot="combobox-chip-remove"]')
  if (!(remove instanceof HTMLElement)) throw new Error('Expected saved model removal action')
  await actions.click(remove)
  const input = screen.getByRole('combobox', { name: 'Select models or add custom ones' })
  await actions.type(input, 'synthetic-manual{Enter}')
  await actions.keyboard('{Escape}')
  expect(screen.getByText('Presets are user-configured templates, not models discovered for this connection.')).toBeVisible()
  await actions.click(await screen.findByRole('button', { name: 'synthetic-custom-template' }))
  fireEvent.submit(form)
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1))
  expect(vi.mocked(api.put).mock.calls[0]?.[1]).toMatchObject({ id: 23, models: 'synthetic-saved-model,synthetic-manual,synthetic-preset' })
  expect(vi.mocked(api.get).mock.calls.some(([url]) => url === '/api/channel/fetch_models/23' || url === '/api/channel/models')).toBe(false)
  expect(api.post).not.toHaveBeenCalled()
})

it('does not use saved credentials or a global fallback for an edited ordinary connection organization', async () => {
  await renderEditor()
  fireEvent.change(screen.getByRole('textbox', { name: 'OpenAI Organization' }), { target: { value: 'synthetic-new-org' } })
  await userEvent.setup().click(screen.getByRole('button', { name: 'Fetch from Upstream' }))
  const dialog = await screen.findByRole('dialog', { name: 'Fetch Models' })
  expect(await within(dialog).findByRole('alert')).toHaveTextContent('Save or restore the changed connection settings before fetching models. You can still enter model IDs manually.')
  expect(within(dialog).queryByRole('button', { name: 'Save Models' })).not.toBeInTheDocument()
  expect(vi.mocked(api.get).mock.calls.some(([url]) => url === '/api/channel/fetch_models/23' || url === '/api/channel/models' || url === '/api/channel/23/key')).toBe(false)
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
})

it('rejects a pending listing when the same actor loses operation permission before its response resolves', async () => {
  let complete: ((value: { data: { success: boolean; data: string[] } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean; data: string[] } }>((resolve) => { complete = resolve })
  const originalGet = vi.mocked(api.get).getMockImplementation()
  vi.mocked(api.get).mockImplementation((url, config) => {
    if (url === '/api/channel/fetch_models/23') return pending
    if (!originalGet) throw new Error('Expected external HTTP fixture')
    return originalGet(url, config)
  })
  await renderEditor()
  await userEvent.setup().click(screen.getByRole('button', { name: 'Fetch from Upstream' }))
  await waitFor(() => expect(vi.mocked(api.get).mock.calls.filter(([url]) => url === '/api/channel/fetch_models/23')).toHaveLength(1))
  await act(async () => {
    useAuthStore.getState().auth.setUser({ ...user, permissions: { admin_permissions: { channel: { ...user.permissions.admin_permissions.channel, operate: false } } } })
    complete?.({ data: { success: true, data: ['synthetic-revoked-result'] } })
    await pending
  })
  const dialog = screen.getByRole('dialog', { name: 'Fetch Models' })
  expect(within(dialog).queryByRole('checkbox', { name: 'synthetic-revoked-result' })).not.toBeInTheDocument()
  expect(within(dialog).queryByRole('button', { name: 'Save Models' })).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'synthetic-revoked-result' })).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'synthetic-legacy/complete-id' })).toBeInTheDocument()
  expect(api.put).not.toHaveBeenCalled()
  expect(api.post).not.toHaveBeenCalled()
})


it('checks live write permission again when an already-rendered apply action is dispatched before the permission rerender', async () => {
  await renderEditor()
  const actions = userEvent.setup()
  await actions.click(screen.getByRole('button', { name: 'Fetch from Upstream' }))
  const dialog = await screen.findByRole('dialog', { name: 'Fetch Models' })
  await actions.click(await within(dialog).findByRole('checkbox', { name: 'synthetic-current-only' }))
  const apply = within(dialog).getByRole('button', { name: 'Save Models' })
  await act(async () => {
    useAuthStore.getState().auth.setUser({ ...user, permissions: { admin_permissions: { channel: { ...user.permissions.admin_permissions.channel, write: false } } } })
    fireEvent.click(apply)
    await Promise.resolve()
  })
  expect(screen.queryByRole('button', { name: 'synthetic-current-only' })).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'synthetic-legacy/complete-id' })).toBeInTheDocument()
  expect(api.put).not.toHaveBeenCalled()
  expect(api.post).not.toHaveBeenCalled()
})
