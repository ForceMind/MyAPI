/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import type { SystemStatus } from '@/features/auth/types'
import { PRODUCT_CAPABILITIES } from '@/lib/product-edition'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { AuthenticatedLayout } from '../authenticated-layout'
import { renderShellRoute } from './shell-test-utils'

function Workspace() {
  return (
    <AuthenticatedLayout>
      <main id='content' tabIndex={-1}>
        Page content
      </main>
    </AuthenticatedLayout>
  )
}

function setViewport(width: number) {
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => {
    const max = /max-width:\s*(\d+)px/.exec(query)
    const min = /min-width:\s*(\d+)px/.exec(query)
    let matches = query.includes('prefers-reduced-motion')
    if (max) matches = width <= Number(max[1])
    if (min) matches = width >= Number(min[1])
    return {
      matches,
      media: query,
      onchange: null,
      addListener: () => undefined,
      removeListener: () => undefined,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      dispatchEvent: () => false,
    }
  })
}

const enabledFunding: SystemStatus = {
  user_funding_mode: 'enabled',
  user_funding_capabilities: {
    mode: 'enabled',
    epoch: 1,
    ready: true,
    can_top_up: true,
    can_redeem: true,
    can_transfer_affiliate_rewards: true,
    can_purchase_subscription: true,
    can_view_funding_history: true,
  },
}

function editionStatus(edition: 'full' | 'lite' | 'missing'): SystemStatus {
  if (edition === 'missing') return enabledFunding
  return {
    ...enabledFunding,
    product_edition: edition,
    capabilities: Object.fromEntries(
      PRODUCT_CAPABILITIES.map((capability) => [capability, edition === 'full'])
    ),
  }
}

const originalGetAnimations = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)

beforeEach(() => {
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
  localStorage.clear()
  setViewport(1440)
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'operator', role: ROLE.SUPER_ADMIN })
})

afterEach(() => {
  if (originalGetAnimations) {
    Object.defineProperty(
      HTMLElement.prototype,
      'getAnimations',
      originalGetAnimations
    )
  } else Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
  useAuthStore.getState().auth.reset('idle')
  localStorage.clear()
})

function topNavigation() {
  return screen.getByRole('navigation', {
    name: 'Personal console navigation',
  })
}

function topLinks() {
  return within(topNavigation())
    .getAllByRole('link')
    .map((link) => [link.textContent, link.getAttribute('href')])
}

async function openMenu(name: string) {
  const user = userEvent.setup()
  const trigger = within(topNavigation()).getByRole('button', { name })
  await user.click(trigger)
  return screen.findByRole('menu')
}

describe('PC top bar', () => {
  test('a root operator gets the five primary areas, Admin and More, and no sidebar', async () => {
    await renderShellRoute(<Workspace />, { status: editionStatus('full') })
    expect(topLinks()).toEqual([
      ['Overview', '/dashboard/overview'],
      ['Upstream', '/connections'],
      ['API Keys', '/keys'],
      ['Requests', '/usage-logs/common'],
      ['Settings', '/system-settings/site'],
    ])
    expect(
      within(topNavigation())
        .getAllByRole('button')
        .map((button) => button.textContent)
    ).toEqual(['Admin', 'More'])
    expect(screen.queryByRole('complementary')).not.toBeInTheDocument()
    expect(
      document.querySelector('.pc-sidebar, [data-myapi-sidebar]')
    ).toBeNull()
    expect(
      within(topNavigation()).getByRole('link', { name: 'Overview' })
    ).toHaveAttribute('aria-current', 'page')
  })

  test.each(['full', 'missing'] as const)(
    'a %s edition status offers users, commerce and system under Admin',
    async (edition) => {
      await renderShellRoute(<Workspace />, { status: editionStatus(edition) })
      const menu = await openMenu('Admin')
      expect(
        within(menu)
          .getAllByRole('menuitem')
          .map((item) => [item.textContent, item.getAttribute('href')])
      ).toEqual([
        ['Users', '/users'],
        ['Wallet', '/wallet'],
        ['Subscriptions', '/subscriptions'],
        ['Redemption Codes', '/redemption-codes'],
        ['System status', '/system-info'],
      ])
    }
  )

  test('a Lite status has no Admin menu, hides capability entries and keeps every route reachable', async () => {
    await renderShellRoute(<Workspace />, {
      path: '/usage-logs/common',
      status: editionStatus('lite'),
    })
    expect(topLinks()).toEqual([
      ['Overview', '/dashboard/overview'],
      ['Upstream', '/connections'],
      ['API Keys', '/keys'],
      ['Requests', '/usage-logs/common'],
      ['Settings', '/system-settings/site'],
    ])
    expect(
      within(topNavigation()).queryByRole('button', { name: 'Admin' })
    ).not.toBeInTheDocument()
    const tabs = screen.getByRole('navigation', { name: 'Requests' })
    expect(
      within(tabs).queryByRole('link', { name: 'Drawing' })
    ).not.toBeInTheDocument()
    const menu = await openMenu('More')
    const hrefs = within(menu)
      .getAllByRole('menuitem')
      .map((item) => item.getAttribute('href'))
    expect(hrefs).toEqual(
      expect.arrayContaining(['/users', '/system-info', '/playground'])
    )
    for (const href of ['/wallet', '/subscriptions', '/redemption-codes']) {
      expect(hrefs).not.toContain(href)
    }
  })

  test('an ordinary user sees their own areas without upstream, settings or Admin', async () => {
    useAuthStore
      .getState()
      .auth.setUser({ id: 2, username: 'member', role: ROLE.USER })
    await renderShellRoute(<Workspace />, { status: editionStatus('full') })
    expect(topLinks()).toEqual([
      ['Overview', '/dashboard/overview'],
      ['API Keys', '/keys'],
      ['Requests', '/usage-logs/common'],
    ])
    expect(
      within(topNavigation()).queryByRole('button', { name: 'Admin' })
    ).not.toBeInTheDocument()
    const menu = await openMenu('More')
    const hrefs = within(menu)
      .getAllByRole('menuitem')
      .map((item) => item.getAttribute('href'))
    for (const href of ['/users', '/system-info', '/subscriptions']) {
      expect(hrefs).not.toContain(href)
    }
  })

  test('an administrator without root sees Users under Admin but no system entries', async () => {
    useAuthStore.getState().auth.setUser({
      id: 3,
      username: 'admin',
      role: ROLE.ADMIN,
      permissions: { admin_permissions: { channel: { read: true } } },
    })
    await renderShellRoute(<Workspace />, { status: editionStatus('full') })
    expect(
      within(topNavigation()).queryByRole('link', { name: 'Settings' })
    ).not.toBeInTheDocument()
    const menu = await openMenu('Admin')
    const names = within(menu)
      .getAllByRole('menuitem')
      .map((item) => item.textContent)
    expect(names).toContain('Users')
    expect(names).not.toContain('System status')
  })

  test('under page tabs a two-level page has no breadcrumb and a three-level page keeps it', async () => {
    const view = await renderShellRoute(<Workspace />, {
      path: '/connections',
    })
    expect(screen.getByRole('navigation', { name: 'Upstream' })).toBeVisible()
    expect(
      screen.queryByRole('navigation', { name: 'Current page' })
    ).not.toBeInTheDocument()
    view.unmount()
    await renderShellRoute(<Workspace />, { path: '/dashboard/flow' })
    expect(
      within(screen.getByRole('navigation', { name: 'Current page' }))
        .getAllByRole('listitem')
        .map((item) => item.textContent)
    ).toEqual(['Requests', 'Analytics', 'Flow'])
  })

  test('the Admin menu works by keyboard and returns focus to its trigger', async () => {
    const user = userEvent.setup()
    const { router } = await renderShellRoute(<Workspace />, {
      status: editionStatus('full'),
    })
    const trigger = within(topNavigation()).getByRole('button', {
      name: 'Admin',
    })
    trigger.focus()
    await user.keyboard('{Enter}')
    const menu = await screen.findByRole('menu')
    expect(trigger).toHaveAttribute('aria-expanded', 'true')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(menu).not.toBeInTheDocument())
    expect(trigger).toHaveFocus()

    // Opening by keyboard highlights the first item; arrows move through
    // the rest.
    await user.keyboard('{Enter}')
    await screen.findByRole('menu')
    const users = screen.getByRole('menuitem', { name: 'Users' })
    await waitFor(() => expect(users).toHaveFocus())
    await user.keyboard('{ArrowDown}')
    await waitFor(() =>
      expect(screen.getByRole('menuitem', { name: 'Wallet' })).toHaveFocus()
    )
    await user.keyboard('{ArrowUp}')
    await waitFor(() => expect(users).toHaveFocus())
    await user.keyboard('{Enter}')
    await waitFor(() => expect(router.state.location.pathname).toBe('/users'))
  })
})

describe('mobile shell', () => {
  test('a root operator gets Overview, Upstream, Key, Requests and More', async () => {
    setViewport(390)
    await renderShellRoute(<Workspace />)
    const bottom = screen.getByRole('navigation', {
      name: 'Mobile console navigation',
    })
    expect(
      within(bottom)
        .getAllByRole('link')
        .map((link) => [link.textContent, link.getAttribute('href')])
    ).toEqual([
      ['Overview', '/dashboard/overview'],
      ['Upstream', '/connections'],
      ['Keys', '/keys'],
      ['Requests', '/usage-logs/common'],
    ])
    expect(within(bottom).getByRole('button', { name: 'More' })).toBeVisible()
    expect(
      screen.queryByRole('navigation', { name: 'Personal console navigation' })
    ).not.toBeInTheDocument()
  })

  test('More holds settings, the admin areas and tools, and returns focus when closed', async () => {
    setViewport(390)
    const user = userEvent.setup()
    await renderShellRoute(<Workspace />, { status: editionStatus('full') })
    const more = screen.getByRole('button', { name: 'More' })
    await user.click(more)
    const sheet = await screen.findByRole('dialog', { name: 'All modules' })
    const hrefs = within(sheet)
      .getAllByRole('link')
      .map((link) => link.getAttribute('href'))
    for (const href of [
      '/system-settings/site',
      '/users',
      '/system-info',
      '/playground',
      '/profile',
    ]) {
      expect(hrefs).toContain(href)
    }
    await user.keyboard('{Escape}')
    await waitFor(() =>
      expect(
        screen.queryByRole('dialog', { name: 'All modules' })
      ).not.toBeInTheDocument()
    )
    expect(more).toHaveFocus()
  })
})
