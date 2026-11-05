import { expect, test } from 'vitest'

import { apiKeySchema } from '../../types'
import { keyClientAddresses, keyClientModels } from '../client-configuration'

const key = apiKeySchema.parse({
  id: 21, name: 'Synthetic Key', key: 'masked', status: 1,
  remain_quota: 10, used_quota: 0, unlimited_quota: false,
  expired_time: -1, created_time: 1, accessed_time: 0,
  model_limits_enabled: false,
})

test.each([
  [' https://gateway.invalid/root/v1/// ', 'https://gateway.invalid/root'],
  ['https://gateway.invalid/root///', 'https://gateway.invalid/root'],
  ['', 'https://current.invalid'],
])('address %s produces one version suffix and preserves its installation path', (address, root) => {
  expect(keyClientAddresses(address, 'https://current.invalid')).toEqual({
    root, openAI: `${root}/v1`, source: address.trim() ? 'setting' : 'current',
  })
})

test('a restricted Key intersects the catalog exactly without guessing model protocols', () => {
  expect(keyClientModels(['claude-custom', 'gpt-custom', 'claude-custom'], {
    ...key, model_limits_enabled: true, model_limits: 'gpt-custom,not-in-catalog',
  })).toEqual(['gpt-custom'])
})

test.each(['', null])('an enabled empty or null Key restriction %s offers no candidate models', (modelLimits) => {
  expect(keyClientModels(['gpt-custom'], { ...key, model_limits_enabled: true, model_limits: modelLimits })).toEqual([])
})

test('candidate models respect an explicit profile list and never invent absent catalog entries', () => {
  expect(keyClientModels(['first', 'second'], {
    ...key, access_profile: { id: 'custom', model_allowlist: ['second', 'absent'] },
  })).toEqual(['second'])
  expect(keyClientModels(['first'], {
    ...key, access_profile: { id: 'custom', model_allowlist: [] },
  })).toEqual([])
})
