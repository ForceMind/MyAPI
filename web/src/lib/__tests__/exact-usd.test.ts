import { expect, test } from 'vitest'

import { usdAmountSchema, usdLessThan } from '../exact-usd'

test('USD comparisons preserve precision beyond JavaScript Number', () => {
  expect(
    usdLessThan('0.00000000000000000000000035', '0.00000000000000000000000036')
  ).toBe(true)
  expect(usdLessThan('9007199254740992.01', '9007199254740992.02')).toBe(true)
  expect(usdLessThan('1.0', '1.00')).toBe(false)
  expect(usdLessThan('1.01', '1.001')).toBe(false)
  for (const amount of [
    '1e3',
    '-1',
    'NaN',
    '0x10',
    '0.',
    '00',
    '9'.repeat(129),
  ]) {
    expect(usdAmountSchema.safeParse(amount).success).toBe(false)
  }
})
