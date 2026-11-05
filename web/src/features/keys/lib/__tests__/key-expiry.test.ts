import { expect, test } from 'vitest'

import { keyExpiry } from '../key-expiry'

test.each([null, undefined, 0, -2, Number.NaN, Number.MAX_SAFE_INTEGER, 1.5])('invalid or absent expiry %s is unknown and never becomes perpetual', (value) => {
  expect(keyExpiry(value)).toEqual({ kind: 'unknown' })
})

test('only the explicit server sentinel means never expires', () => {
  expect(keyExpiry(-1)).toEqual({ kind: 'never' })
  expect(keyExpiry(1700000000)).toEqual({ kind: 'dated', timestamp: 1700000000 })
})
