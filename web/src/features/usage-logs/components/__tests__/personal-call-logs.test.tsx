import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import i18next from 'i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { usageLogSchema } from '../../data/schema'
import { PersonalCallLogs } from '../personal-call-logs'

const clients: QueryClient[] = []
const originalAuth = useAuthStore.getState()
beforeEach(() =>
  useAuthStore.setState({
    auth: {
      ...originalAuth.auth,
      user: { id: 1, username: 'owner', role: 100 },
    },
  })
)
const missingUsage = usageLogSchema.parse({
  id: 1,
  user_id: 1,
  created_at: 1792027200,
  type: 5,
  content: 'completed without usage',
  token_name: 'desktop-key',
  model_name: 'gpt-test',
  request_id: 'req-complete',
  other: JSON.stringify({
    request_outcome: 'completed',
    usage_outcome: 'missing',
    settlement_status: 'pending_review',
  }),
})
const failed = usageLogSchema.parse({
  ...missingUsage,
  id: 2,
  request_id: 'req-failed',
  content: 'Upstream rate limited',
  other: JSON.stringify({
    request_outcome: 'failed',
    usage_outcome: 'missing',
  }),
})

afterEach(() => {
  useAuthStore.setState(originalAuth)
  clients.splice(0).forEach((client) => client.clear())
  vi.restoreAllMocks()
})

async function renderLogs() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  clients.push(client)
  const root = createRootRoute({ component: PersonalCallLogs })
  const router = createRouter({
    routeTree: root,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return userEvent.setup()
}

describe('personal call list and detail', () => {
  it.each(['zhCN', 'zhTW'])(
    'renders the %s locale with valid Intl dates and token numbers',
    async (language) => {
      vi.spyOn(api, 'get').mockResolvedValue({
        data: { success: true, data: { items: [missingUsage], total: 1 } },
      })
      await i18next.changeLanguage(language)
      try {
        await renderLogs()
        expect(
          await screen.findByRole('button', { name: 'View call req-complete' })
        ).toBeInTheDocument()
        expect(
          screen.getByRole('heading', { name: 'Request succeeded' })
        ).toBeInTheDocument()
      } finally {
        await i18next.changeLanguage('en')
      }
    }
  )
  it('shows a completed missing-usage request as successful with unknown tokens and no balance or review requirement', async () => {
    const get = vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { items: [missingUsage, failed], total: 2 },
      },
    })
    const user = await renderLogs()
    await user.click(
      await screen.findByRole('button', { name: 'View call req-complete' })
    )
    expect(
      screen.getByRole('heading', { name: 'Request succeeded' })
    ).toBeInTheDocument()
    const detail = screen.getByLabelText('Call details')
    expect(within(detail).getByText('Usage not provided')).toBeInTheDocument()
    expect(within(detail).getAllByText('Unknown')).toHaveLength(4)
    expect(
      within(detail).getByRole('link', { name: 'View this Key' })
    ).toHaveAttribute('href', expect.stringContaining('filter=desktop-key'))
    expect(screen.queryByText('Current Balance')).not.toBeInTheDocument()
    expect(screen.queryByText('Usage pending review')).not.toBeInTheDocument()
    expect(String(get.mock.calls[0][0])).toContain('/api/log/self?')
  })
  it('lets users switch to failure details and return to the list by keyboard', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { items: [missingUsage, failed], total: 2 },
      },
    })
    const user = await renderLogs()
    const row = await screen.findByRole('button', {
      name: 'View call req-failed',
    })
    row.focus()
    await user.keyboard('{Enter}')
    expect(
      screen.getByRole('heading', { name: 'Request failed' })
    ).toBeInTheDocument()
    expect(screen.getByText('Upstream rate limited')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Back to call list' }))
    expect(row).toHaveFocus()
  })
  it('does not display a log belonging to another user in the self response', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { items: [{ ...missingUsage, user_id: 99 }], total: 1 },
      },
    })
    await renderLogs()
    expect(await screen.findByText('No calls found')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'View call req-complete' })
    ).not.toBeInTheDocument()
  })
  it('offers a retry after a failed API read and then displays an empty state', async () => {
    vi.spyOn(api, 'get')
      .mockRejectedValueOnce(new Error('Synthetic unavailable'))
      .mockResolvedValue({
        data: { success: true, data: { items: [], total: 0 } },
      })
    const user = await renderLogs()
    await screen.findByRole('alert')
    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('No calls found')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
  })
  it('submits Key filtering to the existing self log endpoint and preserves success semantics in the result filter', async () => {
    const get = vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: { items: [missingUsage, failed], total: 2 },
      },
    })
    const user = await renderLogs()
    await screen.findByRole('button', { name: 'View call req-complete' })
    await user.selectOptions(
      screen.getByLabelText('Results on this page'),
      'success'
    )
    expect(
      screen.queryByRole('button', { name: 'View call req-failed' })
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'View call req-complete' })
    ).toBeInTheDocument()
    await user.type(
      screen.getByRole('textbox', { name: 'API Key' }),
      'desktop-key'
    )
    await user.click(screen.getByRole('button', { name: 'Filter' }))
    await screen.findByRole('button', { name: 'View call req-complete' })
    expect(
      get.mock.calls.some(([url]) =>
        String(url).includes('token_name=desktop-key')
      )
    ).toBe(true)
  })
})
