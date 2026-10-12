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
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { toast } from 'sonner'

import {
  buildRegistrationResult,
  createCredential,
  isPasskeySupported as detectPasskeySupport,
  prepareCredentialCreationOptions,
} from '@/lib/passkey'

import {
  beginPasskeyRegistration,
  deletePasskey,
  finishPasskeyRegistration,
  getPasskeyStatus,
} from '../api'
import type { PasskeyStatus } from '../types'

interface UsePasskeyManagementOptions {
  onStatusChange?: (status: PasskeyStatus | null) => void
}

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

  return {
    ...candidate,
    enabled: candidate.enabled,
    last_used_at: candidate.last_used_at as string | null | undefined,
    backup_eligible: candidate.backup_eligible as boolean | undefined,
    backup_state: candidate.backup_state as boolean | undefined,
  }
}

export function usePasskeyManagement(
  options: UsePasskeyManagementOptions = {}
) {
  const { onStatusChange } = options

  const [status, setStatus] = useState<PasskeyStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [registering, setRegistering] = useState(false)
  const [removing, setRemoving] = useState(false)
  const [supported, setSupported] = useState(false)
  const pendingRequest = useRef<AbortController | null>(null)

  const fetchStatus = useCallback(async () => {
    pendingRequest.current?.abort()
    const controller = new AbortController()
    pendingRequest.current = controller
    try {
      setLoading(true)
      setError(false)
      const res = await getPasskeyStatus(controller.signal)
      if (controller.signal.aborted) return

      const nextStatus = parsePasskeyStatus(res.data)
      if (res.success !== true || !nextStatus) {
        throw new Error(res.message || 'Invalid Passkey status response')
      }

      setStatus(nextStatus)
      onStatusChange?.(nextStatus)
    } catch (error) {
      if (controller.signal.aborted) return
      // eslint-disable-next-line no-console
      console.error('[Passkey] Failed to fetch status', error)
      setStatus(null)
      setError(true)
    } finally {
      if (
        pendingRequest.current === controller &&
        !controller.signal.aborted
      ) {
        setLoading(false)
      }
    }
  }, [onStatusChange])

  useEffect(() => {
    fetchStatus()
    return () => pendingRequest.current?.abort()
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
      if (!supported) {
        toast.error(i18next.t('This device does not support Passkey'))
        return false
      }
      if (!navigator?.credentials) {
        toast.error(i18next.t('Passkey is not supported in this environment'))
        return false
      }

      setRegistering(true)
      try {
        const beginResponse = await beginPasskeyRegistration(proofToken)
        if (!beginResponse.success) {
          toast.error(
            beginResponse.message ||
              i18next.t('Failed to start Passkey registration')
          )
          return false
        }

        const publicKey = prepareCredentialCreationOptions(
          beginResponse.data?.options ?? beginResponse.data
        )
        const flowToken = beginResponse.data?.flow_token
        if (!flowToken) {
          toast.error(i18next.t('Registration flow expired. Please try again.'))
          return false
        }

        const credential = (await createCredential(
          publicKey
        )) as PublicKeyCredential | null
        if (!credential) {
          toast.error(i18next.t('Passkey registration was cancelled'))
          return false
        }

        const attestation = buildRegistrationResult(credential)
        if (!attestation) {
          toast.error(i18next.t('Invalid Passkey registration response'))
          return false
        }

        const finishResponse = await finishPasskeyRegistration(
          flowToken,
          attestation,
          proofToken
        )
        if (!finishResponse.success) {
          toast.error(
            finishResponse.message || i18next.t('Failed to register Passkey')
          )
          return false
        }

        toast.success(i18next.t('Passkey registered successfully'))
        await fetchStatus()
        return true
      } catch (error: unknown) {
        if (error instanceof DOMException && error.name === 'NotAllowedError') {
          toast.info(i18next.t('Passkey registration was cancelled'))
          return false
        }
        // eslint-disable-next-line no-console
        console.error('[Passkey] Registration error', error)
        toast.error(
          error instanceof Error
            ? error.message
            : i18next.t('Failed to register Passkey')
        )
        return false
      } finally {
        setRegistering(false)
      }
    },
    [supported, fetchStatus]
  )

  const remove = useCallback(
    async (proofToken?: string) => {
      setRemoving(true)
      try {
        const res = await deletePasskey(proofToken)
        if (!res.success) {
          toast.error(res.message || i18next.t('Failed to remove Passkey'))
          return false
        }

        toast.success(i18next.t('Passkey removed successfully'))
        await fetchStatus()
        return true
      } catch (error) {
        // eslint-disable-next-line no-console
        console.error('[Passkey] Removal error', error)
        toast.error(i18next.t('Failed to remove Passkey'))
        return false
      } finally {
        setRemoving(false)
      }
    },
    [fetchStatus]
  )

  const enabled = useMemo(() => Boolean(status?.enabled), [status])
  const lastUsed = useMemo(() => status?.last_used_at ?? null, [status])

  return {
    status,
    loading,
    error,
    registering,
    removing,
    supported,
    enabled,
    lastUsed,
    fetchStatus,
    register,
    remove,
  }
}
