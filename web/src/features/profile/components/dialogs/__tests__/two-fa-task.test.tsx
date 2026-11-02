import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { TwoFASetupDialog } from '../two-fa-setup-dialog'

const originalAuth = useAuthStore.getState()
afterEach(() => { cleanup(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })
function SetupTask() {
  const [open, setOpen] = useState(false)
  return <><button type='button' onClick={() => setOpen(true)}>Open setup</button><TwoFASetupDialog open={open} onOpenChange={setOpen} onSuccess={() => {}} securityScopeKey='synthetic-owner-totp' /></>
}
it('2FA setup keeps QR backup and verification order and clears the old secret when closed before a fresh setup', async () => {
  useAuthStore.getState().auth.setUser({ id: 42, role: 1, username: 'synthetic-owner' })
  const firstSecret = 'SYNTHETIC_ONLY_LONG_SETUP_SECRET_WITH_A_COMPLETE_SUFFIX'
  const secondSecret = 'SYNTHETIC_ONLY_NEW_SETUP_SECRET'
  let setups = 0
  const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (url !== '/api/user/2fa/setup') throw new Error(`Unexpected synthetic security POST ${url}`)
    setups += 1
    return { data: { success: true, data: { secret: setups === 1 ? firstSecret : secondSecret, qr_code_data: 'otpauth://totp/synthetic-only?secret=SYNTHETIC', backup_codes: ['SYNTHETIC-BACKUP-ONE', 'SYNTHETIC-BACKUP-TWO'] } } }
  })
  const user = userEvent.setup()
  render(<SetupTask />)
  await user.click(screen.getByRole('button', { name: 'Open setup' }))
  expect(await screen.findByText(firstSecret)).toBeVisible()
  expect(screen.getByRole('region', { name: 'Account security details' })).toHaveAttribute('tabindex', '0')
  await user.click(screen.getByRole('button', { name: 'Next' }))
  expect(screen.getByText('SYNTHETIC-BACKUP-ONE')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Next' }))
  expect(screen.getByRole('button', { name: 'Enable 2FA' })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Close' }))
  await waitFor(() => expect(screen.queryByText(firstSecret)).not.toBeInTheDocument())
  await user.click(screen.getByRole('button', { name: 'Open setup' }))
  expect(await screen.findByText(secondSecret)).toBeVisible()
  expect(screen.queryByText(firstSecret)).not.toBeInTheDocument()
  expect(post).toHaveBeenCalledTimes(2)
})
