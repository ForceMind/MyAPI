import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useRef, useState } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getModels, getVendors } from '../../../api'
import { modelsQueryKeys, vendorsQueryKeys } from '../../../lib/query-keys'
import { VendorMutateDialog } from '../vendor-mutate-dialog'

const originalAuth = useAuthStore.getState()
let client: QueryClient

beforeEach(() => {
  localStorage.clear()
  useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.ADMIN, username: 'synthetic-admin' })
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
})

afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.setState(originalAuth)
  localStorage.clear()
  vi.restoreAllMocks()
})

function VendorCreateFixture() {
  const [open, setOpen] = useState(true)
  const opener = useRef<HTMLButtonElement>(null)
  return (
    <QueryClientProvider client={client}>
      <button ref={opener} onClick={() => setOpen(true)}>Create vendor entry</button>
      <VendorMutateDialog open={open} onOpenChange={setOpen} returnFocus={() => opener.current} />
    </QueryClientProvider>
  )
}

function ListCacheFixture() {
  const vendors = useQuery({ queryKey: vendorsQueryKeys.lists(), queryFn: () => getVendors() })
  const models = useQuery({ queryKey: modelsQueryKeys.lists(), queryFn: () => getModels() })
  return <><output aria-label='Vendor list total'>{vendors.data?.data?.total}</output><output aria-label='Model list total'>{models.data?.data?.total}</output></>
}

it('creates the exact vendor payload with its hidden default status and refreshes both lists after success', async () => {
  let saved = false
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/vendors/' || url === '/api/models/') return { data: { success: true, data: { items: [], total: saved ? 2 : 1 } } }
    throw new Error(`Unexpected GET ${url}`)
  })
  const post = vi.spyOn(api, 'post').mockImplementation(async () => {
    saved = true
    return { data: { success: true } }
  })
  const put = vi.spyOn(api, 'put')
  render(<><QueryClientProvider client={client}><ListCacheFixture /></QueryClientProvider><VendorCreateFixture /></>)
  await waitFor(() => expect(screen.getByLabelText('Vendor list total')).toHaveTextContent('1'))
  const dialog = screen.getByRole('dialog', { name: 'Create Vendor' })
  expect(within(dialog).queryByRole('switch')).not.toBeInTheDocument()
  const user = userEvent.setup()
  await user.type(screen.getByLabelText('Vendor Name *'), 'Synthetic vendor')
  await user.type(screen.getByLabelText('Icon'), 'OpenAI')
  await user.type(screen.getByLabelText('Description'), 'Complete vendor description')
  await user.click(within(dialog).getByRole('button', { name: 'Create' }))
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
  expect(post).toHaveBeenCalledWith('/api/vendors/', { name: 'Synthetic vendor', icon: 'OpenAI', description: 'Complete vendor description', status: 1 }, { skipAuthRefresh: true })
  expect(put).not.toHaveBeenCalled()
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  await waitFor(() => expect(screen.getByLabelText('Vendor list total')).toHaveTextContent('2'))
  await waitFor(() => expect(screen.getByLabelText('Model list total')).toHaveTextContent('2'))
  await waitFor(() => expect(screen.getByRole('button', { name: 'Create vendor entry' })).toHaveFocus())
})

it('keeps a duplicate-name failure beside the draft and allows the same fields to be corrected and retried', async () => {
  const post = vi.spyOn(api, 'post').mockResolvedValueOnce({ data: { success: false, message: 'Vendor name already exists' } }).mockResolvedValueOnce({ data: { success: true } })
  render(<VendorCreateFixture />)
  const user = userEvent.setup()
  const longName = 'A vendor with a deliberately long name that remains editable without clipping its field container'
  await user.type(screen.getByLabelText('Vendor Name *'), longName)
  await user.type(screen.getByLabelText('Icon'), 'Anthropic')
  await user.type(screen.getByLabelText('Description'), 'Keep this draft after a failure')
  await user.click(screen.getByRole('button', { name: 'Create', exact: true }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Vendor name already exists')
  expect(screen.getByLabelText('Vendor Name *')).toHaveValue(longName)
  expect(screen.getByLabelText('Icon')).toHaveValue('Anthropic')
  expect(screen.getByLabelText('Description')).toHaveValue('Keep this draft after a failure')
  expect(screen.getByRole('button', { name: 'Create', exact: true })).toBeEnabled()
  await user.clear(screen.getByLabelText('Vendor Name *'))
  await user.type(screen.getByLabelText('Vendor Name *'), 'Corrected vendor')
  await user.click(screen.getByRole('button', { name: 'Create', exact: true }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(post).toHaveBeenLastCalledWith('/api/vendors/', { name: 'Corrected vendor', icon: 'Anthropic', description: 'Keep this draft after a failure', status: 1 }, { skipAuthRefresh: true })
})

it('keeps a transport failure visible while retaining all entered fields', async () => {
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Synthetic network failure'))
  render(<VendorCreateFixture />)
  const user = userEvent.setup()
  await user.type(screen.getByLabelText('Vendor Name *'), 'Retry vendor')
  await user.type(screen.getByLabelText('Description'), 'Unsaved description')
  await user.click(screen.getByRole('button', { name: 'Create', exact: true }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Synthetic network failure')
  expect(screen.getByLabelText('Vendor Name *')).toHaveValue('Retry vendor')
  expect(screen.getByLabelText('Description')).toHaveValue('Unsaved description')
})

it('cancels without a write and reopens with an empty create form', async () => {
  const post = vi.spyOn(api, 'post')
  render(<VendorCreateFixture />)
  const user = userEvent.setup()
  await user.type(screen.getByLabelText('Vendor Name *'), 'Discarded vendor')
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(post).not.toHaveBeenCalled()
  await waitFor(() => expect(screen.getByRole('button', { name: 'Create vendor entry' })).toHaveFocus())
  await user.click(screen.getByRole('button', { name: 'Create vendor entry' }))
  expect(await screen.findByLabelText('Vendor Name *')).toHaveValue('')
})

it('marks an empty vendor name invalid and returns keyboard focus to the required field without a write', async () => {
  const post = vi.spyOn(api, 'post')
  render(<VendorCreateFixture />)
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Create', exact: true }))
  expect(await screen.findByText('Vendor name is required')).toBeVisible()
  expect(screen.getByLabelText('Vendor Name *')).toHaveAttribute('aria-invalid', 'true')
  await waitFor(() => expect(screen.getByLabelText('Vendor Name *')).toHaveFocus())
  expect(post).not.toHaveBeenCalled()
})

it('keeps one scroll region between the fixed title and actions with 44px input and action targets', () => {
  render(<VendorCreateFixture />)
  const dialog = screen.getByRole('dialog', { name: 'Create Vendor' })
  expect(dialog).toHaveClass('h-dvh', 'overflow-hidden', 'sm:h-auto')
  const scrollRegions = [...dialog.querySelectorAll<HTMLElement>('*')].filter((element) => element.classList.contains('overflow-y-auto'))
  expect(scrollRegions).toHaveLength(1)
  expect(scrollRegions[0]).not.toContainElement(screen.getByRole('heading', { name: 'Create Vendor' }))
  expect(scrollRegions[0]).not.toContainElement(screen.getByRole('button', { name: 'Create', exact: true }))
  expect(scrollRegions[0]).toContainElement(screen.getByLabelText('Vendor Name *'))
  expect(screen.getByLabelText('Vendor Name *')).toHaveClass('min-h-11')
  expect(screen.getByLabelText('Icon')).toHaveClass('min-h-11')
  expect(screen.getByRole('button', { name: 'Cancel' })).toHaveClass('min-h-11')
  expect(screen.getByRole('button', { name: 'Create', exact: true })).toHaveClass('min-h-11')
  expect(screen.getByRole('button', { name: 'Close' })).toHaveClass('min-h-11', 'min-w-11')
})

it('keeps the pending form busy and ignores repeated form submissions until the current write finishes', async () => {
  let complete: ((result: { data: { success: boolean } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean } }>((resolve) => { complete = resolve })
  const post = vi.spyOn(api, 'post').mockReturnValue(pending)
  render(<VendorCreateFixture />)
  const user = userEvent.setup()
  await user.type(screen.getByLabelText('Vendor Name *'), 'Pending vendor')
  const form = screen.getByLabelText('Vendor Name *').closest('form')
  expect(form).not.toBeNull()
  if (!form) throw new Error('Vendor form missing')
  fireEvent.submit(form)
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
  expect(screen.getByRole('button', { name: 'Saving...' })).toBeDisabled()
  expect(form).toHaveAttribute('aria-busy', 'true')
  expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
  expect(screen.getByLabelText('Vendor Name *')).toBeDisabled()
  await user.keyboard('{Escape}')
  expect(screen.getByRole('dialog')).toBeVisible()
  await act(async () => { fireEvent.submit(form) })
  expect(post).toHaveBeenCalledTimes(1)
  await act(async () => { complete?.({ data: { success: true } }); await pending })
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
})
