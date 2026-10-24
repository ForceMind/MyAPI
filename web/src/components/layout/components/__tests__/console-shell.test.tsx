import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { DirectionProvider } from '@/context/direction-provider'
import { ThemeProvider } from '@/context/theme-provider'
import type { SystemStatus } from '@/features/auth/types'
import { PRODUCT_CAPABILITIES } from '@/lib/product-edition'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'
import { useInspectorStore } from '@/stores/inspector-store'

import { AuthenticatedLayout } from '../authenticated-layout'
import { renderShellRoute } from './shell-test-utils'

function Workspace() {
  return (
    <DirectionProvider>
      <ThemeProvider>
        <AuthenticatedLayout>
          <main id='content' tabIndex={-1}>
            Page content
          </main>
        </AuthenticatedLayout>
      </ThemeProvider>
    </DirectionProvider>
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
  document.cookie = 'sidebar_state=; max-age=0; path=/'
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
  document.cookie = 'sidebar_state=; max-age=0; path=/'
  localStorage.clear()
})

function desktopNavigation() {
  return screen.getByRole('navigation', {
    name: 'Personal console navigation',
  })
}

describe('edition capabilities shape the navigation', () => {
  test.each(['full', 'missing'] as const)(
    'a %s edition status keeps every optional entry',
    async (edition) => {
      const user = userEvent.setup()
      await renderShellRoute(<Workspace />, { status: editionStatus(edition) })
      const navigation = desktopNavigation()
      expect(
        within(navigation).getByRole('link', { name: 'Drawing' })
      ).toHaveAttribute('href', '/usage-logs/drawing')
      await user.click(
        within(navigation).getByRole('button', { name: 'Commerce' })
      )
      for (const [name, href] of [
        ['Wallet', '/wallet'],
        ['Subscriptions', '/subscriptions'],
        ['Redemption Codes', '/redemption-codes'],
      ]) {
        expect(within(navigation).getByRole('link', { name })).toHaveAttribute(
          'href',
          href
        )
      }
    }
  )

  test('a Lite status hides entries whose capability is false and keeps the core workflow', async () => {
    await renderShellRoute(<Workspace />, { status: editionStatus('lite') })
    const navigation = desktopNavigation()
    expect(
      within(navigation).queryByRole('link', { name: 'Drawing' })
    ).not.toBeInTheDocument()
    expect(
      within(navigation).queryByRole('button', { name: 'Commerce' })
    ).not.toBeInTheDocument()
    for (const [name, href] of [
      ['Connections', '/connections'],
      ['Account windows', '/upstream-accounts'],
      ['API Keys', '/keys'],
      ['Requests', '/usage-logs/common'],
    ]) {
      expect(within(navigation).getByRole('link', { name })).toHaveAttribute(
        'href',
        href
      )
    }
  })
})

describe('roles shape the navigation', () => {
  test('an ordinary user sees daily entries without upstream, user or system administration', async () => {
    useAuthStore
      .getState()
      .auth.setUser({ id: 2, username: 'member', role: ROLE.USER })
    await renderShellRoute(<Workspace />)
    const navigation = desktopNavigation()
    expect(
      within(navigation).getByRole('link', { name: 'API Keys' })
    ).toBeInTheDocument()
    expect(
      within(navigation).getByRole('link', { name: 'Requests' })
    ).toBeInTheDocument()
    for (const group of ['Upstream', 'System']) {
      expect(
        within(navigation).queryByRole('group', { name: group })
      ).not.toBeInTheDocument()
    }
    expect(
      within(navigation).queryByRole('link', { name: 'Users' })
    ).not.toBeInTheDocument()
  })

  test('an administrator sees upstream and users but not root-only system entries', async () => {
    useAuthStore.getState().auth.setUser({
      id: 3,
      username: 'admin',
      role: ROLE.ADMIN,
      permissions: { admin_permissions: { channel: { read: true } } },
    })
    await renderShellRoute(<Workspace />)
    const navigation = desktopNavigation()
    const upstream = within(navigation).getByRole('group', { name: 'Upstream' })
    expect(
      within(upstream)
        .getAllByRole('link')
        .map((link) => link.textContent)
    ).toEqual(['Connections', 'Account windows', 'Model catalog'])
    expect(
      within(navigation).getByRole('link', { name: 'Users' })
    ).toHaveAttribute('href', '/users')
    expect(
      within(navigation).queryByRole('group', { name: 'System' })
    ).not.toBeInTheDocument()
  })

  test('the channel table view is reached through the single Connections entry', async () => {
    await renderShellRoute(<Workspace />, { path: '/channels' })
    const upstream = within(desktopNavigation()).getByRole('group', {
      name: 'Upstream',
    })
    expect(
      within(upstream).getByRole('link', { name: 'Connections' })
    ).toHaveAttribute('aria-current', 'page')
    expect(
      within(upstream).getByRole('link', { name: 'Account windows' })
    ).not.toHaveAttribute('aria-current')
  })
})

describe('desktop sidebar width defaults', () => {
  test.each([
    [1100, 'true'],
    [1440, 'false'],
  ])(
    'without a saved preference a %ipx window starts collapsed=%s',
    async (width, collapsed) => {
      setViewport(width)
      const { container } = await renderShellRoute(<Workspace />)
      expect(
        container.querySelector('[data-myapi-shell="authenticated"]')
      ).toHaveAttribute('data-collapsed', collapsed)
    }
  )
})

describe('mobile composition', () => {
  test('an operator gets four destinations plus More and no desktop sidebar', async () => {
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
      ['Records', '/usage-logs/common'],
    ])
    expect(within(bottom).getByRole('button', { name: 'More' })).toBeVisible()
    expect(
      screen.queryByRole('navigation', { name: 'Personal console navigation' })
    ).not.toBeInTheDocument()
  })

  test('an ordinary user does not get an upstream destination', async () => {
    setViewport(390)
    useAuthStore
      .getState()
      .auth.setUser({ id: 2, username: 'member', role: ROLE.USER })
    await renderShellRoute(<Workspace />)
    const bottom = screen.getByRole('navigation', {
      name: 'Mobile console navigation',
    })
    expect(
      within(bottom)
        .getAllByRole('link')
        .map((link) => link.textContent)
    ).toEqual(['Overview', 'Keys', 'Records'])
  })

  test('More lists only the remaining entries and narrows them by search', async () => {
    setViewport(390)
    const user = userEvent.setup()
    await renderShellRoute(<Workspace />)
    await user.click(screen.getByRole('button', { name: 'More' }))
    const more = await screen.findByRole('dialog', { name: 'All modules' })
    const hrefs = within(more)
      .getAllByRole('link')
      .map((link) => link.getAttribute('href'))
    for (const primary of [
      '/dashboard/overview',
      '/connections',
      '/keys',
      '/usage-logs/common',
    ]) {
      expect(hrefs).not.toContain(primary)
    }
    for (const href of [
      '/upstream-accounts',
      '/users',
      '/full-content-logs',
      '/profile',
    ]) {
      expect(hrefs).toContain(href)
    }
    await user.type(
      within(more).getByRole('searchbox', { name: 'Search' }),
      'Users'
    )
    expect(
      within(more)
        .getAllByRole('link')
        .map((link) => link.textContent)
    ).toEqual(['Users'])
  })

  test('a page outside the main destinations shows a back control and its title', async () => {
    setViewport(390)
    await renderShellRoute(<Workspace />, { path: '/profile' })
    const header = screen.getByRole('banner')
    expect(within(header).getByRole('button', { name: 'Back' })).toBeVisible()
    expect(within(header).getByText('Profile')).toHaveAttribute(
      'title',
      'Profile'
    )
  })

  test('a main destination shows the brand mark instead of a back control', async () => {
    setViewport(390)
    await renderShellRoute(<Workspace />, { path: '/keys' })
    const header = screen.getByRole('banner')
    expect(
      within(header).queryByRole('button', { name: 'Back' })
    ).not.toBeInTheDocument()
    expect(within(header).getByText('API Keys')).toHaveAttribute(
      'title',
      'API Keys'
    )
    await waitFor(() =>
      expect(
        within(header).getByRole('link', { name: 'Overview' })
      ).toHaveAttribute('href', '/dashboard/overview')
    )
  })

  test('an open inspector makes a main destination a second-level page whose Back closes it', async () => {
    setViewport(390)
    const user = userEvent.setup()
    const close = vi.fn()
    await renderShellRoute(<Workspace />, { path: '/keys' })
    act(() =>
      useInspectorStore.getState().setCurrent({ title: 'API Key #21', close })
    )
    const header = screen.getByRole('banner')
    expect(within(header).getByText('API Key #21')).toHaveAttribute(
      'title',
      'API Key #21'
    )
    await user.click(within(header).getByRole('button', { name: 'Back' }))
    expect(close).toHaveBeenCalledOnce()
    act(() => useInspectorStore.getState().setCurrent(null))
  })
})

describe('inspector in the PC breadcrumb', () => {
  test('the breadcrumb names the open object and its page item closes the inspector', async () => {
    const user = userEvent.setup()
    const close = vi.fn()
    await renderShellRoute(<Workspace />, { path: '/keys' })
    act(() =>
      useInspectorStore.getState().setCurrent({ title: 'API Key #21', close })
    )
    const crumbs = screen.getByRole('navigation', { name: 'Current page' })
    expect(within(crumbs).getByText('API Key #21')).toHaveAttribute(
      'aria-current',
      'page'
    )
    await user.click(within(crumbs).getByRole('button', { name: 'API Keys' }))
    expect(close).toHaveBeenCalledOnce()
    act(() => useInspectorStore.getState().setCurrent(null))
  })
})
