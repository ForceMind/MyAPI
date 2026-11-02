import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { channelSchema } from '../../../types'
import { ChannelsProvider, useChannels } from '../../channels-provider'
import { MultiKeyManageDialog } from '../multi-key-manage-dialog'

const originalAuth = useAuthStore.getState()
const channel = channelSchema.parse({ id: 23, type: 1, name: 'synthetic-channel', key: '', status: 1, models: 'synthetic-model', group: 'default', created_time: 1, test_time: 0, response_time: 0, balance_updated_time: 0 })
const snapshot = { keys: [{ index: 0, status: 1, reason: 'synthetic-key-reason' }], total: 1, page: 1, page_size: 10, total_pages: 1, enabled_count: 1, manual_disabled_count: 0, auto_disabled_count: 0 }
let client: QueryClient
let omitCounts: boolean
let failRead: boolean

beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.SUPER_ADMIN, username: 'synthetic-one' })
  omitCounts = false
  failRead = false
  vi.spyOn(api, 'post').mockImplementation(async (url, body) => {
    if (url !== '/api/channel/multi_key/manage') throw new Error(`Unexpected POST ${url}`)
    if ((body as { action: string }).action !== 'get_key_status') throw new Error('Synthetic multi-key write failed')
    if (failRead) throw new Error('Synthetic multi-key read failed')
    return { data: { success: true, data: omitCounts ? { keys: snapshot.keys } : snapshot } }
  })
})
afterEach(() => { cleanup(); client.clear(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })

function Tools() {
  const { open, setOpen, setCurrentRow } = useChannels()
  return <><button type='button' onClick={() => { setCurrentRow(channel); setOpen('multi-key-manage') }}>Open multi-key tools</button><MultiKeyManageDialog open={open === 'multi-key-manage'} onOpenChange={(next) => !next && setOpen(null)} /></>
}
async function openTools() {
  render(<QueryClientProvider client={client}><TooltipProvider><ChannelsProvider><Tools /></ChannelsProvider></TooltipProvider></QueryClientProvider>)
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Open multi-key tools' }))
  await screen.findByText('synthetic-key-reason')
  return user
}

it('shows omitted multi-key counts as unknown while retaining returned key rows', async () => {
  omitCounts = true
  await openTools()
  expect(screen.getAllByText('Unknown').length).toBeGreaterThan(0)
  expect(screen.queryByText('0')).not.toBeInTheDocument()
})

it('shows a failed filtered multi-key read without old key rows or fabricated zero counts', async () => {
  const user = await openTools()
  failRead = true
  await user.click(screen.getByRole('combobox'))
  await user.click(screen.getByRole('option', { name: 'Enabled' }))
  expect(await screen.findByText('Synthetic multi-key read failed')).toBeVisible()
  expect(screen.queryByText('synthetic-key-reason')).not.toBeInTheDocument()
  expect(screen.queryByText('0')).not.toBeInTheDocument()
})

it('keeps a failed per-key action in its independent confirmation for correction or cancellation', async () => {
  const user = await openTools()
  await user.click(screen.getByRole('button', { name: 'Disable' }))
  const confirm = await screen.findByRole('alertdialog')
  await user.click(within(confirm).getByRole('button', { name: 'Continue' }))
  expect(await screen.findByText('Synthetic multi-key write failed')).toBeVisible()
  expect(screen.getByRole('alertdialog')).toHaveTextContent('Synthetic multi-key write failed')
  expect(vi.mocked(api.post).mock.calls.filter(([, body]) => (body as { action: string }).action === 'disable_key')).toHaveLength(1)
})

it('clears the selected channel task when the login identity changes', async () => {
  await openTools()
  await act(async () => { useAuthStore.getState().auth.setUser({ id: 2, role: ROLE.SUPER_ADMIN, username: 'synthetic-two' }) })
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(screen.queryByText('synthetic-key-reason')).not.toBeInTheDocument()
})
