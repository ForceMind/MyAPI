import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { TokenBudgetDialog } from '../token-budget-dialog'

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn() },
}))
const initial = {
  policy: {
    token_id: 3,
    user_id: 2,
    enabled: false,
    account_threshold_enabled: false,
    account_min_remaining_bps: 2000,
    account_max_age_seconds: 300,
    fee_enabled: false,
    fee_limit_usd: '0',
    fee_used_usd: '0',
    fee_reserved_usd: '0',
    limit: 100,
    used: 0,
    reserved: 0,
    pending_request_id: '',
    revision: 0,
  },
  pending: null,
}
const originalUser = useAuthStore.getState().auth.user
const confirmedPolicy =
  'I confirm all running instances support this budget and I understand its request restrictions.'
const confirmedUsage =
  'I verified that the request ended and the actual token counts and frozen pricing are correct.'
beforeEach(() => {
  vi.resetAllMocks()
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'fixture-root', role: 100 })
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: initial },
  })
})
afterEach(() => {
  act(() => useAuthStore.getState().auth.setUser(originalUser))
})

function renderBudget(onClose = vi.fn()) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return {
    onClose,
    ...render(
      <QueryClientProvider client={client}>
        <TokenBudgetDialog tokenId={3} onClose={onClose} />
      </QueryClientProvider>
    ),
  }
}

test('requires confirmation and preserves used counters when enabling a token budget', async () => {
  const user = userEvent.setup()
  vi.mocked(api.put).mockResolvedValue({
    data: {
      success: true,
      data: {
        ...initial,
        policy: { ...initial.policy, enabled: true, revision: 1 },
      },
    },
  })
  renderBudget()
  const save = await screen.findByRole('button', { name: 'Save' })
  await user.click(save)
  expect(api.put).not.toHaveBeenCalled()
  await user.click(
    screen.getByRole('checkbox', { name: 'Enable strict Token budget' })
  )
  await user.click(screen.getByRole('checkbox', { name: confirmedPolicy }))
  await user.click(save)
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1))
  expect(vi.mocked(api.put).mock.calls[0]?.[1]).toMatchObject({
    expected_revision: 0,
    enabled: true,
    limit: 100,
    confirmed: true,
  })
  expect(vi.mocked(api.put).mock.calls[0]?.[1]).toMatchObject({
    id: expect.stringMatching(/^[a-f0-9]{64}$/),
  })
  await waitFor(() =>
    expect(
      screen.getByRole('checkbox', { name: confirmedPolicy })
    ).not.toBeChecked()
  )
  await user.click(screen.getByRole('button', { name: 'Save' }))
  expect(api.put).toHaveBeenCalledTimes(1)
})

test('retries the identical operation after an uncertain failure without allowing edits', async () => {
  const user = userEvent.setup()
  vi.mocked(api.put)
    .mockRejectedValueOnce(new Error('network interrupted'))
    .mockResolvedValueOnce({
      data: {
        success: true,
        data: { ...initial, policy: { ...initial.policy, revision: 1 } },
      },
    })
  renderBudget()
  await user.click(
    await screen.findByRole('checkbox', { name: confirmedPolicy })
  )
  await user.click(screen.getByRole('button', { name: 'Save' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'The outcome may already be saved.'
  )
  expect(screen.getByLabelText('Token total limit')).toHaveAttribute('readonly')
  expect(screen.getByRole('button', { name: 'Refresh' })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(2))
  expect(vi.mocked(api.put).mock.calls[1]?.[1]).toEqual(
    vi.mocked(api.put).mock.calls[0]?.[1]
  )
})

test('blocks duplicate submits and dismissal while a write is pending', async () => {
  const user = userEvent.setup()
  let resolveWrite: (value: unknown) => void = () => {}
  vi.mocked(api.put).mockImplementation(
    () =>
      new Promise((resolve) => {
        resolveWrite = resolve
      })
  )
  const view = renderBudget()
  await user.click(
    await screen.findByRole('checkbox', { name: confirmedPolicy })
  )
  await user.click(screen.getByRole('button', { name: 'Save' }))
  expect(
    await screen.findByRole('button', { name: 'Processing...' })
  ).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Close' })).toBeDisabled()
  await user.keyboard('{Escape}')
  expect(view.onClose).not.toHaveBeenCalled()
  expect(api.put).toHaveBeenCalledTimes(1)
  await act(async () =>
    resolveWrite({
      data: {
        success: true,
        data: { ...initial, policy: { ...initial.policy, revision: 1 } },
      },
    })
  )
  await waitFor(() =>
    expect(
      screen.getAllByRole('button', { name: 'Close' }).at(-1)
    ).not.toBeDisabled()
  )
  await user.keyboard('{Escape}')
  expect(view.onClose).toHaveBeenCalledTimes(1)
})

test('allows an owner to read a budget without exposing root mutations', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 2, username: 'fixture-owner', role: 1 })
  renderBudget()
  expect(
    await screen.findByText('Only Root can change budgets or reconcile usage.')
  ).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument()
  expect(api.put).not.toHaveBeenCalled()
  expect(api.post).not.toHaveBeenCalled()
})

test('requires actual input and output counts before reconciling a dispatched request', async () => {
  const user = userEvent.setup()
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: {
        policy: {
          ...initial.policy,
          enabled: true,
          reserved: 30,
          pending_request_id: 'request-fixture',
          revision: 4,
        },
        pending: {
          request_id: 'request-fixture',
          token_id: 3,
          user_id: 2,
          state: 'usage_unknown',
          reserved: 30,
          model_name: 'fixture',
          fee_enabled: false,
          fee_reserved_usd: '0',
        },
        review: {
          request_id: 'request-fixture',
          token_id: 3,
          user_id: 2,
          state: 'usage_unknown',
          reserved_quota: 100,
          actual_quota: null,
        },
      },
    },
  })
  vi.mocked(api.post).mockResolvedValue({
    data: {
      success: true,
      data: {
        ...initial,
        policy: { ...initial.policy, enabled: true, used: 15, revision: 5 },
      },
    },
  })
  renderBudget()
  await user.type(
    await screen.findByLabelText('Confirmed quota (internal units)'),
    '20'
  )
  await user.type(
    screen.getByLabelText('Evidence reference'),
    'verified terminal fixture'
  )
  await user.click(screen.getByRole('checkbox', { name: confirmedUsage }))
  await user.click(
    screen.getByRole('button', { name: 'Confirm reconciliation' })
  )
  expect(api.post).not.toHaveBeenCalled()
  await user.type(screen.getByLabelText('Confirmed input tokens'), '10')
  await user.type(screen.getByLabelText('Confirmed output tokens'), '5')
  await user.click(
    screen.getByRole('button', { name: 'Confirm reconciliation' })
  )
  await waitFor(() => expect(api.post).toHaveBeenCalledTimes(1))
  expect(vi.mocked(api.post).mock.calls[0]?.[1]).toEqual({
    request_id: 'request-fixture',
    action: 'reconcile',
    evidence_reference: 'verified terminal fixture',
    confirmed_reliable_evidence: true,
    actual_quota: 20,
    actual_input_tokens: 10,
    actual_output_tokens: 5,
  })
})

test('cancels a proven undispatched request without sending invented zero usage', async () => {
  const user = userEvent.setup()
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: {
        policy: {
          ...initial.policy,
          enabled: true,
          reserved: 30,
          pending_request_id: 'not-sent',
          revision: 2,
        },
        pending: {
          request_id: 'not-sent',
          token_id: 3,
          user_id: 2,
          state: 'prepared',
          reserved: 30,
          model_name: 'fixture',
          fee_enabled: false,
          fee_reserved_usd: '0',
        },
      },
    },
  })
  vi.mocked(api.post).mockResolvedValue({
    data: { success: true, data: initial },
  })
  renderBudget()
  await user.type(
    await screen.findByLabelText('Evidence reference'),
    'verified no dispatch'
  )
  await user.click(
    screen.getByRole('checkbox', {
      name: 'I confirm the request was never dispatched. Release only its reservation.',
    })
  )
  await user.click(
    screen.getByRole('button', { name: 'Cancel undispatched request' })
  )
  await waitFor(() => expect(api.post).toHaveBeenCalledTimes(1))
  expect(vi.mocked(api.post).mock.calls[0]?.[1]).toEqual({
    request_id: 'not-sent',
    action: 'cancel_not_sent',
    evidence_reference: 'verified no dispatch',
    confirmed_reliable_evidence: true,
  })
})

test('clears drafts when the signed-in user changes', async () => {
  const user = userEvent.setup()
  renderBudget()
  const limit = await screen.findByLabelText('Token total limit')
  await user.clear(limit)
  await user.type(limit, '999')
  act(() =>
    useAuthStore
      .getState()
      .auth.setUser({ id: 2, username: 'fixture-owner', role: 1 })
  )
  await waitFor(() =>
    expect(screen.queryByDisplayValue('999')).not.toBeInTheDocument()
  )
  expect(api.put).not.toHaveBeenCalled()
})

test('rejects a response for another key and does not render its edit form', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: { ...initial, policy: { ...initial.policy, token_id: 9 } },
    },
  })
  renderBudget()
  expect(await screen.findByRole('alert')).toHaveTextContent('Operation failed')
  expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument()
})

test('USD policy and recovery retain tiny decimal strings without Number conversion', async () => {
  const user = userEvent.setup()
  const tiny = '0.00000000000000000000000036'
  vi.mocked(api.put).mockResolvedValue({
    data: {
      success: true,
      data: {
        ...initial,
        policy: {
          ...initial.policy,
          fee_enabled: true,
          fee_limit_usd: tiny,
          revision: 1,
        },
      },
    },
  })
  const rendered = renderBudget()
  await user.click(
    await screen.findByRole('checkbox', { name: 'Enable USD fee budget' })
  )
  await user.clear(screen.getByLabelText('USD total limit'))
  await user.type(screen.getByLabelText('USD total limit'), tiny)
  await user.click(screen.getByRole('checkbox', { name: confirmedPolicy }))
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1))
  expect(vi.mocked(api.put).mock.calls[0]?.[1]).toMatchObject({
    enabled: false,
    fee: { enabled: true, limit_usd: tiny },
  })
  rendered.unmount()
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: {
        policy: {
          ...initial.policy,
          fee_enabled: true,
          fee_limit_usd: '1',
          fee_reserved_usd: '0.001',
          reserved: 30,
          pending_request_id: 'fee-review',
          revision: 4,
        },
        pending: {
          request_id: 'fee-review',
          token_id: 3,
          user_id: 2,
          state: 'usage_unknown',
          reserved: 30,
          model_name: 'fixture',
          fee_enabled: true,
          fee_reserved_usd: '0.001',
        },
        review: {
          request_id: 'fee-review',
          token_id: 3,
          user_id: 2,
          state: 'usage_unknown',
          reserved_quota: 100,
          actual_quota: null,
        },
      },
    },
  })
  vi.mocked(api.post).mockResolvedValue({
    data: {
      success: true,
      data: {
        ...initial,
        policy: {
          ...initial.policy,
          fee_enabled: true,
          fee_limit_usd: '1',
          fee_used_usd: tiny,
          used: 15,
          revision: 5,
        },
      },
    },
  })
  renderBudget()
  await user.type(
    await screen.findByLabelText('Confirmed quota (internal units)'),
    '20'
  )
  await user.type(screen.getByLabelText('Confirmed input tokens'), '10')
  await user.type(screen.getByLabelText('Confirmed output tokens'), '5')
  await user.type(
    screen.getByLabelText('Evidence reference'),
    'verified exact USD evidence'
  )
  await user.click(
    screen.getByRole('checkbox', {
      name: 'I verified that the request ended and the actual token counts, USD cost and frozen pricing are correct.',
    })
  )
  await user.click(
    screen.getByRole('button', { name: 'Confirm reconciliation' })
  )
  expect(api.post).not.toHaveBeenCalled()
  await user.type(screen.getByLabelText('Confirmed API usage cost (USD)'), tiny)
  await user.click(
    screen.getByRole('button', { name: 'Confirm reconciliation' })
  )
  await waitFor(() => expect(api.post).toHaveBeenCalledTimes(1))
  expect(vi.mocked(api.post).mock.calls[0]?.[1]).toMatchObject({
    actual_fee_usd: tiny,
    actual_input_tokens: 10,
    actual_output_tokens: 5,
  })
})

test('saves an account safety threshold in exact basis points and rejects mixed strict modes', async () => {
  const user = userEvent.setup()
  vi.mocked(api.put).mockResolvedValue({
    data: {
      success: true,
      data: {
        ...initial,
        policy: {
          ...initial.policy,
          account_threshold_enabled: true,
          account_min_remaining_bps: 2001,
          account_max_age_seconds: 120,
          revision: 1,
        },
      },
    },
  })
  renderBudget()
  await user.click(
    await screen.findByRole('checkbox', {
      name: 'Enable account safety threshold',
    })
  )
  const percent = screen.getByLabelText('Minimum remaining percentage')
  await user.clear(percent)
  await user.type(percent, '20.001')
  await user.click(screen.getByRole('checkbox', { name: confirmedPolicy }))
  await user.click(screen.getByRole('button', { name: 'Save' }))
  expect(api.put).not.toHaveBeenCalled()
  await user.clear(percent)
  await user.type(percent, '20.01')
  await user.clear(screen.getByLabelText('Maximum observation age (seconds)'))
  await user.type(
    screen.getByLabelText('Maximum observation age (seconds)'),
    '120'
  )
  await user.click(
    screen.getByRole('checkbox', { name: 'Enable strict Token budget' })
  )
  await user.click(screen.getByRole('button', { name: 'Save' }))
  expect(api.put).not.toHaveBeenCalled()
  expect(
    screen.getByText(
      'Account thresholds cannot be combined with Token or USD budgets on this key.'
    )
  ).toBeVisible()
  await user.click(
    screen.getByRole('checkbox', { name: 'Enable strict Token budget' })
  )
  await waitFor(() =>
    expect(
      screen.queryByText(
        'Account thresholds cannot be combined with Token or USD budgets on this key.'
      )
    ).not.toBeInTheDocument()
  )
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1))
  expect(vi.mocked(api.put).mock.calls[0]?.[1]).toMatchObject({
    enabled: false,
    fee: { enabled: false },
    account_threshold: {
      enabled: true,
      minimum_remaining_bps: 2001,
      max_age_seconds: 120,
    },
  })
  expect(await screen.findByText('20.01%')).toBeVisible()
})

test('shows account threshold and freshness to the owner without editable controls', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 2, username: 'fixture-owner', role: 1 })
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: {
        ...initial,
        policy: {
          ...initial.policy,
          account_threshold_enabled: true,
          account_min_remaining_bps: 2000,
          account_max_age_seconds: 300,
        },
      },
    },
  })
  renderBudget()
  expect(await screen.findByText('20%')).toBeVisible()
  expect(screen.getByText('300')).toBeVisible()
  expect(
    screen.queryByRole('checkbox', { name: 'Enable account safety threshold' })
  ).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument()
})

test('request details explain native Chat limits and conservative bounds before enabling a budget', async () => {
  renderBudget()
  await screen.findByRole('button', { name: 'Save' })
  await userEvent.click(screen.getByText('Supported request fields'))
  expect(
    screen.getByText(
      'Responses requires explicit max_output_tokens. Chat requires explicit max_completion_tokens from 1 to 128,000 and n omitted or 1; streaming requires stream_options.include_usage=true.'
    )
  ).toBeVisible()
  expect(
    screen.getByText(
      'Chat reserves 1,050,000 total tokens for input and completion, including reasoning. This is a conservative bound, not measured usage or a tokenizer estimate. Even a small request can fail if the remaining budget cannot cover this bound.'
    )
  ).toBeVisible()
  expect(
    screen.getByText(
      'Chat USD budgets require service_tier=default, exact published short/long prices frozen at dispatch, and explicit cache-read and cache-write usage counters, including zero. Missing evidence retains the reservation for review.'
    )
  ).toBeVisible()
  expect(api.put).not.toHaveBeenCalled()
})
