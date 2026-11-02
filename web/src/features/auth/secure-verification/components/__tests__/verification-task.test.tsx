import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { SecureVerificationDialog } from '../secure-verification-dialog'

afterEach(() => { cleanup(); vi.restoreAllMocks() })
it('the linked verification field accepts an existing alphabetic backup code without changing its submitted method or payload', async () => {
  const user = userEvent.setup()
  const verify = vi.fn()
  render(<SecureVerificationDialog open onOpenChange={() => {}} methods={{ has2FA: true, hasPasskey: false, passkeySupported: false }} state={{ method: '2fa', loading: false, code: 'ABCD1234' }} onVerify={verify} onCancel={() => {}} onCodeChange={() => {}} onMethodChange={() => {}} />)
  const code = screen.getByRole('textbox', { name: 'Authenticator or backup code' })
  expect(code).toHaveAttribute('inputmode', 'text')
  expect(code).toHaveValue('ABCD1234')
  expect(code).toHaveAttribute('maxlength', '8')
  await user.click(screen.getByRole('button', { name: 'Verify' }))
  expect(verify).toHaveBeenCalledWith('2fa', 'ABCD1234')
})

it('an in-flight verification keeps its original action lock and cannot be dismissed by Escape', async () => {
  const user = userEvent.setup()
  const close = vi.fn()
  render(<SecureVerificationDialog open onOpenChange={close} methods={{ has2FA: true, hasPasskey: false, passkeySupported: false }} state={{ method: '2fa', loading: true, code: '123456' }} onVerify={() => {}} onCancel={() => {}} onCodeChange={() => {}} onMethodChange={() => {}} />)
  expect(screen.getByRole('button', { name: 'Verify' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
  await user.keyboard('{Escape}')
  expect(close).not.toHaveBeenCalled()
})
