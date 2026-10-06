import { beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { completeCodexOAuth } from '../api'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), post: vi.fn() } }))
beforeEach(() => vi.resetAllMocks())

test('new OAuth completion posts creation metadata to the server', async () => {
  vi.mocked(api.post).mockResolvedValue({
    data: { success: true, data: { channel_id: 8 } },
  } as never)
  const create = {
    mode: 'single' as const,
    channel: { name: 'Synthetic Codex', type: 57, key: '' },
  }
  await completeCodexOAuth('code=fixture&state=fixture', undefined, create)
  expect(api.post).toHaveBeenCalledWith(
    '/api/channel/codex/oauth/complete',
    { input: 'code=fixture&state=fixture', create },
    expect.any(Object)
  )
})

test('existing-channel OAuth completion uses the channel-specific route without creation fields', async () => {
  vi.mocked(api.post).mockResolvedValue({ data: { success: true } } as never)
  await completeCodexOAuth('code=fixture&state=fixture', 7)
  expect(api.post).toHaveBeenCalledWith(
    '/api/channel/7/codex/oauth/complete',
    { input: 'code=fixture&state=fixture', create: undefined },
    expect.any(Object)
  )
})
