/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AxiosAdapter } from 'axios'
import type { AnchorHTMLAttributes } from 'react'
import { expect, test, vi } from 'vitest'

import { getPlaygroundKeys } from '@/features/playground/api'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { PersonalUpstream } from '../personal-upstream'

vi.mock('@/features/playground/api', () => ({ getPlaygroundKeys: vi.fn() }))
vi.mock('@tanstack/react-router', () => ({
  Link: (
    props: AnchorHTMLAttributes<HTMLAnchorElement> & {
      to: string
      search?: { quotaChannelId?: number; connection?: number }
    }
  ) => {
    const { to, search, children, ...attributes } = props
    return (
      <a
        {...attributes}
        href={
          search
            ? `${to}?${new URLSearchParams(
                Object.entries(search)
                  .filter(([, value]) => value !== undefined)
                  .map(([key, value]) => [key, String(value)])
              )}`
            : to
        }
      >
        {children}
      </a>
    )
  },
}))

function mountAccounts(role: number = ROLE.SUPER_ADMIN, showChannels = true) {
  const user = { id: 7, username: 'synthetic-console', role }
  useAuthStore.getState().auth.setBundle({
    access_token: 'synthetic-account-picker',
    token_type: 'Bearer',
    access_expires_at: 4_000_000_000,
    user,
    session: {
      sid: 'synthetic-account-picker-session',
      current: true,
      login_method: 'password',
      ip: '127.0.0.1',
      user_agent: 'test',
      created_at: 1,
      last_active_at: 1,
      expires_at: 4_000_000_000,
    },
  })
  vi.mocked(getPlaygroundKeys).mockResolvedValue([])
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(['status'], {
    SidebarModulesAdmin: JSON.stringify({ admin: { channel: showChannels } }),
  })
  let quotaReads = 0
  const previousAdapter = api.defaults.adapter
  const adapter: AxiosAdapter = async (config) => {
    let data
    if (config.url === '/api/user/self') data = { success: true, data: user }
    else {
      expect(config.url).toBe('/api/channel/quota/changes')
      quotaReads++
      const base = {
        metric_type: 'codex_rate_limit',
        unit: 'percent',
        status: 'success',
        observed_at: Math.floor(Date.now() / 1000),
        window_type: 'five_hour',
      }
      data = {
        success: true,
        data: {
          source_complete: true,
          items_complete: true,
          items: [
            {
              ...base,
              channel_id: 7,
              series_id: 'first-primary',
              name: 'First account',
              current_available: 100,
            },
            {
              ...base,
              channel_id: 7,
              series_id: 'first-secondary',
              name: 'First account',
              status: 'error',
              window_type: 'weekly',
              current_available: null,
            },
            {
              ...base,
              channel_id: 8,
              series_id: 'second-primary',
              name: 'Second account',
              current_available: 0,
            },
          ],
        },
      }
    }
    return { data, status: 200, statusText: 'OK', headers: {}, config }
  }
  api.defaults.adapter = adapter
  const rendered = render(
    <QueryClientProvider client={client}>
      <PersonalUpstream />
    </QueryClientProvider>
  )
  return {
    get quotaReads() {
      return quotaReads
    },
    cleanup() {
      rendered.unmount()
      client.clear()
      api.defaults.adapter = previousAdapter
      useAuthStore.getState().auth.reset('idle')
    },
  }
}

test('the account list selects saved evidence on every width without another snapshot read', async () => {
  const fixture = mountAccounts()
  try {
    const user = userEvent.setup()
    await screen.findByRole('heading', { name: 'First account' })
    expect(screen.getByText('0%')).toBeInTheDocument()
    expect(screen.getByText('Usage sample failed')).toBeInTheDocument()
    const desktop = screen.getByRole('complementary', {
      name: 'Select upstream account',
    })
    await user.click(
      within(desktop).getByRole('button', {
        name: 'Second account',
      })
    )
    expect(
      screen.getByRole('heading', { name: 'Second account' })
    ).toBeInTheDocument()
    expect(screen.getByText('100%')).toBeInTheDocument()
    expect(
      screen.getByText('Manage account connection').closest('a')
    ).toHaveAttribute('href', '/connections?connection=8')
    expect(screen.queryByText('Usage sample failed')).not.toBeInTheDocument()
    // One list serves PC and phone (no native select on either).
    expect(
      screen.queryByRole('combobox', { name: 'Select upstream account' })
    ).not.toBeInTheDocument()
    await user.click(
      within(desktop).getByRole('button', { name: 'First account' })
    )
    expect(
      screen.getByRole('heading', { name: 'First account' })
    ).toBeInTheDocument()
    expect(screen.getByText('Usage sample failed')).toBeInTheDocument()
    expect(fixture.quotaReads).toBe(1)
  } finally {
    fixture.cleanup()
  }
})

test('a disabled Channels module hides connection and history actions while preserving saved account observations', async () => {
  const fixture = mountAccounts(ROLE.SUPER_ADMIN, false)
  try {
    await screen.findByRole('heading', { name: 'First account' })
    expect(
      screen.queryByText('Manage account connection')
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'View channel history' })
    ).not.toBeInTheDocument()
    expect(screen.getByText('0%')).toBeInTheDocument()
  } finally {
    fixture.cleanup()
  }
})

test('window changes over time are read only after an explicit request', async () => {
  const fixture = mountAccounts()
  try {
    const user = userEvent.setup()
    await screen.findByRole('heading', { name: 'First account' })
    expect(fixture.quotaReads).toBe(1)
    const toggle = screen.getByRole('button', { name: 'Show window changes' })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    await user.click(toggle)
    expect(
      screen.getByRole('button', { name: 'Hide window changes' })
    ).toHaveAttribute('aria-expanded', 'true')
    await waitFor(() => expect(fixture.quotaReads).toBe(2))
  } finally {
    fixture.cleanup()
  }
})

test('a non-admin self snapshot starts no window or Key reads and exposes no connection actions', async () => {
  const fixture = mountAccounts(ROLE.USER)
  try {
    await screen.findByText(
      'Channel read permission is required to view upstream account windows.'
    )
    expect(fixture.quotaReads).toBe(0)
    expect(getPlaygroundKeys).not.toHaveBeenCalled()
    expect(
      screen.queryByText('Manage account connection')
    ).not.toBeInTheDocument()
  } finally {
    fixture.cleanup()
  }
})
