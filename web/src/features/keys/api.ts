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
import { api, type ApiRequestConfig } from '@/lib/api'

import {
  apiKeySchema,
  type ApiKey,
  type ApiResponse,
  type GetApiKeysParams,
  type GetApiKeysResponse,
  type SearchApiKeysParams,
  type ApiKeyFormData,
  type ApiKeyUpdateData,
  type TokenAutoGroupsConfig,
} from './types'

// ============================================================================
// API Key Management
// ============================================================================

// Get paginated API keys list
export async function getApiKeys(
  params: GetApiKeysParams = {}
): Promise<GetApiKeysResponse> {
  const { p = 1, size = 10 } = params
  const res = await api.get(`/api/token/?p=${p}&size=${size}`)
  const response = res.data as GetApiKeysResponse
  if (response.data?.items) {
    response.data.items = apiKeySchema.array().parse(response.data.items)
  }
  return response
}

// Search API keys by keyword or token (with pagination)
export async function searchApiKeys(
  params: SearchApiKeysParams
): Promise<GetApiKeysResponse> {
  const { keyword = '', token = '', p, size } = params
  const queryParams = new URLSearchParams()
  if (keyword) queryParams.set('keyword', keyword)
  if (token) queryParams.set('token', token)
  if (p != null) queryParams.set('p', String(p))
  if (size != null) queryParams.set('size', String(size))
  const res = await api.get(`/api/token/search?${queryParams.toString()}`)
  const response = res.data as GetApiKeysResponse
  if (response.data?.items) {
    response.data.items = apiKeySchema.array().parse(response.data.items)
  }
  return response
}

// Get single API key by ID
export async function getApiKey(id: number): Promise<ApiResponse<ApiKey>> {
  const res = await api.get(`/api/token/${id}`)
  const response = res.data as ApiResponse<ApiKey>
  if (response.data !== undefined) {
    response.data = apiKeySchema.parse(response.data)
  }
  return response
}

// Get the current user's global Auto order and the per-token selection limit.
export async function getTokenAutoGroups(): Promise<
  ApiResponse<TokenAutoGroupsConfig>
> {
  const res = await api.get('/api/token/auto-groups')
  return res.data
}

// Create a new API key
export async function createApiKey(
  data: ApiKeyFormData,
  config?: ApiRequestConfig
): Promise<ApiResponse<ApiKey>> {
  const res = config
    ? await api.post('/api/token/', data, config)
    : await api.post('/api/token/', data)
  return res.data
}

// Update an existing API key
export async function updateApiKey(
  data: ApiKeyUpdateData,
  config?: ApiRequestConfig
): Promise<ApiResponse<ApiKey>> {
  const res = config
    ? await api.put('/api/token/', data, config)
    : await api.put('/api/token/', data)
  return res.data
}

// Delete a single API key
export async function deleteApiKey(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/token/${id}/`)
  return res.data
}

// Batch delete multiple API keys
export async function batchDeleteApiKeys(
  ids: number[]
): Promise<ApiResponse<number>> {
  const res = await api.post('/api/token/batch', { ids })
  return res.data
}

// Update API key status (enable/disable)
export async function updateApiKeyStatus(
  id: number,
  status: number,
  config?: ApiRequestConfig
): Promise<ApiResponse<ApiKey>> {
  const res = config
    ? await api.put('/api/token/?status_only=true', { id, status }, config)
    : await api.put('/api/token/?status_only=true', { id, status })
  return res.data
}

// Fetch the real (unmasked) key for a token by ID
export async function fetchTokenKey(
  id: number,
  config?: ApiRequestConfig
): Promise<{ success: boolean; message?: string; data?: { key: string } }> {
  const res = config
    ? await api.post(`/api/token/${id}/key`, undefined, config)
    : await api.post(`/api/token/${id}/key`)
  return res.data
}

// Batch fetch real (unmasked) keys for multiple tokens
export async function fetchTokenKeysBatch(
  ids: number[],
  config?: ApiRequestConfig
): Promise<{
  success: boolean
  message?: string
  data?: { keys: Record<number, string> }
}> {
  const res = config
    ? await api.post('/api/token/batch/keys', { ids }, config)
    : await api.post('/api/token/batch/keys', { ids })
  return res.data
}
