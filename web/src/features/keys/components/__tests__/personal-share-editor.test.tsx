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

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn() },
}))
vi.mock('sonner', () => ({ toast: { success: vi.fn() } }))
const key = apiKeySchema.parse({
  id: 21,
  name: 'Desktop application',
  key: 'masked',
  status: 1,
  remain_quota: 100,
  used_quota: 0,
  unlimited_quota: false,
  expired_time: -1,
  created_time: 1,
  accessed_time: 1,
  model_limits_enabled: false,
})
const pool = {
  pool_id: 7,
  snapshot_id: 70,
  revision: 4,
  channel_id: 9,
  channel_name: 'Subscription account A',
  plan_type: 'plus',
  window_slot: 'primary',
  window_type: 'five_hour',
  window_seconds: 18000,
  reset_at: 1797003600,
  observed_at: 1797000000,
  max_age_seconds: 300,
  sample_age_seconds: 30,
  state: 'active',
  allocated_bps: 7000,
  share_bps: 2000,
  estimated_used_micro: 5000000,
  unattributed_micro: 1000000,
  pending_attempts: 2,
  unsampled_attempts: 1,
  approximate: true as const,
  baseline_time: 1796999400,
}
const originalAuth = useAuthStore.getState()
let client: QueryClient
const response = (overrides = {}) => ({
  data: {
    success: true,
    data: { token_id: 21, pools: [{ ...pool, ...overrides }] },
  },
})

function renderEditor() {
  const onClose = vi.fn()
  render(
    <QueryClientProvider client={client}>
      <SubscriptionShareDialog
        tokenId={21}
        personalKey={key}
        onClose={onClose}
      />
    </QueryClientProvider>
  )
  return onClose
}
async function selectWindow(user: ReturnType<typeof userEvent.setup>) {
  await user.selectOptions(
    await screen.findByLabelText('Subscription account'),
    '9'
  )
  await user.selectOptions(
    screen.getByLabelText('Choose one subscription window'),
    '7:70'
  )
}
async function confirmApproximation(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    screen.getByRole('checkbox', {
      name: 'I understand this is an estimate with no verified error bound and may stop requests only after the share is exceeded.',
    })
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
      user: { id: 1, role: 100, username: 'owner' },
      session: null,
    },
  }))
  vi.mocked(api.get).mockResolvedValue(response())
  vi.mocked(api.put).mockResolvedValue(response())
})
afterEach(() => {
  client.clear()
  useAuthStore.setState(originalAuth)
  vi.restoreAllMocks()
})

test('explicitly selecting an account and window previews 20 plus 50 as 70 and saves the existing Key', async () => {
  const user = userEvent.setup()
  const onClose = renderEditor()
  const save = await screen.findByRole('button', { name: 'Save changes' })
  expect(save).toBeDisabled()
  await selectWindow(user)
  const preview = screen.getByRole('complementary', {
    name: 'Allocation preview',
  })
  const allocationLabel = within(preview).getByText('Total allocated', {
    selector: 'dt:not(details dt)',
  })
  expect(allocationLabel.nextElementSibling).toHaveTextContent(/^70%$/)
  expect(within(preview).getByText('25%')).toBeVisible()
  expect(
    within(preview).getByText(
      'Equivalent to 5 percentage points of the total window.'
    )
  ).toBeVisible()
  await user.click(screen.getByLabelText('Share of total window'))
  await user.keyboard('{Enter}')
  expect(api.put).not.toHaveBeenCalled()
  await confirmApproximation(user)
  await user.click(screen.getByLabelText('Share of total window'))
  await user.keyboard('{Enter}')
  await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1))
  expect(api.put).toHaveBeenCalledOnce()
  expect(api.put).toHaveBeenCalledWith(
    '/api/token/21/subscription-share',
    expect.objectContaining({
      pool_id: 7,
      snapshot_id: 70,
      expected_revision: 4,
      share_bps: 2000,
      confirmed_approximate: true,
    }),
    expect.any(Object)
  )
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
  expect(
    screen.getByText('This share would make the pool allocation exceed 100%.')
  ).toBeVisible()
  expect(screen.getByRole('complementary')).toHaveTextContent('110%')
  expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
  expect(api.put).not.toHaveBeenCalled()
})

test('zero share pauses new requests while preserving current-window usage evidence', async () => {
  const user = userEvent.setup()
  renderEditor()
  await selectWindow(user)
  const input = screen.getByLabelText('Share of total window')
  await user.clear(input)
  await user.type(input, '0')
  expect(
    screen.getByText('0% pauses new requests and keeps existing usage.')
  ).toBeVisible()
  expect(
    screen.getByText('Equivalent to 5 percentage points of the total window.')
  ).toBeVisible()
  await confirmApproximation(user)
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await waitFor(() =>
    expect(api.put).toHaveBeenCalledWith(
      '/api/token/21/subscription-share',
      expect.objectContaining({ share_bps: 0 }),
      expect.any(Object)
    )
  )
})

test('100 percent is allowed when no other Key is allocated and does not promise reserved capacity', async () => {
  vi.mocked(api.get).mockResolvedValue(response({ allocated_bps: 2000 }))
  const user = userEvent.setup()
  renderEditor()
  await selectWindow(user)
  const input = screen.getByLabelText('Share of total window')
  await user.clear(input)
  await user.type(input, '100')
  expect(
    screen.getByText(
      '100% allocation is allowed. Shares do not reserve upstream capacity.'
    )
  ).toBeVisible()
  await confirmApproximation(user)
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await waitFor(() =>
    expect(api.put).toHaveBeenCalledWith(
      '/api/token/21/subscription-share',
      expect.objectContaining({ share_bps: 10000 }),
      expect.any(Object)
    )
  )
})

test('changing the percentage retains five percentage points of usage and updates its share-relative ratio', async () => {
  const user = userEvent.setup()
  renderEditor()
  await selectWindow(user)
  const input = screen.getByLabelText('Share of total window')
  await user.clear(input)
  await user.type(input, '25')
  const preview = screen.getByRole('complementary')
  expect(
    within(preview).getByText('Estimated usage of share after saving')
  ).toBeVisible()
  expect(within(preview).getByText('20%', { selector: 'p' })).toBeVisible()
  expect(
    within(preview).getByText(
      'Equivalent to 5 percentage points of the total window.'
    )
  ).toBeVisible()
  expect(api.put).not.toHaveBeenCalled()
})

test('cancelling an edited share returns to the Key list without writing', async () => {
  const user = userEvent.setup()
  const close = renderEditor()
  await selectWindow(user)
  await user.clear(screen.getByLabelText('Share of total window'))
  await user.type(screen.getByLabelText('Share of total window'), '30')
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(close).toHaveBeenCalledOnce()
  expect(api.put).not.toHaveBeenCalled()
})

test('an unknown save result locks the editor until a deliberate reload', async () => {
  vi.mocked(api.put).mockRejectedValue(new Error('connection closed'))
  const user = userEvent.setup()
  renderEditor()
  await selectWindow(user)
  await confirmApproximation(user)
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  const notice = await screen.findByRole('alert', {
    name: 'The save result is unknown or out of date',
  })
  expect(notice).toBeVisible()
  expect(notice).toHaveFocus()
  expect(screen.getByLabelText('Share of total window')).toHaveValue('20')
  expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
  await user.tab()
  expect(
    screen.getByRole('button', { name: 'Reload current state' })
  ).toHaveFocus()
  await user.keyboard('{Enter}')
  await waitFor(() => expect(api.get).toHaveBeenCalledTimes(2))
  expect(api.put).toHaveBeenCalledTimes(1)
  expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled()
})

test('a missing trustworthy baseline displays unknown estimated usage rather than zero', async () => {
  vi.mocked(api.get).mockResolvedValue(
    response({
      baseline_time: 0,
      estimated_used_micro: 0,
      state: 'baseline_pending',
    })
  )
  const user = userEvent.setup()
  renderEditor()
  await selectWindow(user)
  expect(
    screen.getByText('Waiting for a trustworthy usage sample.')
  ).toBeVisible()
  expect(screen.getByRole('complementary')).not.toHaveTextContent(
    'Equivalent to 0 percentage points'
  )
})

test('advanced controls retain the existing pool closing and recovery interface', async () => {
  const user = userEvent.setup()
  renderEditor()
  await selectWindow(user)
  await user.click(
    screen.getByRole('button', { name: 'Advanced window controls' })
  )
  const dialog = await screen.findByRole('dialog', {
    name: 'Subscription shares',
  })
  expect(
    within(dialog).getByRole('button', { name: 'Close pool' })
  ).toBeVisible()
})

test('leaving advanced controls returns to the share editor without writing', async () => {
  const user = userEvent.setup()
  renderEditor()
  await selectWindow(user)
  await user.click(
    screen.getByRole('button', { name: 'Advanced window controls' })
  )
  const dialog = await screen.findByRole('dialog', {
    name: 'Subscription shares',
  })

  await user.click(
    within(dialog).getByRole('button', { name: 'Back to share editor' })
  )

  expect(
    await screen.findByRole('heading', { name: 'Adjust window share' })
  ).toBeVisible()
  expect(api.put).not.toHaveBeenCalled()
  expect(api.post).not.toHaveBeenCalled()
})

test('personal replacement compares the current and target descriptors before the mobile action', async () => {
  const currentPool = {
    ...pool,
    window_source: 'codex_wham_usage_primary',
  }
  const targetPool = {
    ...pool,
    snapshot_id: 71,
    channel_id: 11,
    channel_name: 'Replacement sample connection',
    window_source: 'codex_wham_usage_primary',
    window_type: 'weekly',
    window_seconds: 604800,
    state: 'candidate',
    replacement_required: true,
  }
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: { token_id: 21, pools: [targetPool, currentPool] },
    },
  })
  const user = userEvent.setup()
  renderEditor()

  await user.selectOptions(
    await screen.findByLabelText('Subscription account'),
    '11'
  )
  await user.selectOptions(
    screen.getByLabelText('Choose one subscription window'),
    '7:71'
  )

  const review = screen.getByRole('region', {
    name: 'Window replacement review',
  })
  expect(within(review).getByText('Current managed window')).toBeVisible()
  expect(within(review).getByText('Target observed window')).toBeVisible()
  expect(within(review).getByText('Subscription account A · #9')).toBeVisible()
  expect(
    within(review).getByText('Replacement sample connection · #11')
  ).toBeVisible()
  expect(
    screen.getByRole('checkbox', {
      name: 'I understand this replacement affects every Key in the pool.',
    })
  ).not.toBeChecked()
  expect(
    screen.getByRole('checkbox', {
      name: 'I understand this is an estimate with no verified error bound and may stop requests only after the share is exceeded.',
    })
  ).not.toBeChecked()
  const action = screen.getByRole('button', { name: 'Replace window' })
  expect(action).toBeDisabled()
  const preview = screen.getByRole('complementary', {
    name: 'Allocation preview',
  })
  expect(
    Boolean(
      preview.compareDocumentPosition(action) & Node.DOCUMENT_POSITION_FOLLOWING
    )
  ).toBe(true)
})

test('mobile selection exposes the complete long account identity and a readable native window label', async () => {
  const account =
    'Subscription_account_for_international_research_with_distinguishing_suffix_A'
  vi.mocked(api.get).mockResolvedValue(response({ channel_name: account }))
  const user = userEvent.setup()
  renderEditor()
  await selectWindow(user)
  expect(
    screen.getByLabelText('Subscription account')
  ).toHaveAccessibleDescription(`${account} · #9`)
  expect(screen.getByText(`${account} · #9`, { selector: 'p' })).toBeVisible()
  expect(
    screen.getByLabelText('Choose one subscription window')
  ).toHaveAccessibleDescription(/5 hours · Primary window · 5-hour window/)
})

test('mobile saving follows the allocation preview and submits the associated form once', async () => {
  const user = userEvent.setup()
  const close = renderEditor()
  await selectWindow(user)
  const preview = screen.getByRole('complementary', {
    name: 'Allocation preview',
  })
  const save = screen.getByRole('button', { name: 'Save changes' })
  expect(
    Boolean(
      preview.compareDocumentPosition(save) & Node.DOCUMENT_POSITION_FOLLOWING
    )
  ).toBe(true)
  await confirmApproximation(user)
  await user.click(save)
  await waitFor(() => expect(close).toHaveBeenCalledOnce())
  expect(api.put).toHaveBeenCalledOnce()
})

test('Enter while a mobile share save is pending never submits a second write', async () => {
  let finishSave!: (value: ReturnType<typeof response>) => void
  vi.mocked(api.put).mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finishSave = resolve
      })
  )
  const user = userEvent.setup()
  const close = renderEditor()
  await selectWindow(user)
  await confirmApproximation(user)
  const save = screen.getByRole('button', { name: 'Save changes' })
  await user.click(screen.getByLabelText('Share of total window'))
  await user.keyboard('{Enter}')
  await waitFor(() => {
    expect(save).toHaveTextContent('Saving...')
    expect(save).toBeDisabled()
  })
  expect(screen.getByLabelText('Share of total window')).toBeDisabled()
  expect(
    screen.getByRole('button', { name: /API Keys \/ Desktop application/ })
  ).toBeDisabled()
  expect(api.put).toHaveBeenCalledOnce()
  await user.keyboard('{Enter}{Enter}')
  expect(api.put).toHaveBeenCalledOnce()
  expect(close).not.toHaveBeenCalled()
  finishSave(response())
  await waitFor(() => expect(close).toHaveBeenCalledOnce())
  expect(api.put).toHaveBeenCalledOnce()
})

test('desktop saving remains in the form beside the allocation preview', async () => {
  const originalMatchMedia = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
    ...originalMatchMedia(query),
    matches: query === '(min-width: 1280px)',
  }))
  renderEditor()
  const save = await screen.findByRole('button', { name: 'Save changes' })
  const form = screen.getByRole('form', { name: 'Adjust window share' })
  const preview = screen.getByRole('complementary', {
    name: 'Allocation preview',
  })
  expect(within(form).getByRole('button', { name: 'Save changes' })).toBe(save)
  expect(
    Boolean(
      save.compareDocumentPosition(preview) & Node.DOCUMENT_POSITION_FOLLOWING
    )
  ).toBe(true)
})
