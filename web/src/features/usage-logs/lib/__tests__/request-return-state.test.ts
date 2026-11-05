import { expect, test } from 'vitest'

import { createRequestReturnStateCache } from '../request-return-state'

const point = { top: 321, left: 17, logId: 8, kind: 'table' as const, viewScope: 'self' as const }

test('a request return point belongs to one actor and one history entry and is consumed once', () => {
  const cache = createRequestReturnStateCache()
  cache.write('source-entry', 'actor-session-role', point)
  expect(cache.read('different-filter-entry', 'actor-session-role')).toBeUndefined()
  expect(cache.take('source-entry', 'actor-session-role')).toEqual(point)
  expect(cache.take('source-entry', 'actor-session-role')).toBeUndefined()
})

test('switching actor invalidates the earlier account return point even if it later returns', () => {
  const cache = createRequestReturnStateCache()
  cache.write('source-entry', 'first-actor', point)
  expect(cache.read('source-entry', 'next-actor')).toBeUndefined()
  expect(cache.read('source-entry', 'first-actor')).toBeUndefined()
})

test('only the twenty most recent numeric return points are retained', () => {
  const cache = createRequestReturnStateCache()
  for (let entry = 1; entry <= 21; entry += 1) cache.write(String(entry), 'actor', { ...point, logId: entry })
  expect(cache.read('1', 'actor')).toBeUndefined()
  expect(cache.read('2', 'actor')?.logId).toBe(2)
  expect(cache.read('21', 'actor')?.logId).toBe(21)
})
