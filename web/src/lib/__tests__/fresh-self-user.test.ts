/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { expect, test } from 'vitest'

import { parseFreshSelfUser } from '../fresh-self-user'

test('accepts only a fresh self response for the active identity', () => {
  expect(() =>
    parseFreshSelfUser(
      { success: true, data: { id: 8, username: 'other', role: 100 } },
      7
    )
  ).toThrow('identity mismatch')
  expect(() =>
    parseFreshSelfUser(
      { success: true, data: { id: 7, username: 'owner', role: 99 } },
      7
    )
  ).toThrow('identity mismatch')
  expect(
    parseFreshSelfUser(
      { success: true, data: { id: 7, username: 'owner', role: 1 } },
      7
    ).id
  ).toBe(7)
})

test.each([
  null,
  'text',
  { success: false, data: { id: 7 } },
  { success: true },
])(
  'an unusable self response %j is rejected rather than trusted',
  (payload) => {
    expect(() => parseFreshSelfUser(payload, 7)).toThrow(
      'Self response unavailable'
    )
  }
)
