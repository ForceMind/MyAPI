import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api, getStatus } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { ApiKeysPrimaryButtons } from '../api-keys-primary-buttons'
import { ApiKeysProvider } from '../api-keys-provider'

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn() },
  getStatus: vi.fn(),
}))
const original = useAuthStore.getState().auth.user
let client: QueryClient
beforeEach(() => {
  vi.resetAllMocks()
  localStorage.clear()
  useAuthStore.getState().auth.setUser({ id: 2, role: 1, username: 'owner' })
  vi.mocked(getStatus).mockResolvedValue({
    user_funding_mode: 'disabled',
    user_funding_capabilities: { mode: 'disabled', ready: true, epoch: 1 },
  })
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: {
        user_id: 2,
        no_balance: true,
        revision: 1,
        legacy_remaining_quota: 0,
      },
    },
  })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
})
afterEach(() => {
  client.clear()
  act(() => useAuthStore.getState().auth.setUser(original))
  localStorage.clear()
})
function renderButtons() {
  render(
    <QueryClientProvider client={client}>
      <ApiKeysProvider>
        <ApiKeysPrimaryButtons />
      </ApiKeysProvider>
    </QueryClientProvider>
  )
}
test('owner opens only their own existing policy with no write controls or automatic requests', async () => {
  const user = userEvent.setup()
  renderButtons()
  expect(api.get).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'My usage policy' }))
  expect(
    await screen.findByText('Only Root can change this policy.')
  ).toBeVisible()
  expect(api.get).toHaveBeenCalledExactlyOnceWith(
    '/api/user/2/usage-policy',
    expect.objectContaining({ signal: expect.any(AbortSignal) })
  )
  expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument()
  expect(api.put).not.toHaveBeenCalled()
  expect(api.post).not.toHaveBeenCalled()
})
test.each([
  { id: 3, role: 1, username: 'next' },
  { id: 2, role: 100, username: 'new-role' },
])(
  'identity change to $id/$role closes the dialog without fetching the next identity',
  async (nextActor) => {
    const user = userEvent.setup()
    renderButtons()
    await user.click(screen.getByRole('button', { name: 'My usage policy' }))
    await screen.findByRole('dialog')
    act(() => useAuthStore.getState().auth.setUser(nextActor))
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
    act(() =>
      useAuthStore
        .getState()
        .auth.setUser({ id: 2, role: 1, username: 'owner' })
    )
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(api.get).toHaveBeenCalledTimes(1)
  }
)
test('logout removes the personal entry', () => {
  renderButtons()
  act(() => useAuthStore.getState().auth.setUser(null))
  expect(
    screen.queryByRole('button', { name: 'My usage policy' })
  ).not.toBeInTheDocument()
})

test('closing and reopening the owner entry never submits a policy or reveals a Key', async () => {
  const user = userEvent.setup()
  renderButtons()
  for (const dismiss of ['close', 'escape']) {
    await user.click(screen.getByRole('button', { name: 'My usage policy' }))
    await screen.findByText('Only Root can change this policy.')
    if (dismiss === 'close') {
      await user.click(screen.getAllByRole('button', { name: 'Close' })[0])
    } else await user.keyboard('{Escape}')
    await waitFor(() =>
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    )
  }
  expect(api.put).not.toHaveBeenCalled()
  expect(api.post).not.toHaveBeenCalled()
  expect(
    vi
      .mocked(api.get)
      .mock.calls.every(([url]) => url === '/api/user/2/usage-policy')
  ).toBe(true)
})
