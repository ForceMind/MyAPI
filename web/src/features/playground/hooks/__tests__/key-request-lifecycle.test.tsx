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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  act,
  cleanup,
  render,
  renderHook,
  screen,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState, type ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { PlaygroundInput } from '../../components/input/playground-input'
import { DEFAULT_CONFIG, DEFAULT_PARAMETER_ENABLED } from '../../constants'
import type {
  ChatCompletionResponse,
  Message,
  PlaygroundConfig,
  PlaygroundKey,
} from '../../types'
import { useChatHandler } from '../use-chat-handler'
import { usePlaygroundConversation } from '../use-playground-conversation'
import { usePlaygroundOptions } from '../use-playground-options'

const streamBoundary = vi.hoisted(() => {
  class ControlledSource {
    static instances: ControlledSource[] = []
    readonly close = vi.fn()
    readonly stream = vi.fn()
    private listeners = new Map<
      string,
      Array<(event: Event & { data?: string; readyState?: number }) => void>
    >()

    constructor(
      readonly url: string,
      readonly options: {
        headers: Record<string, string>
        payload: string
        method: string
        start?: boolean
        autoReconnect?: boolean
      }
    ) {
      ControlledSource.instances.push(this)
      // sse.js starts during construction unless explicitly disabled.
      if (options.start !== false) this.stream()
    }

    addEventListener(
      type: string,
      listener: (event: Event & { data?: string; readyState?: number }) => void
    ) {
      const listeners = this.listeners.get(type) ?? []
      listeners.push(listener)
      this.listeners.set(type, listeners)
    }

    emit(type: string, data: string) {
      // Deliver even after close so regressions cannot hide behind the transport mock.
      for (const listener of this.listeners.get(type) ?? []) {
        listener(Object.assign(new Event(type), { data }))
      }
    }
  }

  return { ControlledSource }
})

vi.mock('sse.js', () => ({ SSE: streamBoundary.ControlledSource }))

const availableKey: PlaygroundKey = {
  id: 17,
  name: 'Personal chat',
  status: 1,
  group: 'personal',
  remain_quota: 100,
  unlimited_quota: false,
  expired_time: -1,
}

const initialMessages: Message[] = [
  {
    key: 'user-1',
    from: 'user',
    versions: [{ id: 'user-version-1', content: 'Hello' }],
    status: 'complete',
  },
  {
    key: 'assistant-1',
    from: 'assistant',
    versions: [{ id: 'assistant-version-1', content: '' }],
    status: 'loading',
  },
]

function completion(content = 'Hello back'): ChatCompletionResponse {
  return {
    id: 'completion-1',
    object: 'chat.completion',
    created: 1,
    model: 'personal-model',
    choices: [
      {
        index: 0,
        message: { role: 'assistant', content },
        finish_reason: 'stop',
      },
    ],
  }
}

function deferred<T>() {
  let resolve: (value: T) => void = () => undefined
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

interface ChatHarnessProps {
  config: PlaygroundConfig
  selectedKey?: PlaygroundKey
  canSend: boolean
  onKeyRejected?: () => void
}

function renderChat(overrides: Partial<ChatHarnessProps> = {}) {
  return renderHook(
    (props: ChatHarnessProps) => {
      const [messages, setMessages] = useState(initialMessages)
      const handler = useChatHandler({
        ...props,
        parameterEnabled: DEFAULT_PARAMETER_ENABLED,
        onMessageUpdate: setMessages,
      })
      return { ...handler, messages }
    },
    {
      initialProps: {
        config: {
          ...DEFAULT_CONFIG,
          model: 'personal-model',
          keyId: 17,
          stream: false,
        },
        selectedKey: availableKey,
        canSend: true,
        ...overrides,
      },
    }
  )
}

function BudgetRejectionChat(props: {
  stream: boolean
  onKeyRejected: () => void
}) {
  const [messages, setMessages] = useState<Message[]>([])
  const [keyId, setKeyId] = useState<number | null>(availableKey.id)
  const config = {
    ...DEFAULT_CONFIG,
    model: 'gpt-6.1-sol',
    keyId,
    stream: props.stream,
  }
  const handler = useChatHandler({
    config,
    selectedKey: keyId === availableKey.id ? availableKey : undefined,
    canSend: keyId !== null,
    onKeyRejected: () => {
      props.onKeyRejected()
      setKeyId(null)
    },
    parameterEnabled: DEFAULT_PARAMETER_ENABLED,
    onMessageUpdate: setMessages,
  })
  const conversation = usePlaygroundConversation({
    messages,
    updateMessages: setMessages,
    sendChat: handler.sendChat,
  })

  return (
    <>
      <PlaygroundInput
        config={config}
        canSend={keyId !== null}
        keys={[availableKey]}
        onKeyChange={setKeyId}
        onRefreshKeys={() => undefined}
        models={[{ label: config.model, value: config.model }]}
        modelValue={config.model}
        onModelChange={() => undefined}
        onConfigChange={() => undefined}
        onParameterEnabledChange={() => undefined}
        parameterEnabled={DEFAULT_PARAMETER_ENABLED}
        onSubmit={conversation.handleSendMessage}
        isGenerating={handler.isGenerating}
        disabled={handler.isGenerating}
        onStop={handler.stopGeneration}
      />
      <output aria-label='Assistant response'>
        {messages.at(-1)?.versions[0]?.content}
      </output>
    </>
  )
}

const queryClients: QueryClient[] = []

function renderOptions(keyId: number | null, currentModel = 'personal-model') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  queryClients.push(client)
  const updateConfig = vi.fn()
  const hook = renderHook(
    (props: { keyId: number | null; currentModel: string }) =>
      usePlaygroundOptions({ ...props, updateConfig }),
    {
      initialProps: { keyId, currentModel },
      wrapper: (props: { children: ReactNode }) => (
        <QueryClientProvider client={client}>
          {props.children}
        </QueryClientProvider>
      ),
    }
  )
  return { ...hook, updateConfig }
}

beforeEach(() => {
  streamBoundary.ControlledSource.instances = []
  localStorage.clear()
  useAuthStore.setState((state) => ({
    auth: {
      ...state.auth,
      accessToken: 'session-access-token',
      accessExpiresAt: Math.floor(Date.now() / 1000) + 3600,
      bootstrapState: 'complete',
    },
  }))
})

afterEach(() => {
  cleanup()
  for (const client of queryClients.splice(0)) client.clear()
  useAuthStore.getState().auth.reset()
  localStorage.clear()
  vi.restoreAllMocks()
})

describe('chat identity and dispatch', () => {
  test.each([false, true])(
    'uses refreshed strict metadata to build a successful text request with stream=%s',
    async (stream) => {
      const post = vi
        .spyOn(api, 'post')
        .mockResolvedValue({ data: completion('Strict text response') })
      const config = {
        ...DEFAULT_CONFIG,
        model: 'gpt-6.1-sol',
        keyId: 17,
        stream,
      }
      const { result, rerender } = renderChat({ config })
      rerender({
        config,
        selectedKey: { ...availableKey, strict_token_budget: true },
        canSend: true,
      })
      let pending: Promise<boolean> | null = null

      await act(async () => {
        pending = result.current.sendChat(initialMessages)
      })

      const expected = {
        model: 'gpt-6.1-sol',
        messages: [{ role: 'user', content: 'Hello' }],
        stream,
        max_completion_tokens: 4096,
        service_tier: 'default',
        ...(stream ? { stream_options: { include_usage: true } } : {}),
      }
      if (stream) {
        expect(post).not.toHaveBeenCalled()
        expect(streamBoundary.ControlledSource.instances).toHaveLength(1)
        const source = streamBoundary.ControlledSource.instances[0]
        expect(JSON.parse(source.options.payload)).toEqual(expected)
        expect(source.options.headers['X-MyAPI-Key-ID']).toBe('17')
        await act(async () => {
          source.emit(
            'message',
            JSON.stringify({
              choices: [{ delta: { content: 'Strict text response' } }],
            })
          )
          source.emit('message', '[DONE]')
          await expect(pending).resolves.toBe(true)
        })
      } else {
        expect(post).toHaveBeenCalledOnce()
        expect(post).toHaveBeenCalledWith(
          '/pg/chat/completions',
          expected,
          expect.objectContaining({
            headers: expect.objectContaining({ 'X-MyAPI-Key-ID': '17' }),
          })
        )
        await expect(pending).resolves.toBe(true)
      }
      expect(result.current.messages.at(-1)).toMatchObject({
        status: 'complete',
        versions: [
          { id: 'assistant-version-1', content: 'Strict text response' },
        ],
      })
    }
  )

  test('returns true and updates the assistant after a selected-key non-streaming response', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({ data: completion() })
    const { result } = renderChat()

    await act(async () => {
      await expect(result.current.sendChat(initialMessages)).resolves.toBe(true)
    })

    expect(post).toHaveBeenCalledWith(
      '/pg/chat/completions',
      expect.objectContaining({
        model: 'personal-model',
        stream: false,
        messages: [{ role: 'user', content: 'Hello' }],
      }),
      expect.objectContaining({
        headers: expect.objectContaining({ 'X-MyAPI-Key-ID': '17' }),
        skipAuthRefresh: true,
        signal: expect.any(AbortSignal),
      })
    )
    expect(post.mock.calls[0]?.[1]).not.toHaveProperty('group')
    expect(result.current.messages.at(-1)).toMatchObject({
      status: 'complete',
      versions: [{ id: 'assistant-version-1', content: 'Hello back' }],
    })
    expect(result.current.isGenerating).toBe(false)
  })

  test('sends selected key identity alongside session authentication for a real stream', async () => {
    const { result } = renderChat({
      config: { ...DEFAULT_CONFIG, keyId: 17, stream: true },
    })
    let pending: Promise<boolean> | null = null

    await act(async () => {
      pending = result.current.sendChat(initialMessages)
    })

    expect(streamBoundary.ControlledSource.instances).toHaveLength(1)
    const source = streamBoundary.ControlledSource.instances[0]
    expect(source.url).toBe('/pg/chat/completions')
    expect(source.options.headers).toEqual({
      'Content-Type': 'application/json',
      Authorization: 'Bearer session-access-token',
      'X-MyAPI-Key-ID': '17',
    })
    expect(JSON.parse(source.options.payload)).not.toHaveProperty('group')
    expect(source.options).toMatchObject({ start: false, autoReconnect: false })
    expect(source.stream).toHaveBeenCalledOnce()

    await act(async () => {
      source.emit(
        'message',
        JSON.stringify({ choices: [{ delta: { content: 'Streamed answer' } }] })
      )
      source.emit('message', '[DONE]')
      await expect(pending).resolves.toBe(true)
    })

    expect(result.current.messages.at(-1)).toMatchObject({
      status: 'complete',
      versions: [{ id: 'assistant-version-1', content: 'Streamed answer' }],
    })
    expect(result.current.isGenerating).toBe(false)
  })

  describe.each([false, true])('with stream=%s', (stream) => {
    test.each([
      ['no selected key', undefined, true],
      ['a removed or different key', { ...availableKey, id: 28 }, true],
      ['a disabled key', { ...availableKey, status: 2 }, true],
      ['an expired key', { ...availableKey, expired_time: 1 }, true],
      ['an exhausted key', { ...availableKey, remain_quota: 0 }, true],
      ['models not ready', availableKey, false],
    ] as const)(
      'synchronously refuses dispatch with %s',
      async (_name, selectedKey, canSend) => {
        const post = vi.spyOn(api, 'post')
        const { result } = renderChat({
          config: { ...DEFAULT_CONFIG, keyId: 17, stream },
          selectedKey,
          canSend,
        })

        await act(async () => {
          expect(result.current.sendChat(initialMessages)).toBeNull()
        })

        expect(post).not.toHaveBeenCalled()
        expect(streamBoundary.ControlledSource.instances).toHaveLength(0)
        expect(result.current.messages).toEqual(initialMessages)
        expect(result.current.isGenerating).toBe(false)
      }
    )

    test('coalesces two sends before a React render into one network request', async () => {
      const response = deferred<{ data: ChatCompletionResponse }>()
      const post = vi.spyOn(api, 'post').mockReturnValue(response.promise)
      const { result } = renderChat({
        config: { ...DEFAULT_CONFIG, keyId: 17, stream },
      })
      let first: Promise<boolean> | null = null

      await act(async () => {
        first = result.current.sendChat(initialMessages)
        expect(result.current.sendChat(initialMessages)).toBeNull()
      })

      expect(first).toBeInstanceOf(Promise)
      if (stream) {
        expect(streamBoundary.ControlledSource.instances).toHaveLength(1)
        expect(post).not.toHaveBeenCalled()
      } else {
        expect(post).toHaveBeenCalledOnce()
      }

      await act(async () => {
        if (stream) {
          streamBoundary.ControlledSource.instances[0].emit('message', '[DONE]')
        } else response.resolve({ data: completion() })
        await expect(first).resolves.toBe(true)
      })
    })
  })
})

describe('cancellation and late-response isolation', () => {
  test.each([false, true])(
    'clicking Stop preserves the draft without replay and allows an explicit new send after partial content=%s',
    async (partial) => {
      const user = userEvent.setup()
      const onKeyRejected = vi.fn()
      const post = vi.spyOn(api, 'post')
      render(<BudgetRejectionChat stream onKeyRejected={onKeyRejected} />)
      const draft = screen.getByRole('textbox', { name: 'Message' })
      await user.type(draft, 'Keep this canceled draft')
      await user.upload(
        screen.getByLabelText('Upload attachments'),
        new File(
          [new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10])],
          'cancel.png',
          { type: 'image/png' }
        )
      )
      await screen.findByRole('img', { name: 'cancel.png' })
      await waitFor(() =>
        expect(screen.getByRole('button', { name: 'Send' })).toBeEnabled()
      )
      await user.click(screen.getByRole('button', { name: 'Send' }))
      await waitFor(() =>
        expect(streamBoundary.ControlledSource.instances).toHaveLength(1)
      )
      const source = streamBoundary.ControlledSource.instances[0]
      expect(draft).toBeDisabled()
      expect(screen.getByRole('combobox', { name: 'API key' })).toBeDisabled()
      if (partial) {
        act(() =>
          source.emit(
            'message',
            JSON.stringify({
              choices: [{ delta: { content: 'Partial answer' } }],
            })
          )
        )
      }

      await user.click(screen.getByRole('button', { name: 'Stop' }))

      expect(source.close).toHaveBeenCalledOnce()
      expect(streamBoundary.ControlledSource.instances).toHaveLength(1)
      expect(post).not.toHaveBeenCalled()
      expect(screen.getByRole('button', { name: 'Send' })).toBeEnabled()
      expect(draft).toBeEnabled()
      expect(draft).toHaveValue('Keep this canceled draft')
      expect(screen.getByRole('img', { name: 'cancel.png' })).toBeVisible()
      expect(screen.getByRole('combobox', { name: 'API key' })).toBeEnabled()
      expect(screen.getByRole('combobox', { name: 'API key' })).toHaveValue(
        '17'
      )
      if (partial) {
        expect(screen.getByLabelText('Assistant response')).toHaveTextContent(
          'Partial answer'
        )
      }
      await act(async () => {
        source.emit(
          'message',
          JSON.stringify({
            choices: [{ delta: { content: 'Late canceled answer' } }],
          })
        )
        source.emit('message', '[DONE]')
      })
      expect(draft).toHaveValue('Keep this canceled draft')
      expect(screen.getByRole('img', { name: 'cancel.png' })).toBeVisible()
      expect(screen.getByLabelText('Assistant response')).not.toHaveTextContent(
        'Late canceled answer'
      )
      expect(streamBoundary.ControlledSource.instances).toHaveLength(1)
      expect(onKeyRejected).not.toHaveBeenCalled()

      await user.click(
        screen.getByRole('button', { name: 'Remove attachment cancel.png' })
      )
      await user.clear(draft)
      await user.type(draft, 'Explicit replacement')
      await user.click(screen.getByRole('button', { name: 'Send' }))
      await waitFor(() =>
        expect(streamBoundary.ControlledSource.instances).toHaveLength(2)
      )
      const replacement = streamBoundary.ControlledSource.instances[1]
      expect(JSON.parse(replacement.options.payload).messages).toEqual([
        { role: 'user', content: 'Explicit replacement' },
      ])
      await act(async () => {
        replacement.emit(
          'message',
          JSON.stringify({ choices: [{ delta: { content: 'New answer' } }] })
        )
        replacement.emit('message', '[DONE]')
      })
      expect(screen.getByLabelText('Assistant response')).toHaveTextContent(
        'New answer'
      )
      expect(draft).toHaveValue('')
      expect(
        screen.queryByRole('img', { name: 'cancel.png' })
      ).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled()
    }
  )

  test('cancels a non-streaming request with false and ignores its late success', async () => {
    const response = deferred<{ data: ChatCompletionResponse }>()
    const post = vi.spyOn(api, 'post').mockReturnValue(response.promise)
    const { result } = renderChat()
    let pending: Promise<boolean> | null = null
    await act(async () => {
      pending = result.current.sendChat(initialMessages)
    })
    expect(post).toHaveBeenCalledOnce()

    await act(async () => {
      result.current.stopGeneration()
      await expect(pending).resolves.toBe(false)
    })
    const canceledMessages = result.current.messages
    expect(post.mock.calls[0]?.[2]?.signal?.aborted).toBe(true)
    expect(result.current.isGenerating).toBe(false)

    await act(async () => {
      response.resolve({ data: completion('Too late') })
      await response.promise
    })

    expect(result.current.messages).toEqual(canceledMessages)
    expect(result.current.messages.at(-1)?.versions[0].content).toBe('')
    expect(result.current.isGenerating).toBe(false)
  })

  test('an old response cannot overwrite a newer request or clear its busy state', async () => {
    const oldResponse = deferred<{ data: ChatCompletionResponse }>()
    const newResponse = deferred<{ data: ChatCompletionResponse }>()
    const post = vi
      .spyOn(api, 'post')
      .mockReturnValueOnce(oldResponse.promise)
      .mockReturnValueOnce(newResponse.promise)
    const { result } = renderChat()
    let oldPending: Promise<boolean> | null = null
    let newPending: Promise<boolean> | null = null
    await act(async () => {
      oldPending = result.current.sendChat(initialMessages)
    })
    expect(post).toHaveBeenCalledOnce()
    await act(async () => {
      result.current.stopGeneration()
      await expect(oldPending).resolves.toBe(false)
    })
    await act(async () => {
      newPending = result.current.sendChat(initialMessages)
    })
    expect(post).toHaveBeenCalledTimes(2)

    await act(async () => {
      oldResponse.resolve({ data: completion('Old answer') })
      await oldResponse.promise
    })

    expect(result.current.isGenerating).toBe(true)
    expect(result.current.messages.at(-1)?.versions[0].content).toBe('')

    await act(async () => {
      newResponse.resolve({ data: completion('Current answer') })
      await expect(newPending).resolves.toBe(true)
    })

    expect(result.current.messages.at(-1)?.versions[0].content).toBe(
      'Current answer'
    )
    expect(result.current.isGenerating).toBe(false)
  })

  test('stopping before stream headers resolve returns false without opening a source', async () => {
    const { result } = renderChat({
      config: { ...DEFAULT_CONFIG, keyId: 17, stream: true },
    })

    await act(async () => {
      const pending = result.current.sendChat(initialMessages)
      result.current.stopGeneration()
      await expect(pending).resolves.toBe(false)
    })

    expect(streamBoundary.ControlledSource.instances).toHaveLength(0)
    expect(result.current.isGenerating).toBe(false)
  })

  test('stopping before non-streaming headers resolve returns false without dispatching', async () => {
    const post = vi.spyOn(api, 'post')
    const { result } = renderChat()

    await act(async () => {
      const pending = result.current.sendChat(initialMessages)
      result.current.stopGeneration()
      await expect(pending).resolves.toBe(false)
    })

    expect(post).not.toHaveBeenCalled()
    expect(result.current.isGenerating).toBe(false)
  })

  test('stopping a stream preserves received content and ignores late content, errors and completion', async () => {
    const { result } = renderChat({
      config: { ...DEFAULT_CONFIG, keyId: 17, stream: true },
    })
    let pending: Promise<boolean> | null = null
    await act(async () => {
      pending = result.current.sendChat(initialMessages)
    })
    const source = streamBoundary.ControlledSource.instances[0]

    await act(async () => {
      source.emit(
        'message',
        JSON.stringify({ choices: [{ delta: { content: 'Partial answer' } }] })
      )
      result.current.stopGeneration()
      await expect(pending).resolves.toBe(false)
    })
    const canceledMessages = result.current.messages
    expect(canceledMessages.at(-1)?.versions[0].content).toBe('Partial answer')
    expect(source.close).toHaveBeenCalledOnce()

    act(() => {
      source.emit(
        'message',
        JSON.stringify({ choices: [{ delta: { content: 'Stale answer' } }] })
      )
      source.emit(
        'error',
        JSON.stringify({ error: { message: 'Late failure' } })
      )
      source.emit('message', '[DONE]')
    })

    expect(result.current.messages).toEqual(canceledMessages)
    expect(result.current.isGenerating).toBe(false)
  })

  test('unmounting settles an unfinished non-streaming request with false and aborts it', async () => {
    const response = deferred<{ data: ChatCompletionResponse }>()
    const post = vi.spyOn(api, 'post').mockReturnValue(response.promise)
    const { result, unmount } = renderChat()
    let pending: Promise<boolean> | null = null
    await act(async () => {
      pending = result.current.sendChat(initialMessages)
    })
    expect(post).toHaveBeenCalledOnce()

    unmount()

    await expect(pending).resolves.toBe(false)
    expect(post.mock.calls[0]?.[2]?.signal?.aborted).toBe(true)
    await act(async () => {
      response.resolve({ data: completion('After unmount') })
      await response.promise
    })
  })

  test('server rejection settles false and exposes an error instead of completing successfully', async () => {
    vi.spyOn(api, 'post').mockRejectedValue({
      response: {
        data: {
          error: { code: 'key_disabled', message: 'Selected key is disabled' },
        },
      },
    })
    const { result } = renderChat()

    await act(async () => {
      await expect(result.current.sendChat(initialMessages)).resolves.toBe(
        false
      )
    })

    expect(result.current.messages.at(-1)).toMatchObject({
      status: 'error',
      errorCode: 'key_disabled',
    })
    expect(result.current.messages.at(-1)?.versions[0].content).toContain(
      'Selected key is disabled'
    )
    expect(result.current.isGenerating).toBe(false)
  })

  test('a response without a completion choice settles false instead of reporting success', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({
      data: { ...completion(), choices: [] },
    })
    const { result } = renderChat()

    await act(async () => {
      await expect(result.current.sendChat(initialMessages)).resolves.toBe(
        false
      )
    })

    expect(result.current.messages.at(-1)?.status).toBe('error')
    expect(result.current.isGenerating).toBe(false)
  })

  test('a stream error settles false and cannot be replaced by a late completion', async () => {
    const { result } = renderChat({
      config: { ...DEFAULT_CONFIG, keyId: 17, stream: true },
    })
    let pending: Promise<boolean> | null = null
    await act(async () => {
      pending = result.current.sendChat(initialMessages)
    })
    const source = streamBoundary.ControlledSource.instances[0]

    await act(async () => {
      source.emit(
        'error',
        JSON.stringify({
          error: { code: 'key_disabled', message: 'Selected key is disabled' },
        })
      )
      await expect(pending).resolves.toBe(false)
    })
    const failedMessages = result.current.messages
    act(() => {
      source.emit('message', '[DONE]')
    })

    expect(result.current.messages).toEqual(failedMessages)
    expect(result.current.messages.at(-1)).toMatchObject({
      status: 'error',
      errorCode: 'key_disabled',
    })
    expect(result.current.isGenerating).toBe(false)
    expect(source.close).toHaveBeenCalledOnce()
  })

  test('unmounting an active stream settles false and closes the source', async () => {
    const { result, unmount } = renderChat({
      config: { ...DEFAULT_CONFIG, keyId: 17, stream: true },
    })
    let pending: Promise<boolean> | null = null
    await act(async () => {
      pending = result.current.sendChat(initialMessages)
    })
    const source = streamBoundary.ControlledSource.instances[0]
    const messagesBeforeUnmount = result.current.messages

    unmount()

    await expect(pending).resolves.toBe(false)
    expect(source.close).toHaveBeenCalledOnce()
    source.emit(
      'message',
      JSON.stringify({ choices: [{ delta: { content: 'After unmount' } }] })
    )
    source.emit('message', '[DONE]')
    expect(result.current.messages).toEqual(messagesBeforeUnmount)
  })

  test('an embedded error followed by DONE remains a failure without an automatic retry', async () => {
    const onKeyRejected = vi.fn()
    const { result } = renderChat({
      config: { ...DEFAULT_CONFIG, keyId: 17, stream: true },
      onKeyRejected,
    })
    let pending: Promise<boolean> | null = null
    await act(async () => {
      pending = result.current.sendChat(initialMessages)
    })
    const source = streamBoundary.ControlledSource.instances[0]

    await act(async () => {
      source.emit(
        'message',
        JSON.stringify({
          error: {
            code: 'upstream_error',
            message: 'Upstream refused the request',
          },
        })
      )
      source.emit('message', '[DONE]')
      await expect(pending).resolves.toBe(false)
    })

    expect(result.current.messages.at(-1)).toMatchObject({
      status: 'error',
      errorCode: 'upstream_error',
    })
    expect(result.current.messages.at(-1)?.versions[0].content).toContain(
      'Upstream refused the request'
    )
    expect(result.current.isGenerating).toBe(false)
    expect(streamBoundary.ControlledSource.instances).toHaveLength(1)
    expect(source.options.autoReconnect).toBe(false)
    expect(source.stream).toHaveBeenCalledOnce()
    expect(source.close).toHaveBeenCalledOnce()
    expect(onKeyRejected).not.toHaveBeenCalled()
  })

  describe.each([false, true])('with stream=%s', (stream) => {
    test.each(['playground_key_invalid', 'playground_key_required'])(
      'notifies key rejection once after the server returns %s',
      async (code) => {
        const error = {
          code,
          message: 'Select an available API key before sending',
        }
        const post = vi
          .spyOn(api, 'post')
          .mockRejectedValue({ response: { data: { error } } })
        const onKeyRejected = vi.fn()
        const { result } = renderChat({
          config: { ...DEFAULT_CONFIG, keyId: 17, stream },
          onKeyRejected,
        })
        let pending: Promise<boolean> | null = null

        await act(async () => {
          pending = result.current.sendChat(initialMessages)
        })
        await act(async () => {
          if (stream) {
            const source = streamBoundary.ControlledSource.instances[0]
            source.emit('message', JSON.stringify({ error }))
            source.emit('error', JSON.stringify({ error }))
            source.emit('message', '[DONE]')
          }
          await expect(pending).resolves.toBe(false)
        })

        expect(onKeyRejected).toHaveBeenCalledOnce()
        expect(result.current.messages.at(-1)).toMatchObject({
          status: 'error',
          errorCode: code,
        })
        expect(result.current.isGenerating).toBe(false)
        if (stream) {
          expect(post).not.toHaveBeenCalled()
          expect(streamBoundary.ControlledSource.instances).toHaveLength(1)
        } else {
          expect(post).toHaveBeenCalledOnce()
        }
      }
    )
  })
})

describe.each([false, true])(
  'strict budget rejection with stream=%s',
  (stream) => {
    test.each(['text-only', 'image attachment'])(
      'explains unsupported %s requirements and preserves the draft and key without retrying',
      async (inputKind) => {
        const error = {
          code: 'token_budget_unsupported_request',
          message: 'Strict budget request requirements were not met',
        }
        const post = vi
          .spyOn(api, 'post')
          .mockRejectedValue({ response: { data: { error } } })
        const onKeyRejected = vi.fn()
        const user = userEvent.setup()
        render(
          <BudgetRejectionChat stream={stream} onKeyRejected={onKeyRejected} />
        )
        const draft = screen.getByRole('textbox', { name: 'Message' })
        await user.type(draft, 'Keep this question')
        if (inputKind === 'image attachment') {
          await user.upload(
            screen.getByLabelText('Upload attachments'),
            new File(
              [new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10])],
              'photo.png',
              { type: 'image/png' }
            )
          )
          await screen.findByRole('img', { name: 'photo.png' })
        }
        await waitFor(() =>
          expect(screen.getByRole('button', { name: 'Send' })).toBeEnabled()
        )
        await user.click(screen.getByRole('button', { name: 'Send' }))
        if (stream) {
          await waitFor(() =>
            expect(streamBoundary.ControlledSource.instances).toHaveLength(1)
          )
          await act(async () => {
            const source = streamBoundary.ControlledSource.instances[0]
            source.emit('message', JSON.stringify({ error }))
            source.emit('error', JSON.stringify({ error }))
            source.emit('message', '[DONE]')
          })
        }

        await waitFor(() =>
          expect(screen.getByLabelText('Assistant response')).toHaveTextContent(
            "This request does not meet the selected key's strict budget requirements. Use a compatible API client or choose another key."
          )
        )
        expect(draft).toHaveValue('Keep this question')
        if (inputKind === 'image attachment') {
          expect(screen.getByRole('img', { name: 'photo.png' })).toBeVisible()
        }
        expect(screen.getByRole('combobox', { name: 'API key' })).toHaveValue(
          '17'
        )
        expect(screen.getByRole('button', { name: 'Send' })).toBeEnabled()
        expect(onKeyRejected).not.toHaveBeenCalled()
        if (stream) {
          expect(post).not.toHaveBeenCalled()
          expect(streamBoundary.ControlledSource.instances).toHaveLength(1)
          const source = streamBoundary.ControlledSource.instances[0]
          expect(source.options.autoReconnect).toBe(false)
          expect(source.stream).toHaveBeenCalledOnce()
          expect(source.close).toHaveBeenCalledOnce()
        } else {
          expect(post).toHaveBeenCalledOnce()
          expect(streamBoundary.ControlledSource.instances).toHaveLength(0)
        }
      }
    )
  }
)

describe('key-scoped model selection', () => {
  test('loads key choices without selecting the first key or requesting unscoped models', async () => {
    const get = vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: { items: [availableKey], total: 1 } },
    })
    const { result, updateConfig } = renderOptions(null)

    await waitFor(() => {
      expect(result.current.isLoadingKeys).toBe(false)
    })

    expect(result.current.keys).toEqual([availableKey])
    expect(result.current.selectedKey).toBeUndefined()
    expect(result.current.models).toEqual([])
    expect(result.current.canSend).toBe(false)
    expect(updateConfig).not.toHaveBeenCalled()
    expect(get).toHaveBeenCalledOnce()
    expect(get).toHaveBeenCalledWith('/pg/keys', {
      params: { p: 1, page_size: 100 },
      skipErrorHandler: true,
    })
  })

  test('loads separate models for each explicitly selected key and gates sending while they load', async () => {
    const secondKey = { ...availableKey, id: 28, name: 'Work chat' }
    const secondModels = deferred<{
      data: { success: true; data: Array<{ id: string }> }
    }>()
    const get = vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
      if (url === '/pg/keys') {
        return {
          data: {
            success: true,
            data: { items: [availableKey, secondKey], total: 2 },
          },
        }
      }
      if (config?.headers?.['X-MyAPI-Key-ID'] === '28') {
        return secondModels.promise
      }
      return { data: { success: true, data: [{ id: 'personal-model' }] } }
    })
    const { result, rerender, updateConfig } = renderOptions(17)
    await waitFor(() => {
      expect(result.current.canSend).toBe(true)
    })

    rerender({ keyId: 28, currentModel: 'personal-model' })

    expect(result.current.canSend).toBe(false)
    expect(result.current.models).toEqual([])
    await waitFor(() => {
      expect(get).toHaveBeenCalledWith(
        '/pg/models',
        expect.objectContaining({
          headers: expect.objectContaining({ 'X-MyAPI-Key-ID': '28' }),
          skipAuthRefresh: true,
          disableDuplicate: true,
        })
      )
    })
    await act(async () => {
      secondModels.resolve({
        data: { success: true, data: [{ id: 'work-model' }] },
      })
      await secondModels.promise
    })
    await waitFor(() => {
      expect(result.current.models).toEqual([
        { label: 'work-model', value: 'work-model' },
      ])
    })

    expect(get).toHaveBeenCalledWith(
      '/pg/models',
      expect.objectContaining({
        headers: expect.objectContaining({ 'X-MyAPI-Key-ID': '17' }),
        skipAuthRefresh: true,
        disableDuplicate: true,
      })
    )
    expect(updateConfig).toHaveBeenCalledWith('model', 'work-model')
    expect(updateConfig).not.toHaveBeenCalledWith('keyId', expect.anything())
    expect(result.current.canSend).toBe(false)

    rerender({ keyId: 28, currentModel: 'work-model' })

    expect(result.current.canSend).toBe(true)
  })

  test('does not fetch models or substitute another key when the stored key has disappeared', async () => {
    const get = vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: { items: [availableKey], total: 1 } },
    })
    const { result, updateConfig } = renderOptions(28)

    await waitFor(() => {
      expect(result.current.isLoadingKeys).toBe(false)
    })

    expect(result.current.selectedKey).toBeUndefined()
    expect(result.current.canSend).toBe(false)
    expect(result.current.keyNotice).toBe(
      'The selected API key is unavailable. Choose another key.'
    )
    expect(get).toHaveBeenCalledOnce()
    expect(updateConfig).not.toHaveBeenCalled()
  })

  test('keeps sending disabled when the selected key has no available models', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (url) => ({
      data:
        url === '/pg/keys'
          ? { success: true, data: { items: [availableKey], total: 1 } }
          : { success: true, data: [] },
    }))
    const { result } = renderOptions(17)

    await waitFor(() => {
      expect(result.current.keyNotice).toBe(
        'No models are available for this API key'
      )
    })

    expect(result.current.canSend).toBe(false)
    expect(result.current.models).toEqual([])
  })

  test('does not enable sending when key-scoped model discovery fails', async () => {
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/pg/keys') {
        return {
          data: { success: true, data: { items: [availableKey], total: 1 } },
        }
      }
      throw new Error('Model discovery failed')
    })
    const { result } = renderOptions(17)

    await waitFor(() => {
      expect(result.current.keyNotice).toBe('Failed to load playground models')
    })

    expect(result.current.canSend).toBe(false)
    expect(result.current.models).toEqual([])
  })

  test('a failed key refresh blocks sending even when models were previously cached', async () => {
    let failKeys = false
    vi.spyOn(api, 'get').mockImplementation(async (url) => {
      if (url === '/pg/keys') {
        if (failKeys) throw new Error('Key refresh failed')
        return {
          data: { success: true, data: { items: [availableKey], total: 1 } },
        }
      }
      return { data: { success: true, data: [{ id: 'personal-model' }] } }
    })
    const { result } = renderOptions(17)
    await waitFor(() => {
      expect(result.current.canSend).toBe(true)
    })

    failKeys = true
    act(() => {
      result.current.refreshKeys()
    })
    await waitFor(() => {
      expect(result.current.keyNotice).toBe('Failed to load API keys')
    })

    expect(result.current.canSend).toBe(false)
    expect(result.current.models).toEqual([])
  })
})

test('an unsupported image route is reported as a failure without retrying or clearing key identity', async () => {
  const post = vi.spyOn(api, 'post').mockRejectedValue({
    response: {
      data: {
        error: {
          code: 'playground_image_provider_unsupported',
          message: 'Unsupported route',
        },
      },
    },
  })
  const onKeyRejected = vi.fn()
  const { result } = renderChat({ onKeyRejected })
  await act(async () => {
    expect(await result.current.sendChat(initialMessages)).toBe(false)
  })
  expect(result.current.messages.at(-1)?.versions[0].content).toContain(
    'This provider route does not support images in this version. Choose an OpenAI-compatible or Codex route.'
  )
  expect(post).toHaveBeenCalledOnce()
  expect(onKeyRejected).not.toHaveBeenCalled()
})
