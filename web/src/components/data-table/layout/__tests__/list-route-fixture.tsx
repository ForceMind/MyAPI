/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClientProvider, type QueryClient } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render } from '@testing-library/react'
import { useState, type ReactNode } from 'react'

import { PageFooterProvider } from '@/components/layout/components/page-footer'
import { TooltipProvider } from '@/components/ui/tooltip'

// oxlint-disable-next-line react/only-export-components -- Integration-test fixture, not a hot-reloaded module.
function ListLayout(props: { children: ReactNode }) {
  const [footer, setFooter] = useState<HTMLDivElement | null>(null)
  return (
    <TooltipProvider>
      <PageFooterProvider container={footer}>
        {props.children}
        <div ref={setFooter} />
      </PageFooterProvider>
    </TooltipProvider>
  )
}

export async function renderListRoute(props: {
  client: QueryClient
  path: string
  entry?: string
  element: ReactNode
}) {
  const rootRoute = createRootRoute()
  const authenticatedRoute = createRoute({
    getParentRoute: () => rootRoute,
    id: '_authenticated',
  })
  const pageRoute = createRoute({
    getParentRoute: () => authenticatedRoute,
    path: props.path,
    validateSearch: (search: Record<string, unknown>) => search,
    component: () => <ListLayout>{props.element}</ListLayout>,
  })
  const router = createRouter({
    routeTree: rootRoute.addChildren([
      authenticatedRoute.addChildren([pageRoute]),
    ]),
    history: createMemoryHistory({
      initialEntries: [props.entry ?? props.path],
    }),
  })
  await router.load()
  const rendered = render(
    <QueryClientProvider client={props.client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return { ...rendered, router }
}
