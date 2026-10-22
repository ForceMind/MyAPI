/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { expect, test } from 'vitest'

import {
  keyRequestContextSchema,
  savedKeyIdSchema,
} from '../../key-request-context'

test('return context preserves list position without credentials or arbitrary destinations', () => {
  expect(
    keyRequestContextSchema.parse({
      page: 3,
      pageSize: 20,
      filter: 'same-name',
      focusKeyId: 21,
      token: 'NEVER-COPY-CREDENTIAL',
      returnUrl: 'https://outside.invalid',
    })
  ).toEqual({ page: 3, pageSize: 20, filter: 'same-name', focusKeyId: 21 })
})

test.each([0, -1, 1.5, 2147483648, 'broken', true, false, null])(
  'rejects an invalid saved Key ID %s rather than widening the request search',
  (value) => {
    expect(savedKeyIdSchema.safeParse(value).success).toBe(false)
  }
)
