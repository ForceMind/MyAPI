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
import i18next from 'i18next'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { toast } from 'sonner'

import type { PasskeyStatus } from '@/features/auth/passkey'
import {
  buildRegistrationResult,
  createCredential,
  isPasskeySupported as detectPasskeySupport,
  prepareCredentialCreationOptions,
} from '@/lib/passkey'

import {
  beginProfilePasskeyRegistration,
  deleteProfilePasskey,
  finishProfilePasskeyRegistration,
  getProfilePasskeyStatus,
} from '../api'
import { useSecurityRequestScope } from './use-security-request-scope'

function parsePasskeyStatus(value: unknown): PasskeyStatus | null {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    return null
  }
  const candidate = value as Record<string, unknown>
  if (typeof candidate.enabled !== 'boolean') return null
  if (
    candidate.last_used_at !== undefined &&
    candidate.last_used_at !== null &&
    typeof candidate.last_used_at !== 'string'
  ) {
    return null
  }
  if (
    candidate.backup_eligible !== undefined &&
    typeof candidate.backup_eligible !== 'boolean'
  ) {
    return null
  }
  if (
    candidate.backup_state !== undefined &&
    typeof candidate.backup_state !== 'boolean'
  ) {
    return null
  }
  return candidate as unknown as PasskeyStatus
}

export function useProfilePasskey(securityScopeKey: string) {
  const [status, setStatus] = useState<PasskeyStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [registering, setRegistering] = useState(false)
  const [removing, setRemoving] = useState(false)
  const [supported, setSupported] = useState(false)
  const statusScope = useSecurityRequestScope(securityScopeKey)
  const mutationScope = useSecurityRequestScope(securityScopeKey)

  const fetchStatus = useCallback(async () => {
    let requestCurrent = true
    setLoading(true)
    setError(false)
    try {
      const result = await statusScope.run((signal) =>
        getProfilePasskeyStatus(signal)
      )
      requestCurrent = result.current
      if (!result.current) return
      const nextStatus = parsePasskeyStatus(result.value.data)
      if (result.value.success !== true || !nextStatus) {
        throw new Error(
          result.value.message || 'Invalid Passkey status response'
        )
      }
      setStatus(nextStatus)
    } catch (statusError) {
      // eslint-disable-next-line no-console
      console.error('[Passkey] Failed to fetch status', statusError)
      setStatus(null)
      setError(true)
    } finally {
      if (requestCurrent) setLoading(false)
    }
  }, [statusScope])

  useEffect(() => {
    void fetchStatus()
  }, [fetchStatus])

  useEffect(() => {
    let active = true
    detectPasskeySupport()
      .then((value) => {
        if (active) setSupported(value)
      })
      .catch(() => {
        if (active) setSupported(false)
      })
    return () => {
      active = false
    }
  }, [])

  const register = useCallback(
    async (proofToken?: string) => {
      if (!supported || !navigator?.credentials) {
        toast.info(i18next.t('This device does not support Passkey'))
        return false
      }
      setRegistering(true)
      let requestCurrent = true
      try {
        const result = await mutationScope.run(async (signal) => {
          const begin = await beginProfilePasskeyRegistration(
            proofToken,
            signal
          )
          if (!begin.success) {
            throw new Error(
              begin.message || i18next.t('Failed to start Passkey registration')
            )
          }
          const flowToken = begin.data?.flow_token
          if (!flowToken) {
            throw new Error(
              i18next.t('Registration flow expired. Please try again.')
            )
          }
          const publicKey = prepareCredentialCreationOptions(
            begin.data?.options ?? begin.data
          )
          const credential = (await createCredential(
            publicKey
          )) as PublicKeyCredential | null
          if (signal.aborted) return false
          if (!credential) {
            throw new DOMException(
              'Passkey registration cancelled',
              'NotAllowedError'
            )
          }
          const attestation = buildRegistrationResult(credential)
          if (!attestation) {
            throw new Error(i18next.t('Invalid Passkey registration response'))
          }
          const finish = await finishProfilePasskeyRegistration(
            flowToken,
            attestation,
            proofToken,
            signal
          )
          if (!finish.success) {
            throw new Error(
              finish.message || i18next.t('Failed to register Passkey')
            )
          }
          return true
        })
        requestCurrent = result.current
        if (!result.current || !result.value) return false
        toast.success(i18next.t('Passkey registered successfully'))
        await fetchStatus()
        return true
      } catch (registerError) {
        if (
          registerError instanceof DOMException &&
          registerError.name === 'NotAllowedError'
        ) {
          toast.info(i18next.t('Passkey registration was cancelled'))
          return false
        }
        toast.error(
          i18next.t(
            'The result could not be confirmed. Verify the current account state before trying again.'
          )
        )
        await fetchStatus()
        return false
      } finally {
        if (requestCurrent) setRegistering(false)
      }
    },
    [fetchStatus, mutationScope, supported]
  )

  const remove = useCallback(
    async (proofToken?: string) => {
      setRemoving(true)
      let requestCurrent = true
      try {
        const result = await mutationScope.run((signal) =>
          deleteProfilePasskey(proofToken, signal)
        )
        requestCurrent = result.current
        if (!result.current) return false
        if (!result.value.success) {
          toast.error(
            result.value.message || i18next.t('Failed to remove Passkey')
          )
          return false
        }
        toast.success(i18next.t('Passkey removed successfully'))
        await fetchStatus()
        return true
      } catch {
        toast.error(
          i18next.t(
            'The result could not be confirmed. Verify the current account state before trying again.'
          )
        )
        await fetchStatus()
        return false
      } finally {
        if (requestCurrent) setRemoving(false)
      }
    },
    [fetchStatus, mutationScope]
  )

  return {
    status,
    loading,
    error,
    registering,
    removing,
    supported,
    enabled: useMemo(() => Boolean(status?.enabled), [status]),
    lastUsed: useMemo(() => status?.last_used_at ?? null, [status]),
    fetchStatus,
    register,
    remove,
  }
}
