/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import type { QueryClient } from '@tanstack/react-query'
import { screen, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'
import { useInspectorStore } from '@/stores/inspector-store'

import {
  KEY_21,
  mockKeysApi,
  newClient,
  renderKeysPage,
  requestLog,
  setViewport,
  signIn,
} from './keys-page-fixture'

const originalAuth = useAuthStore.getState()
let client: QueryClient

const logs = [
  requestLog({ id: 4 }),
  requestLog({
    id: 3,
    type: 5,
    other: JSON.stringify({ request_outcome: 'failed' }),
    prompt_tokens: 0,
    completion_tokens: 0,
  }),
  requestLog({
    id: 2,
    type: 5,
    other: JSON.stringify({
      request_outcome: 'completed',
      usage_outcome: 'missing',
    }),
    prompt_tokens: 0,
    completion_tokens: 0,
  }),
]

const sharePool = {
  pool_id: 7,
  snapshot_id: 70,
  revision: 4,
  channel_name: 'Subscription account A',
  plan_type: 'plus',
  window_slot: 'primary',
  window_type: 'five_hour',
  window_seconds: 18000,
  reset_at: 1797003600,
  observed_at: 1797000000,
  max_age_seconds: 300,
  sample_age_seconds: 30,
  state: 'active',
  allocated_bps: 7000,
  share_bps: 2000,
  estimated_used_micro: 8000000,
  unattributed_micro: 0,
  pending_attempts: 0,
  unsampled_attempts: 0,
  approximate: true,
  baseline_time: 1796999400,
}

beforeEach(() => {
  localStorage.clear()
  client = newClient()
  signIn(1)
  setViewport(1440)
})

afterEach(() => {
  client.clear()
  useAuthStore.setState(originalAuth)
  useInspectorStore.setState({ current: null })
  vi.restoreAllMocks()
  localStorage.clear()
})

function tile(label: string | RegExp) {
  const term = screen.getByText(label, { selector: 'dt' })
  return term.parentElement as HTMLElement
}

async function openStats() {
  await renderKeysPage(client, '/keys/21')
  await screen.findByRole('heading', { level: 1, name: 'Desktop application' })
}

test('requests in 24 hours are exact totals of the Key’s consumption and error records', async () => {
  const mocks = mockKeysApi({ keys: [KEY_21], logs, status: {} })
  await openStats()
  expect(
    await within(tile('Requests in 24 hours')).findByText('3')
  ).toBeVisible()
  const reads = mocks.get.mock.calls
    .map(([url]) => new URL(String(url), 'http://synthetic.invalid'))
    .filter((url) => url.searchParams.get('page_size') === '1')
  expect(reads.map((url) => url.searchParams.get('type')).sort()).toEqual([
    '2',
    '5',
  ])
  for (const url of reads) {
    // An ordinary user reads only their own records of this Key.
    expect(url.pathname).toBe('/api/log/self')
    expect(url.searchParams.get('token_id')).toBe('21')
    expect(Number(url.searchParams.get('start_timestamp'))).toBeGreaterThan(0)
  }
})

test('failures count only unfinished requests among the latest ones, and missing usage is named', async () => {
  mockKeysApi({ keys: [KEY_21], logs, status: {} })
  await openStats()
  const failed = tile('Failed')
  expect(await within(failed).findByText('1')).toBeVisible()
  expect(failed).toHaveTextContent('Among the latest 3 requests')
  expect(failed).toHaveTextContent('Usage not provided: 1')
})

test('unreadable totals are unknown, never zero', async () => {
  const mocks = mockKeysApi({ keys: [KEY_21], logs, status: {} })
  const original = mocks.get.getMockImplementation()
  if (!original) throw new Error('fixture has no GET handler')
  mocks.get.mockImplementation(async (url, config) => {
    if (String(url).includes('page_size=1&')) throw new Error('unavailable')
    return original(url, config)
  })
  await openStats()
  const total = tile('Requests in 24 hours')
  expect(await within(total).findByText('Unknown')).toBeVisible()
  expect(total).not.toHaveTextContent(/\b0\b/)
})

test('a held window share shows its estimated use, and an unknown baseline stays unknown', async () => {
  mockKeysApi({
    keys: [KEY_21],
    logs,
    status: {},
    extraGet: (path) =>
      path === '/api/token/21/subscription-share'
        ? {
            data: {
              success: true,
              data: {
                token_id: 21,
                pools: [
                  sharePool,
                  {
                    ...sharePool,
                    pool_id: 8,
                    snapshot_id: 80,
                    baseline_time: 0,
                  },
                ],
              },
            },
          }
        : undefined,
  })
  await openStats()
  await screen.findByText('5-hour window share used', { selector: 'dt' })
  const share = tile('5-hour window share used')
  expect(await within(share).findByText('40%')).toBeVisible()
  expect(share).toHaveTextContent(
    'Approximate, based on Subscription account A'
  )
})

test('without shares the third figure is the remaining quota', async () => {
  mockKeysApi({ keys: [KEY_21], logs, status: {} })
  await openStats()
  expect(
    await screen.findByText('Remaining quota', { selector: 'dt' })
  ).toBeVisible()
})

test('on a phone each request is one two-line row without the column header', async () => {
  setViewport(390)
  mockKeysApi({ keys: [KEY_21], logs, status: {} })
  await openStats()
  const rows = await screen.findAllByRole('button', { name: /synthetic-model/ })
  expect(rows).toHaveLength(3)
  expect(screen.queryByText('Duration')).not.toBeInTheDocument()
  // Model, usage and duration share the second line.
  expect(rows[0]).toHaveTextContent(/synthetic-model · 15 Tokens · 2 s/)
})
