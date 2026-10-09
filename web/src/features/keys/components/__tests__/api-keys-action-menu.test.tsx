/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { getCoreRowModel, useReactTable } from '@tanstack/react-table'
import { act, cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { DataTableView } from '@/components/data-table/core/data-table-view'
import { TooltipProvider } from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { apiKeySchema } from '../../types'
import { useApiKeysColumns } from '../api-keys-columns'
import { ApiKeysDialogs } from '../api-keys-dialogs'
import { ApiKeysProvider, useApiKeys } from '../api-keys-provider'

const keys = [
  apiKeySchema.parse({
    id: 21,
    name: 'Owner fixture',
    key: 'masked',
    status: 1,
    remain_quota: 100,
    used_quota: 0,
    unlimited_quota: false,
    expired_time: -1,
    created_time: 1,
    accessed_time: 1,
    group: 'default',
    model_limits_enabled: false,
  }),
]
const originalAuth = useAuthStore.getState()
const originalConfig = useSystemConfigStore.getState()
let client: QueryClient

function KeysTableFixture() {
  // The real table consumes this context and rebuilds columns when reveal state changes.
  useApiKeys()
  const columns = useApiKeysColumns(1_700_000_000_000)
  const table = useReactTable({
    data: keys,
    columns,
    getCoreRowModel: getCoreRowModel(),
  })
  return <DataTableView table={table} />
}

function renderKeys() {
  return render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <ApiKeysProvider>
          <KeysTableFixture />
          <ApiKeysDialogs />
        </ApiKeysProvider>
      </TooltipProvider>
    </QueryClientProvider>
  )
}

function deferredReveal() {
  let resolve!: (value: {
    data: { success: boolean; data?: { key: string } }
  }) => void
  let reject!: (reason: Error) => void
  const promise = new Promise<{
    data: { success: boolean; data?: { key: string } }
  }>((accept, deny) => {
    resolve = accept
    reject = deny
  })
  return { promise, resolve, reject }
}

beforeEach(() => {
  localStorage.clear()
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  useAuthStore
    .getState()
    .auth.setUser({ id: 2, role: 1, username: 'owner-fixture' })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/status') {
      return { data: { success: true, data: { chats: [] } } }
    }
    if (url === '/api/user/self/groups') {
      return {
        data: {
          success: true,
          data: { default: { desc: 'Default', ratio: 1 } },
        },
      }
    }
    if (url === '/api/token/21/access') {
      return {
        data: {
          success: true,
          data: {
            token_id: 21,
            assigned: true,
            user_revision: 1,
            token_revision: 1,
            models: [
              { model: 'owner-public-model', allowed: true, reasons: [] },
            ],
          },
        },
      }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
})
afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.setState(originalAuth)
  useSystemConfigStore.setState(originalConfig)
  localStorage.clear()
  vi.restoreAllMocks()
})

test('the first menu opening remains usable while its real-key reveal is pending', async () => {
  const user = userEvent.setup()
  const reveal = deferredReveal()
  const post = vi.spyOn(api, 'post').mockReturnValue(reveal.promise)
  renderKeys()
  await screen.findByText('1x')
  expect(post).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Open menu' }))
  expect(post).toHaveBeenCalledExactlyOnceWith('/api/token/21/key')
  expect(screen.getByRole('button', { name: 'Open menu' })).toHaveAttribute(
    'aria-expanded',
    'true'
  )
  await user.click(
    await screen.findByRole('menuitem', { name: 'Assigned access' })
  )
  expect(await screen.findByText('owner-public-model')).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Save assignment' })
  ).not.toBeInTheDocument()
  await act(async () =>
    reveal.resolve({
      data: { success: true, data: { key: 'synthetic-secret' } },
    })
  )
  expect(screen.getByRole('dialog', { name: 'Assigned access' })).toBeVisible()
})

test.each(['succeeds', 'fails'] as const)(
  'an open menu retains keyboard selection when key reveal %s',
  async (outcome) => {
    const user = userEvent.setup()
    const reveal = deferredReveal()
    vi.spyOn(api, 'post').mockReturnValue(reveal.promise)
    renderKeys()
    await screen.findByText('1x')
    const trigger = screen.getByRole('button', { name: 'Open menu' })
    trigger.focus()
    await user.keyboard('{ArrowDown}')
    const assignment = await screen.findByRole('menuitem', {
      name: 'Assigned access',
    })
    expect(assignment).toHaveFocus()
    await act(async () => {
      if (outcome === 'succeeds') {
        reveal.resolve({
          data: { success: true, data: { key: 'synthetic-secret' } },
        })
      } else reveal.reject(new Error('fixture reveal unavailable'))
    })
    expect(screen.getByRole('button', { name: 'Open menu' })).toHaveAttribute(
      'aria-expanded',
      'true'
    )
    expect(
      screen.getByRole('menuitem', { name: 'Assigned access' })
    ).toHaveFocus()
    await user.keyboard('{Enter}')
    expect(await screen.findByText('owner-public-model')).toBeVisible()
  }
)

test('visible budget action selects this Key without revealing its secret', async () => {
  const user = userEvent.setup()
  const post = vi.spyOn(api, 'post')
  renderKeys()
  const budget = screen.getByRole('button', { name: 'API Key usage budgets' })
  expect(budget).toHaveTextContent('Usage budgets')
  await user.click(budget)
  expect(
    await screen.findByRole('dialog', { name: 'API Key usage budgets' })
  ).toBeVisible()
  expect(api.get).toHaveBeenCalledWith(
    '/api/token/21/budget',
    expect.objectContaining({ signal: expect.any(AbortSignal) })
  )
  expect(post).not.toHaveBeenCalled()
})
