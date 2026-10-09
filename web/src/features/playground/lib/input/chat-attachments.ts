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
import { nanoid } from 'nanoid'

import type { ChatAttachment, Message } from '../../types'

export const IMAGE_MIME_TYPES = [
  'image/png',
  'image/jpeg',
  'image/webp',
  'image/gif',
]
export const ATTACHMENT_MIME_TYPES = [...IMAGE_MIME_TYPES, 'application/pdf']
export const ATTACHMENT_ACCEPT = ATTACHMENT_MIME_TYPES.join(',')
export const MAX_ATTACHMENT_COUNT = 4
export const MAX_ATTACHMENT_BYTES = 10 * 1024 * 1024
export const MAX_TOTAL_ATTACHMENT_BYTES = 20 * 1024 * 1024

export function validateAttachmentFiles(
  files: readonly File[],
  existing: readonly ChatAttachment[]
): void {
  if (files.length + existing.length > MAX_ATTACHMENT_COUNT) {
    throw new Error('You can attach up to 4 files')
  }
  for (const file of files) {
    if (!ATTACHMENT_MIME_TYPES.includes(file.type)) {
      throw new Error('Only PNG, JPEG, WEBP, GIF and PDF files are supported')
    }
    if (
      file.type === 'application/pdf' &&
      (!file.name.toLowerCase().endsWith('.pdf') ||
        new TextEncoder().encode(file.name).length > 255 ||
        ['\0', '\r', '\n', '/', '\\'].some((character) =>
          file.name.includes(character)
        ))
    ) {
      throw new Error(
        'PDF filenames must end in .pdf, fit within 255 bytes and contain no path separators'
      )
    }
    if (file.size === 0 || file.size > MAX_ATTACHMENT_BYTES) {
      throw new Error('Each file must be non-empty and no larger than 10 MiB')
    }
  }
  const total = [...existing, ...files].reduce(
    (size, file) => size + file.size,
    0
  )
  if (total > MAX_TOTAL_ATTACHMENT_BYTES) {
    throw new Error('Attachments must total no more than 20 MiB')
  }
}

export function hasMatchingImageSignature(
  bytes: Uint8Array,
  mimeType: string
): boolean {
  if (mimeType === 'image/png') {
    return [137, 80, 78, 71, 13, 10, 26, 10].every(
      (byte, index) => bytes[index] === byte
    )
  }
  if (mimeType === 'image/jpeg') {
    return bytes[0] === 255 && bytes[1] === 216 && bytes[2] === 255
  }
  const signature = String.fromCharCode(...bytes.slice(0, 12))
  if (mimeType === 'image/gif') {
    return signature.startsWith('GIF87a') || signature.startsWith('GIF89a')
  }
  return (
    mimeType === 'image/webp' &&
    signature.startsWith('RIFF') &&
    signature.slice(8, 12) === 'WEBP'
  )
}

export function readChatAttachment(file: File): Promise<ChatAttachment> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.addEventListener(
      'error',
      () =>
        reject(
          new Error('Failed to read file. Please try attaching it again.')
        ),
      { once: true }
    )
    reader.addEventListener(
      'abort',
      () =>
        reject(
          new Error('Failed to read file. Please try attaching it again.')
        ),
      { once: true }
    )
    reader.addEventListener(
      'load',
      () => {
        if (typeof reader.result !== 'string') {
          reject(
            new Error('Failed to read file. Please try attaching it again.')
          )
          return
        }
        const base64 = reader.result.split(',')[1] ?? ''
        const bytes = Uint8Array.from(atob(base64.slice(0, 32)), (char) =>
          char.charCodeAt(0)
        )
        if (
          !(file.type === 'application/pdf'
            ? String.fromCharCode(...bytes.slice(0, 5)) === '%PDF-'
            : hasMatchingImageSignature(bytes, file.type))
        ) {
          reject(new Error('The file content does not match its type'))
          return
        }
        resolve({
          id: nanoid(),
          name: file.name,
          mimeType: file.type,
          size: file.size,
          dataUrl: reader.result,
        })
      },
      { once: true }
    )
    reader.readAsDataURL(file)
  })
}

export function validateConversationAttachments(messages: Message[]): void {
  if (messages.some((message) => message.missingAttachments)) {
    throw new Error(
      'Attachments from a previous session are unavailable. Remove those messages or start a new conversation.'
    )
  }
  const attachments = messages.flatMap((message) => message.attachments ?? [])
  if (
    attachments.length > MAX_ATTACHMENT_COUNT ||
    attachments.reduce((size, image) => size + image.size, 0) >
      MAX_TOTAL_ATTACHMENT_BYTES
  ) {
    throw new Error(
      'This conversation exceeds 4 attachments or 20 MiB. Remove earlier attachment messages or start a new conversation.'
    )
  }
}
