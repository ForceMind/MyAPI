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
import { api, getFreshAuthHeaders } from '@/lib/api'

import { API_ENDPOINTS } from './constants'
import type {
  ChatCompletionRequest,
  ChatCompletionResponse,
  ModelOption,
  GroupOption,
  PlaygroundParameterKey,
  PlaygroundKey,
} from './types'

const PLAYGROUND_PARAMETER_KEYS = new Set<PlaygroundParameterKey>([
  'temperature',
  'top_p',
  'max_tokens',
  'frequency_penalty',
  'presence_penalty',
  'seed',
])

type UserModelCapability = {
  provider?: unknown
  unsupported_parameters?: unknown
}

export function parseUserModelOptions(payload: unknown): ModelOption[] {
  if (!payload || typeof payload !== 'object') {
    return []
  }

  const response = payload as {
    success?: unknown
    data?: unknown
    capabilities?: unknown
  }
  if (response.success !== true || !Array.isArray(response.data)) {
    return []
  }

  const capabilities =
    response.capabilities && typeof response.capabilities === 'object'
      ? (response.capabilities as Record<string, UserModelCapability>)
      : {}

  return response.data
    .map((model: unknown) => {
      if (typeof model === 'string') return model
      if (
        model &&
        typeof model === 'object' &&
        'id' in model &&
        typeof model.id === 'string'
      ) {
        return model.id
      }
      return null
    })
    .filter((model): model is string => Boolean(model))
    .map((model) => {
      const capability = capabilities[model]
      const unsupportedParameters = Array.isArray(
        capability?.unsupported_parameters
      )
        ? capability.unsupported_parameters.filter(
            (key): key is PlaygroundParameterKey =>
              typeof key === 'string' &&
              PLAYGROUND_PARAMETER_KEYS.has(key as PlaygroundParameterKey)
          )
        : []

      return {
        label: model,
        value: model,
        ...(typeof capability?.provider === 'string'
          ? { provider: capability.provider }
          : {}),
        ...(unsupportedParameters.length > 0 ? { unsupportedParameters } : {}),
      }
    })
}

/**
 * Send chat completion request (non-streaming)
 */
export async function sendChatCompletion(
  payload: ChatCompletionRequest,
  keyId: number,
  signal?: AbortSignal
): Promise<ChatCompletionResponse> {
  const keyHeaders = getPlaygroundKeyHeaders(keyId)
  const sessionHeaders = await getFreshAuthHeaders()
  signal?.throwIfAborted()
  const res = await api.post(API_ENDPOINTS.CHAT_COMPLETIONS, payload, {
    signal,
    headers: { ...sessionHeaders, ...keyHeaders },
    skipErrorHandler: true,
    // A revoked API key also returns 401. Never refresh/replay this paid request.
    skipAuthRefresh: true,
  })
  return res.data
}

/**
 * Get user available models
 */
export async function getUserModels(
  keyId: number,
  signal?: AbortSignal
): Promise<ModelOption[]> {
  const keyHeaders = getPlaygroundKeyHeaders(keyId)
  const sessionHeaders = await getFreshAuthHeaders()
  signal?.throwIfAborted()
  const res = await api.get(API_ENDPOINTS.USER_MODELS, {
    headers: { ...sessionHeaders, ...keyHeaders },
    signal,
    skipErrorHandler: true,
    skipAuthRefresh: true,
    // Generic GET deduplication does not include headers in its identity.
    disableDuplicate: true,
  })
  const { data } = res

  return parseUserModelOptions(data)
}

/**
 * Get user groups
 */
export async function getUserGroups(): Promise<GroupOption[]> {
  const res = await api.get(API_ENDPOINTS.USER_GROUPS)
  const { data } = res

  if (!data.success || !data.data) {
    return []
  }

  const groupData = data.data as Record<string, { desc: string; ratio: number }>

  // label is for button display (name only); desc is for dropdown content
  return Object.entries(groupData).map(([group, info]) => ({
    label: group,
    value: group,
    ratio: info.ratio,
    desc: info.desc,
  }))
}

export function getPlaygroundKeyHeaders(keyId: number): Record<string, string> {
  if (!Number.isSafeInteger(keyId) || keyId <= 0) {
    throw new Error('Select an available API key before sending')
  }
  return { 'X-MyAPI-Key-ID': String(keyId) }
}

// Explicitly copy the safe fields; never retain a secret even if the server adds one.
export function parsePlaygroundKeys(payload: unknown): PlaygroundKey[] {
  if (!payload || typeof payload !== 'object') return []
  const response = payload as { success?: unknown; data?: { items?: unknown } }
  if (response.success !== true || !Array.isArray(response.data?.items)) {
    return []
  }
  return response.data.items.flatMap((item: unknown) => {
    if (!item || typeof item !== 'object') return []
    const key = item as Record<string, unknown>
    if (
      !Number.isSafeInteger(key.id) ||
      Number(key.id) <= 0 ||
      typeof key.name !== 'string' ||
      typeof key.status !== 'number' ||
      typeof key.group !== 'string' ||
      typeof key.remain_quota !== 'number' ||
      typeof key.unlimited_quota !== 'boolean' ||
      typeof key.expired_time !== 'number'
    ) {
      return []
    }
    return [
      {
        id: Number(key.id),
        name: key.name,
        status: key.status,
        group: key.group,
        remain_quota: key.remain_quota,
        unlimited_quota: key.unlimited_quota,
        expired_time: key.expired_time,
        ...(typeof key.strict_token_budget === 'boolean'
          ? { strict_token_budget: key.strict_token_budget }
          : {}),
      },
    ]
  })
}

export function isPlaygroundKeyAvailable(
  key: PlaygroundKey,
  now = Date.now()
): boolean {
  return (
    key.status === 1 &&
    (key.expired_time === -1 || key.expired_time > now / 1000) &&
    (key.unlimited_quota || key.remain_quota > 0)
  )
}

export async function getPlaygroundKeys(): Promise<PlaygroundKey[]> {
  const keys: PlaygroundKey[] = []
  for (let page = 1; ; page++) {
    const response = await api.get(API_ENDPOINTS.KEYS, {
      params: { p: page, page_size: 100 },
      skipErrorHandler: true,
    })
    if (response.data?.success !== true) {
      throw new Error('Failed to load API keys')
    }
    const items = parsePlaygroundKeys(response.data)
    keys.push(...items)
    const total: unknown = response.data?.data?.total
    if (
      typeof total !== 'number' ||
      keys.length >= total ||
      items.length === 0
    ) {
      break
    }
  }
  return keys
}
