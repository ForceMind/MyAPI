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
import i18next from 'i18next'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { SubscriptionShareDialog } from '../subscription-share-dialog'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), put: vi.fn() } }))
vi.mock('sonner', () => ({ toast: { success: vi.fn() } }))

const now = 1_797_000_000
const activePool = {
  pool_id: 7,
  snapshot_id: 70,
  revision: 4,
  channel_name: 'Personal Codex',
  plan_type: 'plus',
  window_slot: 'primary',
  window_type: 'weekly',
  window_seconds: 604800,
  reset_at: now + 3000,
  observed_at: now - 30,
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
  baseline_time: now - 600,
  replacement_required: false,
  rebaseline_required: false,
}
const candidatePool = {
  ...activePool,
  pool_id: 0,
  snapshot_id: 71,
  revision: 0,
  channel_name: 'Fresh candidate',
  window_type: 'primary',
  allocated_bps: 0,
  share_bps: 0,
  estimated_used_micro: 0,
  unattributed_micro: 0,
  pending_attempts: 0,
  unsampled_attempts: 0,
  baseline_time: 0,
}
const replacementPool = {
  ...activePool,
  snapshot_id: 72,
  channel_name: 'Replacement window',
  state: 'candidate',
  window_type: 'monthly',
  replacement_required: true as const,
}
const rebaselinePool = {
  ...activePool,
  snapshot_id: 73,
  channel_name: 'Recovery baseline',
  uncertain_attempts: 3,
  rebaseline_required: true as const,
}
const originalAuth = useAuthStore.getState()
const originalInnerWidth = window.innerWidth
let client: QueryClient

function setSession(id: number, role: number, sid: string) {
  useAuthStore.setState((state) => ({
    ...state,
    auth: {
      ...state.auth,
      user: { id, role, username: `fixture-${id}` },
      session: {
        sid,
        current: true,
        login_method: 'password',
        ip: '127.0.0.1',
        user_agent: 'test',
        created_at: 1,
        last_active_at: 1,
        expires_at: now + 1000,
      },
    },
  }))
}

function response(pools = [activePool, candidatePool]) {
  return { data: { success: true, data: { token_id: 21, pools } } }
}

function renderDialog(onClose = vi.fn()) {
  render(
    <QueryClientProvider client={client}>
      <SubscriptionShareDialog tokenId={21} onClose={onClose} />
    </QueryClientProvider>
  )
  return onClose
}

beforeEach(() => {
  vi.resetAllMocks()
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  setSession(1, 100, 'root-session')
  vi.mocked(api.get).mockResolvedValue(response())
  vi.mocked(api.put).mockResolvedValue(response())
})

afterEach(async () => {
  client.clear()
  useAuthStore.setState(originalAuth)
  Object.defineProperty(window, 'innerWidth', {
    configurable: true,
    value: originalInnerWidth,
  })
  await i18next.changeLanguage('en')
  vi.restoreAllMocks()
})

test('Root explicitly selects a window and saves an exact basis-point share', async () => {
  const user = userEvent.setup()
  const onClose = renderDialog()
  const dialog = await screen.findByRole('dialog', {
    name: 'Subscription shares',
  })

  expect(api.get).toHaveBeenCalledWith(
    '/api/token/21/subscription-share',
    expect.objectContaining({ signal: expect.any(AbortSignal) })
  )
  expect(within(dialog).getByRole('button', { name: 'Save share' })).toBeDisabled()
  expect(within(dialog).getByText('Personal Codex')).toBeVisible()
  expect(within(dialog).getByText('Fresh candidate')).toBeVisible()
  expect(dialog).toHaveTextContent('Native window: primary · weekly')
  expect(dialog).toHaveTextContent(
    'Sample age: 30 seconds · Maximum age: 300 seconds'
  )

  await user.click(within(dialog).getByLabelText(/Fresh candidate/))
  await user.clear(within(dialog).getByLabelText('Share percentage'))
  expect(within(dialog).getByText('This creates a managed pool')).toBeVisible()
  await user.type(
    within(dialog).getByLabelText('Share percentage'),
    '12.34'
  )
  await user.click(
    within(dialog).getByLabelText(
      'I understand this is an estimate with no verified error bound and may stop requests only after the share is exceeded.'
    )
  )
  await user.click(within(dialog).getByRole('button', { name: 'Save share' }))

  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1))
  expect(api.put).toHaveBeenCalledWith(
    '/api/token/21/subscription-share',
    {
      id: expect.stringMatching(/^[0-9a-f]{64}$/),
      pool_id: 0,
      snapshot_id: 71,
      expected_revision: 0,
      share_bps: 1234,
      confirmed_approximate: true,
    },
    expect.any(Object)
  )
  expect(onClose).toHaveBeenCalledTimes(1)
})

test('Root can save zero to pause a Key but cannot exceed the pool allocation', async () => {
  const user = userEvent.setup()
  renderDialog()
  const dialog = await screen.findByRole('dialog', {
    name: 'Subscription shares',
  })
  await user.click(within(dialog).getByLabelText(/Personal Codex/))
  const input = within(dialog).getByLabelText('Share percentage')
  const confirmation = within(dialog).getByLabelText(
    'I understand this is an estimate with no verified error bound and may stop requests only after the share is exceeded.'
  )
  await user.click(confirmation)

  await user.clear(input)
  await user.type(input, '0')
  expect(
    within(dialog).getByText(
      'Use 0% to 100% with at most two decimal places. A 0% share blocks this Key’s requests for the window while keeping its binding and history.'
    )
  ).toBeVisible()
  expect(within(dialog).getByRole('button', { name: 'Save share' })).toBeEnabled()

  await user.clear(input)
  await user.type(input, '20.01')
  expect(
    within(dialog).getByText('This share would make the pool allocation exceed 100%.')
  ).toBeVisible()
  expect(api.put).not.toHaveBeenCalled()
})

test('saving zero sends an explicit zero share without removing the pool binding', async () => {
  const user = userEvent.setup()
  renderDialog()
  const dialog = await screen.findByRole('dialog', {
    name: 'Subscription shares',
  })
  await user.click(within(dialog).getByLabelText(/Personal Codex/))
  const input = within(dialog).getByLabelText('Share percentage')
  await user.clear(input)
  await user.type(input, '0')
  await user.click(
    within(dialog).getByLabelText(
      'I understand this is an estimate with no verified error bound and may stop requests only after the share is exceeded.'
    )
  )
  await user.click(within(dialog).getByRole('button', { name: 'Save share' }))
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1))
  expect(api.put).toHaveBeenCalledWith(
    '/api/token/21/subscription-share',
    expect.objectContaining({
      pool_id: 7,
      snapshot_id: 70,
      expected_revision: 4,
      share_bps: 0,
    }),
    expect.any(Object)
  )
})

test('Root must confirm a pool-wide window replacement before it is written explicitly', async () => {
  const user = userEvent.setup()
  vi.mocked(api.get).mockResolvedValue(
    response([replacementPool, candidatePool])
  )
  vi.mocked(api.put).mockResolvedValue(
    response([replacementPool, candidatePool])
  )
  renderDialog()
  const dialog = await screen.findByRole('dialog', {
    name: 'Subscription shares',
  })

  await user.click(within(dialog).getByLabelText(/Replacement window/))
  expect(
    within(dialog).getByText(
      'This replaces the managed window for every Key in this pool'
    )
  ).toBeVisible()
  expect(
    within(dialog).getByText(
      'Existing Key shares and old window history are kept. The replacement starts from a new fresh baseline and does not require signing in again.'
    )
  ).toBeVisible()
  await user.click(
    within(dialog).getByLabelText(
      'I understand this is an estimate with no verified error bound and may stop requests only after the share is exceeded.'
    )
  )
  const save = within(dialog).getByRole('button', { name: 'Save share' })
  expect(save).toBeDisabled()
  await user.click(save)
  expect(api.put).not.toHaveBeenCalled()

  await user.click(
    within(dialog).getByLabelText(
      'I understand this replacement affects every Key in the pool.'
    )
  )
  expect(save).toBeEnabled()
  await user.click(save)

  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1))
  expect(api.put).toHaveBeenCalledWith(
    '/api/token/21/subscription-share',
    {
      id: expect.stringMatching(/^[0-9a-f]{64}$/),
      pool_id: 7,
      snapshot_id: 72,
      expected_revision: 4,
      share_bps: 1000,
      confirmed_approximate: true,
      replace_window: true,
    },
    expect.any(Object)
  )
})

test('changing the selected window clears both replacement confirmations', async () => {
  const user = userEvent.setup()
  vi.mocked(api.get).mockResolvedValue(
    response([replacementPool, candidatePool])
  )
  renderDialog()
  const dialog = await screen.findByRole('dialog', {
    name: 'Subscription shares',
  })

  await user.click(within(dialog).getByLabelText(/Replacement window/))
  await user.click(
    within(dialog).getByLabelText(
      'I understand this replacement affects every Key in the pool.'
    )
  )
  await user.click(
    within(dialog).getByLabelText(
      'I understand this is an estimate with no verified error bound and may stop requests only after the share is exceeded.'
    )
  )
  await user.click(within(dialog).getByLabelText(/Fresh candidate/))
  await user.click(within(dialog).getByLabelText(/Replacement window/))

  expect(
    within(dialog).getByLabelText(
      'I understand this replacement affects every Key in the pool.'
    )
  ).not.toBeChecked()
  expect(
    within(dialog).getByLabelText(
      'I understand this is an estimate with no verified error bound and may stop requests only after the share is exceeded.'
    )
  ).not.toBeChecked()
  expect(within(dialog).getByRole('button', { name: 'Save share' })).toBeDisabled()
  expect(api.put).not.toHaveBeenCalled()
})

test('Root must confirm recovery from a fresh baseline without clearing estimated usage', async () => {
  const user = userEvent.setup()
  vi.mocked(api.get).mockResolvedValue(response([rebaselinePool]))
  vi.mocked(api.put).mockResolvedValue(response([rebaselinePool]))
  renderDialog()
  const dialog = await screen.findByRole('dialog', {
    name: 'Subscription shares',
  })

  const uncertainLabel = within(dialog).getByText('Uncertain attempts')
  expect(uncertainLabel.parentElement).toHaveTextContent('3')
  await user.click(within(dialog).getByLabelText(/Recovery baseline/))
  expect(
    within(dialog).getByText('Resume observation from a fresh baseline')
  ).toBeVisible()
  expect(
    within(dialog).getByText(
      'Existing estimated usage is kept. Requests that could not be attributed remain uncertain, and observation resumes from a new fresh sample.'
    )
  ).toBeVisible()
  await user.click(
    within(dialog).getByLabelText(
      'I understand this is an estimate with no verified error bound and may stop requests only after the share is exceeded.'
    )
  )
  const save = within(dialog).getByRole('button', { name: 'Save share' })
  expect(save).toBeDisabled()
  expect(api.put).not.toHaveBeenCalled()

  await user.click(
    within(dialog).getByLabelText(
      'I understand this recovery affects every Key in the pool.'
    )
  )
  await user.click(save)

  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1))
  expect(api.put).toHaveBeenCalledWith(
    '/api/token/21/subscription-share',
    {
      id: expect.stringMatching(/^[0-9a-f]{64}$/),
      pool_id: 7,
      snapshot_id: 73,
      expected_revision: 4,
      share_bps: 1000,
      confirmed_approximate: true,
      replace_window: true,
    },
    expect.any(Object)
  )
})

test('an owner sees only their configured share and no pool allocation controls', async () => {
  setSession(2, 1, 'owner-session')
  vi.mocked(api.get).mockResolvedValue(
    response([activePool, candidatePool, replacementPool])
  )
  renderDialog()
  const region = await screen.findByRole('region', {
    name: 'Your subscription share',
  })
  expect(within(region).getByText('Personal Codex')).toBeVisible()
  expect(within(region).getByText('10%')).toBeVisible()
  expect(within(region).queryByText('Fresh candidate')).not.toBeInTheDocument()
  expect(within(region).queryByText('Replacement window')).not.toBeInTheDocument()
  expect(within(region).queryByText('Recovery baseline')).not.toBeInTheDocument()
  expect(within(region).queryByText('Total allocated')).not.toBeInTheDocument()
  expect(screen.queryByLabelText('Share percentage')).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Save share' })).not.toBeInTheDocument()
})

test('an owner can inspect a zero-percent paused share without editing it', async () => {
  setSession(2, 1, 'owner-paused-session')
  vi.mocked(api.get).mockResolvedValue(response([{ ...activePool, share_bps: 0 }]))
  renderDialog()
  const dialog = await screen.findByRole('dialog', { name: 'Subscription shares' })
  expect(await within(dialog).findByText('Personal Codex')).toBeVisible()
  expect(within(dialog).queryByRole('button', { name: 'Save share' })).not.toBeInTheDocument()
})

test('an owner still sees a stopped allocated pool that Root can rebaseline', async () => {
  setSession(2, 1, 'owner-stopped-session')
  vi.mocked(api.get).mockResolvedValue(response([{ ...activePool, state: 'discontinuity', rebaseline_required: true, uncertain_attempts: 3 }]))
  renderDialog()
  const region = await screen.findByRole('region', { name: 'Your subscription share' })
  expect(await within(region).findByText('Personal Codex')).toBeVisible()
  expect(within(region).getByText('10%')).toBeVisible()
  expect(screen.queryByRole('button', { name: 'Save share' })).not.toBeInTheDocument()
})

test('an unknown save result stays locked until a manual reload and is never repeated', async () => {
  const user = userEvent.setup()
  vi.mocked(api.put).mockRejectedValue(new Error('connection closed'))
  renderDialog()
  const dialog = await screen.findByRole('dialog', {
    name: 'Subscription shares',
  })
  await user.click(within(dialog).getByLabelText(/Personal Codex/))
  await user.click(
    within(dialog).getByLabelText(
      'I understand this is an estimate with no verified error bound and may stop requests only after the share is exceeded.'
    )
  )
  await user.click(within(dialog).getByRole('button', { name: 'Save share' }))

  expect(
    await within(dialog).findByText('The save result is unknown or out of date')
  ).toBeVisible()
  expect(api.put).toHaveBeenCalledTimes(1)
  expect(within(dialog).getByRole('button', { name: 'Save share' })).toBeDisabled()
  await user.click(within(dialog).getByRole('button', { name: 'Reload current state' }))
  await waitFor(() => expect(api.get).toHaveBeenCalledTimes(2))
  expect(api.put).toHaveBeenCalledTimes(1)
  expect(within(dialog).getByRole('button', { name: 'Save share' })).toBeDisabled()
})

test('a failed initial read hides controls and a manual retry loads current data', async () => {
  const user = userEvent.setup()
  vi.mocked(api.get)
    .mockRejectedValueOnce(new Error('temporary read failure'))
    .mockResolvedValueOnce(response())
  renderDialog()
  const failure = await screen.findByText('Could not load subscription shares.')
  const alert = failure.closest('[role="alert"]') as HTMLElement
  expect(within(alert).getByText('Could not load subscription shares.')).toBeVisible()
  expect(screen.queryByLabelText('Share percentage')).not.toBeInTheDocument()
  await user.click(within(alert).getByRole('button', { name: 'Retry' }))
  expect(await screen.findByText('Personal Codex')).toBeVisible()
  expect(api.get).toHaveBeenCalledTimes(2)
})

test('stale and baseline states explain pauses and distinct pending counters', async () => {
  vi.mocked(api.get).mockResolvedValue(
    response([
      {
        ...activePool,
        observed_at: now - 301,
        sample_age_seconds: 301,
        state: 'stale',
      },
      {
        ...candidatePool,
        pool_id: 8,
        state: 'baseline_pending',
        pending_attempts: 3,
        unsampled_attempts: 0,
      },
      {
        ...candidatePool,
        pool_id: 9,
        snapshot_id: 72,
        state: 'discontinuity',
        reason: 'usage_decreased',
      },
    ])
  )
  renderDialog()
  const dialog = await screen.findByRole('dialog', {
    name: 'Subscription shares',
  })
  expect(within(dialog).getByText('Stale sample — requests are paused')).toBeVisible()
  expect(within(dialog).getByText('Waiting for baseline sample')).toBeVisible()
  expect(
    within(dialog).getByText('Discontinuity — requests are paused')
  ).toBeVisible()
  expect(within(dialog).getByText('usage_decreased')).toBeVisible()
  expect(
    within(dialog).getByText(
      'A fresh sample after enabling is required before this managed pool can accept requests.'
    )
  ).toBeVisible()
  expect(within(dialog).getAllByText('Attempts awaiting attribution')).toHaveLength(3)
  expect(within(dialog).getAllByText('Unsampled attempts')).toHaveLength(3)
  expect(
    within(dialog).getAllByText(
      'A successful zero-growth sample releases unsampled request slots, but it does not prove those requests used no subscription capacity; attribution remains pending.'
    )
  ).toHaveLength(2)
})

test('a late response from an old session cannot display or update the new session', async () => {
  const resolvers: Array<(value: ReturnType<typeof response>) => void> = []
  vi.mocked(api.get).mockImplementation(
    () =>
      new Promise((done) => {
        resolvers.push(done)
      }) as never
  )
  renderDialog()
  await screen.findByText('Loading...')
  act(() => setSession(3, 1, 'new-session'))
  await act(async () => resolvers[0](response()))
  expect(screen.queryByText('Personal Codex')).not.toBeInTheDocument()
  expect(screen.getByText('Loading...')).toBeVisible()
  expect(api.put).not.toHaveBeenCalled()
})

test('a session change clears a selected replacement and its confirmations', async () => {
  const user = userEvent.setup()
  vi.mocked(api.get).mockResolvedValue(
    response([replacementPool, candidatePool])
  )
  renderDialog()
  const dialog = await screen.findByRole('dialog', {
    name: 'Subscription shares',
  })
  await user.click(within(dialog).getByLabelText(/Replacement window/))
  await user.click(
    within(dialog).getByLabelText(
      'I understand this replacement affects every Key in the pool.'
    )
  )
  await user.click(
    within(dialog).getByLabelText(
      'I understand this is an estimate with no verified error bound and may stop requests only after the share is exceeded.'
    )
  )

  act(() => setSession(3, 100, 'new-root-session'))

  await waitFor(() => expect(api.get).toHaveBeenCalledTimes(2))
  expect(screen.getByLabelText(/Replacement window/)).not.toBeChecked()
  expect(
    screen.queryByLabelText(
      'I understand this replacement affects every Key in the pool.'
    )
  ).not.toBeInTheDocument()
  expect(api.put).not.toHaveBeenCalled()
})

test('critical approximation copy remains accessible in a 320px viewport with long translations', async () => {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: 320 })
  i18next.addResourceBundle(
    'fr',
    'translation',
    {
      'Subscription shares':
        'Répartition approximative des fenêtres d’abonnement',
      'Request size, model, failures, external clients, measurement delay and rounding can all cause error. There is no verified numerical error bound.':
        'La taille des requêtes, le modèle, les échecs, les clients externes, le délai de mesure et l’arrondi peuvent tous produire une erreur. Aucune limite numérique vérifiée de cette erreur ne peut être garantie.',
    },
    true,
    true
  )
  await i18next.changeLanguage('fr')
  renderDialog()
  const dialog = await screen.findByRole('dialog', {
    name: 'Répartition approximative des fenêtres d’abonnement',
  })
  expect(dialog).toBeVisible()
  expect(
    within(dialog).getByText(/Aucune limite numérique vérifiée/)
  ).toBeVisible()
  expect(within(dialog).getByText('Personal Codex')).toBeVisible()
})
