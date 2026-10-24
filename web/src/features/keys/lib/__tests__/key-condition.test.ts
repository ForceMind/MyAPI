/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { expect, test } from 'vitest'

import { getKeyCondition } from '../key-condition'

const key = (
  overrides: Partial<Parameters<typeof getKeyCondition>[0]> = {}
) => ({
  status: 1,
  remain_quota: 10,
  unlimited_quota: false,
  expired_time: -1,
  ...overrides,
})

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
  'maps the server Key state %j to %s without widening availability',
  (candidate, expected) => {
    expect(getKeyCondition(candidate, 100)).toBe(expected)
  }
)
