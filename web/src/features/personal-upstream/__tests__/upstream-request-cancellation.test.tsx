/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import { CanceledError, type AxiosAdapter, type AxiosResponse } from 'axios'
import type { ReactNode } from 'react'
import { expect, test } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { useUpstreamAccounts } from '../use-upstream-accounts'

test('a remounted personal window observer owns its request after the previous observer aborts', async () => {
  const user = { id: 7, username: 'root', role: ROLE.SUPER_ADMIN }
  const sid = 'upstream-cancellation-session'
  useAuthStore.getState().auth.setBundle({
    access_token: 'synthetic-access',
    token_type: 'Bearer',
    access_expires_at: 4_000_000_000,
    user,
    session: {
      sid,
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
  // A validated self snapshot is already cached when the observer remounts.
  client.setQueryData(
    ['personal-console', 'self', user.id, sid, user.role, true],
    user
  )
  function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>
  }
  const items = ['primary', 'secondary'].map((slot, index) => ({
    channel_id: 1,
    series_id: slot,
    name: 'Synthetic account',
    metric_type: 'codex_rate_limit',
    window_type: index === 0 ? 'five_hour' : 'weekly',
    unit: 'percent',
    current_available: index === 0 ? 45 : 32,
    status: 'success',
  }))
  let adapterEntered!: () => void
  const firstEntered = new Promise<void>((resolve) => {
    adapterEntered = resolve
  })
  let cancelFirst!: () => void
  let aborted = false
  let quotaCalls = 0
  const originalAdapter = api.defaults.adapter
  const adapter: AxiosAdapter = (config) => {
    expect(config.url).toBe('/api/channel/quota/changes')
    expect(config.params).toEqual({
      range: '24h',
      limit: 100,
      sort: 'observed_desc',
    })
    quotaCalls++
    if (quotaCalls === 1) {
      config.signal?.addEventListener?.('abort', () => {
        aborted = true
      })
      return new Promise<AxiosResponse>((_resolve, reject) => {
        // Hold the adapter rejection until C2 starts. This models the real
        // interval between AbortSignal dispatch and Axios promise settlement.
        cancelFirst = () => {
          expect(config.signal?.aborted).toBe(true)
          reject(new CanceledError('Synthetic observer cancelled', config))
        }
        adapterEntered()
      })
    }
    expect(config.signal?.aborted).toBe(false)
    return Promise.resolve({
      config,
      data: { success: true, data: { items } },
      status: 200,
      statusText: 'OK',
      headers: {},
    })
  }
  api.defaults.adapter = adapter
  const first = renderHook(() => useUpstreamAccounts(), { wrapper: Wrapper })
  let unmountSecond: (() => void) | undefined
  try {
    await firstEntered
    first.unmount()
    expect(aborted).toBe(true)
    // No await separates C1 cancellation and C2 creation: C1's HTTP promise
    // is still pending when the same account-window read starts again.
    const second = renderHook(() => useUpstreamAccounts(), { wrapper: Wrapper })
    unmountSecond = second.unmount
    cancelFirst()
    await waitFor(() =>
      expect(second.result.current.accountsQuery.isSuccess).toBe(true)
    )
    expect(second.result.current.canRead).toBe(true)
    expect(second.result.current.accountsQuery.data?.items).toEqual(items)
    expect(second.result.current.accountsQuery.isError).toBe(false)
    expect(quotaCalls).toBe(2)
  } finally {
    first.unmount()
    unmountSecond?.()
    client.clear()
    api.defaults.adapter = originalAdapter
    useAuthStore.getState().auth.reset('idle')
  }
})
