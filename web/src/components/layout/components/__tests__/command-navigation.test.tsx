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
    expect(screen.getByRole('option', { name: 'Profile' })).toBeInTheDocument()
    expect(
      screen.queryByRole('option', { name: 'Connections' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('option', { name: 'Settings' })
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

describe('command search opens a pasted Request ID', () => {
  const requestId = '20261024093015123456789ab12cd34Xy7Zq9Lm'

  beforeEach(() => {
    Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
      configurable: true,
      value: () => [],
    })
    localStorage.clear()
    useAuthStore
      .getState()
      .auth.setUser({ id: 1, username: 'root', role: ROLE.SUPER_ADMIN })
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

  test('pasting a Request ID and pressing Enter opens that request’s details in the call records', async () => {
    const user = userEvent.setup()
    const { router } = await renderShellRoute(<SearchWorkspace />)
    const input = await screen.findByRole('combobox')
    await user.click(input)
    await user.paste(`  ${requestId}\n`)
    expect(
      screen.getByRole('option', { name: `Open request ${requestId}` })
    ).toHaveAttribute('aria-selected', 'true')
    expect(screen.queryByText('No results found.')).not.toBeInTheDocument()
    await user.keyboard('{Enter}')
    await waitFor(() =>
      expect(router.state.location.pathname).toBe('/usage-logs/common')
    )
    expect(router.state.location.search).toMatchObject({
      requestId,
      inspect: 'request',
    })
    await waitFor(() =>
      expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
    )
  })

  test('typing a page name does not offer a Request ID lookup', async () => {
    const user = userEvent.setup()
    await renderShellRoute(<SearchWorkspace />)
    await user.type(await screen.findByRole('combobox'), 'Settings')
    expect(
      screen.queryByRole('option', { name: /^Open request/ })
    ).not.toBeInTheDocument()
  })

  test('a user whose call records are hidden is not offered a Request ID lookup', async () => {
    const user = userEvent.setup()
    await renderShellRoute(<SearchWorkspace />, {
      status: {
        SidebarModulesAdmin: JSON.stringify({
          console: { enabled: true, log: false },
        }),
      },
    })
    await user.click(await screen.findByRole('combobox'))
    await user.paste(requestId)
    expect(
      screen.queryByRole('option', { name: /^Open request/ })
    ).not.toBeInTheDocument()
  })
})
