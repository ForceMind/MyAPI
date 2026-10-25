/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient } from '@tanstack/react-query'
import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import {
  completePlaygroundKeyHandoff,
  readPlaygroundKeyHandoff,
} from '@/features/playground/lib/playground-key-handoff'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useInspectorStore } from '@/stores/inspector-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import type { ApiKey } from '../types'
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
const PERSONAL_STATUS = {
  user_funding_mode: 'disabled',
  user_funding_capabilities: { mode: 'disabled', epoch: 1, ready: true },
}
let client: QueryClient
let fixture: {
  keys: ApiKey[]
  logs: never[]
  status: Record<string, unknown>
  extraGet?: (path: string) => { data: unknown } | undefined
}
let mocks: ReturnType<typeof mockKeysApi>

function mount(extraGet?: (path: string) => { data: unknown } | undefined) {
  fixture.extraGet = extraGet
  mocks = mockKeysApi(fixture)
}

beforeEach(() => {
  localStorage.clear()
  client = newClient()
  signIn(100)
  setViewport(1440)
  fixture = { keys: [KEY_21], logs: [], status: {} }
})

afterEach(() => {
  client.clear()
  useAuthStore.setState(originalAuth)
  useSystemConfigStore.setState(originalConfig)
  useInspectorStore.setState({ current: null })
  localStorage.clear()
  vi.restoreAllMocks()
})

/** The Key's page, on one of its tabs (requests by default). */
async function openKeyPage(tab?: 'scope' | 'delivery') {
  const result = await renderKeysPage(
    client,
    `/keys/21${tab ? `?tab=${tab}` : ''}`
  )
  const page = await screen.findByTestId('key-page')
  await within(page).findByRole('heading', {
    level: 1,
    name: 'Desktop application',
  })
  return { ...result, page }
}

test('in personal workbench mode Keys show no financial quota and no automatic Key is created', async () => {
  fixture.status = PERSONAL_STATUS
  mount()
  const { page, router } = await openKeyPage('scope')
  await waitFor(() =>
    expect(
      screen.queryByRole('button', { name: 'My usage policy' })
    ).not.toBeInTheDocument()
  )
  expect(within(page).getByText('Access profile')).toBeVisible()
  expect(
    screen.queryByText(/remaining quota|used quota|balance|wallet|123456/i)
  ).not.toBeInTheDocument()
  // The list keeps creation available, still without quota figures.
  await act(async () => {
    await router.navigate({ to: '/keys' })
  })
  expect(
    await screen.findByRole('button', { name: 'Create API Key' })
  ).toBeVisible()
  expect(
    screen.queryByText(/remaining quota|used quota|balance|wallet|123456/i)
  ).not.toBeInTheDocument()
  expect(mocks.post).not.toHaveBeenCalled()
})

test.each([
  ['primary', 'five_hour', 'Primary window', '5-hour window'],
  ['primary', 'weekly', 'Primary window', 'Weekly window'],
  ['secondary', 'weekly', 'Secondary window', 'Weekly window'],
])(
  'a configured %s %s share uses readable window and slot labels',
  async (windowSlot, windowType, slotLabel, windowLabel) => {
    mount((path) =>
      path === '/api/token/21/subscription-share'
        ? {
            data: {
              success: true,
              data: {
                token_id: 21,
                pools: [
                  {
                    pool_id: 7,
                    snapshot_id: 70,
                    revision: 4,
                    channel_name: 'Subscription account A',
                    plan_type: 'plus',
                    window_slot: windowSlot,
                    window_type: windowType,
                    window_seconds: windowType === 'weekly' ? 604800 : 18000,
                    reset_at: 1797003600,
                    observed_at: 1797000000,
                    max_age_seconds: 300,
                    sample_age_seconds: 30,
                    state: 'active',
                    allocated_bps: 7000,
                    share_bps: 2000,
                    estimated_used_micro: 5000000,
                    unattributed_micro: 1000000,
                    pending_attempts: 0,
                    unsampled_attempts: 0,
                    approximate: true,
                    baseline_time: 1796999400,
                  },
                ],
              },
            },
          }
        : undefined
    )
    await openKeyPage('scope')
    expect(
      await screen.findByRole('region', {
        name: `Subscription account A ${slotLabel}`,
      })
    ).toBeVisible()
    expect(
      screen.getByText(`Subscription account A · ${slotLabel} · ${windowLabel}`)
    ).toBeVisible()
    expect(
      screen.queryByText(windowType, { exact: true })
    ).not.toBeInTheDocument()
    expect(mocks.put).not.toHaveBeenCalled()
    expect(mocks.post).not.toHaveBeenCalled()
  }
)

test('explicit pause and resume change only this page’s Key status', async () => {
  mount()
  mocks.put.mockImplementation(async (_url, body) => {
    const next = (body as { status: number }).status
    fixture.keys = [{ ...KEY_21, status: next }]
    return { data: { success: true } }
  })
  const user = userEvent.setup()
  await openKeyPage()
  await user.click(await screen.findByRole('button', { name: 'Pause Key' }))
  expect(
    await screen.findByRole('button', { name: 'Resume Key' })
  ).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Resume Key' }))
  expect(await screen.findByRole('button', { name: 'Pause Key' })).toBeVisible()
  const calls = vi
    .mocked(api.put)
    .mock.calls.filter(([url]) => url === '/api/token/?status_only=true')
  expect(calls.map(([, body]) => body)).toEqual([
    { id: 21, status: 2 },
    { id: 21, status: 1 },
  ])
})

test('copy reads this page’s Key only after a user click', async () => {
  mount()
  const user = userEvent.setup()
  const clipboard = vi.spyOn(navigator.clipboard, 'writeText')
  const { page } = await openKeyPage()
  const copy = within(page).getByRole('button', { name: 'Copy' })
  expect(mocks.post).not.toHaveBeenCalled()
  await user.click(copy)
  await waitFor(() =>
    expect(clipboard).toHaveBeenCalledWith('sk-inert-existing-key')
  )
  expect(mocks.post).toHaveBeenCalledOnce()
  expect(mocks.post).toHaveBeenCalledWith('/api/token/21/key')
})

test('an unknown status write locks the toggle until the current state is reloaded', async () => {
  mount()
  mocks.put.mockRejectedValueOnce(new Error('connection closed'))
  const user = userEvent.setup()
  await openKeyPage()
  await user.click(await screen.findByRole('button', { name: 'Pause Key' }))
  expect(
    await screen.findByText('Failed to update API key status')
  ).toBeVisible()
  expect(screen.getByRole('button', { name: 'Pause Key' })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Reload current state' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Pause Key' })).toBeEnabled()
  )
  expect(mocks.put).toHaveBeenCalledOnce()
})

test('a Key filter link from call logs applies its search and same-route navigation updates it', async () => {
  mount()
  const { router } = await renderKeysPage(client, '/keys?filter=Desktop')
  expect(await screen.findByPlaceholderText('Filter by name...')).toHaveValue(
    'Desktop'
  )
  await waitFor(() =>
    expect(mocks.get).toHaveBeenCalledWith(
      '/api/token/search?keyword=Desktop&p=1&size=20'
    )
  )
  await act(async () => {
    await router.navigate({ to: '/keys', search: { filter: 'Shared device' } })
  })
  await waitFor(() =>
    expect(screen.getByPlaceholderText('Filter by name...')).toHaveValue(
      'Shared device'
    )
  )
  await waitFor(() =>
    expect(mocks.get).toHaveBeenCalledWith(
      '/api/token/search?keyword=Shared+device&p=1&size=20'
    )
  )
})

test('the scope tab opens model scope without entering the full metadata editor', async () => {
  mount((path) =>
    path === '/api/token/21/model-limits'
      ? {
          data: {
            success: true,
            data: {
              token_id: 21,
              token_name: 'Desktop application',
              model_limits_enabled: false,
              models: [],
              revision: 'a'.repeat(64),
            },
          },
        }
      : undefined
  )
  const user = userEvent.setup()
  const { page } = await openKeyPage('scope')
  await user.click(within(page).getByRole('button', { name: 'Model scope' }))
  expect(
    await screen.findByRole('dialog', { name: 'Model scope' })
  ).toBeVisible()
  expect(
    screen.getByRole('radio', { name: /No additional model limit/ })
  ).toBeChecked()
  expect(mocks.put).not.toHaveBeenCalled()
})

test('the Key page keeps assigned access, budgets, credential maintenance, client export and deletion reachable', async () => {
  mount()
  const user = userEvent.setup()
  const { page } = await openKeyPage('scope')
  expect(
    within(page).getByRole('button', { name: 'Usage budgets' })
  ).toBeVisible()
  expect(
    within(page).getByRole('button', { name: 'Assigned access' })
  ).toBeVisible()
  // Root maintains the stored credential from the delivery tab.
  await user.click(within(page).getByRole('tab', { name: 'Delivery' }))
  expect(
    await within(page).findByRole('button', { name: 'Credential maintenance' })
  ).toBeVisible()
  await user.click(within(page).getByRole('button', { name: 'Open menu' }))
  expect(
    await screen.findByRole('menuitem', { name: 'CC Switch' })
  ).toBeVisible()
  expect(screen.getByRole('menuitem', { name: 'Delete' })).toBeVisible()
})

test('cancelling the delete confirmation keeps the Key and sends no deletion', async () => {
  mount()
  const user = userEvent.setup()
  const { page } = await openKeyPage()
  await user.click(within(page).getByRole('button', { name: 'Open menu' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
  const confirmation = await screen.findByRole('alertdialog', {
    name: 'Are you sure?',
  })
  await user.click(within(confirmation).getByRole('button', { name: 'Cancel' }))
  await waitFor(() =>
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  )
  expect(screen.getAllByText('Desktop application')[0]).toBeVisible()
  expect(mocks.delete).not.toHaveBeenCalled()
})

test('deleting the open Key leaves its page for the list instead of showing a removed Key', async () => {
  mount()
  const user = userEvent.setup()
  const { page, router } = await openKeyPage()
  await user.click(within(page).getByRole('button', { name: 'Open menu' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
  const confirmation = await screen.findByRole('alertdialog', {
    name: 'Are you sure?',
  })
  await user.click(within(confirmation).getByRole('button', { name: 'Delete' }))
  await waitFor(() => expect(mocks.delete).toHaveBeenCalledTimes(1))
  await waitFor(() => expect(router.state.location.pathname).toBe('/keys'))
  expect(screen.queryByTestId('key-page')).not.toBeInTheDocument()
})

test('Back after deleting a Key from its page never shows the deleted Key from the list cache', async () => {
  mount()
  mocks.delete.mockImplementation(async () => {
    fixture.keys = []
    return { data: { success: true } }
  })
  // The application keeps list results fresh for 10 seconds.
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 10_000 } },
  })
  const user = userEvent.setup()
  const { router } = await renderKeysPage(client)
  await user.click(
    await screen.findByRole('button', { name: /Desktop application/ })
  )
  const page = await screen.findByTestId('key-page')
  await user.click(
    await within(page).findByRole('button', { name: 'Open menu' })
  )
  await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
  const confirmation = await screen.findByRole('alertdialog', {
    name: 'Are you sure?',
  })
  await user.click(within(confirmation).getByRole('button', { name: 'Delete' }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/keys'))
  expect(await screen.findByText('No API Keys Found')).toBeVisible()
  expect(
    screen.queryByRole('button', { name: /Desktop application/ })
  ).not.toBeInTheDocument()
})

test('Open Playground hands over only this page’s available Key after a click', async () => {
  useAuthStore.getState().auth.setBundle({
    access_token: 'access-token',
    token_type: 'Bearer',
    access_expires_at: 4_000_000_000,
    user: { id: 7, username: 'root', role: 100 },
    session: {
      sid: 'session-a',
      current: true,
      login_method: 'password',
      ip: '127.0.0.1',
      user_agent: 'test',
      created_at: 1,
      last_active_at: 1,
      expires_at: 4_000_000_000,
    },
  })
  mount()
  const user = userEvent.setup()
  const { page, router } = await openKeyPage()
  expect(readPlaygroundKeyHandoff(7, 'session-a')).toEqual({ status: 'none' })
  await user.click(
    within(page).getByRole('button', { name: 'Open Playground' })
  )
  expect(readPlaygroundKeyHandoff(7, 'session-a')).toMatchObject({
    status: 'accepted',
    keyId: 21,
  })
  await waitFor(() =>
    expect(router.state.location.pathname).toBe('/playground')
  )
  const handoff = readPlaygroundKeyHandoff(7, 'session-a')
  if (handoff.status !== 'none') completePlaygroundKeyHandoff(handoff.requestId)
})

const drawerReads = (path: string) => {
  if (path === '/api/user/models') return { data: { success: true, data: [] } }
  if (path === '/api/token/auto-groups') {
    return {
      data: { success: true, data: { groups: ['default'], max_count: 5 } },
    }
  }
  if (path === '/api/user/self/groups') {
    return {
      data: {
        success: true,
        data: { default: { desc: 'Standard', ratio: 1 } },
      },
    }
  }
  return undefined
}

test('creating a Key through the existing drawer adds it to the list without reloading the page', async () => {
  mount(drawerReads)
  mocks.post.mockImplementation(async (url, body) => {
    if (url === '/api/token/') {
      const created = {
        ...KEY_21,
        id: 22,
        name: (body as { name: string }).name,
      }
      fixture.keys = [...fixture.keys, created]
      return { data: { success: true, data: created } }
    }
    return { data: { success: true, data: { key: 'inert-existing-key' } } }
  })
  const user = userEvent.setup()
  await renderKeysPage(client)
  await user.click(
    await screen.findByRole('button', { name: 'Create API Key' })
  )
  const drawer = await screen.findByRole('dialog', { name: 'Create API Key' })
  const save = within(drawer).getByRole('button', { name: 'Save changes' })
  await waitFor(() => expect(save).toBeEnabled())
  await user.type(
    within(drawer).getByRole('textbox', { name: 'Name' }),
    'Shared device'
  )
  await user.click(save)
  expect(
    await screen.findByRole('button', { name: /Shared device/ })
  ).toBeVisible()
  expect(mocks.post).toHaveBeenCalledWith(
    '/api/token/',
    expect.objectContaining({ name: 'Shared device' }),
    { skipErrorHandler: true, skipBusinessError: true, skipAuthRefresh: true }
  )
  expect(
    screen.getByRole('button', { name: /Desktop application/ })
  ).toBeVisible()
})

test('Edit Key uses the existing drawer and Escape cancels without writing', async () => {
  mount(drawerReads)
  const user = userEvent.setup()
  const { page } = await openKeyPage('scope')
  await user.click(within(page).getByRole('button', { name: 'Edit Key' }))
  const drawer = await screen.findByRole('dialog', { name: 'Update API Key' })
  await waitFor(() =>
    expect(
      within(drawer).getByRole('button', { name: 'Save changes' })
    ).toBeEnabled()
  )
  const name = within(drawer).getByRole('textbox', { name: 'Name' })
  expect(name).toHaveValue('Desktop application')
  await user.clear(name)
  await user.type(name, 'Uncommitted name')
  await user.keyboard('{Escape}')
  await waitFor(() =>
    expect(
      screen.queryByRole('dialog', { name: 'Update API Key' })
    ).not.toBeInTheDocument()
  )
  expect(mocks.put).not.toHaveBeenCalled()
  expect(mocks.delete).not.toHaveBeenCalled()
})

test('a paused Key cannot open Playground', async () => {
  fixture.keys = [{ ...KEY_21, status: 2 }]
  mount()
  const { page } = await openKeyPage()
  expect(
    within(page).getByRole('button', { name: 'Open Playground' })
  ).toBeDisabled()
})

test('the scope tab labels an enabled empty model scope as blocking all models', async () => {
  fixture.keys = [{ ...KEY_21, model_limits_enabled: true, model_limits: '' }]
  mount()
  const { page } = await openKeyPage('scope')
  expect(within(page).getByText('Block all models')).toBeVisible()
})
