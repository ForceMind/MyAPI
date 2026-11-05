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
import axios, { type AxiosRequestConfig } from 'axios'
import { t } from 'i18next'
import { toast } from 'sonner'

import {
  applyAuthRotation,
  clearAuthentication,
  refreshAuthentication,
} from '@/lib/auth-session'
import { getServerErrorMessageKey } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

declare module 'axios' {
  export interface AxiosRequestConfig {
    skipBusinessError?: boolean
    skipErrorHandler?: boolean
    disableDuplicate?: boolean
    skipAuthRefresh?: boolean
    authRetry?: boolean
    acceptAuthRotation?: boolean
    /** Opt-in UI lifetime/actor guard. Unscoped callers keep existing behavior. */
    isCurrentScope?: () => boolean
  }
}

export type ApiRequestConfig = AxiosRequestConfig

export const api = axios.create({
  baseURL: '',
  withCredentials: true,
  headers: {
    'Cache-Control': 'no-store',
  },
})

const inFlightGet = new Map<string, Promise<unknown>>()
const originalGet = api.get.bind(api)

api.get = ((url: string, config: ApiRequestConfig = {}) => {
  if (config.disableDuplicate || config.isCurrentScope) return originalGet(url, config)

  const params = config.params ? JSON.stringify(config.params) : '{}'
  const sessionSID = useAuthStore.getState().auth.session?.sid || 'anonymous'
  const key = `${sessionSID}:${url}?${params}`
  const existingRequest = inFlightGet.get(key)
  if (existingRequest) return existingRequest

  const request = originalGet(url, config).finally(() => {
    inFlightGet.delete(key)
  })
  inFlightGet.set(key, request)
  return request
}) as typeof api.get

function redirectToSignIn(): void {
  if (
    typeof window !== 'undefined' &&
    window.location.pathname !== '/sign-in'
  ) {
    window.location.replace('/sign-in')
  }
}

api.interceptors.response.use(
  (response) => {
    if (response.config.isCurrentScope && !response.config.isCurrentScope()) {
      throw new axios.CanceledError('Request scope changed', response.config)
    }
    if (response.config.acceptAuthRotation && response.data?.success === true) {
      applyAuthRotation(response.data.data)
    }

    if (
      !response.config.skipBusinessError &&
      typeof response.data?.success === 'boolean' &&
      !response.data.success
    ) {
      const messageKey = getServerErrorMessageKey(response.data)
      toast.error(
        messageKey
          ? t(messageKey)
          : response.data.message || t('Request failed')
      )
    }
    return response
  },
  async (error) => {
    // Canceling an observer is not a service failure. Keep the rejection intact.
    if (axios.isCancel(error)) throw error

    const config = error?.config as ApiRequestConfig | undefined
    if (config?.isCurrentScope && !config.isCurrentScope()) {
      throw new axios.CanceledError('Request scope changed', error.config)
    }
    const skipErrorHandler = config?.skipErrorHandler
    const status = error?.response?.status

    if (status === 401) {
      if (config && !config.skipAuthRefresh && !config.authRetry) {
        config.authRetry = true
        const scopePath = typeof window === 'undefined' ? undefined : window.location.pathname
        const outcome = await refreshAuthentication()
        if (config.isCurrentScope && !config.isCurrentScope()) {
          const current = useAuthStore.getState().auth
          const expiredHere = (outcome.kind === 'anonymous' || outcome.kind === 'out_of_sync') &&
            !current.user && !current.session && !current.accessToken &&
            (typeof window === 'undefined' || window.location.pathname === scopePath)
          // A failed refresh legitimately clears this session. Preserve its login recovery,
          // while never redirecting a newer actor or a different page for an old task.
          if (!expiredHere) throw new axios.CanceledError('Request scope changed', error.config)
        }
        if (outcome.kind === 'authenticated') {
          const token = useAuthStore.getState().auth.accessToken
          if (token) {
            config.headers = {
              ...config.headers,
              Authorization: `Bearer ${token}`,
            }
          }
          return api.request(config)
        }

        if (outcome.kind === 'anonymous' || outcome.kind === 'out_of_sync') {
          if (!skipErrorHandler) toast.error(t('Session expired!'))
          redirectToSignIn()
        }
      } else if (config?.authRetry) {
        clearAuthentication(false)
        if (!skipErrorHandler) toast.error(t('Session expired!'))
        redirectToSignIn()
      } else if (!skipErrorHandler) {
        toast.error(t('Session expired!'))
      }
    } else if (!skipErrorHandler) {
      const messageKey = getServerErrorMessageKey(error)
      const message = messageKey
        ? t(messageKey)
        : error?.response?.data?.message ||
          error?.message ||
          t('Request failed')
      toast.error(message)
    }
    throw error
  }
)

api.interceptors.request.use(
  (config) => {
    if (config.isCurrentScope && !config.isCurrentScope()) {
      throw new axios.CanceledError('Request scope changed', config)
    }
    const accessToken = useAuthStore.getState().auth.accessToken
    if (accessToken) {
      config.headers.Authorization = `Bearer ${accessToken}`
    }
    return config
  },
  (error) => { throw error },
  { synchronous: true }
)
