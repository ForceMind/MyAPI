/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { apiKeySchema } from '../../types'
import { SubscriptionShareDialog } from '../subscription-share-dialog'

vi.mock('@/lib/api', () => ({api: {get: vi.fn(), put: vi.fn(), post: vi.fn()}}))
vi.mock('sonner', () => ({toast: {success: vi.fn()}}))
const key = apiKeySchema.parse({id: 21, name: 'Desktop application', key: 'masked', status: 1, remain_quota: 100, used_quota: 0, unlimited_quota: false, expired_time: -1, created_time: 1, accessed_time: 1, model_limits_enabled: false})
const pool = {pool_id: 7, snapshot_id: 70, revision: 4, channel_id: 9, channel_name: 'Subscription account A', plan_type: 'plus', window_slot: 'primary', window_type: 'five_hour', window_seconds: 18000, reset_at: 1797003600, observed_at: 1797000000, max_age_seconds: 300, sample_age_seconds: 30, state: 'active', allocated_bps: 7000, share_bps: 2000, estimated_used_micro: 5000000, unattributed_micro: 1000000, pending_attempts: 2, unsampled_attempts: 1, approximate: true as const, baseline_time: 1796999400}
const originalAuth = useAuthStore.getState()
let client: QueryClient
const response = (overrides = {}) => ({data: {success: true, data: {token_id: 21, pools: [{...pool, ...overrides}]}}})

function renderEditor() {
  const onClose = vi.fn()
  render(<QueryClientProvider client={client}><SubscriptionShareDialog tokenId={21} personalKey={key} onClose={onClose} /></QueryClientProvider>)
  return onClose
}
async function selectWindow(user: ReturnType<typeof userEvent.setup>) {
  await user.selectOptions(await screen.findByLabelText('Subscription account'), '9')
  await user.selectOptions(screen.getByLabelText('Choose one subscription window'), '7:70')
}
async function confirmApproximation(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('checkbox', {name: 'I understand this is an estimate with no verified error bound and may stop requests only after the share is exceeded.'}))
}
beforeEach(() => {
  vi.resetAllMocks()
  client = new QueryClient({defaultOptions: {queries: {retry: false}, mutations: {retry: false}}})
  useAuthStore.setState((state) => ({...state, auth: {...state.auth, user: {id: 1, role: 100, username: 'owner'}, session: null}}))
  vi.mocked(api.get).mockResolvedValue(response())
  vi.mocked(api.put).mockResolvedValue(response())
})
afterEach(() => {client.clear(); useAuthStore.setState(originalAuth)})

test('explicitly selecting an account and window previews 20 plus 50 as 70 and saves the existing Key', async () => {
  const user = userEvent.setup()
  const onClose = renderEditor()
  const save = await screen.findByRole('button', {name: 'Save changes'})
  expect(save).toBeDisabled()
  await selectWindow(user)
  const preview = screen.getByRole('complementary', {name: 'Allocation preview'})
  expect(within(preview).getByText('70%')).toBeVisible()
  expect(within(preview).getByText('25%')).toBeVisible()
  expect(within(preview).getByText('Equivalent to 5 percentage points of the total window.')).toBeVisible()
  await confirmApproximation(user)
  await user.click(screen.getByLabelText('Share of total window'))
  await user.keyboard('{Enter}')
  await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1))
  expect(api.put).toHaveBeenCalledWith('/api/token/21/subscription-share', expect.objectContaining({pool_id: 7, snapshot_id: 70, expected_revision: 4, share_bps: 2000, confirmed_approximate: true}), expect.any(Object))
  expect(api.post).not.toHaveBeenCalled()
})

test('60 plus other Keys 50 shows an invalid 110 allocation and blocks saving', async () => {
  const user = userEvent.setup()
  renderEditor()
  await selectWindow(user)
  const input = screen.getByLabelText('Share of total window')
  await user.clear(input)
  await user.type(input, '60')
  await confirmApproximation(user)
  expect(input).toHaveAttribute('aria-invalid', 'true')
  expect(screen.getByText('This share would make the pool allocation exceed 100%.')).toBeVisible()
  expect(screen.getByRole('complementary')).toHaveTextContent('110%')
  expect(screen.getByRole('button', {name: 'Save changes'})).toBeDisabled()
  expect(api.put).not.toHaveBeenCalled()
})

test('zero share pauses new requests while preserving current-window usage evidence', async () => {
  const user = userEvent.setup()
  renderEditor()
  await selectWindow(user)
  const input = screen.getByLabelText('Share of total window')
  await user.clear(input)
  await user.type(input, '0')
  expect(screen.getByText('0% pauses new requests and keeps existing usage.')).toBeVisible()
  expect(screen.getByText('Equivalent to 5 percentage points of the total window.')).toBeVisible()
  await confirmApproximation(user)
  await user.click(screen.getByRole('button', {name: 'Save changes'}))
  await waitFor(() => expect(api.put).toHaveBeenCalledWith('/api/token/21/subscription-share', expect.objectContaining({share_bps: 0}), expect.any(Object)))
})

test('100 percent is allowed when no other Key is allocated and does not promise reserved capacity', async () => {
  vi.mocked(api.get).mockResolvedValue(response({allocated_bps: 2000}))
  const user = userEvent.setup()
  renderEditor()
  await selectWindow(user)
  const input = screen.getByLabelText('Share of total window')
  await user.clear(input)
  await user.type(input, '100')
  expect(screen.getByText('100% allocation is allowed. Shares do not reserve upstream capacity.')).toBeVisible()
  await confirmApproximation(user)
  await user.click(screen.getByRole('button', {name: 'Save changes'}))
  await waitFor(() => expect(api.put).toHaveBeenCalledWith('/api/token/21/subscription-share', expect.objectContaining({share_bps: 10000}), expect.any(Object)))
})

test('changing the percentage retains five percentage points of usage and updates its share-relative ratio', async () => {
  const user = userEvent.setup()
  renderEditor()
  await selectWindow(user)
  const input = screen.getByLabelText('Share of total window')
  await user.clear(input)
  await user.type(input, '25')
  const preview = screen.getByRole('complementary')
  expect(within(preview).getByText('20%')).toBeVisible()
  expect(within(preview).getByText('Equivalent to 5 percentage points of the total window.')).toBeVisible()
  expect(api.put).not.toHaveBeenCalled()
})

test('cancelling an edited share returns to the Key list without writing', async () => {
  const user = userEvent.setup()
  const close = renderEditor()
  await selectWindow(user)
  await user.clear(screen.getByLabelText('Share of total window'))
  await user.type(screen.getByLabelText('Share of total window'), '30')
  await user.click(screen.getByRole('button', {name: 'Cancel', exact: true}))
  expect(close).toHaveBeenCalledOnce()
  expect(api.put).not.toHaveBeenCalled()
})

test('an unknown save result locks the editor until a deliberate reload', async () => {
  vi.mocked(api.put).mockRejectedValue(new Error('connection closed'))
  const user = userEvent.setup()
  renderEditor()
  await selectWindow(user)
  await confirmApproximation(user)
  await user.click(screen.getByRole('button', {name: 'Save changes'}))
  expect(await screen.findByText('The save result is unknown or out of date')).toBeVisible()
  expect(screen.getByRole('button', {name: 'Save changes'})).toBeDisabled()
  await user.click(screen.getByRole('button', {name: 'Reload current state'}))
  await waitFor(() => expect(api.get).toHaveBeenCalledTimes(2))
  expect(api.put).toHaveBeenCalledTimes(1)
  expect(screen.getByRole('button', {name: 'Save changes'})).toBeDisabled()
})

test('a missing trustworthy baseline displays unknown estimated usage rather than zero', async () => {
  vi.mocked(api.get).mockResolvedValue(response({baseline_time: 0, estimated_used_micro: 0, state: 'baseline_pending'}))
  const user = userEvent.setup()
  renderEditor()
  await selectWindow(user)
  expect(screen.getByText('Waiting for a trustworthy usage sample.')).toBeVisible()
  expect(screen.getByRole('complementary')).not.toHaveTextContent('Equivalent to 0 percentage points')
})

test('advanced controls retain the existing pool closing and recovery interface', async () => {
  const user = userEvent.setup()
  renderEditor()
  await selectWindow(user)
  await user.click(screen.getByRole('button', {name: 'Advanced window controls'}))
  const dialog = await screen.findByRole('dialog', {name: 'Subscription shares'})
  expect(within(dialog).getByRole('button', {name: 'Close pool'})).toBeVisible()
})
