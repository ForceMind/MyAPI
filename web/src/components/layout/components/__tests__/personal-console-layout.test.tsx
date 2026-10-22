import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test } from 'vitest'

import { SidebarProvider } from '@/components/ui/sidebar'
import { LayoutProvider } from '@/context/layout-provider'
import { SearchProvider } from '@/context/search-provider'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { PersonalConsoleLayout } from '../personal-console-layout'
import { renderShellRoute } from './shell-test-utils'

const originalGetAnimations = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)

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

test('complete desktop navigation and mobile More retain diagnostic and maintenance modules', async () => {
  await renderShellRoute(
    <LayoutProvider>
      <SearchProvider>
        <SidebarProvider>
          <PersonalConsoleLayout>
            <main id='content'>Workspace</main>
          </PersonalConsoleLayout>
        </SidebarProvider>
      </SearchProvider>
    </LayoutProvider>
  )
  for (const label of [
    'Personal console navigation',
    'Mobile console navigation',
  ]) {
    const navigation = screen.getByRole('navigation', { name: label })
    expect(
      within(navigation).getByRole('link', { name: 'Overview' })
    ).toHaveAttribute('aria-current', 'page')
    expect(
      within(navigation).getByRole('link', { name: 'API Keys' })
    ).toHaveAttribute('href', '/keys')
    expect(
      within(navigation).queryByRole('link', {
        name: /Wallet|Balance|Recharge/i,
      })
    ).not.toBeInTheDocument()
  }
  const desktop = screen.getByRole('navigation', {
    name: 'Personal console navigation',
  })
  for (const [name, href] of [
    ['API Request Logs', '/full-content-logs'],
    ['Task Logs', '/usage-logs/task'],
    ['Drawing Logs', '/usage-logs/drawing'],
    ['Connections', '/connections'],
    ['System Info', '/system-info'],
  ]) {
    expect(within(desktop).getByRole('link', { name })).toHaveAttribute(
      'href',
      href
    )
  }
  await userEvent.setup().click(screen.getByRole('button', { name: 'More' }))
  const more = await screen.findByRole('dialog', { name: 'All modules' })
  expect(
    within(more).getByRole('link', { name: 'API Request Logs' })
  ).toHaveAttribute('href', '/full-content-logs')
  expect(
    within(more).getByRole('link', { name: 'System Settings' })
  ).toHaveAttribute('href', '/system-settings/site')
  expect(screen.getByTestId('console-build-revision')).toHaveTextContent(
    /^rv\./
  )
})

test('a lower role cannot use the upstream destination and sidebar narrowing still applies', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 2, username: 'key-owner', role: ROLE.USER })
  await renderShellRoute(
    <LayoutProvider>
      <SearchProvider>
        <SidebarProvider>
          <PersonalConsoleLayout>
            <main id='content'>Workspace</main>
          </PersonalConsoleLayout>
        </SidebarProvider>
      </SearchProvider>
    </LayoutProvider>,
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
    within(navigation).queryByRole('link', { name: 'Upstream accounts' })
  ).not.toBeInTheDocument()
  expect(
    within(navigation).queryByRole('link', { name: 'API Keys' })
  ).not.toBeInTheDocument()
})

test('channel management has its own destination instead of selecting the account window page', async () => {
  await renderShellRoute(
    <LayoutProvider>
      <SearchProvider>
        <SidebarProvider>
          <PersonalConsoleLayout>
            <main id='content'>Channel management</main>
          </PersonalConsoleLayout>
        </SidebarProvider>
      </SearchProvider>
    </LayoutProvider>,
    { path: '/channels' }
  )
  for (const label of ['Personal console navigation']) {
    const navigation = screen.getByRole('navigation', { name: label })
    expect(
      within(navigation).getByRole('link', { name: 'Connections' })
    ).toHaveAttribute('aria-current', 'page')
    expect(
      within(navigation).getByRole('link', { name: 'Upstream accounts' })
    ).not.toHaveAttribute('aria-current')
  }
})

test('keyboard users can reach the skip link before the navigation', async () => {
  const user = userEvent.setup()
  await renderShellRoute(
    <LayoutProvider>
      <SearchProvider>
        <SidebarProvider>
          <PersonalConsoleLayout>
            <main id='content' tabIndex={-1}>
              Workspace
            </main>
          </PersonalConsoleLayout>
        </SidebarProvider>
      </SearchProvider>
    </LayoutProvider>
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
