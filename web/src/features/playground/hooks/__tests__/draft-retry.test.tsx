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
import { act, renderHook } from '@testing-library/react'
import { useState } from 'react'
import { describe, expect, test, vi } from 'vitest'

import { DEFAULT_CONFIG, DEFAULT_PARAMETER_ENABLED } from '../../constants'
import { validateConversationAttachments } from '../../lib/input/chat-attachments'
import { buildChatCompletionPayload } from '../../lib/streaming/payload-builder'
import type { ChatAttachment, Message } from '../../types'
import { usePlaygroundConversation } from '../use-playground-conversation'

const attachment: ChatAttachment = {
  id: 'attachment-1',
  name: 'notes.pdf',
  mimeType: 'application/pdf',
  dataUrl: 'data:application/pdf;base64,JVBERi0xLjQK',
  size: 9,
}

describe('draft retry history', () => {
  test('a deliberate retry of the same failed attachment draft replaces its failed pair', async () => {
    const sendChat = vi.fn().mockResolvedValue(false)
    const { result } = renderHook(() => {
      const [messages, updateMessages] = useState<Message[]>([])
      return {
        messages,
        ...usePlaygroundConversation({ messages, updateMessages, sendChat }),
      }
    })
    await act(async () => {
      await result.current.handleSendMessage('read it', [attachment])
    })
    expect(result.current.messages).toHaveLength(2)
    sendChat.mockResolvedValue(true)
    await act(async () => {
      await result.current.handleSendMessage('read it', [attachment])
    })
    expect(result.current.messages).toHaveLength(2)
    expect(sendChat.mock.calls[1][0][0]).toMatchObject({
      from: 'user',
      attachments: [attachment],
    })
  })

  test('a rejected send does not append placeholder messages or consume the draft', async () => {
    const sendChat = vi.fn().mockReturnValue(null)
    const { result } = renderHook(() => {
      const [messages, updateMessages] = useState<Message[]>([])
      return {
        messages,
        ...usePlaygroundConversation({ messages, updateMessages, sendChat }),
      }
    })
    await act(async () => {
      expect(await result.current.handleSendMessage('', [attachment])).toBe(
        false
      )
    })
    expect(result.current.messages).toEqual([])
  })
})

describe('editing a failed composer draft', () => {
  test('editing text after a three-image failure replaces the tail instead of counting six images', async () => {
    const images = ['first', 'second', 'third'].map((id) => ({
      ...attachment,
      id,
      name: `${id}.png`,
      mimeType: 'image/png',
      dataUrl: 'data:image/png;base64,iVBORw0KGgo=',
      size: 8,
    }))
    const sendChat = vi.fn((messages: Message[]) => {
      validateConversationAttachments(messages)
      return Promise.resolve(false)
    })
    const { result } = renderHook(() => {
      const [messages, updateMessages] = useState<Message[]>([])
      return {
        messages,
        ...usePlaygroundConversation({ messages, updateMessages, sendChat }),
      }
    })
    await act(async () => {
      await result.current.handleSendMessage('first attempt', images)
    })
    await act(async () => {
      await result.current.handleSendMessage('corrected prompt', images)
    })
    expect(result.current.messages).toHaveLength(2)
    expect(result.current.messages[0]).toMatchObject({
      versions: [{ content: 'corrected prompt' }],
      attachments: images,
    })
  })

  test('removing a rejected PDF from the draft sends no hidden PDF and retains earlier successful history', async () => {
    const earlier: Message[] = [
      {
        key: 'earlier-user',
        from: 'user',
        versions: [{ id: 'earlier-u', content: 'earlier question' }],
      },
      {
        key: 'earlier-assistant',
        from: 'assistant',
        versions: [{ id: 'earlier-a', content: 'earlier answer' }],
        status: 'complete',
      },
    ]
    const sendChat = vi.fn().mockResolvedValue(false)
    const { result } = renderHook(() => {
      const [messages, updateMessages] = useState<Message[]>(earlier)
      return {
        messages,
        ...usePlaygroundConversation({ messages, updateMessages, sendChat }),
      }
    })
    await act(async () => {
      await result.current.handleSendMessage('read the PDF', [attachment])
    })
    sendChat.mockResolvedValue(true)
    await act(async () => {
      await result.current.handleSendMessage('use only this text', [])
    })
    const payload = buildChatCompletionPayload(
      sendChat.mock.calls[1][0],
      DEFAULT_CONFIG,
      DEFAULT_PARAMETER_ENABLED
    )
    expect(result.current.messages.slice(0, 2)).toEqual(earlier)
    expect(result.current.messages).toHaveLength(4)
    expect(payload.messages).toEqual([
      { role: 'user', content: 'earlier question' },
      { role: 'assistant', content: 'earlier answer' },
      { role: 'user', content: 'use only this text' },
    ])
  })

  test('replacing a failed attachment sends only the replacement attachment', async () => {
    const sendChat = vi.fn().mockResolvedValue(false)
    const { result } = renderHook(() => {
      const [messages, updateMessages] = useState<Message[]>([])
      return {
        messages,
        ...usePlaygroundConversation({ messages, updateMessages, sendChat }),
      }
    })
    await act(async () => {
      await result.current.handleSendMessage('', [attachment])
    })
    const replacement = {
      ...attachment,
      id: 'replacement',
      name: 'replacement.pdf',
    }
    await act(async () => {
      await result.current.handleSendMessage('', [replacement])
    })
    expect(result.current.messages).toHaveLength(2)
    expect(result.current.messages[0].attachments).toEqual([replacement])
  })

  test('opening and canceling a history edit releases composer ownership of the failed pair', async () => {
    const sendChat = vi.fn().mockResolvedValue(false)
    const { result } = renderHook(() => {
      const [messages, updateMessages] = useState<Message[]>([])
      return {
        messages,
        ...usePlaygroundConversation({ messages, updateMessages, sendChat }),
      }
    })
    await act(async () => {
      await result.current.handleSendMessage('read it', [attachment])
    })
    const failedUser = result.current.messages[0]
    act(() => {
      result.current.handleEditMessage(failedUser)
    })
    act(() => {
      result.current.handleEditOpenChange(false)
    })
    await act(async () => {
      await result.current.handleSendMessage('read it', [attachment])
    })
    expect(result.current.messages).toHaveLength(4)
    expect(result.current.messages[0]).toBe(failedUser)
  })
})

describe('successful error-bubble retry draft cleanup', () => {
  test('a successful retry reports the original failed draft for conditional composer cleanup', async () => {
    const sendChat = vi.fn().mockResolvedValue(false)
    const { result } = renderHook(() => {
      const [messages, updateMessages] = useState<Message[]>([])
      return {
        messages,
        ...usePlaygroundConversation({ messages, updateMessages, sendChat }),
      }
    })
    await act(async () => {
      await result.current.handleSendMessage('read it', [attachment])
    })
    const failedUser = result.current.messages[0]
    sendChat.mockResolvedValue(true)
    await act(async () => {
      await result.current.handleRegenerateMessage(result.current.messages[1])
    })
    expect(result.current.completedRetryDraft).toEqual({
      key: failedUser.key,
      text: 'read it',
      attachmentIds: [attachment.id],
    })
  })
})

test('a failed error-bubble retry keeps its composer ownership so a removed PDF stays removed on the next send', async () => {
  const sendChat = vi.fn().mockResolvedValue(false)
  const { result } = renderHook(() => {
    const [messages, updateMessages] = useState<Message[]>([])
    return {
      messages,
      ...usePlaygroundConversation({ messages, updateMessages, sendChat }),
    }
  })
  await act(async () => {
    await result.current.handleSendMessage('read it', [attachment])
  })
  await act(async () => {
    await result.current.handleRegenerateMessage(result.current.messages[1])
  })
  await act(async () => {
    await result.current.handleSendMessage('text only', [])
  })
  expect(result.current.messages).toHaveLength(2)
  expect(result.current.messages[0].attachments).toBeUndefined()
})
