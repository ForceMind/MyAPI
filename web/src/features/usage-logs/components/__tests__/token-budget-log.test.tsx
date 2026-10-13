import { render, screen, within } from '@testing-library/react'
import { expect, test } from 'vitest'

import type { UsageLog } from '../../data/schema'
import { DetailsDialog } from '../dialogs/details-dialog'

const log: UsageLog = {
  id: 1,
  user_id: 2,
  created_at: 1756560000,
  type: 2,
  content: '',
  username: 'synthetic-user',
  token_name: 'synthetic-key',
  model_name: 'gpt-6.1-sol',
  quota: 1,
  prompt_tokens: 10,
  completion_tokens: 5,
  use_time: 1,
  is_stream: false,
  channel: 3,
  channel_name: 'Synthetic',
  token_id: 4,
  group: 'default',
  ip: '',
  request_id: '',
  upstream_request_id: '',
  other: JSON.stringify({
    admin_info: {
      token_budget: {
        bound_source: 'openai_chat_context_window',
        input_tokens_bound: 1050000,
        max_output_tokens: 128000,
        reserved: 1050000,
        fee_reserved_usd: '6.53',
        actual_input: 10,
        actual_output: 5,
        actual_fee_usd: '0.0001',
      },
    },
  }),
}

test.each([2, 5])(
  'administrator log type %s shows reserved ceiling separately from confirmed settlement',
  (type) => {
    render(
      <DetailsDialog
        log={{ ...log, type }}
        isAdmin
        open
        onOpenChange={() => {}}
      />
    )
    const evidence = screen.getByRole('region', {
      name: 'Reservation and actual usage',
    })
    expect(within(evidence).getAllByText('1,050,000')).toHaveLength(2)
    expect(within(evidence).getByText('15')).toBeInTheDocument()
    expect(within(evidence).getByText('0.0001')).toBeInTheDocument()
  }
)

test('nonadministrator never sees administrator budget evidence even if it is present in a response', () => {
  render(
    <DetailsDialog log={log} isAdmin={false} open onOpenChange={() => {}} />
  )
  expect(
    screen.queryByRole('region', { name: 'Reservation and actual usage' })
  ).not.toBeInTheDocument()
  expect(screen.queryByText('6.53')).not.toBeInTheDocument()
})

test('completed response with unknown usage explains the pending state without claiming a zero charge', () => {
  render(
    <DetailsDialog
      log={{
        ...log,
        type: 5,
        content: 'usage_pending_review',
        other: JSON.stringify({
          request_outcome: 'completed',
          usage_outcome: 'missing',
          settlement_status: 'pending_review',
          actual_quota: null,
        }),
      }}
      isAdmin={false}
      open
      onOpenChange={() => {}}
    />
  )
  expect(screen.getByText('Completed · usage pending')).toBeInTheDocument()
  expect(screen.queryByText('Error', { exact: true })).not.toBeInTheDocument()
  expect(
    screen.getByText(
      'The response completed, but upstream usage is unavailable. Usage and cost remain unconfirmed; existing reservations stay in place.'
    )
  ).toBeInTheDocument()
})

test('old pending logs without response evidence retain their original error status', () => {
  render(
    <DetailsDialog
      log={{
        ...log,
        type: 5,
        other: JSON.stringify({ settlement_status: 'pending_review' }),
      }}
      isAdmin={false}
      open
      onOpenChange={() => {}}
    />
  )
  expect(screen.getByText('Error', { exact: true })).toBeInTheDocument()
  expect(screen.queryByText('Completed · usage pending')).not.toBeInTheDocument()
})
