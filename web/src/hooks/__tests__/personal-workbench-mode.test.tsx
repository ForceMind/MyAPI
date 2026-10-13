import { renderHook } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import type { SystemStatus } from '@/features/auth/types'

import { usePersonalWorkbenchMode } from '../use-personal-workbench-mode'
import { useStatus } from '../use-status'

vi.mock('../use-status', () => ({ useStatus: vi.fn() }))
vi.mock('@/lib/self-use-build', () => ({ SELF_USE_MINIMAL: true }))

const disabled = {
  user_funding_mode: 'disabled',
  user_funding_capabilities: { mode: 'disabled', ready: true, epoch: 4 },
}

afterEach(() => vi.resetAllMocks())

describe('personal workbench server confirmation', () => {
  test.each([
    ['confirmed disabled', disabled, true, true],
    ['cached disabled placeholder', disabled, false, false],
    ['missing status', null, true, false],
    ['missing capabilities', { user_funding_mode: 'disabled' }, true, false],
    ['not ready', { ...disabled, user_funding_capabilities: { ...disabled.user_funding_capabilities, ready: false } }, true, false],
    ['zero epoch', { ...disabled, user_funding_capabilities: { ...disabled.user_funding_capabilities, epoch: 0 } }, true, false],
    ['unsafe epoch', { ...disabled, user_funding_capabilities: { ...disabled.user_funding_capabilities, epoch: Number.MAX_SAFE_INTEGER + 1 } }, true, false],
    ['mismatched modes', { ...disabled, user_funding_mode: 'enabled' }, true, false],
    ['enabled in a minimal build', { user_funding_mode: 'enabled', user_funding_capabilities: { mode: 'enabled', ready: true, epoch: 4 } }, true, false],
    ['retirement', { user_funding_mode: 'retirement', user_funding_capabilities: { mode: 'retirement', ready: true, epoch: 4 } }, true, false],
    ['capabilities-only disabled', { user_funding_capabilities: disabled.user_funding_capabilities }, true, false],
  ])('%s', (_name, status, confirmed, expected) => {
    vi.mocked(useStatus).mockReturnValue({
      status: status as SystemStatus | null,
      confirmed: confirmed as boolean,
      loading: false,
      error: null,
    })
    const { result } = renderHook(() => usePersonalWorkbenchMode())
    expect(result.current).toBe(expected)
  })
})
