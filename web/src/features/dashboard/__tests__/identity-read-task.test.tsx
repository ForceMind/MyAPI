import { QueryClient } from '@tanstack/react-query'
import { act, cleanup, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { DirectionProvider } from '@/context/direction-provider'
import { ThemeProvider } from '@/context/theme-provider'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { Dashboard } from '../index'

vi.mock('@visactor/react-vchart', () => ({ VChart: () => <div data-testid='analysis-chart' /> }))
vi.mock('@visactor/vchart', () => ({ ThemeManager: { setCurrentTheme: vi.fn() } }))

const originalAuth = useAuthStore.getState()
let client: QueryClient
function setSession(sid: string) {
  useAuthStore.setState((state) => ({ auth: { ...state.auth, user: { id: 71, role: 100, username: 'synthetic-admin' }, session: { sid, current: true, login_method: 'password', ip: '192.0.2.1', user_agent: 'Synthetic browser', created_at: 1, last_active_at: 1, expires_at: 9999999999 } } }))
}
beforeEach(() => { client = new QueryClient({ defaultOptions: { queries: { retry: false } } }); setSession('synthetic-old-session'); localStorage.clear() })
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); localStorage.clear(); vi.restoreAllMocks() })
it('does not publish a deferred model statistics response after the same user has replaced the login session', async () => {
  let finish: ((value: { data: { success: boolean; data: Array<{ created_at: number; count: number; token_used: number; quota: number }> } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean; data: Array<{ created_at: number; count: number; token_used: number; quota: number }> } }>((resolve) => { finish = resolve })
  let modelReads = 0
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/data') { modelReads += 1; return modelReads === 1 ? pending : { data: { success: true, data: [] } } }
    if (url === '/api/perf-metrics/summary') return { data: { success: true, data: { models: [] } } }
    throw new Error(`Unexpected synthetic analytics GET ${url}`)
  })
  await renderListRoute({ client, path: '/dashboard/$section', entry: '/dashboard/models', element: <DirectionProvider><ThemeProvider><Dashboard /></ThemeProvider></DirectionProvider> })
  await waitFor(() => expect(modelReads).toBe(1))
  await act(async () => { setSession('synthetic-new-session') })
  await waitFor(() => expect(modelReads).toBe(2))
  await act(async () => { finish?.({ data: { success: true, data: [{ created_at: 1, count: 918273, token_used: 876543, quota: 123 }] } }); await pending })
  expect(screen.queryByTitle('918,273')).not.toBeInTheDocument()
  expect(screen.queryByTitle('876,543')).not.toBeInTheDocument()
  expect(screen.getByText('Total Count')).toBeVisible()
  expect(modelReads).toBe(2)
})
