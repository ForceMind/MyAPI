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
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { isPlaygroundKeyAvailable, sendChatCompletion } from '../api'
import { ERROR_MESSAGES } from '../constants'
import {
  applyStreamingChunk,
  buildChatCompletionPayload,
  updateAssistantMessageWithError,
  updateLastAssistantMessage,
  parseRequestErrorDetails,
  applyChatCompletionResponse,
  completeAssistantMessage,
  hasChatCompletionChoice,
  isAssistantMessageFinal,
  isAssistantMessagePending,
} from '../lib'
import { validateConversationAttachments } from '../lib/input/chat-attachments'
import type {
  Message,
  PlaygroundConfig,
  ParameterEnabled,
  PlaygroundKey,
} from '../types'
import { useStreamRequest } from './use-stream-request'

interface UseChatHandlerOptions {
  config: PlaygroundConfig
  selectedKey?: PlaygroundKey
  canSend: boolean
  onKeyRejected?: () => void
  parameterEnabled: ParameterEnabled
  onMessageUpdate: (updater: (prev: Message[]) => Message[]) => void
}

const KNOWN_ERROR_MESSAGES = new Set<string>(Object.values(ERROR_MESSAGES))
const STREAM_UPDATE_FLUSH_MS = 50

type PendingStreamChunks = {
  generation: number
  content: string
  reasoning: string
}

function mergePendingStreamChunk(
  currentChunk: string,
  nextChunk: string
): string {
  if (!currentChunk || !nextChunk.startsWith(currentChunk)) {
    return currentChunk + nextChunk
  }

  return nextChunk
}

/**
 * Hook for handling chat message sending and receiving
 */
export function useChatHandler({
  config,
  selectedKey,
  canSend,
  onKeyRejected,
  parameterEnabled,
  onMessageUpdate,
}: UseChatHandlerOptions) {
  const { t } = useTranslation()
  const { sendStreamRequest, stopStream, isStreaming } = useStreamRequest()
  const [isRequesting, setIsRequesting] = useState(false)
  const abortControllerRef = useRef<AbortController | null>(null)
  const requestGenerationRef = useRef(0)
  const activeResultRef = useRef<((success: boolean) => void) | null>(null)
  const settleRequest = useCallback((success: boolean) => {
    const resolve = activeResultRef.current
    activeResultRef.current = null
    resolve?.(success)
  }, [])
  const pendingStreamChunksRef = useRef<PendingStreamChunks>({
    generation: 0,
    content: '',
    reasoning: '',
  })
  const streamFlushTimerRef = useRef<number | null>(null)

  const discardPendingStreamUpdates = useCallback((generation: number) => {
    if (streamFlushTimerRef.current !== null) {
      window.clearTimeout(streamFlushTimerRef.current)
      streamFlushTimerRef.current = null
    }
    pendingStreamChunksRef.current = {
      generation,
      content: '',
      reasoning: '',
    }
  }, [])

  const flushStreamUpdates = useCallback(
    (generation: number) => {
      if (generation !== requestGenerationRef.current) return
      if (streamFlushTimerRef.current !== null) {
        window.clearTimeout(streamFlushTimerRef.current)
        streamFlushTimerRef.current = null
      }

      const pendingChunks = pendingStreamChunksRef.current
      if (pendingChunks.generation !== generation) return
      if (!pendingChunks.reasoning && !pendingChunks.content) {
        return
      }

      pendingStreamChunksRef.current = {
        generation,
        content: '',
        reasoning: '',
      }
      onMessageUpdate((prev) => {
        // This batch was current when queued. Keep it when Stop advances the
        // generation before React evaluates this updater.
        return updateLastAssistantMessage(prev, (message) => {
          let updatedMessage = message

          if (pendingChunks.reasoning) {
            updatedMessage = applyStreamingChunk(
              updatedMessage,
              'reasoning',
              pendingChunks.reasoning
            )
          }

          if (pendingChunks.content) {
            updatedMessage = applyStreamingChunk(
              updatedMessage,
              'content',
              pendingChunks.content
            )
          }

          return updatedMessage
        })
      })
    },
    [onMessageUpdate]
  )

  const scheduleStreamFlush = useCallback(
    (generation: number) => {
      if (generation !== requestGenerationRef.current) return
      if (streamFlushTimerRef.current !== null) {
        return
      }

      streamFlushTimerRef.current = window.setTimeout(() => {
        flushStreamUpdates(generation)
      }, STREAM_UPDATE_FLUSH_MS)
    },
    [flushStreamUpdates]
  )

  useEffect(
    () => () => {
      requestGenerationRef.current += 1
      if (streamFlushTimerRef.current !== null) {
        window.clearTimeout(streamFlushTimerRef.current)
      }
      abortControllerRef.current?.abort()
      abortControllerRef.current = null
      activeResultRef.current?.(false)
      activeResultRef.current = null
    },
    []
  )

  const getDisplayError = useCallback(
    (error: string, errorCode?: string) => {
      if (errorCode === 'playground_invalid_media') {
        return t(
          'An attachment was rejected. Check its type, size and contents.'
        )
      }
      if (errorCode === 'playground_payload_too_large') {
        return t(
          'The conversation is too large. Remove earlier attachments or start a new conversation.'
        )
      }
      if (errorCode === 'token_budget_unsupported_request') {
        return t(
          'Strict token budgets do not support attachments. Choose another key or send text only.'
        )
      }
      if (errorCode === 'playground_image_provider_unsupported') {
        return t(
          'This provider route does not support images in this version. Choose an OpenAI-compatible or Codex route.'
        )
      }
      if (errorCode === 'playground_file_provider_unsupported') {
        return t(
          'This provider does not support inline PDF files. Choose a compatible model and provider.'
        )
      }
      if (KNOWN_ERROR_MESSAGES.has(error)) {
        return t(error)
      }

      const connectionClosedSuffix = `: ${ERROR_MESSAGES.CONNECTION_CLOSED}`
      if (error.endsWith(connectionClosedSuffix)) {
        return `${error.slice(0, -ERROR_MESSAGES.CONNECTION_CLOSED.length)}${t(
          ERROR_MESSAGES.CONNECTION_CLOSED
        )}`
      }

      return error
    },
    [t]
  )

  // Handle stream update
  const handleStreamUpdate = useCallback(
    (generation: number, type: 'reasoning' | 'content', chunk: string) => {
      if (generation !== requestGenerationRef.current) return
      if (pendingStreamChunksRef.current.generation !== generation) return
      pendingStreamChunksRef.current[type] = mergePendingStreamChunk(
        pendingStreamChunksRef.current[type],
        chunk
      )
      scheduleStreamFlush(generation)
    },
    [scheduleStreamFlush]
  )

  // Handle stream complete
  const handleStreamComplete = useCallback(
    (generation: number) => {
      if (generation !== requestGenerationRef.current) return
      flushStreamUpdates(generation)
      settleRequest(true)
      setIsRequesting(false)
      onMessageUpdate((prev) => {
        if (generation !== requestGenerationRef.current) return prev
        return updateLastAssistantMessage(prev, (message) =>
          isAssistantMessageFinal(message)
            ? message
            : completeAssistantMessage(message)
        )
      })
    },
    [flushStreamUpdates, onMessageUpdate, settleRequest]
  )

  // Handle stream error
  const handleStreamError = useCallback(
    (generation: number, error: string, errorCode?: string) => {
      if (generation !== requestGenerationRef.current) return
      flushStreamUpdates(generation)
      settleRequest(false)
      setIsRequesting(false)
      if (
        errorCode === 'playground_key_invalid' ||
        errorCode === 'playground_key_required'
      ) {
        onKeyRejected?.()
      }
      const displayError = getDisplayError(error, errorCode)
      toast.error(displayError)
      const errorTitle = t(ERROR_MESSAGES.API_REQUEST_ERROR)
      onMessageUpdate((prev) => {
        if (generation !== requestGenerationRef.current) return prev
        return updateAssistantMessageWithError(
          prev,
          displayError,
          errorCode,
          errorTitle
        )
      })
    },
    [
      flushStreamUpdates,
      getDisplayError,
      onMessageUpdate,
      onKeyRejected,
      settleRequest,
      t,
    ]
  )

  // Send streaming chat request
  const sendStreamingChat = useCallback(
    (messages: Message[], keyId: number) => {
      const generation = requestGenerationRef.current + 1
      requestGenerationRef.current = generation
      abortControllerRef.current?.abort()
      abortControllerRef.current = null
      discardPendingStreamUpdates(generation)
      setIsRequesting(true)
      const payload = buildChatCompletionPayload(
        messages,
        config,
        parameterEnabled
      )
      void sendStreamRequest(
        payload,
        (type, chunk) => handleStreamUpdate(generation, type, chunk),
        () => handleStreamComplete(generation),
        (error, errorCode) => handleStreamError(generation, error, errorCode),
        keyId
      )
    },
    [
      config,
      parameterEnabled,
      sendStreamRequest,
      discardPendingStreamUpdates,
      handleStreamUpdate,
      handleStreamComplete,
      handleStreamError,
    ]
  )

  // Send non-streaming chat request
  const sendNonStreamingChat = useCallback(
    async (messages: Message[], keyId: number) => {
      const payload = buildChatCompletionPayload(
        messages,
        config,
        parameterEnabled
      )
      const generation = requestGenerationRef.current + 1
      const abortController = new AbortController()

      requestGenerationRef.current = generation
      stopStream()
      discardPendingStreamUpdates(generation)
      abortControllerRef.current?.abort()
      abortControllerRef.current = abortController

      try {
        setIsRequesting(true)
        const response = await sendChatCompletion(
          payload,
          keyId,
          abortController.signal
        )
        if (
          abortController.signal.aborted ||
          requestGenerationRef.current !== generation
        ) {
          return
        }

        if (!hasChatCompletionChoice(response)) {
          handleStreamError(generation, ERROR_MESSAGES.API_REQUEST_ERROR)
          return
        }

        onMessageUpdate((prev) => {
          if (requestGenerationRef.current !== generation) return prev
          return updateLastAssistantMessage(prev, (message) => {
            const updatedMessage = applyChatCompletionResponse(
              message,
              response
            )

            return updatedMessage ?? message
          })
        })
        settleRequest(true)
      } catch (error: unknown) {
        if (
          abortController.signal.aborted ||
          requestGenerationRef.current !== generation
        ) {
          return
        }

        const { errorCode, errorMessage } = parseRequestErrorDetails(error)
        handleStreamError(generation, errorMessage, errorCode)
      } finally {
        if (requestGenerationRef.current === generation) {
          abortControllerRef.current = null
          setIsRequesting(false)
        }
      }
    },
    [
      config,
      parameterEnabled,
      stopStream,
      discardPendingStreamUpdates,
      onMessageUpdate,
      handleStreamError,
      settleRequest,
    ]
  )

  // A synchronous latch covers rapid clicks before React renders the busy state.
  const sendChat = useCallback(
    (messages: Message[]): Promise<boolean> | null => {
      if (activeResultRef.current) return null
      if (
        !canSend ||
        !selectedKey ||
        selectedKey.id !== config.keyId ||
        !isPlaygroundKeyAvailable(selectedKey)
      ) {
        toast.error(t('Select an available API key before sending'))
        return null
      }
      try {
        validateConversationAttachments(messages)
      } catch (error) {
        toast.error(
          t(
            error instanceof Error
              ? error.message
              : ERROR_MESSAGES.API_REQUEST_ERROR
          )
        )
        return null
      }
      const result = new Promise<boolean>((resolve) => {
        activeResultRef.current = resolve
      })
      if (config.stream) sendStreamingChat(messages, selectedKey.id)
      else void sendNonStreamingChat(messages, selectedKey.id)
      return result
    },
    [
      canSend,
      selectedKey,
      config.keyId,
      config.stream,
      sendStreamingChat,
      sendNonStreamingChat,
      t,
    ]
  )

  // Stop generation
  const stopGeneration = useCallback(() => {
    const stoppedGeneration = requestGenerationRef.current
    flushStreamUpdates(stoppedGeneration)
    const idleGeneration = stoppedGeneration + 1
    requestGenerationRef.current = idleGeneration
    discardPendingStreamUpdates(idleGeneration)
    stopStream()
    settleRequest(false)
    abortControllerRef.current?.abort()
    abortControllerRef.current = null
    setIsRequesting(false)
    onMessageUpdate((prev) => {
      if (requestGenerationRef.current !== idleGeneration) return prev
      return updateLastAssistantMessage(prev, (message) =>
        isAssistantMessagePending(message)
          ? completeAssistantMessage(message)
          : message
      )
    })
  }, [
    stopStream,
    flushStreamUpdates,
    discardPendingStreamUpdates,
    onMessageUpdate,
    settleRequest,
  ])

  return {
    sendChat,
    stopGeneration,
    isGenerating: isStreaming || isRequesting,
  }
}
