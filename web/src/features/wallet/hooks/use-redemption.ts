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
import { useState, useCallback, useRef } from 'react'
import { toast } from 'sonner'

import { formatQuota } from '@/lib/format'
import {
  isUserFundingUnavailableError,
  USER_FUNDING_UNAVAILABLE_MESSAGE_KEY,
} from '@/lib/self-use-build'

import { redeemTopupCode, getWalletAccount } from '../api'
import { useWalletSession, walletRequestConfig, walletWriteIsUnknown, walletWriteErrorMessage } from './use-wallet-session'

// ============================================================================
// Redemption Hook
// ============================================================================

export function useRedemption() {
  const isCurrent = useWalletSession()
  const writeFlight = useRef(false)
  const unconfirmedCode = useRef<string | null>(null)
  const [outcomeUnknown, setOutcomeUnknown] = useState(false)
  const [redeeming, setRedeeming] = useState(false)

  const redeemCode = useCallback(async (code: string): Promise<boolean> => {
    if (!isCurrent() || writeFlight.current) return false
    if (unconfirmedCode.current !== null && unconfirmedCode.current !== code) return false
    if (!code || code.trim() === '') {
      toast.error(i18next.t('Please enter a redemption code'))
      return false
    }

    writeFlight.current = true
    try {
      setRedeeming(true)
      const response = await redeemTopupCode({ key: code }, walletRequestConfig(isCurrent))

      if (!isCurrent()) return false
      if (response.success && response.data) {
        unconfirmedCode.current = null
        setOutcomeUnknown(false)
        const quotaAdded = response.data
        toast.success(
          i18next.t('Redemption successful! Added: {{quota}}', {
            quota: formatQuota(quotaAdded),
          })
        )
        try {
          await getWalletAccount(walletRequestConfig(isCurrent))
        } catch {
          // The write was acknowledged. The wallet owns visible snapshot recovery.
        }
        return isCurrent()
      }

      toast.error(
        isUserFundingUnavailableError(response)
          ? i18next.t(USER_FUNDING_UNAVAILABLE_MESSAGE_KEY)
          : response.message || i18next.t('Redemption failed')
      )
      return false
    } catch (error) {
      if (!isCurrent()) return false
      const unknown = walletWriteIsUnknown(error)
      if (unknown) unconfirmedCode.current = code
      setOutcomeUnknown(unknown || unconfirmedCode.current !== null)
      let message = i18next.t('Redemption failed')
      if (isUserFundingUnavailableError(error)) message = i18next.t(USER_FUNDING_UNAVAILABLE_MESSAGE_KEY)
      if (unknown) message = i18next.t('Funding request outcome is unknown. Check current records and balances before retrying.')
      toast.error(walletWriteErrorMessage(error, message))
      return false
    } finally {
      writeFlight.current = false
      if (isCurrent()) setRedeeming(false)
    }
  }, [isCurrent])

  return {
    outcomeUnknown,
    redeeming,
    redeemCode,
  }
}
