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
import { useState, useEffect, useCallback, useRef } from 'react'
import { toast } from 'sonner'

import { getUserProfile, updateUserProfile, updateUserSettings } from '../api'
import type {
  UserProfile,
  UpdateUserRequest,
  UpdateUserSettingsRequest,
} from '../types'

// ============================================================================
// Profile Hook
// ============================================================================

export function useProfile(enabled = true) {
  const [profile, setProfile] = useState<UserProfile | null>(null)
  const [loading, setLoading] = useState(true)
  const [updating, setUpdating] = useState(false)
  const [error, setError] = useState(false)
  const pendingRequest = useRef<AbortController | null>(null)

  // Fetch user profile (with optional silent mode)
  const fetchProfile = useCallback(async (silent = false) => {
    pendingRequest.current?.abort()
    const controller = new AbortController()
    pendingRequest.current = controller
    try {
      if (!silent) {
        setLoading(true)
      }
      const response = await getUserProfile(controller.signal)
      if (controller.signal.aborted) return

      if (response.success && response.data) {
        setProfile(response.data)
        setError(false)
      } else {
        if (!silent) setError(true)
      }
    } catch {
      if (controller.signal.aborted) return
      if (!silent) setError(true)
    } finally {
      if (!controller.signal.aborted) {
        setLoading(false)
      }
    }
  }, [])

  // Refresh profile silently (without loading state)
  const refreshProfile = useCallback(async () => {
    await fetchProfile(true)
  }, [fetchProfile])

  // Update user profile
  const updateProfile = useCallback(
    async (data: UpdateUserRequest): Promise<boolean> => {
      try {
        setUpdating(true)
        const response = await updateUserProfile(data)

        if (response.success) {
          toast.success(i18next.t('Profile updated successfully'))
          await refreshProfile() // Refresh profile silently
          return true
        }

        toast.error(response.message || i18next.t('Failed to update profile'))
        return false
      } catch (error) {
        // eslint-disable-next-line no-console
        console.error('Failed to update profile:', error)
        toast.error(i18next.t('Failed to update profile'))
        return false
      } finally {
        setUpdating(false)
      }
    },
    [refreshProfile]
  )

  // Update user settings
  const updateSettings = useCallback(
    async (data: UpdateUserSettingsRequest): Promise<boolean> => {
      try {
        setUpdating(true)
        const response = await updateUserSettings(data)

        if (response.success) {
          toast.success(i18next.t('Settings updated successfully'))
          await refreshProfile() // Refresh profile silently
          return true
        }

        toast.error(response.message || i18next.t('Failed to update settings'))
        return false
      } catch (error) {
        // eslint-disable-next-line no-console
        console.error('Failed to update settings:', error)
        toast.error(i18next.t('Failed to update settings'))
        return false
      } finally {
        setUpdating(false)
      }
    },
    [refreshProfile]
  )

  // The initial funding read can replace the whole page shell. Start the
  // profile read after that decision so the temporary shell does not send an
  // immediately cancelled duplicate. Keep the wait bounded if status stalls.
  useEffect(() => {
    if (!enabled) {
      const deadline = globalThis.setTimeout(() => {
        setLoading(false)
        setError(true)
      }, 15000)
      return () => {
        globalThis.clearTimeout(deadline)
        pendingRequest.current?.abort()
      }
    }
    void fetchProfile()
    return () => pendingRequest.current?.abort()
  }, [enabled, fetchProfile])

  return {
    profile,
    loading,
    updating,
    error,
    fetchProfile,
    refreshProfile,
    updateProfile,
    updateSettings,
  }
}
