import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { redemptionSchema } from '../../types'
import { RedemptionsDeleteDialog } from '../redemptions-delete-dialog'
import { RedemptionsProvider, useRedemptions } from '../redemptions-provider'

const item = redemptionSchema.parse({ id: 17, user_id: 1, name: 'Synthetic redemption label with a complete long suffix', key: 'synthetic-unused-code', status: 1, quota: 500000, created_time: 1, redeemed_time: 0, expired_time: 0, used_user_id: 0 })
afterEach(() => { cleanup(); vi.restoreAllMocks() })
function Confirmation() {
  const { setOpen, setCurrentRow } = useRedemptions()
  return <><button type='button' onClick={() => { setCurrentRow(item); setOpen('delete') }}>Open deletion</button><RedemptionsDeleteDialog /></>
}
it('redemption deletion identifies the complete label and record ID and cancelling sends no deletion', async () => {
  const remove = vi.spyOn(api, 'delete').mockRejectedValue(new Error('Unexpected synthetic redemption deletion'))
  const user = userEvent.setup()
  render(<RedemptionsProvider><Confirmation /></RedemptionsProvider>)
  await user.click(screen.getByRole('button', { name: 'Open deletion' }))
  const confirmation = await screen.findByRole('alertdialog')
  expect(within(confirmation).getByText(item.name)).toBeVisible()
  expect(within(confirmation).getByText('#17')).toBeVisible()
  expect(within(confirmation).queryByText('synthetic-unused-code')).not.toBeInTheDocument()
  await user.click(within(confirmation).getByRole('button', { name: 'Cancel' }))
  expect(remove).not.toHaveBeenCalled()
})

it('a failed redemption deletion leaves the original named confirmation open with a readable failure', async () => {
  const remove = vi.spyOn(api, 'delete').mockResolvedValue({ data: { success: false, message: 'Synthetic deletion failed' } })
  const user = userEvent.setup()
  render(<RedemptionsProvider><Confirmation /></RedemptionsProvider>)
  await user.click(screen.getByRole('button', { name: 'Open deletion' }))
  const confirmation = await screen.findByRole('alertdialog')
  await user.click(within(confirmation).getByRole('button', { name: 'Delete' }))
  expect(await within(confirmation).findByRole('alert')).toHaveTextContent('Synthetic deletion failed')
  expect(within(confirmation).getByText(item.name)).toBeVisible()
  expect(within(confirmation).getByText('#17')).toBeVisible()
  expect(remove).toHaveBeenCalledTimes(1)
})
