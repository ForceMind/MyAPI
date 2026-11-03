/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient } from '@tanstack/react-query'
import { act, cleanup, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { Channels } from '../index'

vi.mock('@/hooks/use-sidebar-config', () => ({ useIsSidebarModuleVisible: () => true }))

const originalAuth = useAuthStore.getState()
let client: QueryClient

async function renderChannels() {
  return renderListRoute({ client, path: '/channels/', entry: '/channels/#channel-routing', element: <Channels /> })
}

function previewReads() {
  return vi.mocked(api.get).mock.calls.filter(([url]) => url === '/api/channel/routing-preview')
}

describe('channel routing preview permission boundary', () => {
  beforeEach(() => {
    localStorage.clear()
    client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      const path = url.split('?')[0]
      if (path === '/api/status') return { data: { success: true, data: {} } }
      if (path === '/api/user/2fa/status' || path === '/api/user/passkey') return { data: { success: true, data: { enabled: false } } }
      if (path === '/api/channel/ops') return { data: { success: true, data: { retry_times: 0 } } }
      if (path === '/api/group/') return { data: { success: true, data: ['default'] } }
      throw new Error(`Unexpected permission-boundary GET ${path}`)
    })
    vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected permission-boundary PUT'))
    vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected permission-boundary POST'))
    vi.spyOn(api, 'delete').mockRejectedValue(new Error('Unexpected permission-boundary DELETE'))
  })
  afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); localStorage.clear(); vi.restoreAllMocks() })

  test('does not mount or query the routing preview after channel read is revoked', async () => {
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'admin',
      role: ROLE.ADMIN,
      permissions: { admin_permissions: { channel: { read: false } } },
    })

    await renderChannels()

    expect(screen.queryByText('Routing Preview')).not.toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'Routing Preview' })).not.toBeInTheDocument()
    expect(previewReads()).toEqual([])
    expect(api.put).not.toHaveBeenCalled()
    expect(api.post).not.toHaveBeenCalled()
    expect(api.delete).not.toHaveBeenCalled()
  })

  test('mounts the routing preview for the root role', async () => {
    useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })

    await renderChannels()

    expect(screen.getByRole('region', { name: 'Routing Preview' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Preview routing' })).toBeInTheDocument()
    expect(previewReads()).toEqual([])

    act(() => { useAuthStore.getState().auth.setUser({ id: 1, username: 'admin', role: ROLE.ADMIN, permissions: { admin_permissions: { channel: { read: false } } } }) })
    await waitFor(() => expect(screen.queryByText('Routing Preview')).not.toBeInTheDocument())
    expect(screen.queryByRole('button', { name: 'Preview routing' })).not.toBeInTheDocument()
    expect(previewReads()).toEqual([])
    expect(api.put).not.toHaveBeenCalled()
    expect(api.post).not.toHaveBeenCalled()
    expect(api.delete).not.toHaveBeenCalled()
  })
})
