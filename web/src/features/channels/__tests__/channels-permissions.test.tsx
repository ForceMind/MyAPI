/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getChannelOps, getChannelRoutingPreview } from '../api'
import { Channels } from '../index'

vi.mock('../api', () => ({
  getChannelOps: vi.fn(),
  getChannelRoutingPreview: vi.fn(),
}))

vi.mock('../components/channel-quota-changes-panel', () => ({
  ChannelQuotaChangesPanel: () => null,
}))
vi.mock('../components/channels-dialogs', () => ({
  ChannelsDialogs: () => null,
}))
vi.mock('../components/channels-primary-buttons', () => ({
  ChannelsPrimaryButtons: () => null,
}))
vi.mock('../components/channels-provider', () => ({
  ChannelsProvider: (props: { children: ReactNode }) => props.children,
}))
vi.mock('../components/channels-table', () => ({ ChannelsTable: () => null }))
vi.mock('@/components/layout', () => ({
  SectionPageLayout: Object.assign(
    (props: { children: ReactNode }) => <div>{props.children}</div>,
    {
      Title: (props: { children: ReactNode }) => <div>{props.children}</div>,
      Actions: (props: { children: ReactNode }) => <div>{props.children}</div>,
      Content: (props: { children: ReactNode }) => <div>{props.children}</div>,
    }
  ),
}))

function renderChannels() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <Channels />
    </QueryClientProvider>
  )
}

describe('channel routing preview permission boundary', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(getChannelOps).mockResolvedValue({
      success: true,
      data: { retry_times: 0 },
    })
  })

  test('does not mount or query the routing preview after channel read is revoked', () => {
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'admin',
      role: ROLE.ADMIN,
      permissions: { admin_permissions: { channel: { read: false } } },
    })

    renderChannels()

    expect(screen.queryByText('Routing Preview')).not.toBeInTheDocument()
    expect(getChannelRoutingPreview).not.toHaveBeenCalled()
  })

  test('mounts the routing preview for the root role', () => {
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'root',
      role: ROLE.SUPER_ADMIN,
    })

    renderChannels()

    expect(screen.getByText('Routing Preview')).toBeInTheDocument()
  })
})
