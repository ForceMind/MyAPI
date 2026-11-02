import { QueryClient } from '@tanstack/react-query'
import { act, cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'

import { DeploymentsTable } from '../deployments-table'

let client: QueryClient
let total: number | undefined
let failRead: boolean

beforeEach(() => {
  localStorage.clear()
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  total = 1
  failRead = false
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/deployments/' || url === '/api/deployments/search') {
      if (failRead) throw new Error('Synthetic deployment list read failed')
      return { data: { success: true, data: { total, items: [{ id: 17, container_name: 'synthetic-deployment', status: 'running', provider: 'synthetic-provider', hardware_name: 'Synthetic complete GPU', hardware_quantity: 2, created_at: 1730000000, time_remaining: '1 hour' }] } } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected deployment write'))
  vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected deployment write'))
  vi.spyOn(api, 'delete').mockRejectedValue(new Error('Unexpected deployment write'))
})

afterEach(() => {
  cleanup()
  client.clear()
  localStorage.clear()
  vi.restoreAllMocks()
})

it('keeps an unavailable deployment total unknown while retaining the returned deployment', async () => {
  total = undefined
  await renderListRoute({ client, path: '/models/$section', entry: '/models/deployments', element: <DeploymentsTable /> })
  await screen.findByText('synthetic-deployment')
  const pager = screen.getByRole('navigation', { name: 'Deployment pagination' })
  expect(within(pager).getByText('Record total unavailable')).toBeVisible()
  expect(within(pager).getByRole('button', { name: 'Last page' })).toBeDisabled()
})

it('exposes deployment ID hardware and creation time through a reachable phone disclosure', async () => {
  const original = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({ ...original(query), matches: query === '(max-width: 640px)' }))
  await renderListRoute({ client, path: '/models/$section', entry: '/models/deployments', element: <DeploymentsTable /> })
  await screen.findByText('synthetic-deployment')
  const user = userEvent.setup()
  await user.click(screen.getByText('More details'))
  expect(screen.getByText('ID')).toBeVisible()
  expect(screen.getByText('Hardware')).toBeVisible()
  expect(screen.getByText('Synthetic complete GPU')).toBeVisible()
  expect(screen.getByText('Created')).toBeVisible()
})

it('replaces saved deployment rows and totals with an explicit error when a reread fails', async () => {
  await renderListRoute({ client, path: '/models/$section', entry: '/models/deployments', element: <DeploymentsTable /> })
  await screen.findByText('synthetic-deployment')
  failRead = true
  await act(async () => { await client.refetchQueries() })
  expect(await screen.findByRole('alert')).toHaveTextContent('Failed to load deployments')
  expect(screen.queryByText('synthetic-deployment')).not.toBeInTheDocument()
  expect(api.delete).not.toHaveBeenCalled()
})

it('keeps the named deletion confirmation and failure visible when the deployment delete fails', async () => {
  vi.mocked(api.delete).mockRejectedValue(new Error('Synthetic deployment deletion failed'))
  await renderListRoute({ client, path: '/models/$section', entry: '/models/deployments', element: <DeploymentsTable /> })
  await screen.findByText('synthetic-deployment')
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Open menu' }))
  await user.click(screen.getByRole('menuitem', { name: 'Delete' }))
  const confirm = await screen.findByRole('alertdialog')
  expect(confirm).toHaveTextContent('synthetic-deployment')
  await user.click(within(confirm).getByRole('button', { name: 'Delete' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Synthetic deployment deletion failed')
  expect(screen.getByRole('alertdialog')).toHaveTextContent('synthetic-deployment')
  expect(api.delete).toHaveBeenCalledTimes(1)
})

it('keeps a pending deletion confirmation open and blocks repeated confirmation or Escape while the one write is unresolved', async () => {
  let complete: ((value: { data: { success: boolean } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean } }>((resolve) => { complete = resolve })
  vi.mocked(api.delete).mockReturnValue(pending)
  await renderListRoute({ client, path: '/models/$section', entry: '/models/deployments', element: <DeploymentsTable /> })
  await screen.findByText('synthetic-deployment')
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Open menu' }))
  await user.click(screen.getByRole('menuitem', { name: 'Delete' }))
  const confirm = await screen.findByRole('alertdialog')
  const submit = within(confirm).getByRole('button', { name: 'Delete' })
  fireEvent.click(submit)
  fireEvent.click(submit)
  await waitFor(() => expect(api.delete).toHaveBeenCalledTimes(1))
  await user.keyboard('{Escape}')
  expect(screen.getByRole('alertdialog')).toHaveTextContent('synthetic-deployment')
  expect(within(confirm).getByRole('button', { name: 'Cancel' })).toBeDisabled()
  await act(async () => { complete?.({ data: { success: true } }); await pending })
  await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
})
