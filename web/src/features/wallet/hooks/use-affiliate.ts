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

import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import {
  isUserFundingUnavailableError,
  USER_FUNDING_UNAVAILABLE_MESSAGE_KEY,
} from '@/lib/self-use-build'

import { getAffiliateCode, getWalletAccount, transferAffiliateQuota } from '../api'
import { generateAffiliateLink } from '../lib'
import { useWalletSession, walletRequestConfig, walletWriteIsUnknown, walletWriteErrorMessage } from './use-wallet-session'

// ============================================================================
// Affiliate Hook
// ============================================================================

export function useAffiliate() {
  const isCurrent = useWalletSession()
  const writeFlight = useRef(false)
  const unknownTransfer = useRef(false)
  const [outcomeUnknown, setOutcomeUnknown] = useState(false)
  const [affiliateCode, setAffiliateCode] = useState<string>('')
  const [affiliateLink, setAffiliateLink] = useState<string>('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [transferring, setTransferring] = useState(false)
  const { copyToClipboard } = useCopyToClipboard()
  // Idempotency key of the in-flight transfer attempt. Generated once per
  // attempt, reused by retries of that same attempt, and cleared on success
  // so the next attempt mints a fresh id.
  const transferAttemptRef = useRef<{
    quota: number
    requestId: string
  } | null>(null)

  // Fetch affiliate code
  const fetchAffiliateCode = useCallback(async () => {
    if (!isCurrent()) return
    try {
      setLoading(true)
      setError(null)
      const response = await getAffiliateCode(walletRequestConfig(isCurrent))

      if (!isCurrent()) return
      if (response.success && response.data) {
        setAffiliateCode(response.data)
        const link = generateAffiliateLink(response.data)
        setAffiliateLink(link)
      } else {
        setAffiliateCode('')
        setAffiliateLink('')
        setError(response.message || i18next.t('Referral information could not be read. Please retry.'))
      }
    } catch (error) {
      if (!isCurrent()) return
      setAffiliateCode('')
      setAffiliateLink('')
      setError(walletWriteErrorMessage(error, i18next.t('Referral information could not be read. Please retry.')))
    } finally {
      if (isCurrent()) setLoading(false)
    }
  }, [isCurrent])

  // Copy affiliate link
  const copyAffiliateLink = useCallback(() => {
    if (isCurrent()) copyToClipboard(affiliateLink)
  }, [affiliateLink, copyToClipboard, isCurrent])

  // Transfer affiliate quota to balance
  const transferQuota = useCallback(async (quota: number, taskCurrent?: () => boolean): Promise<boolean> => {
    const owns = () => isCurrent() && (taskCurrent?.() ?? true)
    if (!owns() || writeFlight.current) return false
    if (unknownTransfer.current && transferAttemptRef.current?.quota !== quota) return false
    if (
      !transferAttemptRef.current ||
      transferAttemptRef.current.quota !== quota
    ) {
      transferAttemptRef.current = { quota, requestId: crypto.randomUUID() }
    }
    const requestId = transferAttemptRef.current.requestId
    writeFlight.current = true
    try {
      setTransferring(true)
      const response = await transferAffiliateQuota({
        quota,
        request_id: requestId,
      }, walletRequestConfig(owns))

      if (!owns()) return false
      if (response.success) {
        unknownTransfer.current = false
        setOutcomeUnknown(false)
        transferAttemptRef.current = null
        toast.success(response.message || i18next.t('Transfer successful'))
        try {
          await getWalletAccount(walletRequestConfig(owns))
        } catch {
          // The write was acknowledged. The wallet owns visible snapshot recovery.
        }
        return owns()
      }

      toast.error(
        isUserFundingUnavailableError(response)
          ? i18next.t(USER_FUNDING_UNAVAILABLE_MESSAGE_KEY)
          : response.message || i18next.t('Transfer failed')
      )
      return false
    } catch (error) {
      if (!owns()) return false
      const unknown = walletWriteIsUnknown(error)
      if (unknown) unknownTransfer.current = true
      setOutcomeUnknown(unknown || unknownTransfer.current)
      let message = i18next.t('Transfer failed')
      if (isUserFundingUnavailableError(error)) message = i18next.t(USER_FUNDING_UNAVAILABLE_MESSAGE_KEY)
      if (unknown) message = i18next.t('Funding request outcome is unknown. Check current records and balances before retrying.')
      toast.error(walletWriteErrorMessage(error, message))
      return false
    } finally {
      writeFlight.current = false
      if (isCurrent()) {
        setTransferring(false)
        if (!owns()) { unknownTransfer.current = true; setOutcomeUnknown(true) }
      }
    }
  }, [isCurrent])

  useEffect(() => {
    fetchAffiliateCode()
  }, [fetchAffiliateCode])

  return {
    outcomeUnknown,
    affiliateCode,
    affiliateLink,
    loading,
    error,
    transferring,
    copyAffiliateLink,
    transferQuota,
    refetch: fetchAffiliateCode,
  }
}
