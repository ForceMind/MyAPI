import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { ModelsDialogs } from '../../models-dialogs'
import { ModelsPrimaryButtons } from '../../models-primary-buttons'
import { ModelsProvider } from '../../models-provider'

const originalAuth = useAuthStore.getState()
let client: QueryClient
let names: string[]
beforeEach(() => {
  localStorage.clear()
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity }, mutations: { retry: false } } })
  names = Array.from({ length: 61 }, (_, index) => `synthetic-missing-${String(index + 1).padStart(3, '0')}`)
  useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.ADMIN, username: 'synthetic-admin' })
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/models/missing') return { data: { success: true, data: [...names] } }
    if (url === '/api/vendors/') return { data: { success: true, data: { items: [], total: 0 } } }
    throw new Error(`Unexpected synthetic GET ${url}`)
  })
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); localStorage.clear(); vi.restoreAllMocks(); vi.unstubAllGlobals() })

function renderTools() {
  render(<QueryClientProvider client={client}><ModelsProvider><ModelsPrimaryButtons /><ModelsDialogs /></ModelsProvider></QueryClientProvider>)
}
async function openMissing(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'More actions' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Missing Models' }))
  return screen.findByRole('dialog', { name: 'Missing Models' })
}

it('the missing list uses shared last numeric jump and 20/50/100 page-size controls', async () => {
  const user = userEvent.setup()
  renderTools()
  await openMissing(user)
  await screen.findByText('synthetic-missing-001')
  const pager = screen.getByRole('navigation', { name: 'Missing models pagination' })
  await user.click(within(pager).getByRole('button', { name: 'Last page' }))
  expect(screen.getByText('synthetic-missing-061')).toBeVisible()
  expect(screen.queryByText('synthetic-missing-001')).not.toBeInTheDocument()
  await user.click(within(pager).getByRole('button', { name: 'Go to page 2' }))
  expect(screen.getByText('synthetic-missing-021')).toBeVisible()
  await user.type(within(pager).getByRole('textbox', { name: 'Page number' }), '3')
  await user.click(within(pager).getByRole('button', { name: 'Go to page' }))
  expect(screen.getByText('synthetic-missing-041')).toBeVisible()
  await user.selectOptions(within(pager).getByRole('combobox', { name: 'Rows per page' }), '50')
  expect(screen.getByText('synthetic-missing-001')).toBeVisible()
  expect(screen.getByText('synthetic-missing-050')).toBeVisible()
  await user.selectOptions(within(pager).getByRole('combobox', { name: 'Rows per page' }), '100')
  expect(screen.getByText('synthetic-missing-061')).toBeVisible()
  expect(within(pager).getByRole('button', { name: 'First page' })).toBeDisabled()
})

it('canceling configuration restores missing search page size and the exact Configure focus', async () => {
  const post = vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected synthetic POST'))
  const user = userEvent.setup()
  renderTools()
  await openMissing(user)
  await screen.findByText('synthetic-missing-001')
  await user.type(screen.getByRole('textbox', { name: 'Search missing models' }), 'synthetic-missing')
  const pager = screen.getByRole('navigation', { name: 'Missing models pagination' })
  await user.selectOptions(within(pager).getByRole('combobox', { name: 'Rows per page' }), '50')
  await user.click(within(pager).getByRole('button', { name: 'Go to next page' }))
  await user.click(screen.getByRole('button', { name: 'Configure model "synthetic-missing-051"' }))
  const editor = await screen.findByRole('dialog', { name: 'Create Model' })
  expect(await screen.findByLabelText('Model Name *')).toHaveValue('synthetic-missing-051')
  await user.click(within(editor).getByRole('button', { name: 'Cancel' }))
  await screen.findByRole('dialog', { name: 'Missing Models' })
  expect(screen.getByRole('textbox', { name: 'Search missing models' })).toHaveValue('synthetic-missing')
  expect(screen.getByRole('combobox', { name: 'Rows per page' })).toHaveValue('50')
  expect(screen.getByRole('button', { name: 'Go to page 2' })).toHaveAttribute('aria-current', 'page')
  await waitFor(() => expect(screen.getByRole('button', { name: 'Configure model "synthetic-missing-051"' })).toHaveFocus())
  expect(post).not.toHaveBeenCalled()
})

it('saving a configured missing model refreshes missing data and focuses the next reachable row on the same page', async () => {
  const post = vi.spyOn(api, 'post').mockImplementation(async (url, body) => {
    if (url !== '/api/models/') throw new Error(`Unexpected synthetic POST ${url}`)
    expect(body).toEqual(expect.objectContaining({ model_name: 'synthetic-missing-021', status: 1, sync_official: 1 }))
    names = names.filter((name) => name !== 'synthetic-missing-021')
    return { data: { success: true } }
  })
  const put = vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected synthetic option write'))
  const user = userEvent.setup()
  renderTools()
  await openMissing(user)
  await screen.findByText('synthetic-missing-001')
  await user.click(screen.getByRole('button', { name: 'Go to page 2' }))
  await user.click(screen.getByRole('button', { name: 'Configure model "synthetic-missing-021"' }))
  await screen.findByRole('dialog', { name: 'Create Model' })
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await screen.findByRole('dialog', { name: 'Missing Models' })
  await waitFor(() => expect(screen.queryByText('synthetic-missing-021')).not.toBeInTheDocument())
  expect(screen.getByRole('button', { name: 'Go to page 2' })).toHaveAttribute('aria-current', 'page')
  await waitFor(() => expect(screen.getByRole('button', { name: 'Configure model "synthetic-missing-022"' })).toHaveFocus())
  expect(post).toHaveBeenCalledTimes(1)
  expect(put).not.toHaveBeenCalled()
})

it('a phone missing list exposes the entire model name and copies the exact name from a 44px action', async () => {
  vi.stubGlobal('innerWidth', 390)
  const modelName = 'synthetic/model-with-an-extremely-long-complete-name-and-a-final-suffix'
  names = [modelName]
  const user = userEvent.setup()
  const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
  renderTools()
  await openMissing(user)
  const body = screen.getByRole('region', { name: 'Missing model results' })
  expect(await within(body).findByText(modelName)).toBeVisible()
  expect(body).toHaveClass('min-h-0', 'overflow-y-auto')
  const copyAction = screen.getByRole('button', { name: `Copy model name "${modelName}"` })
  expect(copyAction).toHaveClass('min-h-11', 'min-w-11')
  await user.click(copyAction)
  expect(copy).toHaveBeenCalledWith(modelName)
  expect(body).not.toContainElement(screen.getByRole('navigation', { name: 'Missing models pagination' }))
})

it('changing identities closes configuration and clears the saved missing-list context before a fresh open', async () => {
  const user = userEvent.setup()
  renderTools()
  await openMissing(user)
  await screen.findByText('synthetic-missing-001')
  await user.type(screen.getByRole('textbox', { name: 'Search missing models' }), 'synthetic-missing')
  await user.selectOptions(screen.getByRole('combobox', { name: 'Rows per page' }), '50')
  await user.click(screen.getByRole('button', { name: 'Configure model "synthetic-missing-001"' }))
  await screen.findByRole('dialog', { name: 'Create Model' })
  await act(async () => { useAuthStore.getState().auth.setUser({ id: 2, role: ROLE.ADMIN, username: 'synthetic-new-admin' }) })
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  await openMissing(user)
  await screen.findByText('synthetic-missing-001')
  expect(screen.getByRole('textbox', { name: 'Search missing models' })).toHaveValue('')
  expect(screen.getByRole('combobox', { name: 'Rows per page' })).toHaveValue('20')
})

it('a late missing read from the previous identity cannot populate the new identity list', async () => {
  let complete: ((value: { data: { success: boolean; data: string[] } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean; data: string[] } }>((resolve) => { complete = resolve })
  const get = vi.mocked(api.get)
  get.mockImplementation(async (url) => {
    if (url !== '/api/models/missing') throw new Error(`Unexpected synthetic GET ${url}`)
    if (useAuthStore.getState().auth.user?.id === 1) return pending
    return { data: { success: true, data: ['synthetic-new-identity-model'] } }
  })
  const user = userEvent.setup()
  renderTools()
  await openMissing(user)
  await waitFor(() => expect(get).toHaveBeenCalledTimes(1))
  await act(async () => { useAuthStore.getState().auth.setUser({ id: 2, role: ROLE.ADMIN, username: 'synthetic-new-admin' }) })
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  await openMissing(user)
  expect(await screen.findByText('synthetic-new-identity-model')).toBeVisible()
  await act(async () => { complete?.({ data: { success: true, data: ['synthetic-old-identity-model'] } }); await pending })
  expect(screen.getByText('synthetic-new-identity-model')).toBeVisible()
  expect(screen.queryByText('synthetic-old-identity-model')).not.toBeInTheDocument()
})
