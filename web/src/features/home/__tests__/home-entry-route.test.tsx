import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import { Home } from '../index'

vi.mock('@/components/layout', () => ({
  PublicLayout: (props: { children?: ReactNode }) => props.children,
}))
vi.mock('@/components/layout/components/footer', () => ({ Footer: () => null }))
vi.mock('@/components/rich-content', () => ({ RichContent: () => null }))
vi.mock('@/context/theme-provider', () => ({
  useTheme: () => ({ resolvedTheme: 'light' }),
}))
vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({ status: {}, confirmed: true }),
}))
vi.mock('../hooks', () => ({
  useHomePageContent: () => ({
    content: '',
    isLoaded: true,
    isUrl: false,
    failed: false,
    retrying: false,
    retry: vi.fn(),
  }),
}))

afterEach(() => useAuthStore.getState().auth.reset('idle'))

test('signed-in home finishes navigation when the root guard reconfirms authentication', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'synthetic-owner', role: 100 })
  const guardedPaths: string[] = []
  const root = createRootRoute({
    component: Outlet,
    beforeLoad: async ({ location }) => {
      guardedPaths.push(location.pathname)
      if (
        guardedPaths.filter((path) => path === '/dashboard/overview').length > 1
      ) {
        throw new Error('Repeated home navigation prevented completion')
      }
      useAuthStore.getState().auth.setBootstrapState('complete')
      await Promise.resolve()
    },
  })
  const home = createRoute({
    getParentRoute: () => root,
    path: '/',
    component: Home,
  })
  const overview = createRoute({
    getParentRoute: () => root,
    path: 'dashboard/$section',
    component: () => <h1>Synthetic workspace</h1>,
  })
  const router = createRouter({
    routeTree: root.addChildren([home, overview]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(<RouterProvider router={router} />)
  await screen.findByRole('heading', { name: 'Synthetic workspace' })
  expect(router.state.location.pathname).toBe('/dashboard/overview')
  expect(
    guardedPaths.filter((path) => path === '/dashboard/overview')
  ).toHaveLength(1)
})
