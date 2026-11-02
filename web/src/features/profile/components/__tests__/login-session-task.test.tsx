import { QueryClient } from '@tanstack/react-query'
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { LoginSessionsCard } from '../login-sessions-card'
import { LoginSessionDialogs } from '../login-session-dialogs'

const originalAuth = useAuthStore.getState()
let client: QueryClient | undefined
afterEach(() => { cleanup(); client?.clear(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })

it('a failed revoke retains an accurate device confirmation without exposing its full session ID', async () => {
  const sid = 'synthetic-session-identifier-not-for-display'
  useAuthStore.getState().auth.setUser({ id: 42, role: 1, username: 'synthetic-owner' })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url !== '/api/user/sessions') throw new Error(`Unexpected synthetic GET ${url}`)
    return { data: { success: true, data: [{ sid, current: false, login_method: 'password', ip: '203.0.113.12', user_agent: 'Mozilla/5.0 (X11; Linux x86_64) Chrome/120.0', created_at: 1, last_active_at: 2, expires_at: 4_000_000_000 }] } }
  })
  const revoke = vi.spyOn(api, 'delete').mockResolvedValue({ data: { success: false, message: `Synthetic revoke failed (${sid})` } })
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  const user = userEvent.setup()
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  await renderListRoute({ client, path: '/profile', element: <LoginSessionsCard securityScopeKey='synthetic-owner-sessions' /> })
  await user.click(await screen.findByRole('button', { name: 'Revoke' }))
  const confirmation = await screen.findByRole('alertdialog', { name: 'Revoke session?' })
  expect(within(confirmation).getByText('203.0.113.12')).toBeVisible()
  expect(confirmation).not.toHaveTextContent(sid)
  await user.click(within(confirmation).getByRole('button', { name: 'Revoke' }))
  expect(await within(confirmation).findByRole('alert')).toHaveTextContent('Synthetic revoke failed')
  expect(confirmation).not.toHaveTextContent(sid)
  expect(within(confirmation).getByText('203.0.113.12')).toBeVisible()
  expect(revoke).toHaveBeenCalledTimes(1)
  expect(revoke.mock.calls[0]?.[0]).toBe(`/api/user/sessions/${sid}`)
})

it.each([false, true])('a pending session confirmation preserves its target when Escape is pressed (other sessions=%s)', async (others) => {
  const target = { sid: 'synthetic-private-session-id', current: false, login_method: 'password', ip: '203.0.113.12', user_agent: 'Synthetic browser', created_at: 1, last_active_at: 2, expires_at: 4_000_000_000 }
  const changeTarget = vi.fn()
  const changeOthers = vi.fn()
  const user = userEvent.setup()
  render(<LoginSessionDialogs revokeTarget={others ? null : target} confirmOthers={others} revoking={!others} revokingOthers={others} onRevokeTargetChange={changeTarget} onConfirmOthersChange={changeOthers} onRevoke={() => {}} onRevokeOthers={() => {}} />)
  await user.keyboard('{Escape}')
  expect(changeTarget).not.toHaveBeenCalled()
  expect(changeOthers).not.toHaveBeenCalled()
  expect(screen.queryByText(target.sid)).not.toBeInTheDocument()
})
