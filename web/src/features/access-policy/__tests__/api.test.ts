import { AxiosError } from 'axios'
import { beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import {
  accessPolicyErrorKind,
  getAssignedPolicy,
  getOwnerAccess,
  previewAssignedPolicy,
  removeAssignedPolicy,
  saveAssignedPolicy,
} from '../api'
import { envelope, tokenPolicy, userPolicy } from './fixtures'

vi.mock('@/lib/api', () => ({
  api: { get: vi.fn(), put: vi.fn(), post: vi.fn(), delete: vi.fn() },
}))
beforeEach(() => vi.resetAllMocks())
const target = { subject: 'user' as const, id: 2, ownerUserId: 2 }
const candidate = {
  enabled: true,
  public_models: null,
  upstream_models: [],
  channel_ids: [1],
}

test('null and missing dimensions inherit while explicit empty dimensions remain denied', async () => {
  vi.mocked(api.get).mockResolvedValue(
    envelope({ ...userPolicy, public_models: undefined, upstream_models: [] })
  )
  const data = await getAssignedPolicy(target)
  expect(data.public_models).toBeNull()
  expect(data.upstream_models).toEqual([])
})

test.each([
  { subject: 'token', subject_id: 2, owner_user_id: 2 },
  { subject: 'user', subject_id: 3, owner_user_id: 2 },
  { subject: 'user', subject_id: 2, owner_user_id: 3 },
])(
  'a mismatched policy identity rejects before exposing fields: %j',
  async (identity) => {
    vi.mocked(api.get).mockResolvedValue(
      envelope({ ...userPolicy, ...identity })
    )
    await expect(getAssignedPolicy(target)).rejects.toThrow('identity_mismatch')
  }
)

test('save sends all scope dimensions and preserves the tombstone revision', async () => {
  vi.mocked(api.put).mockResolvedValue(
    envelope({ ...userPolicy, assigned: true, revision: 6, ...candidate })
  )
  await saveAssignedPolicy(target, { ...candidate, expected_revision: 5 })
  expect(api.put).toHaveBeenCalledWith(
    '/api/access-policy/user/2',
    { ...candidate, expected_revision: 5 },
    expect.objectContaining({ skipBusinessError: true })
  )
})

test('removal sends the expected revision in the DELETE body and accepts a revision tombstone', async () => {
  vi.mocked(api.delete).mockResolvedValue(
    envelope({ ...userPolicy, revision: 8 })
  )
  expect((await removeAssignedPolicy(target, 7)).revision).toBe(8)
  expect(api.delete).toHaveBeenCalledWith(
    '/api/access-policy/user/2',
    expect.objectContaining({ data: { expected_revision: 7 } })
  )
})

test('preview posts only a candidate to the read-only endpoint and never writes a policy', async () => {
  vi.mocked(api.post).mockResolvedValue(
    envelope({ policy: { ...userPolicy, ...candidate }, models: [] })
  )
  await previewAssignedPolicy(target, candidate)
  expect(api.post).toHaveBeenCalledWith(
    '/api/access-policy/user/2/preview',
    candidate,
    expect.any(Object)
  )
  expect(api.put).not.toHaveBeenCalled()
  expect(api.delete).not.toHaveBeenCalled()
})

test('owner response parsing strips private inventories and verifies the token identity', async () => {
  vi.mocked(api.get).mockResolvedValue(
    envelope({
      token_id: 7,
      assigned: true,
      user_revision: 2,
      token_revision: 3,
      models: [],
      channel_ids: [999],
      upstream_models: ['private-upstream'],
    })
  )
  expect(await getOwnerAccess(7)).toEqual({
    token_id: 7,
    assigned: true,
    user_revision: 2,
    token_revision: 3,
    models: [],
  })
  await expect(getOwnerAccess(8)).rejects.toThrow('identity_mismatch')
})

test('malformed success and error envelopes never become editable policy data', async () => {
  vi.mocked(api.get)
    .mockResolvedValueOnce({
      data: { success: false, code: 'forbidden', message: 'private-data' },
    })
    .mockResolvedValueOnce(envelope({ ...tokenPolicy, revision: -1 }))
  await expect(getAssignedPolicy(target)).rejects.toThrow('forbidden')
  await expect(getAssignedPolicy(target)).rejects.toThrow()
})

test('forbidden and revision conflicts use status-only classifications without raw server messages', () => {
  const error = new AxiosError('private message')
  error.response = {
    status: 409,
    data: { message: 'private' },
    statusText: '',
    headers: {},
    config: { headers: undefined as never },
  }
  expect(accessPolicyErrorKind(error)).toBe('conflict')
  error.response.status = 403
  expect(accessPolicyErrorKind(error)).toBe('forbidden')
})
