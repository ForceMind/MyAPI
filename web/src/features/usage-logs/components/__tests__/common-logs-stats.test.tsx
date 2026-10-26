/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { useMediaQuery } from '@/hooks'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { CommonLogsStats } from '../common-logs-stats'
import { UsageLogsProvider, useUsageLogsContext } from '../usage-logs-provider'

const route = vi.hoisted(() => ({ search: {} as Record<string, unknown> }))
vi.mock('@tanstack/react-router', () => ({
  getRouteApi: () => ({ useSearch: () => route.search }),
}))
vi.mock('@/hooks', async () => {
  const actual = await vi.importActual<typeof import('@/hooks')>('@/hooks')
  return { ...actual, useMediaQuery: vi.fn(() => false) }
})
const original = useAuthStore.getState()
const clients: QueryClient[] = []
const partial = {
  records: 3,
  known_records: 2,
  unknown_records: 1,
  input_tokens: null,
  output_tokens: null,
  total_tokens: null,
  known_input_tokens: '1000',
  known_output_tokens: '100',
  known_total_tokens: '1100',
  cache_known_records: 1,
  cache_unknown_records: 2,
  cache_read_tokens: '200',
  cache_input_tokens: '800',
  cache_hit_rate: 0.25,
}

beforeEach(() => {
  route.search = {}
  vi.mocked(useMediaQuery).mockReturnValue(false)
  useAuthStore.setState({
    auth: { ...original.auth, user: { id: 1, username: 'owner', role: 100 } },
  })
})
afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.setState(original)
  vi.restoreAllMocks()
})

function PrivacyControl() {
  const { setSensitiveVisible } = useUsageLogsContext()
  return (
    <button type='button' onClick={() => setSensitiveVisible(false)}>
      Hide usage
    </button>
  )
}

function renderStats() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  clients.push(client)
  return render(
    <QueryClientProvider client={client}>
      <UsageLogsProvider showCosts={false}>
        <PrivacyControl />
        <CommonLogsStats />
      </UsageLogsProvider>
    </QueryClientProvider>
  )
}

test('mixed usage shows known Token subtotals and unknown records while keeping money hidden', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: { quota: 99, rpm: 1, tpm: 2, token_usage: partial },
    },
  })
  renderStats()
  const stats = await screen.findByRole('region', {
    name: 'Token usage statistics',
  })
  expect(within(stats).getByText('1,100')).toBeInTheDocument()
  expect(stats).toHaveTextContent('1 record with unknown usage')
  expect(stats).toHaveTextContent('Known subtotal')
  expect(stats).toHaveTextContent('25%')
  expect(stats).toHaveTextContent('Cache evidence: 1 / 3 records')
  expect(within(stats).queryByText('99')).not.toBeInTheDocument()
})

test('an empty selected range has zero Tokens and an undefined cache rate', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        quota: 0,
        rpm: 0,
        tpm: 0,
        token_usage: {
          ...partial,
          records: 0,
          known_records: 0,
          unknown_records: 0,
          input_tokens: '0',
          output_tokens: '0',
          total_tokens: '0',
          known_input_tokens: '0',
          known_output_tokens: '0',
          known_total_tokens: '0',
          cache_known_records: 0,
          cache_unknown_records: 0,
          cache_read_tokens: '0',
          cache_input_tokens: '0',
          cache_hit_rate: null,
        },
      },
    },
  })
  renderStats()
  const stats = await screen.findByRole('region', {
    name: 'Token usage statistics',
  })
  expect(stats).toHaveTextContent('No call records match this range.')
  expect(within(stats).getAllByText('0')).toHaveLength(3)
  expect(within(stats).getByText('Unknown')).toBeInTheDocument()
})

test('malformed aggregate evidence exposes an error instead of a plausible total', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        quota: 0,
        rpm: 0,
        tpm: 0,
        token_usage: { ...partial, known_total_tokens: '1101' },
      },
    },
  })
  renderStats()
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Failed to load usage statistics'
  )
  expect(screen.queryByText('1,101')).not.toBeInTheDocument()
})

test('the existing privacy control hides Token values and cache ratios', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: { quota: 0, rpm: 0, tpm: 0, token_usage: partial },
    },
  })
  renderStats()
  const stats = await screen.findByRole('region', {
    name: 'Token usage statistics',
  })
  await userEvent
    .setup()
    .click(screen.getByRole('button', { name: 'Hide usage' }))
  expect(stats).not.toHaveTextContent('1,100')
  expect(stats).not.toHaveTextContent('25%')
  expect(stats).toHaveTextContent('••••')
})

test('scope and actor changes cancel the old read and retain request filters without admin-only parameters', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: { quota: 0, rpm: 0, tpm: 0, token_usage: partial },
    },
  })
  renderStats()
  await screen.findByText('1,100')
  let signal: { aborted?: boolean } | undefined
  get.mockImplementationOnce((_url, config) => {
    signal = config?.signal
    return new Promise<never>(() => {})
  })
  route.search = {
    requestId: 'selected-request',
    upstreamRequestId: 'selected-upstream',
    username: 'other-user',
    channel: '7',
  }
  act(() =>
    useAuthStore.setState({
      auth: { ...original.auth, user: { id: 1, username: 'owner', role: 1 } },
    })
  )
  await waitFor(() => expect(get).toHaveBeenCalledTimes(2))
  expect(screen.queryByText('1,100')).not.toBeInTheDocument()
  const request = new URL(String(get.mock.calls[1][0]), 'http://localhost')
  expect(request.pathname).toBe('/api/log/self/stat')
  expect(request.searchParams.get('request_id')).toBe('selected-request')
  expect(request.searchParams.get('upstream_request_id')).toBe(
    'selected-upstream'
  )
  expect(request.searchParams.has('username')).toBe(false)
  expect(request.searchParams.has('channel')).toBe(false)
  get.mockResolvedValue({
    data: {
      success: true,
      data: {
        quota: 0,
        rpm: 0,
        tpm: 0,
        token_usage: {
          ...partial,
          records: 1,
          known_records: 1,
          unknown_records: 0,
          input_tokens: '7',
          output_tokens: '0',
          total_tokens: '7',
          known_input_tokens: '7',
          known_output_tokens: '0',
          known_total_tokens: '7',
          cache_known_records: 0,
          cache_unknown_records: 1,
          cache_read_tokens: '0',
          cache_input_tokens: '0',
          cache_hit_rate: null,
        },
      },
    },
  })
  act(() =>
    useAuthStore.setState({
      auth: {
        ...original.auth,
        user: { id: 2, username: 'second-owner', role: 1 },
      },
    })
  )
  expect(await screen.findAllByText('7')).toHaveLength(2)
  expect(screen.queryByText('1,100')).not.toBeInTheDocument()
  expect(signal?.aborted).toBe(true)
  expect(get).toHaveBeenCalledTimes(3)
})

test('reported cache zero displays zero percent instead of unknown', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        quota: 0,
        rpm: 0,
        tpm: 0,
        token_usage: {
          ...partial,
          records: 1,
          known_records: 1,
          unknown_records: 0,
          input_tokens: '100',
          output_tokens: '0',
          total_tokens: '100',
          known_input_tokens: '100',
          known_output_tokens: '0',
          known_total_tokens: '100',
          cache_known_records: 1,
          cache_unknown_records: 0,
          cache_read_tokens: '0',
          cache_input_tokens: '100',
          cache_hit_rate: 0,
        },
      },
    },
  })
  renderStats()
  const stats = await screen.findByRole('region', {
    name: 'Token usage statistics',
  })
  expect(stats).toHaveTextContent('0%')
  expect(stats).not.toHaveTextContent('Known subtotal')
})

test('all missing usage remains unknown rather than a zero total', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        quota: 0,
        rpm: 0,
        tpm: 0,
        token_usage: {
          ...partial,
          records: 2,
          known_records: 0,
          unknown_records: 2,
          known_input_tokens: '0',
          known_output_tokens: '0',
          known_total_tokens: '0',
          cache_known_records: 0,
          cache_unknown_records: 2,
          cache_read_tokens: '0',
          cache_input_tokens: '0',
          cache_hit_rate: null,
        },
      },
    },
  })
  renderStats()
  const stats = await screen.findByRole('region', {
    name: 'Token usage statistics',
  })
  expect(within(stats).getAllByText('Unknown')).toHaveLength(4)
  expect(stats).toHaveTextContent('2 records with unknown usage')
})

test('statistics above the safe integer limit retain exact digits', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        quota: 0,
        rpm: 0,
        tpm: 0,
        token_usage: {
          ...partial,
          records: 1,
          known_records: 1,
          unknown_records: 0,
          input_tokens: '9007199254740993',
          output_tokens: '1',
          total_tokens: '9007199254740994',
          known_input_tokens: '9007199254740993',
          known_output_tokens: '1',
          known_total_tokens: '9007199254740994',
          cache_known_records: 0,
          cache_unknown_records: 1,
          cache_read_tokens: '0',
          cache_input_tokens: '0',
          cache_hit_rate: null,
        },
      },
    },
  })
  renderStats()
  expect(await screen.findByText('9,007,199,254,740,994')).toBeInTheDocument()
})

test('a failed statistics read exposes retry and never fabricates zero usage', async () => {
  const get = vi
    .spyOn(api, 'get')
    .mockRejectedValueOnce(new Error('synthetic outage'))
    .mockResolvedValue({
      data: {
        success: true,
        data: { quota: 0, rpm: 0, tpm: 0, token_usage: partial },
      },
    })
  renderStats()
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Failed to load usage statistics'
  )
  expect(
    screen.queryByRole('region', { name: 'Token usage statistics' })
  ).not.toBeInTheDocument()
  await userEvent.setup().click(screen.getByRole('button', { name: 'Retry' }))
  expect(
    await screen.findByRole('region', { name: 'Token usage statistics' })
  ).toHaveTextContent('1,100')
  expect(get).toHaveBeenCalledTimes(2)
})

test('a phone keeps the four figures on one line and the evidence under the statistics basis', async () => {
  vi.mocked(useMediaQuery).mockReturnValue(true)
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: { quota: 0, rpm: 3, tpm: 4, token_usage: partial },
    },
  })
  renderStats()
  const stats = await screen.findByRole('region', {
    name: 'Token usage statistics',
  })
  expect(stats.querySelectorAll('dd')).toHaveLength(4)
  expect(within(stats).getByText('1,100')).toBeVisible()
  expect(within(stats).getByText('Known Token subtotal')).toBeVisible()
  const basis = within(stats).getByText('Statistics basis')
  expect(basis.tagName).toBe('SUMMARY')
  expect(basis.closest('details')).toHaveTextContent(
    '1 record with unknown usage'
  )
  expect(basis.closest('details')).toHaveTextContent('RPM')
  expect(basis.closest('details')).not.toHaveAttribute('open')
})

test('an older statistics API cannot manufacture Token or cache totals', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { quota: 0, rpm: 0, tpm: 0 } },
  })
  renderStats()
  expect(
    await screen.findByText('Token statistics unavailable')
  ).toBeInTheDocument()
})
