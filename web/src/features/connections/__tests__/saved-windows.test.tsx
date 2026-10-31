/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AxiosAdapter } from 'axios'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { mountConnections } from './connections-page-fixture'

vi.mock('@/hooks/use-sidebar-config', () => ({
  useIsSidebarModuleVisible: () => true,
}))

const originalAdapter = api.defaults.adapter
const originalMatchMedia = Object.getOwnPropertyDescriptor(window, 'matchMedia')
const originalVisibility = Object.getOwnPropertyDescriptor(
  document,
  'visibilityState'
)
let teardown: (() => void) | undefined

afterEach(() => {
  teardown?.()
  teardown = undefined
  api.defaults.adapter = originalAdapter
  if (originalMatchMedia) {
    Object.defineProperty(window, 'matchMedia', originalMatchMedia)
  }
  if (originalVisibility) {
    Object.defineProperty(document, 'visibilityState', originalVisibility)
  } else {
    Reflect.deleteProperty(document, 'visibilityState')
  }
  useAuthStore.getState().auth.reset('idle')
  vi.useRealTimers()
  vi.restoreAllMocks()
})

const now = Math.floor(Date.now() / 1000)
const saved = {
  channel_id: 7,
  series_id: 'a'.repeat(64),
  name: 'Synthetic connection',
  metric_type: 'codex_rate_limit',
  unit: 'percent',
  source: 'codex_wham_usage_primary',
  window_type: 'five_hour',
  window_seconds: 18000,
  status: 'success',
  current_available: 18,
  observed_at: now - 30,
}

async function mount(options: Parameters<typeof mountConnections>[0]) {
  const fixture = await mountConnections(options)
  teardown = fixture.teardown
  return fixture
}

test('connection detail retains distinct current WHAM, historical identity error and relay event with each observation time', async () => {
  await mount({
    entry: '/connections/7',
    windows: [
      saved,
      {
        ...saved,
        series_id: 'b'.repeat(64),
        status: 'error',
        source: 'codex_wham_usage',
        observed_at: now - 3600,
        current_available: null,
      },
      {
        ...saved,
        series_id: 'c'.repeat(64),
        source: 'codex_relay_usage_limit',
        status: 'error',
        window_type: 'none',
        window_seconds: undefined,
        current_available: null,
      },
    ],
  })
  const windows = await screen.findAllByTestId('connection-window')
  expect(windows).toHaveLength(3)
  expect(within(windows[0]).getByText('Snapshot available')).toBeVisible()
  expect(within(windows[0]).getByText('18% left')).toBeVisible()
  expect(
    within(windows[1]).getByText('Historical saved sample error')
  ).toBeVisible()
  expect(within(windows[1]).getByText(/not marked resolved/)).toBeVisible()
  expect(within(windows[2]).getByText('Relay usage-limit event')).toBeVisible()
  for (const window of windows) {
    expect(within(window).getByText(/^Observed /)).toBeVisible()
  }
  expect(screen.queryByText('Usage sample failed')).not.toBeInTheDocument()
})

test('the connection card keeps each saved series with its own status and observation time', async () => {
  await mount({
    windows: [
      saved,
      {
        ...saved,
        series_id: 'b'.repeat(64),
        status: 'error',
        source: 'codex_wham_usage',
        observed_at: now - 3600,
        current_available: null,
      },
      {
        ...saved,
        series_id: 'c'.repeat(64),
        source: 'codex_relay_usage_limit',
        status: 'error',
        window_type: 'none',
        current_available: null,
      },
    ],
  })
  const summary = await screen.findByRole('list', { name: 'Saved account windows' })
  expect(summary).toHaveTextContent('Historical saved sample error')
  expect(summary).toHaveTextContent('Snapshot available')
  expect(summary).toHaveTextContent('Relay usage-limit event')
  expect(summary.textContent?.match(/Observed /g)).toHaveLength(3)
  const series = within(summary).getAllByRole('listitem')
  expect(series).toHaveLength(3)
  for (const item of series) {
    expect(within(item).getByText(/Observed /)).toBeVisible()
  }
})

test.each(['/connections', '/connections/7'])('read-only refresh at %s reads saved windows only and preserves unknown usage', async (entry) => {
  const fixture = await mount({ entry, write: false, windows: [saved] })
  const user = userEvent.setup()
  await screen.findByRole('img', { name: /82% used, 18% left/ })
  const before = fixture.requests.length
  fixture.state.windows = [{ ...saved, current_available: null }]
  await user.click(screen.getByRole('button', { name: 'Refresh saved windows' }))
  await waitFor(() =>
    expect(screen.queryByRole('img', { name: /82% used/ })).not.toBeInTheDocument()
  )
  expect(screen.getByText(/No reliable usage sample/)).toBeVisible()
  expect(screen.queryByText(/100% left/)).not.toBeInTheDocument()
  expect(fixture.requests.slice(before)).toEqual([
    {
      method: 'get',
      path: '/api/channel/quota/changes',
      params: {
        channel_ids: '7',
        range: '24h',
        limit: 100,
        sort: 'observed_desc',
      },
    },
  ])
})

test('visible pages update saved windows and time every 60 seconds, hidden pages wait until visible and unmount stops refresh', async () => {
  vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] })
  vi.setSystemTime(now * 1000)
  let visibility = 'visible'
  Object.defineProperty(document, 'visibilityState', {
    configurable: true,
    get: () => visibility,
  })
  const fixture = await mount({
    entry: '/connections/7',
    windows: [{ ...saved, observed_at: now - 270 }],
  })
  await screen.findByText('Snapshot available')
  const before = fixture.requests.length
  await act(async () => vi.advanceTimersByTimeAsync(59_999))
  expect(fixture.requests).toHaveLength(before)
  await act(async () => vi.advanceTimersByTimeAsync(1))
  await screen.findByText('Stale sample')
  expect(fixture.requests.slice(before).map((request) => request.path)).toEqual([
    '/api/channel/quota/changes',
  ])
  visibility = 'hidden'
  act(() => document.dispatchEvent(new Event('visibilitychange')))
  await act(async () => vi.advanceTimersByTimeAsync(120_000))
  expect(fixture.requests).toHaveLength(before + 1)
  fixture.state.windows = [{ ...saved, observed_at: now + 180, current_available: 42 }]
  visibility = 'visible'
  await act(async () => document.dispatchEvent(new Event('visibilitychange')))
  await screen.findByText('42% left')
  expect(fixture.requests.slice(before).map((request) => request.path)).toEqual([
    '/api/channel/quota/changes',
    '/api/channel/quota/changes',
  ])
  fixture.teardown()
  teardown = undefined
  await act(async () => vi.advanceTimersByTimeAsync(60_000))
  expect(fixture.requests).toHaveLength(before + 2)
})

test('saved read failure remains a load error and explicit refresh recovers without testing the provider', async () => {
  const fixture = await mount({ entry: '/connections/7', windowsFail: true })
  await screen.findByText('Unable to load account windows')
  const before = fixture.requests.length
  fixture.state.windowsFail = false
  fixture.state.windows = [saved]
  await userEvent.setup().click(screen.getByRole('button', { name: 'Refresh saved windows' }))
  await screen.findByText('Snapshot available')
  expect(screen.queryByText('Unable to load account windows')).not.toBeInTheDocument()
  expect(fixture.requests.slice(before).map((request) => request.path)).toEqual([
    '/api/channel/quota/changes',
  ])
})

test('a pending saved-window refresh disables its button until the read finishes', async () => {
  await mount({ entry: '/connections/7', windows: [saved] })
  await screen.findByText('Snapshot available')
  const adapter = api.defaults.adapter as AxiosAdapter
  let release = () => {}
  const pending = new Promise<void>((resolve) => {
    release = resolve
  })
  api.defaults.adapter = async (config) => {
    await pending
    return adapter(config)
  }
  const refresh = screen.getByRole('button', { name: 'Refresh saved windows' })
  await userEvent.setup().click(refresh)
  expect(refresh).toBeDisabled()
  await act(async () => release())
  await waitFor(() => expect(refresh).toBeEnabled())
})

test('unverified read permission never starts a saved-window timer request', async () => {
  vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] })
  const fixture = await mount({ entry: '/connections/7', read: false })
  await screen.findByText('Connection access could not be verified.')
  await act(async () => vi.advanceTimersByTimeAsync(60_000))
  expect(fixture.requests.map((request) => request.path)).toEqual([
    '/api/user/self',
  ])
  expect(
    screen.queryByRole('button', { name: 'Refresh saved windows' })
  ).not.toBeInTheDocument()
})
