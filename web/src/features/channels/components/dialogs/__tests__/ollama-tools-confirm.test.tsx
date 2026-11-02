import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useEffect, useState } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { channelSchema } from '../../../types'
import { ChannelsProvider, useChannels } from '../../channels-provider'
import { OllamaModelsDialog } from '../ollama-models-dialog'

vi.mock('@/lib/api', async (importOriginal) => {
  const original = await importOriginal<typeof import('@/lib/api')>()
  return { ...original, getFreshAuthHeaders: vi.fn().mockResolvedValue({}) }
})

let client: QueryClient
const channel = channelSchema.parse({ id: 23, type: 4, name: 'synthetic-ollama-channel', key: '', status: 1, base_url: 'https://synthetic.invalid', created_time: 1, test_time: 0, response_time: 0, balance_updated_time: 0 })
beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/channel/fetch_models/23') return { data: { success: true, data: ['synthetic-ollama-model'] } }
    throw new Error(`Unexpected GET ${url}`)
  })
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Unexpected provider POST'))
  vi.spyOn(api, 'delete').mockRejectedValue(new Error('Synthetic Ollama delete failed'))
  vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('Unexpected provider stream'))
})
afterEach(() => { cleanup(); client.clear(); vi.restoreAllMocks() })

function Tools() {
  const [open, setOpen] = useState(true)
  const { setCurrentRow } = useChannels()
  useEffect(() => { setCurrentRow(channel) }, [setCurrentRow])
  return <OllamaModelsDialog open={open} onOpenChange={setOpen} />
}

it('keeps the exact Ollama model confirmation and failure visible after a failed delete without starting a pull', async () => {
  render(<QueryClientProvider client={client}><ChannelsProvider><Tools /></ChannelsProvider></QueryClientProvider>)
  await screen.findByText('synthetic-ollama-model')
  const dialog = screen.getByRole('dialog')
  // The existing row action is icon-only; keep the same query valid after it gains a label.
  const remove = within(dialog).getAllByRole('button').find((button) => button.textContent?.trim() === '' && button.getAttribute('data-slot') !== 'dialog-close')
  if (!remove) throw new Error('Ollama model delete action is missing')
  const user = userEvent.setup()
  await user.click(remove)
  const confirm = await screen.findByRole('alertdialog')
  expect(confirm).toHaveTextContent('synthetic-ollama-model')
  await user.click(within(confirm).getByRole('button', { name: 'Delete' }))
  expect(await screen.findByText('Synthetic Ollama delete failed')).toBeVisible()
  expect(screen.getByRole('alertdialog')).toHaveTextContent('synthetic-ollama-model')
  expect(api.delete).toHaveBeenCalledTimes(1)
  expect(globalThis.fetch).not.toHaveBeenCalled()
})

it('aborts the existing pull stream when the user closes its task without issuing a model delete', async () => {
  let pullSignal: AbortSignal | undefined
  vi.mocked(globalThis.fetch).mockImplementation(async (_url, init) => {
    const signal = init?.signal
    if (!signal) throw new Error('Pull stream signal is missing')
    pullSignal = signal
    const body = new ReadableStream<Uint8Array>({ start(controller) {
      signal.addEventListener('abort', () => controller.error(new DOMException('Synthetic cancellation', 'AbortError')), { once: true })
    } })
    return new Response(body, { status: 200, headers: { 'Content-Type': 'text/event-stream' } })
  })
  render(<QueryClientProvider client={client}><ChannelsProvider><Tools /></ChannelsProvider></QueryClientProvider>)
  await screen.findByText('synthetic-ollama-model')
  const user = userEvent.setup()
  await user.type(screen.getByLabelText('Pull model'), 'synthetic-new-model')
  await user.click(screen.getByRole('button', { name: 'Pull' }))
  await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledTimes(1))
  const close = screen.getAllByRole('button', { name: 'Close' }).at(-1)
  if (!close) throw new Error('Pull task close action is missing')
  await user.click(close)
  await waitFor(() => expect(pullSignal?.aborted).toBe(true))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(api.delete).not.toHaveBeenCalled()
})
