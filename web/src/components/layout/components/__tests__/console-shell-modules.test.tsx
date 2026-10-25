import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { SearchProvider } from '@/context/search-provider'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { ConsoleShell } from '../console-shell'
import { renderShellRoute } from './shell-test-utils'

const originalGetAnimations = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)

function setMobileViewport() {
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

function Shell(props: { children: React.ReactNode }) {
  return (
    <SearchProvider>
      <ConsoleShell>{props.children}</ConsoleShell>
    </SearchProvider>
  )
}

beforeEach(() => {
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'maintainer', role: ROLE.SUPER_ADMIN })
})
afterEach(() => {
  useAuthStore.getState().auth.reset('idle')
  if (originalGetAnimations) {
    Object.defineProperty(
      HTMLElement.prototype,
      'getAnimations',
      originalGetAnimations
    )
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
  }
})

test('desktop navigation retains diagnostic and maintenance modules without commerce', async () => {
  const user = userEvent.setup()
  const { router } = await renderShellRoute(
    <Shell>
      <main id='content'>Workspace</main>
    </Shell>
  )
  const desktop = screen.getByRole('navigation', {
    name: 'Personal console navigation',
  })
  expect(
    within(desktop).getByRole('link', { name: 'Overview' })
  ).toHaveAttribute('aria-current', 'page')
  expect(
    within(desktop).getByRole('link', { name: 'API Keys' })
  ).toHaveAttribute('href', '/keys')
  expect(
    within(desktop).getByRole('link', { name: 'Upstream' })
  ).toHaveAttribute('href', '/connections')
  expect(
    within(desktop).queryByRole('link', { name: /Wallet|Balance|Recharge/i })
  ).not.toBeInTheDocument()
  // The record pages are the Requests area's tabs.
  await user.click(within(desktop).getByRole('link', { name: 'Requests' }))
  await waitFor(() =>
    expect(router.state.location.pathname).toBe('/usage-logs/common')
  )
  const records = screen.getByRole('navigation', { name: 'Requests' })
  for (const [name, href] of [
    ['Bodies & SSE', '/full-content-logs'],
    ['Tasks', '/usage-logs/task'],
    ['Drawing', '/usage-logs/drawing'],
  ]) {
    expect(within(records).getByRole('link', { name })).toHaveAttribute(
      'href',
      href
    )
  }
  const admin = within(desktop).getByRole('button', { name: 'Admin' })
  expect(admin).toHaveAttribute('aria-expanded', 'false')
  await user.click(admin)
  expect(admin).toHaveAttribute('aria-expanded', 'true')
  const menu = await screen.findByRole('menu')
  expect(
    within(menu).getByRole('menuitem', { name: 'System status' })
  ).toHaveAttribute('href', '/system-info')
  expect(
    within(menu).queryByRole('menuitem', {
      name: /Wallet|Balance|Recharge/i,
    })
  ).not.toBeInTheDocument()
  await user.keyboard('{Escape}')
  await user.click(screen.getByRole('button', { name: 'Account menu' }))
  expect(await screen.findByTestId('console-build-revision')).toHaveTextContent(
    /^rv\./
  )
})

test('mobile More retains diagnostic and maintenance modules without commerce', async () => {
  setMobileViewport()
  await renderShellRoute(
    <Shell>
      <main id='content'>Workspace</main>
    </Shell>
  )
  const mobile = screen.getByRole('navigation', {
    name: 'Mobile console navigation',
  })
  expect(
    within(mobile).getByRole('link', { name: 'Overview' })
  ).toHaveAttribute('aria-current', 'page')
  expect(within(mobile).getByRole('link', { name: 'Keys' })).toHaveAttribute(
    'href',
    '/keys'
  )
  await userEvent.setup().click(screen.getByRole('button', { name: 'More' }))
  const more = await screen.findByRole('dialog', { name: 'All modules' })
  expect(
    within(more).getByRole('link', { name: 'Bodies & SSE' })
  ).toHaveAttribute('href', '/full-content-logs')
  expect(within(more).getByRole('link', { name: 'Settings' })).toHaveAttribute(
    'href',
    '/system-settings/site'
  )
  expect(
    within(more).queryByRole('link', { name: /Wallet|Balance|Recharge/i })
  ).not.toBeInTheDocument()
})

test('a lower role cannot use the upstream destination and sidebar narrowing still applies', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 2, username: 'key-owner', role: ROLE.USER })
  await renderShellRoute(
    <Shell>
      <main id='content'>Workspace</main>
    </Shell>,
    {
      status: {
        SidebarModulesAdmin: JSON.stringify({
          console: { enabled: true, token: false },
        }),
      },
    }
  )
  const navigation = screen.getByRole('navigation', {
    name: 'Personal console navigation',
  })
  expect(
    within(navigation).queryByRole('link', { name: 'Upstream' })
  ).not.toBeInTheDocument()
  expect(
    within(navigation).queryByRole('link', { name: 'API Keys' })
  ).not.toBeInTheDocument()
})

test('channel management has its own destination instead of selecting the account window page', async () => {
  await renderShellRoute(
    <Shell>
      <main id='content'>Channel management</main>
    </Shell>,
    { path: '/channels' }
  )
  const navigation = screen.getByRole('navigation', {
    name: 'Personal console navigation',
  })
  expect(
    within(navigation).getByRole('link', { name: 'Upstream' })
  ).toHaveAttribute('aria-current', 'page')
  const pages = screen.getByRole('navigation', { name: 'Upstream' })
  expect(
    within(pages)
      .getAllByRole('link')
      .map((link) => link.textContent)
  ).toEqual(['Connections', 'Account windows', 'Model catalog'])
  expect(
    within(pages).getByRole('link', { name: 'Connections' })
  ).toHaveAttribute('aria-current', 'page')
  expect(
    within(pages).getByRole('link', { name: 'Account windows' })
  ).not.toHaveAttribute('aria-current')
})

test('keyboard users can reach the skip link before the navigation', async () => {
  const user = userEvent.setup()
  await renderShellRoute(
    <Shell>
      <main id='content' tabIndex={-1}>
        Workspace
      </main>
    </Shell>
  )
  await user.tab()
  expect(screen.getByRole('link', { name: 'Skip to Main' })).toHaveFocus()
  await user.keyboard('{Enter}')
  expect(screen.getByRole('main')).toHaveFocus()
})

test('confirmed personal mode retains context for existing maintenance and error pages', async () => {
  const { AuthenticatedLayout } = await import('../authenticated-layout')
  const { useSearch } = await import('@/context/search-provider')
  const { ThemeProvider } = await import('@/context/theme-provider')
  function ExistingPage() {
    const search = useSearch()
    return (
      <main id='content'>
        <button type='button' onClick={() => search.setOpen(true)}>
          Find a page
        </button>
      </main>
    )
  }
  await renderShellRoute(
    <ThemeProvider>
      <AuthenticatedLayout>
        <ExistingPage />
      </AuthenticatedLayout>
    </ThemeProvider>,
    {
      status: {
        user_funding_mode: 'disabled',
        user_funding_capabilities: {
          mode: 'disabled',
          epoch: 1,
          ready: true,
          can_top_up: false,
          can_redeem: false,
          can_transfer_affiliate_rewards: false,
          can_purchase_subscription: false,
          can_view_funding_history: true,
        },
      },
    }
  )
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'Find a page' }))
  expect(await screen.findByRole('dialog')).toBeInTheDocument()
})
