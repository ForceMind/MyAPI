/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { operationsSignIn } from './operations-fixtures'

import { SystemInfo } from '..'

const instances = [
  {
    node_name: 'node-online-a',
    status: 'online',
    stale_after_seconds: 90,
    started_at: 1_700_000_000,
    last_seen_at: 1_700_000_100,
  },
  {
    node_name: 'node-online-b',
    status: 'online',
    stale_after_seconds: 90,
    started_at: 1_700_000_000,
    last_seen_at: 1_700_000_100,
  },
  {
    node_name: 'node-stale',
    status: 'stale',
    stale_after_seconds: 90,
    started_at: 1_700_000_000,
    last_seen_at: 1_700_000_000,
  },
]
const tasks = [
  {
    task_id: 'task-running',
    type: 'log_cleanup',
    status: 'running',
    updated_at: 1_700_000_100,
  },
  {
    task_id: 'task-failed',
    type: 'channel_test',
    status: 'failed',
    error: 'synthetic failure',
    updated_at: 1_700_000_100,
  },
  {
    task_id: 'task-done',
    type: 'model_update',
    status: 'succeeded',
    updated_at: 1_700_000_100,
  },
]
const clients: QueryClient[] = []
const originalAuth = useAuthStore.getState()
beforeEach(() => operationsSignIn())

afterEach(() => {
  for (const client of clients.splice(0)) client.clear()
  useAuthStore.setState(originalAuth)
  vi.restoreAllMocks()
})

async function renderStatus(instancesRead: 'ok' | 'failed') {
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/system-info/instances') {
      if (instancesRead === 'failed') {
        return { data: { success: false, message: 'synthetic unavailable' } }
      }
      return { data: { success: true, message: '', data: instances } }
    }
    if (url === '/api/system-task/list') {
      return { data: { success: true, message: '', data: tasks } }
    }
    if (url === '/api/operations/plans' || url === '/api/operations/instances') return { data: { success: true, data: [] } }
    throw new Error(`Unexpected GET ${url}`)
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const router = createRouter({
    routeTree: createRootRoute({ component: SystemInfo }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return get
}

/** The figure (StatTile) whose label is `label`. */
function figure(label: string) {
  const term = screen.getByText(label, { selector: 'dt' })
  const tile = term.parentElement
  if (!tile) throw new Error(`No figure for ${label}`)
  return within(tile)
}

test('figures count instances and recent tasks from the same reads as the lists', async () => {
  const get = await renderStatus('ok')

  expect(
    await screen.findByRole('heading', { level: 1, name: /System status/ })
  ).toBeInTheDocument()
  expect(await screen.findByText('node-stale')).toBeInTheDocument()
  expect(await figure('Online instances').findByText('2')).toBeInTheDocument()
  expect(await figure('Stale instances').findByText('1')).toBeInTheDocument()
  expect(await figure('Active Tasks').findByText('1')).toBeInTheDocument()
  expect(await figure('Failed tasks').findByText('1')).toBeInTheDocument()
  expect(
    get.mock.calls.filter(([url]) => url === '/api/system-info/instances')
  ).toHaveLength(1)
  expect(
    get.mock.calls.filter(([url]) => url === '/api/system-task/list')
  ).toHaveLength(1)
  expect(screen.getByRole('link', { name: 'Log Maintenance' })).toHaveAttribute(
    'href',
    '/system-settings/operations/logs'
  )
})

test('an unreadable instance list shows its figures as Unknown, never 0', async () => {
  await renderStatus('failed')

  expect(
    await screen.findByText('We could not load instances.')
  ).toBeInTheDocument()
  expect(
    await figure('Online instances').findByText('Unknown')
  ).toBeInTheDocument()
  expect(
    await figure('Stale instances').findByText('Unknown')
  ).toBeInTheDocument()
  expect(figure('Online instances').queryByText('0')).not.toBeInTheDocument()
  // The task figures are read separately and still count.
  expect(await figure('Failed tasks').findByText('1')).toBeInTheDocument()
})


test('losing Root access removes runtime records and does not request instances or tasks under the ordinary account', async () => {
  const get = await renderStatus('ok')
  await screen.findByText('node-stale')
  const before = get.mock.calls.length
  act(() => operationsSignIn(1, 'ordinary-session', 2))
  expect(screen.queryByText('node-stale')).not.toBeInTheDocument()
  expect(screen.queryByText('Log cleanup')).not.toBeInTheDocument()
  expect(screen.queryByRole('heading', { name: /System status/ })).not.toBeInTheDocument()
  expect(await screen.findByText('Unauthorized Access')).toBeVisible()
  expect(get.mock.calls).toHaveLength(before)
})

test.each(['actor', 'SID'] as const)('changing the Root %s waits for its own runtime snapshot without reusing former records', async (change) => {
  const get = await renderStatus('ok')
  await screen.findByText('node-stale')
  let release!: () => void
  const gate = new Promise<void>((resolve) => { release = resolve })
  const original = get.getMockImplementation()
  if (!original) throw new Error('Expected runtime API fixture')
  get.mockImplementation(async (url, config) => {
    if (url === '/api/system-info/instances') {
      await gate
      return { data: { success: true, data: [{ ...instances[0], node_name: 'next-root-node' }] } }
    }
    if (url === '/api/system-task/list') {
      await gate
      return { data: { success: true, data: [{ ...tasks[0], task_id: 'next-root-task', type: 'next-root-task-type' }] } }
    }
    return original(url, config)
  })
  act(() => operationsSignIn(100, 'next-root-session', change === 'actor' ? 2 : 1))
  expect(screen.queryByText('node-stale')).not.toBeInTheDocument()
  expect(screen.queryByText('Log cleanup')).not.toBeInTheDocument()
  expect(figure('Online instances').queryByText('2')).not.toBeInTheDocument()
  await act(async () => release())
  expect(await screen.findByText('next-root-node')).toBeVisible()
  expect(await screen.findByRole('row', { name: /next-root-task-type/ })).toBeVisible()
  await waitFor(() => expect(get.mock.calls.filter(([url]) => url === '/api/system-info/instances')).toHaveLength(2))
  expect(get.mock.calls.filter(([url]) => url === '/api/system-task/list')).toHaveLength(2)
  expect(screen.queryByText('node-stale')).not.toBeInTheDocument()
})
