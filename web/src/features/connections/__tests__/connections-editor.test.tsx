/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AxiosAdapter } from 'axios'
import { useState, type AnchorHTMLAttributes } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import channel from '../../../../../tools/ui/connection-editor-fixture.json'
import type { ConnectionsSearch } from '../connection-state'
import { ConnectionsWorkspace } from '../connections'

// Navigation is outside this test. Keep the real workspace, provider, drawer,
// form transform, query hooks and cancellation behavior, unlike the unit mock.
vi.mock('@tanstack/react-router', () => ({
  getRouteApi: () => ({}),
  Link: ({ children }: AnchorHTMLAttributes<HTMLAnchorElement>) => (
    <a href='/synthetic'>{children}</a>
  ),
}))
vi.mock('@/hooks/use-sidebar-config', () => ({
  useIsSidebarModuleVisible: () => true,
}))

const originalAdapter = api.defaults.adapter
const originalMatchMedia = Object.getOwnPropertyDescriptor(
  window,
  'matchMedia'
) ?? { configurable: true, value: window.matchMedia }
let teardown: (() => void) | undefined

afterEach(() => {
  teardown?.()
  api.defaults.adapter = originalAdapter
  Object.defineProperty(window, 'matchMedia', originalMatchMedia)
  vi.restoreAllMocks()
})

test.each([false, true])(
  'real connection editor loads its native DTO and cancels without writes (mobile=%s)',
  async (mobile) => {
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: vi.fn().mockReturnValue({
        matches: mobile,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }),
    })
    const user = {
      id: 1,
      username: 'synthetic-operator',
      role: ROLE.SUPER_ADMIN,
      permissions: {
        admin_permissions: {
          channel: { read: true, write: true, sensitive_write: true },
        },
      },
    }
    useAuthStore.getState().auth.setBundle({
      access_token: 'synthetic-editor-access',
      token_type: 'Bearer',
      access_expires_at: 4_000_000_000,
      user,
      session: {
        sid: 'synthetic-editor-session',
        current: true,
        login_method: 'synthetic',
        ip: '127.0.0.1',
        user_agent: 'test',
        created_at: 1,
        last_active_at: 1,
        expires_at: 4_000_000_000,
      },
    })
    const requests: { method: string | undefined; path: string }[] = []
    const adapter: AxiosAdapter = async (config) => {
      const path = config.url ?? ''
      requests.push({ method: config.method, path })
      expect(config.method).toBe('get')
      const reads: Record<string, unknown> = {
        '/api/user/self': user,
        '/api/channel': { total: 1, items: [channel] },
        '/api/channel/search': { total: 1, items: [channel] },
        '/api/channel/1': channel,
        '/api/channel/quota/changes': { items: [] },
        '/api/group': ['default'],
        '/api/channel/models': [{ id: 'synthetic-text-model' }],
        '/api/prefill_group': [],
        '/api/user/2fa/status': { enabled: false, locked: false },
        '/api/user/passkey': { enabled: false, credentials: [] },
      }
      expect(Object.hasOwn(reads, path), `Unexpected request: ${path}`).toBe(
        true
      )
      return {
        config,
        status: 200,
        statusText: 'OK',
        headers: {},
        data: { success: true, data: reads[path] },
      }
    }
    api.defaults.adapter = adapter
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const initialSearch: ConnectionsSearch = {
      connection: channel.id,
      filter: 'Synthetic',
      page: 2,
      status: 'enabled',
    }
    let latestSearch = initialSearch
    function Harness() {
      const [search, setSearch] = useState(latestSearch)
      latestSearch = search
      return <ConnectionsWorkspace search={search} onSearch={setSearch} />
    }
    const view = render(
      <QueryClientProvider client={client}>
        <Harness />
      </QueryClientProvider>
    )
    teardown = () => {
      view.unmount()
      client.clear()
    }
    const interaction = userEvent.setup()
    await screen.findByRole('heading', { name: channel.name })
    const edit = screen.getByRole('button', { name: 'Edit connection' })
    await interaction.click(edit)
    const dialog = await screen.findByRole('dialog')
    expect(
      within(dialog).getByText('Edit Channel', { exact: true })
    ).toBeVisible()
    const name = await within(dialog).findByDisplayValue(channel.name)
    await interaction.clear(name)
    await interaction.paste('Unsaved synthetic name')
    expect(name).toHaveValue('Unsaved synthetic name')
    await interaction.keyboard('{Escape}')
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    expect(latestSearch).toEqual(initialSearch)
    await waitFor(() => expect(edit).toHaveFocus())
    await interaction.click(edit)
    expect(
      await within(await screen.findByRole('dialog')).findByDisplayValue(
        channel.name
      )
    ).toBeVisible()
    await interaction.keyboard('{Escape}')
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    expect(requests.every((request) => request.method === 'get')).toBe(true)
    expect(
      requests.some((request) => request.path === '/api/channel/models')
    ).toBe(true)
    expect(
      requests.some(
        (request) =>
          /key|test|balance|fetch_models/.test(request.path) &&
          !request.path.endsWith('/passkey')
      )
    ).toBe(false)
  },
  // Two real heavy forms are mounted per case; keep a bounded integration
  // budget under the full suite rather than relying on the 5s unit default.
  15_000
)
