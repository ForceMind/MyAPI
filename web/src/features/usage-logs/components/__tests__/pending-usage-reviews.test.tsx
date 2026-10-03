import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { PendingUsageReviews } from '../pending-usage-reviews'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), post: vi.fn() } }))
const originalUser = useAuthStore.getState().auth.user
const fixture = {
  request_id: 'pending-without-log',
  user_id: 2,
  token_id: 3,
  state: 'open',
  reserved_quota: 100,
  actual_quota: null,
  text_dispatch_pending: true,
  can_recover_text_dispatch: true,
}
beforeEach(() => {
  vi.resetAllMocks()
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'fixture-root', role: 100 })
  vi.mocked(api.get).mockImplementation(async (url) => ({
    data: {
      success: true,
      data:
        url === '/api/usage-reviews/pending'
          ? { items: [fixture], next_after: '' }
          : fixture,
    },
  }))
})
afterEach(() => {
  act(() => useAuthStore.getState().auth.setUser(originalUser))
})
function renderPending() {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <PendingUsageReviews />
    </QueryClientProvider>
  )
}

test('opens an unlogged request for review and closing the dialog never writes', async () => {
  const user = userEvent.setup()
  renderPending()
  expect(api.get).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Pending requests' }))
  await user.click(
    await screen.findByRole('button', {
      name: 'Request ID: pending-without-log',
    })
  )
  expect(
    await screen.findByLabelText('Confirmed quota (internal units)')
  ).toBeInTheDocument()
  await user.keyboard('{Escape}')
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
  expect(api.post).not.toHaveBeenCalled()
})

test('advances an empty bounded scan page and resets the cursor when changing writers', async () => {
  const user = userEvent.setup()
  vi.mocked(api.get).mockResolvedValueOnce({
    data: { success: true, data: { items: [], next_after: '100' } },
  })
  renderPending()
  await user.click(screen.getByRole('button', { name: 'Pending requests' }))
  expect(
    await screen.findByText('No requests on this page')
  ).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Next' }))
  await screen.findByRole('button', { name: 'Request ID: pending-without-log' })
  expect(api.get).toHaveBeenCalledWith(
    '/api/usage-reviews/pending',
    expect.objectContaining({
      params: { writer: 'authoritative', after: '100' },
    })
  )
  await user.click(screen.getByRole('button', { name: 'Legacy reservations' }))
  await waitFor(() =>
    expect(api.get).toHaveBeenCalledWith(
      '/api/usage-reviews/pending',
      expect.objectContaining({ params: { writer: 'legacy', after: '0' } })
    )
  )
})

test('removes root data and controls when the authenticated role changes', async () => {
  const user = userEvent.setup()
  renderPending()
  await user.click(screen.getByRole('button', { name: 'Pending requests' }))
  await screen.findByRole('button', { name: 'Request ID: pending-without-log' })
  act(() =>
    useAuthStore.getState().auth.setUser({ id: 2, username: 'owner', role: 1 })
  )
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Pending requests' })
  ).not.toBeInTheDocument()
  expect(api.post).not.toHaveBeenCalled()
})
