import { describe, expect, test } from 'vitest'

import type { PlaygroundKey } from '@/features/playground/types'

import {
  getFreshUsagePolicy,
  getPlaygroundKeyCondition,
  parseFreshSelfUser,
} from '../workbench-state'

function key(overrides: Partial<PlaygroundKey> = {}): PlaygroundKey {
  return {
    id: 1,
    name: 'personal-key',
    status: 1,
    group: 'default',
    remain_quota: 10,
    unlimited_quota: false,
    expired_time: -1,
    ...overrides,
  }
}

describe('personal workbench state boundaries', () => {
  test.each([
    [key({ status: 1 }), 'available'],
    [key({ status: 2 }), 'disabled'],
    [key({ status: 3 }), 'expired'],
    [key({ status: 4 }), 'exhausted'],
    [key({ status: 99 }), 'unknown'],
    [key({ expired_time: Number.NaN }), 'unknown'],
    [key({ remain_quota: Number.NaN }), 'unknown'],
    [key({ remain_quota: Number.MAX_SAFE_INTEGER + 1 }), 'unknown'],
    [key({ remain_quota: 0.5 }), 'unknown'],
    [key({ expired_time: Number.MAX_SAFE_INTEGER + 1 }), 'unknown'],
    [key({ expired_time: 99 }), 'expired'],
    [key({ remain_quota: 0 }), 'exhausted'],
    [key({ remain_quota: 0, unlimited_quota: true }), 'available'],
  ] as const)(
    'maps the server Key state without widening availability',
    (candidate, expected) => {
      expect(getPlaygroundKeyCondition(candidate, 100)).toBe(expected)
    }
  )

  test('accepts only a fresh self response for the active identity', () => {
    expect(() =>
      parseFreshSelfUser(
        {
          success: true,
          data: { id: 8, username: 'other', role: 100 },
        },
        7
      )
    ).toThrow('identity mismatch')
    expect(() =>
      parseFreshSelfUser(
        {
          success: true,
          data: { id: 7, username: 'owner', role: 99 },
        },
        7
      )
    ).toThrow('identity mismatch')

    expect(
      parseFreshSelfUser(
        {
          success: true,
          data: { id: 7, username: 'owner', role: 1 },
        },
        7
      ).id
    ).toBe(7)
  })

  test('does not build a confirmed policy from incomplete cached fields', () => {
    expect(
      getFreshUsagePolicy({ id: 7, username: 'owner', role: 1 })
    ).toBeUndefined()
    expect(
      getFreshUsagePolicy({
        id: 7,
        username: 'owner',
        role: 1,
        quota: 123,
        self_use_no_balance: true,
        usage_policy_revision: 2,
      })
    ).toEqual({
      user_id: 7,
      no_balance: true,
      revision: 2,
      legacy_remaining_quota: 123,
    })
  })
})
