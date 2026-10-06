import { render, screen, within } from '@testing-library/react'
import { expect, test } from 'vitest'

import { TokenBudgetEvidence } from '../token-budget-evidence'

const evidence = {
  bound_source: 'openai_chat_context_window',
  input_tokens_bound: 1050000,
  max_output_tokens: 128000,
  reserved: 1050000,
  fee_reserved_usd: '6.53',
}

test('Chat reservation bounds remain distinct from unconfirmed actual usage', () => {
  render(<TokenBudgetEvidence evidence={evidence} />)
  const panel = screen.getByRole('region', {
    name: 'Reservation and actual usage',
  })
  expect(
    within(panel).getByText(
      'Chat reserves the 1,050,000-token context ceiling, not a measured input count. The total includes input and completion; the output cap is not added twice.'
    )
  ).toBeInTheDocument()
  expect(within(panel).getAllByText('1,050,000')).toHaveLength(2)
  expect(within(panel).getAllByText('Not confirmed')).toHaveLength(3)
  expect(within(panel).getByText('6.53')).toBeInTheDocument()
})

test('explicit actual zeros display as confirmed counts while missing fee remains unconfirmed', () => {
  render(
    <TokenBudgetEvidence
      evidence={{ ...evidence, actual_input: 0, actual_output: 0 }}
    />
  )
  expect(screen.getAllByText('0')).toHaveLength(3)
  expect(screen.queryByText('Not confirmed')).not.toBeInTheDocument()
  expect(
    screen.queryByText('Confirmed API usage cost (USD)')
  ).not.toBeInTheDocument()
})

test('invalid or missing reservation metadata is not shown as authoritative', () => {
  const view = render(
    <TokenBudgetEvidence evidence={{ ...evidence, reserved: -1 }} />
  )
  expect(screen.queryByRole('region')).not.toBeInTheDocument()
  view.rerender(<TokenBudgetEvidence evidence={{ actual_input: 100 }} />)
  expect(screen.queryByRole('region')).not.toBeInTheDocument()
})
