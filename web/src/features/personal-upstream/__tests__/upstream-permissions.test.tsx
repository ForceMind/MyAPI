/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import type { AxiosAdapter } from 'axios'
import type { ReactNode } from 'react'
import { expect, test } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { useUpstreamAccounts } from '../use-upstream-accounts'

test.each([
  { label: 'a role downgrade', role: ROLE.SUPER_ADMIN, nextRole: ROLE.USER },
  {
    label: 'a channel.read revocation',
    role: ROLE.ADMIN,
    nextRole: ROLE.ADMIN,
  },
])(
  '$label in the same session confirms permissions again and stops channel reads',
  async (scenario) => {
    const user = {
      id: 7,
      username: 'synthetic-root',
      role: scenario.role,
      permissions: { admin_permissions: { channel: { read: true } } },
    }
    useAuthStore.getState().auth.setBundle({
      access_token: 'synthetic-window-permissions',
      token_type: 'Bearer',
      access_expires_at: 4_000_000_000,
      user,
      session: {
        sid: 'synthetic-window-session',
        current: true,
        login_method: 'password',
        ip: '127.0.0.1',
        user_agent: 'test',
        created_at: 1,
        last_active_at: 1,
        expires_at: 4_000_000_000,
      },
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    })
    function Wrapper(props: { children: ReactNode }) {
      return (
        <QueryClientProvider client={client}>
          {props.children}
        </QueryClientProvider>
      )
    }
    let servedRole: number = scenario.role
    let servedRead = true
    let selfReads = 0
    let quotaReads = 0
    const previousAdapter = api.defaults.adapter
    const adapter: AxiosAdapter = async (config) => {
      let data
      if (config.url === '/api/user/self') {
        selfReads++
        data = {
          success: true,
          data: {
            ...user,
            role: servedRole,
            permissions: {
              admin_permissions: { channel: { read: servedRead } },
            },
          },
        }
      } else {
        expect(config.url).toBe('/api/channel/quota/changes')
        quotaReads++
        data = { success: true, data: { items: [] } }
      }
      return { data, status: 200, statusText: 'OK', headers: {}, config }
    }
    api.defaults.adapter = adapter
    const observer = renderHook(() => useUpstreamAccounts(), {
      wrapper: Wrapper,
    })
    try {
      await waitFor(() =>
        expect(observer.result.current.accountsQuery.isSuccess).toBe(true)
      )
      expect(selfReads).toBe(1)
      expect(quotaReads).toBe(1)
      servedRole = scenario.nextRole
      servedRead = false
      useAuthStore
        .getState()
        .auth.setUser({
          ...user,
          role: scenario.nextRole,
          permissions: { admin_permissions: { channel: { read: false } } },
        })
      await waitFor(() => expect(selfReads).toBe(2))
      await waitFor(() =>
        expect(observer.result.current.selfQuery.data?.role).toBe(
          scenario.nextRole
        )
      )
      expect(observer.result.current.canRead).toBe(false)
      expect(observer.result.current.accountsQuery.data).toBeUndefined()
      expect(quotaReads).toBe(1)
    } finally {
      observer.unmount()
      client.clear()
      api.defaults.adapter = previousAdapter
      useAuthStore.getState().auth.reset('idle')
    }
  }
)
