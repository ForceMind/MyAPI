import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { SystemInstancesPanel } from '../system-instances-panel'
import { SystemTasksPanel } from '../system-tasks-panel'

const originalAuth = useAuthStore.getState()
let client: QueryClient
beforeEach(() => { client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } }); useAuthStore.getState().auth.setUser({ id: 1, role: 100, username: 'synthetic-root' }) })
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })

it('missing task progress remains indeterminate while actual zero and positive progress retain their exact ARIA values', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url !== '/api/system-task/list') throw new Error(`Unexpected synthetic runtime GET ${url}`)
    return { data: { success: true, data: [{ task_id: 'synthetic-unknown-id', type: 'synthetic-unknown-progress', status: 'running', updated_at: 1, state: {} }, { task_id: 'synthetic-zero-id', type: 'synthetic-zero-progress', status: 'running', updated_at: 1, state: { progress: 0 } }, { task_id: 'synthetic-positive-id', type: 'synthetic-positive-progress', status: 'running', updated_at: 1, state: { progress: 35 } }] } }
  })
  render(<QueryClientProvider client={client}><SystemTasksPanel /></QueryClientProvider>)
  const unknown = await screen.findByRole('row', { name: /synthetic-unknown-progress/ })
  expect(within(unknown).getByRole('progressbar')).not.toHaveAttribute('aria-valuenow')
  expect(within(screen.getByRole('row', { name: /synthetic-zero-progress/ })).getByRole('progressbar')).toHaveAttribute('aria-valuenow', '0')
  expect(within(screen.getByRole('row', { name: /synthetic-positive-progress/ })).getByRole('progressbar')).toHaveAttribute('aria-valuenow', '35')
})
it('a stale-instance confirmation identifies its exact node even when another instance has the same friendly name and cancelling deletes nothing', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url !== '/api/system-info/instances') throw new Error(`Unexpected synthetic runtime GET ${url}`)
    return { data: { success: true, data: ['a', 'b'].map((suffix) => ({ node_name: `synthetic-node-${suffix}`, status: 'stale', stale_after_seconds: 90, started_at: 1, last_seen_at: 1, info: { node: { name: 'Shared synthetic friendly name' }, host: { hostname: `host-${suffix}.synthetic.invalid` } } })) } }
  })
  const remove = vi.spyOn(api, 'delete').mockRejectedValue(new Error('Unexpected synthetic instance deletion'))
  const user = userEvent.setup()
  render(<QueryClientProvider client={client}><SystemInstancesPanel /></QueryClientProvider>)
  const row = await screen.findByRole('row', { name: /host-b.synthetic.invalid/ })
  await user.click(within(row).getByRole('button', { name: 'Delete stale instance' }))
  const confirmation = await screen.findByRole('alertdialog', { name: 'Delete stale instance' })
  expect(within(confirmation).getByText('synthetic-node-b')).toBeVisible()
  expect(within(confirmation).getByText('host-b.synthetic.invalid')).toBeVisible()
  await user.click(within(confirmation).getByRole('button', { name: 'Cancel' }))
  expect(remove).not.toHaveBeenCalled()
})
