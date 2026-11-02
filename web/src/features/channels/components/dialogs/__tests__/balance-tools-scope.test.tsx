import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useEffect } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { channelSchema, type Channel } from '../../../types'
import { ChannelsProvider, useChannels } from '../../channels-provider'
import { BalanceQueryDialog } from '../balance-query-dialog'

const first = channelSchema.parse({ id: 23, type: 1, name: 'synthetic-first-channel', key: '', status: 1, balance: 12, models: 'synthetic-model', created_time: 1, test_time: 0, response_time: 0, balance_updated_time: 1 })
const second = { ...first, id: 24, name: 'synthetic-second-channel', balance: 24 }
let client: QueryClient
beforeEach(() => { client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } }) })
afterEach(() => { cleanup(); client.clear(); vi.restoreAllMocks() })
function Tools(props: { row: Channel }) {
  const { currentRow, setCurrentRow } = useChannels()
  useEffect(() => { setCurrentRow(props.row) }, [props.row, setCurrentRow])
  return <><output aria-label='Saved channel balance'>{currentRow?.balance}</output><BalanceQueryDialog open onOpenChange={vi.fn()} /></>
}
function fixture(row: Channel) {
  return <QueryClientProvider client={client}><ChannelsProvider><Tools row={row} /></ChannelsProvider></QueryClientProvider>
}
function readMetadata(url: string) {
  if (url === '/api/status') return { data: { success: true, data: {} } }
  if (url.includes('/quota/history')) return { data: { success: true, data: { channel_id: 23, start: 1, end: 2, limit: 5000, points: [] } } }
  throw new Error(`Unexpected GET ${url}`)
}

it('keeps the newly selected channel when an older channel balance read resolves late', async () => {
  let finishOld: ((value: { data: { success: boolean; balance: number } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean; balance: number } }>((resolve) => { finishOld = resolve })
  const get = vi.spyOn(api, 'get').mockImplementation((url) => url === '/api/channel/update_balance/23' ? pending : Promise.resolve(readMetadata(String(url))))
  const view = render(fixture(first))
  await userEvent.setup().click(await screen.findByRole('button', { name: 'Update Balance' }))
  await waitFor(() => expect(get.mock.calls.some(([url]) => url === '/api/channel/update_balance/23')).toBe(true))
  view.rerender(fixture(second))
  await screen.findByText('synthetic-second-channel')
  await act(async () => { finishOld?.({ data: { success: true, balance: 99 } }); await pending })
  expect(screen.getByText('synthetic-second-channel')).toBeVisible()
  expect(screen.getByLabelText('Saved channel balance')).toHaveTextContent('24')
})

it('preserves the saved balance when a successful response has an explicit null balance', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => url === '/api/channel/update_balance/23' ? { data: { success: true, balance: null } } : readMetadata(String(url)))
  render(fixture(first))
  await userEvent.setup().click(await screen.findByRole('button', { name: 'Update Balance' }))
  await waitFor(() => expect(screen.getByRole('button', { name: 'Update Balance' })).toBeEnabled())
  expect(screen.getByLabelText('Saved channel balance')).toHaveTextContent('12')
})
