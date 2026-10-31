/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { expect, test } from 'vitest'

import {
  connectionValues,
  connectionListContext,
  connectionsToTableSearch,
  tableToConnectionsSearch,
  connectionPageSearchSchema,
  connectionFilterValue,
  connectionsSearchSchema,
  parseConnection,
  parseConnections,
} from '../connection-state'

const item = {
  id: 7,
  type: 1,
  name: 'Connection',
  status: 1,
  models: 'alpha,beta',
  priority: 0,
  key: 'NEVER-CACHE',
  other: 'opaque',
  settings: 'opaque',
  base_url: 'private-endpoint',
}

test('list and detail cache only whitelisted metadata, preserving explicit zero and unknown values', () => {
  const list = parseConnections({
    success: true,
    data: { total: 21, items: [item] },
  })
  const detail = parseConnection({ success: true, data: item }, 7)
  expect(list.total).toBe(21)
  expect(detail).toEqual({
    id: 7,
    type: 1,
    name: 'Connection',
    status: 1,
    models: 'alpha,beta',
    priority: 0,
  })
  expect(list.items).toEqual([detail])
  expect(detail.weight).toBeUndefined()
  expect(JSON.stringify(list)).not.toContain('NEVER-CACHE')
})

test.each([
  { success: false, data: { total: 1, items: [item] } },
  { success: true, data: { total: 0, items: [item] } },
  { success: true, data: { total: 2, items: [item, item] } },
  { success: true, data: { total: -1, items: [] } },
  { success: true, data: { total: 1, items: [{ ...item, id: '7' }] } },
])(
  'rejects malformed lists instead of presenting misleading empty or duplicated connections',
  (payload) => {
    expect(() => parseConnections(payload)).toThrow()
  }
)

test('detail cannot silently substitute a different connection ID', () => {
  expect(() => parseConnection({ success: true, data: item }, 8)).toThrow(
    'Connection identity changed'
  )
})

test('bookmarked filters and selection reject invalid IDs and keep valid state', () => {
  expect(
    connectionsSearchSchema.parse({ page: -2, connection: 1.5, status: 'all' })
  ).toEqual({
    page: undefined,
    connection: undefined,
    status: undefined,
  })
  expect(
    connectionsSearchSchema.parse({
      page: 2,
      connection: 7,
      filter: ' alpha ',
      status: 'disabled',
    })
  ).toEqual({ page: 2, connection: 7, filter: 'alpha', status: 'disabled' })
})

test('model and routing labels are normalized without creating default evidence', () => {
  expect(connectionValues(' alpha, beta,alpha, ')).toEqual(['alpha', 'beta'])
  expect(connectionValues(undefined)).toEqual([])
})


test('table and card view changes preserve every filter, old page sizes and advanced quota context', () => {
  const table = {
    page: 4,
    pageSize: 10,
    filter: 'retained query',
    model: 'synthetic-model',
    status: ['disabled'],
    type: ['57', '1'],
    group: ['primary', 'secondary'],
    quotaChannelId: 23,
  }
  const cards = connectionsSearchSchema.parse(tableToConnectionsSearch(table))
  expect(cards.status).toBe('disabled')
  expect(connectionsToTableSearch(cards)).toEqual(table)
  const context = connectionListContext({ ...cards, view: 'table' })
  expect(connectionPageSearchSchema.parse({ connectionReturn: context }).connectionReturn).toEqual({ ...cards, view: 'table' })
  expect(connectionFilterValue(cards.type)).toBe('57')
  expect(connectionFilterValue(cards.group)).toBe('primary')
  expect(connectionFilterValue(['all'])).toBeUndefined()
  expect(connectionFilterValue(['enabled', 'all'])).toBeUndefined()
  expect(connectionFilterValue(['4', 'all'])).toBeUndefined()
})

test('unbounded or fractional pagination cannot enter the connection list while valid legacy sizes remain', () => {
  expect(connectionsSearchSchema.parse({ pageSize: 1000, page: Number.MAX_SAFE_INTEGER + 1 })).toEqual({ pageSize: undefined, page: undefined })
  expect(connectionsSearchSchema.parse({ pageSize: 1, page: 3 })).toEqual({ pageSize: 1, page: 3 })
  expect(connectionsSearchSchema.parse({ pageSize: 2.5 })).toEqual({ pageSize: undefined })
})

test('view changes keep unrecognized legacy status arrays and long existing search strings', () => {
  const table = { status: ['legacy', 'enabled'], filter: 'x'.repeat(250), type: [], group: [], pageSize: 20 }
  expect(connectionsToTableSearch(connectionsSearchSchema.parse(tableToConnectionsSearch(table)))).toEqual(table)
})
