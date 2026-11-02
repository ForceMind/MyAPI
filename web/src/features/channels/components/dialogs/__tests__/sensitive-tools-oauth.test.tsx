import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { CodexOAuthDialog } from '../codex-oauth-dialog'

const originalAuth = useAuthStore.getState()
beforeEach(() => {
  useAuthStore.getState().auth.setUser({ id: 1, role: ROLE.SUPER_ADMIN, username: 'synthetic-one' })
  vi.spyOn(window, 'open').mockReturnValue(null)
})
afterEach(() => { cleanup(); useAuthStore.setState(originalAuth); vi.restoreAllMocks() })

function OAuthFixture(props: { saved?: () => void }) {
  const [open, setOpen] = useState(true)
  return <CodexOAuthDialog open={open} onOpenChange={setOpen} channelId={23} onCredentialSaved={props.saved} />
}

it('keeps the OAuth task open and callback draft intact when Escape is pressed during code exchange', async () => {
  let complete: ((value: { data: { success: boolean } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean } }>((resolve) => { complete = resolve })
  const post = vi.spyOn(api, 'post').mockReturnValue(pending)
  render(<OAuthFixture />)
  fireEvent.change(screen.getByRole('textbox'), { target: { value: 'code=synthetic&state=synthetic' } })
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Complete login' }))
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
  await user.keyboard('{Escape}')
  expect(screen.getByRole('dialog')).toBeInTheDocument()
  expect(screen.getByRole('textbox')).toHaveValue('code=synthetic&state=synthetic')
  expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
  await act(async () => { complete?.({ data: { success: true } }); await pending })
})

it('keeps a failed authorization visible beside the original callback draft', async () => {
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Synthetic code exchange failed'))
  render(<OAuthFixture />)
  fireEvent.change(screen.getByRole('textbox'), { target: { value: 'code=synthetic&state=synthetic' } })
  await userEvent.setup().click(screen.getByRole('button', { name: 'Complete login' }))
  expect(await screen.findByText('Synthetic code exchange failed')).toBeVisible()
  expect(screen.getByRole('textbox')).toHaveValue('code=synthetic&state=synthetic')
})

it('does not report a credential saved when an old identity exchange resolves after the identity changes', async () => {
  let complete: ((value: { data: { success: boolean } }) => void) | undefined
  const pending = new Promise<{ data: { success: boolean } }>((resolve) => { complete = resolve })
  const post = vi.spyOn(api, 'post').mockReturnValue(pending)
  const saved = vi.fn()
  render(<OAuthFixture saved={saved} />)
  fireEvent.change(screen.getByRole('textbox'), { target: { value: 'code=synthetic&state=synthetic' } })
  await userEvent.setup().click(screen.getByRole('button', { name: 'Complete login' }))
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
  await act(async () => { useAuthStore.getState().auth.setUser({ id: 2, role: ROLE.SUPER_ADMIN, username: 'synthetic-two' }) })
  await act(async () => { complete?.({ data: { success: true } }); await pending })
  expect(saved).not.toHaveBeenCalled()
  expect(screen.getByRole('dialog')).toBeInTheDocument()
})
