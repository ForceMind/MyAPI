/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { describe, expect, test } from 'vitest'

import {
  createFullContentLogListState,
  createFullContentLogListStateCache,
  getFullContentPageCount,
} from '../list-state'

describe('full content log list navigation state', () => {
  test('returning to a history entry restores filters, page and page size', () => {
    const cache = createFullContentLogListStateCache()
    const initial = cache.read('list-entry', 7, 'session-a')
    const state = {
      ...initial,
      page: 3,
      pageSize: 50 as const,
      filters: { ...initial.filters, model: 'synthetic-model', token: '12' },
    }
    cache.write('list-entry', 7, 'session-a', state)
    expect(cache.read('list-entry', 7, 'session-a')).toEqual(state)
    expect(cache.read('new-entry', 7, 'session-a')).toEqual(initial)
  })

  test.each([
    [8, 'session-a'],
    [7, 'session-b'],
    [null, null],
  ] as const)('switching identity to %s / %s clears the earlier navigation state', (userId, sessionId) => {
    const cache = createFullContentLogListStateCache()
    const initial = cache.read('list-entry', 7, 'session-a')
    cache.write('list-entry', 7, 'session-a', { ...initial, page: 3 })
    expect(cache.read('list-entry', userId, sessionId)).toEqual(initial)
    expect(cache.read('list-entry', 7, 'session-a')).toEqual(initial)
  })

  test('retains at most twenty history entries and rejects writes from a stale session', () => {
    const cache = createFullContentLogListStateCache()
    const state = { ...cache.read('0', 7, 'session-a'), page: 2 }
    for (let index = 0; index < 21; index += 1) {
      cache.write(String(index), 7, 'session-a', state)
    }
    expect(cache.read('0', 7, 'session-a').page).toBe(1)
    expect(cache.read('20', 7, 'session-a').page).toBe(2)
    cache.read('20', 7, 'session-b')
    cache.write('20', 7, 'session-a', state)
    expect(cache.read('20', 7, 'session-b').page).toBe(1)
  })

  test('cache snapshots do not change when a caller edits its filter object', () => {
    const cache = createFullContentLogListStateCache()
    const state = cache.read('list-entry', 7, 'session-a')
    cache.write('list-entry', 7, 'session-a', state)
    state.filters.model = 'modified-draft'
    expect(cache.read('list-entry', 7, 'session-a').filters.model).toBe('')
  })

  test.each([undefined, Number.NaN, -1, Number.POSITIVE_INFINITY])(
    'an unknown or invalid total %s has no page count',
    (total) => expect(getFullContentPageCount(total, 20)).toBeNull()
  )

  test('a confirmed empty result has one page and a partial last page is counted', () => {
    expect(getFullContentPageCount(0, 20)).toBe(1)
    expect(getFullContentPageCount(101, 50)).toBe(3)
    expect(createFullContentLogListState().pageSize).toBe(20)
  })
})
