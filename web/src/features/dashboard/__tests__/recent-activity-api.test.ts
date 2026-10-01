import { afterEach, expect, test, vi } from 'vitest'

import { getRecentLogOverview } from '../api'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: { get } }))

afterEach(() => get.mockReset())

test.each([
  { admin: true, path: '/api/log/overview' },
  { admin: false, path: '/api/log/self/overview' },
])(
  'recent activity $path leaves failure presentation to the inline panel',
  async ({ admin, path }) => {
    get.mockResolvedValue({
      data: { success: true, data: { requests: [], errors: [] } },
    })

    await getRecentLogOverview(admin)

    expect(get).toHaveBeenCalledWith(path, {
      skipBusinessError: true,
      skipErrorHandler: true,
    })
  }
)
