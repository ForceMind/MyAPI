/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { LoginSessionsCard } from '../login-sessions-card'

afterEach(() => vi.restoreAllMocks())

test('mobile session actions occupy a separate full-width row so long labels cannot squeeze the title', async () => {
  vi.spyOn(api, 'get').mockImplementation(async () => ({
    data: { success: true, data: [] },
  }))
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const router = createRouter({
    routeTree: createRootRoute({
      component: () => (
        <LoginSessionsCard securityScopeKey='synthetic-layout-scope' />
      ),
    }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  const title = await screen.findByText('Login sessions')
  expect(title.closest('[data-slot="card-header"]')).toHaveClass(
    'has-data-[slot=card-action]:grid-cols-1'
  )
  const action = screen.getByRole('button', { name: 'Sign out other sessions' })
  expect(action.closest('[data-slot="card-action"]')).toHaveClass(
    'col-start-1',
    'row-start-3',
    'lg:col-start-2',
    'lg:row-start-1'
  )
  expect(action).toHaveClass('w-full', 'whitespace-normal', 'lg:w-auto')
  client.clear()
})
