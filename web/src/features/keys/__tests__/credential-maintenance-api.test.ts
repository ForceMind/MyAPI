import { AxiosError, AxiosHeaders } from 'axios'
/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import {
  commitCredentialMigration,
  getCredentialMaintenance,
  getCredentialMigrationReceipt,
  isUncertainCredentialCommit,
  previewCredentialMigration,
} from '../credential-maintenance-api'

const digest = 'a'.repeat(64)
const input = {
  target_channel_id: 8,
  target_shares: [{ source_pool_id: 11, target_pool_id: 12, share_bps: 2500 }],
}
const changes = {
  policy: {
    changed: true,
    before_revision: 2,
    after_revision: 3,
    source_channel_id: 7,
    target_channel_id: 8,
  },
  allocations: [
    {
      source_pool_id: 11,
      target_pool_id: 12,
      window_slot: 'primary',
      share_bps: 2500,
      target_existing: false,
    },
  ],
}

test('an HTTP request timeout cannot establish that migration did not commit', () => {
  const error = new AxiosError('Request timeout')
  error.response = {
    status: 408,
    statusText: 'Request Timeout',
    headers: {},
    config: { headers: new AxiosHeaders() },
    data: {},
  }
  expect(isUncertainCredentialCommit(error)).toBe(true)
})

afterEach(() => vi.restoreAllMocks())

test('maintenance reads are fresh bounded requests that cannot refresh and replay authentication', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        token: {
          id: 21,
          name: 'Desktop',
          masked_key: 'sk-…abcd',
          owner_user_id: 1,
        },
        permissions: { can_migrate: true, reason: '' },
        rotation: {
          commit_supported: false,
          status: 'blocked',
          blockers: [
            { code: 'rotation_commit_not_supported', state: 'blocked' },
          ],
        },
        migration: {
          status: 'ready',
          source: {
            channel_id: 7,
            channel_name: 'Current',
            subject_hint: 'abcd…wxyz',
            windows: [
              {
                pool_id: 11,
                window_slot: 'primary',
                revision: 3,
                share_bps: 2500,
                observed_at: 1,
                reset_at: 2,
              },
            ],
          },
          candidates: [],
          blockers: [],
        },
      },
    },
  } as never)
  const controller = new AbortController()

  await getCredentialMaintenance(21, controller.signal)

  expect(get).toHaveBeenCalledWith('/api/token/21/maintenance', {
    signal: controller.signal,
    timeout: 15000,
    disableDuplicate: true,
    skipAuthRefresh: true,
    skipBusinessError: true,
    skipErrorHandler: true,
  })
})

test('preview preserves exact source shares and commit sends one scoped proof without secrets', async () => {
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          status: 'ready',
          preview_revision: digest,
          expires_at: 123,
          blockers: [],
          changes,
        },
      },
    } as never)
    .mockResolvedValueOnce({
      data: {
        success: true,
        data: {
          operation_id: digest,
          state: 'committed',
          replayed: false,
          token_id: 21,
          source_subject_hint: 'abcd…wxyz',
          target_subject_hint: 'efgh…stuv',
          source_channel_id: 7,
          target_channel_id: 8,
          policy_before_revision: 2,
          policy_after_revision: 3,
          created_at: 123,
          changes,
        },
      },
    } as never)

  await previewCredentialMigration(21, input)
  await commitCredentialMigration(
    21,
    {
      ...input,
      operation_id: digest,
      preview_revision: digest,
      confirmed: true,
    },
    'synthetic-proof'
  )

  expect(post.mock.calls[0]?.[1]).toEqual(input)
  expect(post.mock.calls[1]?.[1]).toEqual({
    ...input,
    operation_id: digest,
    preview_revision: digest,
    confirmed: true,
  })
  expect(post.mock.calls[1]?.[2]).toMatchObject({
    timeout: 15000,
    disableDuplicate: true,
    skipAuthRefresh: true,
    headers: { 'X-Security-Proof': 'synthetic-proof' },
  })
  expect(JSON.stringify(post.mock.calls[1]?.[1])).not.toContain('subject')
  expect(JSON.stringify(post.mock.calls[1]?.[1])).not.toContain('secret')
})

test('receipt read rejects a response for another operation', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        operation_id: 'b'.repeat(64),
        state: 'committed',
        token_id: 21,
        source_subject_hint: 'abcd…wxyz',
        target_subject_hint: 'efgh…stuv',
        source_channel_id: 7,
        target_channel_id: 8,
        policy_before_revision: 2,
        policy_after_revision: 3,
        created_at: 123,
        changes,
      },
    },
  } as never)

  await expect(getCredentialMigrationReceipt(21, digest)).rejects.toThrow(
    'receipt identity mismatch'
  )
})
