import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import type { SyncDiffData } from '../../../types'
import { ModelsProvider, useModels } from '../../models-provider'
import { UpstreamConflictDialog } from '../upstream-conflict-dialog'

const originalAuth = useAuthStore.getState()
let client: QueryClient
const conflicts = Array.from({ length: 6 }, (_, index) => ({
  model_name: `synthetic-model-${index + 1}-with-a-complete-long-name`,
  fields: [
    { field: 'description', local: `local description ${index + 1}`, upstream: `upstream description ${index + 1}` },
    { field: 'tags', local: ['local'], upstream: ['upstream'] },
  ],
}))
beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.ADMIN, username: 'synthetic-admin' })
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); vi.restoreAllMocks(); vi.unstubAllGlobals() })

function ConflictTools({ entries }: { entries: SyncDiffData['conflicts'] }) {
  const { open, setOpen, setUpstreamConflicts } = useModels()
  return <>
    <button onClick={() => { setUpstreamConflicts(entries); setOpen('upstream-conflict') }}>Open conflicts</button>
    <UpstreamConflictDialog open={open === 'upstream-conflict'} onOpenChange={(next) => !next && setOpen(null)} />
  </>
}
function renderTools(entries = conflicts) {
  render(<QueryClientProvider client={client}><ModelsProvider><ConflictTools entries={entries} /></ModelsProvider></QueryClientProvider>)
}

it('select all from one conflict page submits every conflict field with an explicit all conflicts scope', async () => {
  const post = vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true } })
  const user = userEvent.setup()
  renderTools()
  await user.click(screen.getByRole('button', { name: 'Open conflicts' }))
  await user.click(screen.getByRole('checkbox', { name: 'Select all conflict fields', exact: true }))
  expect(screen.getByText('12 selected fields across all pages')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Apply Overwrite' }))
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
  expect(post.mock.calls[0]?.[1]).toEqual({
    locale: 'zh', source: 'official',
    overwrite: conflicts.map((entry) => ({ model_name: entry.model_name, fields: ['description', 'tags'] })),
  })
})

it('field search preserves all fields of matching models and keeps selections across pages explicit', async () => {
  const user = userEvent.setup()
  renderTools()
  await user.click(screen.getByRole('button', { name: 'Open conflicts' }))
  await user.click(screen.getAllByRole('checkbox', { name: 'Select row' })[1]!)
  await user.type(screen.getByRole('textbox', { name: 'Search conflicting models or fields' }), 'description')
  await waitFor(() => expect(screen.getByText('Showing 1–10 of 12 fields')).toBeVisible())
  expect(screen.getByText('1 selected fields across all pages')).toBeVisible()
  expect(screen.getAllByText('Tags')).toHaveLength(5)
  await user.click(screen.getByRole('button', { name: 'Clear selection' }))
  expect(screen.getByText('0 selected fields across all pages')).toBeVisible()
})

it('a narrow conflict view exposes complete local and upstream values within one scroll region', async () => {
  vi.stubGlobal('innerWidth', 390)
  const user = userEvent.setup()
  renderTools(conflicts.slice(0, 1))
  await user.click(screen.getByRole('button', { name: 'Open conflicts' }))
  const region = screen.getByRole('region', { name: 'Conflicting fields' })
  expect(region).toHaveClass('overflow-y-auto', 'min-h-0')
  expect(within(region).getByText('local description 1')).toBeVisible()
  expect(within(region).getByText('upstream description 1')).toBeVisible()
  expect(within(region).getAllByText(conflicts[0]!.model_name)[0]).toBeVisible()
  expect(region).not.toContainElement(screen.getByRole('navigation', { name: 'Conflict pagination' }))
  expect(region).not.toContainElement(screen.getByRole('button', { name: 'Apply Overwrite' }))
})

it('a late overwrite result cannot close a newly opened conflict task after closing the submitted task', async () => {
  let complete: ((value: { data: { success: boolean } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean } }>((resolve) => { complete = resolve })
  const post = vi.spyOn(api, 'post').mockReturnValue(pending)
  const user = userEvent.setup()
  renderTools(conflicts.slice(0, 1))
  await user.click(screen.getByRole('button', { name: 'Open conflicts' }))
  await user.click(screen.getAllByRole('checkbox', { name: 'Select row' })[0]!)
  await user.click(screen.getByRole('button', { name: 'Apply Overwrite' }))
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  await user.click(screen.getByRole('button', { name: 'Open conflicts' }))
  await user.click(screen.getAllByRole('checkbox', { name: 'Select row' })[1]!)
  await act(async () => { complete?.({ data: { success: true } }); await pending })
  expect(screen.getByRole('dialog', { name: 'Resolve Conflicts' })).toBeVisible()
  expect(screen.getAllByRole('checkbox', { name: 'Select row' })[1]).toBeChecked()
  expect(post).toHaveBeenCalledTimes(1)
})
