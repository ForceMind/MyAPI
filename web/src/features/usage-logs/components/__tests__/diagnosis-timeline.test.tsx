/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { render, screen, within } from '@testing-library/react'
import { expect, test } from 'vitest'

import type { UsageLog } from '../../data/schema'
import { DiagnosisTimeline } from '../diagnosis-timeline'

function record(other: Record<string, unknown>): UsageLog {
  return {
    id: 1, user_id: 2, created_at: 1792470000, type: 5,
    content: '', username: 'owner', token_name: 'identity-key', token_id: 8,
    model_name: 'identity-model', channel: 4, channel_name: 'identity-connection',
    quota: 0, prompt_tokens: 0, completion_tokens: 0, use_time: 1,
    is_stream: true, group: 'fixture-group', ip: '', request_id: 'REQ-TIMELINE',
    upstream_request_id: '', other: JSON.stringify(other),
  }
}

test('the timeline retains dispatch order and termination evidence without repeating the page identity', () => {
  render(<DiagnosisTimeline isAdmin log={record({
    request_outcome: 'completed', usage_outcome: 'missing', http_status: 200,
    settlement_status: 'pending_review', frt: 900,
    admin_info: { relay_attempts: [
      { channel_id: 4, key_index: 0, upstream_model: 'attempt-a', outcome: 'retryable_refusal', status: 429, cooldown_seconds: 12 },
      { channel_id: 5, key_index: 1, upstream_model: 'attempt-b', outcome: 'completed' },
    ] },
  })} />)
  const timeline = screen.getByRole('list', { name: 'Diagnosis timeline' })
  expect(within(timeline).getByText('Request received')).toBeVisible()
  const attempts = within(timeline).getByRole('list', { name: 'Relay attempts' })
  const rows = within(attempts).getAllByRole('listitem')
  expect(rows).toHaveLength(2)
  expect(rows[0]).toHaveTextContent('attempt-a')
  expect(rows[0]).toHaveTextContent('429')
  expect(rows[0]).toHaveTextContent('Future-request account hold: 12 seconds')
  expect(rows[1]).toHaveTextContent('attempt-b')
  expect(within(rows[1]).queryByText('Failure HTTP status')).not.toBeInTheDocument()
  expect(within(timeline).getByText('Upstream completed')).toBeVisible()
  expect(within(timeline).getByText('HTTP 200 · First byte 0.9 s')).toBeVisible()
  expect(within(timeline).getByText('Settlement pending review')).toBeVisible()
  expect(within(timeline).getByText('Usage not provided')).toBeVisible()
  for (const identity of ['identity-key', 'identity-model', 'identity-connection', 'REQ-TIMELINE']) {
    expect(within(timeline).queryByText(identity)).not.toBeInTheDocument()
  }
  expect(within(timeline).queryByText(/Recorded at/)).not.toBeInTheDocument()
})

test('a connection ID alone does not invent a dispatch attempt or repeat its identity', () => {
  render(<DiagnosisTimeline isAdmin log={record({ request_outcome: 'completed' })} />)

  expect(screen.getByText('Routing')).toBeVisible()
  expect(screen.getByText('No routing attempts were recorded.')).toBeVisible()
  expect(screen.queryByText('identity-connection')).not.toBeInTheDocument()
  expect(screen.queryByRole('list', { name: 'Relay attempts' })).not.toBeInTheDocument()
  expect(screen.getByText('No HTTP status recorded')).toBeVisible()
})

test('a rejected request retains its saved reason and has no routed or upstream completion step', () => {
  render(<DiagnosisTimeline isAdmin log={record({
    request_outcome: 'rejected', preflight_rejected: true,
    reject_reason: 'saved input limit reason', settlement_status: 'not_dispatched',
  })} />)

  expect(screen.getByText('saved input limit reason')).toBeVisible()
  expect(screen.getByText('Rejected before sending; nothing was sent upstream')).toBeVisible()
  expect(screen.getByText('Not sent, not charged')).toBeVisible()
  expect(screen.queryByText('Routing')).not.toBeInTheDocument()
  expect(screen.queryByText('Upstream completed')).not.toBeInTheDocument()
})

test('an ordinary user never receives administrator attempts from the timeline', () => {
  render(<DiagnosisTimeline isAdmin={false} log={record({
    request_outcome: 'failed',
    admin_info: { relay_attempts: [{ channel_id: 4, key_index: 0, upstream_model: 'admin-target', outcome: 'failed' }] },
  })} />)

  expect(screen.getByText('Routing details are visible to administrators only.')).toBeVisible()
  expect(screen.queryByText('admin-target')).not.toBeInTheDocument()
  expect(screen.queryByRole('list', { name: 'Relay attempts' })).not.toBeInTheDocument()
})
