/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'
import { useAuthStore, type AuthBundle } from '@/stores/auth-store'

import { AccessTokenDialog } from '../dialogs/access-token-dialog'
import { TwoFABackupDialog } from '../dialogs/two-fa-backup-dialog'

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}))

const originalAuth = useAuthStore.getState()

function authBundle(): AuthBundle {
  return {
    access_token: 'test-access',
    token_type: 'Bearer',
    access_expires_at: 4_000_000_000,
    user: { id: 42, username: 'owner', role: 1 },
    session: {
      sid: 'session-1',
      current: true,
      login_method: 'password',
      ip: '127.0.0.1',
      user_agent: 'test',
      created_at: 1,
      last_active_at: 1,
      expires_at: 4_000_000_000,
    },
  }
}

beforeEach(() => {
  useAuthStore.getState().auth.setBundle(authBundle())
})

afterEach(() => {
  useAuthStore.setState(originalAuth)
  vi.restoreAllMocks()
})

test('access token generation cannot be dismissed before its one-time result arrives', async () => {
  type GetResponse = Awaited<ReturnType<typeof api.get>>
  let resolveGenerate!: (response: GetResponse) => void
  const pending = new Promise<GetResponse>((resolve) => {
    resolveGenerate = resolve
  })
  const get = vi.spyOn(api, 'get').mockReturnValue(pending)
  function Fixture() {
    const [open, setOpen] = useState(true)
    return (
      <>
        <Button onClick={() => setOpen(true)}>Open access token</Button>
        <AccessTokenDialog
          open={open}
          onOpenChange={setOpen}
          securityScopeKey='42:1:session-1'
        />
      </>
    )
  }
  const user = userEvent.setup()
  render(<Fixture />)

  await user.click(screen.getByRole('button', { name: 'Regenerate' }))
  const confirmation = await screen.findByRole('alertdialog', {
    name: 'Regenerate access token?',
  })
  await user.click(
    within(confirmation).getByRole('button', { name: 'Regenerate token' })
  )
  await waitFor(() => expect(get).toHaveBeenCalledOnce())
  const dialog = screen.getByRole('dialog', {
    name: 'Access Token',
    hidden: true,
  })
  expect(
    within(dialog).getByRole('button', { name: 'Close', hidden: true })
  ).toBeDisabled()
  expect(dialog.querySelector('[data-slot="dialog-close"]')).toBeNull()
  await user.keyboard('{Escape}')
  expect(confirmation).toBeVisible()
  expect(dialog).toBeInTheDocument()

  await act(async () => {
    resolveGenerate({
      data: { success: true, data: 'one-time-access-token' },
    } as GetResponse)
    await pending
  })
  expect(await screen.findByDisplayValue('one-time-access-token')).toBeVisible()
  await user.keyboard('{Escape}')
  await waitFor(() =>
    expect(
      screen.queryByRole('dialog', { name: 'Access Token' })
    ).not.toBeInTheDocument()
  )

  await user.click(screen.getByRole('button', { name: 'Open access token' }))
  expect(screen.queryByDisplayValue('one-time-access-token')).toBeNull()
  expect(get).toHaveBeenCalledOnce()
})

test('backup-code regeneration cannot be dismissed before the replacement codes arrive', async () => {
  type PostResponse = Awaited<ReturnType<typeof api.post>>
  let resolveRegenerate!: (response: PostResponse) => void
  const pending = new Promise<PostResponse>((resolve) => {
    resolveRegenerate = resolve
  })
  const post = vi.spyOn(api, 'post').mockReturnValue(pending)
  function Fixture() {
    const [open, setOpen] = useState(true)
    return (
      <>
        <Button onClick={() => setOpen(true)}>Open backup codes</Button>
        <TwoFABackupDialog
          open={open}
          onOpenChange={setOpen}
          onSuccess={vi.fn()}
          securityScopeKey='42:1:session-1'
        />
      </>
    )
  }
  const user = userEvent.setup()
  render(<Fixture />)

  const dialog = screen.getByRole('dialog', {
    name: 'Regenerate Backup Codes',
  })
  await user.type(within(dialog).getByLabelText('Verification Code'), '123456')
  await user.click(
    within(dialog).getByRole('button', { name: 'Generate New Codes' })
  )
  await waitFor(() => expect(post).toHaveBeenCalledOnce())
  expect(within(dialog).getByRole('button', { name: 'Cancel' })).toBeDisabled()
  expect(dialog.querySelector('[data-slot="dialog-close"]')).toBeNull()
  await user.keyboard('{Escape}')
  expect(dialog).toBeVisible()

  await act(async () => {
    resolveRegenerate({
      data: { success: true, data: { backup_codes: ['backup-code-one'] } },
    } as PostResponse)
    await pending
  })
  expect(await screen.findByText('backup-code-one')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Done' }))
  await user.click(screen.getByRole('button', { name: 'Open backup codes' }))
  expect(screen.queryByText('backup-code-one')).toBeNull()
  expect(
    within(screen.getByRole('dialog')).getByLabelText('Verification Code')
  ).toHaveValue('')
  expect(post).toHaveBeenCalledOnce()
})
