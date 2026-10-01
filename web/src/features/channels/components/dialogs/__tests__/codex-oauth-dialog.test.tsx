import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { completeCodexOAuth } from '../../../api'
import { CodexOAuthDialog } from '../codex-oauth-dialog'

vi.mock('../../../api', () => ({
  completeCodexOAuth: vi.fn(),
  startCodexOAuth: vi.fn(),
}))
vi.mock('sonner', () => ({
  toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))
afterEach(() => vi.clearAllMocks())

test('new-channel login sends validated creation fields and accepts a channel ID without receiving credentials', async () => {
  const payload = {
    mode: 'single' as const,
    channel: { name: 'Synthetic Codex', type: 57, key: '' },
  }
  vi.mocked(completeCodexOAuth).mockResolvedValue({
    success: true,
    data: { channel_id: 8 },
  })
  const saved = vi.fn()
  const close = vi.fn()
  render(
    <CodexOAuthDialog
      open
      onOpenChange={close}
      getCreatePayload={vi.fn().mockResolvedValue(payload)}
      onCredentialSaved={saved}
    />
  )
  fireEvent.change(screen.getByRole('textbox'), {
    target: { value: 'code=fixture&state=fixture' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Complete login' }))
  await waitFor(() => expect(saved).toHaveBeenCalledOnce())
  expect(completeCodexOAuth).toHaveBeenCalledWith(
    'code=fixture&state=fixture',
    undefined,
    payload
  )
  expect(close).toHaveBeenCalledWith(false)
})

test('invalid creation fields keep the dialog open and do not exchange the authorization code', async () => {
  const close = vi.fn()
  render(
    <CodexOAuthDialog
      open
      onOpenChange={close}
      getCreatePayload={vi.fn().mockResolvedValue(null)}
    />
  )
  fireEvent.change(screen.getByRole('textbox'), {
    target: { value: 'code=fixture&state=fixture' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Complete login' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Complete login' })).toBeEnabled()
  )
  expect(completeCodexOAuth).not.toHaveBeenCalled()
  expect(close).not.toHaveBeenCalled()
})
