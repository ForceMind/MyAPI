import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { AccessPolicyError } from '../api'
import { AccessPolicyDialog } from '../components/access-policy-dialog'
import {
  authenticate,
  deferred,
  envelope,
  queryWrapper,
  tokenPolicy,
} from './fixtures'

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn(), delete: vi.fn() },
}))
const ownerAccess = {
  token_id: 7,
  assigned: true,
  user_revision: 1,
  token_revision: 2,
  models: [],
}
beforeEach(() => {
  vi.resetAllMocks()
  authenticate(1)
  vi.mocked(api.get).mockResolvedValue(envelope(ownerAccess))
})
afterEach(() => act(() => useAuthStore.getState().auth.reset()))

test('ordinary owners only request the redacted endpoint and have no assignment controls', async () => {
  vi.mocked(api.get).mockResolvedValue(
    envelope({
      ...ownerAccess,
      channel_ids: [998],
      upstream_models: ['private-upstream'],
      models: [
        {
          model: 'public-chat',
          allowed: false,
          reasons: [
            'upstream_model_denied',
            'channel_denied',
            'private-channel-998',
          ],
        },
      ],
    })
  )
  render(<AccessPolicyDialog tokenId={7} onClose={vi.fn()} />, queryWrapper())
  expect(await screen.findByText('public-chat')).toBeVisible()
  expect(api.get).toHaveBeenCalledWith(
    '/api/token/7/access',
    expect.objectContaining({
      disableDuplicate: true,
      signal: expect.any(AbortSignal),
    })
  )
  expect(
    screen.getByText('The upstream model is outside the assigned scope.')
  ).toBeVisible()
  expect(
    screen.getByText('Access is unavailable. Ask an administrator.')
  ).toBeVisible()
  expect(
    screen.queryByText(/private-upstream|private-channel-998/)
  ).not.toBeInTheDocument()
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Save assignment' })
  ).not.toBeInTheDocument()
})

test('an empty owner model list is explicit rather than implying access is granted', async () => {
  render(<AccessPolicyDialog tokenId={7} onClose={vi.fn()} />, queryWrapper())
  expect(await screen.findByText('No public models to display.')).toBeVisible()
  expect(screen.queryByText('Available')).not.toBeInTheDocument()
})

test('a failed owner refresh removes stale availability rather than leaving it presented as current', async () => {
  const user = userEvent.setup()
  vi.mocked(api.get)
    .mockResolvedValueOnce(
      envelope({
        ...ownerAccess,
        models: [{ model: 'public-old', allowed: true, reasons: [] }],
      })
    )
    .mockRejectedValueOnce(new AccessPolicyError('forbidden'))
  render(<AccessPolicyDialog tokenId={7} onClose={vi.fn()} />, queryWrapper())
  expect(await screen.findByText('public-old')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Refresh' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'You do not have permission'
  )
  expect(screen.queryByText('public-old')).not.toBeInTheDocument()
})

test('ordinary users cannot read user defaults by mounting the admin entrypoint', async () => {
  render(<AccessPolicyDialog userId={2} onClose={vi.fn()} />, queryWrapper())
  expect(screen.getByRole('alert')).toHaveTextContent('Only administrators')
  expect(api.get).not.toHaveBeenCalled()
})

test('changing the session for the same user cancels old reads and never displays their stale results', async () => {
  const oldRead = deferred<ReturnType<typeof envelope>>()
  vi.mocked(api.get)
    .mockReturnValueOnce(oldRead.promise)
    .mockResolvedValueOnce(
      envelope({
        ...ownerAccess,
        models: [{ model: 'new-session-model', allowed: true, reasons: [] }],
      })
    )
  render(<AccessPolicyDialog tokenId={7} onClose={vi.fn()} />, queryWrapper())
  await waitFor(() => expect(api.get).toHaveBeenCalledTimes(1))
  const signal = vi.mocked(api.get).mock.calls[0]?.[1]?.signal
  act(() => authenticate(1, 1, 'new-session'))
  expect(await screen.findByText('new-session-model')).toBeVisible()
  expect(signal?.aborted).toBe(true)
  await act(async () =>
    oldRead.resolve(
      envelope({
        ...ownerAccess,
        models: [{ model: 'stale-private-model', allowed: true, reasons: [] }],
      })
    )
  )
  expect(screen.queryByText('stale-private-model')).not.toBeInTheDocument()
  expect(screen.getByText('new-session-model')).toBeVisible()
})

test('switching accounts replaces the owner query rather than reusing the previous account cache', async () => {
  vi.mocked(api.get)
    .mockResolvedValueOnce(
      envelope({
        ...ownerAccess,
        models: [{ model: 'old-account-model', allowed: true, reasons: [] }],
      })
    )
    .mockResolvedValueOnce(envelope({ ...ownerAccess, models: [] }))
  render(<AccessPolicyDialog tokenId={7} onClose={vi.fn()} />, queryWrapper())
  await screen.findByText('old-account-model')
  act(() => authenticate(1, 2, 'other-account'))
  expect(await screen.findByText('No public models to display.')).toBeVisible()
  expect(screen.queryByText('old-account-model')).not.toBeInTheDocument()
  expect(api.get).toHaveBeenCalledTimes(2)
})

function ClosableDialog(props: { onSaved: () => void }) {
  const [open, setOpen] = useState(true)
  return open ? (
    <AccessPolicyDialog
      tokenId={7}
      onClose={() => setOpen(false)}
      onSaved={props.onSaved}
    />
  ) : (
    <p>Closed fixture</p>
  )
}

test('closing a pending write aborts its signal and suppresses late success effects', async () => {
  authenticate(100)
  const user = userEvent.setup()
  const write = deferred<ReturnType<typeof envelope>>()
  const saved = vi.fn()
  vi.mocked(api.get).mockImplementation(async (url) =>
    envelope(url === '/api/token/7/access' ? ownerAccess : tokenPolicy)
  )
  vi.mocked(api.put).mockReturnValue(write.promise)
  render(<ClosableDialog onSaved={saved} />, queryWrapper())
  await user.click(
    await screen.findByRole('button', { name: 'Save assignment' })
  )
  const signal = vi.mocked(api.put).mock.calls[0]?.[2]?.signal
  await user.click(screen.getByRole('button', { name: 'Close' }))
  expect(screen.getByText('Closed fixture')).toBeVisible()
  expect(signal?.aborted).toBe(true)
  await act(async () =>
    write.resolve(envelope({ ...tokenPolicy, assigned: true, revision: 1 }))
  )
  expect(saved).not.toHaveBeenCalled()
  expect(screen.queryByText('Assignment saved.')).not.toBeInTheDocument()
})

test('the narrow dialog scrolls within the viewport and Close is keyboard reachable', async () => {
  const user = userEvent.setup()
  const close = vi.fn()
  render(<AccessPolicyDialog tokenId={7} onClose={close} />, queryWrapper())
  await screen.findByText('No public models to display.')
  const dialog = screen.getByRole('dialog', { name: 'Assigned access' })
  expect(dialog).toHaveClass('min-w-0', 'overflow-y-auto', 'max-h-[85dvh]')
  screen.getByRole('button', { name: 'Refresh' }).focus()
  await user.tab()
  expect(screen.getByRole('button', { name: 'Close' })).toHaveFocus()
  await user.keyboard('{Enter}')
  expect(close).toHaveBeenCalledTimes(1)
})

test('Escape dismisses an owner dialog even while a read is pending', async () => {
  const user = userEvent.setup()
  const close = vi.fn()
  vi.mocked(api.get).mockReturnValue(
    deferred<ReturnType<typeof envelope>>().promise
  )
  render(<AccessPolicyDialog tokenId={7} onClose={close} />, queryWrapper())
  expect(await screen.findByRole('status')).toHaveTextContent('Loading...')
  await user.keyboard('{Escape}')
  expect(close).toHaveBeenCalledTimes(1)
})

test('prototype-named and unknown denial codes render safe text rather than server content', async () => {
  vi.mocked(api.get).mockResolvedValue(
    envelope({
      ...ownerAccess,
      models: [
        {
          model: 'public-chat',
          allowed: false,
          reasons: ['__proto__', 'constructor', 'private upstream 999'],
        },
      ],
    })
  )
  render(<AccessPolicyDialog tokenId={7} onClose={vi.fn()} />, queryWrapper())
  expect(await screen.findByText('public-chat')).toBeVisible()
  expect(
    screen.getAllByText('Access is unavailable. Ask an administrator.')
  ).toHaveLength(3)
  expect(screen.queryByText('private upstream 999')).not.toBeInTheDocument()
})

test('a role downgrade replaces the admin editor with the redacted owner view', async () => {
  authenticate(100)
  vi.mocked(api.get).mockImplementation(async (url) =>
    envelope(
      url === '/api/token/7/access'
        ? ownerAccess
        : { ...tokenPolicy, upstream_models: ['private-upstream'] }
    )
  )
  render(<AccessPolicyDialog tokenId={7} onClose={vi.fn()} />, queryWrapper())
  await screen.findByRole('textbox', { name: 'Upstream model scope' })
  act(() => authenticate(1, 1, 'test-session'))
  expect(await screen.findByText('No public models to display.')).toBeVisible()
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Save assignment' })
  ).not.toBeInTheDocument()
  expect(vi.mocked(api.get).mock.calls.at(-1)?.[0]).toBe('/api/token/7/access')
})

test('an admin whose own Key policy is forbidden still sees owner-safe availability without edit controls', async () => {
  authenticate(10)
  vi.mocked(api.get).mockImplementation(async (url) => {
    if (url === '/api/token/7/access') {
      return envelope({
        ...ownerAccess,
        models: [{ model: 'own-key-public', allowed: true, reasons: [] }],
      })
    }
    throw new AccessPolicyError('access_policy_forbidden')
  })
  render(<AccessPolicyDialog tokenId={7} onClose={vi.fn()} />, queryWrapper())
  expect(await screen.findByText('own-key-public')).toBeVisible()
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'You do not have permission to view or change this access policy.'
  )
  expect(screen.getByRole('region', { name: 'Admin assignment' })).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Save assignment' })
  ).not.toBeInTheDocument()
  expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
  expect(api.put).not.toHaveBeenCalled()
})
