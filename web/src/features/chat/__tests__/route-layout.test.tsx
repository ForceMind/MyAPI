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
import { Route as ChatRoute } from '@/routes/_authenticated/chat/$chatId'

const clients: QueryClient[] = []

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  localStorage.clear()
})

async function renderChat(chats: Array<Record<string, string>>, chatId = '0') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  client.setQueryData(['status'], { chats })
  const root = createRootRoute({ component: Outlet })
  const authenticated = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
    component: Outlet,
  })
  const chat = createRoute({
    getParentRoute: () => authenticated,
    path: '/chat/$chatId',
    component: ChatRoute.options.component,
  })
  const router = createRouter({
    routeTree: root.addChildren([authenticated.addChildren([chat])]),
    history: createMemoryHistory({ initialEntries: [`/chat/${chatId}`] }),
  })
  await router.load()
  return render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

describe('chat route main landmark', () => {
  it('preserves the configured preset ID when earlier malformed entries are omitted', async () => {
    await renderChat([{}, { Workspace: 'https://chat.example.test/' }], '1')
    expect(screen.getByTitle('Chat preset: Workspace')).toHaveAttribute(
      'src',
      'https://chat.example.test/'
    )
  })
  it('keeps missing-preset recovery within the focusable content target', async () => {
    await renderChat([])
    const main = screen.getByRole('main')
    expect(main).toHaveAttribute('id', 'content')
    main.focus()
    expect(main).toHaveFocus()
    expect(main).toContainElement(
      screen.getByRole('heading', { name: 'Chat preset not found' })
    )
    expect(
      screen.getByRole('button', { name: 'Return to dashboard' })
    ).toHaveAttribute('href', '/dashboard')
  })

  it('retains iframe sandbox restrictions inside one main without requesting an unnecessary API key', async () => {
    const get = vi.spyOn(api, 'get')
    await renderChat([{ Workspace: 'https://chat.example.test/' }])
    const iframe = screen.getByTitle('Chat preset: Workspace')
    expect(screen.getByRole('main')).toContainElement(iframe)
    expect(iframe).toHaveAttribute('sandbox', 'allow-scripts allow-forms')
    expect(iframe).toHaveAttribute('src', 'https://chat.example.test/')
    expect(get).not.toHaveBeenCalled()
  })
})
