/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { getChannelQuotaChanges } from '@/features/channels/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { OperationalAttentionPanel } from '../operational-attention-panel'

vi.mock('@/features/channels/api', () => ({
  getChannelQuotaChanges: vi.fn(),
}))

vi.mock('@tanstack/react-router', () => ({
  Link: (props: { children?: ReactNode; to?: string }) => (
    <a href={props.to}>{props.children}</a>
  ),
}))

function renderPanel() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <OperationalAttentionPanel />
    </QueryClientProvider>
  )
}

function setPermittedAdministrator() {
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'attention-test',
    role: ROLE.ADMIN,
    permissions: { admin_permissions: { channel: { read: true } } },
  })
}

describe('operational attention panel', () => {
  beforeEach(() => {
    setPermittedAdministrator()
    vi.clearAllMocks()
  })

  afterEach(() => {
    useAuthStore.getState().auth.setUser(null)
  })

  test('shows a quota alert and links to the quota analysis tab', async () => {
    vi.mocked(getChannelQuotaChanges).mockResolvedValue({
      success: true,
      data: {
        items: [
          {
            channel_id: 17,
            name: 'Provider quota alert',
            status: 'success',
            alert: { status: 'critical' },
          },
        ],
      },
    } as never)

    renderPanel()

    expect(await screen.findByText('Provider quota alert')).toHaveAttribute(
      'href',
      '/channels'
    )
    expect(screen.getByText('Quota alert')).toBeInTheDocument()
    await waitFor(() =>
      expect(getChannelQuotaChanges).toHaveBeenCalledWith({
        range: '24h',
        limit: 20,
        sort: 'observed_desc',
      })
    )
  })

  test('shows the no-alert state after a successful empty response', async () => {
    vi.mocked(getChannelQuotaChanges).mockResolvedValue({
      success: true,
      data: { items: [] },
    } as never)

    renderPanel()

    expect(
      await screen.findByText('No quota alerts in the loaded samples.')
    ).toBeInTheDocument()
  })
})
