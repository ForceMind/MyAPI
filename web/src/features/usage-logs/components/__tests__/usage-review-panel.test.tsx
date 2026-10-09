import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { UsageReviewPanel } from '../usage-review-panel'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), post: vi.fn() } }))
const fixture = {
  request_id: 'review-fixture',
  user_id: 2,
  token_id: 3,
  state: 'usage_unknown',
  reserved_quota: 100,
  actual_quota: null,
}
const originalUser = useAuthStore.getState().auth.user
beforeEach(() => {
  vi.resetAllMocks()
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'fixture-root', role: 100 })
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: fixture },
  })
})
afterEach(() => {
  act(() => useAuthStore.getState().auth.setUser(originalUser))
})
function renderReview() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <UsageReviewPanel requestId='review-fixture' />
    </QueryClientProvider>
  )
}

test('automatic settlement pending is read-only instead of requesting manual quota and evidence', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: {
        ...fixture,
        settlement_status: 'pending',
        recovery_block_reason: 'automatic_settlement_pending',
        can_recover_text_dispatch: false,
      },
    },
  })
  renderReview()
  await screen.findByRole('region', { name: 'Usage pending review' })
  expect(
    screen.queryByLabelText('Confirmed quota (internal units)')
  ).not.toBeInTheDocument()
  expect(screen.queryByLabelText('Evidence reference')).not.toBeInTheDocument()
  expect(screen.getByText('Automatic settlement pending')).toBeInTheDocument()
  expect(api.post).not.toHaveBeenCalled()
})

test('requires evidence and confirmation before sending one exact recovery request', async () => {
  const user = userEvent.setup()
  vi.mocked(api.post).mockResolvedValue({
    data: {
      success: true,
      data: { ...fixture, state: 'settled', actual_quota: 120 },
    },
  })
  renderReview()
  await user.click(
    await screen.findByRole('button', {
      name: 'Advanced: manual reconciliation',
    })
  )
  const submit = await screen.findByRole('button', {
    name: 'Confirm reconciliation',
  })
  await user.click(submit)
  expect(api.post).not.toHaveBeenCalled()
  await user.type(
    screen.getByLabelText('Confirmed quota (internal units)'),
    '120'
  )
  await user.type(
    screen.getByLabelText('Evidence reference'),
    'synthetic invoice reference'
  )
  await user.click(submit)
  expect(api.post).not.toHaveBeenCalled()
  await user.click(
    screen.getByRole('checkbox', {
      name: 'I verified the evidence and frozen pricing.',
    })
  )
  await user.click(submit)
  expect(await screen.findByText('Reconciled')).toBeInTheDocument()
  expect(api.post).toHaveBeenCalledTimes(1)
  expect(api.post).toHaveBeenCalledWith(
    '/api/usage-review/review-fixture/reconcile',
    {
      actual_quota: 120,
      evidence_reference: 'synthetic invoice reference',
      confirmed_reliable_evidence: true,
    },
    { skipErrorHandler: true }
  )
})

test('allows an owner to read status without exposing root recovery controls', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 2, username: 'fixture-owner', role: 1 })
  renderReview()
  expect(await screen.findByText('Usage pending review')).toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Confirm reconciliation' })
  ).not.toBeInTheDocument()
  expect(api.post).not.toHaveBeenCalled()
})

test('rejects mismatched request data and shows a retry rather than another request details', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: { ...fixture, request_id: 'someone-else' } },
  })
  renderReview()
  expect(await screen.findByRole('alert')).toHaveTextContent('Operation failed')
  expect(
    screen.queryByRole('button', { name: 'Confirm reconciliation' })
  ).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
})

test('clears an unfinished recovery form when the signed-in user changes', async () => {
  const user = userEvent.setup()
  renderReview()
  await user.click(
    await screen.findByRole('button', {
      name: 'Advanced: manual reconciliation',
    })
  )
  await user.type(
    await screen.findByLabelText('Evidence reference'),
    'private fixture reference'
  )
  act(() =>
    useAuthStore
      .getState()
      .auth.setUser({ id: 2, username: 'fixture-owner', role: 1 })
  )
  await waitFor(() =>
    expect(
      screen.queryByDisplayValue('private fixture reference')
    ).not.toBeInTheDocument()
  )
  expect(
    screen.queryByRole('button', { name: 'Confirm reconciliation' })
  ).not.toBeInTheDocument()
})

test('requires finished-request confirmation to recover a persisted open dispatch', async () => {
  const user = userEvent.setup()
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: {
        ...fixture,
        state: 'open',
        text_dispatch_pending: true,
        can_recover_text_dispatch: true,
      },
    },
  })
  vi.mocked(api.post).mockResolvedValue({
    data: {
      success: true,
      data: { ...fixture, state: 'applied', actual_quota: 20 },
    },
  })
  renderReview()
  await user.click(
    await screen.findByRole('button', {
      name: 'Advanced: manual reconciliation',
    })
  )
  const amount = await screen.findByLabelText(
    'Confirmed quota (internal units)'
  )
  await user.type(amount, '20')
  await user.type(
    screen.getByLabelText('Evidence reference'),
    'synthetic completed request'
  )
  await user.click(
    screen.getByRole('button', { name: 'Confirm reconciliation' })
  )
  expect(api.post).not.toHaveBeenCalled()
  await user.click(
    screen.getByRole('checkbox', {
      name: 'I verified that the request ended and the actual usage and frozen pricing are correct.',
    })
  )
  await user.click(
    screen.getByRole('button', { name: 'Confirm reconciliation' })
  )
  expect(await screen.findByText('Reconciled')).toBeInTheDocument()
  expect(api.post).toHaveBeenCalledWith(
    '/api/usage-review/review-fixture/recover-dispatch',
    {
      actual_quota: 20,
      evidence_reference: 'synthetic completed request',
      confirmed_reliable_evidence: true,
      confirmed_request_finished: true,
    },
    { skipErrorHandler: true }
  )
})

test('retries the same dispatch recovery after a lost response even if refresh shows the held state', async () => {
  const user = userEvent.setup()
  vi.mocked(api.get).mockResolvedValueOnce({
    data: {
      success: true,
      data: { ...fixture, state: 'open', can_recover_text_dispatch: true },
    },
  })
  vi.mocked(api.post)
    .mockRejectedValueOnce(new Error('synthetic acknowledgement loss'))
    .mockResolvedValueOnce({
      data: {
        success: true,
        data: { ...fixture, state: 'applied', actual_quota: 20 },
      },
    })
  renderReview()
  await user.click(
    await screen.findByRole('button', {
      name: 'Advanced: manual reconciliation',
    })
  )
  await user.type(
    await screen.findByLabelText('Confirmed quota (internal units)'),
    '20'
  )
  await user.type(
    screen.getByLabelText('Evidence reference'),
    'unchanged synthetic evidence'
  )
  await user.click(
    screen.getByRole('checkbox', {
      name: 'I verified that the request ended and the actual usage and frozen pricing are correct.',
    })
  )
  await user.click(
    screen.getByRole('button', { name: 'Confirm reconciliation' })
  )
  expect(await screen.findByText('Operation failed')).toBeInTheDocument()
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Confirm reconciliation' })
    ).toBeEnabled()
  )
  expect(
    screen.getByLabelText('Confirmed quota (internal units)')
  ).toHaveAttribute('readonly')
  await user.click(
    screen.getByRole('button', { name: 'Confirm reconciliation' })
  )
  expect(await screen.findByText('Reconciled')).toBeInTheDocument()
  expect(api.post).toHaveBeenCalledTimes(2)
  expect(vi.mocked(api.post).mock.calls[1]).toEqual(
    vi.mocked(api.post).mock.calls[0]
  )
})

test('pending Chat review preserves bounds and explicit zero without treating missing actual output as zero', async () => {
  const user = userEvent.setup()
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: {
        ...fixture,
        token_budget: {
          request_id: fixture.request_id,
          user_id: 2,
          token_id: 3,
          state: 'usage_unknown',
          fee_enabled: false,
          bound_source: 'openai_chat_context_window',
          input_tokens_bound: 1050000,
          max_output_tokens: 128000,
          reserved: 1050000,
          actual_input: 0,
          actual_output: null,
        },
      },
    },
  })
  renderReview()
  expect(
    await screen.findByRole('region', { name: 'Reservation and actual usage' })
  ).toBeInTheDocument()
  expect(screen.getAllByText('1,050,000')).toHaveLength(2)
  expect(screen.getAllByText('Not confirmed')).toHaveLength(2)
  await user.click(
    screen.getByRole('button', { name: 'Advanced: manual reconciliation' })
  )
  expect(screen.getByLabelText('Confirmed input tokens')).toHaveValue('0')
  expect(screen.getByLabelText('Confirmed input tokens')).toHaveAttribute(
    'readonly'
  )
  expect(screen.getByLabelText('Confirmed output tokens')).toHaveValue('')
  expect(api.post).not.toHaveBeenCalled()
})

test.each([
  ['applied', 'Settlement applied'],
  [
    'applied_journal_pending',
    'Settlement applied; record finalization pending',
  ],
  ['manual', 'Settlement needs administrator attention'],
])(
  'automatic %s reports its exact status without manual recovery controls',
  async (status, title) => {
    vi.mocked(api.get).mockResolvedValue({
      data: {
        success: true,
        data: {
          ...fixture,
          state: 'prepared',
          settlement_status: status,
          can_reconcile_usage: false,
          actual_quota: status === 'manual' ? null : 120,
          review_metadata: 'private pricing fixture',
        },
      },
    })
    renderReview()
    expect(await screen.findByText(title)).toBeInTheDocument()
    expect(screen.queryByText('Reconciled')).not.toBeInTheDocument()
    expect(
      screen.queryByText('private pricing fixture')
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Advanced: manual reconciliation' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByLabelText('Evidence reference')
    ).not.toBeInTheDocument()
    expect(api.post).not.toHaveBeenCalled()
  }
)

test('manual review requires an explicit keyboard expansion and collapsing never submits', async () => {
  const user = userEvent.setup()
  renderReview()
  const advanced = await screen.findByRole('button', {
    name: 'Advanced: manual reconciliation',
  })
  expect(advanced).toHaveAttribute('aria-expanded', 'false')
  expect(
    screen.queryByLabelText('Confirmed quota (internal units)')
  ).not.toBeInTheDocument()
  advanced.focus()
  await user.keyboard('{Enter}')
  expect(advanced).toHaveAttribute('aria-expanded', 'true')
  expect(screen.getByLabelText('Confirmed quota (internal units)')).toHaveValue(
    ''
  )
  expect(screen.getByLabelText('Evidence reference')).toHaveValue('')
  await user.click(advanced)
  expect(advanced).toHaveAttribute('aria-expanded', 'false')
  expect(screen.queryByLabelText('Evidence reference')).not.toBeInTheDocument()
  expect(api.post).not.toHaveBeenCalled()
})

test('explicitly denied reconciliation stays read-only even for an otherwise recoverable root review', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: {
        ...fixture,
        can_recover_text_dispatch: true,
        can_reconcile_usage: false,
      },
    },
  })
  renderReview()
  await screen.findByRole('region', { name: 'Usage pending review' })
  expect(
    screen.queryByRole('button', { name: 'Advanced: manual reconciliation' })
  ).not.toBeInTheDocument()
  expect(api.post).not.toHaveBeenCalled()
})

test('refreshing the same request replaces a manual draft with automatic status and then applied status without posting', async () => {
  const user = userEvent.setup()
  vi.mocked(api.get)
    .mockResolvedValueOnce({ data: { success: true, data: fixture } })
    .mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          ...fixture,
          settlement_status: 'pending',
          can_reconcile_usage: false,
        },
      },
    })
    .mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          ...fixture,
          state: 'prepared',
          settlement_status: 'applied_journal_pending',
          can_reconcile_usage: false,
          actual_quota: 120,
        },
      },
    })
  renderReview()
  await user.click(
    await screen.findByRole('button', {
      name: 'Advanced: manual reconciliation',
    })
  )
  await user.type(
    screen.getByLabelText('Evidence reference'),
    'unfinished private evidence'
  )
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  expect(
    await screen.findByText('Automatic settlement pending')
  ).toBeInTheDocument()
  expect(
    screen.queryByDisplayValue('unfinished private evidence')
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Advanced: manual reconciliation' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByText(/Confirmed quota \(internal units\):/)
  ).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  expect(
    await screen.findByText('Settlement applied; record finalization pending')
  ).toBeInTheDocument()
  expect(
    screen.getByText('Confirmed quota (internal units): 120')
  ).toBeInTheDocument()
  expect(api.post).not.toHaveBeenCalled()
})

test('a refresh error hides a previously eligible manual draft and does not expose raw server details', async () => {
  const user = userEvent.setup()
  vi.mocked(api.get)
    .mockResolvedValueOnce({ data: { success: true, data: fixture } })
    .mockRejectedValueOnce(
      new Error('SELECT secret FROM database; private credential')
    )
  renderReview()
  await user.click(
    await screen.findByRole('button', {
      name: 'Advanced: manual reconciliation',
    })
  )
  await user.type(
    screen.getByLabelText('Evidence reference'),
    'private unfinished draft'
  )
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Operation failed')
  expect(
    screen.queryByRole('button', { name: 'Confirm reconciliation' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByDisplayValue('private unfinished draft')
  ).not.toBeInTheDocument()
  expect(
    screen.queryByText(/SELECT|private credential/)
  ).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
  expect(api.post).not.toHaveBeenCalled()
})

test('an unknown automatic settlement status fails closed instead of exposing manual fields', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: { ...fixture, settlement_status: 'unrecognized' },
    },
  })
  renderReview()
  expect(await screen.findByRole('alert')).toHaveTextContent('Operation failed')
  expect(
    screen.queryByRole('button', { name: 'Advanced: manual reconciliation' })
  ).not.toBeInTheDocument()
  expect(api.post).not.toHaveBeenCalled()
})

test('an owner can refresh automatic status without seeing root evidence or manual controls', async () => {
  const user = userEvent.setup()
  useAuthStore
    .getState()
    .auth.setUser({ id: 2, username: 'fixture-owner', role: 1 })
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: {
        ...fixture,
        settlement_status: 'pending',
        review_metadata: 'root pricing detail',
        can_reconcile_usage: true,
      },
    },
  })
  renderReview()
  expect(
    await screen.findByText('Automatic settlement pending')
  ).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  await waitFor(() => expect(api.get).toHaveBeenCalledTimes(2))
  expect(screen.queryByText('root pricing detail')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Advanced: manual reconciliation' })
  ).not.toBeInTheDocument()
  expect(api.post).not.toHaveBeenCalled()
})

test('applied quota preserves a separately authorized strict-token review without guessing missing counts', async () => {
  const user = userEvent.setup()
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: {
        ...fixture,
        settlement_status: 'applied_journal_pending',
        can_reconcile_usage: true,
        actual_quota: 0,
        token_budget: {
          request_id: fixture.request_id,
          user_id: 2,
          token_id: 3,
          state: 'sent',
          fee_enabled: false,
          reserved: 30,
          actual_input: 10,
          actual_output: null,
        },
      },
    },
  })
  renderReview()
  await screen.findByText('Settlement applied; record finalization pending')
  await user.click(
    screen.getByRole('button', { name: 'Advanced: manual reconciliation' })
  )
  expect(screen.getByLabelText('Confirmed quota (internal units)')).toHaveValue(
    '0'
  )
  expect(
    screen.getByLabelText('Confirmed quota (internal units)')
  ).toHaveAttribute('readonly')
  expect(screen.getByLabelText('Confirmed input tokens')).toHaveValue('10')
  expect(screen.getByLabelText('Confirmed input tokens')).toHaveAttribute(
    'readonly'
  )
  expect(screen.getByLabelText('Confirmed output tokens')).toHaveValue('')
  await user.click(
    screen.getByRole('button', { name: 'Confirm reconciliation' })
  )
  expect(api.post).not.toHaveBeenCalled()
})

test('new confirmed values on the same request replace an unfinished draft and reset confirmation', async () => {
  const user = userEvent.setup()
  const budget = {
    request_id: fixture.request_id,
    user_id: 2,
    token_id: 3,
    state: 'usage_unknown',
    fee_enabled: false,
    reserved: 30,
    actual_input: null,
    actual_output: null,
  }
  vi.mocked(api.get)
    .mockResolvedValueOnce({
      data: { success: true, data: { ...fixture, token_budget: budget } },
    })
    .mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          ...fixture,
          actual_quota: 120,
          token_budget: { ...budget, actual_input: 0, actual_output: 9 },
        },
      },
    })
  renderReview()
  await user.click(
    await screen.findByRole('button', {
      name: 'Advanced: manual reconciliation',
    })
  )
  await user.type(screen.getByLabelText('Confirmed input tokens'), '11')
  await user.type(
    screen.getByLabelText('Evidence reference'),
    'old draft evidence'
  )
  await user.click(
    screen.getByRole('checkbox', {
      name: 'I verified that the request ended and the actual token counts and frozen pricing are correct.',
    })
  )
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Advanced: manual reconciliation' })
    ).toHaveAttribute('aria-expanded', 'false')
  )
  await user.click(
    screen.getByRole('button', { name: 'Advanced: manual reconciliation' })
  )
  expect(screen.getByLabelText('Confirmed quota (internal units)')).toHaveValue(
    '120'
  )
  expect(screen.getByLabelText('Confirmed input tokens')).toHaveValue('0')
  expect(screen.getByLabelText('Confirmed input tokens')).toHaveAttribute(
    'readonly'
  )
  expect(screen.getByLabelText('Confirmed output tokens')).toHaveValue('9')
  expect(screen.getByLabelText('Evidence reference')).toHaveValue('')
  expect(screen.getByRole('checkbox')).not.toBeChecked()
  expect(api.post).not.toHaveBeenCalled()
})

test('a decision learned after a lost response keeps the confirmed identical manual command retryable', async () => {
  const user = userEvent.setup()
  vi.mocked(api.get)
    .mockResolvedValueOnce({ data: { success: true, data: fixture } })
    .mockResolvedValue({
      data: {
        success: true,
        data: {
          ...fixture,
          state: 'review_pending',
          settlement_status: 'none',
          can_reconcile_usage: true,
          decision: {
            id: 8,
            actual_quota: 120,
            evidence_reference: 'verified immutable evidence',
          },
        },
      },
    })
  vi.mocked(api.post)
    .mockRejectedValueOnce(new Error('lost acknowledgement'))
    .mockResolvedValueOnce({
      data: {
        success: true,
        data: { ...fixture, state: 'settled', actual_quota: 120 },
      },
    })
  renderReview()
  await user.click(
    await screen.findByRole('button', {
      name: 'Advanced: manual reconciliation',
    })
  )
  await user.type(
    screen.getByLabelText('Confirmed quota (internal units)'),
    '120'
  )
  await user.type(
    screen.getByLabelText('Evidence reference'),
    'verified immutable evidence'
  )
  await user.click(
    screen.getByRole('checkbox', {
      name: 'I verified the evidence and frozen pricing.',
    })
  )
  await user.click(
    screen.getByRole('button', { name: 'Confirm reconciliation' })
  )
  await screen.findByText('Operation failed')
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Advanced: manual reconciliation' })
    ).toHaveAttribute('aria-expanded', 'false')
  )
  await user.click(
    screen.getByRole('button', { name: 'Advanced: manual reconciliation' })
  )
  expect(screen.getByRole('checkbox')).toBeChecked()
  expect(screen.getByRole('checkbox')).toHaveAttribute('aria-disabled', 'true')
  await user.click(
    screen.getByRole('button', { name: 'Confirm reconciliation' })
  )
  await screen.findByText('Reconciled')
  expect(api.post).toHaveBeenCalledTimes(2)
  expect(vi.mocked(api.post).mock.calls[1]).toEqual(
    vi.mocked(api.post).mock.calls[0]
  )
})

test('changed authoritative evidence after a failed command requires a newly confirmed command instead of silently retrying old values', async () => {
  const user = userEvent.setup()
  const budget = {
    request_id: fixture.request_id,
    user_id: 2,
    token_id: 3,
    state: 'usage_unknown',
    fee_enabled: false,
    reserved: 30,
    actual_input: 10,
    actual_output: 5,
  }
  vi.mocked(api.get)
    .mockResolvedValueOnce({
      data: { success: true, data: { ...fixture, token_budget: budget } },
    })
    .mockResolvedValue({
      data: {
        success: true,
        data: {
          ...fixture,
          actual_quota: 130,
          settlement_status: 'applied_journal_pending',
          can_reconcile_usage: true,
          token_budget: budget,
        },
      },
    })
  vi.mocked(api.post)
    .mockRejectedValueOnce(new Error('uncertain original command'))
    .mockResolvedValueOnce({
      data: {
        success: true,
        data: { ...fixture, state: 'settled', actual_quota: 130 },
      },
    })
  renderReview()
  await user.click(
    await screen.findByRole('button', {
      name: 'Advanced: manual reconciliation',
    })
  )
  await user.type(
    screen.getByLabelText('Confirmed quota (internal units)'),
    '120'
  )
  await user.type(
    screen.getByLabelText('Evidence reference'),
    'original evidence'
  )
  await user.click(screen.getByRole('checkbox'))
  await user.click(
    screen.getByRole('button', { name: 'Confirm reconciliation' })
  )
  await screen.findByText('Settlement applied; record finalization pending')
  await user.click(
    screen.getByRole('button', { name: 'Advanced: manual reconciliation' })
  )
  expect(screen.getByLabelText('Confirmed quota (internal units)')).toHaveValue(
    '130'
  )
  expect(screen.getByRole('checkbox')).not.toBeChecked()
  expect(screen.getByRole('checkbox')).not.toHaveAttribute(
    'aria-disabled',
    'true'
  )
  expect(screen.getByLabelText('Evidence reference')).not.toHaveAttribute(
    'readonly'
  )
  await user.type(
    screen.getByLabelText('Evidence reference'),
    'new verified evidence'
  )
  await user.click(screen.getByRole('checkbox'))
  await user.click(
    screen.getByRole('button', { name: 'Confirm reconciliation' })
  )
  await screen.findByText('Reconciled')
  expect(api.post).toHaveBeenCalledTimes(2)
  expect(vi.mocked(api.post).mock.calls[1]?.[1]).toMatchObject({
    actual_quota: 130,
    evidence_reference: 'new verified evidence',
    actual_input_tokens: 10,
    actual_output_tokens: 5,
  })
})

test('an inconsistent applied projection without confirmed quota cannot open strict-token reconciliation', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: {
        ...fixture,
        settlement_status: 'applied_journal_pending',
        can_reconcile_usage: true,
        token_budget: {
          request_id: fixture.request_id,
          user_id: 2,
          token_id: 3,
          state: 'usage_unknown',
          fee_enabled: false,
          reserved: 30,
          actual_input: 10,
          actual_output: null,
        },
      },
    },
  })
  renderReview()
  await screen.findByText('Settlement applied; record finalization pending')
  expect(
    screen.queryByRole('button', { name: 'Advanced: manual reconciliation' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByLabelText('Confirmed quota (internal units)')
  ).not.toBeInTheDocument()
  expect(api.post).not.toHaveBeenCalled()
})
