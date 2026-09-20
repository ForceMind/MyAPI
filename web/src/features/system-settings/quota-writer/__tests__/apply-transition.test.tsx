import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import axios from 'axios'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import {
  applyQuotaWriterTransition,
  getQuotaWriterTransitionPlan,
} from '../api'
import { ApplyTransitionCard } from '../components/apply-transition-card'
import type { QuotaWriterEpochState } from '../types'

vi.mock('../api', () => ({
  getQuotaWriterStatus: vi.fn(),
  getQuotaWriterTransitionPlan: vi.fn(),
  applyQuotaWriterTransition: vi.fn(),
  getQuotaWriterTransitions: vi.fn(),
  driveQuotaWriterDrains: vi.fn(),
}))

const legacyState: QuotaWriterEpochState = {
  id: 1,
  schema_version: 1,
  mode: 'legacy',
  epoch: 7,
  lock_version: 3,
  updated_at: 1750000000,
}

const planResponse = {
  success: true,
  data: {
    current: legacyState,
    target_mode: 'bridge' as const,
    proposed_epoch: 8,
    ready: false,
    validation: ['cluster_drain_ack'],
    audit: {
      can_enable: false,
      writers: [],
      all_writers_migrated: true,
      batch_queue_empty: true,
      projection_pending: 0,
      balance_drain_pending: 0,
      balance_drain_inflight_zero: true,
      maintenance_backfill_done: true,
      redis_epoch_consistent: true,
      cluster_drain_ack: false,
      inflight_sessions: 0,
      inflight_zero: true,
    },
  },
}

function renderCard() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <ApplyTransitionCard state={legacyState} />
    </QueryClientProvider>
  )
}

function conflictError(missing: string[]) {
  return new axios.AxiosError(
    'Request failed with status code 409',
    'ERR_BAD_REQUEST',
    undefined,
    undefined,
    {
      status: 409,
      statusText: 'Conflict',
      headers: {},
      config: {} as never,
      data: {
        success: false,
        message: 'transition preconditions are not met',
        data: { missing },
      },
    }
  )
}

const ACK_PLACEHOLDER =
  'Describe how cluster-wide drain was confirmed (e.g. every node reports zero in-flight sessions).'

describe('ApplyTransitionCard', () => {
  beforeEach(() => {
    vi.mocked(getQuotaWriterTransitionPlan).mockResolvedValue(planResponse)
    vi.mocked(applyQuotaWriterTransition).mockReset()
  })

  test('blocks submit without the required acknowledgement note for legacy to bridge', async () => {
    const user = userEvent.setup()
    renderCard()

    await user.click(screen.getByRole('button', { name: 'Apply transition' }))

    expect(
      await screen.findByText(
        'An acknowledgement note (1-512 characters) is required for this transition.'
      )
    ).toBeInTheDocument()
    expect(applyQuotaWriterTransition).not.toHaveBeenCalled()
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  })

  test('shows the one-way irreversibility warning before applying', async () => {
    const user = userEvent.setup()
    renderCard()

    await user.type(screen.getByPlaceholderText(ACK_PLACEHOLDER), 'all drained')
    await user.click(screen.getByRole('button', { name: 'Apply transition' }))

    const dialog = await screen.findByRole('alertdialog')
    expect(dialog).toHaveTextContent(/one-way transition and cannot be undone/)
    expect(dialog).toHaveTextContent(/from legacy to bridge/)
    expect(applyQuotaWriterTransition).not.toHaveBeenCalled()
  })

  test('renders the missing audit checks when apply is rejected with 409', async () => {
    const user = userEvent.setup()
    vi.mocked(applyQuotaWriterTransition).mockRejectedValue(
      conflictError(['batch_queue_empty', 'projection_pending'])
    )
    renderCard()

    await user.type(screen.getByPlaceholderText(ACK_PLACEHOLDER), 'all drained')
    await user.click(screen.getByRole('button', { name: 'Apply transition' }))

    const dialog = await screen.findByRole('alertdialog')
    await user.click(
      within(dialog).getByRole('button', { name: 'Apply transition' })
    )

    await waitFor(() => {
      expect(screen.getByText('Preconditions are not met')).toBeInTheDocument()
    })
    expect(
      screen.getByText('The batch update queue is not empty.')
    ).toBeInTheDocument()
    expect(
      screen.getByText('Projection obligations are still pending.')
    ).toBeInTheDocument()
    expect(applyQuotaWriterTransition).toHaveBeenCalledWith({
      target_mode: 'bridge',
      expected_epoch: 7,
      ack_note: 'all drained',
    })
  })
})
