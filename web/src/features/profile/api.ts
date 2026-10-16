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
import type {
  PasskeyOptionsPayload,
  PasskeyStatus,
} from '@/features/auth/passkey'
import { api } from '@/lib/api'
import type { CustomOAuthBinding } from '@/lib/oauth'
import type { LoginSession } from '@/stores/auth-store'

import type {
  ApiResponse,
  UserProfile,
  UpdateUserRequest,
  UpdateUserSettingsRequest,
  DeleteAccountRequest,
  CheckinStatusResponse,
  CheckinResponse,
  TwoFASetupData,
} from './types'

// ============================================================================
// User Profile APIs
// ============================================================================

/**
 * Get current user profile
 */
export async function getUserProfile(
  signal?: AbortSignal
): Promise<ApiResponse<UserProfile>> {
  const controller = new AbortController()
  const cancel = () => controller.abort()
  const cancelled = new Promise<never>((_, reject) => {
    controller.signal.addEventListener(
      'abort',
      () => reject(new Error('Profile request cancelled')),
      { once: true }
    )
  })
  signal?.addEventListener('abort', cancel, { once: true })
  if (signal?.aborted) cancel()
  // Axios' adapter timeout does not include authentication interceptors.
  // Bound the complete read while retaining the existing refresh behavior.
  const deadline = globalThis.setTimeout(cancel, 15000)
  try {
    const res = await Promise.race([
      api.get<ApiResponse<UserProfile>>('/api/user/self', {
        signal: controller.signal,
        timeout: 15000,
        disableDuplicate: true,
        skipErrorHandler: true,
        skipBusinessError: true,
      }),
      cancelled,
    ])
    return res.data
  } finally {
    globalThis.clearTimeout(deadline)
    signal?.removeEventListener('abort', cancel)
  }
}

/**
 * Update user profile
 */
export async function updateUserProfile(
  data: UpdateUserRequest,
  signal?: AbortSignal
): Promise<ApiResponse> {
  const res = await api.put('/api/user/self', data, {
    acceptAuthRotation: Boolean(data.password),
    skipAuthRefresh: Boolean(data.password),
    skipErrorHandler: Boolean(data.password),
    skipBusinessError: Boolean(data.password),
    signal,
  })
  return res.data
}

/**
 * Update user settings
 */
export async function updateUserSettings(
  data: UpdateUserSettingsRequest
): Promise<ApiResponse> {
  const res = await api.put('/api/user/setting', data)
  return res.data
}

/**
 * Update interface language preference
 */
export async function updateUserLanguage(
  language: string
): Promise<ApiResponse> {
  const res = await api.put('/api/user/self', { language })
  return res.data
}

/**
 * Delete user account
 */
export async function deleteUserAccount(
  data?: DeleteAccountRequest,
  signal?: AbortSignal
): Promise<ApiResponse> {
  const res = await api.delete('/api/user/self', {
    data,
    signal,
    skipAuthRefresh: true,
    skipErrorHandler: true,
    skipBusinessError: true,
  })
  return res.data
}

/**
 * Generate/regenerate system access token
 */
export async function generateAccessToken(
  signal?: AbortSignal
): Promise<ApiResponse<string>> {
  const res = await api.get('/api/user/token', {
    signal,
    disableDuplicate: true,
    skipAuthRefresh: true,
    skipErrorHandler: true,
    skipBusinessError: true,
  })
  return res.data
}

// ============================================================================
// Account Binding APIs
// ============================================================================

/**
 * Send email verification code
 */
export async function sendEmailVerification(
  email: string,
  turnstileToken?: string,
  signal?: AbortSignal
): Promise<ApiResponse> {
  const params = new URLSearchParams({ email })
  if (turnstileToken) {
    params.append('turnstile', turnstileToken)
  }
  const res = await api.get(`/api/verification?${params}`, {
    signal,
    disableDuplicate: true,
    skipAuthRefresh: true,
    skipErrorHandler: true,
    skipBusinessError: true,
  })
  return res.data
}

/**
 * Bind email account
 */
export async function bindEmail(
  email: string,
  code: string,
  signal?: AbortSignal
): Promise<ApiResponse> {
  const res = await api.post(
    '/api/oauth/email/bind',
    { email, code },
    {
      signal,
      skipAuthRefresh: true,
      skipErrorHandler: true,
      skipBusinessError: true,
    }
  )
  return res.data
}

/**
 * Bind WeChat account
 */
export async function bindWeChat(
  code: string,
  signal?: AbortSignal
): Promise<ApiResponse> {
  const res = await api.post(
    '/api/oauth/wechat/bind',
    { code },
    {
      signal,
      skipAuthRefresh: true,
      skipBusinessError: true,
      skipErrorHandler: true,
    }
  )
  return res.data
}

export interface TelegramBindFlow {
  flow_token: string
  callback_url: string
  expires_at: number
}

export async function startTelegramBind(
  signal?: AbortSignal
): Promise<ApiResponse<TelegramBindFlow>> {
  const res = await api.post('/api/oauth/telegram/bind/start', undefined, {
    signal,
    skipAuthRefresh: true,
    skipErrorHandler: true,
    skipBusinessError: true,
  })
  return res.data
}

export async function startOAuthBindFlow(
  provider: string,
  signal?: AbortSignal
): Promise<string> {
  const res = await api.post(
    '/api/oauth/state',
    { provider, intent: 'bind' },
    {
      signal,
      skipAuthRefresh: true,
      skipErrorHandler: true,
      skipBusinessError: true,
    }
  )
  const value = res.data?.data
  if (res.data?.success && typeof value === 'string') return value
  if (res.data?.success && typeof value?.flow_token === 'string') {
    return value.flow_token
  }
  throw new Error(res.data?.message || 'Failed to initialize OAuth')
}

export async function completeOAuthBindFlow(
  provider: string,
  params: Record<string, string>,
  signal?: AbortSignal
): Promise<ApiResponse> {
  const res = await api.get(`/api/oauth/${provider}`, {
    params,
    signal,
    disableDuplicate: true,
    skipAuthRefresh: true,
    skipErrorHandler: true,
    skipBusinessError: true,
  })
  return res.data
}

// ============================================================================
// Login Session APIs
// ============================================================================

export async function getLoginSessions(
  signal?: AbortSignal
): Promise<ApiResponse<LoginSession[]>> {
  const res = await api.get('/api/user/sessions', {
    signal,
    disableDuplicate: true,
    skipAuthRefresh: true,
    skipErrorHandler: true,
    skipBusinessError: true,
  })
  return res.data
}

export async function revokeLoginSession(
  sid: string,
  signal?: AbortSignal
): Promise<ApiResponse> {
  const res = await api.delete(
    `/api/user/sessions/${encodeURIComponent(sid)}`,
    {
      signal,
      skipAuthRefresh: true,
      skipErrorHandler: true,
      skipBusinessError: true,
    }
  )
  return res.data
}

export async function revokeOtherLoginSessions(
  signal?: AbortSignal
): Promise<ApiResponse> {
  const res = await api.post('/api/user/sessions/revoke-others', undefined, {
    signal,
    skipAuthRefresh: true,
    skipErrorHandler: true,
    skipBusinessError: true,
  })
  return res.data
}

// ============================================================================
// Two-factor authentication APIs
// ============================================================================

export async function getTwoFAStatus(
  signal?: AbortSignal
): Promise<ApiResponse> {
  if (signal?.aborted) throw new Error('2FA status request cancelled')

  const controller = new AbortController()
  const cancel = () => controller.abort()
  const cancelled = new Promise<never>((_, reject) => {
    controller.signal.addEventListener(
      'abort',
      () => reject(new Error('2FA status request cancelled')),
      { once: true }
    )
  })
  signal?.addEventListener('abort', cancel, { once: true })
  const deadline = globalThis.setTimeout(cancel, 15000)
  try {
    const res = await Promise.race([
      api.get('/api/user/2fa/status', {
        signal: controller.signal,
        timeout: 15000,
        disableDuplicate: true,
        skipAuthRefresh: true,
        skipErrorHandler: true,
        skipBusinessError: true,
      }),
      cancelled,
    ])
    return res.data
  } finally {
    globalThis.clearTimeout(deadline)
    signal?.removeEventListener('abort', cancel)
  }
}

export async function setupTwoFA(
  signal?: AbortSignal
): Promise<ApiResponse<TwoFASetupData>> {
  const res = await api.post<unknown>('/api/user/2fa/setup', undefined, {
    signal,
    skipAuthRefresh: true,
    skipErrorHandler: true,
    skipBusinessError: true,
  })
  const payload = res.data
  if (
    typeof payload !== 'object' ||
    payload === null ||
    !('success' in payload) ||
    typeof payload.success !== 'boolean'
  ) {
    throw new Error('Invalid 2FA setup response')
  }

  const message = 'message' in payload ? payload.message : undefined
  if (message !== undefined && typeof message !== 'string') {
    throw new Error('Invalid 2FA setup response')
  }
  if (!payload.success) {
    return {
      success: false,
      ...(message === undefined ? {} : { message }),
    }
  }

  const data = 'data' in payload ? payload.data : undefined
  if (
    typeof data !== 'object' ||
    data === null ||
    !('secret' in data) ||
    typeof data.secret !== 'string' ||
    !('qr_code_data' in data) ||
    typeof data.qr_code_data !== 'string' ||
    !('backup_codes' in data) ||
    !Array.isArray(data.backup_codes) ||
    !data.backup_codes.every(
      (code: unknown): code is string => typeof code === 'string'
    )
  ) {
    throw new Error('Invalid 2FA setup response')
  }

  return {
    success: true,
    ...(message === undefined ? {} : { message }),
    data: {
      secret: data.secret,
      qr_code_data: data.qr_code_data,
      backup_codes: data.backup_codes,
    },
  }
}

export async function enableTwoFA(
  code: string,
  signal?: AbortSignal
): Promise<ApiResponse> {
  const res = await api.post(
    '/api/user/2fa/enable',
    { code },
    {
      signal,
      acceptAuthRotation: true,
      skipAuthRefresh: true,
      skipErrorHandler: true,
      skipBusinessError: true,
    }
  )
  return res.data
}

export async function disableTwoFA(
  code: string,
  signal?: AbortSignal
): Promise<ApiResponse> {
  const res = await api.post(
    '/api/user/2fa/disable',
    { code },
    {
      signal,
      acceptAuthRotation: true,
      skipAuthRefresh: true,
      skipErrorHandler: true,
      skipBusinessError: true,
    }
  )
  return res.data
}

export async function regenerateTwoFABackupCodes(
  code: string,
  signal?: AbortSignal
): Promise<ApiResponse<{ backup_codes: string[] }>> {
  const res = await api.post(
    '/api/user/2fa/backup_codes',
    { code },
    {
      signal,
      acceptAuthRotation: true,
      skipAuthRefresh: true,
      skipErrorHandler: true,
      skipBusinessError: true,
    }
  )
  return res.data
}

// ============================================================================
// Passkey APIs
// ============================================================================

function securityProofHeaders(
  proofToken?: string
): Record<string, string> | undefined {
  return proofToken ? { 'X-Security-Proof': proofToken } : undefined
}

export async function getProfilePasskeyStatus(
  signal?: AbortSignal
): Promise<ApiResponse<PasskeyStatus>> {
  if (signal?.aborted) throw new Error('Passkey status request cancelled')

  const controller = new AbortController()
  const cancel = () => controller.abort()
  const cancelled = new Promise<never>((_, reject) => {
    controller.signal.addEventListener(
      'abort',
      () => reject(new Error('Passkey status request cancelled')),
      { once: true }
    )
  })
  signal?.addEventListener('abort', cancel, { once: true })
  const deadline = globalThis.setTimeout(cancel, 15000)
  try {
    const res = await Promise.race([
      api.get<ApiResponse<PasskeyStatus>>('/api/user/passkey', {
        signal: controller.signal,
        timeout: 15000,
        disableDuplicate: true,
        skipAuthRefresh: true,
        skipErrorHandler: true,
        skipBusinessError: true,
      }),
      cancelled,
    ])
    return res.data
  } finally {
    globalThis.clearTimeout(deadline)
    signal?.removeEventListener('abort', cancel)
  }
}

export async function beginProfilePasskeyRegistration(
  proofToken?: string,
  signal?: AbortSignal
): Promise<ApiResponse<PasskeyOptionsPayload>> {
  const res = await api.post<ApiResponse<PasskeyOptionsPayload>>(
    '/api/user/passkey/register/begin',
    undefined,
    {
      headers: securityProofHeaders(proofToken),
      signal,
      skipAuthRefresh: true,
      skipErrorHandler: true,
      skipBusinessError: true,
    }
  )
  return res.data
}

export async function finishProfilePasskeyRegistration(
  flowToken: string,
  payload: Record<string, unknown>,
  proofToken?: string,
  signal?: AbortSignal
): Promise<ApiResponse> {
  const res = await api.post<ApiResponse>(
    '/api/user/passkey/register/finish',
    { flow_token: flowToken, credential: payload },
    {
      headers: securityProofHeaders(proofToken),
      signal,
      acceptAuthRotation: true,
      skipAuthRefresh: true,
      skipErrorHandler: true,
      skipBusinessError: true,
    }
  )
  return res.data
}

export async function deleteProfilePasskey(
  proofToken?: string,
  signal?: AbortSignal
): Promise<ApiResponse> {
  const res = await api.delete<ApiResponse>('/api/user/passkey', {
    headers: securityProofHeaders(proofToken),
    signal,
    acceptAuthRotation: true,
    skipAuthRefresh: true,
    skipErrorHandler: true,
    skipBusinessError: true,
  })
  return res.data
}

// ============================================================================
// Custom OAuth Binding APIs
// ============================================================================

/**
 * Get current user's custom OAuth bindings
 */
export async function getSelfOAuthBindings(
  signal?: AbortSignal
): Promise<ApiResponse<CustomOAuthBinding[]>> {
  const res = await api.get('/api/user/oauth/bindings', {
    signal,
    disableDuplicate: true,
    skipAuthRefresh: true,
    skipErrorHandler: true,
    skipBusinessError: true,
  })
  return res.data
}

/**
 * Unbind a custom OAuth provider for current user
 */
export async function unbindCustomOAuth(
  providerId: number,
  signal?: AbortSignal
): Promise<ApiResponse> {
  const res = await api.delete(`/api/user/oauth/bindings/${providerId}`, {
    signal,
    skipAuthRefresh: true,
    skipErrorHandler: true,
    skipBusinessError: true,
  })
  return res.data
}

// ============================================================================
// Checkin APIs
// ============================================================================

/**
 * Get checkin status for a specific month
 */
export async function getCheckinStatus(
  month: string
): Promise<ApiResponse<CheckinStatusResponse>> {
  const res = await api.get(`/api/user/checkin?month=${month}`)
  return res.data
}

/**
 * Perform daily checkin
 */
export async function performCheckin(
  turnstileToken?: string
): Promise<ApiResponse<CheckinResponse>> {
  const url = turnstileToken
    ? `/api/user/checkin?turnstile=${encodeURIComponent(turnstileToken)}`
    : '/api/user/checkin'
  const res = await api.post(url)
  return res.data
}
