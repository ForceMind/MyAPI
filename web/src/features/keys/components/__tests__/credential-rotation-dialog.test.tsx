/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import type {
  CredentialRotationPreview,
  CredentialRotationReceipt,
} from '../../credential-rotation-api'
import {
  readPendingCredentialRotation,
  savePendingCredentialRotation,
} from '../../credential-rotation-pending-operation'
import { CredentialRotationDialog } from '../credential-rotation-dialog'

const apiMocks = vi.hoisted(() => ({
  preview: vi.fn(),
  commit: vi.fn(),
  receipt: vi.fn(),
  disposition: vi.fn(() => 'unknown' as 'not_accepted' | 'unknown'),
  startOptions: null as { scope?: string } | null,
}))

vi.mock('../../credential-rotation-api', async (original) => {
  const actual =
    await original<typeof import('../../credential-rotation-api')>()
  return {
    ...actual,
    createCredentialRotationOperationId: () => 'c'.repeat(64),
    previewCredentialRotation: apiMocks.preview,
    commitCredentialRotation: apiMocks.commit,
    getCredentialRotationReceipt: apiMocks.receipt,
    getCredentialRotationCommitDisposition: apiMocks.disposition,
  }
})

vi.mock('../../hooks/use-key-maintenance-session', () => ({
  useKeyMaintenanceSession: () => () => true,
}))

vi.mock('@/features/auth/secure-verification', async () => {
  const React = await import('react')
  const { Dialog, DialogContent, DialogTitle } =
    await import('@/components/ui/dialog')
  return {
    useSecureVerification: () => {
      const [callback, setCallback] = React.useState<
        ((proof?: string) => Promise<unknown>) | null
      >(null)
      return {
        open: Boolean(callback),
        methods: {
          has2FA: true,
          hasPasskey: false,
          passkeySupported: false,
        },
        state: { method: '2fa', loading: false, code: '123456' },
        startVerification: async (
          next: (proof?: string) => Promise<unknown>,
          options: { scope?: string }
        ) => {
          apiMocks.startOptions = options
          setCallback(() => next)
          return true
        },
        executeVerification: async () => {
          const result = await callback?.('synthetic-proof')
          setCallback(null)
          return result
        },
        cancel: () => setCallback(null),
        setCode: vi.fn(),
        switchMethod: vi.fn(),
      }
    },
    SecureVerificationDialog: (props: {
      open: boolean
      onOpenChange: (open: boolean) => void
      onVerify: (method: '2fa', code: string) => Promise<void>
    }) =>
      props.open ? (
        <Dialog open onOpenChange={props.onOpenChange}>
          <DialogContent>
            <DialogTitle>Verify credential rotation</DialogTitle>
            <button
              type='button'
              onClick={() => void props.onVerify('2fa', '123456')}
            >
              Complete rotation verification
            </button>
          </DialogContent>
        </Dialog>
      ) : null,
  }
})

const digest = 'a'.repeat(64)
const secret = `sk-${'A'.repeat(48)}`
const preview: CredentialRotationPreview = {
  status: 'ready',
  preview_revision: digest,
  expires_at: 123,
  credential_version_before: 1,
  credential_version_after: 2,
  blockers: [],
  effects: {
    token_id_preserved: true,
    old_secret_invalidation: 'immediate',
    accepted_settlement: 'continues',
    writer_changed: false,
  },
}
const receipt: CredentialRotationReceipt = {
  operation_id: 'c'.repeat(64),
  state: 'committed',
  token_id: 21,
  credential_version_before: 1,
  credential_version_after: 2,
  masked_key: 'sk-…AAAA',
  created_at: 123,
  old_secret_invalidated_at: 123,
  settlement_mode: 'authoritative_persistent',
  writer_changed: false,
}

function rotationDialog(open: boolean, onOpenChange = vi.fn()) {
  return (
    <CredentialRotationDialog
      open={open}
      tokenId={21}
      tokenName='Desktop Key'
      maskedKey='sk-…abcd'
      userId={1}
      onOpenChange={onOpenChange}
      onWriteLockChange={vi.fn()}
      onCommitted={vi.fn()}
    />
  )
}

function renderRotation(onOpenChange = vi.fn()) {
  return render(rotationDialog(true, onOpenChange))
}

async function confirmRotation(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    screen.getByRole('button', { name: 'Check rotation eligibility' })
  )
  await screen.findByText('Rotation preview')
  await user.click(
    screen.getByRole('checkbox', {
      name: 'I understand the old Key value will stop working immediately.',
    })
  )
  await user.click(
    screen.getByRole('checkbox', {
      name: 'I understand accepted requests may continue settling after rotation.',
    })
  )
  await user.click(
    screen.getByRole('checkbox', {
      name: 'I understand the new Key value is shown automatically only once.',
    })
  )
  await user.click(
    screen.getByRole('button', { name: 'Verify and rotate Key' })
  )
}

beforeEach(() => {
  window.sessionStorage.clear()
  apiMocks.preview.mockResolvedValue(preview)
  apiMocks.commit.mockResolvedValue({
    receipt,
    replayed: false,
    reveal: { secret },
  })
  apiMocks.receipt.mockRejectedValue(new Error('Synthetic missing receipt'))
  apiMocks.disposition.mockReturnValue('unknown')
  apiMocks.startOptions = null
})

afterEach(() => {
  window.sessionStorage.clear()
  vi.clearAllMocks()
})

test('fresh rotation uses the dedicated proof and displays the new value once', async () => {
  const user = userEvent.setup()
  renderRotation()
  await confirmRotation(user)
  expect(apiMocks.startOptions).toMatchObject({
    scope: 'token.credential_rotation',
  })

  await user.click(
    screen.getByRole('button', { name: 'Complete rotation verification' })
  )

  const input = await screen.findByRole('textbox', { name: 'New Key value' })
  expect(input).toHaveValue(secret)
  expect(apiMocks.commit).toHaveBeenCalledTimes(1)
  expect(apiMocks.commit).toHaveBeenCalledWith(
    21,
    {
      operation_id: 'c'.repeat(64),
      preview_revision: digest,
      confirmed: true,
    },
    'synthetic-proof',
    expect.any(AbortSignal)
  )
  expect(readPendingCredentialRotation(1, 21)).toBeNull()
})

test('unknown outcome survives close, reopen, and remount with the original operation', async () => {
  const user = userEvent.setup()
  apiMocks.commit.mockRejectedValue(new Error('Synthetic network loss'))
  const onOpenChange = vi.fn()
  const first = renderRotation(onOpenChange)
  await confirmRotation(user)
  await user.click(
    screen.getByRole('button', { name: 'Complete rotation verification' })
  )

  expect(await screen.findByText('Rotation result is unknown')).toBeVisible()
  expect(apiMocks.commit).toHaveBeenCalledTimes(1)
  expect(apiMocks.receipt).toHaveBeenCalledTimes(1)
  expect(readPendingCredentialRotation(1, 21)).toBe('c'.repeat(64))

  const closeButtons = screen.getAllByRole('button', { name: 'Close' })
  const closeButton = closeButtons.at(-1)
  if (!closeButton) throw new Error('Rotation close button was not rendered')
  await user.click(closeButton)
  expect(onOpenChange).toHaveBeenCalledWith(false)
  first.rerender(rotationDialog(false, onOpenChange))
  first.rerender(rotationDialog(true, onOpenChange))
  await waitFor(() => expect(apiMocks.receipt).toHaveBeenCalledTimes(2))
  expect(apiMocks.commit).toHaveBeenCalledTimes(1)

  first.unmount()
  renderRotation()

  expect(await screen.findByText('Rotation result is unknown')).toBeVisible()
  expect(apiMocks.commit).toHaveBeenCalledTimes(1)
  expect(apiMocks.receipt).toHaveBeenCalledTimes(3)
  expect(apiMocks.receipt).toHaveBeenLastCalledWith(
    21,
    'c'.repeat(64),
    expect.any(AbortSignal)
  )
})

test('storage failure blocks the rotation POST', async () => {
  const user = userEvent.setup()
  renderRotation()
  await confirmRotation(user)
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new Error('Synthetic storage failure')
  })

  await user.click(
    screen.getByRole('button', { name: 'Complete rotation verification' })
  )

  await waitFor(() => expect(apiMocks.commit).not.toHaveBeenCalled())
  expect(screen.getByText('Rotation was not accepted')).toBeVisible()
})

test('a strict not-accepted disposition clears the saved operation', async () => {
  const user = userEvent.setup()
  apiMocks.commit.mockRejectedValue(new Error('Synthetic rejection'))
  apiMocks.disposition.mockReturnValue('not_accepted')
  renderRotation()
  await confirmRotation(user)

  await user.click(
    screen.getByRole('button', { name: 'Complete rotation verification' })
  )

  expect(await screen.findByText('Rotation was not accepted')).toBeVisible()
  expect(readPendingCredentialRotation(1, 21)).toBeNull()
  expect(apiMocks.receipt).not.toHaveBeenCalled()
  expect(
    screen.getByRole('button', { name: 'Check rotation eligibility' })
  ).toBeEnabled()
})

test('a duplicate verification callback cannot send a second rotation POST', async () => {
  const user = userEvent.setup()
  let resolveCommit:
    | ((value: {
        receipt: CredentialRotationReceipt
        replayed: true
        reveal: null
      }) => void)
    | undefined
  apiMocks.commit.mockImplementation(
    () =>
      new Promise((resolve) => {
        resolveCommit = resolve
      })
  )
  renderRotation()
  await confirmRotation(user)
  const complete = screen.getByRole('button', {
    name: 'Complete rotation verification',
  })

  await user.click(complete)
  await waitFor(() => expect(apiMocks.commit).toHaveBeenCalledTimes(1))
  await user.click(complete)
  expect(apiMocks.commit).toHaveBeenCalledTimes(1)

  resolveCommit?.({ receipt, replayed: true, reveal: null })
  expect(await screen.findByText('Credential rotation committed')).toBeVisible()
})

test('a recovered receipt unlocks without reading or inventing a secret', async () => {
  savePendingCredentialRotation(1, 21, receipt.operation_id)
  apiMocks.receipt.mockResolvedValue(receipt)

  renderRotation()

  expect(await screen.findByText('Credential rotation committed')).toBeVisible()
  expect(
    screen.getByText(
      'This receipt does not contain the new Key value. Use Reveal Key from the Key menu when you need to view the current value.'
    )
  ).toBeVisible()
  expect(apiMocks.commit).not.toHaveBeenCalled()
  expect(readPendingCredentialRotation(1, 21)).toBeNull()
  expect(screen.queryByLabelText('New Key value')).not.toBeInTheDocument()
})
