import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { SidebarProvider } from '@/components/ui/sidebar'
import { LayoutProvider } from '@/context/layout-provider'
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
    <LayoutProvider>
      <SearchProvider>
        <SidebarProvider>
          <ConsoleShell>{props.children}</ConsoleShell>
        </SidebarProvider>
      </SearchProvider>
    </LayoutProvider>
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
  await renderShellRoute(
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
    within(desktop).queryByRole('link', { name: /Wallet|Balance|Recharge/i })
  ).not.toBeInTheDocument()
  for (const [name, href] of [
    ['Bodies & SSE', '/full-content-logs'],
    ['Tasks', '/usage-logs/task'],
    ['Drawing', '/usage-logs/drawing'],
    ['Connections', '/connections'],
  ]) {
    expect(within(desktop).getByRole('link', { name })).toHaveAttribute(
      'href',
      href
    )
  }
  const system = within(desktop).getByRole('button', { name: 'System' })
  expect(system).toHaveAttribute('aria-expanded', 'false')
  await user.click(system)
  expect(system).toHaveAttribute('aria-expanded', 'true')
  expect(
    within(desktop).getByRole('link', { name: 'System status' })
  ).toHaveAttribute('href', '/system-info')
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
    within(navigation).queryByRole('link', { name: 'Account windows' })
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
    within(navigation).getByRole('link', { name: 'Connections' })
  ).toHaveAttribute('aria-current', 'page')
  expect(
    within(navigation).getByRole('link', { name: 'Account windows' })
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
  const { useLayout } = await import('@/context/layout-provider')
  const { useSearch } = await import('@/context/search-provider')
  const { useSidebar } = await import('@/components/ui/sidebar')
  const { ThemeProvider } = await import('@/context/theme-provider')
  function ExistingPage() {
    const layout = useLayout()
    const search = useSearch()
    const sidebar = useSidebar()
    return (
      <main id='content'>
        <button
          type='button'
          onClick={() => {
            layout.resetLayout()
            search.setOpen(true)
            sidebar.setOpen(false)
          }}
        >
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
