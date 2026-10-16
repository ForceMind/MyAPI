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
import { useState, useCallback } from 'react'
import { toast } from 'sonner'

import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'

import { generateAccessToken } from '../api'
import { useSecurityRequestScope } from './use-security-request-scope'

// ============================================================================
// Access Token Hook
// ============================================================================

export function useAccessToken(securityScopeKey: string) {
  const [token, setToken] = useState<string>('')
  const [generating, setGenerating] = useState(false)
  const { copyToClipboard } = useCopyToClipboard({ notify: false })
  const requestScope = useSecurityRequestScope(securityScopeKey)

  // Generate new access token
  const generate = useCallback(async (): Promise<boolean> => {
    let requestCurrent = true
    try {
      setGenerating(true)
      const result = await requestScope.run((signal) =>
        generateAccessToken(signal)
      )
      requestCurrent = result.current
      if (!result.current) return false
      const response = result.value

      if (response.success && response.data) {
        setToken(response.data)
        copyToClipboard(response.data)
        toast.success(i18next.t('Token regenerated and copied to clipboard'))
        return true
      }

      toast.error(response.message || i18next.t('Failed to generate token'))
      return false
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to generate token:', error)
      toast.error(
        i18next.t(
          'The result could not be confirmed. Your previous access token may already be invalid. Regenerate again only when you are ready to replace it.'
        )
      )
      return false
    } finally {
      if (requestCurrent) setGenerating(false)
    }
  }, [copyToClipboard, requestScope])

  const clearToken = useCallback(() => {
    setToken('')
  }, [])

  const cancel = useCallback(() => {
    requestScope.cancel()
    setGenerating(false)
  }, [requestScope])

  return {
    token,
    generating,
    generate,
    clearToken,
    cancel,
  }
}
