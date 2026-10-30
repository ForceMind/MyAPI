import { describe, expect, test } from 'vitest'

import { getLogPageCount, normalizeLogPagination } from '../log-pagination'

describe('log server pagination boundaries', () => {
  test.each([
    [4, 500, { pageIndex: 4, pageSize: 100 }],
    [-1, -20, { pageIndex: 0, pageSize: 20 }],
    [1.5, NaN, { pageIndex: 0, pageSize: 20 }],
    [Number.MAX_SAFE_INTEGER, Infinity, { pageIndex: 0, pageSize: 20 }],
    [2, 1, { pageIndex: 2, pageSize: 1 }],
    [2, 30, { pageIndex: 2, pageSize: 30 }],
  ])('bounds outgoing page and size (%s, %s)', (page, size, expected) => {
    expect(normalizeLogPagination(page, size)).toEqual(expected)
  })

  test.each([undefined, null, NaN, Infinity, -1, 1.5, Number.MAX_SAFE_INTEGER + 1])('invalid totals remain unknown (%s)', (total) => {
    expect(getLogPageCount(total as number | undefined, 20)).toBeNull()
  })

  test('only an explicit zero produces a confirmed empty page', () => {
    expect(getLogPageCount(0, 20)).toBe(1)
    expect(getLogPageCount(121, 20)).toBe(7)
    expect(getLogPageCount(121, 50)).toBe(3)
    expect(getLogPageCount(121, 100)).toBe(2)
    expect(getLogPageCount(121, 0)).toBeNull()
  })
})
