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
