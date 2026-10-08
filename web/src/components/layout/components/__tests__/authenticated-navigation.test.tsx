import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { DirectionProvider } from '@/context/direction-provider'
import { ThemeCustomizationProvider } from '@/context/theme-customization-provider'
import { ThemeProvider } from '@/context/theme-provider'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { AuthenticatedLayout } from '../authenticated-layout'
import { renderShellRoute } from './shell-test-utils'

function OperatorWorkspace() {
  return (
    <DirectionProvider>
      <ThemeProvider>
        <ThemeCustomizationProvider>
          <AuthenticatedLayout>
            <main id='content' tabIndex={-1}>
              Page content
            </main>
          </AuthenticatedLayout>
        </ThemeCustomizationProvider>
      </ThemeProvider>
    </DirectionProvider>
  )
}

const originalGetAnimations = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)
const originalInnerWidth = Object.getOwnPropertyDescriptor(window, 'innerWidth')

function setViewport(width: number) {
  Object.defineProperty(window, 'innerWidth', {
    configurable: true,
    value: width,
  })
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
    matches:
      query.includes('prefers-reduced-motion') ||
      (query.includes('max-width') && width < 768),
    media: query,
    onchange: null,
    addListener: () => undefined,
    removeListener: () => undefined,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    dispatchEvent: () => false,
  }))
}

describe('authenticated operator navigation', () => {
  beforeEach(() => {
    Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
      configurable: true,
      value: () => [],
    })
    setViewport(1440)
    localStorage.clear()
    document.cookie = 'sidebar_state=; max-age=0; path=/'
    useAuthStore
      .getState()
      .auth.setUser({ id: 1, username: 'operator', role: ROLE.SUPER_ADMIN })
  })

  afterEach(async () => {
    if (originalGetAnimations) {
      Object.defineProperty(
        HTMLElement.prototype,
        'getAnimations',
        originalGetAnimations
      )
    } else Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
    if (originalInnerWidth) {
      Object.defineProperty(window, 'innerWidth', originalInnerWidth)
    }
    useAuthStore.getState().auth.reset('idle')
    document.cookie = 'sidebar_state=; max-age=0; path=/'
    localStorage.clear()
    await i18next.changeLanguage('en')
    i18next.removeResourceBundle('fr', 'translation')
  })

  test('daily tasks lead the navigation and administration precedes optional chat', async () => {
    await renderShellRoute(<OperatorWorkspace />)
    const navigation = await screen.findByRole('navigation', {
      name: 'Navigation',
    })

    expect(
      within(navigation)
        .getAllByRole('link')
        .slice(0, 6)
        .map((link) => link.textContent)
    ).toEqual([
      'Overview',
      'Channels',
      'Models',
      'API Keys',
      'Usage Logs',
      'System Settings',
    ])
    expect(
      within(navigation)
        .getAllByRole('group')
        .map((group) => group.getAttribute('aria-label'))
    ).toEqual(['Console', 'Admin', 'Tools', 'Personal'])
    expect(
      within(navigation).getByRole('link', { name: 'Overview' })
    ).toHaveAttribute('aria-current', 'page')
  })

  test('disabled commerce is absent from the account menu while profile remains reachable', async () => {
    await renderShellRoute(<OperatorWorkspace />)
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Account menu' }))
    expect(
      await screen.findByRole('menuitem', { name: 'Profile' })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('menuitem', { name: 'Funding history' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('menuitem', { name: 'Wallet' })
    ).not.toBeInTheDocument()
  })

  test('the header identifies the actual tab and preserves compact access to every primary action', async () => {
    await renderShellRoute(<OperatorWorkspace />, {
      path: '/dashboard/flow?period=7d',
    })
    const header = screen.getByRole('banner')
    const context = within(header).getByRole('navigation', {
      name: 'Current page',
    })

    expect(
      within(context).getByRole('link', { current: 'page' })
    ).toHaveTextContent('Flow')
    expect(within(header).getByRole('button', { name: 'Search' })).toHaveClass(
      'size-9',
      'sm:w-9'
    )
    expect(
      within(header).getByRole('button', { name: 'Change language' })
    ).toBeInTheDocument()
    expect(
      within(header).getByRole('button', { name: 'Notifications' })
    ).toBeInTheDocument()
    expect(
      within(header).getByRole('button', { name: 'Account menu' })
    ).toBeInTheDocument()
    // Column shrinkage is the contract; touch/desktop row placement and actual
    // clipping are qualified by the production-build browser journey.
    expect(context.parentElement).toHaveClass('min-w-0')
  })

  test('long translated page context updates live without losing its accessible full name', async () => {
    await renderShellRoute(<OperatorWorkspace />, { path: '/dashboard/models' })
    const longTitle =
      'Analyse détaillée des appels aux modèles et de leur utilisation quotidienne'
    i18next.addResourceBundle('fr', 'translation', {
      'Current page': 'Page actuelle',
      'Model Call Analytics': longTitle,
    })

    await act(async () => {
      await i18next.changeLanguage('fr')
    })

    const context = screen.getByRole('navigation', { name: 'Page actuelle' })
    const currentPage = within(context).getByRole('link', { current: 'page' })
    expect(currentPage).toHaveAccessibleName(longTitle)
    expect(currentPage).toHaveAttribute('title', longTitle)
    expect(currentPage).toHaveClass('truncate')
  })

  test('a mobile drawer supports Escape and restores keyboard focus to its trigger', async () => {
    setViewport(320)
    const user = userEvent.setup()
    await renderShellRoute(<OperatorWorkspace />)
    const toggle = screen.getByRole('button', { name: 'Toggle sidebar' })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')

    await user.click(toggle)
    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    await user.keyboard('{Escape}')

    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(toggle).toHaveFocus()
  })

  test('the visible mobile Close action preserves the current destination', async () => {
    setViewport(320)
    const user = userEvent.setup()
    const { router } = await renderShellRoute(<OperatorWorkspace />, {
      path: '/usage-logs/common?type=consume',
    })
    await user.click(screen.getByRole('button', { name: 'Toggle sidebar' }))
    const drawer = await screen.findByRole('dialog')

    await user.click(within(drawer).getByRole('button', { name: 'Close' }))

    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    expect(router.state.location.href).toBe('/usage-logs/common?type=consume')
  })

  test('choosing a mobile destination closes the drawer and browser Back closes a reopened drawer', async () => {
    setViewport(320)
    const user = userEvent.setup()
    const { router } = await renderShellRoute(<OperatorWorkspace />)
    const toggle = screen.getByRole('button', { name: 'Toggle sidebar' })

    await user.click(toggle)
    const navigation = await screen.findByRole('navigation', {
      name: 'Navigation',
    })
    await user.click(within(navigation).getByRole('link', { name: 'API Keys' }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/keys'))
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    await user.click(toggle)
    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    await act(async () => {
      router.history.back()
    })

    await waitFor(() =>
      expect(router.state.location.pathname).toBe('/dashboard/overview')
    )
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    const context = screen.getByRole('navigation', { name: 'Current page' })
    expect(
      within(context).getByRole('link', { current: 'page' })
    ).toHaveTextContent('Overview')
  })

  test('nested settings return to overview through the mobile back affordance', async () => {
    setViewport(320)
    const user = userEvent.setup()
    const { router } = await renderShellRoute(<OperatorWorkspace />, {
      path: '/system-settings/operations/update-checker?check=latest',
    })
    const context = screen.getByRole('navigation', { name: 'Current page' })
    expect(
      within(context).getByRole('link', { current: 'page' })
    ).toHaveTextContent('System maintenance')

    await user.click(screen.getByRole('button', { name: 'Toggle sidebar' }))
    const back = await screen.findByRole('link', { name: 'Back to Dashboard' })
    await user.click(back)

    await waitFor(() =>
      expect(router.state.location.pathname).toBe('/dashboard/overview')
    )
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
  })

  test('collapsed desktop settings remain reachable by keyboard', async () => {
    document.cookie = 'sidebar_state=false; path=/'
    const user = userEvent.setup()
    const { router } = await renderShellRoute(<OperatorWorkspace />, {
      path: '/system-settings/site/system-info',
    })
    const navigation = screen.getByRole('navigation', { name: 'Navigation' })
    const operations = within(navigation).getByRole('button', {
      name: 'Operations',
    })
    operations.focus()

    await user.keyboard('{Enter}')
    const maintenance = await screen.findByRole('menuitem', {
      name: 'System maintenance',
    })
    expect(operations).toHaveAttribute('aria-expanded', 'true')
    maintenance.focus()
    await user.keyboard('{Enter}')

    await waitFor(() =>
      expect(router.state.location.pathname).toBe(
        '/system-settings/operations/update-checker'
      )
    )
    await waitFor(() =>
      expect(screen.queryByRole('menu')).not.toBeInTheDocument()
    )
  })
})
