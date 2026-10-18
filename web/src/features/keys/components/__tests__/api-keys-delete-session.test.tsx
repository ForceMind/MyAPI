/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { Table } from '@tanstack/react-table'
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useEffect } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore, type AuthBundle } from '@/stores/auth-store'

import { apiKeySchema, type ApiKey } from '../../types'
import { ApiKeysDeleteDialog } from '../api-keys-delete-dialog'
import { ApiKeysMultiDeleteDialog } from '../api-keys-multi-delete-dialog'
import { ApiKeysProvider, useApiKeys } from '../api-keys-provider'

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

const { toast } = await import('sonner')
const originalAuth = useAuthStore.getState()
const key = apiKeySchema.parse({
  id: 21,
  name: 'Synthetic Key',
  key: 'masked',
  status: 1,
  remain_quota: 100,
  used_quota: 0,
  unlimited_quota: true,
  expired_time: -1,
  created_time: 1,
  accessed_time: 1,
  group: 'default',
  model_limits_enabled: false,
})

function bundle(userId: number, role: number, sid: string): AuthBundle {
  return {
    access_token: `synthetic-${sid}`,
    token_type: 'Bearer',
    access_expires_at: 4_000_000_000,
    user: { id: userId, role, username: `fixture-${userId}` },
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
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => {
    resolve = done
  })
  return { promise, resolve }
}

function RefreshProbe() {
  const { refreshTrigger } = useApiKeys()
  return <output data-testid='refresh-trigger'>{refreshTrigger}</output>
}

function SingleDeleteFixture() {
  const { setCurrentRow, setOpen } = useApiKeys()
  useEffect(() => {
    setCurrentRow(key)
    setOpen('delete')
  }, [setCurrentRow, setOpen])
  return (
    <>
      <RefreshProbe />
      <ApiKeysDeleteDialog />
    </>
  )
}

function renderSingleDelete() {
  return render(
    <ApiKeysProvider>
      <SingleDeleteFixture />
    </ApiKeysProvider>
  )
}

function makeTable(resetRowSelection = vi.fn()): Table<ApiKey> {
  return {
    getFilteredSelectedRowModel: () => ({
      rows: [{ original: key }],
    }),
    resetRowSelection,
  } as unknown as Table<ApiKey>
}

function renderBatchDelete(props: {
  table: Table<ApiKey>
  onOpenChange: (open: boolean) => void
}) {
  return render(
    <ApiKeysProvider>
      <RefreshProbe />
      <ApiKeysMultiDeleteDialog
        open
        onOpenChange={props.onOpenChange}
        table={props.table}
      />
    </ApiKeysProvider>
  )
}

beforeEach(() => {
  useAuthStore.getState().auth.setBundle(bundle(1, 100, 'original'))
  vi.mocked(toast.success).mockClear()
  vi.mocked(toast.error).mockClear()
})

afterEach(() => {
  cleanup()
  useAuthStore.setState(originalAuth)
  vi.restoreAllMocks()
})

test.each([
  {
    change: 'identity',
    replacement: bundle(2, 1, 'replacement'),
  },
  {
    change: 'SID',
    replacement: bundle(1, 100, 'replacement'),
  },
])(
  'single delete uses the bounded auth request and ignores a late response after the $change changes',
  async ({ replacement }) => {
    const pending = deferred<{ data: { success: true } }>()
    const request = vi.spyOn(api, 'delete').mockReturnValue(pending.promise)
    renderSingleDelete()
    const button = await screen.findByRole('button', { name: 'Delete' })

    act(() => {
      button.click()
      button.click()
    })

    expect(request).toHaveBeenCalledExactlyOnceWith('/api/token/21/', {
      skipAuthRefresh: true,
      skipBusinessError: true,
      skipErrorHandler: true,
    })
    act(() => useAuthStore.getState().auth.setBundle(replacement))
    await act(async () => pending.resolve({ data: { success: true } }))

    expect(toast.success).not.toHaveBeenCalled()
    expect(toast.error).not.toHaveBeenCalled()
    expect(screen.getByTestId('refresh-trigger')).toHaveTextContent('0')
    expect(screen.getByRole('alertdialog')).toBeVisible()
  }
)

test('single delete preserves same-session feedback and refresh', async () => {
  vi.spyOn(api, 'delete').mockResolvedValue({ data: { success: true } })
  renderSingleDelete()

  await userEvent
    .setup()
    .click(await screen.findByRole('button', { name: 'Delete' }))

  await waitFor(() => expect(toast.success).toHaveBeenCalledOnce())
  expect(screen.getByTestId('refresh-trigger')).toHaveTextContent('1')
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
})

test('batch delete uses the bounded auth request and ignores a late response after the role changes', async () => {
  const pending = deferred<{ data: { success: true; data: number } }>()
  const request = vi.spyOn(api, 'post').mockReturnValue(pending.promise)
  const resetRowSelection = vi.fn()
  const onOpenChange = vi.fn()
  renderBatchDelete({ table: makeTable(resetRowSelection), onOpenChange })
  const button = await screen.findByRole('button', { name: 'Delete' })

  act(() => {
    button.click()
    button.click()
  })

  expect(request).toHaveBeenCalledExactlyOnceWith(
    '/api/token/batch',
    { ids: [21] },
    {
      skipAuthRefresh: true,
      skipBusinessError: true,
      skipErrorHandler: true,
    }
  )
  act(() => useAuthStore.getState().auth.setBundle(bundle(1, 1, 'original')))
  await act(async () => pending.resolve({ data: { success: true, data: 1 } }))

  expect(toast.success).not.toHaveBeenCalled()
  expect(toast.error).not.toHaveBeenCalled()
  expect(resetRowSelection).not.toHaveBeenCalled()
  expect(onOpenChange).not.toHaveBeenCalled()
  expect(screen.getByTestId('refresh-trigger')).toHaveTextContent('0')
})

test('batch delete preserves same-session feedback, selection reset, close, and refresh', async () => {
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: 1 },
  })
  const resetRowSelection = vi.fn()
  const onOpenChange = vi.fn()
  renderBatchDelete({ table: makeTable(resetRowSelection), onOpenChange })

  await userEvent
    .setup()
    .click(await screen.findByRole('button', { name: 'Delete' }))

  await waitFor(() => expect(toast.success).toHaveBeenCalledOnce())
  expect(resetRowSelection).toHaveBeenCalledOnce()
  expect(onOpenChange).toHaveBeenCalledExactlyOnceWith(false)
  expect(screen.getByTestId('refresh-trigger')).toHaveTextContent('1')
})

test('unmounting a pending batch delete suppresses its late response effects', async () => {
  const pending = deferred<{ data: { success: true; data: number } }>()
  vi.spyOn(api, 'post').mockReturnValue(pending.promise)
  const resetRowSelection = vi.fn()
  const onOpenChange = vi.fn()
  const view = renderBatchDelete({
    table: makeTable(resetRowSelection),
    onOpenChange,
  })

  await userEvent
    .setup()
    .click(await screen.findByRole('button', { name: 'Delete' }))
  view.unmount()
  await act(async () => pending.resolve({ data: { success: true, data: 1 } }))

  expect(toast.success).not.toHaveBeenCalled()
  expect(toast.error).not.toHaveBeenCalled()
  expect(resetRowSelection).not.toHaveBeenCalled()
  expect(onOpenChange).not.toHaveBeenCalled()
})
