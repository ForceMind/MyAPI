import { describe, expect, it } from 'vitest'

import { channelSchema } from '../../types'
import {
  buildSettingJSON,
  channelFormSchema,
  transformChannelToFormDefaults,
} from '../channel-form'
import { isValidInputTokenLimits } from '../input-token-limits'

describe('channel estimated input policy', () => {
  it('round trips exact model limits through existing channel settings', () => {
    const channel = channelSchema.parse({
      id: 1,
      type: 57,
      name: 'Synthetic',
      key: '',
      status: 1,
      created_time: 0,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
      setting: '{"input_token_limits":{"gpt-6.1-sol":920000}}',
    })
    const values = transformChannelToFormDefaults(channel)
    expect(JSON.parse(values.input_token_limits ?? '')).toEqual({
      'gpt-6.1-sol': 920000,
    })
    expect(JSON.parse(buildSettingJSON(values)).input_token_limits).toEqual({
      'gpt-6.1-sol': 920000,
    })
  })
  it.each([
    '[]',
    'null',
    '{"":1}',
    '{" model":1}',
    '{"model":0}',
    '{"model":-1}',
    '{"model":1.2}',
    '{"model":2147483648}',
    '{"model":"100"}',
    '{bad',
  ])('rejects invalid input limits %s before save', (value) => {
    expect(isValidInputTokenLimits(value)).toBe(false)
    const result = channelFormSchema.safeParse({
      name: 'Synthetic',
      type: 1,
      key: 'synthetic-test-key',
      models: ['gpt-6.1-sol'],
      group: ['default'],
      input_token_limits: value,
    })
    expect(result.success).toBe(false)
    if (!result.success) {
      expect(
        result.error.issues.some((e) => e.path.includes('input_token_limits'))
      ).toBe(true)
    }
  })
  it('empty settings do not enable a new policy on existing channels', () => {
    expect(isValidInputTokenLimits('')).toBe(true)
    expect(
      JSON.parse(
        buildSettingJSON({ input_token_limits: '' } as Parameters<
          typeof buildSettingJSON
        >[0])
      )
    ).not.toHaveProperty('input_token_limits')
  })
})
