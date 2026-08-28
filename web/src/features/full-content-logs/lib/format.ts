/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

export function formatLogBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  const unitIndex = Math.min(
    units.length - 1,
    Math.floor(Math.log(bytes) / Math.log(1024))
  )
  const value = bytes / 1024 ** unitIndex
  return `${value.toFixed(unitIndex === 0 ? 0 : 2)} ${units[unitIndex]}`
}

export function formatLogBody(
  body: string,
  encoding?: string,
  contentType?: string
): string {
  if (!body) return ''
  if (encoding === 'base64') return body
  if (encoding !== 'json' && !contentType?.toLowerCase().includes('json')) {
    return body
  }
  try {
    return JSON.stringify(JSON.parse(body) as unknown, null, 2)
  } catch {
    return body
  }
}

type LogJsonObject = Record<string, unknown>

function isLogJsonObject(value: unknown): value is LogJsonObject {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function parseLogJson(body: string, encoding?: string): unknown | undefined {
  if (!body || encoding === 'base64') return undefined
  try {
    return JSON.parse(body) as unknown
  } catch {
    return undefined
  }
}

function logContentText(value: unknown): string[] {
  if (typeof value === 'string') return value.trim() ? [value] : []
  if (Array.isArray(value)) return value.flatMap(logContentText)
  if (!isLogJsonObject(value)) return []

  for (const key of ['text', 'input_text', 'output_text'] as const) {
    const candidate = value[key]
    if (typeof candidate === 'string' && candidate.trim()) return [candidate]
    if (isLogJsonObject(candidate)) {
      const nested = candidate.value
      if (typeof nested === 'string' && nested.trim()) return [nested]
    }
  }

  if (value.content !== undefined) return logContentText(value.content)
  if (value.parts !== undefined) return logContentText(value.parts)
  return []
}

function roleSections(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  const sections: string[] = []
  for (const item of value) {
    if (!isLogJsonObject(item)) {
      sections.push(...logContentText(item))
      continue
    }
    const text = logContentText(item.content ?? item.parts ?? item.text)
      .join('\n')
      .trim()
    if (!text) continue
    const role = typeof item.role === 'string' ? item.role.toUpperCase() : ''
    sections.push(role ? `${role}\n${text}` : text)
  }
  return sections
}

export function extractLogRequestText(
  body: string,
  encoding?: string,
  contentType?: string
): string {
  const payload = parseLogJson(body, encoding)
  if (!isLogJsonObject(payload)) {
    if (encoding === 'base64') return ''
    return formatLogBody(body, encoding, contentType).trim()
  }

  const sections: string[] = []
  const appendSection = (label: string, value: unknown) => {
    const text = logContentText(value).join('\n').trim()
    if (text) sections.push(`${label}\n${text}`)
  }

  appendSection('INSTRUCTIONS', payload.instructions)
  appendSection('SYSTEM', payload.system)
  sections.push(...roleSections(payload.messages))

  if (typeof payload.input === 'string') {
    appendSection('USER', payload.input)
  } else {
    const inputSections = roleSections(payload.input)
    if (inputSections.length > 0) sections.push(...inputSections)
    else appendSection('USER', payload.input)
  }

  sections.push(...roleSections(payload.contents))
  appendSection('PROMPT', payload.prompt)
  if (sections.length > 0) return sections.join('\n\n')

  return logContentText(payload).join('\n').trim()
}

function responsePayloadText(value: unknown): string[] {
  if (!isLogJsonObject(value)) return logContentText(value)

  if (typeof value.output_text === 'string' && value.output_text.trim()) {
    return [value.output_text]
  }

  const pieces: string[] = []
  if (Array.isArray(value.choices)) {
    for (const choice of value.choices) {
      if (!isLogJsonObject(choice)) continue
      pieces.push(
        ...logContentText(choice.message ?? choice.delta ?? choice.text)
      )
    }
  }
  pieces.push(...logContentText(value.output))
  pieces.push(...logContentText(value.content))

  if (Array.isArray(value.candidates)) {
    for (const candidate of value.candidates) {
      if (!isLogJsonObject(candidate)) continue
      pieces.push(...logContentText(candidate.content))
    }
  }
  if (pieces.length === 0 && value.response !== undefined) {
    pieces.push(...responsePayloadText(value.response))
  }
  return pieces
}

function responseDeltaText(value: unknown): string[] {
  if (!isLogJsonObject(value)) return []
  const pieces: string[] = []
  const eventType = typeof value.type === 'string' ? value.type : ''

  if (
    eventType.includes('output_text.delta') &&
    typeof value.delta === 'string'
  ) {
    pieces.push(value.delta)
  }
  if (
    eventType.includes('content_block_delta') &&
    isLogJsonObject(value.delta)
  ) {
    const text = value.delta.text
    if (typeof text === 'string') pieces.push(text)
  }
  if (Array.isArray(value.choices)) {
    for (const choice of value.choices) {
      if (!isLogJsonObject(choice) || !isLogJsonObject(choice.delta)) continue
      pieces.push(...logContentText(choice.delta.content))
    }
  }
  if (Array.isArray(value.candidates)) {
    for (const candidate of value.candidates) {
      if (!isLogJsonObject(candidate)) continue
      pieces.push(...logContentText(candidate.content))
    }
  }
  return pieces
}

export function extractLogResponseText(
  body: string,
  encoding?: string,
  contentType?: string
): string {
  if (!body || encoding === 'base64') return ''
  const isEventStream =
    contentType?.toLowerCase().includes('text/event-stream') ||
    /(^|\n)data:\s/.test(body)

  if (isEventStream) {
    const deltas: string[] = []
    const completed: string[] = []
    for (const line of body.split(/\r?\n/)) {
      if (!line.startsWith('data:')) continue
      const data = line.slice(5).trimStart()
      if (!data || data.trim() === '[DONE]') continue
      const payload = parseLogJson(data)
      if (payload === undefined) continue
      deltas.push(...responseDeltaText(payload))
      const fullText = responsePayloadText(payload).join('')
      if (fullText.trim()) completed.push(fullText)
    }
    if (deltas.length > 0) return deltas.join('').trim()
    if (completed.length > 0) return completed.at(-1)?.trim() ?? ''
  }

  const payload = parseLogJson(body, encoding)
  if (payload !== undefined) {
    const text = responsePayloadText(payload).join('\n').trim()
    if (text) return text
  }
  return formatLogBody(body, encoding, contentType).trim()
}
