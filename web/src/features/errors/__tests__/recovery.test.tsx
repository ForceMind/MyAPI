import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { describe, expect, it } from 'vitest'

import { ForbiddenError } from '../forbidden'
import { GeneralError } from '../general-error'
import { MaintenanceError } from '../maintenance-error'
import { NotFoundError } from '../not-found-error'
import { UnauthorisedError } from '../unauthorized-error'

async function renderError(element: ReactNode) {
  const router = createRouter({
    routeTree: createRootRoute({ component: () => element }),
    history: createMemoryHistory({
      initialEntries: ['/previous', '/'],
      initialIndex: 1,
    }),
  })
  await router.load()
  return { ...render(<RouterProvider router={router} />), router }
}

describe('error recovery', () => {
  it('replaces the inert maintenance action with a working home destination', async () => {
    await renderError(<MaintenanceError />)
    expect(screen.getByRole('link', { name: 'Back to Home' })).toHaveAttribute(
      'href',
      '/'
    )
    expect(
      screen.queryByRole('button', { name: 'Learn more' })
    ).not.toBeInTheDocument()
  })

  it.each([
    { element: <ForbiddenError />, title: 'Access Forbidden', code: '403' },
    { element: <NotFoundError />, title: 'Oops! Page Not Found!', code: '404' },
    {
      element: <UnauthorisedError />,
      title: 'Unauthorized Access',
      code: '401',
    },
  ])(
    'provides a readable $code title and one focusable main landmark',
    async ({ element, title, code }) => {
      await renderError(element)
      expect(
        screen.getByRole('heading', { name: title, level: 1 })
      ).toBeInTheDocument()
      expect(screen.getByText(code)).toBeInTheDocument()
      const main = screen.getByRole('main')
      expect(main).toHaveAttribute('id', 'content')
      main.focus()
      expect(main).toHaveFocus()
    }
  )

  it('lets keyboard users recover through the previous history entry', async () => {
    const user = userEvent.setup()
    const { router } = await renderError(<NotFoundError />)
    screen.getByRole('button', { name: 'Go Back' }).focus()
    await user.keyboard('{Enter}')
    await waitFor(() =>
      expect(router.state.location.pathname).toBe('/previous')
    )
  })

  it('offers a direct sign-in recovery for unauthorized access', async () => {
    await renderError(<UnauthorisedError />)
    expect(screen.getByRole('link', { name: 'Sign in' })).toHaveAttribute(
      'href',
      '/sign-in'
    )
  })

  it('keeps rate limiting distinct from server failure with safe external feedback', async () => {
    await renderError(<GeneralError error={{ response: { status: 429 } }} />)
    expect(
      screen.getByRole('heading', { name: 'Too many requests', level: 1 })
    ).toBeInTheDocument()
    expect(screen.getByText('429')).toBeInTheDocument()
    expect(screen.queryByText('500')).not.toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Report an issue' })
    ).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it('keeps minimal errors embeddable without duplicate main landmarks or recovery controls', async () => {
    await renderError(
      <main id='content'>
        <GeneralError minimal />
      </main>
    )
    expect(screen.getAllByRole('main')).toHaveLength(1)
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })
})
