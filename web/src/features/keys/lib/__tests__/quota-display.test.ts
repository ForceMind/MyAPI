import { describe, expect, test } from 'vitest'

import { checkedAddSafeIntegers } from '../quota-display'

describe('checkedAddSafeIntegers', () => {
  test.each([
    [Number.MAX_SAFE_INTEGER - 1, 1, Number.MAX_SAFE_INTEGER],
    [Number.MIN_SAFE_INTEGER + 1, -1, Number.MIN_SAFE_INTEGER],
    [Number.MAX_SAFE_INTEGER, -Number.MAX_SAFE_INTEGER, 0],
  ])('returns the exact sum for %s + %s', (left, right, expected) => {
    expect(checkedAddSafeIntegers(left, right)).toBe(expected)
  })

  test.each([
    [Number.MAX_SAFE_INTEGER, 1],
    [Number.MIN_SAFE_INTEGER, -1],
  ])('returns null before adding an unrepresentable %s + %s', (left, right) => {
    expect(checkedAddSafeIntegers(left, right)).toBeNull()
  })
})
