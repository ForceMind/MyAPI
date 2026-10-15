import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { AnchorHTMLAttributes } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { getPlaygroundKeys } from '@/features/playground/api'
import { completePlaygroundKeyHandoff, readPlaygroundKeyHandoff } from '@/features/playground/lib/playground-key-handoff'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { OverviewKeys } from '../overview-keys'

const navigate = vi.hoisted(() => vi.fn())
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
  Link: (props: AnchorHTMLAttributes<HTMLAnchorElement> & { to?: string }) => {
    const { to, children, ...attributes } = props
    return <a href={to} {...attributes}>{children}</a>
  },
}))
vi.mock('@/features/playground/api', () => ({ getPlaygroundKeys: vi.fn() }))
vi.mock('@/hooks/use-sidebar-config', () => ({ useIsSidebarModuleVisible: () => true }))
const selectedKey = { id: 31, name: 'chosen-key', status: 1, group: 'default', remain_quota: 100, unlimited_quota: false, expired_time: -1 }

beforeEach(() => {
  navigate.mockClear()
  vi.mocked(getPlaygroundKeys).mockResolvedValue([selectedKey])
  useAuthStore.getState().auth.setBundle({
    access_token: 'access-token', token_type: 'Bearer', access_expires_at: 4_000_000_000,
    user: { id: 7, username: 'root', role: ROLE.SUPER_ADMIN },
    session: { sid: 'session-a', current: true, login_method: 'password', ip: '127.0.0.1', user_agent: 'test', created_at: 1, last_active_at: 1, expires_at: 4_000_000_000 },
  })
})
afterEach(() => {
  const handoff = readPlaygroundKeyHandoff(7, 'session-a')
  if (handoff.status !== 'none') completePlaygroundKeyHandoff(handoff.requestId)
  useAuthStore.getState().auth.reset('idle')
})
function renderKeys() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={client}><OverviewKeys /></QueryClientProvider>)
}

test('creates a session-bound handoff only after an explicit Key choice and Playground action', async () => {
  const user = userEvent.setup()
  renderKeys()
  await user.click(screen.getByText('Test with an existing Key'))
  const select = screen.getByRole('combobox', { name: 'Select an API Key' })
  await waitFor(() => expect(select).not.toBeDisabled())
  expect(select).toHaveValue('')
  expect(screen.getByRole('button', { name: 'Open Playground' })).toBeDisabled()
  await user.selectOptions(select, '31')
  expect(readPlaygroundKeyHandoff(7, 'session-a')).toEqual({ status: 'none' })
  await user.click(screen.getByRole('button', { name: 'Open Playground' }))
  expect(readPlaygroundKeyHandoff(7, 'session-a')).toMatchObject({ status: 'accepted', keyId: 31 })
  expect(navigate).toHaveBeenCalledWith({ to: '/playground' })
})

test('keeps a disabled Key from opening Playground and preserves its visible identity', async () => {
  vi.mocked(getPlaygroundKeys).mockResolvedValue([{ ...selectedKey, status: 2 }])
  const user = userEvent.setup()
  renderKeys()
  await user.click(screen.getByText('Test with an existing Key'))
  const select = screen.getByRole('combobox', { name: 'Select an API Key' })
  await waitFor(() => expect(select).not.toBeDisabled())
  await user.selectOptions(select, '31')
  expect(screen.getByTestId('request-flow-selected-key')).toHaveTextContent('chosen-key')
  expect(screen.getByRole('button', { name: 'Open Playground' })).toBeDisabled()
  expect(readPlaygroundKeyHandoff(7, 'session-a')).toEqual({ status: 'none' })
})

test('offers a retry after a safe Key read fails without showing a fake Key count', async () => {
  vi.mocked(getPlaygroundKeys).mockRejectedValueOnce(new Error('offline'))
  const user = userEvent.setup()
  renderKeys()
  expect(await screen.findByText('Unable to load Keys')).toBeInTheDocument()
  expect(screen.queryByText('0', { exact: true })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByText('Keep using your existing Keys')).toBeInTheDocument()
})
