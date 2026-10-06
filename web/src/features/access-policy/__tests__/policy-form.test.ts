import { expect, test } from 'vitest'

import {
  policyCandidate,
  policyFormDefaults,
  policyFormSchema,
} from '../lib/policy-form'
import { userPolicy } from './fixtures'

test('a new assignment defaults to inherited dimensions without converting explicit empty restrictions', () => {
  const values = policyFormDefaults({
    ...userPolicy,
    public_models: [],
    upstream_models: ['upstream-v1'],
  })
  expect(policyCandidate(values)).toEqual({
    enabled: true,
    public_models: [],
    upstream_models: ['upstream-v1'],
    channel_ids: null,
  })
})

test('disabled stored assignments remain disabled and exact scope lists deduplicate', () => {
  const values = policyFormDefaults({
    ...userPolicy,
    assigned: true,
    enabled: false,
  })
  values.public = {
    inherit: false,
    text: 'model-public\nmodel-public, model-2',
  }
  values.channels = { inherit: false, text: '2, 3\n2' }
  expect(policyCandidate(values)).toEqual({
    enabled: false,
    public_models: ['model-public', 'model-2'],
    upstream_models: null,
    channel_ids: [2, 3],
  })
})

test.each(['0', '-1', '1.5', '1e2', '9007199254740992', 'not-an-id'])(
  'invalid channel ID %s fails validation',
  (text) => {
    const values = policyFormDefaults(userPolicy)
    values.channels = { inherit: false, text }
    expect(policyFormSchema.safeParse(values).success).toBe(false)
  }
)

test('inherited channels ignore an inactive malformed draft while an empty restricted list is valid denial', () => {
  const values = policyFormDefaults(userPolicy)
  values.channels.text = 'invalid'
  expect(policyFormSchema.safeParse(values).success).toBe(true)
  expect(policyCandidate(values).channel_ids).toBeNull()
  values.channels = { inherit: false, text: '' }
  expect(policyFormSchema.safeParse(values).success).toBe(true)
  expect(policyCandidate(values).channel_ids).toEqual([])
})

test.each(['public', 'upstream', 'channels'] as const)(
  'more than 128 distinct values in %s fail validation before a request',
  (dimension) => {
    const values = policyFormDefaults(userPolicy)
    values[dimension] = {
      inherit: false,
      text: Array.from({ length: 129 }, (_, index) => `${index + 1}`).join(
        '\n'
      ),
    }
    expect(policyFormSchema.safeParse(values).success).toBe(false)
  }
)

test.each(['a'.repeat(256), '模'.repeat(86), 'model\u0001', 'model\u200b'])(
  'oversized or unsafe model name fails validation',
  (text) => {
    const values = policyFormDefaults(userPolicy)
    values.public = { inherit: false, text }
    expect(policyFormSchema.safeParse(values).success).toBe(false)
  }
)

test('a model at the exact UTF-8 byte limit remains valid', () => {
  const values = policyFormDefaults(userPolicy)
  values.public = { inherit: false, text: '模'.repeat(85) }
  expect(policyFormSchema.safeParse(values).success).toBe(true)
})
