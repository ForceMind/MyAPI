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
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
  useLocation,
} from '@tanstack/react-router'
import { render } from '@testing-library/react'
import { AxiosError, CanceledError, type AxiosAdapter } from 'axios'
import { vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { Route as connectionPageRoute } from '@/routes/_authenticated/connections/$connectionId/index'
import { Route as connectionsRoute } from '@/routes/_authenticated/connections/index'
import { useAuthStore } from '@/stores/auth-store'
import { useInspectorStore } from '@/stores/inspector-store'

export const CONNECTION = {
  id: 7,
  type: 57,
  name: 'Synthetic connection',
  status: 1,
  models: 'alpha,beta',
  group: 'default',
  priority: 0,
  weight: 0,
  test_time: 0,
  response_time: 0,
  key: 'SECRET-SENTINEL',
}

export type ConnectionsFixtureOptions = {
  entry?: string
  mobile?: boolean
  read?: boolean
  write?: boolean
  operate?: boolean
  failDetail?: boolean
  windowsFail?: boolean
  wrongWindow?: boolean
  windows?: Record<string, unknown>[]
  empty?: boolean
  holdList?: boolean
  failList?: boolean
  missingRow?: boolean
  holdDetail?: boolean
  freshId?: number
  testOutcome?: 'success' | 'failure'
  logs?: Record<string, unknown>[]
  failLogs?: boolean
  /** Other reads answered as `{ success: true, data }`, by path. */
  extraReads?: Record<string, unknown>
  items?: Record<string, unknown>[]
  total?: number
}

/** Width of the emulated viewport for the console breakpoints. */
function setViewport(width: number) {
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    value: vi.fn((query: string) => {
      const max = /max-width:\s*(\d+)px/.exec(query)
      const min = /min-width:\s*(\d+)px/.exec(query)
      let matches = false
      if (max) matches = width <= Number(max[1])
      if (min) matches = width >= Number(min[1])
      return {
        matches,
        media: query,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }
    }),
  })
}

let sessionCounter = 0

/**
 * The real connection grid and connection page routes (with their redirects
 * of earlier links), answered from in-memory fixtures. Writes are the
 * existing status action and connection test, and are recorded.
 */
export async function mountConnections(
  initial: ConnectionsFixtureOptions = {}
) {
  // Mutable, so a test can change what the next read answers.
  const options = { ...initial }
  // 390px phone or 1440px desktop.
  setViewport(options.mobile ? 390 : 1440)
  const grants = {
    read: true,
    write: true,
    ...(options.operate ? { operate: true } : {}),
  }
  const user = {
    id: 7,
    username: 'synthetic-operator',
    role: ROLE.ADMIN,
    permissions: { admin_permissions: { channel: grants } },
  }
  // A distinct session per mount keeps test results from leaking across.
  sessionCounter += 1
  const session = {
    sid: `connections-test-session-${sessionCounter}`,
    current: true,
    login_method: 'password',
    ip: '127.0.0.1',
    user_agent: 'test',
    created_at: 1,
    last_active_at: 1,
    expires_at: 4_000_000_000,
  }
  useAuthStore.getState().auth.setBundle({
    access_token: 'synthetic-connection-access',
    token_type: 'Bearer',
    access_expires_at: 4_000_000_000,
    user,
    session,
  })
  const requests: {
    method: string
    path: string
    params: Record<string, unknown>
  }[] = []
  // List reads wait while a gate is set (`holdList`, `holdLists()`).
  let release = () => {}
  let listGate: Promise<void> | null = null
  const holdLists = () => {
    listGate = new Promise<void>((resolve) => {
      release = () => {
        listGate = null
        resolve()
      }
    })
  }
  if (options.holdList) holdLists()
  let failDetail = options.failDetail
  let detailAborted = false
  const status = new Map<number, number>()
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  type Item = Record<string, unknown> & { id: number; status: number }
  const items = () =>
    (
      (options.items as Item[] | undefined) ?? [
        { ...CONNECTION, id: options.missingRow ? 8 : 7 },
      ]
    ).map((item) => ({ ...item, status: status.get(item.id) ?? item.status }))
  const adapter: AxiosAdapter = async (config) => {
    const [path, query = ''] = (config.url ?? '').split('?')
    const params = {
      ...Object.fromEntries(new URLSearchParams(query)),
      ...config.params,
    }
    const method = config.method ?? 'get'
    requests.push({ method, path, params })
    let data: unknown
    if (method !== 'get') {
      const statusWrite = /^\/api\/channel\/(\d+)\/status$/.exec(path)
      if (method === 'post' && statusWrite) {
        status.set(Number(statusWrite[1]), JSON.parse(config.data).status)
        data = { success: true, data: true }
      } else throw new Error(`Unexpected write: ${method} ${path}`)
    } else if (options.extraReads && Object.hasOwn(options.extraReads, path)) {
      data = { success: true, data: options.extraReads[path] }
    } else if (path === '/api/user/self') {
      data = {
        success: true,
        data: {
          ...user,
          id: options.freshId ?? user.id,
          permissions: {
            admin_permissions: {
              channel: {
                ...grants,
                read: options.read ?? true,
                write: options.write ?? true,
              },
            },
          },
        },
      }
    } else if (path === '/api/channel' || path === '/api/channel/search') {
      if (listGate) await listGate
      if (options.failList) throw new Error('Inventory unavailable')
      data = {
        success: true,
        data: {
          total: options.empty ? 0 : (options.total ?? items().length),
          items: options.empty ? [] : items(),
        },
      }
    } else if (/^\/api\/channel\/\d+$/.test(path)) {
      const id = Number(path.split('/').at(-1))
      if (options.holdDetail) {
        return new Promise((_resolve, reject) => {
          config.signal?.addEventListener?.('abort', () => {
            detailAborted = true
            reject(new CanceledError('Observer left', config))
          })
        })
      }
      if (failDetail) {
        throw new AxiosError(
          'Service unavailable',
          'ERR_BAD_RESPONSE',
          config,
          undefined,
          {
            config,
            status: 503,
            statusText: 'Unavailable',
            headers: {},
            data: { success: false },
          }
        )
      }
      const item = items().find((entry) => entry.id === id) ?? {
        ...CONNECTION,
        id,
        name: `Synthetic connection ${id}`,
      }
      data = {
        success: true,
        data: { ...item, status: status.get(id) ?? item.status },
      }
    } else if (path === '/api/channel/quota/changes') {
      let windows = options.windows ?? []
      if (options.wrongWindow) {
        windows = [
          {
            channel_id: 8,
            name: 'Wrong connection',
            metric_type: 'codex_rate_limit',
            unit: 'percent',
          },
        ]
      }
      data = options.windowsFail
        ? { success: false }
        : { success: true, data: { items: windows } }
    } else if (path === '/api/log' && options.failLogs) {
      data = { success: false, message: 'Synthetic failure' }
    } else if (path === '/api/log') {
      let rows = options.logs ?? []
      if (params.channel) {
        rows = rows.filter(
          (row) => String(row.channel) === String(params.channel)
        )
      }
      if (params.type) {
        rows = rows.filter((row) => String(row.type) === String(params.type))
      }
      if (params.result === 'failed') {
        rows = rows.filter(
          (row) =>
            row.type === 5 &&
            !String(row.other ?? '').includes('"preflight_rejected":true')
        )
      }
      data = { success: true, data: { items: rows, total: rows.length } }
    } else if (path === '/pg/keys') {
      data = { success: true, data: { items: [], total: 0 } }
    } else if (path.startsWith('/api/channel/test/')) {
      data =
        options.testOutcome === 'failure'
          ? {
              success: false,
              message: 'Upstream rejected the key',
              error_code: 'invalid_api_key',
            }
          : { success: true, time: 1.25 }
    } else throw new Error(`Unexpected request: ${path}`)
    return { config, status: 200, statusText: 'OK', headers: {}, data }
  }
  api.defaults.adapter = adapter

  const root = createRootRoute({ component: Outlet })
  const authenticated = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
    component: Outlet,
  })
  const grid = createRoute({
    getParentRoute: () => authenticated,
    path: 'connections/',
    validateSearch: connectionsRoute.options.validateSearch,
    beforeLoad: (beforeLoad) =>
      connectionsRoute.options.beforeLoad?.(beforeLoad as never),
    component: connectionsRoute.options.component,
  })
  const page = createRoute({
    getParentRoute: () => authenticated,
    path: 'connections/$connectionId/',
    validateSearch: connectionPageRoute.options.validateSearch,
    beforeLoad: (beforeLoad) =>
      connectionPageRoute.options.beforeLoad?.(beforeLoad as never),
    component: connectionPageRoute.options.component,
  })
  const stub = (path: string, label: string) =>
    createRoute({
      getParentRoute: () => authenticated,
      path,
      component: function Stub() {
        const location = useLocation()
        return (
          <p>
            {label} {location.href}
          </p>
        )
      },
    })
  const router = createRouter({
    routeTree: root.addChildren([
      authenticated.addChildren([
        grid,
        page,
        stub('channels/', 'Table view page'),
        stub('keys/', 'Keys page'),
        stub('usage-logs/$section', 'Call records page'),
        stub('usage-logs/request/$requestId', 'Request page'),
        stub('403', 'Forbidden page'),
      ]),
    ]),
    history: createMemoryHistory({
      initialEntries: [options.entry ?? '/connections'],
    }),
  })
  await router.load()
  const view = render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <RouterProvider router={router} />
      </TooltipProvider>
    </QueryClientProvider>
  )
  return {
    view,
    router,
    client,
    requests,
    releaseList: () => release(),
    holdLists,
    user,
    session,
    recover: () => {
      failDetail = false
    },
    state: options,
    get detailAborted() {
      return detailAborted
    },
    teardown: () => {
      view.unmount()
      client.clear()
      useInspectorStore.setState({ current: null })
    },
  }
}

export function connectionLog(
  overrides: Record<string, unknown> & { id: number }
) {
  return {
    user_id: 2,
    created_at: Math.floor(Date.now() / 1000) - overrides.id * 60,
    type: 2,
    content: '',
    username: 'teammate',
    token_name: 'Team Key',
    token_id: 21,
    model_name: 'alpha',
    quota: 0,
    prompt_tokens: 10,
    completion_tokens: 5,
    use_time: 2,
    is_stream: false,
    channel: 7,
    channel_name: 'Synthetic connection',
    group: 'default',
    ip: '',
    other: JSON.stringify({
      request_outcome: 'completed',
      usage_outcome: 'reported',
    }),
    request_id: `REQ-${overrides.id}`,
    upstream_request_id: '',
    ...overrides,
  }
}
