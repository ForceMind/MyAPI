/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { getChannelRouting } from '@/features/channel-routing/api'
import { getChannelQuotaChanges } from '@/features/channels/api'
import { useAuthStore } from '@/stores/auth-store'

import { OperationalAttentionPanel } from '../operational-attention-panel'

vi.mock('@/features/channel-routing/api', () => ({
  getChannelRouting: vi.fn(),
}))
vi.mock('@/features/channels/api', () => ({ getChannelQuotaChanges: vi.fn() }))
vi.mock('@tanstack/react-router', () => ({
  Link: (props: { children: ReactNode; to: string }) => (
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

beforeEach(() => {
  vi.resetAllMocks()
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'test-admin', role: 100 })
  vi.mocked(getChannelRouting).mockResolvedValue({
    success: true,
    data: {
      policy: {
        enabled: false,
        sticky_enabled: true,
        session_ttl_seconds: 86400,
        quota_max_age_seconds: 600,
      },
      channels: [],
    },
  })
  vi.mocked(getChannelQuotaChanges).mockResolvedValue({
    success: true,
    data: { items: [] },
  })
})

describe('operational attention', () => {
  test('ordinary users do not request channel metadata', () => {
    useAuthStore.getState().auth.setUser({ id: 2, username: 'user', role: 1 })
    const { container } = renderPanel()
    expect(container).toBeEmptyDOMElement()
    expect(getChannelRouting).not.toHaveBeenCalled()
    expect(getChannelQuotaChanges).not.toHaveBeenCalled()
  })
  test('disabled intelligent allocation is an informational entry with a preview link', async () => {
    renderPanel()
    expect(
      await screen.findByText('Intelligent allocation is disabled.')
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Preview routing' })
    ).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
  test('a failed sample query shows an error instead of a healthy empty state', async () => {
    vi.mocked(getChannelQuotaChanges).mockRejectedValue(new Error('fixture'))
    renderPanel()
    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(
      screen.queryByText('No quota alerts in the loaded samples.')
    ).not.toBeInTheDocument()
  })
})
