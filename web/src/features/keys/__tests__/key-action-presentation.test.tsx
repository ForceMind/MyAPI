/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClientProvider, type QueryClient } from '@tanstack/react-query'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { ApiKeysDialogs } from '../components/api-keys-dialogs'
import { ApiKeysPrimaryButtons } from '../components/api-keys-primary-buttons'
import { ApiKeysProvider } from '../components/api-keys-provider'
import { DataTableRowActions } from '../components/data-table-row-actions'
import {
  KEY_21,
  mockKeysApi,
  newClient,
  renderKeysPage,
  setViewport,
  signIn,
} from './keys-page-fixture'

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}))

const originalAuth = useAuthStore.getState()
const originalConfig = useSystemConfigStore.getState()
let client: QueryClient
let mocks: ReturnType<typeof mockKeysApi>

function renderRow(
  props: {
    menuOnly?: boolean
    primaryActionsVisible?: boolean
  } = {}
) {
  const onOpenKey = vi.fn()
  const viewRequests = vi.fn()
  const view = render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <ApiKeysProvider viewRequests={viewRequests}>
          <div onClick={onOpenKey}>
            <DataTableRowActions
              apiKey={KEY_21}
              menuOnly={props.menuOnly}
              primaryActionsVisible={props.primaryActionsVisible}
            />
          </div>
          <ApiKeysPrimaryButtons />
          <ApiKeysDialogs />
        </ApiKeysProvider>
      </TooltipProvider>
    </QueryClientProvider>
  )
  return { ...view, onOpenKey, viewRequests }
}

function deferredKey() {
  let resolve!: (response: {
    data: { success: boolean; data: { key: string } }
  }) => void
  const promise = new Promise<{
    data: { success: boolean; data: { key: string } }
  }>((accept) => {
    resolve = accept
  })
  return { promise, resolve }
}

beforeEach(() => {
  localStorage.clear()
  vi.clearAllMocks()
  client = newClient()
  signIn(100)
  setViewport(390)
  mocks = mockKeysApi({
    keys: [KEY_21],
    logs: [],
    status: {},
    extraGet: (path) =>
      path === '/api/user/models'
        ? { data: { success: true, data: [] } }
        : undefined,
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

test('row actions expose readable common commands and mobile touch targets without opening the Key', async () => {
  const user = userEvent.setup()
  const view = renderRow()
  const pause = screen.getByRole('button', { name: 'Pause Key' })
  expect(screen.getByRole('button', { name: 'Edit' })).toBeVisible()
  expect(screen.getByRole('button', { name: 'Copy Key' })).toBeVisible()
  expect(pause.closest('[data-row-actions]')).toHaveClass(
    'flex-wrap',
    'max-sm:[&_button]:min-h-11',
    'max-sm:[&_button]:min-w-11'
  )
  expect(screen.getByRole('button', { name: 'Create API Key' })).toHaveClass(
    'h-auto',
    'whitespace-normal',
    'max-sm:min-h-11'
  )
  await user.click(screen.getByRole('button', { name: 'View Key requests' }))
  expect(view.viewRequests).toHaveBeenCalledExactlyOnceWith(KEY_21)
  expect(view.onOpenKey).not.toHaveBeenCalled()
  expect(mocks.post).not.toHaveBeenCalled()
  expect(mocks.put).not.toHaveBeenCalled()
})

test('the detail header exposes edit and copy once while advanced actions remain reachable in More', async () => {
  const user = userEvent.setup()
  await renderKeysPage(client, '/keys/21')
  const page = await screen.findByTestId('key-page')
  expect(await within(page).findByRole('button', { name: 'Edit' })).toBeVisible()
  expect(
    within(page).getAllByRole('button', { name: 'Copy Key' })
  ).toHaveLength(1)
  await user.click(within(page).getByRole('button', { name: 'Open menu' }))
  expect(
    screen.queryByRole('menuitem', { name: 'Copy Key' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('menuitem', { name: 'Edit' })
  ).not.toBeInTheDocument()
  for (const name of [
    'Usage budgets',
    'Assigned access',
    'Model scope',
    'Subscription shares',
    'Copy Connection Info',
    'CC Switch',
    'Delete',
  ]) {
    expect(screen.getByRole('menuitem', { name })).toBeVisible()
  }
  expect(mocks.post).not.toHaveBeenCalled()
  expect(mocks.put).not.toHaveBeenCalled()
})

test('a compact menu preserves common actions and waits for an explicit copy before reading the Key', async () => {
  const user = userEvent.setup()
  const clipboard = vi.spyOn(navigator.clipboard, 'writeText')
  const pending = deferredKey()
  mocks.post.mockReturnValue(pending.promise)
  const view = renderRow({ menuOnly: true })
  const trigger = screen.getByRole('button', { name: 'Open menu' })
  trigger.focus()
  await user.keyboard('{ArrowDown}')
  expect(trigger).toHaveAttribute('aria-expanded', 'true')
  expect(
    screen.getByRole('menuitem', { name: 'View Key requests' })
  ).toHaveFocus()
  expect(screen.getByRole('menuitem', { name: 'Edit' })).toBeVisible()
  expect(screen.getByRole('menuitem', { name: 'Pause Key' })).toBeVisible()
  expect(mocks.post).not.toHaveBeenCalled()
  await user.click(screen.getByRole('menuitem', { name: 'Copy Key' }))
  expect(mocks.post).toHaveBeenCalledOnce()
  expect(clipboard).not.toHaveBeenCalled()
  await act(async () =>
    pending.resolve({
      data: { success: true, data: { key: 'synthetic-copy-secret' } },
    })
  )
  expect(clipboard).toHaveBeenCalledExactlyOnceWith('sk-synthetic-copy-secret')
  expect(toast.success).toHaveBeenCalledWith('Copied successfully')
  expect(view.onOpenKey).not.toHaveBeenCalled()
  expect(mocks.post).toHaveBeenCalledOnce()
})

test('a pending explicit copy is disabled and cannot dispatch a second secret read', async () => {
  const user = userEvent.setup()
  const pending = deferredKey()
  mocks.post.mockReturnValue(pending.promise)
  renderRow()
  const copy = screen.getByRole('button', { name: 'Copy Key' })
  await user.click(copy)
  expect(copy).toBeDisabled()
  expect(copy).toHaveAttribute('aria-busy', 'true')
  fireEvent.click(copy)
  expect(mocks.post).toHaveBeenCalledOnce()
  await act(async () =>
    pending.resolve({
      data: { success: true, data: { key: 'synthetic-copy-secret' } },
    })
  )
  expect(copy).toBeEnabled()
  expect(copy).toHaveAttribute('aria-busy', 'false')
})

test('a denied explicit copy reports failure without copying or retrying the secret read', async () => {
  const user = userEvent.setup()
  const clipboard = vi.spyOn(navigator.clipboard, 'writeText')
  mocks.post.mockResolvedValue({
    data: { success: false, message: 'Synthetic Key unavailable' },
  })
  renderRow()
  const copy = screen.getByRole('button', { name: 'Copy Key' })
  await user.click(copy)
  await waitFor(() => expect(copy).toBeEnabled())
  expect(toast.error).toHaveBeenCalledExactlyOnceWith('Synthetic Key unavailable')
  expect(toast.success).not.toHaveBeenCalled()
  expect(clipboard).not.toHaveBeenCalled()
  expect(mocks.post).toHaveBeenCalledOnce()
})

test.each(['copy', 'CC Switch'] as const)(
  'a pending explicit %s result cannot act after a session change',
  async (action) => {
    const user = userEvent.setup()
    const clipboard = vi.spyOn(navigator.clipboard, 'writeText')
    const pending = deferredKey()
    mocks.post.mockReturnValue(pending.promise)
    renderRow()
    if (action === 'copy') {
      await user.click(screen.getByRole('button', { name: 'Copy Key' }))
    } else {
      await user.click(screen.getByRole('button', { name: 'Open menu' }))
      expect(mocks.post).not.toHaveBeenCalled()
      await user.click(screen.getByRole('menuitem', { name: 'CC Switch' }))
    }
    expect(mocks.post).toHaveBeenCalledOnce()
    await act(async () => {
      useAuthStore.setState((state) => ({
        ...state,
        auth: {
          ...state.auth,
          session: {
            sid: 'replacement-session',
            current: true,
            login_method: 'password',
            ip: '',
            user_agent: '',
            created_at: 1,
            last_active_at: 1,
            expires_at: 4_000_000_000,
          },
        },
      }))
      pending.resolve({ data: { success: true, data: { key: 'stale-secret' } } })
    })
    expect(clipboard).not.toHaveBeenCalled()
    expect(
      screen.queryByRole('dialog', { name: 'Import to CC Switch' })
    ).not.toBeInTheDocument()
    expect(toast.success).not.toHaveBeenCalled()
    expect(toast.error).not.toHaveBeenCalled()
    expect(mocks.post).toHaveBeenCalledOnce()
  }
)

test('CC Switch waits for one explicitly requested Key read before opening its own dialog', async () => {
  const user = userEvent.setup()
  const pending = deferredKey()
  mocks.post.mockReturnValue(pending.promise)
  renderRow()
  await user.click(screen.getByRole('button', { name: 'Open menu' }))
  expect(mocks.post).not.toHaveBeenCalled()
  await user.click(screen.getByRole('menuitem', { name: 'CC Switch' }))
  expect(mocks.post).toHaveBeenCalledOnce()
  expect(
    screen.queryByRole('dialog', { name: 'Import to CC Switch' })
  ).not.toBeInTheDocument()
  await act(async () =>
    pending.resolve({
      data: { success: true, data: { key: 'synthetic-delivery-secret' } },
    })
  )
  expect(
    await screen.findByRole('dialog', { name: 'Import to CC Switch' })
  ).toBeVisible()
  await waitFor(() =>
    expect(mocks.get).toHaveBeenCalledWith('/api/user/models')
  )
  expect(mocks.post).toHaveBeenCalledOnce()
  expect(mocks.put).not.toHaveBeenCalled()
})

