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

import { DEFAULT_CONFIG, DEFAULT_PARAMETER_ENABLED } from '../../../constants'
import type { Message, PlaygroundKey } from '../../../types'
import { buildChatCompletionPayload } from '../payload-builder'

const strictKey: PlaygroundKey & { strict_token_budget: boolean } = {
  id: 17,
  name: 'Strict text key',
  status: 1,
  group: 'default',
  remain_quota: 100,
  unlimited_quota: false,
  expired_time: -1,
  strict_token_budget: true,
}
const messages: Message[] = [
  { key: 'user', from: 'user', versions: [{ id: 'text', content: 'Hello' }] },
]
const config = { ...DEFAULT_CONFIG, keyId: 17, model: 'gpt-6.1-sol' }

describe('strict selected-key Chat payload', () => {
  test.each([false, true])(
    'builds qualified text fields with default controls and stream=%s',
    (stream) => {
      const payload = buildChatCompletionPayload(
        messages,
        { ...config, stream },
        DEFAULT_PARAMETER_ENABLED,
        strictKey
      )

      expect(payload).toEqual({
        model: 'gpt-6.1-sol',
        messages: [{ role: 'user', content: 'Hello' }],
        stream,
        max_completion_tokens: 4096,
        service_tier: 'default',
        ...(stream ? { stream_options: { include_usage: true } } : {}),
      })
    }
  )

  test.each([
    [1, 1],
    [128000, 128000],
    [200000, 128000],
    [0, 1],
    [-2, 1],
    [50.8, 50],
    [Number.NaN, 4096],
    [Infinity, 4096],
  ])(
    'normalizes max tokens %s to the qualified output bound %s',
    (value, expected) => {
      const payload = buildChatCompletionPayload(
        messages,
        { ...config, max_tokens: value },
        { ...DEFAULT_PARAMETER_ENABLED, max_tokens: true },
        strictKey
      )

      expect(payload).toHaveProperty('max_completion_tokens', expected)
      expect(payload).not.toHaveProperty('max_tokens')
    }
  )

  test.each([
    ['ordinary key', { ...strictKey, strict_token_budget: false }, config],
    [
      'old metadata without a strict flag',
      { ...strictKey, strict_token_budget: undefined },
      config,
    ],
    ['stale selected-key metadata', { ...strictKey, id: 18 }, config],
    ['unqualified model', strictKey, { ...config, model: 'gpt-6.1-sol-alias' }],
  ])(
    'preserves the existing user parameters for %s',
    (_name, key, requestConfig) => {
      const payload = buildChatCompletionPayload(
        messages,
        { ...requestConfig, seed: 42 },
        { ...DEFAULT_PARAMETER_ENABLED, max_tokens: true, seed: true },
        key
      )

      expect(payload).toEqual({
        model: requestConfig.model,
        messages: [{ role: 'user', content: 'Hello' }],
        stream: true,
        temperature: 0.7,
        top_p: 1,
        max_tokens: 4096,
        frequency_penalty: 0,
        presence_penalty: 0,
        seed: 42,
      })
    }
  )

  test('preserves an attached image for server rejection without stripping or downgrading the key', () => {
    const payload = buildChatCompletionPayload(
      [
        {
          ...messages[0],
          attachments: [
            {
              id: 'image',
              name: 'test.png',
              mimeType: 'image/png',
              size: 1,
              dataUrl: 'data:image/png;base64,AA==',
            },
          ],
        },
      ],
      config,
      DEFAULT_PARAMETER_ENABLED,
      strictKey
    )

    expect(payload.messages[0].content).toEqual([
      { type: 'text', text: 'Hello' },
      { type: 'image_url', image_url: { url: 'data:image/png;base64,AA==' } },
    ])
    expect(payload.model).toBe('gpt-6.1-sol')
  })
})
