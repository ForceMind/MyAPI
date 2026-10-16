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
import { useState, useEffect, useCallback, useRef } from 'react'

import { getTwoFAStatus } from '../api'
import type { TwoFAStatus } from '../types'

// ============================================================================
// Two-FA Hook
// ============================================================================

function parseTwoFAStatus(value: unknown): TwoFAStatus | null {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    return null
  }

  const candidate = value as Record<string, unknown>
  if (
    typeof candidate.enabled !== 'boolean' ||
    typeof candidate.locked !== 'boolean'
  ) {
    return null
  }

  const backupCodes = candidate.backup_codes_remaining
  let backupCodesRemaining = 0
  if (backupCodes !== undefined) {
    if (
      typeof backupCodes !== 'number' ||
      !Number.isInteger(backupCodes) ||
      backupCodes < 0
    ) {
      return null
    }
    backupCodesRemaining = backupCodes
  }
  if (candidate.enabled && backupCodes === undefined) return null

  return {
    enabled: candidate.enabled,
    locked: candidate.locked,
    backup_codes_remaining: backupCodesRemaining,
  }
}

export function useTwoFA(enabled = true) {
  const [loading, setLoading] = useState(true)
  const [status, setStatus] = useState<TwoFAStatus | null>(null)
  const [error, setError] = useState(false)
  const pendingRequest = useRef<AbortController | null>(null)

  const fetchStatus = useCallback(async () => {
    if (!enabled) return

    pendingRequest.current?.abort()
    const controller = new AbortController()
    pendingRequest.current = controller
    try {
      setLoading(true)
      setError(false)
      const response = await getTwoFAStatus(controller.signal)
      if (controller.signal.aborted) return

      const nextStatus = parseTwoFAStatus(response?.data)
      if (response?.success !== true || !nextStatus) {
        throw new Error('Invalid 2FA status response')
      }

      setStatus(nextStatus)
    } catch (error) {
      if (controller.signal.aborted) return
      // eslint-disable-next-line no-console
      console.error('Failed to fetch 2FA status:', error)
      setStatus(null)
      setError(true)
    } finally {
      if (pendingRequest.current === controller && !controller.signal.aborted) {
        setLoading(false)
      }
    }
  }, [enabled])

  useEffect(() => {
    fetchStatus()
    return () => pendingRequest.current?.abort()
  }, [fetchStatus])

  return {
    status,
    loading,
    error,
    refetch: fetchStatus,
  }
}
