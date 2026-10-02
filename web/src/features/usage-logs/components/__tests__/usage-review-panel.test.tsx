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

test('requires evidence and confirmation before sending one exact recovery request', async () => {
  const user = userEvent.setup()
  vi.mocked(api.post).mockResolvedValue({
    data: {
      success: true,
      data: { ...fixture, state: 'settled', actual_quota: 120 },
    },
  })
  renderReview()
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
