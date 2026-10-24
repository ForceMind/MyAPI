import { renderHook } from '@testing-library/react'
import { beforeEach, expect, test, vi } from 'vitest'

import { useStatus } from '@/hooks/use-status'
import { MYAPI_DOCS_URL } from '@/lib/build-branding'
import { useAuthStore } from '@/stores/auth-store'

import { useTopNavLinks } from '../use-top-nav-links'

vi.mock('@/hooks/use-status', () => ({ useStatus: vi.fn() }))

beforeEach(() => {
  useAuthStore.getState().auth.reset('idle')
  vi.mocked(useStatus).mockReturnValue({
    status: {},
    loading: false,
    error: null,
    confirmed: true,
  })
})

test('missing runtime docs link opens the project README instead of an absent local route', () => {
  const { result } = renderHook(() => useTopNavLinks())

  expect(result.current.find((link) => link.title === 'Docs')).toMatchObject({
    href: MYAPI_DOCS_URL,
    external: true,
  })
})

test('explicit local docs link keeps its internal navigation behavior', () => {
  vi.mocked(useStatus).mockReturnValue({
    status: { docs_link: '/custom-docs' },
    loading: false,
    error: null,
    confirmed: true,
  })

  const { result } = renderHook(() => useTopNavLinks())

  expect(result.current.find((link) => link.title === 'Docs')).toMatchObject({
    href: '/custom-docs',
    external: false,
  })
})

test('explicit external docs link keeps its target', () => {
  vi.mocked(useStatus).mockReturnValue({
    status: { docs_link: 'https://docs.example.test' },
    loading: false,
    error: null,
    confirmed: true,
  })

  const { result } = renderHook(() => useTopNavLinks())

  expect(result.current.find((link) => link.title === 'Docs')).toMatchObject({
    href: 'https://docs.example.test',
    external: true,
  })
})

test.each([
  ['full', true],
  ['lite', false],
] as const)(
  'a %s edition decides whether Model Square and Rankings are offered',
  (edition, offered) => {
    useAuthStore.getState().auth.setUser({ id: 2, username: 'member', role: 1 })
    vi.mocked(useStatus).mockReturnValue({
      status: {
        HeaderNavModules: JSON.stringify({
          pricing: { enabled: true, requireAuth: false },
          rankings: { enabled: true, requireAuth: false },
        }),
        product_edition: edition,
        capabilities: { public_pricing: offered, rankings: offered },
      },
      loading: false,
      error: null,
      confirmed: true,
    })

    const { result } = renderHook(() => useTopNavLinks())
    const hrefs = new Set(result.current.map((link) => link.href))

    expect(hrefs.has('/pricing')).toBe(offered)
    expect(hrefs.has('/rankings')).toBe(offered)
  }
)
