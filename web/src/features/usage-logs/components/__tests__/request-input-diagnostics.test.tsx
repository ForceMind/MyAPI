import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import { RequestInputDiagnostics } from '../request-input-diagnostics'

test('a context failure displays its cause and an estimate without claiming reported usage', () => {
  render(
    <RequestInputDiagnostics
      other={{
        upstream_error_code: 'context_length_exceeded',
        request_input_estimate: {
          input_tokens: 922000,
          method: 'o200k_base_text_estimate',
          complete: true,
        },
      }}
    />
  )
  expect(screen.getByText('context_length_exceeded')).toBeInTheDocument()
  expect(screen.getByText('Estimated input Tokens')).toBeInTheDocument()
  expect(screen.getByText('922,000')).toBeInTheDocument()
  expect(
    screen.getByText(
      'Local estimate; not used as reported usage or for settlement.'
    )
  ).toBeInTheDocument()
})

test('a configured preflight rejection identifies the limit and that no upstream was sent', () => {
  render(
    <RequestInputDiagnostics
      other={{
        preflight_rejected: true,
        request_input_estimate: {
          input_tokens: 101,
          limit_tokens: 100,
          method: 'local_text_estimate',
          complete: true,
        },
      }}
    />
  )
  expect(screen.getByText('Blocked before upstream')).toBeInTheDocument()
  expect(screen.getByText('Configured input limit')).toBeInTheDocument()
  expect(screen.getByText('100')).toBeInTheDocument()
})

test('partial and invalid historical estimates do not pretend to be complete input usage', () => {
  const view = render(
    <RequestInputDiagnostics
      other={{
        request_input_estimate: {
          input_tokens: 8,
          method: 'local_text_estimate',
          complete: false,
        },
      }}
    />
  )
  expect(
    screen.getByText('Text only; media Tokens are not included.')
  ).toBeInTheDocument()
  view.rerender(
    <RequestInputDiagnostics
      other={{
        request_input_estimate: {
          input_tokens: -1,
          method: 'local_text_estimate',
          complete: true,
        },
      }}
    />
  )
  expect(screen.queryByText('Estimated input Tokens')).not.toBeInTheDocument()
})
