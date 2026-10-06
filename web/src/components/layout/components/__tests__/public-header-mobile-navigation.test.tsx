import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AnchorHTMLAttributes } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import { PublicHeader } from '../public-header'

vi.mock('@tanstack/react-router', () => ({
  Link: (props: AnchorHTMLAttributes<HTMLAnchorElement> & { to?: string }) => {
    const { to, children, ...attributes } = props
    return (
      <a href={to} {...attributes}>
        {children}
      </a>
    )
  },
  useNavigate: () => vi.fn(),
  useRouterState: () => ({ location: { pathname: '/' } }),
}))
vi.mock('@/components/dialog', () => ({ Dialog: () => null }))
vi.mock('@/components/language-switcher', () => ({
  LanguageSwitcher: () => <button type='button'>Language picker</button>,
}))
vi.mock('@/components/notification-popover', () => ({
  NotificationPopover: () => null,
}))
vi.mock('@/components/profile-dropdown', () => ({
  ProfileDropdown: () => null,
}))
vi.mock('@/components/theme-switch', () => ({ ThemeSwitch: () => null }))
vi.mock('@/hooks/use-notifications', () => ({ useNotifications: () => ({}) }))
vi.mock('@/hooks/use-system-config', () => ({
  useSystemConfig: () => ({
    systemName: 'My API',
    logo: '/myapi-logo-v1.png',
    loading: false,
    logoLoaded: true,
  }),
}))
vi.mock('@/hooks/use-top-nav-links', () => ({ useTopNavLinks: () => [] }))
vi.mock('../header-logo', () => ({ HeaderLogo: () => null }))

function renderHeader(showLanguageSwitcher = false) {
  return render(
    <PublicHeader
      navLinks={[{ title: 'Home', href: '/' }]}
      showThemeSwitch={false}
      showLanguageSwitcher={showLanguageSwitcher}
      showNotifications={false}
      showAuthButtons={false}
    />
  )
}

describe('PublicHeader mobile navigation', () => {
  beforeEach(() => {
    useAuthStore.getState().auth.reset('idle')
  })

  afterEach(() => {
    document.body.style.overflow = ''
    useAuthStore.getState().auth.reset('idle')
    vi.clearAllMocks()
  })

  test('closed menu is excluded from keyboard and accessibility navigation', () => {
    renderHeader()
    const toggle = screen.getByRole('button', {
      name: 'Toggle navigation menu',
    })
    const overlay = document.querySelector('#public-mobile-navigation')

    expect(overlay).not.toBeNull()
    expect(toggle).toHaveAttribute('aria-controls', 'public-mobile-navigation')
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(overlay).toHaveAttribute('aria-hidden', 'true')
    expect(overlay).toHaveAttribute('inert')
    expect(
      within(overlay as HTMLElement).queryByRole('link', { name: 'Home' })
    ).toBeNull()
  })

  test('opening exposes links and Escape closes the menu again', async () => {
    const user = userEvent.setup()
    renderHeader()
    const toggle = screen.getByRole('button', {
      name: 'Toggle navigation menu',
    })
    const overlay = document.querySelector('#public-mobile-navigation')

    await user.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(overlay).toHaveAttribute('aria-hidden', 'false')
    expect(overlay).not.toHaveAttribute('inert')
    expect(
      within(overlay as HTMLElement).getByRole('link', { name: 'Home' })
    ).toBeInTheDocument()
    await user.keyboard('{Escape}')
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(overlay).toHaveAttribute('inert')
  })

  test('closing restores another overlay’s pre-existing body overflow', async () => {
    const user = userEvent.setup()
    document.body.style.overflow = 'auto'
    renderHeader()
    const toggle = screen.getByRole('button', {
      name: 'Toggle navigation menu',
    })

    expect(document.body.style.overflow).toBe('auto')
    await user.click(toggle)
    expect(document.body.style.overflow).toBe('hidden')
    await user.click(toggle)
    expect(document.body.style.overflow).toBe('auto')
  })

  test('mobile visitors can reach the existing language control after opening the menu', async () => {
    const user = userEvent.setup()
    renderHeader(true)
    const toggle = screen.getByRole('button', {
      name: 'Toggle navigation menu',
    })
    const overlay = document.querySelector('#public-mobile-navigation')

    expect(
      within(overlay as HTMLElement).queryByRole('button', {
        name: 'Language picker',
      })
    ).toBeNull()
    await user.click(toggle)
    expect(
      within(overlay as HTMLElement).getByRole('button', {
        name: 'Language picker',
      })
    ).toBeInTheDocument()
  })
})
