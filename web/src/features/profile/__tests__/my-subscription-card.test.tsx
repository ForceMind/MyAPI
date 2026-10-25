/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import type { AxiosAdapter } from 'axios'
import type { AnchorHTMLAttributes } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { MySubscriptionCard } from '../components/my-subscription-card'

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    to,
    children,
    ...attributes
  }: AnchorHTMLAttributes<HTMLAnchorElement> & { to: string }) => (
    <a {...attributes} href={to}>
      {children}
    </a>
  ),
}))
vi.mock('@/hooks/use-sidebar-config', () => ({
  useIsSidebarModuleVisible: () => true,
}))

const NOW = Math.floor(Date.now() / 1000)
const subscription = {
  id: 3,
  user_id: 1,
  plan_id: 2,
  status: 'active',
  source: 'admin',
  start_time: NOW - 86400,
  end_time: NOW + 86400 * 20,
  amount_total: 5000000,
  amount_used: 1250000,
  next_reset_time: NOW + 3600,
}
const originalAdapter = api.defaults.adapter
let requests: { method?: string; url?: string }[]
let answer: unknown

beforeEach(() => {
  requests = []
  answer = {
    success: true,
    data: {
      billing_preference: 'subscription_first',
      subscriptions: [{ subscription }],
      all_subscriptions: [{ subscription }],
    },
  }
  useAuthStore.getState().auth.setUser({ id: 1, username: 'owner', role: 1 })
  const adapter: AxiosAdapter = async (config) => {
    requests.push({ method: config.method, url: config.url })
    return { config, status: 200, statusText: 'OK', headers: {}, data: answer }
  }
  api.defaults.adapter = adapter
})

afterEach(() => {
  api.defaults.adapter = originalAdapter
  useAuthStore.getState().auth.reset('idle')
  vi.restoreAllMocks()
})

function renderCard(canManage: boolean) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <MySubscriptionCard canManage={canManage} />
    </QueryClientProvider>
  )
}

test('an existing subscription is shown read only, with missing fields as not provided', async () => {
  answer = {
    success: true,
    data: {
      billing_preference: 'subscription_first',
      subscriptions: [],
      all_subscriptions: [
        { subscription: { ...subscription, next_reset_time: 0 } },
      ],
    },
  }
  renderCard(false)
  expect(
    await screen.findByRole('heading', { name: 'My subscription' })
  ).toBeVisible()
  expect(screen.getByText('Plan #2')).toBeVisible()
  expect(screen.getByText('Active')).toBeVisible()
  // The plan name and a reset time the server did not send are not invented.
  expect(screen.getAllByText('Not provided')).toHaveLength(2)
  // Read only: no purchase, renewal, preference or management control.
  expect(screen.queryByRole('button')).not.toBeInTheDocument()
  expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
  expect(screen.queryByRole('link')).not.toBeInTheDocument()
  expect(requests).toEqual([{ method: 'get', url: '/api/subscription/self' }])
})

test('without any subscription the card is not shown', async () => {
  answer = {
    success: true,
    data: {
      billing_preference: 'wallet_first',
      subscriptions: [],
      all_subscriptions: [],
    },
  }
  const { container } = renderCard(false)
  await waitFor(() => expect(requests).toHaveLength(1))
  expect(container).toBeEmptyDOMElement()
})

test('a refused or unreadable reply shows no card', async () => {
  answer = { success: false, message: 'blocked' }
  const { container } = renderCard(false)
  await waitFor(() => expect(requests).toHaveLength(1))
  expect(container).toBeEmptyDOMElement()
})

test('a record of another user is never shown as mine', async () => {
  answer = {
    success: true,
    data: {
      billing_preference: 'subscription_first',
      subscriptions: [],
      all_subscriptions: [{ subscription: { ...subscription, user_id: 9 } }],
    },
  }
  const { container } = renderCard(false)
  await waitFor(() => expect(requests).toHaveLength(1))
  expect(container).toBeEmptyDOMElement()
})

test('where the edition sells subscriptions the card links to the wallet that manages them', async () => {
  renderCard(true)
  expect(
    await screen.findByRole('link', { name: 'Manage in wallet' })
  ).toHaveAttribute('href', '/wallet')
  expect(screen.queryByRole('button')).not.toBeInTheDocument()
})
