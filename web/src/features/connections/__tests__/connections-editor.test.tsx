/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import channel from '../../../../../tools/ui/connection-editor-fixture.json'
import { mountConnections } from './connections-page-fixture'

// Keep the real connection page, provider, drawer, form transform, query hooks
// and cancellation behavior, unlike the unit mock.
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
  useAuthStore.getState().auth.reset('idle')
  vi.restoreAllMocks()
})

test.each([false, true])(
  'real connection editor loads its native DTO and cancels without writes (mobile=%s)',
  async (mobile) => {
    const fixture = await mountConnections({
      mobile,
      entry: `/connections/${channel.id}?connectionReturn=${encodeURIComponent(
        JSON.stringify({ filter: 'Synthetic', page: 2, status: 'enabled' })
      )}`,
      items: [channel],
      extraReads: {
        '/api/group': ['default'],
        '/api/channel/models': [{ id: 'synthetic-text-model' }],
        '/api/prefill_group': [],
        '/api/user/2fa/status': { enabled: false, locked: false },
        '/api/user/passkey': { enabled: false, credentials: [] },
      },
    })
    teardown = fixture.teardown
    // A root operator with sensitive write, as the editor requires.
    useAuthStore.getState().auth.setUser({
      ...fixture.user,
      role: ROLE.SUPER_ADMIN,
      permissions: {
        admin_permissions: {
          channel: { read: true, write: true, sensitive_write: true },
        },
      },
    })
    const initialLocation = fixture.router.state.location.href
    const interaction = userEvent.setup()
    await screen.findByRole('heading', { level: 1, name: channel.name })
    const edit = await screen.findByRole('button', { name: 'Edit connection' })
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
    expect(fixture.router.state.location.href).toBe(initialLocation)
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Edit connection' })
      ).toHaveFocus()
    )
    await interaction.click(
      screen.getByRole('button', { name: 'Edit connection' })
    )
    expect(
      await within(await screen.findByRole('dialog')).findByDisplayValue(
        channel.name
      )
    ).toBeVisible()
    await interaction.keyboard('{Escape}')
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    expect(fixture.requests.every((request) => request.method === 'get')).toBe(
      true
    )
    expect(
      fixture.requests.some((request) => request.path === '/api/channel/models')
    ).toBe(true)
    expect(
      fixture.requests.some(
        (request) =>
          /key|test|balance|fetch_models/.test(request.path) &&
          !request.path.endsWith('/passkey') &&
          request.path !== '/pg/keys'
      )
    ).toBe(false)
  },
  // Two real heavy forms are mounted per case; keep a bounded integration
  // budget under the full suite rather than relying on the 5s unit default.
  15_000
)
