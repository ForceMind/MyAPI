import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { DirectionProvider } from '@/context/direction-provider'
import { ThemeProvider } from '@/context/theme-provider'
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
  localStorage.clear()
})

describe('mobile composition', () => {
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
    ).toEqual(['Overview', 'Keys', 'Requests'])
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
