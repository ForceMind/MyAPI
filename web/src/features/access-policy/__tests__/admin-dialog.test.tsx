import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AxiosError, AxiosHeaders } from 'axios'
import { toast } from 'sonner'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

import { AccessPolicyError } from '../api'
import { AccessPolicyDialog } from '../components/access-policy-dialog'
import {
  authenticate,
  deferred,
  envelope,
  queryWrapper,
  userPolicy,
} from './fixtures'

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn(), delete: vi.fn() },
}))
beforeEach(() => {
  vi.resetAllMocks()
  authenticate()
  vi.mocked(api.get).mockResolvedValue(envelope(userPolicy))
})
afterEach(() => act(() => useAuthStore.getState().auth.reset()))
function renderDialog(onSaved = vi.fn()) {
  return render(
    <AccessPolicyDialog userId={2} onClose={vi.fn()} onSaved={onSaved} />,
    queryWrapper()
  )
}

test('opening an unassigned user keeps legacy access and does not write automatically', async () => {
  renderDialog()
  expect(
    await screen.findByText('No assignment. Existing access rules apply.')
  ).toBeVisible()
  expect(
    screen.getAllByRole('checkbox', { name: 'Inherit existing scope' })
  ).toHaveLength(3)
  expect(api.put).not.toHaveBeenCalled()
  expect(api.post).not.toHaveBeenCalled()
  expect(
    screen.queryByRole('button', { name: 'Remove assignment' })
  ).not.toBeInTheDocument()
})

test('previewing explicit empty public scope is read-only and changing the draft clears stale results', async () => {
  const user = userEvent.setup()
  vi.mocked(api.post).mockResolvedValue(
    envelope({
      policy: { ...userPolicy, assigned: true, public_models: [] },
      models: [
        {
          model: 'public-chat',
          allowed: false,
          reasons: ['public_model_denied'],
        },
      ],
    })
  )
  renderDialog()
  const group = await screen.findByRole('group', { name: 'Public model scope' })
  await user.click(within(group).getByRole('checkbox'))
  await user.click(screen.getByRole('button', { name: 'Preview access' }))
  expect(
    await screen.findByText('This public model is outside the assigned scope.')
  ).toBeVisible()
  expect(api.post).toHaveBeenCalledWith(
    '/api/access-policy/user/2/preview',
    {
      enabled: true,
      public_models: [],
      upstream_models: null,
      channel_ids: null,
    },
    expect.any(Object)
  )
  expect(api.put).not.toHaveBeenCalled()
  await user.type(
    screen.getByRole('textbox', { name: 'Public model scope' }),
    'public-chat'
  )
  expect(screen.queryByText('public-chat')).not.toBeInTheDocument()
})

test('repeated save clicks submit one full replacement and lock fields while it is pending', async () => {
  const user = userEvent.setup()
  const write = deferred<ReturnType<typeof envelope>>()
  vi.mocked(api.put).mockReturnValue(write.promise)
  renderDialog()
  const save = await screen.findByRole('button', { name: 'Save assignment' })
  await user.dblClick(save)
  expect(api.put).toHaveBeenCalledTimes(1)
  expect(save).toBeDisabled()
  expect(
    screen.getByRole('checkbox', { name: 'Enable assigned access' })
  ).toHaveAttribute('aria-disabled', 'true')
  expect(api.put).toHaveBeenCalledWith(
    '/api/access-policy/user/2',
    {
      enabled: true,
      expected_revision: 0,
      public_models: null,
      upstream_models: null,
      channel_ids: null,
    },
    expect.any(Object)
  )
  await act(async () =>
    write.resolve(envelope({ ...userPolicy, assigned: true, revision: 1 }))
  )
  expect(await screen.findByText('Assignment saved.')).toBeVisible()
})

test('a revision conflict blocks stale writes until an explicit refresh loads the current revision', async () => {
  const user = userEvent.setup()
  vi.mocked(api.put).mockRejectedValue(
    new AccessPolicyError('revision_conflict')
  )
  renderDialog()
  await user.click(
    await screen.findByRole('button', { name: 'Save assignment' })
  )
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'This policy changed elsewhere.'
  )
  expect(screen.getByRole('button', { name: 'Save assignment' })).toBeDisabled()
  vi.mocked(api.get).mockResolvedValue(
    envelope({
      ...userPolicy,
      revision: 9,
      assigned: true,
      public_models: ['updated-public'],
    })
  )
  await user.click(screen.getByRole('button', { name: 'Refresh policy' }))
  expect(
    await screen.findByRole('textbox', { name: 'Public model scope' })
  ).toHaveValue('updated-public')
  expect(screen.getByRole('button', { name: 'Save assignment' })).toBeEnabled()
  vi.mocked(api.put).mockResolvedValue(
    envelope({
      ...userPolicy,
      revision: 10,
      assigned: true,
      public_models: ['updated-public'],
    })
  )
  await user.click(screen.getByRole('button', { name: 'Save assignment' }))
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(2))
  expect(vi.mocked(api.put).mock.calls[1]?.[1]).toMatchObject({
    expected_revision: 9,
  })
})

test('removal requires acknowledgment and uses the current assignment revision', async () => {
  const user = userEvent.setup()
  vi.mocked(api.get).mockResolvedValue(
    envelope({ ...userPolicy, assigned: true, revision: 4 })
  )
  vi.mocked(api.delete).mockResolvedValue(
    envelope({ ...userPolicy, revision: 5 })
  )
  renderDialog()
  const remove = await screen.findByRole('button', {
    name: 'Remove assignment',
  })
  expect(remove).toBeDisabled()
  await user.click(
    screen.getByRole('checkbox', {
      name: 'I understand that removing this assignment may broaden access.',
    })
  )
  await user.click(remove)
  await waitFor(() =>
    expect(api.delete).toHaveBeenCalledWith(
      '/api/access-policy/user/2',
      expect.objectContaining({ data: { expected_revision: 4 } })
    )
  )
  expect(
    await screen.findByText('No assignment. Existing access rules apply.')
  ).toBeVisible()
})

test('a typed Key belonging to another user is rejected without exposing or saving its scope', async () => {
  const user = userEvent.setup()
  renderDialog()
  await screen.findByRole('button', { name: 'Save assignment' })
  await user.selectOptions(
    screen.getByRole('combobox', { name: 'Assignment target' }),
    'token'
  )
  await user.type(screen.getByRole('textbox', { name: 'Key ID' }), '99')
  vi.mocked(api.get).mockResolvedValue(
    envelope({
      ...userPolicy,
      subject: 'token',
      subject_id: 99,
      owner_user_id: 3,
      upstream_models: ['other-private-scope'],
    })
  )
  await user.click(screen.getByRole('button', { name: 'Load Key policy' }))
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'The policy does not belong to the selected user or Key.'
  )
  expect(
    screen.queryByRole('button', { name: 'Save assignment' })
  ).not.toBeInTheDocument()
  expect(screen.queryByText('other-private-scope')).not.toBeInTheDocument()
})

test('a typed Key owned by the selected user edits the token policy rather than the user default', async () => {
  const user = userEvent.setup()
  renderDialog()
  await screen.findByRole('button', { name: 'Save assignment' })
  await user.selectOptions(
    screen.getByRole('combobox', { name: 'Assignment target' }),
    'token'
  )
  await user.type(screen.getByRole('textbox', { name: 'Key ID' }), '99')
  const key = { ...userPolicy, subject: 'token', subject_id: 99, revision: 3 }
  vi.mocked(api.get).mockResolvedValue(envelope(key))
  vi.mocked(api.put).mockResolvedValue(
    envelope({ ...key, assigned: true, revision: 4 })
  )
  await user.click(screen.getByRole('button', { name: 'Load Key policy' }))
  await user.click(
    await screen.findByRole('button', { name: 'Save assignment' })
  )
  expect(api.put).toHaveBeenCalledWith(
    '/api/access-policy/token/99',
    expect.objectContaining({ expected_revision: 3 }),
    expect.any(Object)
  )
})

test('invalid channel IDs are associated with an invalid field and never sent', async () => {
  const user = userEvent.setup()
  renderDialog()
  const group = await screen.findByRole('group', { name: 'Channel ID scope' })
  await user.click(within(group).getByRole('checkbox'))
  await user.type(
    screen.getByRole('textbox', { name: 'Channel ID scope' }),
    '1e4'
  )
  await user.click(screen.getByRole('button', { name: 'Save assignment' }))
  expect(
    await screen.findByText(
      'Enter positive channel IDs separated by commas or new lines.'
    )
  ).toBeVisible()
  expect(
    screen.getByRole('textbox', { name: 'Channel ID scope' })
  ).toHaveAttribute('aria-invalid', 'true')
  expect(api.put).not.toHaveBeenCalled()
})

test('forbidden reads show only a safe error and offer refresh without mutation controls', async () => {
  vi.mocked(api.get).mockRejectedValue(new AccessPolicyError('forbidden'))
  renderDialog()
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'You do not have permission'
  )
  expect(
    screen.queryByRole('button', { name: 'Save assignment' })
  ).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Refresh policy' })).toBeEnabled()
})

test('turning off assigned access saves a denial rather than removing the assignment', async () => {
  const user = userEvent.setup()
  vi.mocked(api.put).mockResolvedValue(
    envelope({ ...userPolicy, assigned: true, enabled: false, revision: 1 })
  )
  renderDialog()
  await user.click(
    await screen.findByRole('checkbox', { name: 'Enable assigned access' })
  )
  await user.click(screen.getByRole('button', { name: 'Save assignment' }))
  await waitFor(() =>
    expect(api.put).toHaveBeenCalledWith(
      '/api/access-policy/user/2',
      expect.objectContaining({ enabled: false }),
      expect.any(Object)
    )
  )
  expect(api.delete).not.toHaveBeenCalled()
  expect(
    screen.getByText(
      'A disabled assignment denies all access. Inherited dimensions keep existing restrictions.'
    )
  ).toBeVisible()
})

test('editing the typed Key ID cancels its pending read and never exposes the former Key controls', async () => {
  const user = userEvent.setup()
  const read = deferred<ReturnType<typeof envelope>>()
  renderDialog()
  await screen.findByRole('button', { name: 'Save assignment' })
  await user.selectOptions(
    screen.getByRole('combobox', { name: 'Assignment target' }),
    'token'
  )
  const input = screen.getByRole('textbox', { name: 'Key ID' })
  await user.type(input, '99')
  vi.mocked(api.get).mockReturnValueOnce(read.promise)
  await user.click(screen.getByRole('button', { name: 'Load Key policy' }))
  const signal = vi.mocked(api.get).mock.calls[1]?.[1]?.signal
  await user.clear(input)
  await user.type(input, '100')
  expect(signal?.aborted).toBe(true)
  await act(async () =>
    read.resolve(
      envelope({
        ...userPolicy,
        subject: 'token',
        subject_id: 99,
        public_models: ['stale-key-public'],
      })
    )
  )
  expect(
    screen.queryByRole('button', { name: 'Save assignment' })
  ).not.toBeInTheDocument()
  expect(screen.queryByText('stale-key-public')).not.toBeInTheDocument()
})

test('an invalid typed Key ID never loads a policy', async () => {
  const user = userEvent.setup()
  renderDialog()
  await screen.findByRole('button', { name: 'Save assignment' })
  await user.selectOptions(
    screen.getByRole('combobox', { name: 'Assignment target' }),
    'token'
  )
  await user.type(screen.getByRole('textbox', { name: 'Key ID' }), '1e3')
  await user.click(screen.getByRole('button', { name: 'Load Key policy' }))
  expect(await screen.findByText('Enter a positive Key ID.')).toBeVisible()
  expect(screen.getByRole('textbox', { name: 'Key ID' })).toHaveAttribute(
    'aria-invalid',
    'true'
  )
  expect(api.get).toHaveBeenCalledTimes(1)
})

test('an HTTP conflict uses only the inline policy error instead of the application default toast and raw error log', async () => {
  const user = userEvent.setup()
  const context = queryWrapper()
  context.client.setDefaultOptions({
    queries: { retry: false },
    mutations: { retry: false, onError: handleServerError },
  })
  const config = {
    headers: new AxiosHeaders(),
    method: 'put',
    url: '/api/access-policy/user/2',
    skipErrorHandler: true,
  }
  const error = new AxiosError(
    'Synthetic access-policy conflict',
    AxiosError.ERR_BAD_REQUEST,
    config,
    undefined,
    {
      config,
      headers: new AxiosHeaders(),
      status: 409,
      statusText: 'Conflict',
      data: {
        success: false,
        code: 'access_policy_conflict',
        message: 'Synthetic raw server detail',
      },
    }
  )
  vi.mocked(api.put).mockRejectedValue(error)
  const toastError = vi.spyOn(toast, 'error')
  const consoleLog = vi
    .spyOn(console, 'log')
    .mockImplementation(() => undefined)
  const consoleError = vi
    .spyOn(console, 'error')
    .mockImplementation(() => undefined)
  const view = render(
    <AccessPolicyDialog userId={2} onClose={vi.fn()} />,
    context
  )
  try {
    await user.click(
      await screen.findByRole('button', { name: 'Save assignment' })
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'This policy changed elsewhere. Refresh and review before saving again.'
    )
    const save = screen.getByRole('button', { name: 'Save assignment' })
    expect(save).toBeDisabled()
    expect(
      screen.getByRole('checkbox', { name: 'Enable assigned access' })
    ).toHaveAttribute('aria-disabled', 'true')
    expect(screen.getByRole('button', { name: 'Refresh policy' })).toBeEnabled()
    await user.click(save)
    expect(api.put).toHaveBeenCalledTimes(1)
    expect(
      screen.queryByText('Synthetic raw server detail')
    ).not.toBeInTheDocument()
    expect.soft(toastError).not.toHaveBeenCalled()
    expect.soft(consoleLog).not.toHaveBeenCalled()
    expect.soft(consoleError).not.toHaveBeenCalled()
  } finally {
    view.unmount()
    context.client.clear()
    toast.dismiss()
    toastError.mockRestore()
    consoleLog.mockRestore()
    consoleError.mockRestore()
  }
})
