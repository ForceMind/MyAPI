/*
Copyright (C) 2026 ForceMind

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or any later
version.
*/
import { AxiosError, type AxiosAdapter, type InternalAxiosRequestConfig } from 'axios'
import { afterEach, expect, it } from 'vitest'

import { api } from '@/lib/api'

import { createManagementKey, revokeManagementKey, type CreateManagementKey } from '../management-key-api'

const previousAdapter = api.defaults.adapter
afterEach(() => { api.defaults.adapter = previousAdapter })

const payload: CreateManagementKey = {
  name: 'local audit', scopes: ['system.read'], channel_ids: [], token_ids: [], expires_at: 2_000_000_000,
}

it.each(['create', 'revoke'] as const)('does not refresh or replay a %s write after a 401', async (operation) => {
  const requests: InternalAxiosRequestConfig[] = []
  api.defaults.adapter = (async (config) => {
    requests.push(config)
    throw new AxiosError('rejected', AxiosError.ERR_BAD_REQUEST, config, undefined, {
      config, data: { success: false }, status: 401, statusText: 'Unauthorized', headers: {},
    })
  }) as AxiosAdapter
  const signal = new AbortController().signal
  const request = operation === 'create' ? createManagementKey(payload, signal) : revokeManagementKey(17, signal)
  await expect(request).rejects.toBeInstanceOf(AxiosError)
  expect(requests).toHaveLength(1)
  expect(requests[0]?.skipAuthRefresh).toBe(true)
  expect(requests[0]?.skipBusinessError).toBe(true)
  expect(requests[0]?.skipErrorHandler).toBe(true)
  expect(requests[0]?.url).toBe(operation === 'create' ? '/api/management/keys' : '/api/management/keys/17')
  if (operation === 'create') expect(JSON.parse(requests[0]?.data as string)).toEqual(payload)
})
