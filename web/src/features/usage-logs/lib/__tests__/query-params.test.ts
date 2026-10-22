import { describe, expect, test } from 'vitest'

import { buildQueryParams } from '../query-params'
import { buildApiParams } from '../utils'

describe('usage log query parameters', () => {
  test('keeps a stable Key ID alongside name, request and connection filters', () => {
    const params = buildApiParams({
      page: 2,
      pageSize: 20,
      isAdmin: true,
      searchParams: {
        tokenId: 21,
        token: 'renamed',
        channel: '7',
        requestId: 'saved-request',
      },
    })
    expect(params).toMatchObject({
      token_id: 21,
      token_name: 'renamed',
      channel: 7,
      request_id: 'saved-request',
      p: 2,
    })
    expect(
      buildQueryParams(params as unknown as Record<string, unknown>).get(
        'token_id'
      )
    ).toBe('21')
  })
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
