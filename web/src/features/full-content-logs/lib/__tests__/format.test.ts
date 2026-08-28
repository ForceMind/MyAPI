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

import { describe, expect, test } from 'vitest'

import {
  extractLogRequestText,
  extractLogResponseText,
  formatLogBody,
  formatLogBytes,
} from '../format'

describe('full content log formatting', () => {
  test('formats JSON request bodies without changing their values', () => {
    expect(
      formatLogBody('{"model":"gpt-5.6-luna","stream":true}', 'json')
    ).toBe('{\n  "model": "gpt-5.6-luna",\n  "stream": true\n}')
  })

  test('keeps base64 attachment bodies unchanged', () => {
    expect(formatLogBody('/wAB', 'base64', 'application/octet-stream')).toBe(
      '/wAB'
    )
  })

  test('formats byte totals with binary units', () => {
    expect(formatLogBytes(0)).toBe('0 B')
    expect(formatLogBytes(1536)).toBe('1.50 KiB')
  })

  test('extracts only instructions and conversational text from a Responses request', () => {
    const body = JSON.stringify({
      model: 'gpt-5.6-terra',
      instructions: 'Answer briefly.',
      input: [
        {
          role: 'user',
          content: [
            { type: 'input_text', text: 'Describe this image.' },
            { type: 'input_image', image_url: 'data:image/png;base64,secret' },
          ],
        },
      ],
      stream: true,
      store: false,
    })

    expect(extractLogRequestText(body, 'json')).toBe(
      'INSTRUCTIONS\nAnswer briefly.\n\nUSER\nDescribe this image.'
    )
  })

  test('extracts message text from Chat Completions without protocol fields', () => {
    const body = JSON.stringify({
      model: 'gpt-5.6-luna',
      messages: [
        { role: 'system', content: 'You are helpful.' },
        {
          role: 'user',
          content: [{ type: 'text', text: 'Hello' }],
        },
      ],
      temperature: 0.3,
    })

    expect(extractLogRequestText(body, 'json')).toBe(
      'SYSTEM\nYou are helpful.\n\nUSER\nHello'
    )
  })

  test('joins Responses SSE deltas and ignores completed event duplication', () => {
    const body = [
      'event: response.output_text.delta',
      'data: {"type":"response.output_text.delta","delta":"Hello"}',
      '',
      'event: response.output_text.delta',
      'data: {"type":"response.output_text.delta","delta":" world"}',
      '',
      'event: response.completed',
      'data: {"type":"response.completed","response":{"output_text":"Hello world"}}',
      '',
      'data: [DONE]',
    ].join('\n')

    expect(extractLogResponseText(body, 'utf-8', 'text/event-stream')).toBe(
      'Hello world'
    )
  })

  test('joins Chat Completions SSE content deltas', () => {
    const body = [
      'data: {"choices":[{"delta":{"content":"A"}}]}',
      '',
      'data: {"choices":[{"delta":{"content":" B"}}]}',
      '',
      'data: [DONE]',
    ].join('\n')

    expect(extractLogResponseText(body, 'utf-8', 'text/event-stream')).toBe(
      'A B'
    )
  })

  test('extracts text from a non-streaming response envelope', () => {
    const body = JSON.stringify({
      id: 'resp_123',
      output: [
        {
          type: 'message',
          content: [{ type: 'output_text', text: 'Finished.' }],
        },
      ],
      usage: { input_tokens: 10, output_tokens: 2 },
    })

    expect(extractLogResponseText(body, 'json', 'application/json')).toBe(
      'Finished.'
    )
  })
})
