/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AxiosError, type AxiosAdapter, type InternalAxiosRequestConfig } from 'axios'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { ManagementKeysCard } from '../components/management-keys-card'
import type { ManagementKeyMetadata } from '../management-key-api'

const originalAuth = useAuthStore.getState()
const previousAdapter = api.defaults.adapter
const clients: QueryClient[] = []
const now = 1_900_000_000
const metadata: ManagementKeyMetadata = {
  id: 17, name: 'local audit', prefix: 'mk_audit',
  scopes: ['system.read'], channel_ids: [], token_ids: [],
  created_at: now - 100, expires_at: now + 86400, revoked_at: 0,
}

function signIn(role: number = ROLE.SUPER_ADMIN, sid = 'owner-session', userId = 1) {
  useAuthStore.getState().auth.setBundle({
    access_token: `session-${sid}`, token_type: 'Bearer', access_expires_at: 4_000_000_000,
    user: { id: userId, username: 'owner', role },
    session: { sid, current: true, login_method: 'password', ip: '127.0.0.1', user_agent: 'synthetic', created_at: 1, last_active_at: 1, expires_at: 4_000_000_000 },
  })
}

function response(config: InternalAxiosRequestConfig, data: unknown) {
  return { config, data, status: 200, statusText: 'OK', headers: {} }
}

function renderKeys(adapter: AxiosAdapter) {
  api.defaults.adapter = adapter
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
  clients.push(client)
  render(<QueryClientProvider client={client}><ManagementKeysCard /></QueryClientProvider>)
  return { user: userEvent.setup(), client }
}

async function openCreate(user: ReturnType<typeof userEvent.setup>) {
  const trigger = await screen.findByRole('button', { name: 'Create management key' })
  await waitFor(() => expect(trigger).toBeEnabled())
  await user.click(trigger)
}

async function reviewKey(user: ReturnType<typeof userEvent.setup>) {
  await openCreate(user)
  await user.type(screen.getByLabelText('Management key name'), 'local audit')
  await user.click(screen.getByRole('button', { name: 'Review management key' }))
}

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  api.defaults.adapter = previousAdapter
  useAuthStore.setState(originalAuth)
  vi.restoreAllMocks()
})

it.each([ROLE.USER, ROLE.GUEST])('hides management key controls for role %s without fetching', (role) => {
  signIn(role)
  const adapter = vi.fn<AxiosAdapter>()
  renderKeys(adapter)
  expect(screen.queryByText('Management keys')).not.toBeInTheDocument()
  expect(adapter).not.toHaveBeenCalled()
})

it('hides management key controls for an admin without a login session', () => {
  useAuthStore.getState().auth.reset()
  useAuthStore.getState().auth.setUser({ id: 1, username: 'owner', role: ROLE.SUPER_ADMIN })
  const adapter = vi.fn<AxiosAdapter>()
  renderKeys(adapter)
  expect(screen.queryByText('Management keys')).not.toBeInTheDocument()
  expect(adapter).not.toHaveBeenCalled()
})

it('confirms a read-only seven-day key, sends bounded resource IDs, and erases its one-time secret on close', async () => {
  signIn()
  vi.spyOn(Date, 'now').mockReturnValue(now * 1000)
  const writes: InternalAxiosRequestConfig[] = []
  const { user, client } = renderKeys(async (config) => {
    if (config.method === 'post') {
      writes.push(config)
      return response(config, { success: true, data: { key: metadata, secret: 'synthetic-one-time-secret' } })
    }
    return response(config, { success: true, data: [] })
  })
  await openCreate(user)
  await user.type(screen.getByLabelText('Management key name'), 'local audit')
  expect(screen.getByRole('checkbox', { name: 'Read channels' })).toBeChecked()
  expect(screen.getByRole('checkbox', { name: 'Operate channels' })).not.toBeChecked()
  await user.click(screen.getByText('Advanced resource limits'))
  await user.type(screen.getByLabelText('Channel IDs'), '12,9,12')
  await user.type(screen.getByLabelText('API token IDs'), '3,3')
  await user.click(screen.getByRole('button', { name: 'Review management key' }))
  expect(writes).toHaveLength(0)
  expect(screen.getByText('Create key named local audit?')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Confirm creation' }))
  expect(await screen.findByLabelText('Management key secret')).toHaveValue('synthetic-one-time-secret')
  expect(JSON.parse(writes[0]?.data as string)).toEqual({
    name: 'local audit', scopes: ['channels.read', 'tokens.read', 'logs.read', 'system.read'],
    channel_ids: [12, 9], token_ids: [3], expires_at: now + 7 * 86400,
  })
  expect(JSON.stringify(client.getQueryCache().getAll().map((query) => query.state.data))).not.toContain('synthetic-one-time-secret')
  vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValue(new Error('denied'))
  await user.click(screen.getByRole('button', { name: 'Copy management key' }))
  expect(await screen.findByText('Copy failed. Select and copy the management key manually.')).toBeInTheDocument()
  await user.click(within(screen.getByRole('dialog')).getByText('Close', { selector: 'button' }))
  await waitFor(() => expect(screen.queryByLabelText('Management key secret')).not.toBeInTheDocument())
  await openCreate(user)
  expect(screen.getByLabelText('Management key name')).toHaveValue('')
  expect(screen.queryByDisplayValue('synthetic-one-time-secret')).not.toBeInTheDocument()
})

it('rejects malformed resource IDs before confirmation and never sends a write', async () => {
  signIn()
  const writes: string[] = []
  const { user } = renderKeys(async (config) => {
    if (config.method !== 'get') writes.push(config.method ?? '')
    return response(config, { success: true, data: [] })
  })
  await openCreate(user)
  await user.type(screen.getByLabelText('Management key name'), 'local audit')
  await user.click(screen.getByText('Advanced resource limits'))
  await user.type(screen.getByLabelText('Channel IDs'), '1,,2')
  await user.click(screen.getByRole('button', { name: 'Review management key' }))
  expect(await screen.findByLabelText('Channel IDs')).toHaveAttribute('aria-invalid', 'true')
  expect(screen.queryByRole('button', { name: 'Confirm creation' })).not.toBeInTheDocument()
  expect(writes).toEqual([])
})

it('does not select or enable channel scopes for an admin whose channel permissions are denied', async () => {
  signIn(ROLE.ADMIN)
  useAuthStore.getState().auth.setUser({ id: 1, username: 'owner', role: ROLE.ADMIN, permissions: { admin_permissions: { channel: { read: false, operate: false } } } })
  const { user } = renderKeys(async (config) => response(config, { success: true, data: [] }))
  await reviewKey(user)
  await user.click(screen.getByRole('button', { name: 'Edit key details' }))
  expect(screen.getByRole('checkbox', { name: 'Read channels' })).not.toBeChecked()
  expect(screen.getByRole('checkbox', { name: 'Read channels' })).toHaveAttribute('aria-disabled', 'true')
  expect(screen.getByRole('checkbox', { name: 'Operate channels' })).toHaveAttribute('aria-disabled', 'true')
})

it.each(['session', 'permissions'] as const)('drops a late creation secret after a %s change and clears the old draft', async (change) => {
  signIn()
  let release!: () => void
  const held = new Promise<void>((resolve) => { release = resolve })
  let started!: () => void
  const requestStarted = new Promise<void>((resolve) => { started = resolve })
  const { user } = renderKeys(async (config) => {
    if (config.method === 'post') {
      started()
      await held
      return response(config, { success: true, data: { key: metadata, secret: 'late-previous-secret' } })
    }
    return response(config, { success: true, data: [] })
  })
  await reviewKey(user)
  await user.click(screen.getByRole('button', { name: 'Confirm creation' }))
  await requestStarted
  expect(screen.getByRole('button', { name: 'Creating...' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Close' })).toBeDisabled()
  act(() => {
    if (change === 'session') signIn(ROLE.SUPER_ADMIN, 'replacement-session', 2)
    else useAuthStore.getState().auth.setUser({ id: 1, username: 'owner', role: ROLE.SUPER_ADMIN, permissions: { admin_permissions: { channel: { read: false } } } })
  })
  await act(async () => { release(); await held })
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(screen.queryByDisplayValue('late-previous-secret')).not.toBeInTheDocument()
  await openCreate(user)
  expect(screen.getByLabelText('Management key name')).toHaveValue('')
})

it('reports an unknown creation outcome without replaying the write or offering a retry', async () => {
  signIn()
  let writes = 0
  const { user } = renderKeys(async (config) => {
    if (config.method === 'post') { writes++; throw new AxiosError('network unavailable', AxiosError.ERR_NETWORK, config) }
    return response(config, { success: true, data: [] })
  })
  await reviewKey(user)
  await user.click(screen.getByRole('button', { name: 'Confirm creation' }))
  expect(await screen.findByText('The result is unknown and the key may already exist. Close this dialog and inspect the key list before creating another key.')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Confirm creation' })).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Edit key details' })).toBeDisabled()
  expect(writes).toBe(1)
})

it('requires named revocation confirmation, restores focus on cancel, and displays the revoked key', async () => {
  signIn()
  let revoked = false
  let writes = 0
  const { user } = renderKeys(async (config) => {
    if (config.method === 'delete') { writes++; revoked = true; return response(config, { success: true }) }
    return response(config, { success: true, data: [{ ...metadata, revoked_at: revoked ? now : 0 }] })
  })
  const button = await screen.findByRole('button', { name: 'Revoke management key local audit' })
  await user.click(button)
  expect(writes).toBe(0)
  expect(within(screen.getByRole('alertdialog')).getByText('Revoke local audit (mk_audit) immediately? Tools using this key will lose access.')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  await waitFor(() => expect(button).toHaveFocus())
  await user.click(button)
  await user.click(screen.getByRole('button', { name: 'Confirm revocation' }))
  await waitFor(() => expect(screen.getByRole('button', { name: 'Revoke management key local audit' })).toBeDisabled())
  expect(screen.getAllByText('Revoked').length).toBeGreaterThan(0)
  expect(writes).toBe(1)
})

it('distinguishes loading, empty, expired, and revoked list entries', async () => {
  signIn()
  vi.spyOn(Date, 'now').mockReturnValue(now * 1000)
  let release!: () => void
  const held = new Promise<void>((resolve) => { release = resolve })
  let items: ManagementKeyMetadata[] = []
  const { user } = renderKeys(async (config) => { await held; return response(config, { success: true, data: items }) })
  expect(screen.getByRole('status')).toHaveTextContent('Loading management keys...')
  await act(async () => { release(); await held })
  expect(await screen.findByText('No management keys yet.')).toBeInTheDocument()
  items = [{ ...metadata, expires_at: now - 1 }, { ...metadata, id: 18, name: 'retired', revoked_at: now - 1 }]
  await user.click(screen.getByRole('button', { name: 'Refresh key list' }))
  expect(await screen.findByText('Expired')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Revoke management key retired' })).toBeDisabled()
})

it('shows a load error without presenting a failed list as empty', async () => {
  signIn()
  renderKeys(async (config) => { throw new AxiosError('offline', AxiosError.ERR_NETWORK, config) })
  expect(await screen.findByRole('alert')).toHaveTextContent('Management keys could not be loaded.')
  expect(screen.queryByText('No management keys yet.')).not.toBeInTheDocument()
})

it('keeps an unknown revocation disabled and refreshes metadata without repeating the write', async () => {
  signIn()
  let writes = 0
  const { user } = renderKeys(async (config) => {
    if (config.method === 'delete') { writes++; throw new AxiosError('offline', AxiosError.ERR_NETWORK, config) }
    return response(config, { success: true, data: [metadata] })
  })
  await user.click(await screen.findByRole('button', { name: 'Revoke management key local audit' }))
  await user.click(screen.getByRole('button', { name: 'Confirm revocation' }))
  expect(await screen.findByText('The revocation result is unknown and may already have taken effect. Refresh the list to inspect this key; this action will not be replayed.')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Revoke management key local audit' })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Refresh key list' }))
  expect(writes).toBe(1)
})

it('refreshes only reads after a committed creation loses its response and hides the unconfirmed empty list', async () => {
  signIn()
  let committed = false
  let writes = 0
  let releaseRead: (() => void) | undefined
  const { user } = renderKeys(async (config) => {
    if (config.method === 'post') {
      writes++
      committed = true
      throw new AxiosError('response lost after commit', AxiosError.ERR_NETWORK, config)
    }
    if (committed) {
      await new Promise<void>((resolve) => { releaseRead = resolve })
      return response(config, { success: true, data: [metadata] })
    }
    return response(config, { success: true, data: [] })
  })
  await reviewKey(user)
  await user.click(screen.getByRole('button', { name: 'Confirm creation' }))
  await screen.findByText('The result is unknown and the key may already exist. Close this dialog and inspect the key list before creating another key.')
  await user.click(within(screen.getByRole('dialog')).getByText('Close', { selector: 'button' }))
  expect(screen.queryByText('No management keys yet.')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Create management key' })).toBeDisabled()
  await waitFor(() => expect(releaseRead).toBeTypeOf('function'))
  await act(async () => { releaseRead?.() })
  expect(await screen.findByRole('button', { name: 'Revoke management key local audit' })).toBeEnabled()
  expect(screen.queryByLabelText('Management key secret')).not.toBeInTheDocument()
  expect(writes).toBe(1)
})

it('keeps long object scopes compact until explicitly expanded, without losing any IDs', async () => {
  signIn()
  const { user } = renderKeys(async (config) => response(config, { success: true, data: [{ ...metadata, channel_ids: [11, 22, 33, 44, 55], token_ids: [61, 62, 63, 64] }] }))
  const channels = await screen.findByRole('button', { name: 'Channel IDs (5)' })
  const tokens = screen.getByRole('button', { name: 'API token IDs (4)' })
  expect(channels).toHaveAttribute('aria-expanded', 'false')
  expect(tokens).toHaveAttribute('aria-expanded', 'false')
  expect(screen.getByText('11, 22, 33, 44, 55')).not.toBeVisible()
  await user.click(channels)
  expect(screen.getByText('11, 22, 33, 44, 55')).toBeVisible()
  await user.click(tokens)
  expect(screen.getByText('61, 62, 63, 64')).toBeVisible()
})

it('offers Root operations permissions without adding them to the existing default selection', async () => {
  signIn()
  const { user } = renderKeys(async (config) => response(config, { success: true, data: [] }))
  await openCreate(user)
  expect(screen.getByRole('checkbox', { name: 'Read upgrade plans' })).not.toBeChecked()
  expect(screen.getByRole('checkbox', { name: 'Prepare upgrades' })).not.toBeChecked()
  expect(screen.getByRole('checkbox', { name: 'Execute approved upgrades' })).not.toBeChecked()
  expect(screen.getAllByRole('checkbox').filter((checkbox) => checkbox.getAttribute('aria-checked') === 'true').map((checkbox) => checkbox.id)).toEqual([
    'management-channels.read', 'management-tokens.read', 'management-logs.read', 'management-system.read',
  ])
})

it('does not offer operations scopes to Admin when creating a management key', async () => {
  signIn(ROLE.ADMIN)
  const { user } = renderKeys(async (config) => response(config, { success: true, data: [] }))
  await openCreate(user)
  expect(screen.queryByRole('checkbox', { name: 'Read upgrade plans' })).not.toBeInTheDocument()
  expect(screen.queryByRole('checkbox', { name: 'Prepare upgrades' })).not.toBeInTheDocument()
  expect(screen.queryByRole('checkbox', { name: 'Execute approved upgrades' })).not.toBeInTheDocument()
})

it('rejects operations scopes combined with resource limits without silently erasing either choice', async () => {
  signIn()
  const writes: InternalAxiosRequestConfig[] = []
  const { user } = renderKeys(async (config) => {
    if (config.method === 'post') writes.push(config)
    return response(config, { success: true, data: [] })
  })
  await openCreate(user)
  await user.type(screen.getByLabelText('Management key name'), 'upgrade operator')
  await user.click(screen.getByRole('checkbox', { name: 'Prepare upgrades' }))
  await user.click(screen.getByText('Advanced resource limits'))
  await user.type(screen.getByLabelText('Channel IDs'), '17')
  await user.click(screen.getByRole('button', { name: 'Review management key' }))
  expect(await screen.findByText('Operations permissions cannot be combined with resource ID limits.')).toBeInTheDocument()
  expect(screen.getByLabelText('Channel IDs')).toHaveValue('17')
  expect(screen.getByRole('checkbox', { name: 'Prepare upgrades' })).toBeChecked()
  expect(screen.queryByRole('button', { name: 'Confirm creation' })).not.toBeInTheDocument()
  expect(writes).toHaveLength(0)
})
