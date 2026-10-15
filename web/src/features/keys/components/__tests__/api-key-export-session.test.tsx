/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { apiKeySchema } from '../../types'
import { ApiKeysProvider } from '../api-keys-provider'
import { DataTableRowActions } from '../data-table-row-actions'

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}))
const { toast } = await import('sonner')
const originalAuth = useAuthStore.getState()
const originalConfig = useSystemConfigStore.getState()
const key = apiKeySchema.parse({
  id: 21,
  name: 'Synthetic session Key',
  key: 'masked',
  status: 1,
  remain_quota: 100,
  used_quota: 0,
  unlimited_quota: false,
  expired_time: -1,
  created_time: 1,
  accessed_time: 1,
  model_limits_enabled: false,
})
let client: QueryClient
type RevealResponse = { data: { success: boolean; data?: { key: string } } }

function deferredReveal() {
  let resolve!: (response: RevealResponse) => void
  let reject!: (error: Error) => void
  const promise = new Promise<RevealResponse>((accept, deny) => {
    resolve = accept
    reject = deny
  })
  return { promise, resolve, reject }
}

function changeIdentity(kind: 'user' | 'role' | 'session') {
  useAuthStore.setState((state) => ({
    ...state,
    auth: {
      ...state.auth,
      user: {
        id: kind === 'user' ? 2 : 1,
        role: kind === 'role' ? 1 : 100,
        username: 'operator',
      },
      session: state.auth.session && {
        ...state.auth.session,
        sid: kind === 'session' ? 'next-session' : state.auth.session.sid,
      },
    },
  }))
}

async function startExport(name = 'Synthetic chat') {
  const user = userEvent.setup()
  const view = render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <ApiKeysProvider>
          <DataTableRowActions apiKey={key} />
        </ApiKeysProvider>
      </TooltipProvider>
    </QueryClientProvider>
  )
  await waitFor(() => expect(api.get).toHaveBeenCalledWith('/api/status'))
  await user.click(screen.getByRole('button', { name: 'Open menu' }))
  const chat = await screen.findByRole('menuitem', {
    name: 'Chat',
  })
  chat.focus()
  await user.keyboard('{ArrowRight}')
  await user.click(await screen.findByRole('menuitem', { name }))
  expect(api.post).toHaveBeenCalledExactlyOnceWith(
    '/api/token/21/key',
    undefined,
    {
      skipErrorHandler: true,
      skipBusinessError: true,
      skipAuthRefresh: true,
    }
  )
  return view
}

beforeEach(() => {
  localStorage.clear()
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  useAuthStore.setState((state) => ({
    ...state,
    auth: {
      ...state.auth,
      user: { id: 1, role: 100, username: 'operator' },
      session: {
        sid: 'original-session',
        current: true,
        login_method: 'password',
        ip: '',
        user_agent: '',
        created_at: 1,
        last_active_at: 1,
        expires_at: 2,
      },
    },
  }))
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/status')
      return {
        data: {
          success: true,
          data: {
            chats: [
              { 'Synthetic chat': 'https://chat.example.test/?key={key}' },
              { 'Synthetic Fluent': 'fluent://prefill' },
            ],
          },
        },
      }
    throw new Error(`Unexpected GET ${url}`)
  })
  vi.spyOn(window, 'open').mockReturnValue(null)
  vi.mocked(toast.success).mockClear()
  vi.mocked(toast.error).mockClear()
  vi.mocked(toast.info).mockClear()
})

afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.setState(originalAuth)
  useSystemConfigStore.setState(originalConfig)
  localStorage.clear()
  document.getElementById('fluent-new-api-container')?.remove()
  vi.restoreAllMocks()
})

test.each(['unmount', 'user', 'role', 'session'] as const)(
  'a delayed Key reveal cannot open a chat after %s changes',
  async (change) => {
    const pending = deferredReveal()
    vi.spyOn(api, 'post').mockReturnValue(pending.promise)
    const view = await startExport()
    await act(async () => {
      if (change === 'unmount') view.unmount()
      else changeIdentity(change)
      pending.resolve({
        data: { success: true, data: { key: 'synthetic-session-secret' } },
      })
    })
    expect(window.open).not.toHaveBeenCalled()
    expect(api.post).toHaveBeenCalledOnce()
    expect(toast.success).not.toHaveBeenCalled()
    expect(toast.error).not.toHaveBeenCalled()
  }
)

test('a delayed Key reveal cannot dispatch to Fluent after its session changes', async () => {
  const pending = deferredReveal()
  vi.spyOn(api, 'post').mockReturnValue(pending.promise)
  const container = document.createElement('div')
  container.id = 'fluent-new-api-container'
  const prefill = vi.fn()
  container.addEventListener('fluent:prefill', prefill)
  document.body.appendChild(container)
  await startExport('Synthetic Fluent')
  await act(async () => {
    changeIdentity('session')
    pending.resolve({
      data: { success: true, data: { key: 'synthetic-session-secret' } },
    })
  })
  expect(prefill).not.toHaveBeenCalled()
  expect(window.open).not.toHaveBeenCalled()
  expect(toast.success).not.toHaveBeenCalled()
})

test.each(['unmount', 'session'] as const)(
  'a delayed Key reveal failure after %s has no stale notification or automatic retry',
  async (change) => {
    const pending = deferredReveal()
    vi.spyOn(api, 'post').mockReturnValue(pending.promise)
    const view = await startExport()
    await act(async () => {
      if (change === 'unmount') view.unmount()
      else changeIdentity(change)
      pending.reject(new Error('synthetic reveal disconnected'))
    })
    expect(window.open).not.toHaveBeenCalled()
    expect(api.post).toHaveBeenCalledOnce()
    expect(toast.error).not.toHaveBeenCalled()
  }
)

test('a current session exports its explicitly selected Key once', async () => {
  const pending = deferredReveal()
  vi.spyOn(api, 'post').mockReturnValue(pending.promise)
  await startExport()
  await act(async () =>
    pending.resolve({
      data: { success: true, data: { key: 'synthetic-session-secret' } },
    })
  )
  expect(window.open).toHaveBeenCalledExactlyOnceWith(
    'https://chat.example.test/?key=sk-synthetic-session-secret',
    '_blank',
    'noopener'
  )
  expect(api.post).toHaveBeenCalledOnce()
})

test.each(['network failure', 'business rejection'] as const)(
  'a current Key reveal %s reports its failure once without exporting or replaying',
  async (failure) => {
    const pending = deferredReveal()
    vi.spyOn(api, 'post').mockReturnValue(pending.promise)
    await startExport()
    await act(async () => {
      if (failure === 'network failure')
        pending.reject(new Error('synthetic reveal disconnected'))
      else pending.resolve({ data: { success: false } })
    })
    expect(toast.error).toHaveBeenCalledOnce()
    expect(window.open).not.toHaveBeenCalled()
    expect(api.post).toHaveBeenCalledOnce()
  }
)
