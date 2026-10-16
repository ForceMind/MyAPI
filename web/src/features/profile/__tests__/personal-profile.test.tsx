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
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { PersonalProfile } from '../components/personal-profile'
import type { UserProfile } from '../types'

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}))

const clients: QueryClient[] = []
const originalAuth = useAuthStore.getState()
const profile: UserProfile = {
  id: 42,
  username: 'owner',
  display_name: 'My workspace',
  role: 100,
  group: 'default',
  quota: 5000,
  used_quota: 1000,
  request_count: 10,
  status: 1,
  aff_count: 0,
  aff_quota: 0,
  aff_history_quota: 0,
  created_time: 1,
}

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.setState(originalAuth)
  vi.restoreAllMocks()
})

async function renderProfile(
  overrides: Partial<Parameters<typeof PersonalProfile>[0]> = {}
) {
  const onRetry = vi.fn()
  const props = {
    profile,
    loading: false,
    error: false,
    onRetry,
    onRefresh: vi.fn(),
    ...overrides,
  }
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  clients.push(client)
  const root = createRootRoute({ component: Outlet })
  const profileRoute = createRoute({
    getParentRoute: () => root,
    path: '/profile',
    component: () => <PersonalProfile {...props} />,
  })
  const overview = createRoute({
    getParentRoute: () => root,
    path: '/dashboard/overview',
    component: () => <h1>Overview</h1>,
  })
  const router = createRouter({
    routeTree: root.addChildren([profileRoute, overview]),
    history: createMemoryHistory({ initialEntries: ['/profile'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return { user: userEvent.setup(), onRetry }
}

describe('personal profile', () => {
  it('shows the real identity and read-only self scope while hiding monetary balances and security forms initially', async () => {
    await renderProfile()
    expect(
      screen.getByRole('heading', { name: 'My workspace' })
    ).toBeInTheDocument()
    expect(screen.getByText('Only me')).toBeInTheDocument()
    expect(
      screen.queryByRole('combobox', { name: 'Management scope' })
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Manage login methods/ })
    ).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('Current Balance')).not.toBeInTheDocument()
    expect(screen.queryByText('Change Password')).not.toBeInTheDocument()
  })
  it('offers keyboard-operable retry and overview recovery after a failed profile read', async () => {
    const { user, onRetry } = await renderProfile({
      profile: null,
      error: true,
    })
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Profile is temporarily unavailable'
    )
    expect(
      screen.getByText('Other pages are still available.')
    ).toBeInTheDocument()
    const retry = screen.getByRole('button', { name: 'Reload profile' })
    retry.focus()
    await user.keyboard('{Enter}')
    expect(onRetry).toHaveBeenCalledOnce()
    const overview = screen.getByRole('button', { name: 'Back to overview' })
    expect(overview).toHaveAttribute('href', '/dashboard/overview')
    await user.click(overview)
    expect(
      await screen.findByRole('heading', { name: 'Overview' })
    ).toBeInTheDocument()
  })
  it('keeps existing login and account security controls available through an explicit expandable entry', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/api/user/sessions' || url === '/api/user/oauth/bindings') {
        return { data: { success: true, data: [] } }
      }
      if (url === '/api/user/2fa/status') {
        return {
          data: {
            success: true,
            data: { enabled: false, locked: false, backup_codes_remaining: 0 },
          },
        }
      }
      if (url === '/api/user/passkey') {
        return { data: { success: true, data: { enabled: false } } }
      }
      if (url === '/api/status') return { data: { success: true, data: {} } }
      throw new Error(`Unexpected test API: ${url}`)
    })
    const { user } = await renderProfile()
    const entry = screen.getByRole('button', { name: /Manage login methods/ })
    await user.click(entry)
    expect(entry).toHaveAttribute('aria-expanded', 'true')
    expect(
      await screen.findByRole('button', {
        name: /Change Password Update your password/,
      })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', {
        name: /Delete Account Root accounts cannot be deleted Unavailable/,
      })
    ).toBeDisabled()
    expect(
      vi.mocked(api.get).mock.calls.filter(([url]) => url === '/api/user/token')
    ).toHaveLength(0)
    await user.click(entry)
    expect(entry).toHaveAttribute('aria-expanded', 'false')
    expect(
      screen.queryByRole('button', {
        name: /Change Password Update your password/,
      })
    ).not.toBeInTheDocument()
  })

  it('clears a pending password form and aborts its request when the role changes in the same session', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/api/user/sessions' || url === '/api/user/oauth/bindings') {
        return { data: { success: true, data: [] } }
      }
      if (url === '/api/user/2fa/status') {
        return {
          data: {
            success: true,
            data: { enabled: false, locked: false, backup_codes_remaining: 0 },
          },
        }
      }
      if (url === '/api/user/passkey') {
        return { data: { success: true, data: { enabled: false } } }
      }
      if (url === '/api/status') return { data: { success: true, data: {} } }
      throw new Error(`Unexpected test API: ${url}`)
    })
    type UpdateResponse = Awaited<ReturnType<typeof api.put>>
    let resolveUpdate!: (response: UpdateResponse) => void
    const pendingUpdate = new Promise<UpdateResponse>((resolve) => {
      resolveUpdate = resolve
    })
    const put = vi.spyOn(api, 'put').mockReturnValue(pendingUpdate)
    useAuthStore.setState({
      auth: {
        ...originalAuth.auth,
        user: { id: 42, username: 'owner', role: 100 },
        session: {
          sid: 'session-one',
          current: true,
          login_method: 'password',
          ip: '127.0.0.1',
          user_agent: 'synthetic-test',
          created_at: 1,
          last_active_at: 1,
          expires_at: 4_000_000_000,
        },
      },
    })
    const { user } = await renderProfile()
    await user.click(
      screen.getByRole('button', { name: /Manage login methods/ })
    )
    await user.click(
      await screen.findByRole('button', {
        name: /Change Password Update your password/,
      })
    )
    const dialog = await screen.findByRole('dialog', {
      name: 'Change Password',
    })
    await user.type(
      within(dialog).getByLabelText('Current Password'),
      'old-password'
    )
    await user.type(
      within(dialog).getByLabelText('New Password'),
      'new-password'
    )
    await user.type(
      within(dialog).getByLabelText('Confirm New Password'),
      'new-password'
    )
    await user.click(
      within(dialog).getByRole('button', { name: 'Change Password' })
    )
    await waitFor(() => expect(put).toHaveBeenCalledOnce())
    const requestSignal = put.mock.calls[0]?.[2]?.signal

    act(() => {
      useAuthStore.setState({
        auth: {
          ...useAuthStore.getState().auth,
          user: { id: 42, username: 'owner', role: 1 },
          session: {
            sid: 'session-one',
            current: true,
            login_method: 'password',
            ip: '127.0.0.1',
            user_agent: 'synthetic-test',
            created_at: 1,
            last_active_at: 1,
            expires_at: 4_000_000_000,
          },
        },
      })
    })

    expect(requestSignal?.aborted).toBe(true)
    expect(
      screen.queryByRole('dialog', { name: 'Change Password' })
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Manage login methods/ })
    ).toHaveAttribute('aria-expanded', 'false')
    await act(async () => {
      resolveUpdate({ data: { success: true } } as UpdateResponse)
      await pendingUpdate
    })
  })

  it('uses the current auth role when reopening deletion controls in the same session', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/api/user/sessions' || url === '/api/user/oauth/bindings') {
        return { data: { success: true, data: [] } }
      }
      if (url === '/api/user/2fa/status') {
        return {
          data: {
            success: true,
            data: { enabled: false, locked: false, backup_codes_remaining: 0 },
          },
        }
      }
      if (url === '/api/user/passkey') {
        return { data: { success: true, data: { enabled: false } } }
      }
      if (url === '/api/status') return { data: { success: true, data: {} } }
      throw new Error(`Unexpected test API: ${url}`)
    })
    const session = {
      sid: 'session-one',
      current: true,
      login_method: 'password',
      ip: '127.0.0.1',
      user_agent: 'synthetic-test',
      created_at: 1,
      last_active_at: 1,
      expires_at: 4_000_000_000,
    }
    useAuthStore.setState({
      auth: {
        ...originalAuth.auth,
        user: { id: 42, username: 'owner', role: 1 },
        session,
      },
    })
    const { user } = await renderProfile({
      profile: { ...profile, role: 1 },
    })
    const entry = screen.getByRole('button', {
      name: /Manage login methods/,
    })
    await user.click(entry)
    expect(
      await screen.findByRole('button', {
        name: /Delete Account Permanently delete your account and all data Available/,
      })
    ).toBeEnabled()

    act(() => {
      useAuthStore.setState({
        auth: {
          ...useAuthStore.getState().auth,
          user: { id: 42, username: 'owner', role: 100 },
          session,
        },
      })
    })

    expect(entry).toHaveAttribute('aria-expanded', 'false')
    await user.click(entry)
    expect(
      await screen.findByRole('button', {
        name: /Delete Account Root accounts cannot be deleted Unavailable/,
      })
    ).toBeDisabled()
  })

  it('reports an unknown account deletion result and retries only after another explicit submit', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/api/user/sessions' || url === '/api/user/oauth/bindings') {
        return { data: { success: true, data: [] } }
      }
      if (url === '/api/user/2fa/status') {
        return {
          data: {
            success: true,
            data: { enabled: false, locked: false, backup_codes_remaining: 0 },
          },
        }
      }
      if (url === '/api/user/passkey') {
        return { data: { success: true, data: { enabled: false } } }
      }
      if (url === '/api/status') {
        return { data: { success: true, data: {} } }
      }
      throw new Error(`Unexpected test API: ${url}`)
    })
    const remove = vi
      .spyOn(api, 'delete')
      .mockRejectedValue(new Error('unknown network result'))
    useAuthStore.setState({
      auth: {
        ...originalAuth.auth,
        user: { id: 42, username: 'owner', role: 1 },
        session: {
          sid: 'session-one',
          current: true,
          login_method: 'password',
          ip: '127.0.0.1',
          user_agent: 'synthetic-test',
          created_at: 1,
          last_active_at: 1,
          expires_at: 4_000_000_000,
        },
      },
    })
    const { user } = await renderProfile({
      profile: { ...profile, role: 1 },
    })
    await user.click(
      screen.getByRole('button', { name: /Manage login methods/ })
    )
    await user.click(
      await screen.findByRole('button', {
        name: /Delete Account Permanently delete your account/,
      })
    )
    const dialog = await screen.findByRole('dialog', {
      name: 'Delete Account',
    })
    const confirmation = within(dialog).getByLabelText(
      /Type.*owner.*to confirm/
    )
    await user.type(confirmation, 'owner')
    const submit = within(dialog).getByRole('button', {
      name: 'Delete Account',
    })
    await user.click(submit)
    await waitFor(() => expect(remove).toHaveBeenCalledOnce())
    expect(toast.error).toHaveBeenCalledWith(
      'The result could not be confirmed. Verify the current account state before trying again.'
    )
    expect(dialog).toBeVisible()
    await Promise.resolve()
    expect(remove).toHaveBeenCalledOnce()

    await waitFor(() => expect(submit).toBeEnabled())
    await user.click(submit)
    await waitFor(() => expect(remove).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(submit).toBeEnabled())
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    await user.click(
      screen.getByRole('button', {
        name: /Delete Account Permanently delete your account/,
      })
    )
    expect(
      within(
        screen.getByRole('dialog', { name: 'Delete Account' })
      ).getByLabelText(/Type.*owner.*to confirm/)
    ).toHaveValue('')
  })
})
