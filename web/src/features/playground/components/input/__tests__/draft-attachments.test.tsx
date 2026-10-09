/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import { DEFAULT_CONFIG, DEFAULT_PARAMETER_ENABLED } from '../../../constants'
import { PlaygroundInput } from '../playground-input'

const png = () =>
  new File([new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10])], 'photo.png', {
    type: 'image/png',
  })

function renderInput(
  options: {
    canSend?: boolean
    onSubmit?: (
      text: string,
      images?: import('../../../types').ChatAttachment[]
    ) => Promise<boolean>
  } = {}
) {
  const onSubmit = options.onSubmit ?? vi.fn().mockResolvedValue(true)
  const composer = (
    completedRetryDraft?: import('../../../types').PlaygroundDraftIdentity
  ) => (
    <PlaygroundInput
      completedRetryDraft={completedRetryDraft}
      config={{ ...DEFAULT_CONFIG, keyId: 8 }}
      canSend={options.canSend ?? true}
      keys={[
        {
          id: 8,
          name: 'Development',
          status: 1,
          group: 'default',
          remain_quota: 100,
          unlimited_quota: false,
          expired_time: -1,
        },
      ]}
      onKeyChange={() => undefined}
      onRefreshKeys={() => undefined}
      models={[{ label: 'vision', value: 'vision' }]}
      modelValue='vision'
      onModelChange={() => undefined}
      onConfigChange={() => undefined}
      onParameterEnabledChange={() => undefined}
      parameterEnabled={DEFAULT_PARAMETER_ENABLED}
      onSubmit={onSubmit}
    />
  )
  const rendered = render(composer())
  return {
    completedRetry: (draft: import('../../../types').PlaygroundDraftIdentity) =>
      rendered.rerender(composer(draft)),
    onSubmit,
    user: userEvent.setup(),
    input: screen.getByLabelText('Upload attachments'),
    text: screen.getByRole('textbox', { name: 'Message' }),
  }
}

async function attachImage(
  user: ReturnType<typeof userEvent.setup>,
  input: HTMLElement
) {
  await user.upload(input, png())
  await screen.findByRole('img', { name: 'photo.png' })
  await waitFor(() =>
    expect(screen.queryByText('Reading attachments...')).not.toBeInTheDocument()
  )
}

describe('controlled image draft', () => {
  test('image-only upload enables send, reaches the submit callback and clears after success', async () => {
    const { user, input, onSubmit } = renderInput()
    expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled()
    await attachImage(user, input)
    await user.click(screen.getByRole('button', { name: 'Send' }))
    expect(onSubmit).toHaveBeenCalledWith('', [
      expect.objectContaining({
        mimeType: 'image/png',
        dataUrl: 'data:image/png;base64,iVBORw0KGgo=',
      }),
    ])
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })

  test('failed or cancelled submission preserves both text and images for a deliberate retry', async () => {
    const onSubmit = vi.fn().mockResolvedValue(false)
    const { user, input, text } = renderInput({ onSubmit })
    await user.type(text, 'describe it')
    await attachImage(user, input)
    await user.click(screen.getByRole('button', { name: 'Send' }))
    expect(text).toHaveValue('describe it')
    expect(screen.getByRole('img', { name: 'photo.png' })).toBeInTheDocument()
    onSubmit.mockResolvedValue(true)
    await user.click(screen.getByRole('button', { name: 'Send' }))
    expect(onSubmit).toHaveBeenCalledTimes(2)
    expect(text).toHaveValue('')
  })

  test('rejected submission keeps the draft and displays the failure notice', async () => {
    const { user, text } = renderInput({
      onSubmit: vi.fn().mockRejectedValue(new Error('network')),
    })
    await user.type(text, 'keep my draft')
    await user.click(screen.getByRole('button', { name: 'Send' }))
    expect(text).toHaveValue('keep my draft')
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Your draft and attachments are still here'
    )
  })

  test('double-clicking send during a pending result dispatches only once', async () => {
    let resolveResult: (value: boolean) => void = () => undefined
    const onSubmit = vi.fn(
      () =>
        new Promise<boolean>((resolve) => {
          resolveResult = resolve
        })
    )
    const { user, text } = renderInput({ onSubmit })
    await user.type(text, 'once')
    await user.dblClick(screen.getByRole('button', { name: 'Send' }))
    expect(onSubmit).toHaveBeenCalledTimes(1)
    resolveResult(false)
    await waitFor(() => expect(text).toHaveValue('once'))
  })

  test('no selected usable key blocks both Send and Enter without dropping the draft', async () => {
    const { user, text, onSubmit } = renderInput({ canSend: false })
    await user.type(text, 'not authorized{Enter}')
    expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled()
    expect(onSubmit).not.toHaveBeenCalled()
    expect(text).toHaveValue('not authorized')
  })

  test('pasted image can be removed and the same file selected again', async () => {
    const { user, text, input } = renderInput()
    fireEvent.paste(text, {
      clipboardData: { items: [{ kind: 'file', getAsFile: png }] },
    })
    await screen.findByRole('img', { name: 'photo.png' })
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Remove attachment photo.png' })
      ).toBeEnabled()
    )
    await user.click(
      screen.getByRole('button', { name: 'Remove attachment photo.png' })
    )
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled()
    await attachImage(user, input)
    expect(screen.getByRole('img', { name: 'photo.png' })).toBeInTheDocument()
  })

  test('a failed replacement upload leaves already accepted images intact', async () => {
    const { user, input } = renderInput()
    await attachImage(user, input)
    await user.upload(
      input,
      new File(['bad signature'], 'bad.png', { type: 'image/png' })
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'does not match its type'
    )
    expect(screen.getAllByRole('img')).toHaveLength(1)
  })
})

describe('PDF draft presentation', () => {
  test('PDF uploads show a named file card and can be sent without accompanying text', async () => {
    const { user, input, onSubmit } = renderInput()
    await user.upload(
      input,
      new File(['%PDF-1.4\n'], 'notes.pdf', { type: 'application/pdf' })
    )
    const preview = await screen.findByRole('img', { name: 'notes.pdf' })
    expect(preview).toHaveTextContent('PDF')
    expect(preview.tagName).not.toBe('IFRAME')
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Send' })).toBeEnabled()
    )
    await user.click(screen.getByRole('button', { name: 'Send' }))
    expect(onSubmit).toHaveBeenCalledWith('', [
      expect.objectContaining({
        mimeType: 'application/pdf',
        name: 'notes.pdf',
        dataUrl: 'data:application/pdf;base64,JVBERi0xLjQK',
      }),
    ])
  })
})

describe('composer keyboard behavior', () => {
  test('Enter during IME composition does not submit until composition finishes', async () => {
    const { user, text, onSubmit } = renderInput()
    await user.type(text, '你好')
    fireEvent.compositionStart(text)
    fireEvent.keyDown(text, { key: 'Enter', isComposing: false })
    expect(onSubmit).not.toHaveBeenCalled()
    fireEvent.compositionEnd(text)
    fireEvent.keyDown(text, { key: 'Enter' })
    await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce())
  })

  test('Shift+Enter inserts a newline without dispatching', async () => {
    const { user, text, onSubmit } = renderInput()
    await user.type(text, 'first{Shift>}{Enter}{/Shift}second')
    expect(text).toHaveValue('first\nsecond')
    expect(onSubmit).not.toHaveBeenCalled()
  })
})

describe('successful retry composer reconciliation', () => {
  test('a successful retry clears only an unchanged failed draft and its attachments', async () => {
    const onSubmit = vi.fn().mockResolvedValue(false)
    const { user, text, input, completedRetry } = renderInput({ onSubmit })
    await user.type(text, 'failed question')
    await attachImage(user, input)
    await user.click(screen.getByRole('button', { name: 'Send' }))
    completedRetry({
      key: 'failed-user',
      text: 'failed question',
      attachmentIds: onSubmit.mock.calls[0][1].map(
        (attachment: import('../../../types').ChatAttachment) => attachment.id
      ),
    })
    await waitFor(() => expect(text).toHaveValue(''))
    expect(
      screen.queryByRole('img', { name: 'photo.png' })
    ).not.toBeInTheDocument()
    await user.type(text, 'failed question')
    expect(text).toHaveValue('failed question')
  })

  test('a successful retry does not clear a composer draft edited since the original failure', async () => {
    const onSubmit = vi.fn().mockResolvedValue(false)
    const { user, text, completedRetry } = renderInput({ onSubmit })
    await user.type(text, 'failed question')
    await user.click(screen.getByRole('button', { name: 'Send' }))
    await user.clear(text)
    await user.type(text, 'new question')
    completedRetry({
      key: 'failed-user',
      text: 'failed question',
      attachmentIds: [],
    })
    expect(text).toHaveValue('new question')
  })
})
