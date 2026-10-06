import { describe, expect, test } from 'vitest'

import { buildQueryParams } from '../query-params'

describe('usage log query parameters', () => {
  test('preserves explicit zero filters while omitting absent values', () => {
    expect(
      buildQueryParams({
        type: 0,
        page: 1,
        empty: '',
        absent: undefined,
      }).toString()
    ).toBe('type=0&page=1')
  })
})
