/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { describe, expect, test } from 'vitest'

import type { FullContentLogFilters } from '../../types'
import { getFullContentLogsQueryKey } from '../query-key'

const filters: FullContentLogFilters = {
  model: '',
  token: '',
  requestId: '',
  startTime: '',
  endTime: '',
}

describe('full content log query key', () => {
  test('separates rows by user and session identity', () => {
    const first = getFullContentLogsQueryKey(1, filters, 7, 'session-a')
    const second = getFullContentLogsQueryKey(1, filters, 8, 'session-b')

    expect(first).not.toEqual(second)
    expect(first.slice(2, 4)).toEqual([7, 'session-a'])
  })
})
