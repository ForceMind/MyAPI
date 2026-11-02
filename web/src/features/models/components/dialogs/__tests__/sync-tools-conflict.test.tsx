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
    <button type='button' onClick={() => { setUpstreamConflicts(entries); setOpen('upstream-conflict') }}>Open conflicts</button>
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
  const allFields = screen.getByRole('checkbox', { name: 'Select all conflict fields' })
  allFields.focus()
  await user.keyboard('[Space]')
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
  const tagsField = screen.getAllByRole('checkbox', { name: 'Select row' }).at(1)
  if (!tagsField) throw new Error('Expected the second field selection control')
  await user.click(tagsField)
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
  expect(within(region).getAllByText('synthetic-model-1-with-a-complete-long-name').at(0)).toBeVisible()
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
  const descriptionField = screen.getAllByRole('checkbox', { name: 'Select row' }).at(0)
  if (!descriptionField) throw new Error('Expected the first field selection control')
  await user.click(descriptionField)
  await user.click(screen.getByRole('button', { name: 'Apply Overwrite' }))
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  await user.click(screen.getByRole('button', { name: 'Open conflicts' }))
  const reopenedTagsField = screen.getAllByRole('checkbox', { name: 'Select row' }).at(1)
  if (!reopenedTagsField) throw new Error('Expected the reopened second field selection control')
  await user.click(reopenedTagsField)
  expect(screen.getByRole('dialog', { name: 'Resolve Conflicts' })).toBeVisible()
  expect(reopenedTagsField).toBeChecked()
  expect(screen.getByRole('button', { name: 'Apply Overwrite' })).toBeEnabled()
  await act(async () => { complete?.({ data: { success: true } }); await pending })
  expect(screen.getByRole('dialog', { name: 'Resolve Conflicts' })).toBeVisible()
  expect(screen.getAllByRole('checkbox', { name: 'Select row' })[1]).toBeChecked()
  expect(post).toHaveBeenCalledTimes(1)
})

it('a failed overwrite keeps the exact independently selected field and retries only that field', async () => {
  const post = vi.spyOn(api, 'post').mockResolvedValueOnce({ data: { success: false, message: 'Synthetic overwrite failed' } }).mockResolvedValue({ data: { success: true } })
  const user = userEvent.setup()
  renderTools(conflicts.slice(0, 1))
  await user.click(screen.getByRole('button', { name: 'Open conflicts' }))
  const tagsField = screen.getAllByRole('checkbox', { name: 'Select row' }).at(1)
  if (!tagsField) throw new Error('Expected the tags field selection control')
  await user.click(tagsField)
  await user.click(screen.getByRole('button', { name: 'Apply Overwrite' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Synthetic overwrite failed')
  expect(tagsField).toBeChecked()
  expect(screen.getAllByRole('checkbox', { name: 'Select row' }).at(0)).not.toBeChecked()
  await user.click(screen.getByRole('button', { name: 'Apply Overwrite' }))
  await waitFor(() => expect(post).toHaveBeenCalledTimes(2))
  const expectedPayload = { locale: 'zh', source: 'official', overwrite: [{ model_name: 'synthetic-model-1-with-a-complete-long-name', fields: ['tags'] }] }
  expect(post.mock.calls[0]?.[1]).toEqual(expectedPayload)
  expect(post.mock.calls[1]?.[1]).toEqual(expectedPayload)
})

it('an empty conflict task displays a readable empty state and cannot submit', async () => {
  const post = vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected synthetic sync write'))
  const user = userEvent.setup()
  renderTools([])
  await user.click(screen.getByRole('button', { name: 'Open conflicts' }))
  expect(screen.getByText('No conflict entries available.')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Apply Overwrite' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Next page' })).toBeDisabled()
  expect(post).not.toHaveBeenCalled()
})

it('searching to another page keeps hidden selected fields explicit and select all includes hidden conflicts', async () => {
  const post = vi.spyOn(api, 'post').mockResolvedValue({ data: { success: true } })
  const user = userEvent.setup()
  renderTools()
  await user.click(screen.getByRole('button', { name: 'Open conflicts' }))
  const firstField = screen.getAllByRole('checkbox', { name: 'Select row' }).at(0)
  if (!firstField) throw new Error('Expected the first field selection control')
  await user.click(firstField)
  await user.type(screen.getByRole('textbox', { name: 'Search conflicting models or fields' }), 'synthetic-model-6')
  await waitFor(() => expect(screen.getByText('Showing 1–2 of 2 fields')).toBeVisible())
  expect(screen.getByText('1 selected fields outside this page')).toBeVisible()
  await user.click(screen.getByRole('checkbox', { name: 'Select all conflict fields' }))
  expect(screen.getByText('12 selected fields across all pages')).toBeVisible()
  expect(screen.getByText('10 selected fields outside this page')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Apply Overwrite' }))
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
  expect(post.mock.calls[0]?.[1]).toEqual({ locale: 'zh', source: 'official', overwrite: conflicts.map((entry) => ({ model_name: entry.model_name, fields: ['description', 'tags'] })) })
})
