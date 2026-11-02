import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { DEFAULT_CURRENCY_CONFIG, useSystemConfigStore } from '@/stores/system-config-store'

import { BillingHistoryDialog } from '../billing-history-dialog'
import { CreemConfirmDialog } from '../creem-confirm-dialog'
import { PaymentConfirmDialog } from '../payment-confirm-dialog'
import { TransferDialog } from '../transfer-dialog'

const originalAuth = useAuthStore.getState()
const originalConfig = useSystemConfigStore.getState()
const order = 'synthetic-order-with-a-complete-long-identifier-and-final-suffix'
beforeEach(() => {
  useAuthStore.getState().auth.setUser({ id: 1, role: 100, username: 'synthetic-root' })
  useSystemConfigStore.setState((state) => ({ ...state, config: { ...state.config, currency: { ...DEFAULT_CURRENCY_CONFIG, quotaPerUnit: 500000, quotaDisplayType: 'USD' } } }))
})
afterEach(() => { cleanup(); useAuthStore.setState(originalAuth); useSystemConfigStore.setState(originalConfig); vi.restoreAllMocks() })

it('billing history exposes the complete order and copies its exact value while the independent completion confirmation cancels without writing', async () => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (!url.startsWith('/api/user/topup?')) throw new Error(`Unexpected synthetic GET ${url}`)
    return { data: { success: true, data: { items: [{ id: 17, user_id: 2, amount: 12.34, money: 9.87, trade_no: order, payment_method: 'stripe', create_time: 1, status: 'pending' }], total: 1 } } }
  })
  const post = vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected synthetic completion'))
  const user = userEvent.setup()
  const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
  render(<BillingHistoryDialog open onOpenChange={() => {}} />)
  const body = screen.getByRole('region', { name: 'Billing history results' })
  const trade = await within(body).findByText(order)
  expect(trade).not.toHaveClass('truncate')
  expect(body).toHaveClass('min-h-0', 'overflow-y-auto')
  expect(body).not.toContainElement(screen.getByRole('navigation', { name: 'Billing history pagination' }))
  await user.click(within(body).getByRole('button', { name: 'Copy' }))
  expect(copy).toHaveBeenCalledWith(order)
  await user.click(screen.getByRole('button', { name: 'Complete Order' }))
  const confirmation = await screen.findByRole('alertdialog', { name: 'Complete Order' })
  expect(within(confirmation).getByText(order)).toBeVisible()
  await user.click(within(confirmation).getByRole('button', { name: 'Cancel' }))
  expect(post).not.toHaveBeenCalled()
})

it('a processing payment keeps its exact quoted amounts visible and both original confirmation actions locked', async () => {
  const user = userEvent.setup()
  const confirm = vi.fn()
  const close = vi.fn()
  render(<PaymentConfirmDialog open onOpenChange={close} onConfirm={confirm} topupAmount={100} paymentAmount={10} paymentMethod={{ type: 'stripe', name: 'Synthetic payment method with a complete long name' }} calculating={false} processing discountRate={0.8} />)
  const body = screen.getByRole('region', { name: 'Payment details' })
  expect(body).toHaveClass('min-h-0', 'overflow-y-auto')
  expect(within(body).getByText('10')).toBeVisible()
  expect(within(body).getByText('12.5')).toBeVisible()
  expect(within(body).getByText('2.5')).toBeVisible()
  expect(within(body).getByText('Synthetic payment method with a complete long name')).toBeVisible()
  const confirmButton = screen.getByRole('button', { name: 'Confirm Payment' })
  expect(confirmButton).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
  expect(body).not.toContainElement(confirmButton)
  await user.keyboard('{Escape}')
  expect(close).not.toHaveBeenCalled()
  expect(confirm).not.toHaveBeenCalled()
})

it('Creem confirmation shows the full product name and exact original currency quote and cancel never purchases', async () => {
  const user = userEvent.setup()
  const confirm = vi.fn()
  const close = vi.fn()
  render(<CreemConfirmDialog open onOpenChange={close} onConfirm={confirm} processing={false} product={{ name: 'Synthetic product with a complete long name and final suffix', productId: 'synthetic-product', price: 12.34, quota: 500000, currency: 'EUR' }} />)
  expect(screen.getByText('Synthetic product with a complete long name and final suffix')).toBeVisible()
  expect(screen.getByText('€12.34')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Confirm Payment' })).toHaveClass('min-h-11')
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(close).toHaveBeenCalledWith(false)
  expect(confirm).not.toHaveBeenCalled()
})

it('the transfer task preserves the existing unit conversion and bounds and keeps the draft when the external confirmation fails', async () => {
  const user = userEvent.setup()
  const confirm = vi.fn().mockResolvedValue(false)
  const close = vi.fn()
  render(<TransferDialog open onOpenChange={close} onConfirm={confirm} availableQuota={2500000} transferring={false} />)
  const amount = screen.getByRole('spinbutton', { name: 'Transfer Amount' })
  expect(amount).toHaveClass('min-h-11')
  expect(amount).toHaveAttribute('min', '1')
  expect(amount).toHaveAttribute('max', '5')
  expect(amount).toHaveAttribute('step', '1')
  fireEvent.change(amount, { target: { value: '2' } })
  await user.click(screen.getByRole('button', { name: 'Transfer' }))
  expect(confirm).toHaveBeenCalledWith(1000000)
  expect(amount).toHaveValue(2)
  expect(close).not.toHaveBeenCalled()
})

it.each(['transport', 'business'] as const)('a %s billing read failure exposes retry without inventing successful empty history or zero counts', async (kind) => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: { items: [], total: 0 } } })
  if (kind === 'transport') get.mockRejectedValueOnce(new Error('Synthetic history failed'))
  else get.mockResolvedValueOnce({ data: { success: false, message: 'Synthetic history failed' } })
  const user = userEvent.setup()
  render(<BillingHistoryDialog open onOpenChange={() => {}} />)
  expect(await screen.findByRole('alert')).toHaveTextContent('Unable to load billing history')
  expect(screen.queryByText('No billing records found')).not.toBeInTheDocument()
  expect(screen.queryByText(/Showing 0/)).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByText('No billing records found')).toBeVisible()
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(get).toHaveBeenCalledTimes(2)
})

it('billing rows remain readable when their total is unknown and pagination cannot treat it as zero', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: { items: [{ id: 17, user_id: 2, amount: 12.34, money: 9.87, trade_no: order, payment_method: 'stripe', create_time: 1, status: 'success' }] } } })
  render(<BillingHistoryDialog open onOpenChange={() => {}} />)
  expect(await screen.findByText(order)).toBeVisible()
  const pagination = screen.getByRole('navigation', { name: 'Billing history pagination' })
  expect(within(pagination).getByText('Record total unavailable')).toBeVisible()
  expect(within(pagination).getByRole('button', { name: 'Next page' })).toBeDisabled()
  expect(screen.queryByText('No billing records found')).not.toBeInTheDocument()
})

it('a failed manual completion keeps the exact order confirmation open with no automatic retry', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: { items: [{ id: 17, user_id: 2, amount: 12.34, money: 9.87, trade_no: order, payment_method: 'stripe', create_time: 1, status: 'pending' }], total: 1 } } })
  const post = vi.spyOn(api, 'post').mockResolvedValue({ data: { success: false, message: 'Synthetic completion failed' } })
  const user = userEvent.setup()
  render(<BillingHistoryDialog open onOpenChange={() => {}} />)
  await user.click(await screen.findByRole('button', { name: 'Complete Order' }))
  const confirmation = await screen.findByRole('alertdialog', { name: 'Complete Order' })
  await user.click(within(confirmation).getByRole('button', { name: 'Confirm' }))
  expect(await within(confirmation).findByRole('alert')).toHaveTextContent('Failed to complete order')
  expect(within(confirmation).getByText(order)).toBeVisible()
  expect(post).toHaveBeenCalledTimes(1)
  expect(post.mock.calls[0]?.[1]).toEqual({ trade_no: order })
})

it('a failed later billing page clears old successful rows and hides their previous total', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValueOnce({ data: { success: true, data: { items: [{ id: 17, user_id: 2, amount: 12.34, money: 9.87, trade_no: order, payment_method: 'stripe', create_time: 1, status: 'success' }], total: 20 } } }).mockResolvedValue({ data: { success: false, message: 'Synthetic next page failed' } })
  const user = userEvent.setup()
  render(<BillingHistoryDialog open onOpenChange={() => {}} />)
  await screen.findByText(order)
  await user.click(screen.getByRole('button', { name: 'Next page' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('Synthetic next page failed')
  expect(screen.queryByText(order)).not.toBeInTheDocument()
  expect(screen.queryByRole('navigation', { name: 'Billing history pagination' })).not.toBeInTheDocument()
  expect(screen.queryByText('No billing records found')).not.toBeInTheDocument()
  expect(get).toHaveBeenCalledTimes(2)
})
