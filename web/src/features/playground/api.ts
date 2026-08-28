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
import { api } from '@/lib/api'

import { API_ENDPOINTS } from './constants'
import type {
  ChatCompletionRequest,
  ChatCompletionResponse,
  ModelOption,
  GroupOption,
  PlaygroundParameterKey,
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
    .filter((model): model is string => typeof model === 'string')
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
  signal?: AbortSignal
): Promise<ChatCompletionResponse> {
  const res = await api.post(API_ENDPOINTS.CHAT_COMPLETIONS, payload, {
    signal,
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Get user available models
 */
export async function getUserModels(group: string): Promise<ModelOption[]> {
  const res = await api.get(API_ENDPOINTS.USER_MODELS, {
    params: { group },
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
