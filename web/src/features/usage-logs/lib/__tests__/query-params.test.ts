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
  test('passes the request-failure filter only when the link asks for it', () => {
    const failed = buildApiParams({
      page: 1,
      pageSize: 20,
      isAdmin: false,
      searchParams: { type: ['5'], result: 'failed' },
    })
    expect(
      buildQueryParams(failed as unknown as Record<string, unknown>).get(
        'result'
      )
    ).toBe('failed')
    const plain = buildApiParams({
      page: 1,
      pageSize: 20,
      isAdmin: false,
      searchParams: { type: ['5'] },
    })
    expect(plain.result).toBeUndefined()
    // An unknown value never reaches the server (it would be a 400).
    const unknown = buildApiParams({
      page: 1,
      pageSize: 20,
      isAdmin: false,
      searchParams: { result: 'pending' },
    })
    expect(unknown.result).toBeUndefined()
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
