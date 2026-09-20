import { describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import {
  createPromptLearningVersion,
  getPromptLearningPolicy,
  getPromptLearningVersions,
  updatePromptLearningPolicy,
} from '../api'

vi.mock('@/lib/api', () => ({
  api: {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
  },
}))

describe('prompt learning API', () => {
  test('uses session-bound policy and version endpoints', async () => {
    vi.mocked(api.get).mockResolvedValue({
      data: { success: true, data: {} },
    } as never)
    vi.mocked(api.put).mockResolvedValue({
      data: { success: true, data: {} },
    } as never)
    vi.mocked(api.post).mockResolvedValue({
      data: { success: true, data: {} },
    } as never)

    await getPromptLearningPolicy()
    await updatePromptLearningPolicy(true)
    await getPromptLearningVersions(2, 10)
    await createPromptLearningVersion({
      commandId: 'manual-1',
      content: 'Instructions',
      parentId: 7,
    })

    expect(api.get).toHaveBeenNthCalledWith(1, '/api/user/prompt-learning')
    expect(api.put).toHaveBeenLastCalledWith('/api/user/prompt-learning', {
      enabled: true,
    })
    expect(api.get).toHaveBeenNthCalledWith(
      2,
      '/api/user/prompt-learning/versions',
      {
        params: { p: 2, page_size: 10 },
      }
    )
    expect(api.post).toHaveBeenLastCalledWith(
      '/api/user/prompt-learning/versions',
      {
        command_id: 'manual-1',
        content: 'Instructions',
        parent_id: 7,
      }
    )
  })

  test('rejects failed business envelopes before UI state is updated', async () => {
    vi.mocked(api.get).mockResolvedValueOnce({
      data: { success: false, message: 'session required' },
    } as never)

    await expect(getPromptLearningPolicy()).rejects.toThrow('session required')
  })
})
