import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import {
  getChannelQuotaAlertDeliveryEvents,
  getChannelQuotaAlertDeliveryStatus,
  runChannelQuotaAlertDelivery,
  updateChannelQuotaAlertDeliverySettings,
} from '@/features/channels/api'

import { ChannelQuotaAlertDeliverySection } from '../channel-quota-alert-delivery-section'

vi.mock('@/features/channels/api', () => ({
  getChannelQuotaAlertDeliveryEvents: vi.fn(),
  getChannelQuotaAlertDeliveryStatus: vi.fn(),
  runChannelQuotaAlertDelivery: vi.fn(),
  updateChannelQuotaAlertDeliverySettings: vi.fn(),
}))

vi.mock('sonner', () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
  },
}))

const configuredStatus = {
  success: true,
  data: {
    policy_enabled: true,
    configured: true,
    endpoint_host: 'alerts.example.com',
    https_only: true,
    redirects_allowed: false,
    timeout_ms: 5000,
    max_attempts: 8,
  },
}

function emptyHistory() {
  return {
    success: true,
    data: { items: [], total: 0, page: 1, page_size: 10 },
  }
}

function renderSection() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <ChannelQuotaAlertDeliverySection />
    </QueryClientProvider>
  )
}

function deferred<T>() {
  let resolve = (_value: T) => {}
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

describe('ChannelQuotaAlertDeliverySection', () => {
  beforeEach(() => {
    vi.mocked(getChannelQuotaAlertDeliveryStatus).mockResolvedValue(
      configuredStatus
    )
    vi.mocked(getChannelQuotaAlertDeliveryEvents).mockResolvedValue(
      emptyHistory()
    )
    vi.mocked(updateChannelQuotaAlertDeliverySettings).mockResolvedValue(
      configuredStatus
    )
    vi.mocked(runChannelQuotaAlertDelivery).mockResolvedValue({
      success: true,
      data: {
        enabled: true,
        claimed: 1,
        delivered: 1,
        retryable: 0,
        quarantined: 0,
      },
    })
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  test('shows a loading state before the delivery status resolves', () => {
    const status = deferred<typeof configuredStatus>()
    vi.mocked(getChannelQuotaAlertDeliveryStatus).mockReturnValue(
      status.promise
    )

    renderSection()

    expect(
      screen.getByText('Loading quota alert delivery...')
    ).toBeInTheDocument()
  })

  test('shows an inline retry state when delivery status cannot be loaded', async () => {
    vi.mocked(getChannelQuotaAlertDeliveryStatus)
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(configuredStatus)

    renderSection()

    expect(
      await screen.findByText('Unable to load quota alert delivery status.')
    ).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
    await waitFor(() => {
      expect(screen.getByText('Configured')).toBeInTheDocument()
    })
  })

  test('keeps manual delivery disabled when the provider alert policy is disabled and shows an empty history', async () => {
    vi.mocked(getChannelQuotaAlertDeliveryStatus).mockResolvedValue({
      ...configuredStatus,
      data: { ...configuredStatus.data, policy_enabled: false },
    })

    renderSection()

    expect(
      await screen.findByText(
        'Enable provider quota alerts before alert events can be delivered.'
      )
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Run delivery now' })
    ).toBeDisabled()
    expect(
      screen.getByText('No quota alert delivery events yet.')
    ).toBeInTheDocument()
  })

  test('disables saving while the delivery configuration is being persisted', async () => {
    const user = userEvent.setup()
    const update = deferred<typeof configuredStatus>()
    vi.mocked(updateChannelQuotaAlertDeliverySettings).mockReturnValue(
      update.promise
    )
    renderSection()

    await screen.findByText('Configured')
    await user.type(
      screen.getByLabelText('Webhook URL'),
      'https://alerts.example.com/quota'
    )
    await user.type(
      screen.getByLabelText('Webhook signing secret'),
      '0123456789abcdef'
    )
    await user.click(
      screen.getByRole('button', { name: 'Save delivery configuration' })
    )

    expect(screen.getByRole('button', { name: 'Saving...' })).toBeDisabled()
    expect(screen.getByLabelText('Webhook URL')).toBeDisabled()
    update.resolve(configuredStatus)

    await waitFor(() => {
      expect(updateChannelQuotaAlertDeliverySettings).toHaveBeenCalledWith({
        webhook_url: 'https://alerts.example.com/quota',
        webhook_secret: '0123456789abcdef',
      })
    })
  })

  test('renders delivery history and runs one manual delivery pass', async () => {
    const user = userEvent.setup()
    vi.mocked(getChannelQuotaAlertDeliveryEvents).mockResolvedValue({
      success: true,
      data: {
        items: [
          {
            id: 17,
            event_key: 'quota-alert-17',
            snapshot_id: 3,
            channel_id: 9,
            status: 'critical',
            kind: 'entered',
            state: 'retryable',
            attempt_count: 2,
            next_attempt_at: 1_756_560_060,
            observed_at: 1_756_560_000,
            created_at: 1_756_560_000,
            updated_at: 1_756_560_000,
          },
        ],
        total: 1,
        page: 1,
        page_size: 10,
      },
    })

    renderSection()

    expect(await screen.findByText('quota-alert-17')).toBeInTheDocument()
    expect(screen.getByText('retryable')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Run delivery now' }))
    await waitFor(() => {
      expect(runChannelQuotaAlertDelivery).toHaveBeenCalledOnce()
    })
  })
})
