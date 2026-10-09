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
import { useCallback, useRef, useState } from 'react'

import {
  appendUserMessagePair,
  applyMessageEdit,
  createRegeneratedMessages,
  removeMessageByKey,
} from '../lib'
import type { ChatAttachment, Message, PlaygroundDraftIdentity } from '../types'

type UsePlaygroundConversationOptions = {
  messages: Message[]
  updateMessages: (
    updater: Message[] | ((prev: Message[]) => Message[])
  ) => void
  sendChat: (messages: Message[]) => Promise<boolean> | null
}

export function usePlaygroundConversation({
  messages,
  updateMessages,
  sendChat,
}: UsePlaygroundConversationOptions) {
  const [editingMessageKey, setEditingMessageKey] = useState<string | null>(
    null
  )

  const [completedRetryDraft, setCompletedRetryDraft] =
    useState<PlaygroundDraftIdentity | null>(null)

  const lastFailedDraftRef = useRef<{
    userKey: string
    assistantKey: string
  } | null>(null)

  const draftOwnershipEpochRef = useRef(0)
  const releaseDraftOwnership = useCallback(() => {
    draftOwnershipEpochRef.current += 1
    lastFailedDraftRef.current = null
  }, [])

  const handleSendMessage = useCallback(
    async (
      text: string,
      attachments: ChatAttachment[] = []
    ): Promise<boolean> => {
      if (!text.trim() && attachments.length === 0) return false
      const failed = lastFailedDraftRef.current
      let history = messages
      // The composer still owns this failed tail even after its text or files change.
      if (
        failed &&
        messages.at(-2)?.key === failed.userKey &&
        messages.at(-1)?.key === failed.assistantKey
      ) {
        history = messages.slice(0, -2)
      }
      const nextMessages = appendUserMessagePair(history, text, attachments)
      const request = sendChat(nextMessages)
      if (!request) return false
      const ownershipEpoch = draftOwnershipEpochRef.current
      lastFailedDraftRef.current = null
      updateMessages(nextMessages)
      const success = await request
      if (draftOwnershipEpochRef.current !== ownershipEpoch) return success
      const userKey = nextMessages.at(-2)?.key
      const assistantKey = nextMessages.at(-1)?.key
      lastFailedDraftRef.current =
        !success && userKey && assistantKey
          ? {
              userKey,
              assistantKey,
            }
          : null
      return success
    },
    [messages, updateMessages, sendChat]
  )

  const handleRegenerateMessage = useCallback(
    async (message: Message) => {
      const nextMessages = createRegeneratedMessages(messages, message.key)
      if (!nextMessages) return

      const failedDraft = lastFailedDraftRef.current
      const isFailedDraftRetry =
        failedDraft &&
        (message.key === failedDraft.userKey ||
          message.key === failedDraft.assistantKey)
      const originalDraft = isFailedDraftRetry
        ? messages.find((item) => item.key === failedDraft.userKey)
        : undefined
      releaseDraftOwnership()
      const ownershipEpoch = draftOwnershipEpochRef.current
      const request = sendChat(nextMessages)
      if (!request) {
        if (isFailedDraftRetry) lastFailedDraftRef.current = failedDraft
        return
      }
      updateMessages(nextMessages)
      const success = await request
      if (!originalDraft || draftOwnershipEpochRef.current !== ownershipEpoch) {
        return
      }
      if (!success) {
        const assistantKey = nextMessages.at(-1)?.key
        if (assistantKey) {
          lastFailedDraftRef.current = {
            userKey: originalDraft.key,
            assistantKey,
          }
        }
        return
      }
      if (success) {
        setCompletedRetryDraft({
          key: originalDraft.key,
          text: originalDraft.versions[0]?.content ?? '',
          attachmentIds:
            originalDraft.attachments?.map((attachment) => attachment.id) ?? [],
        })
      }
    },
    [messages, updateMessages, sendChat, releaseDraftOwnership]
  )

  const handleEditMessage = useCallback(
    (message: Message) => {
      releaseDraftOwnership()
      setEditingMessageKey(message.key)
    },
    [releaseDraftOwnership]
  )

  const handleEditOpenChange = useCallback(
    (open: boolean) => {
      if (!open) {
        releaseDraftOwnership()
        setEditingMessageKey(null)
      }
    },
    [releaseDraftOwnership]
  )

  const applyEdit = useCallback(
    (newContent: string, shouldSubmit: boolean) => {
      if (!editingMessageKey) return
      releaseDraftOwnership()

      const editResult = applyMessageEdit(
        messages,
        editingMessageKey,
        newContent,
        shouldSubmit
      )
      if (!editResult) return

      if (editResult.shouldSend && !sendChat(editResult.messages)) return
      setEditingMessageKey(null)
      updateMessages(editResult.messages)
    },
    [
      editingMessageKey,
      messages,
      updateMessages,
      sendChat,
      releaseDraftOwnership,
    ]
  )

  const handleDeleteMessage = useCallback(
    (message: Message) => {
      releaseDraftOwnership()
      updateMessages((previousMessages) =>
        removeMessageByKey(previousMessages, message.key)
      )
    },
    [updateMessages, releaseDraftOwnership]
  )

  return {
    completedRetryDraft,
    editingMessageKey,
    handleSendMessage,
    handleRegenerateMessage,
    handleEditMessage,
    handleEditOpenChange,
    applyEdit,
    handleDeleteMessage,
  }
}
