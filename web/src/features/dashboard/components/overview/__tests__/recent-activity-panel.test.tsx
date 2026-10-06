import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { getRecentLogOverview } from '@/features/dashboard/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { RecentActivityPanel } from '../recent-activity-panel'

vi.mock('@/features/dashboard/api', () => ({ getRecentLogOverview: vi.fn() }))
vi.mock('@tanstack/react-router', () => ({
  Link: (props: {
    children?: ReactNode
    to?: string
    params?: { section?: string }
    search?: { type?: string[]; startTime?: number }
    'aria-label'?: string
  }) => {
    let href = props.to?.replace('$section', props.params?.section ?? '') ?? ''
    if (props.search?.type?.[0]) {
      href += `?type=${props.search.type[0]}`
      if (props.search.startTime) {
        href += `&startTime=${props.search.startTime}`
      }
    }
    return (
      <a href={href} aria-label={props['aria-label']}>
        {props.children}
      </a>
    )
  },
}))

function renderPanel() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <RecentActivityPanel />
    </QueryClientProvider>
  )
}

function setUser(id: number, role: number) {
  useAuthStore.getState().auth.setUser({ id, username: `user-${id}`, role })
}

afterEach(() => {
  useAuthStore.getState().auth.reset('idle')
  vi.clearAllMocks()
})

test('regular user sees only its minimal recent log projection, never raw content', async () => {
  setUser(1, ROLE.USER)
  vi.mocked(getRecentLogOverview).mockResolvedValue({
    success: true,
    data: {
      requests: [
        {
          id: 1,
          created_at: 1_700_000_000,
          model_name: 'gpt-safe',
          content: 'private prompt',
        },
      ],
      errors: [
        {
          id: 2,
          created_at: 1_700_000_100,
          model_name: 'failed-model',
          content: 'private provider error',
        },
      ],
    },
  } as never)

  renderPanel()

  expect(await screen.findByText('gpt-safe')).toBeInTheDocument()
  expect(screen.getByText('failed-model')).toBeInTheDocument()
  expect(screen.queryByText(/private/)).not.toBeInTheDocument()
  expect(getRecentLogOverview).toHaveBeenCalledWith(false)
  expect(screen.getByRole('link', { name: 'View logs' })).toHaveAttribute(
    'href',
    '/usage-logs/common'
  )
  expect(
    screen.getByRole('link', { name: 'Recent errors · View logs' })
  ).toHaveAttribute('href', '/usage-logs/common?type=5&startTime=1700000040000')
})

test('switching accounts removes the previous administrator activity', async () => {
  setUser(1, ROLE.ADMIN)
  vi.mocked(getRecentLogOverview)
    .mockResolvedValueOnce({
      success: true,
      data: {
        requests: [
          {
            id: 1,
            created_at: 1_700_000_000,
            model_name: 'old-model',
            username: 'alice',
          },
        ],
        errors: [],
      },
    })
    .mockResolvedValueOnce({
      success: true,
      data: {
        requests: [
          {
            id: 2,
            created_at: 1_700_000_100,
            model_name: 'new-model',
          },
        ],
        errors: [],
      },
    })

  renderPanel()
  expect(
    await screen.findByText('old-model', { exact: false })
  ).toBeInTheDocument()
  expect(getRecentLogOverview).toHaveBeenCalledWith(true)

  setUser(2, ROLE.USER)

  expect(await screen.findByText('new-model')).toBeInTheDocument()
  expect(screen.queryByText(/alice/)).not.toBeInTheDocument()
  expect(screen.queryByText(/old-model/)).not.toBeInTheDocument()
  expect(getRecentLogOverview).toHaveBeenLastCalledWith(false)
  expect(
    screen.getByRole('link', { name: 'Recent errors · View logs' })
  ).toHaveAttribute('href', '/usage-logs/common?type=5')
})

test('failed activity load shows a generic retry without exposing server text', async () => {
  setUser(1, ROLE.USER)
  vi.mocked(getRecentLogOverview)
    .mockResolvedValueOnce({
      success: false,
      message: 'private upstream detail',
    })
    .mockResolvedValueOnce({
      success: true,
      data: { requests: [], errors: [] },
    })

  renderPanel()

  expect(
    await screen.findByText('Unable to load recent activity')
  ).toBeInTheDocument()
  expect(screen.queryByText(/private upstream detail/)).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
  await waitFor(() =>
    expect(
      screen.queryByText('Unable to load recent activity')
    ).not.toBeInTheDocument()
  )
  expect(screen.getByText('No recent requests')).toBeInTheDocument()
  expect(screen.getByText('No recent errors')).toBeInTheDocument()
})

test('invalid log timestamps degrade to an unknown time without breaking the panel', async () => {
  setUser(1, ROLE.USER)
  vi.mocked(getRecentLogOverview).mockResolvedValue({
    success: true,
    data: {
      requests: [
        {
          id: 3,
          created_at: Number.MAX_VALUE,
          model_name: 'valid-model',
        },
      ],
      errors: [],
    },
  })

  renderPanel()

  expect(await screen.findByText('valid-model')).toBeInTheDocument()
  expect(screen.getByText('Unknown')).toBeInTheDocument()
})
