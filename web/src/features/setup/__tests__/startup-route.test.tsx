import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

const clients: QueryClient[] = []

beforeEach(() => {
  vi.resetModules()
  localStorage.clear()
})

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  localStorage.clear()
})

async function openApplication(setupResults: unknown[], oldCheckedFlag = false) {
  if (oldCheckedFlag) localStorage.setItem('setup_status_checked', 'true')
  const { api } = await import('@/lib/api')
  const { useAuthStore } = await import('@/stores/auth-store')
  useAuthStore.getState().auth.setBundle({
    access_token: 'synthetic-startup-token',
    token_type: 'Bearer',
    access_expires_at: 2_100_000_000,
    user: { id: 1, username: 'operator', role: 1 },
    session: {
      sid: 'synthetic-startup-session', current: true, login_method: 'password',
      ip: '127.0.0.1', user_agent: 'vitest', created_at: 1_900_000_000,
      last_active_at: 1_900_000_000, expires_at: 2_100_000_000,
    },
  })
  const results = [...setupResults]
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/setup') return { data: results.shift() }
    if (url === '/api/status') return { data: { success: true, data: {} } }
    throw new Error(`Unexpected API read: ${url}`)
  })
  const { Route } = await import('@/routes/__root')
  const home = createRoute({ getParentRoute: () => Route, path: '/', component: () => <h1>Application home</h1> })
  const setup = createRoute({ getParentRoute: () => Route, path: '/setup', component: () => <h1>Installation wizard</h1> })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  clients.push(client)
  const router = createRouter({
    routeTree: Route.addChildren([home, setup]),
    context: { queryClient: client },
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await act(async () => { await router.load() })
  render(<QueryClientProvider client={client}><RouterProvider router={router} /></QueryClientProvider>)
  return { router, get }
}

describe('application installation guard', () => {
  test('keeps a failed first read recoverable and checks setup again before opening the application', async () => {
    const { router } = await openApplication([
      { success: false },
      { success: true, data: { status: false, root_init: false, database_type: 'sqlite' } },
    ])
    expect(screen.queryByRole('heading', { name: 'Application home' })).not.toBeInTheDocument()
    expect(localStorage.getItem('setup_status_checked')).not.toBe('true')

    await userEvent.setup().click(await screen.findByRole('button', { name: 'Retry' }))

    expect(await screen.findByRole('heading', { name: 'Installation wizard' })).toBeInTheDocument()
    await waitFor(() => expect(router.state.location.pathname).toBe('/setup'))
  })

  test('checks the current installation even when an old browser stored a completed flag', async () => {
    const { router } = await openApplication([
      { success: true, data: { status: false, root_init: false, database_type: 'sqlite' } },
    ], true)

    expect(await screen.findByRole('heading', { name: 'Installation wizard' })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/setup')
    expect(screen.queryByRole('heading', { name: 'Application home' })).not.toBeInTheDocument()
  })
})
