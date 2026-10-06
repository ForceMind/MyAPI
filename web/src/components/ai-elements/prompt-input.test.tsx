import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { PromptInput, usePromptInputAttachments } from './prompt-input'

function AttachmentControls() {
  const attachments = usePromptInputAttachments()
  return (
    <>
      <output>
        {attachments.files.map((file) => file.filename).join(',')}
      </output>
      {attachments.files.map((file) => (
        <button
          key={file.id}
          type='button'
          onClick={() => attachments.remove(file.id)}
        >
          Remove {file.id}
        </button>
      ))}
    </>
  )
}

describe('PromptInput attachments', () => {
  const createObjectURL = vi.fn()
  const revokeObjectURL = vi.fn()

  beforeEach(() => {
    vi.stubGlobal('URL', {
      createObjectURL,
      revokeObjectURL,
    })
  })

  afterEach(() => {
    vi.clearAllMocks()
    vi.unstubAllGlobals()
  })

  test('allows selecting the same file again and revokes every removed object URL', () => {
    createObjectURL
      .mockReturnValueOnce('blob:first')
      .mockReturnValueOnce('blob:second')
    render(
      <PromptInput onSubmit={vi.fn()}>
        <AttachmentControls />
      </PromptInput>
    )
    const input = screen.getByLabelText('Upload files')
    const file = new File(['content'], 'note.txt', { type: 'text/plain' })

    fireEvent.change(input, { target: { files: [file] } })
    fireEvent.change(input, { target: { files: [file] } })

    expect(screen.getByText('note.txt,note.txt')).toBeInTheDocument()
    expect(input).toHaveValue('')
    expect(createObjectURL).toHaveBeenCalledTimes(2)
    expect(revokeObjectURL).not.toHaveBeenCalled()

    for (const removeButton of screen.getAllByRole('button', {
      name: /^Remove /,
    })) {
      fireEvent.click(removeButton)
    }

    expect(revokeObjectURL).toHaveBeenCalledWith('blob:first')
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:second')
  })

  test('releases current local attachment URLs on unmount', () => {
    createObjectURL.mockReturnValue('blob:current')
    const view = render(
      <PromptInput onSubmit={vi.fn()}>
        <AttachmentControls />
      </PromptInput>
    )
    const input = screen.getByLabelText('Upload files')
    const file = new File(['content'], 'note.txt', { type: 'text/plain' })

    fireEvent.change(input, { target: { files: [file] } })
    view.unmount()

    expect(revokeObjectURL).toHaveBeenCalledWith('blob:current')
  })
})
