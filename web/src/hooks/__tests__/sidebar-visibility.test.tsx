import { screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, test } from 'vitest'

import { renderShellRoute } from '@/components/layout/components/__tests__/shell-test-utils'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { useSidebarView } from '../use-sidebar-view'

function NavigationProbe() {
  const { navGroups } = useSidebarView()
  return (
    <nav>
      {navGroups.map((group) => (
        <section key={group.id} aria-label={group.title}>
          {group.items.map((item) =>
            item.url ? (
              <a href={item.url} key={item.title}>
                {item.title}
              </a>
            ) : (
              <div key={item.title}>
                {item.title}
                {item.items?.map((child) => (
                  <a href={child.url} key={child.title}>
                    {child.title}
                  </a>
                ))}
              </div>
            )
          )}
        </section>
      ))}
    </nav>
  )
}

describe('sidebar role and module visibility', () => {
  beforeEach(() => {
    localStorage.clear()
    useAuthStore
      .getState()
      .auth.setUser({ id: 1, username: 'operator', role: ROLE.ADMIN })
  })

  afterEach(() => {
    useAuthStore.getState().auth.reset('idle')
    localStorage.clear()
  })

  test('an administrator cannot see the root-only System Settings entry', async () => {
    await renderShellRoute(<NavigationProbe />)

    expect(screen.getByRole('link', { name: 'Channels' })).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'System Settings' })
    ).not.toBeInTheDocument()
  })

  test('a root operator can see System Settings and System Info', async () => {
    useAuthStore
      .getState()
      .auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })
    await renderShellRoute(<NavigationProbe />)

    expect(
      screen.getByRole('link', { name: 'System Settings' })
    ).toHaveAttribute('href', '/system-settings/site')
    expect(
      screen.getByRole('link', { name: 'System Info' })
    ).toBeInTheDocument()
  })

  test('an ordinary user never receives the administration group', async () => {
    useAuthStore
      .getState()
      .auth.setUser({ id: 2, username: 'member', role: ROLE.USER })
    await renderShellRoute(<NavigationProbe />)

    expect(
      screen.queryByRole('region', { name: 'Admin' })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'API Keys' })).toBeInTheDocument()
  })

  test('admin exclusions and user exclusions both narrow the same navigation', async () => {
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'operator',
      role: ROLE.ADMIN,
      sidebar_modules: JSON.stringify({ console: { token: true, log: false } }),
    })
    await renderShellRoute(<NavigationProbe />, {
      status: {
        SidebarModulesAdmin: JSON.stringify({
          console: { enabled: true, token: false },
        }),
      },
    })

    expect(
      screen.queryByRole('link', { name: 'API Keys' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Usage Logs' })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Overview' })).toBeInTheDocument()
  })

  test('a lower-role session at a stale settings URL cannot enumerate nested settings', async () => {
    await renderShellRoute(<NavigationProbe />, {
      path: '/system-settings/site/system-info',
    })

    expect(
      screen.queryByRole('link', { name: 'System Information' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'System Settings' })
    ).not.toBeInTheDocument()
  })

  test('a root account that cannot customize modules ignores a stale user narrowing overlay', async () => {
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'root',
      role: ROLE.SUPER_ADMIN,
      permissions: { sidebar_settings: false },
      sidebar_modules: JSON.stringify({ admin: { enabled: false } }),
    })
    await renderShellRoute(<NavigationProbe />)

    expect(
      screen.getByRole('link', { name: 'System Settings' })
    ).toBeInTheDocument()
  })

  test('disabled commerce leaves funding history out of the default navigation', async () => {
    await renderShellRoute(<NavigationProbe />)
    for (const name of [
      'Funding history',
      'Wallet',
      'Subscriptions',
      'Redemption Codes',
    ]) {
      expect(screen.queryByRole('link', { name })).not.toBeInTheDocument()
    }
    expect(screen.queryByText('History and recovery')).not.toBeInTheDocument()
  })

  test('root navigation puts upstream operations before keys and usage', async () => {
    useAuthStore
      .getState()
      .auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })
    await renderShellRoute(<NavigationProbe />)
    const console = screen.getByRole('region', { name: 'Console' })
    expect(
      within(console)
        .getAllByRole('link')
        .map((link) => link.textContent)
    ).toEqual([
      'Overview',
      'Channels',
      'Models',
      'API Keys',
      'Usage Logs',
      'System Settings',
    ])
  })

  test('ordinary users do not receive upstream or settings destinations in the console', async () => {
    useAuthStore
      .getState()
      .auth.setUser({ id: 2, username: 'owner', role: ROLE.USER })
    await renderShellRoute(<NavigationProbe />)
    for (const name of ['Channels', 'Models', 'System Settings']) {
      expect(screen.queryByRole('link', { name })).not.toBeInTheDocument()
    }
  })
})
