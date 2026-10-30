/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { CONTENT_PREVIEW_LIMIT } from '../../lib/content-preview'
import { ContentPanel } from '../content-panel'

// Only the browser clipboard is replaced; formatting and the copy hook are real.
afterEach(() => vi.restoreAllMocks())

describe('saved content panel', () => {
  test('original mode preserves JSON spacing and copies the exact saved representation', async () => {
    const user = userEvent.setup()
    const writeText = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined)
    const body = '\uFEFF{ "sequence": 9007199254740993, "value": 1.0 }'
    render(<ContentPanel title='Saved body' body={body} encoding='json' emptyText='Empty' preserveOriginal />)

    const panel = screen.getByRole('region', { name: 'Saved body' })
    expect(panel.querySelector('pre')?.textContent).toBe(body)
    await user.click(within(panel).getByRole('button', { name: 'Copy' }))
    expect(writeText).toHaveBeenCalledWith(body)
  })

  test('a one-MB UI preview keeps the full loaded copy and separates server truncation', async () => {
    const user = userEvent.setup()
    const writeText = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined)
    const body = 'x'.repeat(CONTENT_PREVIEW_LIMIT - 1) + '😀saved-tail'
    render(<ContentPanel title='Saved body' body={body} emptyText='Empty' preserveOriginal truncated totalBytes={new TextEncoder().encode(body).length + 10} />)

    const panel = screen.getByRole('region', { name: 'Saved body' })
    expect(panel.querySelector('pre')?.textContent).toHaveLength(CONTENT_PREVIEW_LIMIT - 1)
    expect(panel.querySelector('pre')).not.toHaveTextContent('saved-tail')
    expect(screen.getByText('The saved response returned by the server is truncated.', { exact: false })).toBeInTheDocument()
    await user.click(within(panel).getByRole('button', { name: 'Copy' }))
    expect(writeText).toHaveBeenCalledWith(body)
  })

  test('a UI-only preview retains the recorded full byte size without claiming server truncation', () => {
    const totalBytes = CONTENT_PREVIEW_LIMIT * 2
    const body = 'x'.repeat(totalBytes)
    render(<ContentPanel title='Saved body' body={body} emptyText='Empty' preserveOriginal totalBytes={totalBytes} />)

    const panel = screen.getByRole('region', { name: 'Saved body' })
    expect(panel.querySelector('pre')?.textContent).toHaveLength(CONTENT_PREVIEW_LIMIT)
    expect(within(panel).getByText('Copy includes all loaded content, beyond this display preview.', { exact: false })).toHaveTextContent('2.00 MiB')
    expect(screen.queryByText('The saved response returned by the server is truncated.', { exact: false })).not.toBeInTheDocument()
  })

  test('line wrapping has a keyboard-operable pressed state and empty content cannot be copied', async () => {
    const user = userEvent.setup()
    render(<ContentPanel title='Saved body' body='' emptyText='Nothing saved' defaultWrap={false} />)

    expect(screen.getByRole('button', { name: 'Copy' })).toBeDisabled()
    const wrap = screen.getByRole('button', { name: 'Wrap lines' })
    expect(wrap).toHaveAttribute('aria-pressed', 'false')
    wrap.focus()
    await user.keyboard('{Enter}')
    expect(wrap).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByText('Encoding not recorded')).toBeInTheDocument()
  })
})
