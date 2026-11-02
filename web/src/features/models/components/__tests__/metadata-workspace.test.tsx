import { QueryClient } from '@tanstack/react-query'
import { act, cleanup, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { ROLE } from '@/lib/roles'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import type { Model } from '../../types'
import { ModelsDialogs } from '../models-dialogs'
import { ModelsPrimaryButtons } from '../models-primary-buttons'
import { ModelsProvider } from '../models-provider'
import { ModelsTable } from '../models-table'

const item: Model = { id: 24, model_name: 'synthetic-metadata-model', description: 'Saved complete model description', status: 1, sync_official: 0, created_time: 1, updated_time: 2, name_rule: 0, tags: 'tag-one,tag-two', endpoints: '{"openai":{"path":"/v1/chat/completions","method":"POST"}}', bound_channels: [{ name: 'Saved connection', type: 1 }], enable_groups: ['saved-group'], quota_types: [0] }
const originalAuth = useAuthStore.getState()
const originalConfig = useSystemConfigStore.getState()
let client: QueryClient
let rows: Model[]
let total: unknown

beforeEach(() => {
  localStorage.clear()
  rows = [item]
  total = 1
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.ADMIN, username: 'synthetic-admin' })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/status') return { data: { success: true, data: {} } }
    if (url === '/api/vendors/') return { data: { success: true, data: { items: [], total: 0 } } }
    if (url === '/api/models/' || url === '/api/models/search') return { data: { success: true, data: { items: rows, total, vendor_counts: { all: total } } } }
    if (url === '/api/models/24') return { data: { success: true, data: item } }
    throw new Error(`Unexpected GET ${url}`)
  })
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); useSystemConfigStore.setState(originalConfig); localStorage.clear(); vi.restoreAllMocks() })

it('exposes complete metadata and runtime fields through a reachable phone disclosure', async () => {
  const original = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({ ...original(query), matches: query === '(max-width: 640px)' }))
  await renderListRoute({ client, path: '/models/$section', entry: '/models/metadata', element: <ModelsProvider><ModelsTable /></ModelsProvider> })
  await screen.findByText(item.model_name)
  const user = userEvent.setup()
  await user.click(screen.getByText('More details'))
  expect(screen.getByText('ID')).toBeVisible()
  expect(screen.getByText('Bound Channels')).toBeVisible()
  expect(screen.getByText('Quota Types')).toBeVisible()
  expect(screen.getByText('Saved complete model description')).toBeVisible()
  expect(screen.getByText('saved-group')).toBeVisible()
  expect(screen.getByText('Updated')).toBeVisible()
})

async function renderWorkspace(entry = '/models/metadata') {
  return renderListRoute({ client, path: '/models/$section', entry, element: <ModelsProvider><ModelsPrimaryButtons /><ModelsTable /><ModelsDialogs /></ModelsProvider> })
}

it('keeps unavailable totals unknown and disables page navigation without clearing returned rows', async () => {
  total = undefined
  await renderWorkspace()
  await screen.findByText(item.model_name)
  const pager = screen.getByRole('navigation', { name: 'Model metadata pagination' })
  expect(within(pager).getByText('Record total unavailable')).toBeVisible()
  expect(within(pager).getByRole('button', { name: 'Last page' })).toBeDisabled()
})

it('uses shared last-page controls and resets to page one when page size changes', async () => {
  total = 61
  const user = userEvent.setup()
  const { router } = await renderWorkspace('/models/metadata?pageSize=20')
  await screen.findByText(item.model_name)
  const pager = screen.getByRole('navigation', { name: 'Model metadata pagination' })
  await user.click(within(pager).getByRole('button', { name: 'Last page' }))
  await waitFor(() => expect(router.state.location.search.page).toBe(4))
  expect(api.get).toHaveBeenCalledWith('/api/models/', { params: { p: 4, page_size: 20 } })
  await user.selectOptions(within(pager).getByRole('combobox', { name: 'Rows per page' }), '50')
  await waitFor(() => expect(router.state.location.search).toMatchObject({ page: 1, pageSize: 50 }))
})

it('retains list context and returns focus to the edit action after closing the editor', async () => {
  total = 100
  const user = userEvent.setup()
  const { router } = await renderWorkspace('/models/metadata?page=3&pageSize=20&filter=synthetic&status=%5B%22enabled%22%5D')
  await screen.findByText(item.model_name)
  const before = router.state.location.search
  const edit = screen.getByRole('button', { name: 'Edit' })
  await user.click(edit)
  const dialog = await screen.findByRole('dialog')
  await screen.findByDisplayValue(item.model_name)
  expect(within(dialog).queryByRole('button', { name: 'Pricing' })).not.toBeInTheDocument()
  expect(api.get).not.toHaveBeenCalledWith('/api/option/')
  await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  await waitFor(() => expect(screen.getByRole('button', { name: 'Edit' })).toHaveFocus())
  expect(router.state.location.search).toEqual(before)
})

it('shows the full description and restores its action after a saved list refresh', async () => {
  const original = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({ ...original(query), matches: query === '(max-width: 640px)' }))
  const user = userEvent.setup()
  await renderWorkspace()
  await screen.findByText(item.model_name)
  await user.click(screen.getByText('More details'))
  const description = screen.getByRole('button', { name: `View description for ${item.model_name}` })
  await user.click(description)
  const dialog = await screen.findByRole('dialog')
  expect(within(dialog).getByText(item.description ?? '')).toBeVisible()
  rows = [{ ...item, updated_time: 8 }]
  await act(async () => { await client.invalidateQueries({ queryKey: ['models', 'list'] }) })
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  await waitFor(() => expect(screen.getByRole('button', { name: `View description for ${item.model_name}` })).toHaveFocus())
})

it('keeps selected model IDs separate when a different server page returns another row', async () => {
  total = 40
  const user = userEvent.setup()
  await renderWorkspace('/models/metadata?pageSize=20')
  await screen.findByText(item.model_name)
  await user.click(screen.getByRole('checkbox', { name: 'Select row' }))
  expect(screen.getByRole('button', { name: 'Delete selected models' })).toBeEnabled()
  rows = [{ ...item, id: 25, model_name: 'second-page-model' }]
  await user.click(screen.getByRole('button', { name: 'Go to next page' }))
  await screen.findByText('second-page-model')
  expect(screen.getByRole('checkbox', { name: 'Select row' })).not.toBeChecked()
  expect(screen.queryByRole('button', { name: 'Delete selected models' })).not.toBeInTheDocument()
})
