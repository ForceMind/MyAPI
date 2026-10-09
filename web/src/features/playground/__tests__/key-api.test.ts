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
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import {
  getPlaygroundKeyHeaders,
  getPlaygroundKeys,
  getUserModels,
  isPlaygroundKeyAvailable,
  parsePlaygroundKeys,
  parseUserModelOptions,
  sendChatCompletion,
} from '../api'
import type { ChatCompletionRequest, PlaygroundKey } from '../types'

const availableKey: PlaygroundKey = {
  id: 17,
  name: 'Personal chat',
  status: 1,
  group: 'personal',
  remain_quota: 100,
  unlimited_quota: false,
  expired_time: -1,
}

beforeEach(() => {
  localStorage.clear()
  useAuthStore.setState((state) => ({
    auth: {
      ...state.auth,
      accessToken: 'session-access-token',
      accessExpiresAt: Math.floor(Date.now() / 1000) + 3600,
      bootstrapState: 'complete',
    },
  }))
})

afterEach(() => {
  useAuthStore.getState().auth.reset()
  localStorage.clear()
  vi.restoreAllMocks()
})

describe('playground key DTO boundary', () => {
  test.each([true, false])(
    'retains the safe strict-budget boolean %s',
    (strict) => {
      const key = { ...availableKey, strict_token_budget: strict }
      expect(
        parsePlaygroundKeys({ success: true, data: { items: [key] } })
      ).toEqual([key])
    }
  )

  test('does not infer a strict budget from malformed metadata', () => {
    expect(
      parsePlaygroundKeys({
        success: true,
        data: { items: [{ ...availableKey, strict_token_budget: 'true' }] },
      })
    ).toEqual([availableKey])
  })

  test('copies only safe key fields when the server includes secrets', () => {
    const response = {
      success: true,
      data: {
        items: [
          {
            ...availableKey,
            key: 'sk-must-not-be-retained',
            secret: 'must-not-be-retained',
            token: 'must-not-be-retained',
            authorization: 'Bearer must-not-be-retained',
            metadata: { secret: 'must-not-be-retained' },
          },
        ],
        total: 1,
      },
    }

    const keys = parsePlaygroundKeys(response)

    expect(keys).toEqual([availableKey])
    expect(keys[0]).not.toBe(response.data.items[0])
    expect(JSON.stringify(keys)).not.toContain('must-not-be-retained')
  })

  test.each([
    ['missing response', undefined],
    [
      'unsuccessful response',
      { success: false, data: { items: [availableKey] } },
    ],
    ['non-paginated response', { success: true, data: [availableKey] }],
    ['missing items', { success: true, data: {} }],
  ])('returns no keys for %s', (_name, response) => {
    expect(parsePlaygroundKeys(response)).toEqual([])
  })

  test('discards malformed rows while preserving valid paginated keys', () => {
    expect(
      parsePlaygroundKeys({
        success: true,
        data: {
          items: [
            null,
            { ...availableKey, id: 0 },
            { ...availableKey, id: '17' },
            { ...availableKey, id: Number.MAX_SAFE_INTEGER + 1 },
            { ...availableKey, name: null },
            { ...availableKey, unlimited_quota: 'false' },
            { ...availableKey, expired_time: undefined },
            availableKey,
          ],
        },
      })
    ).toEqual([availableKey])
  })

  test('reads every keys page and sanitizes secrets from each page', async () => {
    const secondKey = { ...availableKey, id: 28, name: 'Work chat' }
    const get = vi
      .spyOn(api, 'get')
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: { items: [{ ...availableKey, key: 'secret-a' }], total: 2 },
        },
      })
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: { items: [{ ...secondKey, secret: 'secret-b' }], total: 2 },
        },
      })

    await expect(getPlaygroundKeys()).resolves.toEqual([
      availableKey,
      secondKey,
    ])

    expect(get).toHaveBeenNthCalledWith(1, '/pg/keys', {
      params: { p: 1, page_size: 100 },
      skipErrorHandler: true,
    })
    expect(get).toHaveBeenNthCalledWith(2, '/pg/keys', {
      params: { p: 2, page_size: 100 },
      skipErrorHandler: true,
    })
    expect(get).toHaveBeenCalledTimes(2)
  })

  test('rejects a failed later page instead of returning a partial key list', async () => {
    vi.spyOn(api, 'get')
      .mockResolvedValueOnce({
        data: { success: true, data: { items: [availableKey], total: 2 } },
      })
      .mockResolvedValueOnce({
        data: { success: false, message: 'Not authorized' },
      })

    await expect(getPlaygroundKeys()).rejects.toThrow('Failed to load API keys')
  })
})

describe('key availability and identity headers', () => {
  const now = 2_000_000_000_000

  test.each([
    ['disabled', { status: 2 }],
    ['expired', { expired_time: now / 1000 - 1 }],
    ['expires at the current instant', { expired_time: now / 1000 }],
    ['depleted', { remain_quota: 0 }],
  ])('rejects a key that is %s', (_name, changes) => {
    expect(isPlaygroundKeyAvailable({ ...availableKey, ...changes }, now)).toBe(
      false
    )
  })

  test('accepts an unlimited key with zero quota and a future expiry', () => {
    expect(
      isPlaygroundKeyAvailable(
        {
          ...availableKey,
          remain_quota: 0,
          unlimited_quota: true,
          expired_time: now / 1000 + 1,
        },
        now
      )
    ).toBe(true)
  })

  test('uses only the selected numeric ID in the identity header', () => {
    expect(getPlaygroundKeyHeaders(17)).toEqual({ 'X-MyAPI-Key-ID': '17' })
  })

  test.each([0, -1, 1.5, Number.NaN, Infinity, Number.MAX_SAFE_INTEGER + 1])(
    'rejects invalid key ID %s before dispatching a model request',
    async (id) => {
      const get = vi.spyOn(api, 'get')

      expect(() => getPlaygroundKeyHeaders(id)).toThrow(
        'Select an available API key'
      )
      await expect(getUserModels(id)).rejects.toThrow(
        'Select an available API key'
      )
      expect(get).not.toHaveBeenCalled()
    }
  )

  test('sends non-streaming requests with the selected identity and abort signal', async () => {
    const payload: ChatCompletionRequest = {
      model: 'personal-model',
      messages: [{ role: 'user', content: 'Hello' }],
      stream: false,
    }
    const response = {
      choices: [{ message: { role: 'assistant', content: 'Hello back' } }],
    }
    const post = vi.spyOn(api, 'post').mockResolvedValue({ data: response })
    const controller = new AbortController()

    await expect(
      sendChatCompletion(payload, 17, controller.signal)
    ).resolves.toBe(response)

    expect(post).toHaveBeenCalledWith('/pg/chat/completions', payload, {
      headers: {
        'Content-Type': 'application/json',
        Authorization: 'Bearer session-access-token',
        'X-MyAPI-Key-ID': '17',
      },
      signal: controller.signal,
      skipErrorHandler: true,
      skipAuthRefresh: true,
    })
  })

  test('rejects an invalid identity before dispatching a completion request', async () => {
    const post = vi.spyOn(api, 'post')

    await expect(
      sendChatCompletion(
        { model: 'personal-model', messages: [], stream: false },
        0
      )
    ).rejects.toThrow('Select an available API key')

    expect(post).not.toHaveBeenCalled()
  })
})

describe('key-scoped model API', () => {
  test('requests models for the selected key and accepts the object DTO', async () => {
    const get = vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: [{ id: 'personal-model', object: 'model', owned_by: 'provider' }],
        capabilities: {
          'personal-model': {
            provider: 'Codex',
            unsupported_parameters: [
              'temperature',
              'seed',
              'unknown_parameter',
            ],
          },
        },
      },
    })

    await expect(getUserModels(17)).resolves.toEqual([
      {
        label: 'personal-model',
        value: 'personal-model',
        provider: 'Codex',
        unsupportedParameters: ['temperature', 'seed'],
      },
    ])

    expect(get).toHaveBeenCalledWith(
      '/pg/models',
      expect.objectContaining({
        headers: {
          'Content-Type': 'application/json',
          Authorization: 'Bearer session-access-token',
          'X-MyAPI-Key-ID': '17',
        },
        skipAuthRefresh: true,
        disableDuplicate: true,
      })
    )
  })

  test('drops malformed model entries without inventing model choices', () => {
    expect(
      parseUserModelOptions({
        success: true,
        data: [{ id: 'usable-model' }, { id: 12 }, { id: '' }, null, false],
      })
    ).toEqual([{ label: 'usable-model', value: 'usable-model' }])
  })
})
