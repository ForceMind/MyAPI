import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api, getStatus } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { UserQuotaCell } from '../user-quota-cell'
import { UserUsagePolicyDialog } from '../user-usage-policy-dialog'

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn() },
  getStatus: vi.fn(),
}))
const original = useAuthStore.getState().auth.user
const initial = {
  user_id: 2,
  no_balance: false,
  revision: 0,
  legacy_remaining_quota: 0,
}
const confirmation =
  'I confirm all running instances support this policy and I understand the supported request paths.'
beforeEach(() => {
  vi.resetAllMocks()
  localStorage.clear()
  vi.mocked(getStatus).mockResolvedValue({
    user_funding_mode: 'disabled',
    user_funding_capabilities: { mode: 'disabled', ready: true, epoch: 1 },
  })
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, role: 100, username: 'root-fixture' })
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: initial },
  })
})
afterEach(() => {
  act(() => useAuthStore.getState().auth.setUser(original))
})
function renderPolicy() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <UserUsagePolicyDialog userId={2} onClose={vi.fn()} />
    </QueryClientProvider>
  )
}
test('requires explicit confirmation and preserves zero allowance when enabling self-use', async () => {
  const user = userEvent.setup()
  vi.mocked(api.put).mockResolvedValue({
    data: {
      success: true,
      data: { ...initial, no_balance: true, revision: 1 },
    },
  })
  renderPolicy()
  const save = await screen.findByRole('button', { name: 'Save' })
  await user.click(
    screen.getByRole('checkbox', {
      name: 'Use Key limits without a user wallet',
    })
  )
  await user.click(save)
  expect(api.put).not.toHaveBeenCalled()
  expect(screen.getByText('Required')).toBeVisible()
  await user.click(screen.getByRole('checkbox', { name: confirmation }))
  await user.click(save)
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1))
  expect(vi.mocked(api.put).mock.calls[0]?.[1]).toEqual({
    id: expect.stringMatching(/^[a-f0-9]{64}$/),
    expected_revision: 0,
    no_balance: true,
    confirmed: true,
  })
  await waitFor(() =>
    expect(
      screen.getByRole('checkbox', { name: confirmation })
    ).not.toBeChecked()
  )
  expect(screen.getByText('0')).toBeVisible()
})
test('uncertain write locks edits and retries the same audited operation', async () => {
  const user = userEvent.setup()
  vi.mocked(api.put)
    .mockRejectedValueOnce(new Error('connection lost'))
    .mockResolvedValueOnce({
      data: { success: true, data: { ...initial, revision: 1 } },
    })
  renderPolicy()
  await user.click(await screen.findByRole('checkbox', { name: confirmation }))
  await user.click(screen.getByRole('button', { name: 'Save' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'The outcome may already be saved.'
  )
  expect(
    screen.getByRole('checkbox', {
      name: 'Use Key limits without a user wallet',
    })
  ).toHaveAttribute('aria-disabled', 'true')
  await user.click(
    screen.getByRole('checkbox', {
      name: 'Use Key limits without a user wallet',
    })
  )
  expect(
    screen.getByRole('checkbox', {
      name: 'Use Key limits without a user wallet',
    })
  ).not.toBeChecked()
  expect(screen.getByRole('button', { name: 'Refresh' })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(2))
  expect(vi.mocked(api.put).mock.calls[1]?.[1]).toEqual(
    vi.mocked(api.put).mock.calls[0]?.[1]
  )
})
test('owner can read the policy but has no mutation controls', async () => {
  useAuthStore
    .getState()
    .auth.setUser({ id: 2, role: 1, username: 'owner-fixture' })
  renderPolicy()
  expect(
    await screen.findByText('Only Root can change this policy.')
  ).toBeVisible()
  expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument()
  expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
})
test('a mismatched user response is rejected rather than exposing its controls', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: { ...initial, user_id: 3 } },
  })
  renderPolicy()
  expect(await screen.findByRole('alert')).toHaveTextContent('Operation failed')
  expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument()
})
test('zero stored allowance is not shown as exhausted when the self-use cap is disabled', () => {
  const view = render(<UserQuotaCell used={0} remaining={0} noBalance />)
  expect(screen.getByText('No user allowance cap')).toBeVisible()
  expect(screen.queryByText('No Quota')).not.toBeInTheDocument()
  view.rerender(<UserQuotaCell used={0} remaining={0} policyPending />)
  expect(screen.getByText('Unavailable')).toBeVisible()
  expect(screen.queryByText('No user allowance cap')).not.toBeInTheDocument()
})

const conditionsMet =
  'No-wallet configuration conditions are met for supported requests.'
const inactivePreference =
  'The saved no-wallet preference is inactive while commercial funding is enabled or retiring.'

test('saved preference stays separate from the unsaved draft and confirmed funding mode', async () => {
  const user = userEvent.setup()
  renderPolicy()
  expect(
    await screen.findByText(
      'The saved preference uses the stored user allowance.'
    )
  ).toBeVisible()
  await user.click(
    screen.getByRole('checkbox', {
      name: 'Use Key limits without a user wallet',
    })
  )
  expect(
    screen.getByText('The saved preference uses the stored user allowance.')
  ).toBeVisible()
  expect(screen.queryByText(conditionsMet)).not.toBeInTheDocument()
  expect(api.put).not.toHaveBeenCalled()
})

test.each(['enabled', 'retirement', 'disabled'])(
  'saved no-wallet preference reports the confirmed server mode %s',
  async (mode) => {
    vi.mocked(getStatus).mockResolvedValue({
      user_funding_mode: mode,
      user_funding_capabilities: { mode, ready: true, epoch: 2 },
    })
    vi.mocked(api.get).mockResolvedValue({
      data: {
        success: true,
        data: { ...initial, no_balance: true, revision: 1 },
      },
    })
    renderPolicy()
    expect(
      await screen.findByText(
        mode === 'disabled' ? conditionsMet : inactivePreference
      )
    ).toBeVisible()
    expect(api.put).not.toHaveBeenCalled()
  }
)

test.each(['policy', 'status'])(
  'failed %s refresh removes configuration claims even when old data remains cached',
  async (failure) => {
    const user = userEvent.setup()
    vi.mocked(api.get).mockResolvedValue({
      data: {
        success: true,
        data: { ...initial, no_balance: true, revision: 1 },
      },
    })
    renderPolicy()
    expect(await screen.findByText(conditionsMet)).toBeVisible()
    if (failure === 'policy') {
      vi.mocked(api.get).mockRejectedValue(new Error('private policy detail'))
    } else {
      vi.mocked(getStatus).mockRejectedValue(new Error('private status detail'))
    }
    await user.click(screen.getByRole('button', { name: 'Refresh' }))
    expect(
      await screen.findByText('Configuration could not be confirmed.')
    ).toBeVisible()
    expect(screen.queryByText(conditionsMet)).not.toBeInTheDocument()
    expect(screen.queryByText(/private .* detail/)).not.toBeInTheDocument()
    expect(getStatus).toHaveBeenCalledTimes(2)
    expect(api.get).toHaveBeenCalledTimes(2)
  }
)

test('cached disabled placeholder is not configuration evidence while the server reply is pending', async () => {
  localStorage.setItem(
    'status',
    JSON.stringify({
      user_funding_mode: 'disabled',
      user_funding_capabilities: { mode: 'disabled', ready: true, epoch: 1 },
    })
  )
  let resolve!: (value: Record<string, unknown>) => void
  vi.mocked(getStatus).mockReturnValue(
    new Promise((accept) => {
      resolve = accept
    })
  )
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: { ...initial, no_balance: true, revision: 1 },
    },
  })
  renderPolicy()
  await screen.findByRole('checkbox', { name: confirmation })
  expect(
    screen.getByText('Configuration could not be confirmed.')
  ).toBeVisible()
  expect(screen.queryByText(conditionsMet)).not.toBeInTheDocument()
  await act(async () =>
    resolve({
      user_funding_mode: 'enabled',
      user_funding_capabilities: { mode: 'enabled', ready: true, epoch: 2 },
    })
  )
  expect(await screen.findByText(inactivePreference)).toBeVisible()
})

test('Root can save an inactive preference without changing the global funding mode', async () => {
  const user = userEvent.setup()
  vi.mocked(getStatus).mockResolvedValue({
    user_funding_mode: 'enabled',
    user_funding_capabilities: { mode: 'enabled', ready: true, epoch: 2 },
  })
  vi.mocked(api.put).mockResolvedValue({
    data: {
      success: true,
      data: { ...initial, no_balance: true, revision: 1 },
    },
  })
  renderPolicy()
  await screen.findByText('Commercial funding enabled')
  await user.click(
    screen.getByRole('checkbox', {
      name: 'Use Key limits without a user wallet',
    })
  )
  await user.click(screen.getByRole('checkbox', { name: confirmation }))
  await user.click(screen.getByRole('button', { name: 'Save' }))
  expect(await screen.findByText(inactivePreference)).toBeVisible()
  expect(api.put).toHaveBeenCalledExactlyOnceWith(
    '/api/user/2/usage-policy',
    expect.objectContaining({
      expected_revision: 0,
      no_balance: true,
      confirmed: true,
    }),
    expect.anything()
  )
  expect(screen.getByText('Commercial funding enabled')).toBeVisible()
})
