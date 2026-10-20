/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AnchorHTMLAttributes } from 'react'
import { expect, test, vi } from 'vitest'

import { getChannels } from '@/features/channels/api'
import { channelSchema } from '@/features/channels/types'

import { OverviewConnections } from '../overview-connections'

vi.mock('@/features/channels/api', () => ({ getChannels: vi.fn() }))
vi.mock('@tanstack/react-router', () => ({
  Link: (
    props: AnchorHTMLAttributes<HTMLAnchorElement> & {
      to: string
      params?: { section: string }
    }
  ) => {
    const { to, params, children, ...attributes } = props
    return (
      <a href={to.replace('$section', params?.section ?? '')} {...attributes}>
        {children}
      </a>
    )
  },
}))

const response = {
  success: true,
  data: {
    total: 9,
    page: 1,
    page_size: 4,
    items: [
      channelSchema.parse({
        id: 7,
        type: 57,
        name: 'Synthetic subscription',
        status: 1,
        key: 'synthetic-secret-sentinel',
        created_time: 1,
        test_time: 0,
        response_time: 0,
        balance_updated_time: 0,
      }),
      channelSchema.parse({
        id: 6,
        type: 1,
        name: 'Synthetic API channel',
        status: 2,
        key: '',
        balance: 99,
        created_time: 1,
        test_time: 0,
        response_time: 0,
        balance_updated_time: 0,
      }),
    ],
  },
}

test('desktop and mobile lists share one metadata read, preserve the server total and omit sensitive fields', async () => {
  vi.mocked(getChannels).mockResolvedValue(response)
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <OverviewConnections canRead showModels />
    </QueryClientProvider>
  )
  expect(await screen.findByText('9')).toBeInTheDocument()
  for (const id of ['connection-desktop-list', 'connection-mobile-list']) {
    const list = within(screen.getByTestId(id))
    expect(list.getByText('Synthetic subscription')).toBeInTheDocument()
    expect(list.getByText('Synthetic API channel')).toBeInTheDocument()
    expect(list.getByText('Enabled')).toBeInTheDocument()
    expect(list.getByText('Disabled')).toBeInTheDocument()
  }
  expect(getChannels).toHaveBeenCalledExactlyOnceWith({
    p: 1,
    page_size: 4,
    sort_by: 'id',
    sort_order: 'desc',
  })
  expect(
    JSON.stringify(
      client
        .getQueryCache()
        .getAll()
        .map((query) => query.state.data)
    )
  ).not.toContain('synthetic-secret-sentinel')
  expect(screen.queryByText('99')).not.toBeInTheDocument()
  expect(
    screen.getByText(
      'Configuration status does not confirm upstream availability.'
    )
  ).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Models' })).toHaveAttribute(
    'href',
    '/models/metadata'
  )
})

test('denied channel access starts no request and hides connection actions', () => {
  const client = new QueryClient()
  render(
    <QueryClientProvider client={client}>
      <OverviewConnections canRead={false} showModels={false} />
    </QueryClientProvider>
  )
  expect(getChannels).not.toHaveBeenCalled()
  expect(screen.queryByTestId('overview-connections')).not.toBeInTheDocument()
})

test('an invalid total is unavailable rather than a fabricated zero', async () => {
  vi.mocked(getChannels).mockResolvedValue({
    ...response,
    data: { ...response.data, total: -1 },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <OverviewConnections canRead showModels={false} />
    </QueryClientProvider>
  )
  expect(
    await screen.findByText('Unable to load connections')
  ).toBeInTheDocument()
  expect(screen.queryByText('0')).not.toBeInTheDocument()
  expect(screen.queryByRole('link', { name: 'Models' })).not.toBeInTheDocument()
  expect(getChannels).toHaveBeenCalledTimes(1)
})

test('explicit retry reads configuration again and revoking access removes the prior private list', async () => {
  vi.mocked(getChannels)
    .mockRejectedValueOnce(new Error('Synthetic read failure'))
    .mockResolvedValue(response)
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const user = userEvent.setup()
  const view = (canRead: boolean) => (
    <QueryClientProvider client={client}>
      <OverviewConnections canRead={canRead} showModels={false} />
    </QueryClientProvider>
  )
  const rendered = render(view(true))
  await screen.findByText('Unable to load connections')
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  await screen.findByText('9')
  expect(getChannels).toHaveBeenCalledTimes(2)
  rendered.rerender(view(false))
  expect(screen.queryByText('Synthetic subscription')).not.toBeInTheDocument()
  expect(screen.queryByTestId('overview-connections')).not.toBeInTheDocument()
})
