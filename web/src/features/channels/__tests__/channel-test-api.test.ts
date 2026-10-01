import { beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { testChannel } from '../api'

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), post: vi.fn() },
}))

beforeEach(() => vi.resetAllMocks())

test('detailed test sends model and streaming options in POST', async () => {
  vi.mocked(api.post).mockResolvedValue({ data: { success: true } } as never)
  const options = {
    model: 'gpt-5-codex',
    endpoint_type: 'openai-response',
    stream: true,
  }

  await testChannel(12, options)

  expect(api.post).toHaveBeenCalledWith(
    '/api/channel/test/12',
    options,
    expect.any(Object)
  )
  expect(api.get).not.toHaveBeenCalled()
})

test('shortcut test leaves options to the server previous-success record', async () => {
  vi.mocked(api.get).mockResolvedValue({ data: { success: true } } as never)

  await testChannel(12)

  expect(api.get).toHaveBeenCalledWith(
    '/api/channel/test/12',
    expect.objectContaining({ params: undefined })
  )
  expect(api.post).not.toHaveBeenCalled()
})
