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

import { getAllLogs } from '@/features/usage-logs/api'
import { usageLogSchema } from '@/features/usage-logs/data/schema'
import { useAuthStore } from '@/stores/auth-store'

import { RoutingSwitchSummary } from '../routing-switch-summary'

vi.mock('@/features/usage-logs/api', () => ({ getAllLogs: vi.fn() }))
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
      <RoutingSwitchSummary />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  vi.resetAllMocks()
  useAuthStore.getState().auth.setUser({ id: 1, username: 'admin', role: 100 })
})

describe('recent channel switches', () => {
  test('reports recorded switches while ignoring malformed logs and private metadata', async () => {
    const log = usageLogSchema.parse({
      id: 1,
      user_id: 1,
      created_at: 100,
      type: 2,
      content: '',
      other: JSON.stringify({
        admin_info: {
          channel_routing: {
            channel_id: 701,
            switch_count: 2,
            switch_reason: 'upstream_status_500',
            session_id: 'never-display-this',
          },
        },
      }),
    })
    vi.mocked(getAllLogs).mockResolvedValue({
      success: true,
      data: {
        items: [log, { ...log, id: 2, other: 'not json' }],
        total: 2,
        page: 1,
        page_size: 20,
      },
    })
    renderPanel()
    expect(await screen.findByText('upstream_status_500')).toBeInTheDocument()
    expect(screen.queryByText('never-display-this')).not.toBeInTheDocument()
    expect(getAllLogs).toHaveBeenCalledWith(
      expect.objectContaining({
        page_size: 20,
        start_timestamp: expect.any(Number),
        end_timestamp: expect.any(Number),
      })
    )
  })
  test('does not fetch global logs for ordinary users', () => {
    useAuthStore.getState().auth.setUser({ id: 2, username: 'user', role: 1 })
    const { container } = renderPanel()
    expect(container).toBeEmptyDOMElement()
    expect(getAllLogs).not.toHaveBeenCalled()
  })
  test('log request failure is visible and is not reported as no switches', async () => {
    vi.mocked(getAllLogs).mockRejectedValue(new Error('fixture'))
    renderPanel()
    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(
      screen.queryByText('No channel switches in the loaded logs.')
    ).not.toBeInTheDocument()
  })
})
