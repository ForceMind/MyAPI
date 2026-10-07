import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { Profile } from '../index'
import type { UserProfile } from '../types'

const originalAuth = useAuthStore.getState()
const clients: QueryClient[] = []
const profile: UserProfile = {
  id: 42,
  username: 'operator',
  display_name: 'Operations',
  role: 1,
  group: 'default',
  quota: 5000000,
  used_quota: 1000000,
  request_count: 42,
  status: 1,
  aff_count: 0,
  aff_quota: 0,
  aff_history_quota: 0,
  created_time: 0,
}

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.setState(originalAuth)
  localStorage.clear()
})

describe('profile task layout', () => {
  it('keeps one task heading and preserves account security dialog dismissal in the new page frame', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/api/user/self') {
        return { data: { success: true, data: profile } }
      }
      if (url === '/api/user/sessions' || url === '/api/user/oauth/bindings') {
        return { data: { success: true, data: [] } }
      }
      if (url === '/api/user/2fa/status') {
        return {
          data: {
            success: true,
            data: { enabled: false, locked: false, backup_codes_remaining: 0 },
          },
        }
      }
      if (url === '/api/user/passkey') {
        return { data: { success: true, data: { enabled: false } } }
      }
      throw new Error(`Unexpected test request: ${url}`)
    })
    useAuthStore.setState({
      auth: {
        ...originalAuth.auth,
        user: { ...profile, permissions: { sidebar_settings: false } },
      },
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    clients.push(client)
    client.setQueryData(['status'], { checkin_enabled: false })
    const route = createRootRoute({ component: Profile })
    const router = createRouter({
      routeTree: route,
      history: createMemoryHistory({ initialEntries: ['/'] }),
    })
    await router.load()
    const user = userEvent.setup()
    render(
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    )
    expect(screen.getByRole('main', { name: 'Profile' })).toHaveAttribute(
      'id',
      'content'
    )
    expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1)
    expect(
      await screen.findByRole('heading', { name: 'Operations', level: 2 })
    ).toBeInTheDocument()
    const changePassword = screen.getByRole('button', {
      name: /Change Password Update your password/,
    })
    await user.click(changePassword)
    expect(
      await screen.findByRole('dialog', { name: 'Change Password' })
    ).toBeInTheDocument()
    await user.keyboard('{Escape}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(changePassword).toHaveFocus()
  })
})
