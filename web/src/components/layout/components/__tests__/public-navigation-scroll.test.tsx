import { cleanup, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import { PublicHeader } from '../public-header'
import { renderShellRoute } from './shell-test-utils'

const originalAuth = useAuthStore.getState()
afterEach(() => { cleanup(); document.body.style.overflow = ''; useAuthStore.setState(originalAuth) })
it('keeps the public mobile menu scrollable with the full site name and lets keyboard navigation close it while restoring the original body lock', async () => {
  useAuthStore.getState().auth.reset('idle')
  document.body.style.overflow = 'auto'
  const name = 'Synthetic public instance ' + 'long-name-'.repeat(12)
  const { router, queryClient } = await renderShellRoute(<PublicHeader siteName={name} showThemeSwitch={false} showLanguageSwitcher={false} showNotifications={false} showAuthButtons={false} />, { path: '/' })
  try {
    const user = userEvent.setup()
    const toggle = screen.getByRole('button', { name: 'Toggle navigation menu' })
    toggle.focus()
    await user.keyboard('{Enter}')
    const overlay = document.querySelector('#public-mobile-navigation') as HTMLElement
    expect(document.body.style.overflow).toBe('hidden')
    expect(within(overlay).getByText(name)).toBeVisible()
    expect(within(overlay).getByRole('navigation').parentElement).toHaveClass('overflow-y-auto', 'overscroll-contain')
    const about = within(overlay).getByRole('link', { name: 'About' })
    about.focus()
    await user.keyboard('{Enter}')
    await waitFor(() => expect(router.state.location.pathname).toBe('/about'))
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(overlay).toHaveAttribute('inert')
    expect(document.body.style.overflow).toBe('auto')
  } finally { queryClient.clear() }
})
