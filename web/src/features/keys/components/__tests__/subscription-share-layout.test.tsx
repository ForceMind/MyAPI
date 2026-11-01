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
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import type {
  SubscriptionSharePool,
  SubscriptionShareView,
} from '../../subscription-share-api'
import { SubscriptionShareDialog } from '../subscription-share-dialog'

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn(), put: vi.fn() },
}))
vi.mock('sonner', () => ({ toast: { success: vi.fn() } }))

const activePool: SubscriptionSharePool = {
  channel_id: 1,
  pool_id: 7,
  snapshot_id: 70,
  revision: 4,
  channel_name: 'Personal Codex subscription connection with a long account label',
  plan_type: 'plus',
  window_slot: 'primary',
  window_type: 'weekly',
  window_seconds: 604800,
  reset_at: 1_797_003_000,
  observed_at: 1_796_999_970,
  max_age_seconds: 300,
  sample_age_seconds: 30,
  state: 'active',
  allocated_bps: 9000,
  share_bps: 1000,
  estimated_used_micro: 12_500_000,
  unattributed_micro: 1_250_000,
  pending_attempts: 2,
  unsampled_attempts: 1,
  uncertain_attempts: 0,
  approximate: true,
  baseline_time: 1_796_999_400,
}
const candidatePool: SubscriptionSharePool = {
  ...activePool,
  pool_id: 0,
  snapshot_id: 71,
  channel_name: 'Fresh candidate',
  state: 'candidate',
  allocated_bps: 0,
  share_bps: 0,
  estimated_used_micro: 0,
  unattributed_micro: 0,
  baseline_time: 0,
}
const originalAuth = useAuthStore.getState()
let client: QueryClient

function renderDialog() {
  render(
    <QueryClientProvider client={client}>
      <SubscriptionShareDialog tokenId={21} onClose={vi.fn()} />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  vi.resetAllMocks()
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  useAuthStore.setState((state) => ({
    ...state,
    auth: {
      ...state.auth,
      user: { id: 1, role: 100, username: 'root-fixture' },
      session: null,
    },
  }))
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: { token_id: 21, pools: [activePool, candidatePool] },
    },
  })
})

afterEach(() => {
  client.clear()
  useAuthStore.setState(originalAuth)
  vi.restoreAllMocks()
})

test('the full-height mobile task scrolls its content while title and actions stay outside the scroll region', async () => {
  renderDialog()
  const dialog = await screen.findByRole('dialog', {
    name: 'Subscription shares',
  })
  const task = within(dialog).getByRole('region', {
    name: 'Subscription share task',
  })
  const save = await within(dialog).findByRole('button', { name: 'Save share' })

  expect(dialog).toHaveClass('h-dvh', 'max-h-dvh', 'overflow-hidden')
  expect(task).toHaveClass('min-h-0', 'overflow-y-auto', 'overscroll-contain')
  expect(task).toHaveAttribute('tabindex', '0')
  expect(task).not.toContainElement(save)
  expect(task).not.toContainElement(
    within(dialog).getByRole('heading', { name: 'Subscription shares' })
  )
  const footer = save.closest('[data-slot="dialog-footer"]')
  expect(footer).toHaveClass('shrink-0')
  expect(footer).toContainElement(
    within(dialog).getAllByRole('button', { name: 'Close' })[0]
  )
  expect(save).toBeDisabled()
})

test('choosing a candidate presents its identity and unknown evidence before the editable draft', async () => {
  const user = userEvent.setup()
  renderDialog()
  const choice = await screen.findByRole('radio', { name: 'Fresh candidate' })
  expect(choice).not.toBeChecked()
  await user.click(choice)

  const selection = screen.getByRole('radiogroup', {
    name: 'Choose one subscription window',
  })
  const evidence = screen.getByRole('region', { name: 'Current window evidence' })
  const draft = screen.getByRole('region', { name: 'Edit share draft' })
  const candidateCard = within(evidence)
    .getByText('Fresh candidate')
    .closest('article') as HTMLElement
  expect(
    selection.compareDocumentPosition(evidence) &
      Node.DOCUMENT_POSITION_FOLLOWING
  ).toBeTruthy()
  expect(
    evidence.compareDocumentPosition(draft) & Node.DOCUMENT_POSITION_FOLLOWING
  ).toBeTruthy()
  expect(
    screen.getByRole('region', { name: 'Key identifier' })
  ).toHaveTextContent('#21')
  expect(
    within(evidence).getByText('Snapshot').nextElementSibling
  ).toHaveTextContent('#71')
  expect(
    within(evidence).getByText('Managed pool').nextElementSibling
  ).toHaveTextContent('Eligible window')
  expect(
    within(candidateCard).getByText('Estimated used').nextElementSibling
  ).toHaveTextContent('Unknown')
  expect(within(evidence).getByText(activePool.channel_name)).not.toBeVisible()
  expect(within(draft).getByLabelText('Share percentage')).toHaveValue('0')
  expect(within(draft).getByText('This creates a managed pool')).toBeVisible()
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
})

test('a read-only server refresh updates evidence while preserving the input, draft confirmation and expanded history', async () => {
  const user = userEvent.setup()
  renderDialog()
  await user.click(
    await screen.findByRole('radio', { name: activePool.channel_name })
  )
  const input = screen.getByLabelText('Share percentage')
  const confirmation = screen.getByRole('checkbox', {
    name: 'I understand this is an estimate with no verified error bound and may stop requests only after the share is exceeded.',
  })
  await user.clear(input)
  await user.type(input, '12.34')
  await user.click(confirmation)
  const disclosure = screen.getByText('Other window evidence')
  await user.click(disclosure)
  const history = disclosure.closest('details') as HTMLDetailsElement
  expect(history).toHaveAttribute('open')

  act(() => {
    client.setQueryData<SubscriptionShareView>(
      ['subscription-share', 1, 100, null, 21],
      {
        token_id: 21,
        pools: [{ ...activePool, pending_attempts: 8 }, candidatePool],
      }
    )
  })

  await waitFor(() => {
    const evidence = screen.getByRole('region', { name: 'Current window evidence' })
    const currentCard = within(evidence).getByText(activePool.channel_name).closest('article') as HTMLElement
    expect(within(currentCard).getByText('Attempts awaiting attribution').nextElementSibling).toHaveTextContent('8')
  })
  expect(screen.getByLabelText('Share percentage')).toBe(input)
  expect(input).toHaveValue('12.34')
  expect(confirmation).toBeChecked()
  expect(disclosure.closest('details')).toBe(history)
  expect(history).toHaveAttribute('open')
  expect(
    screen.getByRole('radio', { name: activePool.channel_name })
  ).toBeChecked()
  expect(api.post).not.toHaveBeenCalled()
  expect(api.put).not.toHaveBeenCalled()
})

test('keyboard selection keeps long window names accessible and exposes the matching draft', async () => {
  const user = userEvent.setup()
  renderDialog()
  const first = await screen.findByRole('radio', {
    name: activePool.channel_name,
  })
  await user.click(first)
  await user.keyboard('{ArrowDown}')

  const candidate = screen.getByRole('radio', { name: 'Fresh candidate' })
  expect(candidate).toBeChecked()
  expect(candidate).toHaveFocus()
  expect(first).toHaveAccessibleName(activePool.channel_name)
  expect(screen.getByLabelText('Share percentage')).toHaveValue('0')
  expect(screen.getByRole('button', { name: 'Save share' })).toBeDisabled()
})
