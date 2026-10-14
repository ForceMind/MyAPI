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
import { beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import {
  getSubscriptionShare,
  writeSubscriptionShare,
} from '../../subscription-share-api'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), put: vi.fn() } }))

const pool = {
  pool_id: 0,
  snapshot_id: 9,
  revision: 0,
  channel_name: 'Candidate',
  plan_type: 'plus',
  window_slot: 'primary',
  window_type: 'weekly',
  window_seconds: 604800,
  reset_at: 1_800_000_000,
  observed_at: 1_799_999_900,
  max_age_seconds: 300,
  sample_age_seconds: 100,
  state: 'active',
  allocated_bps: 0,
  share_bps: 0,
  estimated_used_micro: 0,
  unattributed_micro: 0,
  pending_attempts: 0,
  unsampled_attempts: 0,
  uncertain_attempts: 0,
  approximate: true,
  baseline_time: 0,
  replacement_required: false,
  rebaseline_required: false,
}

beforeEach(() => vi.resetAllMocks())

test('GET validates token identity and forwards cancellation without request deduplication', async () => {
  const controller = new AbortController()
  vi.mocked(api.get).mockResolvedValue({
    data: { success: true, data: { token_id: 21, pools: [pool] } },
  })
  await expect(getSubscriptionShare(21, controller.signal)).resolves.toEqual({
    token_id: 21,
    pools: [pool],
  })
  expect(api.get).toHaveBeenCalledWith('/api/token/21/subscription-share', {
    signal: controller.signal,
    disableDuplicate: true,
    skipBusinessError: true,
    skipErrorHandler: true,
  })

  vi.mocked(api.get).mockResolvedValueOnce({
    data: { success: true, data: { token_id: 22, pools: [] } },
  })
  await expect(getSubscriptionShare(21)).rejects.toThrow(
    'Subscription share identity mismatch'
  )
})

test('PUT preserves the integer basis-point CAS command and validates approximate responses', async () => {
  const command = {
    id: 'a'.repeat(64),
    pool_id: 0,
    snapshot_id: 9,
    expected_revision: 0,
    share_bps: 1234,
    confirmed_approximate: true as const,
  }
  vi.mocked(api.put).mockResolvedValue({
    data: {
      success: true,
      data: { token_id: 21, pools: [{ ...pool, approximate: true }] },
    },
  })
  await writeSubscriptionShare(21, command)
  expect(api.put).toHaveBeenCalledWith(
    '/api/token/21/subscription-share',
    command,
    { skipBusinessError: true, skipErrorHandler: true }
  )

  vi.mocked(api.put).mockResolvedValueOnce({
    data: {
      success: true,
      data: { token_id: 21, pools: [{ ...pool, approximate: false }] },
    },
  })
  await expect(writeSubscriptionShare(21, command)).rejects.toThrow()
})

test('replacement candidates and explicit replacement writes preserve their flags', async () => {
  const replacementPool = {
    ...pool,
    pool_id: 7,
    uncertain_attempts: 3,
    replacement_required: true as const,
  }
  const command = {
    id: 'b'.repeat(64),
    pool_id: 7,
    snapshot_id: 9,
    expected_revision: 2,
    share_bps: 2500,
    confirmed_approximate: true as const,
    replace_window: true as const,
  }
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: { token_id: 21, pools: [replacementPool] },
    },
  })
  vi.mocked(api.put).mockResolvedValue({
    data: {
      success: true,
      data: { token_id: 21, pools: [replacementPool] },
    },
  })

  await expect(getSubscriptionShare(21)).resolves.toEqual({
    token_id: 21,
    pools: [replacementPool],
  })
  await writeSubscriptionShare(21, command)
  expect(api.put).toHaveBeenCalledWith(
    '/api/token/21/subscription-share',
    command,
    { skipBusinessError: true, skipErrorHandler: true }
  )
})

test('rebaseline candidates preserve recovery and uncertain-attempt fields', async () => {
  const rebaselinePool = {
    ...pool,
    pool_id: 7,
    uncertain_attempts: 4,
    rebaseline_required: true as const,
  }
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: { token_id: 21, pools: [rebaselinePool] },
    },
  })

  await expect(getSubscriptionShare(21)).resolves.toEqual({
    token_id: 21,
    pools: [rebaselinePool],
  })
})
