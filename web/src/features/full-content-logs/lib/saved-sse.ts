/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { previewContent } from './content-preview'

export interface SavedSseMarker {
  kind: 'completion' | 'error'
  /** The exact saved protocol field that supplied this observation. */
  source: string
}

export interface SavedSseEvent {
  index: number
  type: string
  typeSource: 'event' | 'payload' | 'default'
  id?: string
  retry?: string
  data: string
  raw: string
  complete: boolean
  markers: SavedSseMarker[]
}

/**
 * Parse only the bounded saved preview. Markers describe complete saved SSE
 * frames; they never establish request success, delivery, usage or settlement.
 * No timestamp is derived from ordering, payload text or chunk_count.
 */
export function parseSavedSse(input: {
  body: string
  encoding?: string
  contentType?: string
  truncated?: boolean
}) {
  const mime = input.contentType?.split(';', 1)[0].trim().toLowerCase()
  const headerDeclaresSse = mime === 'text/event-stream'
  const bounded = previewContent(input.body)
  const previewLimited = bounded.limited
  const events: SavedSseEvent[] = []
  let hasSseFields = false
  let incompleteTail = false
  if (input.encoding === 'base64') {
    return {
      events, headerDeclaresSse, previewLimited, hasSseFields,
      incompleteTail, sourceTruncated: input.truncated === true,
    }
  }

  const text = bounded.preview.startsWith('\uFEFF') ? bounded.preview.slice(1) : bounded.preview
  const preview = text.replaceAll(/\r\n?/g, '\n')
  const blocks = preview.split('\n\n')
  for (let blockIndex = 0; blockIndex < blocks.length; blockIndex += 1) {
    const raw = blocks[blockIndex]
    let eventType: string | undefined
    let id: string | undefined
    let retry: string | undefined
    const dataLines: string[] = []
    for (const line of raw.split('\n')) {
      if (line.startsWith(':')) continue
      const colon = line.indexOf(':')
      const field = colon === -1 ? line : line.slice(0, colon)
      let value = colon === -1 ? '' : line.slice(colon + 1)
      // SSE removes one optional space, rather than trimming payload data.
      if (value.startsWith(' ')) value = value.slice(1)
      switch (field) {
        case 'data':
          hasSseFields = true
          dataLines.push(value)
          break
        case 'event':
          hasSseFields = true
          eventType = value
          break
        case 'id':
          hasSseFields = true
          if (!value.includes('\0')) id = value
          break
        case 'retry':
          hasSseFields = true
          retry = value
          break
      }
    }
    if (dataLines.length === 0) continue
    const complete = blockIndex < blocks.length - 1
    if (!complete) incompleteTail = true
    const data = dataLines.join('\n')
    let payload: Record<string, unknown> | undefined
    try {
      const parsed: unknown = JSON.parse(data)
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        payload = parsed as Record<string, unknown>
      }
    } catch {
      // Plain text, [DONE] and partial JSON remain readable as saved data.
    }
    const payloadType = typeof payload?.type === 'string' ? payload.type : undefined
    const markers: SavedSseMarker[] = []
    if (complete) {
      if (data === '[DONE]') {
        markers.push({ kind: 'completion', source: 'data: [DONE]' })
      }
      for (const [field, value] of [['event', eventType], ['data.type', payloadType]] as const) {
        if (value === 'response.completed' || value === 'message_stop') {
          markers.push({ kind: 'completion', source: `${field}: ${value}` })
        }
        if (value === 'error' || value === 'response.failed') {
          markers.push({ kind: 'error', source: `${field}: ${value}` })
        }
      }
      const error = payload?.error
      if ((typeof error === 'string' && error.trim()) ||
        (error && typeof error === 'object' && !Array.isArray(error))) {
        markers.push({ kind: 'error', source: 'data.error' })
      }
    }
    let typeSource: SavedSseEvent['typeSource'] = 'default'
    if (eventType) typeSource = 'event'
    else if (payloadType) typeSource = 'payload'
    events.push({
      index: events.length + 1,
      type: eventType || payloadType || 'message',
      typeSource,
      id,
      retry,
      data,
      raw,
      complete,
      markers,
    })
  }
  return {
    events, headerDeclaresSse, hasSseFields, previewLimited,
    incompleteTail, sourceTruncated: input.truncated === true,
  }
}
