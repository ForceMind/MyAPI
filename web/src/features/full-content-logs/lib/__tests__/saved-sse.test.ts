/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { describe, expect, test } from 'vitest'

import { CONTENT_PREVIEW_LIMIT } from '../content-preview'
import { parseSavedSse } from '../saved-sse'

describe('saved SSE evidence', () => {
  test('CRLF frames preserve multiline data and distinguish event fields from payload types', () => {
    const result = parseSavedSse({
      body: 'id: source-7\r\nevent: progress\r\nretry: 3000\r\ndata: first\r\ndata:  second\r\n\r\ndata: {"type":"response.completed"}\r\n\r\n',
    })

    expect(result.headerDeclaresSse).toBe(false)
    expect(result.hasSseFields).toBe(true)
    expect(result.events[0]).toMatchObject({
      index: 1, type: 'progress', typeSource: 'event',
      id: 'source-7', retry: '3000', data: 'first\n second', complete: true,
      markers: [],
    })
    expect(result.events[1]).toMatchObject({
      index: 2, type: 'response.completed', typeSource: 'payload',
      markers: [{ kind: 'completion', source: 'data.type: response.completed' }],
    })
  })

  test('an initial BOM leaves the first saved SSE event available', () => {
    const body = '\uFEFFdata: [DONE]\n\n'
    const result = parseSavedSse({ body })

    expect(result.events).toHaveLength(1)
    expect(result.events[0].markers).toEqual([{ kind: 'completion', source: 'data: [DONE]' }])
  })

  test.each([
    [' TEXT/EVENT-STREAM ; charset=utf-8', true],
    ['application/json; note="text/event-stream"', false],
  ] as const)('content type %s confirms streaming only through its MIME essence', (contentType, expected) => {
    expect(parseSavedSse({ body: '', contentType }).headerDeclaresSse).toBe(expected)
  })

  test('a field-only frame is not dispatched while an empty data field remains a saved event', () => {
    const result = parseSavedSse({ body: 'event: ping\n\n: comment\n\ndata:\n\n' })

    expect(result.hasSseFields).toBe(true)
    expect(result.events).toHaveLength(1)
    expect(result.events[0]).toMatchObject({ index: 1, type: 'message', data: '', complete: true })
  })

  test('an error saved after a completion marker remains separate protocol evidence', () => {
    const result = parseSavedSse({
      body: 'data: [DONE]\n\nevent: error\ndata: {"error":{"message":"late failure"}}\n\n',
      contentType: 'text/event-stream',
      truncated: true,
    })

    expect(result.sourceTruncated).toBe(true)
    expect(result.events[0].markers).toEqual([{ kind: 'completion', source: 'data: [DONE]' }])
    expect(result.events[1].markers).toEqual([
      { kind: 'error', source: 'event: error' },
      { kind: 'error', source: 'data.error' },
    ])
  })

  test('an unterminated saved frame never confirms its apparent completion marker', () => {
    const result = parseSavedSse({ body: 'event: response.completed\ndata: [DONE]' })

    expect(result.incompleteTail).toBe(true)
    expect(result.events[0].complete).toBe(false)
    expect(result.events[0].markers).toEqual([])
  })

  test('binary data and assistant text do not create SSE events or error markers', () => {
    expect(parseSavedSse({ body: 'data: [DONE]\n\n', encoding: 'base64' }).events).toEqual([])
    expect(parseSavedSse({ body: '{"text":"an error occurred"}' }).hasSseFields).toBe(false)
    expect(parseSavedSse({ body: 'data: {"error":false}\n\n' }).events[0].markers).toEqual([])
  })

  test('a UTF-8 preview cut keeps its final frame incomplete and never confirms a following marker', () => {
    const head = 'event: response.completed\ndata: '
    const body = head + '中'.repeat(Math.floor((CONTENT_PREVIEW_LIMIT - head.length) / 3)) + '😀\n\ndata: [DONE]\n\n'
    const result = parseSavedSse({ body })

    expect(result.previewLimited).toBe(true)
    expect(result.incompleteTail).toBe(true)
    expect(result.events).toHaveLength(1)
    expect(result.events[0].complete).toBe(false)
    expect(result.events[0].data).not.toContain('😀')
    expect(result.events[0].markers).toEqual([])
  })

  test('parsing includes every event inside the preview and excludes markers beyond its explicit boundary', () => {
    const frame = 'data: {"text":"saved"}\n\n'
    const padded = frame + ':' + ' '.repeat(CONTENT_PREVIEW_LIMIT - frame.length - 1)
    const result = parseSavedSse({ body: padded + '\n\ndata: [DONE]\n\n' })

    expect(result.previewLimited).toBe(true)
    expect(result.events).toHaveLength(1)
    expect(result.events[0].markers).toEqual([])
    const manyEvents = Array.from({ length: 501 }, (_, index) => `data: {"text":"event-${index + 1}"}\n\n`).join('')
    expect(parseSavedSse({ body: manyEvents }).events[500]).toMatchObject({
      index: 501, data: '{"text":"event-501"}', complete: true,
    })
  })
})
