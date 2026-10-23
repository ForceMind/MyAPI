import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { expect, test } from 'vitest'

import { NavigationProgress } from '../../navigation-progress'

async function renderConsoleNavigation(
  loader: () => Promise<void>,
  deferIndicator = false
) {
  function ConsolePage() {
    const [shown, setShown] = useState(!deferIndicator)
    return (
      <main>
        {shown ? (
          <NavigationProgress scope='workspace' />
        ) : (
          <button type='button' onClick={() => setShown(true)}>
            Show indicator
          </button>
        )}
        <Outlet />
      </main>
    )
  }
  const root = createRootRoute({
    component: () => (
      <>
        <NavigationProgress />
        <Outlet />
      </>
    ),
  })
  const authenticated = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
    component: ConsolePage,
  })
  const home = createRoute({
    getParentRoute: () => authenticated,
    path: '/',
    component: () => <p>Current page</p>,
  })
  const pending = createRoute({
    getParentRoute: () => authenticated,
    path: '/keys',
    loader,
    component: () => <p>Next page</p>,
  })
  const router = createRouter({
    routeTree: root.addChildren([authenticated.addChildren([home, pending])]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  const rendered = render(<RouterProvider router={router} />)
  return { ...rendered, router }
}

test('opening an idle console page leaves the loading bar empty and scoped to its workspace', async () => {
  const { container, router } = await renderConsoleNavigation(
    async () => {},
    true
  )
  await waitFor(() => expect(router.state.status).toBe('idle'))
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'Show indicator' }))
  const bars = container.querySelectorAll('.myapi-navigation-progress')
  expect(bars).toHaveLength(1)
  expect(screen.getByRole('main')).toContainElement(bars[0] as HTMLElement)
  expect(bars[0]).toHaveStyle({ position: 'absolute' })
  expect(bars[0].firstElementChild).toHaveStyle({ width: '0%' })
})

test('a pending console route shows loading inside the workspace and finishes with the route', async () => {
  let finish = () => {}
  const loaded = new Promise<void>((resolve) => {
    finish = resolve
  })
  const { container, router } = await renderConsoleNavigation(() => loaded)
  let navigation: Promise<void> | undefined
  await act(async () => {
    navigation = router.navigate({ to: '/keys' })
  })
  const bar = container.querySelector('.myapi-navigation-progress')
  await waitFor(() =>
    expect(bar?.firstElementChild).not.toHaveStyle({ width: '0%' })
  )
  await act(async () => {
    finish()
    await navigation
  })
  expect(screen.getByText('Next page')).toBeInTheDocument()
  expect(container.querySelectorAll('.myapi-navigation-progress')).toHaveLength(
    1
  )
})
