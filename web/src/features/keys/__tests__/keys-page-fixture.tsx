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
} from '@tanstack/react-router'
import { render } from '@testing-library/react'
import { vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { Route as keyPageRoute } from '@/routes/_authenticated/keys/$keyId/index'
import { Route as keyRequestRoute } from '@/routes/_authenticated/keys/$keyId/requests/$requestId'
import { Route as keysRoute } from '@/routes/_authenticated/keys/index'
import { useAuthStore } from '@/stores/auth-store'

import { apiKeySchema, type ApiKey } from '../types'

export const KEY_21 = apiKeySchema.parse({
  id: 21,
  name: 'Desktop application',
  key: 'masked-value',
  status: 1,
  remain_quota: 123456,
  used_quota: 100,
  unlimited_quota: false,
  expired_time: -1,
  created_time: 1,
  accessed_time: 0,
  group: 'default',
  model_limits_enabled: false,
})

export const KEY_22 = apiKeySchema.parse({
  ...KEY_21,
  id: 22,
  name: 'Shared device',
})

export function requestLog(
  overrides: Partial<Record<string, unknown>> & { id: number }
) {
  return {
    user_id: 1,
    created_at: 1_797_000_000 - overrides.id,
    type: 2,
    content: '',
    username: 'owner',
    token_name: KEY_21.name,
    token_id: 21,
    model_name: 'synthetic-model',
    quota: 0,
    prompt_tokens: 10,
    completion_tokens: 5,
    use_time: 2,
    is_stream: true,
    channel: 1,
    channel_name: 'Synthetic connection',
    group: 'default',
    ip: '',
    other: JSON.stringify({
      request_outcome: 'completed',
      usage_outcome: 'reported',
      http_status: 200,
    }),
    request_id: `REQ-${overrides.id}`,
    upstream_request_id: '',
    ...overrides,
  }
}

export type KeysFixture = {
  keys: ApiKey[]
  logs: ReturnType<typeof requestLog>[]
  status: Record<string, unknown>
}

/** Answers the Keys page reads from in-memory fixtures; writes are spies. */
export function mockKeysApi(
  fixture: KeysFixture & {
    extraGet?: (path: string) => { data: unknown } | undefined
  }
) {
  const get = vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
    const path = String(url)
    const extra = fixture.extraGet?.(path)
    if (extra) return extra
    if (path === '/api/status') {
      return { data: { success: true, data: fixture.status } }
    }
    if (
      path.startsWith('/api/token/?') ||
      path.startsWith('/api/token/search?')
    ) {
      return {
        data: {
          success: true,
          data: {
            items: fixture.keys,
            total: fixture.keys.length,
            page: 1,
            page_size: 20,
          },
        },
      }
    }
    const single = path.match(/^\/api\/token\/(\d+)$/)
    if (single) {
      const key = fixture.keys.find((item) => item.id === Number(single[1]))
      return key
        ? { data: { success: true, data: key } }
        : { data: { success: false, message: 'not found' } }
    }
    const share = path.match(/^\/api\/token\/(\d+)\/subscription-share$/)
    if (share) {
      return {
        data: {
          success: true,
          data: { token_id: Number(share[1]), pools: [] },
        },
      }
    }
    if (path.startsWith('/api/log/self?') || path.startsWith('/api/log?')) {
      const params = new URLSearchParams(path.split('?')[1])
      const tokenId = params.get('token_id')
      const requestId = params.get('request_id')
      const type = params.get('type')
      const items = fixture.logs.filter(
        (log) =>
          (!tokenId || String(log.token_id) === tokenId) &&
          (!requestId || log.request_id === requestId) &&
          (!type || String(log.type) === type)
      )
      return { data: { success: true, data: { items, total: items.length } } }
    }
    if (path.startsWith('/api/full-content-logs/')) {
      const error = Object.assign(new Error('not found'), {
        isAxiosError: true,
        response: { status: 404, data: { success: false } },
      })
      throw error
    }
    if (path === '/api/full-content-logs') {
      return {
        data: {
          success: true,
          data: {
            enabled: false,
            page: 1,
            page_size: 1,
            total: 0,
            items: [],
            files: { count: 0, total_size: 0, files: [] },
            facets: { models: [], tokens: [] },
          },
        },
      }
    }
    if (path === '/api/user/self/groups') {
      return {
        data: {
          success: true,
          data: { default: { desc: 'Default', ratio: 1 } },
        },
      }
    }
    void config
    throw new Error(`Unexpected GET ${path}`)
  })
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: { key: 'inert-existing-key' } },
  })
  const del = vi
    .spyOn(api, 'delete')
    .mockResolvedValue({ data: { success: true } })
  return { get, put, post, delete: del }
}

/** Width of the emulated viewport for the console breakpoints. */
export function setViewport(width: number) {
  vi.spyOn(window, 'matchMedia').mockImplementation((query: string) => {
    const max = query.match(/max-width:\s*(\d+)px/)
    const min = query.match(/min-width:\s*(\d+)px/)
    let matches = false
    if (max) matches = width <= Number(max[1])
    if (min) matches = width >= Number(min[1])
    return {
      matches,
      media: query,
      onchange: null,
      addListener: () => undefined,
      removeListener: () => undefined,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      dispatchEvent: () => false,
    }
  })
}

export function signIn(role = 100) {
  useAuthStore.setState((state) => ({
    ...state,
    auth: {
      ...state.auth,
      user: { id: 1, role, username: 'owner' },
      session: null,
    },
  }))
}

export async function renderKeysPage(client: QueryClient, entry = '/keys') {
  const root = createRootRoute({ component: Outlet })
  const authenticated = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
    component: Outlet,
  })
  // The real list, Key page and request page routes, including their
  // redirects of earlier inspector links.
  const keys = createRoute({
    getParentRoute: () => authenticated,
    path: 'keys/',
    validateSearch: keysRoute.options.validateSearch,
    beforeLoad: (options) => keysRoute.options.beforeLoad?.(options as never),
    component: keysRoute.options.component,
  })
  const keyPage = createRoute({
    getParentRoute: () => authenticated,
    path: 'keys/$keyId/',
    validateSearch: keyPageRoute.options.validateSearch,
    beforeLoad: (options) =>
      keyPageRoute.options.beforeLoad?.(options as never),
    component: keyPageRoute.options.component,
  })
  const keyRequest = createRoute({
    getParentRoute: () => authenticated,
    path: 'keys/$keyId/requests/$requestId',
    validateSearch: keyRequestRoute.options.validateSearch,
    beforeLoad: (options) =>
      keyRequestRoute.options.beforeLoad?.(options as never),
    component: keyRequestRoute.options.component,
  })
  const playground = createRoute({
    getParentRoute: () => authenticated,
    path: 'playground/',
    component: () => <p>Playground page</p>,
  })
  const logs = createRoute({
    getParentRoute: () => authenticated,
    path: 'usage-logs/$section',
    component: () => <p>Call records page</p>,
  })
  const router = createRouter({
    routeTree: root.addChildren([
      authenticated.addChildren([keys, keyPage, keyRequest, playground, logs]),
    ]),
    history: createMemoryHistory({ initialEntries: [entry] }),
  })
  await router.load()
  const rendered = render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <RouterProvider router={router} />
      </TooltipProvider>
    </QueryClientProvider>
  )
  return { ...rendered, router }
}

export function newClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
}
