import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { Route as ChatRedirectRoute } from '@/routes/_authenticated/chat2link'

const clients: QueryClient[] = []

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  localStorage.clear()
  vi.restoreAllMocks()
})

async function renderRedirect(chats: Array<Record<string, string>>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  client.setQueryData(['status'], { chats })
  const root = createRootRoute({ component: Outlet })
  const route = createRoute({
    getParentRoute: () => root,
    path: '/chat2link',
    component: ChatRedirectRoute.options.component,
  })
  const router = createRouter({
    routeTree: root.addChildren([route]),
    history: createMemoryHistory({ initialEntries: ['/chat2link'] }),
  })
  await router.load()
  return render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

describe('chat redirect recovery', () => {
  it('offers recovery instead of an endless redirect when no chat is configured', async () => {
    const get = vi.spyOn(api, 'get')
    await renderRedirect([])
    expect(screen.getByRole('main')).toHaveAttribute('id', 'content')
    expect(screen.getByText('Chat preset not found')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Return to dashboard' })
    ).toHaveAttribute('href', '/dashboard')
    expect(
      screen.queryByText('Redirecting to chat page...')
    ).not.toBeInTheDocument()
    expect(get).not.toHaveBeenCalled()
  })
})
