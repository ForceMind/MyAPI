/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { render, screen } from '@testing-library/react'
import type { AnchorHTMLAttributes } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import type { ChannelQuotaChangeItem } from '@/features/channels/types'

import { WindowCard } from '../components/window-card'

vi.mock('@tanstack/react-router', () => ({
  Link: (
    props: AnchorHTMLAttributes<HTMLAnchorElement> & {
      to: string
      search?: { quotaChannelId: number }
    }
  ) => {
    const { to, search, children, ...attributes } = props
    return (
      <a
        {...attributes}
        href={`${to}?quotaChannelId=${search?.quotaChannelId}`}
      >
        {children}
      </a>
    )
  },
}))

afterEach(() => vi.restoreAllMocks())

const sample: ChannelQuotaChangeItem = {
  channel_id: 7,
  name: 'Subscription connection',
  metric_type: 'codex_rate_limit',
  unit: 'percent',
  window_type: 'five_hour',
  window_seconds: 18000,
  status: 'success',
  current_available: 45,
  observed_at: 1000,
}

test('a failed sample explains the failure and links to recorded channel history without inventing usage', () => {
  vi.spyOn(Date, 'now').mockReturnValue(1100_000)
  render(<WindowCard item={{ ...sample, status: 'error' }} showHistory />)
  expect(screen.getByText('Usage sample failed')).toBeInTheDocument()
  expect(
    screen.getByText(
      'The latest usage sample failed. Inspect the channel history and connection.'
    )
  ).toBeInTheDocument()
  expect(screen.queryByText('55%')).not.toBeInTheDocument()
  expect(screen.queryByText('Unknown')).not.toBeInTheDocument()
  expect(
    screen.getByRole('link', { name: 'View channel history' })
  ).toHaveAttribute('href', '/channels?quotaChannelId=7')
})

test('an unsupported window is distinguished from a failed sample', () => {
  render(<WindowCard item={{ ...sample, status: 'unsupported' }} />)
  expect(screen.getByText('Window reporting unsupported')).toBeInTheDocument()
  expect(
    screen.getByText('This provider does not report this window.')
  ).toBeInTheDocument()
  expect(screen.queryByText('Usage sample failed')).not.toBeInTheDocument()
})

test('a fresh reported zero stays zero and has an available snapshot status', () => {
  vi.spyOn(Date, 'now').mockReturnValue(1100_000)
  render(<WindowCard item={{ ...sample, current_available: 100 }} />)
  expect(screen.getByText('0%')).toBeInTheDocument()
  expect(screen.getByText('Snapshot available')).toBeInTheDocument()
})

test('a stale numeric observation is labelled as past usage rather than current availability', () => {
  vi.spyOn(Date, 'now').mockReturnValue(1400_000)
  render(<WindowCard item={sample} />)
  expect(screen.getByText('55%')).toBeInTheDocument()
  expect(screen.getByText('Stale sample')).toBeInTheDocument()
  expect(screen.getByText('Last reported usage')).toBeInTheDocument()
  expect(screen.queryByText('Snapshot available')).not.toBeInTheDocument()
})

test('a missing observation time cannot make a numeric value current', () => {
  render(<WindowCard item={{ ...sample, observed_at: undefined }} />)
  expect(screen.getByText('Sample time unverified')).toBeInTheDocument()
  expect(screen.getByText('Last reported usage')).toBeInTheDocument()
  expect(screen.getByText('No timestamp')).toBeInTheDocument()
})

test('a hidden channel module does not expose its history action', () => {
  render(<WindowCard item={sample} showHistory={false} />)
  expect(
    screen.queryByRole('link', { name: 'View channel history' })
  ).not.toBeInTheDocument()
})
