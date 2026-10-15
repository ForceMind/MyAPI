/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type { AnchorHTMLAttributes } from 'react'
import { expect, test, vi } from 'vitest'

import { getUserLogs } from '@/features/usage-logs/api'

import { OverviewRecent } from '../overview-recent'

vi.mock('@/features/usage-logs/api', () => ({ getUserLogs: vi.fn() }))
vi.mock('@tanstack/react-router', () => ({
  Link: (
    props: AnchorHTMLAttributes<HTMLAnchorElement> & {
      to?: string
      params?: unknown
    }
  ) => {
    const { to, children, params: _params, ...attributes } = props
    return (
      <a href={to} {...attributes}>
        {children}
      </a>
    )
  },
}))

test('a completed answer without usage remains successful and does not invent zero Tokens', async () => {
  vi.mocked(getUserLogs).mockResolvedValue({
    success: true,
    data: {
      total: 1,
      page: 1,
      page_size: 10,
      items: [
        {
          id: 501,
          user_id: 7,
          type: 5,
          created_at: 1000,
          content: '',
          username: '',
          token_name: 'Desktop app',
          model_name: 'model-a',
          quota: 0,
          prompt_tokens: 0,
          completion_tokens: 0,
          use_time: 2,
          is_stream: false,
          channel: 1,
          channel_name: 'Subscription account',
          token_id: 31,
          group: 'default',
          ip: '',
          other: JSON.stringify({
            request_outcome: 'completed',
            usage_outcome: 'unavailable',
            settlement_status: 'pending_review',
          }),
          request_id: '',
          upstream_request_id: '',
        },
      ],
    },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <OverviewRecent />
    </QueryClientProvider>
  )
  expect(await screen.findByText('Request successful')).toBeInTheDocument()
  expect(screen.getByText('Usage not provided')).toBeInTheDocument()
  expect(screen.queryByText('0 Tokens')).not.toBeInTheDocument()
  expect(getUserLogs).toHaveBeenCalledWith({ p: 1, page_size: 10 })
})
