import { describe, expect, test } from 'vitest'

import { getAccountTierId } from './account-tier'
import { transformFormDataToPayload, transformUserToFormDefaults } from './user-form'

describe('account tier form identity', () => {
  test('maps legacy groups to stable account tier IDs', () => {
    expect(getAccountTierId('default')).toBe('standard')
    expect(getAccountTierId('vip')).toBe('priority')
    expect(getAccountTierId('team-enterprise')).toBe('team-enterprise')
  })

  test('preserves an explicit account tier ID when editing a user', () => {
    expect(getAccountTierId('vip', 'team-enterprise')).toBe('team-enterprise')
    const defaults = transformUserToFormDefaults({
      id: 1,
      username: 'alice',
      display_name: 'Alice',
      quota: 0,
      used_quota: 0,
      request_count: 0,
      group: 'vip',
      account_tier_id: 'team-enterprise',
      status: 1,
      role: 1,
    })
    expect(defaults.account_tier_id).toBe('team-enterprise')
    expect(transformFormDataToPayload(defaults, 1).account_tier_id).toBe('team-enterprise')
  })
})
