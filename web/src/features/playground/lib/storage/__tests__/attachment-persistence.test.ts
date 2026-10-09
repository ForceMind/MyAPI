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
import { afterEach, describe, expect, test } from 'vitest'

import { STORAGE_KEYS } from '../../../constants'
import { validateConversationAttachments } from '../../input/chat-attachments'
import { appendUserMessagePair } from '../../message/conversation-message-utils'
import { loadConfig, loadMessages, saveConfig, saveMessages } from '../storage'

afterEach(() => localStorage.clear())

describe('attachment persistence boundary', () => {
  test('saved image turns contain a missing marker but never image bytes or attachment names', () => {
    const messages = appendUserMessagePair([], 'describe', [
      {
        id: 'photo',
        name: 'private-photo.png',
        mimeType: 'image/png',
        size: 8,
        dataUrl: 'data:image/png;base64,iVBORw0KGgo=',
      },
    ])
    saveMessages(messages, 101)
    const stored = localStorage.getItem(`${STORAGE_KEYS.MESSAGES}:user:101`)
    expect(stored).not.toContain('data:image')
    expect(stored).not.toContain('private-photo')
    expect(stored).toContain('missingAttachments')
    const loaded = loadMessages(101) ?? []
    expect(loaded[0].missingAttachments).toBe(true)
    expect(() => validateConversationAttachments(loaded)).toThrow('unavailable')
  })

  test('configuration persists only the selected numeric ID and discards unexpected key secrets', () => {
    const config = {
      keyId: 8,
      model: 'vision',
      key: 'never-store-this',
      secret: 'never-store-that',
    }
    saveConfig(config, 101)
    expect(loadConfig(101)).toEqual({ keyId: 8, model: 'vision' })
    expect(
      localStorage.getItem(`${STORAGE_KEYS.CONFIG}:user:101`)
    ).not.toContain('never-store')
  })
})
