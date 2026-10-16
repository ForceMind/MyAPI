/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { OAUTH_BIND_CALLBACK_MESSAGE } from '@/features/auth/constants'
import { useStatus } from '@/hooks/use-status'
import { useAuthStore, type AuthBundle } from '@/stores/auth-store'

import { completeOAuthBindFlow, startOAuthBindFlow } from '../../api'
import type { UserProfile } from '../../types'
import { AccountBindingsTab } from '../tabs/account-bindings-tab'

vi.mock('@/hooks/use-status', () => ({ useStatus: vi.fn() }))
vi.mock('@/features/auth/lib/oauth-bind-window', () => ({
  watchOAuthPopupClosed: vi.fn(() => vi.fn()),
}))
vi.mock('@/features/auth/lib/oauth-callback-mode', () => ({
  getOAuthSessionStorage: vi.fn(() => window.sessionStorage),
  markOAuthBindPopup: vi.fn(() => true),
}))
vi.mock('../../api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../api')>()),
  completeOAuthBindFlow: vi.fn(),
  startOAuthBindFlow: vi.fn(),
}))
vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

const originalAuth = useAuthStore.getState()
const profile: UserProfile = {
  id: 42,
  username: 'owner',
  display_name: 'Owner',
  role: 1,
  group: 'default',
  quota: 0,
  used_quota: 0,
  request_count: 0,
  status: 1,
  aff_count: 0,
  aff_quota: 0,
  aff_history_quota: 0,
  created_time: 1,
}

function authBundle(): AuthBundle {
  return {
    access_token: 'test-access',
    token_type: 'Bearer',
    access_expires_at: 4_000_000_000,
    user: { id: 42, username: 'owner', role: 1 },
    session: {
      sid: 'session-1',
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

beforeEach(() => {
  useAuthStore.getState().auth.setBundle(authBundle())
  vi.mocked(useStatus).mockReturnValue({
    status: {
      github_oauth: true,
      github_client_id: 'github-client',
      discord_oauth: true,
      discord_client_id: 'discord-client',
    },
    loading: false,
    error: null,
    confirmed: true,
  } as ReturnType<typeof useStatus>)
})

afterEach(() => {
  useAuthStore.setState(originalAuth)
  vi.restoreAllMocks()
})

test('a replaced OAuth callback cannot publish feedback or refresh profile state', async () => {
  const user = userEvent.setup()
  const popups = [0, 1].map(() => ({
    closed: false,
    close: vi.fn(),
    location: { replace: vi.fn() },
    postMessage: vi.fn(),
  }))
  vi.spyOn(window, 'open')
    .mockReturnValueOnce(popups[0] as unknown as Window)
    .mockReturnValueOnce(popups[1] as unknown as Window)
  vi.mocked(startOAuthBindFlow)
    .mockResolvedValueOnce('github-state')
    .mockResolvedValueOnce('discord-state')
  let resolveOldCallback!: (
    value: Awaited<ReturnType<typeof completeOAuthBindFlow>>
  ) => void
  vi.mocked(completeOAuthBindFlow).mockReturnValueOnce(
    new Promise((resolve) => {
      resolveOldCallback = resolve
    })
  )
  const onUpdate = vi.fn()
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  render(
    <QueryClientProvider client={client}>
      <AccountBindingsTab
        profile={profile}
        onUpdate={onUpdate}
        securityScopeKey='42:1:session-1'
      />
    </QueryClientProvider>
  )

  const githubBind = await screen.findByRole('button', {
    name: 'Bind GitHub',
  })
  const discordBind = await screen.findByRole('button', {
    name: 'Bind Discord',
  })
  await user.click(githubBind)
  await waitFor(() => expect(popups[0].location.replace).toHaveBeenCalledOnce())
  expect(startOAuthBindFlow).toHaveBeenNthCalledWith(
    1,
    'github',
    expect.any(AbortSignal)
  )
  await act(async () => {
    window.dispatchEvent(
      new MessageEvent('message', {
        origin: window.location.origin,
        source: popups[0] as unknown as Window,
        data: {
          type: OAUTH_BIND_CALLBACK_MESSAGE,
          provider: 'github',
          state: 'github-state',
          code: 'old-code',
        },
      })
    )
  })
  await waitFor(() => expect(completeOAuthBindFlow).toHaveBeenCalledOnce())
  expect(completeOAuthBindFlow).toHaveBeenCalledWith(
    'github',
    { state: 'github-state', code: 'old-code' },
    expect.any(AbortSignal)
  )

  await user.click(discordBind)
  await waitFor(() => expect(popups[1].location.replace).toHaveBeenCalledOnce())
  expect(startOAuthBindFlow).toHaveBeenNthCalledWith(
    2,
    'discord',
    expect.any(AbortSignal)
  )
  await act(async () => {
    resolveOldCallback({ success: true })
  })

  expect(toast.success).not.toHaveBeenCalled()
  expect(toast.error).not.toHaveBeenCalled()
  expect(onUpdate).not.toHaveBeenCalled()
  expect(popups[0].postMessage).not.toHaveBeenCalled()
})
