import { QueryClient } from '@tanstack/react-query'
import { act, cleanup, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { ModelsDialogs } from '../models-dialogs'
import { ModelsPrimaryButtons } from '../models-primary-buttons'
import { ModelsProvider } from '../models-provider'

const originalAuth = useAuthStore.getState()
let client: QueryClient
beforeEach(() => {
  localStorage.clear()
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.ADMIN, username: 'synthetic-admin' })
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/prefill_group') return { data: { success: true, data: [] } }
    throw new Error(`Unexpected GET ${url}`)
  })
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); localStorage.clear(); vi.restoreAllMocks() })

async function entry() {
  return renderListRoute({ client, path: '/models/$section', entry: '/models/metadata?page=3&pageSize=20&filter=synthetic', element: <ModelsProvider><ModelsPrimaryButtons /><ModelsDialogs /></ModelsProvider> })
}

it('opens vendor creation with an accurate menu label and restores list context and menu focus on cancel', async () => {
  const post = vi.spyOn(api, 'post')
  const user = userEvent.setup()
  const { router } = await entry()
  const before = router.state.location.search
  await user.click(screen.getByRole('button', { name: 'More actions' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Add vendor' }))
  await screen.findByRole('dialog', { name: 'Create Vendor' })
  await user.type(screen.getByLabelText('Vendor Name *'), 'Unsaved vendor')
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  await waitFor(() => expect(screen.getByRole('button', { name: 'More actions' })).toHaveFocus())
  expect(router.state.location.search).toEqual(before)
  expect(post).not.toHaveBeenCalled()
  expect(api.get).not.toHaveBeenCalled()
})

it('closes the complete prefill management flow back to its original menu without changing filters', async () => {
  const user = userEvent.setup()
  const { router } = await entry()
  const before = router.state.location.search
  await user.click(screen.getByRole('button', { name: 'More actions' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Prefill Groups' }))
  await screen.findByRole('dialog', { name: 'Prefill Group Management' })
  await screen.findByText('No prefill groups yet')
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  await waitFor(() => expect(screen.getByRole('button', { name: 'More actions' })).toHaveFocus())
  expect(router.state.location.search).toEqual(before)
  expect(vi.mocked(api.get).mock.calls.every(([url]) => url === '/api/prefill_group')).toBe(true)
})

it('clears the previous identity vendor draft and requires a fresh open after the account changes', async () => {
  const user = userEvent.setup()
  await entry()
  await user.click(screen.getByRole('button', { name: 'More actions' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Add vendor' }))
  await screen.findByRole('dialog', { name: 'Create Vendor' })
  await user.type(screen.getByLabelText('Vendor Name *'), 'Old identity draft')
  await act(async () => { useAuthStore.getState().auth.setUser({ id: 2, role: ROLE.ADMIN, username: 'synthetic-new-admin' }) })
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  await user.click(screen.getByRole('button', { name: 'More actions' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Add vendor' }))
  expect(await screen.findByLabelText('Vendor Name *')).toHaveValue('')
})

it('a late vendor write result cannot close or clear a new identity creation task', async () => {
  let complete: ((value: { data: { success: boolean } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean } }>((resolve) => { complete = resolve })
  const post = vi.spyOn(api, 'post').mockReturnValue(pending)
  const user = userEvent.setup()
  await entry()
  await user.click(screen.getByRole('button', { name: 'More actions' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Add vendor' }))
  await user.type(await screen.findByLabelText('Vendor Name *'), 'Old pending vendor')
  await user.click(screen.getByRole('button', { name: 'Create', exact: true }))
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
  await act(async () => { useAuthStore.getState().auth.setUser({ id: 2, role: ROLE.ADMIN, username: 'synthetic-new-admin' }) })
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  await user.click(screen.getByRole('button', { name: 'More actions' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Add vendor' }))
  await user.type(await screen.findByLabelText('Vendor Name *'), 'New identity draft')
  await act(async () => { complete?.({ data: { success: true } }); await pending })
  expect(screen.getByRole('dialog', { name: 'Create Vendor' })).toBeVisible()
  expect(screen.getByLabelText('Vendor Name *')).toHaveValue('New identity draft')
  expect(post).toHaveBeenCalledTimes(1)
})
