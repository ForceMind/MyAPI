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
  CHANNEL_TYPE_NEW_API,
  CHANNEL_TYPE_OPTIONS,
  CHANNEL_TYPE_WARNINGS,
  MODEL_FETCHABLE_TYPES,
  TYPE_TO_KEY_PROMPT,
} from '../../constants'
import { CHANNEL_FORM_DEFAULT_VALUES, channelFormSchema } from '../channel-form'
import { getChannelTypeConfig } from '../channel-type-config'
import { getChannelTypeIcon, getKeyPromptForType } from '../channel-utils'

function myAPIForm(baseUrl: string) {
  return {
    ...CHANNEL_FORM_DEFAULT_VALUES,
    name: 'MyAPI upstream',
    type: CHANNEL_TYPE_NEW_API,
    base_url: baseUrl,
    key: 'test-key',
    models: 'gpt-5',
  }
}

describe('MyAPI channel', () => {
  test('registers selection, ordering, model discovery, and icon metadata', () => {
    const option = CHANNEL_TYPE_OPTIONS.find(
      (item) => item.value === CHANNEL_TYPE_NEW_API
    )

    expect(option).toEqual({
      value: CHANNEL_TYPE_NEW_API,
      label: 'MyAPI',
    })
    expect(
      CHANNEL_TYPE_OPTIONS.findIndex(
        (item) => item.value === CHANNEL_TYPE_NEW_API
      ) + 1
    ).toBe(CHANNEL_TYPE_OPTIONS.findIndex((item) => item.value === 58))
    expect(MODEL_FETCHABLE_TYPES.has(CHANNEL_TYPE_NEW_API)).toBe(true)
    expect(getChannelTypeIcon(CHANNEL_TYPE_NEW_API)).toBe('MyAPI')
    expect(getKeyPromptForType(CHANNEL_TYPE_NEW_API)).toBe(
      'Enter API key for this channel'
    )
    expect(getChannelTypeConfig(CHANNEL_TYPE_NEW_API).icon).toBe('MyAPI')

    // Advanced Custom remains type 58, but should not display a legacy
    // upstream product icon; it is a neutral compatibility route.
    expect(getChannelTypeIcon(58)).toBe('Custom')
    expect(getChannelTypeConfig(58).icon).toBe('Custom')

    expect(TYPE_TO_KEY_PROMPT[50]).toBe(
      'Format: AccessKey|SecretKey (or just ApiKey for compatible upstream relays)'
    )
    expect(CHANNEL_TYPE_WARNINGS[8]).toBe(
      'If connecting to compatible upstream relay projects, use OpenAI type unless you know what you are doing'
    )
  })

  test('puts Codex first and explains provider support boundaries', () => {
    expect(CHANNEL_TYPE_OPTIONS[0]).toEqual({
      value: 57,
      label: 'ChatGPT Subscription (Codex)',
    })

    expect(getChannelTypeConfig(57).capabilityNote).toContain(
      'Codex OAuth channel'
    )
    expect(getChannelTypeConfig(14).capabilityNote).toContain(
      'subscription account login'
    )
    expect(getChannelTypeConfig(24).capabilityNote).toContain(
      'Google Antigravity'
    )
  })

  test('requires a non-blank Base URL', () => {
    const blankResult = channelFormSchema.safeParse(myAPIForm('  '))

    expect(blankResult.success).toBe(false)
    if (!blankResult.success) {
      expect(
        blankResult.error.issues.some(
          (issue) =>
            issue.path[0] === 'base_url' &&
            issue.message === 'Base URL is required for this channel type'
        )
      ).toBe(true)
    }

    expect(
      channelFormSchema.safeParse(myAPIForm('https://my-api.example')).success
    ).toBe(true)
  })

  test('keeps Sub2API Base URL validation unchanged', () => {
    const result = channelFormSchema.safeParse({
      ...myAPIForm(''),
      type: 59,
    })

    expect(result.success).toBe(true)
  })
})
