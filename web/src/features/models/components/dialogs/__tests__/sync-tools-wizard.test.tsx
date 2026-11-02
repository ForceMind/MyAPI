import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { ModelsProvider, useModels } from '../../models-provider'
import { SyncWizardDialog } from '../sync-wizard-dialog'
import { UpstreamConflictDialog } from '../upstream-conflict-dialog'

const originalAuth = useAuthStore.getState()
let client: QueryClient
beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.ADMIN, username: 'synthetic-admin' })
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })

function SyncTools() {
  const { open, setOpen } = useModels()
  return <>
    <button onClick={() => setOpen('sync-wizard')}>Open sync</button>
    <SyncWizardDialog open={open === 'sync-wizard'} onOpenChange={(next) => !next && setOpen(null)} />
    <UpstreamConflictDialog open={open === 'upstream-conflict'} onOpenChange={(next) => !next && setOpen(null)} />
  </>
}
function renderTools() {
  render(<QueryClientProvider client={client}><ModelsProvider><SyncTools /></ModelsProvider></QueryClientProvider>)
}

it('selecting Japanese survives a rerender and sends the selected locale to preview and sync', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: { conflicts: [] } } })
  const post = vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true, data: {} } })
  const user = userEvent.setup()
  renderTools()
  await user.click(screen.getByRole('button', { name: 'Open sync' }))
  expect(get).not.toHaveBeenCalled()
  expect(post).not.toHaveBeenCalled()
  await user.click(screen.getByRole('radio', { name: 'Japanese' }))
  expect(screen.getByRole('radio', { name: 'Japanese' })).toBeChecked()
  await user.click(screen.getByRole('button', { name: 'Sync Now' }))
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
  expect(get.mock.calls[0]?.[0]).toBe('/api/models/sync_upstream/preview?locale=ja&source=official')
  expect(post.mock.calls[0]?.[1]).toEqual({ locale: 'ja', source: 'official' })
})

it('closing while preview is pending prevents a later empty preview from submitting a write', async () => {
  let complete: ((value: { data: { success: boolean; data: { conflicts: [] } } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean; data: { conflicts: [] } } }>((resolve) => { complete = resolve })
  const get = vi.spyOn(api, 'get').mockReturnValue(pending)
  const post = vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true } })
  const user = userEvent.setup()
  renderTools()
  await user.click(screen.getByRole('button', { name: 'Open sync' }))
  await user.click(screen.getByRole('button', { name: 'Sync Now' }))
  await waitFor(() => expect(get).toHaveBeenCalledTimes(1))
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  await act(async () => { complete?.({ data: { success: true, data: { conflicts: [] } } }); await pending })
  expect(post).not.toHaveBeenCalled()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})

it('closing while preview is pending prevents late conflicts from reopening a dialog', async () => {
  const conflicts = [{ model_name: 'synthetic-model', fields: [{ field: 'description', local: 'Local', upstream: 'Upstream' }] }]
  let complete: ((value: { data: { success: boolean; data: { conflicts: typeof conflicts } } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean; data: { conflicts: typeof conflicts } } }>((resolve) => { complete = resolve })
  const get = vi.spyOn(api, 'get').mockReturnValue(pending)
  const post = vi.spyOn(api, 'post')
  const user = userEvent.setup()
  renderTools()
  await user.click(screen.getByRole('button', { name: 'Open sync' }))
  await user.click(screen.getByRole('button', { name: 'Sync Now' }))
  await waitFor(() => expect(get).toHaveBeenCalledTimes(1))
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  await act(async () => { complete?.({ data: { success: true, data: { conflicts } } }); await pending })
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(post).not.toHaveBeenCalled()
})

it('switching identities while preview is pending prevents the old identity from submitting a write', async () => {
  let complete: ((value: { data: { success: boolean; data: { conflicts: [] } } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean; data: { conflicts: [] } } }>((resolve) => { complete = resolve })
  const get = vi.spyOn(api, 'get').mockReturnValue(pending)
  const post = vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true } })
  const user = userEvent.setup()
  renderTools()
  await user.click(screen.getByRole('button', { name: 'Open sync' }))
  await user.click(screen.getByRole('button', { name: 'Sync Now' }))
  await waitFor(() => expect(get).toHaveBeenCalledTimes(1))
  await act(async () => { useAuthStore.getState().auth.setUser({ id: 2, role: ROLE.ADMIN, username: 'synthetic-new-admin' }) })
  await act(async () => { complete?.({ data: { success: true, data: { conflicts: [] } } }); await pending })
  expect(post).not.toHaveBeenCalled()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})
