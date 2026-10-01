import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, expect, test, vi } from 'vitest'

import { getUserQuotaDates } from '@/features/dashboard/api'

import { SummaryCards } from '../summary-cards'
const funding = vi.hoisted(() => ({ mode: 'enabled' as 'enabled' | 'disabled' }))
beforeEach(() => { funding.mode = 'enabled' })

vi.mock('@/features/dashboard/api', () => ({
  getUserQuotaDates: vi.fn(),
}))
vi.mock('@tanstack/react-router', () => ({
  Link: (props: { children?: ReactNode; to?: string }) => (
    <a href={props.to}>{props.children}</a>
  ),
}))
vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({ status: { user_funding_capabilities: { mode: funding.mode, ready: true, epoch: 1 } }, loading: false, error: null, confirmed: true }),
}))
vi.mock('@/components/page-transition', () => ({
  StaggerContainer: (props: { children: ReactNode }) => (
    <div>{props.children}</div>
  ),
  StaggerItem: (props: { children: ReactNode }) => <div>{props.children}</div>,
}))
vi.mock('@/features/dashboard/components/ui/stat-card', () => ({
  StatCard: (props: { title: string; value: string }) => (
    <div>{`${props.title}: ${props.value}`}</div>
  ),
}))

test('enabled commercial summary keeps balance status but does not promote wallet purchase', async () => {
  vi.mocked(getUserQuotaDates).mockResolvedValue({
    success: true,
    data: [],
  } as never)

  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <SummaryCards />
    </QueryClientProvider>
  )

  expect(screen.getByText('Usage at a glance')).toBeInTheDocument()
  expect(screen.getByText('Credit remaining')).toBeInTheDocument()
  expect(screen.queryByRole('link', { name: 'Wallet' })).not.toBeInTheDocument()
})

test('disabled commercial summary omits credit balance and runway but preserves usage', () => {
  funding.mode = 'disabled'
  vi.mocked(getUserQuotaDates).mockResolvedValue({ success: true, data: [] } as never)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={client}><SummaryCards /></QueryClientProvider>)
  expect(screen.getByText('Usage at a glance')).toBeInTheDocument()
  expect(screen.queryByText('Credit remaining')).not.toBeInTheDocument()
  expect(screen.queryByText('Runway')).not.toBeInTheDocument()
})
