import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render } from '@testing-library/react'
import type { ReactNode } from 'react'

import type { SystemStatus } from '@/features/auth/types'

export async function renderShellRoute(
  element: ReactNode,
  options: { path?: string; status?: SystemStatus } = {}
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  queryClient.setQueryData(['status'], options.status ?? {})
  queryClient.setQueryData(['notice'], { success: true, data: '' })
  const rootRoute = createRootRoute({ component: () => element })
  const pageRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '$',
  })
  const router = createRouter({
    routeTree: rootRoute.addChildren([pageRoute]),
    history: createMemoryHistory({
      initialEntries: [options.path ?? '/dashboard/overview'],
    }),
  })
  await router.load()
  const rendered = render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return { ...rendered, router, queryClient }
}
