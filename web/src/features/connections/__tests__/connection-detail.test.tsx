/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { CONNECTION, mountConnections } from './connections-page-fixture'

vi.mock('@/hooks/use-sidebar-config', () => ({
  useIsSidebarModuleVisible: () => true,
}))

const originalAdapter = api.defaults.adapter
const originalMatchMedia = Object.getOwnPropertyDescriptor(window, 'matchMedia')
let teardown: (() => void) | undefined

afterEach(() => {
  teardown?.()
  teardown = undefined
  api.defaults.adapter = originalAdapter
  if (originalMatchMedia) {
    Object.defineProperty(window, 'matchMedia', originalMatchMedia)
  } else {
    Reflect.deleteProperty(window, 'matchMedia')
  }
  useAuthStore.getState().auth.reset('idle')
  vi.restoreAllMocks()
})

async function mount(options: Parameters<typeof mountConnections>[0]) {
  const fixture = await mountConnections(options)
  teardown = fixture.teardown
  return fixture
}

const now = Math.floor(Date.now() / 1000)
const windowSample = {
  channel_id: 7,
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

test('saved windows keep one source and timestamp per series while unknown usage stays distinct from zero remaining', async () => {
  await mount({
    entry: '/connections/7',
    mobile: true,
    windows: [
      { ...windowSample, series_id: 'a'.repeat(64), current_available: 0 },
      {
        ...windowSample,
        series_id: 'b'.repeat(64),
        account_label: 'Synthetic unknown account',
        current_available: null,
        observed_at: 0,
      },
    ],
  })

  const rows = await screen.findAllByTestId('connection-window')
  expect(within(rows[0]).getByText('0% left')).toBeVisible()
  expect(
    within(rows[0]).getByRole('img', { name: /100% used, 0% left/ })
  ).toBeVisible()
  expect(within(rows[0]).getAllByText(/^Observed /)).toHaveLength(1)
  expect(
    within(rows[0]).getAllByText('codex_wham_usage_primary')
  ).toHaveLength(1)
  expect(within(rows[1]).getByText('Usage unavailable')).toBeVisible()
  expect(within(rows[1]).getByText('No timestamp')).toBeVisible()
  expect(within(rows[1]).getByText('No reliable usage sample')).toBeVisible()
  expect(within(rows[1]).queryByRole('img')).not.toBeInTheDocument()
  expect(within(rows[1]).queryByText('0% left')).not.toBeInTheDocument()
  expect(rows[0]).toHaveClass('grid', 'lg:grid-cols-2')
})

test('a saved-window read error has one refresh action and no duplicate retry action', async () => {
  await mount({ entry: '/connections/7', windowsFail: true })

  const section = await screen.findByRole('region', {
    name: 'Saved account windows',
  })
  expect(within(section).getByRole('alert')).toHaveTextContent(
    'Unable to load account windows'
  )
  expect(
    within(section).getAllByRole('button', { name: 'Refresh saved windows' })
  ).toHaveLength(1)
  expect(
    within(section).queryByRole('button', { name: 'Retry' })
  ).not.toBeInTheDocument()
})

test('routing keeps zero priority and weight plus all models in responsive configuration panels', async () => {
  await mount({ entry: '/connections/7?tab=routing' })

  const section = await screen.findByRole('region', {
    name: 'Routing configuration',
  })
  expect(
    within(section).getAllByRole('definition').map((entry) => entry.textContent)
  ).toEqual(['0', '0', 'default'])
  expect(within(section).getByText('alpha')).toBeVisible()
  expect(within(section).getByText('beta')).toBeVisible()
  expect(section).toHaveClass('grid', 'lg:grid-cols-2')
})

test('history retains historical identity errors and relay events alongside newer observations and saved test duration', async () => {
  await mount({
    entry: '/connections/7?tab=history',
    items: [{ ...CONNECTION, test_time: now - 60, response_time: 250 }],
    windows: [
      { ...windowSample, series_id: 'a'.repeat(64) },
      {
        ...windowSample,
        series_id: 'b'.repeat(64),
        account_label: 'Synthetic older identity',
        source: 'codex_wham_usage',
        status: 'error',
        current_available: null,
        observed_at: now - 3600,
      },
      {
        ...windowSample,
        series_id: 'c'.repeat(64),
        source: 'codex_relay_usage_limit',
        status: 'error',
        current_available: null,
      },
    ],
  })

  const history = await screen.findByRole('region', { name: 'History' })
  expect(
    await within(history).findByText(/sample: Historical saved sample error/)
  ).toBeVisible()
  expect(within(history).getByText(/not marked resolved/)).toBeVisible()
  expect(within(history).getByText(/sample: Relay usage-limit event/)).toBeVisible()
  expect(within(history).getByText(/sample: 18% left/)).toBeVisible()
  expect(within(history).getByText('Synthetic older identity')).toBeVisible()
  expect(
    within(history).getByText('Connectivity test saved · 250 ms')
  ).toBeVisible()
})

test('the detail keeps one last-test summary and labels its existing Test action as a quick test', async () => {
  const fixture = await mount({
    entry: '/connections/7',
    operate: true,
    items: [{ ...CONNECTION, test_time: now - 60, response_time: 250 }],
  })
  const page = within(await screen.findByTestId('connection-page'))
  await page.findByRole('heading', { name: 'Synthetic connection' })

  expect(await page.findAllByText(/^Last test:/)).toHaveLength(1)
  expect(page.getByRole('button', { name: 'Test' })).toHaveAttribute(
    'title',
    'Quick test'
  )
  await userEvent.setup().click(page.getByRole('tab', { name: 'History' }))
  expect(
    await page.findByText('Connectivity test saved · 250 ms')
  ).toBeVisible()
  expect(fixture.requests.every((request) => request.method === 'get')).toBe(true)
})


test('a subscription with saved windows still exposes that it has never been tested', async () => {
  await mount({ entry: '/connections/7', windows: [windowSample] })
  const page = within(await screen.findByTestId('connection-page'))
  expect(await page.findByText('Not tested')).toBeVisible()
  expect(page.getAllByText(/Last test/)).toHaveLength(1)
})
