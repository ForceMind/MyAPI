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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { PersonalProfile } from '../components/personal-profile'
import type { UserProfile } from '../types'

const clients: QueryClient[] = []
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
    await user.click(entry)
    expect(entry).toHaveAttribute('aria-expanded', 'false')
    expect(
      screen.queryByRole('button', {
        name: /Change Password Update your password/,
      })
    ).not.toBeInTheDocument()
  })
})
