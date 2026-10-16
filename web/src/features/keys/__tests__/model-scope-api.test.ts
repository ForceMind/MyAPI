/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import {
  getModelScope,
  getModelScopeCatalog,
  updateModelScope,
} from '../model-scope-api'

const revision = 'b'.repeat(64)
const envelope = {
  success: true,
  data: {
    token_id: 21,
    token_name: 'Desktop application',
    model_limits_enabled: true,
    models: ['gpt-5-codex'],
    revision,
  },
}

afterEach(() => vi.restoreAllMocks())

test('model scope reads use an isolated abortable no-replay request', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({ data: envelope })
  const controller = new AbortController()
  await expect(getModelScope(21, controller.signal)).resolves.toEqual(
    envelope.data
  )
  expect(get).toHaveBeenCalledWith('/api/token/21/model-limits', {
    signal: controller.signal,
    timeout: 15000,
    disableDuplicate: true,
    skipAuthRefresh: true,
    skipBusinessError: true,
    skipErrorHandler: true,
  })
})

test('model scope writes contain only revision, enabled state, and models', async () => {
  const put = vi.spyOn(api, 'put').mockResolvedValue({ data: envelope })
  const body = {
    expected_revision: revision,
    model_limits_enabled: true,
    models: ['gpt-5-codex'],
  }
  await updateModelScope(21, body)
  expect(put).toHaveBeenCalledWith(
    '/api/token/21/model-limits',
    body,
    expect.objectContaining({
      timeout: 15000,
      skipAuthRefresh: true,
      skipBusinessError: true,
      skipErrorHandler: true,
    })
  )
  expect(Object.keys(put.mock.calls[0][1] as object).sort()).toEqual([
    'expected_revision',
    'model_limits_enabled',
    'models',
  ])
})

test('model scope responses reject wrong identity, revision, and extra fields', async () => {
  vi.spyOn(api, 'get')
    .mockResolvedValueOnce({
      data: { ...envelope, data: { ...envelope.data, token_id: 22 } },
    })
    .mockResolvedValueOnce({
      data: { ...envelope, data: { ...envelope.data, revision: 'ABC' } },
    })
    .mockResolvedValueOnce({
      data: {
        ...envelope,
        data: { ...envelope.data, key: 'unexpected-secret-field' },
      },
    })
  await expect(getModelScope(21)).rejects.toThrow('identity mismatch')
  await expect(getModelScope(21)).rejects.toThrow()
  await expect(getModelScope(21)).rejects.toThrow()
})

test('catalog reads carry the Key group and validate the suggestion list', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: ['alpha', 'beta'] },
  })
  await expect(getModelScopeCatalog('auto')).resolves.toEqual(['alpha', 'beta'])
  expect(get).toHaveBeenCalledWith(
    '/api/user/models',
    expect.objectContaining({ params: { group: 'auto' } })
  )
})
