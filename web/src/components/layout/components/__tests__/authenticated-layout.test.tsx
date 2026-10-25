/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { act, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { DirectionProvider } from '@/context/direction-provider'
import { ThemeProvider } from '@/context/theme-provider'
import type { SystemStatus } from '@/features/auth/types'
import { useStatus } from '@/hooks/use-status'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { AuthenticatedLayout } from '../authenticated-layout'
import { renderShellRoute } from './shell-test-utils'

beforeEach(() => {
  // A wide desktop window, where the top bar carries the navigation.
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
    matches: query.includes('min-width'),
    media: query,
    onchange: null,
    addListener: () => undefined,
    removeListener: () => undefined,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    dispatchEvent: () => false,
  }))
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'synthetic-maintainer',
    role: ROLE.SUPER_ADMIN,
  })
})
afterEach(() => useAuthStore.getState().auth.reset('idle'))

function fundingStatus(
  mode: 'enabled' | 'disabled' | 'retirement'
): SystemStatus {
  return {
    user_funding_mode: mode,
    user_funding_capabilities: {
      mode,
      epoch: 1,
      ready: true,
      can_top_up: mode === 'enabled',
      can_redeem: mode === 'enabled',
      can_transfer_affiliate_rewards: mode === 'enabled',
      can_purchase_subscription: mode === 'enabled',
      can_view_funding_history: true,
    },
  }
}

function RequestFilterPage() {
  const { status } = useStatus()
  return (
    <main id='content'>
      <output aria-label='Funding snapshot'>
        {status?.user_funding_mode ?? 'unconfirmed'}
      </output>
      <label htmlFor='request-filter'>Request filter</label>
      <input id='request-filter' />
    </main>
  )
}

test.each(['enabled', 'retirement', 'unconfirmed'] as const)(
  'a funding change to %s preserves the open page and complete platform navigation',
  async (mode) => {
    const { queryClient } = await renderShellRoute(
      <DirectionProvider>
        <ThemeProvider>
          <AuthenticatedLayout>
            <RequestFilterPage />
          </AuthenticatedLayout>
        </ThemeProvider>
      </DirectionProvider>,
      { status: fundingStatus('disabled') }
    )
    await userEvent
      .setup()
      .type(screen.getByLabelText('Request filter'), 'request-42')
    await act(async () => {
      queryClient.setQueryData(
        ['status'],
        mode === 'unconfirmed' ? {} : fundingStatus(mode)
      )
    })
    await screen.findByText(mode)
    expect(screen.getByLabelText('Request filter')).toHaveValue('request-42')
    const navigation = screen.getByRole('navigation', {
      name: 'Personal console navigation',
    })
    const user = userEvent.setup()
    await user.click(within(navigation).getByRole('button', { name: 'Admin' }))
    const menu = await screen.findByRole('menu')
    expect(
      within(menu).getByRole('menuitem', { name: 'System status' })
    ).toHaveAttribute('href', '/system-info')
    const wallet = within(menu).queryByRole('menuitem', { name: 'Wallet' })
    if (mode === 'enabled') expect(wallet).toHaveAttribute('href', '/wallet')
    else expect(wallet).not.toBeInTheDocument()
    await user.keyboard('{Escape}')
    expect(screen.getByLabelText('Request filter')).toHaveValue('request-42')
    queryClient.clear()
  }
)

function useMobileViewport() {
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
    matches: query.includes('max-width'),
    media: query,
    onchange: null,
    addListener: () => undefined,
    removeListener: () => undefined,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    dispatchEvent: () => false,
  }))
}

test('a Root settings page exposes existing setting sections on desktop', async () => {
  const { queryClient } = await renderShellRoute(
    <DirectionProvider>
      <ThemeProvider>
        <AuthenticatedLayout>
          <main id='content'>Settings</main>
        </AuthenticatedLayout>
      </ThemeProvider>
    </DirectionProvider>,
    {
      path: '/system-settings/site/system-info',
      status: fundingStatus('disabled'),
    }
  )
  const pages = screen.getByRole('navigation', { name: 'Settings' })
  await userEvent
    .setup()
    .click(within(pages).getByRole('link', { name: 'Operations' }))
  expect(
    await within(pages).findByRole('link', { name: 'Log Maintenance' })
  ).toHaveAttribute('href', '/system-settings/operations/logs')
  queryClient.clear()
})

test('a Root settings page exposes existing setting sections in mobile More', async () => {
  useMobileViewport()
  const { queryClient } = await renderShellRoute(
    <DirectionProvider>
      <ThemeProvider>
        <AuthenticatedLayout>
          <main id='content'>Settings</main>
        </AuthenticatedLayout>
      </ThemeProvider>
    </DirectionProvider>,
    {
      path: '/system-settings/site/system-info',
      status: fundingStatus('disabled'),
    }
  )
  expect(
    within(
      screen.getByRole('navigation', { name: 'Mobile console navigation' })
    ).getByRole('link', { name: 'Requests' })
  ).toHaveAttribute('href', '/usage-logs/common')
  await userEvent.setup().click(screen.getByRole('button', { name: 'More' }))
  const dialog = await screen.findByRole('dialog', { name: 'All modules' })
  expect(
    within(dialog).getByRole('link', { name: 'SSRF Protection' })
  ).toHaveAttribute('href', '/system-settings/security/ssrf')
  queryClient.clear()
})
