import { expect, test } from 'vitest'

import { hasSavedSegments, parseSavedSegment, savedByteOffset, SAVED_SEGMENT_LIMIT } from '../saved-segment'
import type { FullContentLogDetail, SavedContentSegment } from '../../types'

const snapshot = 'a'.repeat(64)
const page: SavedContentSegment = { request_id: 'saved-1', part: 'response', requested_offset: '0', offset: '0', next_offset: null, total_bytes: '4', expected_bytes: null, has_more: false, snapshot, encoding: 'utf-8', content_type: 'text/plain', body: 'TAIL', source_complete: null }

test.each(['', '00', '-1', '1.0', '1e3', ' 0', '9223372036854775808', 0, null])('rejects noncanonical or overflowing saved byte offset %s', (value) => {
  expect(savedByteOffset(value)).toBeNull()
})

test('large canonical offsets preserve exact decimal positions beyond JavaScript safe integers', () => {
  const large = { ...page, requested_offset: '9007199254740993', offset: '9007199254740993', total_bytes: '9007199254740997' }
  expect(parseSavedSegment({ success: true, data: large }, 'saved-1', { part: 'response', offset: large.offset, snapshot })).toEqual(large)
})

test.each([
  { expected_bytes: null, source_complete: null },
  { expected_bytes: '0', source_complete: true },
  { expected_bytes: '4', source_complete: false },
])('empty saved bytes retain nullable metadata and explicit capture evidence: %j', (metadata) => {
  const empty = { ...page, ...metadata, total_bytes: '0', body: '' }
  expect(parseSavedSegment({ success: true, data: empty }, 'saved-1', { part: 'response', offset: '0' })).toEqual(empty)
})

test.each([
  { request_id: 'other' }, { part: 'request' }, { requested_offset: '1' },
  { offset: '1' }, { next_offset: '4' }, { has_more: true }, { total_bytes: 4 },
  { expected_bytes: undefined }, { source_complete: undefined }, { snapshot: 'bad' },
  { body: 'LONGER' }, { body: '\uD800', total_bytes: '3' },
  { encoding: 'base64', body: 'AB==', total_bytes: '1' },
])('invalid identity, byte ranges or metadata cannot become a saved segment: %j', (override) => {
  expect(() => parseSavedSegment({ success: true, data: { ...page, ...override } }, 'saved-1', { part: 'response', offset: '0' })).toThrow('Segment data is invalid.')
})

test('UTF8 alignment may move back at most three bytes and the cursor follows decoded bytes', () => {
  const unicode = { ...page, requested_offset: '1', offset: '0', body: '😀', next_offset: '4', total_bytes: '5', has_more: true }
  expect(parseSavedSegment({ success: true, data: unicode }, 'saved-1', { part: 'response', offset: '1', snapshot, limit: 4 })).toEqual(unicode)
  expect(() => parseSavedSegment({ success: true, data: { ...unicode, requested_offset: '4' } }, 'saved-1', { part: 'response', offset: '4', snapshot, limit: 4 })).toThrow()
})

test('the byte budget applies to decoded UTF8 rather than JSON escaping overhead', () => {
  const body = '\0\n\r\t'
  expect(JSON.stringify(body).length).toBeGreaterThan(4)
  expect(parseSavedSegment({ success: true, data: { ...page, body } }, 'saved-1', { part: 'response', offset: '0', limit: 4 }).body).toBe(body)
})

test('binary data is validated by decoded byte length while its complete base64 representation is retained', () => {
  const body = btoa('x'.repeat(SAVED_SEGMENT_LIMIT - 4) + 'TAIL')
  const binary = { ...page, body, encoding: 'base64', total_bytes: String(SAVED_SEGMENT_LIMIT) }
  expect(body.length).toBeGreaterThan(SAVED_SEGMENT_LIMIT)
  expect(parseSavedSegment({ success: true, data: binary }, 'saved-1', { part: 'response', offset: '0' }).body).toBe(body)
})

test('response entry survives an older missing truncation flag when saved total exceeds loaded bytes', () => {
  const detail = { request_body: '', response_body: 'HEAD', response_encoding: 'utf-8', response_body_total_bytes: 20 } as FullContentLogDetail
  expect(hasSavedSegments(detail, 'response')).toBe(true)
  expect(hasSavedSegments({ ...detail, response_body_total_bytes: 4 }, 'response')).toBe(false)
  expect(hasSavedSegments({ ...detail, request_body: 'x'.repeat(SAVED_SEGMENT_LIMIT + 1) }, 'request')).toBe(true)
})
