import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { PrefillGroupManagement } from '../prefill-group-management'

let client: QueryClient | undefined
afterEach(() => {
  cleanup()
  client?.clear()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

const groups = Array.from({ length: 12 }, (_, index) => ({
  id: index + 1,
  name: `Saved group ${String(index + 1).padStart(2, '0')}`,
  description: 'Saved description',
  type: 'model',
  items: ['first', 'second', 'third', 'fourth', 'fifth-very-long-model-name-with-an-extension'],
}))

function renderManagement(response: { success: boolean; data: typeof groups; message?: string } = { success: true, data: groups }) {
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  vi.spyOn(api, 'get').mockResolvedValue({ data: response })
  return render(<QueryClientProvider client={client}><PrefillGroupManagement open onOpenChange={() => {}} /></QueryClientProvider>)
}

it('a business read failure displays the returned message instead of an empty group list', async () => {
  renderManagement({ success: false, data: [], message: 'Synthetic group read failed' })
  expect(await screen.findByRole('alert')).toHaveTextContent('Synthetic group read failed')
  expect(screen.queryByText('No prefill groups yet')).not.toBeInTheDocument()
  expect(screen.queryByText('No matching prefill groups')).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Refresh' })).toBeEnabled()
})

it('expanding saved items exposes every item without truncating long model names', async () => {
  const user = userEvent.setup()
  renderManagement()
  const row = (await screen.findByRole('heading', { name: 'Saved group 01' })).closest('tr')
  if (!row) throw new Error('Expected the saved group to appear in a table row')
  const summary = within(row).getByText('View all items')
  await user.click(summary)
  expect(within(row).getByText('fifth-very-long-model-name-with-an-extension')).toBeVisible()
})

it('a narrow viewport uses the same saved summary and actions in cards with a single body scroll region', async () => {
  vi.stubGlobal('innerWidth', 390)
  const user = userEvent.setup()
  renderManagement()
  const card = await screen.findByRole('article', { name: 'Saved group 01' })
  expect(within(card).getByText('Saved description')).toBeVisible()
  await user.click(within(card).getByText('View all items'))
  expect(within(card).getByText('fifth-very-long-model-name-with-an-extension')).toBeVisible()
  expect(within(card).getByRole('button', { name: 'Edit group "Saved group 01"' })).toHaveClass('min-h-11', 'min-w-11')
  const body = screen.getByRole('region', { name: 'Prefill groups' })
  expect(body).toHaveClass('overflow-y-auto', 'min-h-0')
  expect(body).not.toContainElement(screen.getByRole('navigation', { name: 'Prefill groups pagination' }))
  expect(body).not.toContainElement(screen.getByRole('heading', { name: 'Prefill Group Management' }))
})

it('canceling an editor restores the saved page, search filter, and edit focus', async () => {
  const user = userEvent.setup()
  renderManagement()
  await screen.findByRole('heading', { name: 'Saved group 01' })
  await user.type(screen.getByRole('textbox', { name: 'Search prefill groups' }), 'Saved group')
  await user.click(screen.getByRole('button', { name: 'Next' }))
  await user.click(await screen.findByRole('button', { name: 'Edit group "Saved group 11"' }))
  await screen.findByRole('heading', { name: 'Edit Prefill Group' })
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(await screen.findByRole('textbox', { name: 'Search prefill groups' })).toHaveValue('Saved group')
  expect(screen.getByText('Page 2 of 2')).toBeVisible()
  await waitFor(() => expect(screen.getByRole('button', { name: 'Edit group "Saved group 11"' })).toHaveFocus())
})

it('saving an editor returns to the same filtered page and focuses the saved group', async () => {
  const user = userEvent.setup()
  const put = vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
  renderManagement()
  await screen.findByRole('heading', { name: 'Saved group 01' })
  await user.type(screen.getByRole('textbox', { name: 'Search prefill groups' }), 'Saved group')
  await user.click(screen.getByRole('button', { name: 'Next' }))
  await user.click(await screen.findByRole('button', { name: 'Edit group "Saved group 11"' }))
  await user.click(await screen.findByRole('button', { name: 'Save changes' }))
  expect(await screen.findByRole('textbox', { name: 'Search prefill groups' })).toHaveValue('Saved group')
  expect(screen.getByText('Page 2 of 2')).toBeVisible()
  expect(put).toHaveBeenCalledTimes(1)
  await waitFor(() => expect(screen.getByRole('button', { name: 'Edit group "Saved group 11"' })).toHaveFocus())
})

it('canceling deletion performs no DELETE and a failed deletion keeps the exact target open', async () => {
  const user = userEvent.setup()
  const remove = vi.spyOn(api, 'delete').mockRejectedValue(new Error('Synthetic deletion failed'))
  renderManagement()
  await user.click(await screen.findByRole('button', { name: 'Delete group "Saved group 01"' }))
  let confirmation = await screen.findByRole('alertdialog')
  expect(within(confirmation).getByText(/Saved group 01/)).toBeVisible()
  expect(within(confirmation).getByText('#1')).toBeVisible()
  await user.click(within(confirmation).getByRole('button', { name: 'Cancel' }))
  expect(remove).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Delete group "Saved group 02"' }))
  confirmation = await screen.findByRole('alertdialog')
  await user.click(within(confirmation).getByRole('button', { name: 'Delete' }))
  await screen.findByRole('alert')
  expect(remove).toHaveBeenCalledTimes(1)
  expect(remove.mock.calls[0]?.[0]).toBe('/api/prefill_group/2')
  expect(within(confirmation).getByText(/Saved group 02/)).toBeVisible()
  expect(within(confirmation).getByText('Synthetic deletion failed')).toBeVisible()
  expect(within(confirmation).getByRole('button', { name: 'Cancel' })).toBeEnabled()
})
