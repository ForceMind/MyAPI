import { expect, it } from 'vitest'
import { readWalletAccount } from '../wallet-account'

it('keeps each known amount and zero while independently marking malformed or missing fields unknown', () => {
  expect(readWalletAccount({ id: 7, username: 'owner', group: 'default', quota: -10, used_quota: null, request_count: 0, aff_quota: 25, aff_history_quota: Number.NaN }, 7)).toEqual({ id: 7, username: 'owner', group: 'default', quota: -10, used_quota: undefined, request_count: 0, aff_quota: 25, aff_history_quota: undefined, aff_count: undefined })
})
it.each([undefined, null, { id: 8, quota: 12 }, { id: '7', quota: 12 }])('rejects a snapshot %j that cannot prove the requested actor identity', (value) => {
  expect(readWalletAccount(value, 7)).toBeNull()
})
