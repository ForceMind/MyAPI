/* Copyright (C) 2026 ForceMind. AGPL-3.0-or-later. */
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AxiosAdapter } from 'axios'
import { afterEach, expect, test, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { PRODUCT_CAPABILITIES } from '@/lib/product-edition'
import { ROLE } from '@/lib/roles'
import { Route as connectionRoute } from '@/routes/_authenticated/connections/$connectionId/index'
import { useAuthStore } from '@/stores/auth-store'
import { useInspectorStore } from '@/stores/inspector-store'

import { overviewSearchSchema } from '../../../lib/overview-flow'
import { OverviewPage } from '../overview-page'

const CONNECTION = {
  id: 7,
  type: 1,
  name: 'Saved routing connection',
  status: 1,
  models: 'saved-alpha,saved-beta',
  group: 'default',
  priority: 0,
  weight: 0,
}
const ENTRY = '/dashboard/overview?connection=7&activity=usage&flow=expanded'
const originalAdapter = api.defaults.adapter
const views: (() => void)[] = []

afterEach(() => {
  views.splice(0).forEach((dispose) => dispose())
  api.defaults.adapter = originalAdapter
  useAuthStore.getState().auth.reset('idle')
  useInspectorStore.setState({ current: null })
  localStorage.clear()
  vi.restoreAllMocks()
})

async function mountRouting(
  options: {
    lite?: boolean
    hideConnections?: boolean
    read?: boolean
    holdStatus?: boolean
  } = {}
) {
  const user = {
    id: 1,
    username: 'routing-reader',
    role: ROLE.ADMIN,
    permissions: {
      admin_permissions: { channel: { read: true, write: false } },
    },
  }
  const session = {
    sid: 'routing-session-one',
    current: true,
    login_method: 'password',
    ip: '127.0.0.1',
    user_agent: 'test',
    created_at: 1,
    last_active_at: 1,
    expires_at: 4_000_000_000,
  }
  const bundle = {
    access_token: 'synthetic-routing-access',
    token_type: 'Bearer',
    access_expires_at: 4_000_000_000,
    user,
    session,
  }
  useAuthStore.getState().auth.setBundle(bundle)
  const status = {
    product_edition: options.lite ? 'lite' : 'full',
    capabilities: Object.fromEntries(
      PRODUCT_CAPABILITIES.map((capability) => [capability, false])
    ),
    api_info_enabled: false,
    announcements_enabled: false,
    faq_enabled: false,
    uptime_kuma_enabled: false,
    user_funding_mode: 'disabled',
    user_funding_capabilities: { mode: 'disabled', epoch: 1, ready: true },
    ...(options.hideConnections
      ? {
          SidebarModulesAdmin: JSON.stringify({
            admin: { enabled: true, channel: false },
          }),
        }
      : {}),
  }
  if (options.holdStatus) {
    localStorage.setItem('status', JSON.stringify(status))
  }
  let releaseStatus = () => {}
  const statusGate = options.holdStatus
    ? new Promise<void>((resolve) => {
        releaseStatus = resolve
      })
    : Promise.resolve()
  let releaseLists = () => {}
  let listGate: Promise<void> | undefined
  let failList = false
  let items = [CONNECTION]
  const reads: { method: string; path: string }[] = []
  const adapter: AxiosAdapter = async (config) => {
    const path = (config.url ?? '').split('?')[0]
    const method = config.method ?? 'get'
    reads.push({ method, path })
    if (method !== 'get') {
      throw new Error('Routing entry must not write or contact a provider')
    }
    let data: unknown
    if (path === '/api/status') {
      await statusGate
      data = { success: true, data: status }
    } else if (path === '/api/user/self') {
      const actor = useAuthStore.getState().auth.user
      data = {
        success: true,
        data: {
          ...actor,
          permissions: {
            admin_permissions: {
              channel: { read: options.read ?? true, write: false },
            },
          },
        },
      }
    } else if (path === '/api/channel') {
      if (listGate) {
        await listGate
      }
      data = failList
        ? { success: false }
        : { success: true, data: { items, total: items.length } }
    } else if (path === '/api/channel/7') {
      data = { success: true, data: CONNECTION }
    } else if (path === '/api/channel/quota/changes') {
      data = {
        success: true,
        data: {
          items: [],
          total_items: 0,
          returned_items: 0,
          source_complete: true,
          items_complete: true,
        },
      }
    } else if (path === '/api/log' || path === '/api/log/self') {
      data = { success: true, data: { items: [], total: 0 } }
    } else if (path === '/pg/keys') {
      data = {
        success: true,
        data: {
          items: [
            {
              id: 21,
              name: 'Saved client Key',
              status: 1,
              group: 'default',
              remain_quota: 100,
              unlimited_quota: false,
              expired_time: -1,
            },
          ],
          total: 1,
        },
      }
    } else if (path === '/api/full-content-logs') {
      data = { success: true, data: { items: [], total: 0, enabled: false } }
    } else {
      throw new Error(`Unexpected routing-entry read: ${path}`)
    }
    return { config, data, status: 200, statusText: 'OK', headers: {} }
  }
  api.defaults.adapter = adapter
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: Infinity, staleTime: Infinity },
    },
  })
  const root = createRootRoute({ component: Outlet })
  const auth = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
    component: Outlet,
  })
  const overview = createRoute({
    getParentRoute: () => auth,
    path: 'dashboard/$section',
    component: OverviewPage,
    validateSearch: overviewSearchSchema,
  })
  const connection = createRoute({
    getParentRoute: () => auth,
    path: 'connections/$connectionId/',
    component: connectionRoute.options.component,
    validateSearch: connectionRoute.options.validateSearch,
    beforeLoad: (context) =>
      connectionRoute.options.beforeLoad?.(context as never),
  })
  const connections = createRoute({
    getParentRoute: () => auth,
    path: 'connections/',
    component: () => <p>Connection inventory</p>,
  })
  const forbidden = createRoute({
    getParentRoute: () => auth,
    path: '403',
    component: () => <p>Forbidden</p>,
  })
  const router = createRouter({
    routeTree: root.addChildren([
      auth.addChildren([overview, connection, connections, forbidden]),
    ]),
    history: createMemoryHistory({ initialEntries: [ENTRY] }),
  })
  await router.load()
  const view = render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <RouterProvider router={router} />
      </TooltipProvider>
    </QueryClientProvider>
  )
  views.push(() => {
    view.unmount()
    client.clear()
  })
  return {
    router,
    client,
    reads,
    bundle,
    user: userEvent.setup(),
    releaseStatus,
    holdLists() {
      listGate = new Promise<void>((resolve) => {
        releaseLists = () => {
          listGate = undefined
          resolve()
        }
      })
      void client.invalidateQueries({
        queryKey: ['overview', 'connections'],
        refetchType: 'none',
      })
    },
    releaseLists: () => releaseLists(),
    removeConnection() {
      items = [{ ...CONNECTION, id: 8, name: 'Different saved connection' }]
    },
    failLists() {
      failList = true
    },
  }
}

async function routingLink() {
  return screen.findByRole('link', { name: 'Routing and models' })
}

async function enterRouting(fixture: Awaited<ReturnType<typeof mountRouting>>) {
  const link = await routingLink()
  const scroller = link.closest<HTMLElement>('[data-page-scroll]')
  expect(scroller).not.toBeNull()
  if (!scroller) {
    throw new Error('Overview page scroller missing')
  }
  scroller.scrollTop = 270
  scroller.scrollLeft = 5
  const originalHref = fixture.router.state.location.href
  await fixture.user.click(link)
  await screen.findByRole('heading', { level: 1, name: CONNECTION.name })
  expect(fixture.router.state.location.pathname).toBe('/connections/7')
  expect(fixture.router.state.location.search).toEqual({ tab: 'routing' })
  expect(fixture.router.state.location.state.inspectorBack).toBe(true)
  expect(
    screen.getByRole('tab', { name: 'Routing and models' })
  ).toHaveAttribute('aria-selected', 'true')
  return originalHref
}

async function returnThroughInspector(
  fixture: Awaited<ReturnType<typeof mountRouting>>
) {
  await waitFor(() =>
    expect(useInspectorStore.getState().current).not.toBeNull()
  )
  await act(async () => useInspectorStore.getState().current?.close())
  await waitFor(() =>
    expect(fixture.router.state.location.pathname).toBe('/dashboard/overview')
  )
}

test('Full routing shortcut opens the real routing tab and inspector Back restores the complete URL, exact Link focus and page scroll', async () => {
  const fixture = await mountRouting()
  const originalHref = await enterRouting(fixture)
  expect(
    screen.getByText(
      'Configuration status does not confirm upstream availability.'
    )
  ).toBeInTheDocument()
  await returnThroughInspector(fixture)
  const returned = await routingLink()
  await waitFor(() => expect(returned).toHaveFocus())
  expect(fixture.router.state.location.href).toBe(originalHref)
  expect(returned).toHaveAttribute('data-overview-routing-link', '7')
  expect(returned.closest('[data-page-scroll]')).toHaveProperty(
    'scrollTop',
    270
  )
  expect(returned.closest('[data-page-scroll]')).toHaveProperty('scrollLeft', 5)
  expect(fixture.reads.every((request) => request.method === 'get')).toBe(true)
  expect(
    fixture.reads.some((request) => /test|balance|\/key\//.test(request.path))
  ).toBe(false)
})

test.each([false, true])(
  'Back waits for a fresh response even with cached connection rows; newer user input cancels restoration: %s',
  async (cancel) => {
    const fixture = await mountRouting()
    await enterRouting(fixture)
    const previousReads = fixture.reads.filter(
      (request) => request.path === '/api/channel'
    ).length
    fixture.holdLists()
    await returnThroughInspector(fixture)
    await waitFor(() =>
      expect(
        fixture.reads.filter((request) => request.path === '/api/channel')
          .length
      ).toBeGreaterThan(previousReads)
    )
    const cached = await routingLink()
    expect(cached).toHaveAttribute('data-overview-routing-link', '7')
    expect(cached).not.toHaveFocus()
    expect(cached.closest('[data-page-scroll]')).toHaveProperty('scrollTop', 0)
    if (cancel) {
      await fixture.user.keyboard('{Escape}')
    }
    await act(async () => fixture.releaseLists())
    await waitFor(() => expect(fixture.client.isFetching()).toBe(0))
    const returned = await routingLink()
    if (cancel) {
      expect(returned).not.toHaveFocus()
      expect(returned.closest('[data-page-scroll]')).toHaveProperty(
        'scrollTop',
        0
      )
    } else {
      await waitFor(() => expect(returned).toHaveFocus())
      expect(returned.closest('[data-page-scroll]')).toHaveProperty(
        'scrollTop',
        270
      )
    }
  }
)

test.each(['actor', 'session', 'role'] as const)(
  'a changed %s immediately discards the return point even if the original identity returns before Back',
  async (change) => {
    const fixture = await mountRouting()
    await enterRouting(fixture)
    await act(async () =>
      useAuthStore.getState().auth.setBundle({
        ...fixture.bundle,
        user: {
          ...fixture.bundle.user,
          id: change === 'actor' ? 2 : 1,
          role: change === 'role' ? ROLE.SUPER_ADMIN : ROLE.ADMIN,
        },
        session: {
          ...fixture.bundle.session,
          sid:
            change === 'session'
              ? 'routing-session-two'
              : fixture.bundle.session.sid,
        },
      })
    )
    await screen.findByRole('heading', { level: 1, name: CONNECTION.name })
    await act(async () =>
      useAuthStore.getState().auth.setBundle(fixture.bundle)
    )
    await screen.findByRole('heading', { level: 1, name: CONNECTION.name })
    await returnThroughInspector(fixture)
    const returned = await routingLink()
    expect(returned).not.toHaveFocus()
    expect(returned.closest('[data-page-scroll]')).toHaveProperty(
      'scrollTop',
      0
    )
  }
)

test.each(['removed', 'failed'] as const)(
  'Back never borrows another connection or restores focus after the source is %s',
  async (outcome) => {
    const fixture = await mountRouting()
    await enterRouting(fixture)
    fixture.holdLists()
    if (outcome === 'removed') {
      fixture.removeConnection()
    } else {
      fixture.failLists()
    }
    await returnThroughInspector(fixture)
    await act(async () => fixture.releaseLists())
    if (outcome === 'removed') {
      const other = await waitFor(() => {
        const link = screen.getByRole('link', { name: 'Routing and models' })
        expect(link).toHaveAttribute('data-overview-routing-link', '8')
        expect(
          screen
            .getByTestId('overview')
            .querySelector('[data-overview-routing-link="7"]')
        ).not.toBeInTheDocument()
        return link
      })
      expect(other).toHaveAttribute('data-overview-routing-link', '8')
      expect(other).not.toHaveFocus()
      expect(other.closest('[data-page-scroll]')).toHaveProperty('scrollTop', 0)
    } else {
      await screen.findByText('Connections could not be loaded.')
      expect(
        screen.queryByRole('link', { name: 'Routing and models' })
      ).not.toBeInTheDocument()
    }
  }
)

test('modifier clicks and an independent routing entry do not capture or consume an overview return point', async () => {
  const fixture = await mountRouting()
  const link = await routingLink()
  fireEvent.click(link, { ctrlKey: true })
  fireEvent.click(link, { metaKey: true })
  fireEvent.click(link, { shiftKey: true })
  fireEvent.click(link, { altKey: true })
  fireEvent.click(link, { button: 1 })
  expect(fixture.router.state.location.pathname).toBe('/dashboard/overview')
  await act(async () =>
    fixture.router.navigate({
      to: '/connections/$connectionId',
      params: { connectionId: '7' },
      search: { tab: 'routing' },
      state: { inspectorBack: true },
    })
  )
  await screen.findByRole('heading', { level: 1, name: CONNECTION.name })
  await returnThroughInspector(fixture)
  const returned = await routingLink()
  expect(returned).not.toHaveFocus()
  expect(returned.closest('[data-page-scroll]')).toHaveProperty('scrollTop', 0)
})

test.each([{ lite: true }, { hideConnections: true }, { read: false }])(
  'Lite, hidden navigation or a denied fresh channel grant omit Full routing tools: %j',
  async (options) => {
    const fixture = await mountRouting(options)
    await screen.findByTestId('overview')
    await screen.findByText('MyAPI gateway')
    await waitFor(() => expect(fixture.client.isFetching()).toBe(0))
    expect(
      screen.queryByRole('link', { name: 'Routing and models' })
    ).not.toBeInTheDocument()
  }
)

test('a placeholder Full status cannot expose routing tools until the server confirms it', async () => {
  const fixture = await mountRouting({ holdStatus: true })
  await screen.findByRole('button', { name: /Saved routing connection/ })
  expect(
    screen.queryByRole('link', { name: 'Routing and models' })
  ).not.toBeInTheDocument()
  await act(async () => fixture.releaseStatus())
  expect(await routingLink()).toHaveAttribute(
    'href',
    '/connections/7?tab=routing'
  )
})
