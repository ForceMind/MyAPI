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
import { describe, expect, test } from 'vitest'

import { DEFAULT_CONFIG, DEFAULT_PARAMETER_ENABLED } from '../../../constants'
import { appendUserMessagePair } from '../../message/conversation-message-utils'
import { buildChatCompletionPayload } from '../../streaming/payload-builder'
import {
  MAX_ATTACHMENT_BYTES,
  MAX_TOTAL_ATTACHMENT_BYTES,
  readChatAttachment,
  validateConversationAttachments,
  validateAttachmentFiles,
} from '../chat-attachments'

const png = new File(
  [new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10])],
  'photo.png',
  { type: 'image/png' }
)

describe('image attachment boundaries', () => {
  test('an image-only turn creates actual inline image_url API content', async () => {
    const image = await readChatAttachment(png)
    const messages = appendUserMessagePair([], '', [image])
    const payload = buildChatCompletionPayload(
      messages,
      DEFAULT_CONFIG,
      DEFAULT_PARAMETER_ENABLED
    )
    expect(payload.messages).toEqual([
      {
        role: 'user',
        content: [
          { type: 'text', text: '' },
          {
            type: 'image_url',
            image_url: { url: 'data:image/png;base64,iVBORw0KGgo=' },
          },
        ],
      },
    ])
    expect(payload).not.toHaveProperty('group')
  })

  test('a disguised image is rejected before it becomes an attachment', async () => {
    await expect(
      readChatAttachment(
        new File(['not an image'], 'fake.png', { type: 'image/png' })
      )
    ).rejects.toThrow('The file content does not match its type')
  })

  test('unsupported MIME rejects the entire selected batch instead of dropping files', () => {
    expect(() =>
      validateAttachmentFiles(
        [png, new File(['<svg/>'], 'image.svg', { type: 'image/svg+xml' })],
        []
      )
    ).toThrow('Only PNG, JPEG, WEBP, GIF and PDF files are supported')
  })

  test('empty and oversize image files cannot enter the draft', () => {
    expect(() =>
      validateAttachmentFiles(
        [new File([], 'empty.png', { type: 'image/png' })],
        []
      )
    ).toThrow('Each file')
    expect(() =>
      validateAttachmentFiles(
        [
          new File([new Uint8Array(MAX_ATTACHMENT_BYTES + 1)], 'large.png', {
            type: 'image/png',
          }),
        ],
        []
      )
    ).toThrow('Each file')
  })

  test('exact limits pass while accumulated draft size and count overflow fail', async () => {
    const image = await readChatAttachment(png)
    expect(() =>
      validateAttachmentFiles(
        [
          new File([new Uint8Array(MAX_ATTACHMENT_BYTES)], 'max.png', {
            type: 'image/png',
          }),
        ],
        [{ ...image, size: MAX_ATTACHMENT_BYTES }]
      )
    ).not.toThrow()
    expect(() =>
      validateAttachmentFiles(
        [png],
        [{ ...image, size: MAX_TOTAL_ATTACHMENT_BYTES }]
      )
    ).toThrow('20 MiB')
    expect(() =>
      validateAttachmentFiles([png], [image, image, image, image])
    ).toThrow('4 files')
  })

  test('earlier conversation images count toward the request limit', async () => {
    const image = await readChatAttachment(png)
    const previous = appendUserMessagePair([], 'earlier', [
      image,
      image,
      image,
      image,
    ])
    expect(() =>
      validateConversationAttachments(
        appendUserMessagePair(previous, 'next', [image])
      )
    ).toThrow('This conversation exceeds')
  })

  test('unavailable images block reuse instead of silently sending text only', () => {
    const messages = appendUserMessagePair([], 'describe this')
    messages[0].missingAttachments = true
    expect(() => validateConversationAttachments(messages)).toThrow(
      'Attachments from a previous session are unavailable'
    )
  })
})

describe('inline PDF boundary', () => {
  test('a PDF-only turn reaches the API as filename and inline file_data without a file_id', async () => {
    const file = new File(['%PDF-1.4\n'], 'notes.pdf', {
      type: 'application/pdf',
    })
    validateAttachmentFiles([file], [])
    const attachment = await readChatAttachment(file)
    const payload = buildChatCompletionPayload(
      appendUserMessagePair([], '', [attachment]),
      DEFAULT_CONFIG,
      DEFAULT_PARAMETER_ENABLED
    )
    expect(payload.messages).toEqual([
      {
        role: 'user',
        content: [
          { type: 'text', text: '' },
          {
            type: 'file',
            file: {
              filename: 'notes.pdf',
              file_data: 'data:application/pdf;base64,JVBERi0xLjQK',
            },
          },
        ],
      },
    ])
    expect(JSON.stringify(payload)).not.toContain('file_id')
  })

  test('a renamed or forged PDF cannot become a supported inline file', async () => {
    expect(() =>
      validateAttachmentFiles(
        [new File(['%PDF-1.4'], 'notes.txt', { type: 'application/pdf' })],
        []
      )
    ).toThrow('PDF filenames')
    await expect(
      readChatAttachment(
        new File(['plain text'], 'notes.pdf', { type: 'application/pdf' })
      )
    ).rejects.toThrow('does not match its type')
  })

  test('PDF filenames with paths or excessive UTF-8 bytes are rejected before sending', () => {
    for (const name of [
      '../notes.pdf',
      'a\\notes.pdf',
      `${'文'.repeat(85)}.pdf`,
    ]) {
      expect(() =>
        validateAttachmentFiles(
          [new File(['%PDF-1.4'], name, { type: 'application/pdf' })],
          []
        )
      ).toThrow('PDF filenames')
    }
  })

  test('PDF and images share the same conversation count and byte budget', async () => {
    const attachment = await readChatAttachment(
      new File(['%PDF-1.4'], 'notes.pdf', { type: 'application/pdf' })
    )
    const image = await readChatAttachment(png)
    expect(() =>
      validateConversationAttachments(
        appendUserMessagePair([], '', [attachment, image, image, image, image])
      )
    ).toThrow('4 attachments')
    expect(() =>
      validateConversationAttachments(
        appendUserMessagePair([], '', [
          { ...attachment, size: MAX_TOTAL_ATTACHMENT_BYTES },
          image,
        ])
      )
    ).toThrow('20 MiB')
  })

  test('plain text and DOCX remain unsupported rather than being disguised as PDF', () => {
    for (const type of [
      'text/plain',
      'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    ]) {
      expect(() =>
        validateAttachmentFiles([new File(['content'], 'notes', { type })], [])
      ).toThrow('Only PNG')
    }
  })
})
