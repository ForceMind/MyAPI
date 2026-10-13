import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { getStatus } from '@/lib/api'

import { OverviewDashboard } from '../overview-dashboard'

vi.mock('@/lib/api', () => ({ getStatus: vi.fn() }))
vi.mock('../legacy-overview-dashboard', () => ({
  LegacyOverviewDashboard: () => <div data-testid='legacy-data-plane' />,
}))
vi.mock('../../personal-workbench/personal-workbench', () => ({
  PersonalWorkbench: () => <div data-testid='personal-data-plane' />,
}))

function fundingStatus(mode: 'disabled' | 'enabled' | 'retirement') {
  return {
    user_funding_mode: mode,
    user_funding_capabilities: {
      mode, epoch: 3, ready: true,
      can_top_up: false, can_redeem: false,
      can_transfer_affiliate_rewards: false,
      can_purchase_subscription: false, can_view_funding_history: true,
    },
  }
}

const clients: QueryClient[] = []
function renderOverview() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  clients.push(client)
  render(<QueryClientProvider client={client}><OverviewDashboard /></QueryClientProvider>)
  return client
}

afterEach(() => {
  clients.splice(0).forEach(client => client.clear())
  localStorage.clear()
  vi.resetAllMocks()
})

test.each(['disabled', 'enabled', 'retirement'] as const)('confirmed %s mounts only its own data plane', async mode => {
  vi.mocked(getStatus).mockResolvedValue(fundingStatus(mode))
  renderOverview()
  const expected = mode === 'disabled' ? 'personal-data-plane' : 'legacy-data-plane'
  await screen.findByTestId(expected)
  expect(screen.queryByTestId(mode === 'disabled' ? 'legacy-data-plane' : 'personal-data-plane')).not.toBeInTheDocument()
})

test.each([
  ['mode conflict', { ...fundingStatus('enabled'), user_funding_mode: 'disabled' }],
  ['zero epoch', { ...fundingStatus('disabled'), user_funding_capabilities: { ...fundingStatus('disabled').user_funding_capabilities, epoch: 0 } }],
  ['missing mode', { user_funding_capabilities: fundingStatus('disabled').user_funding_capabilities }],
])('%s mounts neither data plane', async (_name, status) => {
  vi.mocked(getStatus).mockResolvedValue(status as ReturnType<typeof fundingStatus>)
  renderOverview()
  await screen.findByText('Unable to confirm workspace mode')
  expect(screen.queryByTestId('personal-data-plane')).not.toBeInTheDocument()
  expect(screen.queryByTestId('legacy-data-plane')).not.toBeInTheDocument()
})

test('a cached commercial placeholder does not mount legacy while the server is being confirmed', async () => {
  localStorage.setItem('status', JSON.stringify(fundingStatus('enabled')))
  let resolveStatus!: (value: ReturnType<typeof fundingStatus>) => void
  vi.mocked(getStatus).mockImplementation(() => new Promise(resolve => { resolveStatus = resolve }))
  renderOverview()
  expect(screen.getByRole('status')).toHaveTextContent('Confirming workspace mode...')
  expect(screen.queryByTestId('personal-data-plane')).not.toBeInTheDocument()
  expect(screen.queryByTestId('legacy-data-plane')).not.toBeInTheDocument()
  await act(async () => resolveStatus(fundingStatus('disabled')))
  await screen.findByTestId('personal-data-plane')
  expect(screen.queryByTestId('legacy-data-plane')).not.toBeInTheDocument()
})

test('failed confirmation offers a read retry and failed refresh removes previously confirmed data', async () => {
  vi.mocked(getStatus).mockRejectedValueOnce(new Error('Synthetic status unavailable')).mockResolvedValueOnce(fundingStatus('disabled')).mockRejectedValueOnce(new Error('Synthetic refresh unavailable'))
  const client = renderOverview()
  await screen.findByText('Unable to confirm workspace mode')
  expect(screen.queryByTestId('legacy-data-plane')).not.toBeInTheDocument()
  await userEvent.setup().click(screen.getByRole('button', { name: 'Retry' }))
  await screen.findByTestId('personal-data-plane')
  await act(async () => { await client.invalidateQueries({ queryKey: ['status'] }) })
  await waitFor(() => expect(screen.queryByTestId('personal-data-plane')).not.toBeInTheDocument())
  expect(screen.queryByTestId('legacy-data-plane')).not.toBeInTheDocument()
  expect(screen.getByText('Unable to confirm workspace mode')).toBeInTheDocument()
})
