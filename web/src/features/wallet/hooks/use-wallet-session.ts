import i18next from 'i18next'
import { isAxiosError } from 'axios'
import { useCallback, useLayoutEffect, useRef } from 'react'

import { useAuthStore } from '@/stores/auth-store'
import type { ApiRequestConfig } from '@/lib/api'
import { getServerErrorMessageKey } from '@/lib/server-error-message'
import { isUserFundingUnavailableError } from '@/lib/self-use-build'

export function walletRequestConfig(isCurrent: () => boolean): ApiRequestConfig {
  return { skipErrorHandler: true, skipBusinessError: true, disableDuplicate: true, isCurrentScope: isCurrent }
}

/** A wallet task keeps its original actor and lifetime, including across awaits. */
export function useWalletSession() {
  const auth = useAuthStore.getState().auth
  const original = useRef([auth.user?.id, auth.user?.role, auth.session?.sid ?? null].join(':'))
  const alive = useRef(true)
  useLayoutEffect(() => {
    alive.current = true
    return () => { alive.current = false }
  }, [])
  return useCallback(() => {
    const current = useAuthStore.getState().auth
    return alive.current && original.current === [current.user?.id, current.user?.role, current.session?.sid ?? null].join(':')
  }, [])
}


/** A transport loss, timeout or server fault cannot prove that a write was rejected. */
export function walletWriteIsUnknown(error: unknown): boolean {
  if (isUserFundingUnavailableError(error)) return false
  if (isAxiosError(error) && error.response) {
    return error.response.status === 408 || error.response.status >= 500
  }
  return true
}


/** Current-scope presentation keeps the server's public message without exposing a response body. */
export function walletWriteErrorMessage(error: unknown, fallback: string): string {
  const messageKey = getServerErrorMessageKey(error)
  if (messageKey) return i18next.t(messageKey)
  if (isAxiosError(error)) {
    if (error.response?.status === 401) return i18next.t('Session expired!')
    const data: unknown = error.response?.data
    if (data && typeof data === 'object' && 'message' in data && typeof data.message === 'string' && data.message.trim()) return data.message
    if (error.response?.status === 403) return i18next.t('Unauthorized')
  }
  return fallback
}


// Session-memory safety markers contain only actor/session and public operation IDs.
// A history read cannot prove the outcome of an uncorrelated order, so it never clears them.
const unconfirmedPayments = new Map<string, Set<string>>()

export function walletPaymentScope(): string | null {
  const auth = useAuthStore.getState().auth
  return auth.user ? `${auth.user.id}:${auth.session?.sid ?? 'legacy-session'}` : null
}

export function hasUnconfirmedWalletPayment(operation: string, owner = walletPaymentScope()): boolean {
  return owner !== null && unconfirmedPayments.get(owner)?.has(operation) === true
}

export function retainUnconfirmedWalletPayment(operation: string, owner = walletPaymentScope()): void {
  if (owner === null) return
  const entries = unconfirmedPayments.get(owner) ?? new Set<string>()
  entries.add(operation)
  unconfirmedPayments.set(owner, entries)
}
