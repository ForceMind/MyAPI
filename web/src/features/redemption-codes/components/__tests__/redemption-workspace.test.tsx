import { QueryClient } from '@tanstack/react-query'
import { act, cleanup, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { renderListRoute } from '@/components/data-table/layout/__tests__/list-route-fixture'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { RedemptionsPrimaryButtons } from '../redemptions-primary-buttons'
import { RedemptionsProvider } from '../redemptions-provider'
import { RedemptionsTable } from '../redemptions-table'

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn(), warning: vi.fn() } }))
const originalAuth = useAuthStore.getState()
const codeName = 'synthetic/code-with-a-complete-extremely-long-name-and-final-suffix'
const code = { id: 18, user_id: 1, name: codeName, key: 'synthetic-nonredeemable-code-only-0001', status: 1, quota: 500000, created_time: 1730000000, redeemed_time: 0, expired_time: 0, used_user_id: 0 }
let client: QueryClient
let rows: typeof code[]
let total: number | undefined
let failRead: 'transport' | 'business' | null

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  useAuthStore.getState().auth.setUser({ id: 1, role: 100, username: 'synthetic-root' })
  rows = [code]
  total = 1
  failRead = null
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url !== '/api/redemption/' && url !== '/api/redemption/search') throw new Error(`Unexpected GET ${url}`)
    if (failRead === 'transport') throw new Error('Synthetic redemption read failed')
    if (failRead === 'business') return { data: { success: false, message: 'Synthetic redemption read failed' } }
    return { data: { success: true, data: { items: rows, total } } }
  })
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected redemption write'))
  vi.spyOn(api, 'put').mockRejectedValue(new Error('Unexpected redemption write'))
  vi.spyOn(api, 'delete').mockRejectedValue(new Error('Unexpected redemption delete'))
})
afterEach(() => { cleanup(); client.clear(); localStorage.clear(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })

async function renderCodes(entry = '/redemption-codes/') {
  return renderListRoute({ client, path: '/redemption-codes/', entry, element: <RedemptionsProvider><RedemptionsPrimaryButtons /><RedemptionsTable /></RedemptionsProvider> })
}

it('keeps unavailable totals unknown and the requested page while bounding an old oversized page size', async () => {
  total = undefined
  await renderCodes('/redemption-codes/?page=4&pageSize=1000')
  await screen.findByText(codeName)
  const pager = screen.getByRole('navigation', { name: 'Redemption codes pagination' })
  expect(pager).toHaveTextContent('Record total unavailable')
  expect(within(pager).getByRole('button', { name: 'Last page' })).toBeDisabled()
  expect(api.get).toHaveBeenCalledWith('/api/redemption/', { params: { p: 4, page_size: 100 } })
})

it.each(['transport', 'business'] as const)('shows %s reread failure without stale rows or a successful empty state', async (failure) => {
  await renderCodes()
  await screen.findByText(codeName)
  failRead = failure
  await act(async () => { await client.refetchQueries() })
  expect(await screen.findByRole('alert')).toHaveTextContent('Failed to load redemption codes')
  expect(screen.queryByText(codeName)).not.toBeInTheDocument()
  expect(screen.queryByText('No Redemption Codes Found')).not.toBeInTheDocument()
  expect(screen.getByRole('navigation', { name: 'Redemption codes pagination' })).toHaveTextContent('Record total unavailable')
})

it('exposes the original creation and redeemed-by fields and the complete name on a phone', async () => {
  const original = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({ ...original(query), matches: query === '(max-width: 1023px)' }))
  await renderCodes()
  const card = await screen.findByRole('article', { name: codeName })
  expect(within(card).getByText(codeName)).toBeVisible()
  const user = userEvent.setup()
  await user.click(within(card).getByText('More details'))
  expect(within(card).getByText('Created')).toBeVisible()
  expect(within(card).getByText('Redeemed By')).toBeVisible()
  expect(screen.queryByText(code.key)).not.toBeInTheDocument()
  expect(api.delete).not.toHaveBeenCalled()
})

it('corrects a confirmed zero total to page one without converting it to an unknown count', async () => {
  rows = []
  total = 0
  await renderCodes('/redemption-codes/?page=4&pageSize=20')
  await screen.findByText('No Redemption Codes Found')
  await waitFor(() => expect(vi.mocked(api.get).mock.calls.at(-1)).toEqual(['/api/redemption/', { params: { p: 1, page_size: 20 } }]))
  const pager = screen.getByRole('navigation', { name: 'Redemption codes pagination' })
  expect(pager).not.toHaveTextContent('Record total unavailable')
  expect(within(pager).getByRole('button', { name: 'Last page' })).toBeDisabled()
})

it('retains the independent global invalid-code confirmation and its readable failure', async () => {
  await renderCodes()
  await screen.findByText(codeName)
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Delete Invalid' }))
  const confirm = await screen.findByRole('alertdialog')
  expect(confirm).toHaveTextContent('used')
  expect(confirm).toHaveTextContent('expired')
  await user.click(within(confirm).getByRole('button', { name: 'Delete Invalid' }))
  expect(await within(confirm).findByRole('alert')).toHaveTextContent('Unexpected redemption delete')
  expect(confirm).toBeVisible()
  expect(api.delete).toHaveBeenCalledTimes(1)
})

it.each([
  { data: undefined, expected: 'Deleted successfully' },
  { data: 0, expected: 'Successfully deleted 0 invalid redemption codes' },
])('distinguishes the invalid-code deletion result $data without inventing zero', async ({ data, expected }) => {
  vi.mocked(api.delete).mockResolvedValue({ data: { success: true, data } })
  await renderCodes()
  await screen.findByText(codeName)
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Delete Invalid' }))
  const confirm = await screen.findByRole('alertdialog')
  await user.click(within(confirm).getByRole('button', { name: 'Delete Invalid' }))
  await waitFor(() => expect(toast.success).toHaveBeenCalledWith(expected))
  expect(api.delete).toHaveBeenCalledTimes(1)
})
