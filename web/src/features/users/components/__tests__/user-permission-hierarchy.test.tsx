import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { userSchema } from '../../types'
import { UsersMutateDrawer } from '../users-mutate-drawer'
import { UsersProvider } from '../users-provider'

const originalAuth = useAuthStore.getState()
const target = userSchema.parse({ id: 22, username: 'synthetic-admin', display_name: 'Synthetic administrator', role: ROLE.ADMIN, group: 'default', status: 1, quota: 0, used_quota: 0, request_count: 0, github_id: 'synthetic-complete/github-id', email: 'synthetic@example.invalid', admin_permissions: { channel: { read: true, write: false } } })
const catalog = {
  resources: [
    { resource: 'channel', label_key: 'Synthetic channels', actions: [
      { action: 'read', label_key: 'Synthetic read', description_key: 'Read description' },
      { action: 'write', label_key: 'Synthetic write', description_key: 'Write description' },
      { action: 'operate', label_key: 'Synthetic operate', description_key: 'Operate description' },
    ] },
    { resource: 'user', label_key: 'Synthetic users', actions: [{ action: 'read', label_key: 'Synthetic user read', description_key: 'User read description' }] },
  ],
  roles: [{ key: 'admin', name: 'Admin', built_in: true, superuser: false, grants: { channel: { read: true, write: true, operate: true }, user: { read: false } } }],
}
let client: QueryClient
beforeEach(() => {
  localStorage.clear()
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.SUPER_ADMIN, username: 'synthetic-root' })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/user/22') return { data: { success: true, data: target } }
    if (url === '/api/group/') return { data: { success: true, data: ['default'] } }
    if (url === '/api/authz/catalog') return { data: { success: true, data: catalog } }
    throw new Error(`Unexpected GET ${url}`)
  })
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: false, message: 'Synthetic save held for inspection' } })
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected synthetic POST'))
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); localStorage.clear(); vi.restoreAllMocks() })

function Editor() {
  const [open, setOpen] = useState(true)
  return <UsersMutateDrawer open={open} onOpenChange={setOpen} currentRow={target} />
}
function editor() {
  return <QueryClientProvider client={client}><UsersProvider><Editor /></UsersProvider></QueryClientProvider>
}

it('keeps true, false and inherited values distinct while resource and binding disclosures preserve the original save payload', async () => {
  render(editor())
  const actions = userEvent.setup()
  const summary = await screen.findByRole('button', { name: 'Synthetic channels · True: 1 · False: 1 · Inherited: 1' })
  expect(summary).toHaveAttribute('aria-expanded', 'false')
  expect(screen.getByText('Only Root can edit administrator permissions.', { exact: false })).toBeVisible()
  expect(screen.queryByRole('checkbox', { name: 'Synthetic write Write description' })).not.toBeInTheDocument()
  const reads = vi.mocked(api.get).mock.calls.length
  summary.focus()
  await actions.keyboard('{Enter}')
  expect(screen.getByRole('checkbox', { name: 'Synthetic read Read description' })).toBeChecked()
  expect(screen.getByRole('checkbox', { name: 'Synthetic write Write description' })).not.toBeChecked()
  expect(screen.getByRole('checkbox', { name: 'Synthetic operate Operate description' })).toBeChecked()
  expect(screen.getByText('Granted (true)')).toBeVisible()
  expect(screen.getByText('Denied (false)')).toBeVisible()
  expect(screen.getByText('Inherited default: Granted')).toBeVisible()
  await actions.click(summary)
  const bindings = screen.getByRole('button', { name: 'Binding values (6)' })
  expect(bindings).toHaveAttribute('aria-expanded', 'false')
  await actions.click(bindings)
  expect(screen.getByDisplayValue('synthetic-complete/github-id')).toBeVisible()
  expect(screen.getByDisplayValue('synthetic-complete/github-id')).toBeDisabled()
  expect(screen.getByDisplayValue('synthetic@example.invalid')).toBeVisible()
  await actions.click(bindings)
  expect(api.get).toHaveBeenCalledTimes(reads)
  const input = screen.getByRole('textbox', { name: 'Username' })
  const form = input.closest('form')
  if (!form) throw new Error('Expected user editor form')
  fireEvent.submit(form)
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1))
  expect(api.put).toHaveBeenCalledWith('/api/user/', {
    id: 22, username: 'synthetic-admin', display_name: 'Synthetic administrator', password: undefined,
    group: 'default', account_tier_id: 'standard', remark: undefined,
    admin_permissions: { channel: { read: true, write: false, operate: true }, user: { read: false } },
  }, { skipAuthRefresh: true })
  expect(api.post).not.toHaveBeenCalled()
})

it('keeps a permission edit through collapse and saves the same explicit false bit with all untouched permission values', async () => {
  render(editor())
  const actions = userEvent.setup()
  await actions.click(await screen.findByRole('button', { name: 'Synthetic channels · True: 1 · False: 1 · Inherited: 1' }))
  await actions.click(screen.getByRole('checkbox', { name: 'Synthetic read Read description' }))
  const summary = screen.getByRole('button', { name: 'Synthetic channels · True: 1 · False: 2 · Inherited: 0' })
  await actions.click(summary)
  await actions.click(summary)
  expect(screen.getByRole('checkbox', { name: 'Synthetic read Read description' })).not.toBeChecked()
  expect(screen.getByRole('checkbox', { name: 'Synthetic write Write description' })).not.toBeChecked()
  expect(screen.getByRole('checkbox', { name: 'Synthetic operate Operate description' })).toBeChecked()
  const input = screen.getByRole('textbox', { name: 'Username' })
  const form = input.closest('form')
  if (!form) throw new Error('Expected user editor form')
  fireEvent.submit(form)
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1))
  expect(vi.mocked(api.put).mock.calls[0]?.[1]).toEqual({
    id: 22, username: 'synthetic-admin', display_name: 'Synthetic administrator', password: undefined,
    group: 'default', account_tier_id: 'standard', remark: undefined,
    admin_permissions: { channel: { read: false, write: false, operate: true }, user: { read: false } },
  })
})

it('keeps Root eligibility visible and removes permission controls when the actor loses Root role', async () => {
  render(editor())
  await screen.findByRole('button', { name: 'Synthetic channels · True: 1 · False: 1 · Inherited: 1' })
  act(() => useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.ADMIN, username: 'synthetic-admin-actor' }))
  await screen.findByDisplayValue(target.username)
  expect(screen.getByText('Only Root can edit administrator permissions.', { exact: false })).toBeVisible()
  expect(screen.queryByRole('button', { name: /^Synthetic channels/ })).not.toBeInTheDocument()
  expect(screen.queryByRole('checkbox', { name: 'Synthetic read Read description' })).not.toBeInTheDocument()
  expect(api.put).not.toHaveBeenCalled()
  expect(api.post).not.toHaveBeenCalled()
})

it('keeps permission and metadata drafts through transport, business and malformed catalog failures until an explicit successful read-only retry', async () => {
  let catalogMode: 'success' | 'transport' | 'business' | 'malformed' = 'success'
  const originalGet = vi.mocked(api.get).getMockImplementation()
  vi.mocked(api.get).mockImplementation(async (url, config) => {
    if (url === '/api/authz/catalog') {
      if (catalogMode === 'transport') throw new Error('Synthetic permission catalog transport failure')
      if (catalogMode === 'business') return { data: { success: false, message: 'Synthetic permission catalog business failure' } }
      if (catalogMode === 'malformed') return { data: { success: true, data: {} } }
    }
    if (!originalGet) throw new Error('Expected external catalog transport fixture')
    return originalGet(url, config)
  })
  render(editor())
  const actions = userEvent.setup()
  await actions.click(await screen.findByRole('button', { name: 'Synthetic channels · True: 1 · False: 1 · Inherited: 1' }))
  await actions.click(screen.getByRole('checkbox', { name: 'Synthetic read Read description' }))
  fireEvent.change(screen.getByRole('textbox', { name: 'Display Name' }), { target: { value: 'Synthetic unsaved display draft' } })
  catalogMode = 'transport'
  await act(async () => { await client.invalidateQueries({ queryKey: ['admin-permission-catalog'] }) })
  expect(await screen.findByRole('alert')).toHaveTextContent('Unable to load the permission catalog. Permission catalog: Unknown')
  expect(screen.queryByRole('button', { name: /^Synthetic channels/ })).not.toBeInTheDocument()
  expect(screen.queryByText('No administrator permissions are listed in this catalog.')).not.toBeInTheDocument()
  expect(screen.getByRole('textbox', { name: 'Display Name' })).toHaveValue('Synthetic unsaved display draft')
  catalogMode = 'business'
  await actions.click(screen.getByRole('button', { name: 'Retry' }))
  await waitFor(() => expect(vi.mocked(api.get).mock.calls.filter(([url]) => url === '/api/authz/catalog')).toHaveLength(3))
  expect(await screen.findByRole('alert')).toHaveTextContent('Permission catalog: Unknown')
  catalogMode = 'malformed'
  await actions.click(screen.getByRole('button', { name: 'Retry' }))
  await waitFor(() => expect(vi.mocked(api.get).mock.calls.filter(([url]) => url === '/api/authz/catalog')).toHaveLength(4))
  expect(await screen.findByRole('alert')).toHaveTextContent('Permission catalog: Unknown')
  catalogMode = 'success'
  await actions.click(screen.getByRole('button', { name: 'Retry' }))
  const restored = await screen.findByRole('button', { name: 'Synthetic channels · True: 1 · False: 2 · Inherited: 0' })
  await actions.click(restored)
  expect(screen.getByRole('checkbox', { name: 'Synthetic read Read description' })).not.toBeChecked()
  expect(screen.getByRole('checkbox', { name: 'Synthetic write Write description' })).not.toBeChecked()
  expect(screen.getByRole('checkbox', { name: 'Synthetic operate Operate description' })).toBeChecked()
  expect(screen.getByRole('textbox', { name: 'Display Name' })).toHaveValue('Synthetic unsaved display draft')
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(vi.mocked(api.get).mock.calls.filter(([url]) => url === '/api/authz/catalog')).toHaveLength(5)
  expect(api.put).not.toHaveBeenCalled()
  expect(api.post).not.toHaveBeenCalled()
})
