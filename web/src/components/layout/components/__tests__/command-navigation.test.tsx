import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeEach, describe, expect, test } from 'vitest'

import { CommandMenu } from '@/components/command-menu'
import { SearchContext } from '@/context/search-context'
import { ThemeProvider } from '@/context/theme-provider'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { renderShellRoute } from './shell-test-utils'

function SearchWorkspace() {
  const [open, setOpen] = useState(true)
  return (
    <ThemeProvider>
      <SearchContext value={{ open, setOpen }}>
        <CommandMenu />
      </SearchContext>
    </ThemeProvider>
  )
}

const originalGetAnimations = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)

describe('command navigation follows sidebar access', () => {
  beforeEach(() => {
    Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
      configurable: true,
      value: () => [],
    })
    localStorage.clear()
    useAuthStore
      .getState()
      .auth.setUser({ id: 2, username: 'member', role: ROLE.USER })
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

  test('an ordinary user can search daily tasks without administrator destinations', async () => {
    await renderShellRoute(<SearchWorkspace />)

    expect(
      await screen.findByRole('option', { name: 'API Keys' })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('option', { name: 'Channels' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('option', { name: 'System Settings' })
    ).not.toBeInTheDocument()
  })

  test('hidden modules do not become reachable from command search', async () => {
    await renderShellRoute(<SearchWorkspace />, {
      status: {
        SidebarModulesAdmin: JSON.stringify({
          console: { enabled: true, token: false },
        }),
      },
    })

    expect(
      await screen.findByRole('option', { name: 'Overview' })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('option', { name: 'API Keys' })
    ).not.toBeInTheDocument()
  })

  test('a root operator keeps contextual settings search and keyboard navigation', async () => {
    const user = userEvent.setup()
    useAuthStore
      .getState()
      .auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })
    const { router } = await renderShellRoute(<SearchWorkspace />, {
      path: '/system-settings/site/system-info',
    })

    const input = await screen.findByRole('combobox')
    await user.type(input, 'SMTP Email')
    expect(
      screen.getByRole('option', { name: /SMTP Email/ })
    ).toBeInTheDocument()
    await user.keyboard('{Enter}')
    await waitFor(() =>
      expect(router.state.location.pathname).toBe(
        '/system-settings/operations/email'
      )
    )
    await waitFor(() =>
      expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
    )
  })
})
