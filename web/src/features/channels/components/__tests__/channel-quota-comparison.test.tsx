/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { getChannelQuotaChanges, getChannelQuotaHistory } from '../../api'
import { ChannelQuotaComparison } from '../channel-quota-comparison'

vi.mock('../../api', () => ({
  getChannelQuotaChanges: vi.fn(),
  getChannelQuotaHistory: vi.fn(),
}))
const now = Date.parse('2026-09-08T12:00:00Z') / 1000
const reset = Date.parse('2026-09-10T10:30:00Z') / 1000
const items = [1, 2].map((channel_id) => ({
  channel_id,
  name: `Channel ${channel_id}`,
  window_type: 'weekly',
  unit: 'percent',
  metric_type: 'codex_rate_limit',
  source: 'codex',
  window_seconds: 604800,
}))
function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ChannelQuotaComparison />
    </QueryClientProvider>
  )
}
beforeEach(() => {
  vi.spyOn(Date, 'now').mockReturnValue(now * 1000)
  vi.mocked(getChannelQuotaChanges).mockResolvedValue({
    success: true,
    data: {
      items: [
        ...items,
        { ...items[0], channel_id: 3, name: 'Dollars', unit: 'USD' },
      ],
    },
  })
  vi.mocked(getChannelQuotaHistory).mockImplementation(async (id, params) => ({
    success: true,
    data: {
      channel_id: id,
      start: Date.parse(params?.start ?? '') / 1000,
      end: Date.parse(params?.end ?? '') / 1000,
      limit: 5000,
      unit: 'percent',
      current: {
        status: 'success',
        observed_at: now - 60,
        reset_at: reset,
        available: 80,
      },
      points: [
        { timestamp: now - 120, status: 'success', available: 80.1234567 },
        {
          timestamp: now - 60,
          status: 'success',
          available: 80,
          consumption: 0.1234567,
        },
      ],
    },
  }))
})
afterEach(() => {
  vi.restoreAllMocks()
  vi.clearAllMocks()
})
test('loads two comparable channels together from raw remaining-based history', async () => {
  mount()
  await waitFor(() =>
    expect(getChannelQuotaHistory).toHaveBeenCalledWith(
      2,
      expect.objectContaining({
        consumption_basis: 'available',
        exact_identity: true,
        granularity: 'raw',
        limit: 5000,
      })
    )
  )
  expect(getChannelQuotaHistory).not.toHaveBeenCalledWith(3, expect.anything())
  expect(screen.getByRole('checkbox', { name: 'Channel 1 · #1' })).toBeChecked()
  expect(screen.getByRole('checkbox', { name: 'Channel 2 · #2' })).toBeChecked()
  expect(screen.getByLabelText('Metric')).toHaveValue('available')
  fireEvent.change(screen.getByLabelText('Metric'), {
    target: { value: 'consumption' },
  })
  expect(
    screen.getByText(
      'Estimated usage is the drop between valid remaining-quota samples. Resets, refills and missing intervals are excluded; this is not billed usage.'
    )
  ).toBeInTheDocument()
  for (const value of ['bar', 'area', 'scatter', 'line']) {
    fireEvent.change(screen.getByLabelText('Chart type'), { target: { value } })
    expect(screen.getByLabelText('Chart type')).toHaveValue(value)
  }
})
test('zoom requests a narrower absolute range instead of magnifying old buckets', async () => {
  mount()
  await waitFor(() =>
    expect(getChannelQuotaHistory).toHaveBeenCalledWith(1, expect.anything())
  )
  fireEvent.click(screen.getByRole('button', { name: 'Zoom in' }))
  await waitFor(() =>
    expect(getChannelQuotaHistory).toHaveBeenCalledWith(
      1,
      expect.objectContaining({
        start: new Date((now - 64800) * 1000).toISOString(),
        end: new Date((now - 21600) * 1000).toISOString(),
      })
    )
  )
  expect(screen.getByLabelText('Time range')).toHaveValue('custom')
})
test('weekly selection and previous cycle use the upstream Thursday reset', async () => {
  mount()
  await waitFor(() =>
    expect(getChannelQuotaHistory).toHaveBeenCalledWith(2, expect.anything())
  )
  fireEvent.change(screen.getByLabelText('Time range'), {
    target: { value: 'cycle' },
  })
  await waitFor(() =>
    expect(getChannelQuotaHistory).toHaveBeenCalledWith(
      1,
      expect.objectContaining({
        start: new Date((reset - 604800) * 1000).toISOString(),
        end: new Date(now * 1000).toISOString(),
      })
    )
  )
  await screen.findByText(/Channel 1 · Reset time/)
  fireEvent.click(screen.getByRole('button', { name: 'Previous cycle' }))
  await waitFor(() =>
    expect(getChannelQuotaHistory).toHaveBeenCalledWith(
      1,
      expect.objectContaining({
        start: new Date((reset - 1209600) * 1000).toISOString(),
        end: new Date((reset - 604800) * 1000).toISOString(),
      })
    )
  )
})
test('truncated history is visibly incomplete', async () => {
  vi.mocked(getChannelQuotaHistory).mockResolvedValue({
    success: true,
    data: {
      channel_id: 1,
      start: now - 86400,
      end: now,
      limit: 5000,
      points: [],
      complete: false,
    },
  })
  mount()
  expect(
    await screen.findByText(
      'History is incomplete. Zoom in or shorten the time range to load finer samples.'
    )
  ).toBeInTheDocument()
})

test('custom inputs follow a second zoom without losing selected channels', async () => {
  mount()
  await waitFor(() =>
    expect(getChannelQuotaHistory).toHaveBeenCalledWith(2, expect.anything())
  )
  vi.mocked(getChannelQuotaChanges).mockResolvedValue({
    success: true,
    data: { items: [] },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Zoom in' }))
  await screen.findByLabelText('Custom range start')
  fireEvent.click(screen.getByRole('button', { name: 'Zoom in' }))
  await waitFor(() =>
    expect(
      new Date(
        (screen.getByLabelText('Custom range start') as HTMLInputElement).value
      ).getTime() / 1000
    ).toBe(now - 54000)
  )
  expect(screen.getByRole('checkbox', { name: 'Channel 1 · #1' })).toBeChecked()
})

test('a failed first channel never borrows the second channel reset anchor', async () => {
  const history = vi.mocked(getChannelQuotaHistory).getMockImplementation()
  if (!history) throw new Error('Missing history fixture')
  vi.mocked(getChannelQuotaHistory).mockImplementation((id, params) =>
    id === 1 ? Promise.resolve({ success: false }) : history(id, params)
  )
  mount()
  await waitFor(() =>
    expect(getChannelQuotaHistory).toHaveBeenCalledWith(2, expect.anything())
  )
  fireEvent.change(screen.getByLabelText('Time range'), {
    target: { value: 'cycle' },
  })
  expect(
    await screen.findByText(
      'Choose a reset anchor because no provider reset time is available.'
    )
  ).toBeInTheDocument()
})

test('re-entering a weekly cycle uses refreshed provider reset time', async () => {
  mount()
  await screen.findByText(/Channel 1 · #1/)
  await waitFor(() =>
    expect(getChannelQuotaHistory).toHaveBeenCalledWith(2, expect.anything())
  )
  fireEvent.change(screen.getByLabelText('Time range'), {
    target: { value: 'cycle' },
  })
  await screen.findByText(/Channel 1 · Reset time/)
  fireEvent.change(screen.getByLabelText('Time range'), {
    target: { value: '24h' },
  })
  const history = vi.mocked(getChannelQuotaHistory).getMockImplementation()
  if (!history) throw new Error('Missing history fixture')
  const changedReset = reset + 3600
  vi.mocked(getChannelQuotaHistory).mockImplementation(async (id, params) => {
    const response = await history(id, params)
    if (response.data?.current) response.data.current.reset_at = changedReset
    return response
  })
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Refresh' })).toBeEnabled()
  )
  fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Refresh' })).toBeEnabled()
  )
  fireEvent.change(screen.getByLabelText('Time range'), {
    target: { value: 'cycle' },
  })
  await waitFor(() =>
    expect(getChannelQuotaHistory).toHaveBeenCalledWith(
      1,
      expect.objectContaining({
        start: new Date((changedReset - 604800) * 1000).toISOString(),
      })
    )
  )
})

test('catalogue truncation never suggests changing the chart range', async () => {
  vi.mocked(getChannelQuotaChanges).mockResolvedValue({
    success: true,
    data: { items, source_complete: false },
  })
  mount()
  expect(
    await screen.findByText(
      'The channel list is partial because its scan or item limit was reached. Some historical series may be missing; changing the chart time range does not expand this list.'
    )
  ).toBeInTheDocument()
  expect(
    screen.queryByText(
      'History is incomplete. Zoom in or shorten the time range to load finer samples.'
    )
  ).not.toBeInTheDocument()
})

test('an empty selected time range explains missing curves and offers a reset', async () => {
  vi.mocked(getChannelQuotaHistory).mockResolvedValue({
    success: true,
    data: {
      channel_id: 1,
      start: now - 86400,
      end: now,
      limit: 5000,
      points: [],
    },
  })
  mount()
  expect(
    await screen.findByText('No quota samples in this time range.')
  ).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Reset view' }))
  expect(screen.getByLabelText('Time range')).toHaveValue('24h')
})

test('sampling controls stay collapsed until advanced settings are opened', async () => {
  mount()
  await screen.findByRole('checkbox', { name: 'Channel 1 · #1' })
  expect(screen.getByLabelText('Resolution')).not.toBeVisible()
  fireEvent.click(screen.getByText('Advanced settings'))
  expect(screen.getByLabelText('Resolution')).toBeVisible()
})
