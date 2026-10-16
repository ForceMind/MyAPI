/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { AxiosError, AxiosHeaders } from 'axios'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import {
  commitCredentialRotation,
  getCredentialRotationCommitDisposition,
  getCredentialRotationReceipt,
  previewCredentialRotation,
} from '../credential-rotation-api'

const digest = 'a'.repeat(64)
const secret = `sk-${'A'.repeat(48)}`
const receipt = {
  operation_id: digest,
  state: 'committed',
  token_id: 21,
  credential_version_before: 1,
  credential_version_after: 2,
  masked_key: 'sk-…AAAA',
  created_at: 123,
  old_secret_invalidated_at: 123,
  settlement_mode: 'authoritative_persistent',
  writer_changed: false,
}

afterEach(() => vi.restoreAllMocks())

test('ready preview requires verified effects and sends a strict empty body', async () => {
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: {
        status: 'ready',
        preview_revision: digest,
        expires_at: 123,
        credential_version_before: 1,
        credential_version_after: 2,
        blockers: [],
        effects: {
          token_id_preserved: true,
          old_secret_invalidation: 'immediate',
          accepted_settlement: 'continues',
          writer_changed: false,
        },
      },
    },
  } as never)

  await previewCredentialRotation(21)

  expect(post).toHaveBeenCalledWith(
    '/api/token/21/credential-rotation/preview',
    {},
    expect.objectContaining({
      timeout: 15000,
      disableDuplicate: true,
      skipAuthRefresh: true,
    })
  )
})

test('blocked preview omits unverified effects while ready cannot omit them', async () => {
  const post = vi.spyOn(api, 'post')
  post.mockResolvedValueOnce({
    data: {
      success: true,
      data: {
        status: 'blocked',
        preview_revision: '',
        expires_at: 0,
        credential_version_before: 1,
        credential_version_after: 0,
        blockers: [{ code: 'legacy_settlement_mode', state: 'blocked' }],
      },
    },
  } as never)
  await expect(previewCredentialRotation(21)).resolves.toMatchObject({
    status: 'blocked',
  })

  post.mockResolvedValueOnce({
    data: {
      success: true,
      data: {
        status: 'ready',
        preview_revision: digest,
        expires_at: 123,
        credential_version_before: 1,
        credential_version_after: 2,
        blockers: [],
      },
    },
  } as never)
  await expect(previewCredentialRotation(21)).rejects.toThrow()
})

test('ready preview rejects a skipped credential generation', async () => {
  vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: {
        status: 'ready',
        preview_revision: digest,
        expires_at: 123,
        credential_version_before: 1,
        credential_version_after: 3,
        blockers: [],
        effects: {
          token_id_preserved: true,
          old_secret_invalidation: 'immediate',
          accepted_settlement: 'continues',
          writer_changed: false,
        },
      },
    },
  } as never)

  await expect(previewCredentialRotation(21)).rejects.toThrow(
    'invalid versions'
  )
})

test('commit sends one scoped proof and accepts the fresh one-time reveal', async () => {
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: { receipt, replayed: false, reveal: { secret } },
    },
  } as never)

  await expect(
    commitCredentialRotation(
      21,
      {
        operation_id: digest,
        preview_revision: digest,
        confirmed: true,
      },
      'synthetic-proof'
    )
  ).resolves.toMatchObject({ replayed: false, reveal: { secret } })
  expect(post).toHaveBeenCalledWith(
    '/api/token/21/credential-rotation',
    {
      operation_id: digest,
      preview_revision: digest,
      confirmed: true,
    },
    expect.objectContaining({
      headers: { 'X-Security-Proof': 'synthetic-proof' },
      skipAuthRefresh: true,
    })
  )
})

test('receipt reads reject a different operation identity', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: { ...receipt, operation_id: 'b'.repeat(64) },
    },
  } as never)

  await expect(getCredentialRotationReceipt(21, digest)).rejects.toThrow(
    'receipt identity mismatch'
  )
})

test('receipt reads reject a skipped credential generation', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: { ...receipt, credential_version_after: 3 },
    },
  } as never)

  await expect(getCredentialRotationReceipt(21, digest)).rejects.toThrow(
    'invalid versions'
  )
})

test.each([
  { status: 403, marker: 'header', expected: 'not_accepted' },
  { status: 422, marker: 'body', expected: 'not_accepted' },
  { status: 409, marker: 'none', expected: 'unknown' },
  { status: 408, marker: 'header', expected: 'unknown' },
  { status: 499, marker: 'body', expected: 'unknown' },
  { status: 503, marker: 'header', expected: 'unknown' },
] as const)(
  '$status with $marker disposition is classified as $expected',
  ({ status, marker, expected }) => {
    const error = new AxiosError('Rotation failed')
    error.response = {
      status,
      statusText: 'Rotation failed',
      headers:
        marker === 'header'
          ? {
              'x-myapi-credential-rotation-disposition': 'not_accepted',
            }
          : {},
      config: { headers: new AxiosHeaders() },
      data:
        marker === 'body'
          ? { data: { commit_disposition: 'not_accepted' } }
          : {},
    }
    expect(getCredentialRotationCommitDisposition(error)).toBe(expected)
  }
)

test('an explicit unknown body cannot be weakened by a default header', () => {
  const error = new AxiosError('Rotation outcome unknown')
  error.response = {
    status: 409,
    statusText: 'Conflict',
    headers: {
      'x-myapi-credential-rotation-disposition': 'not_accepted',
    },
    config: { headers: new AxiosHeaders() },
    data: { data: { commit_disposition: 'unknown' } },
  }
  expect(getCredentialRotationCommitDisposition(error)).toBe('unknown')
})
