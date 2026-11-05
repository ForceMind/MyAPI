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
import { useState, useCallback, useRef, useLayoutEffect } from 'react'

import { useWalletSession, walletWriteIsUnknown, walletWriteErrorMessage, walletRequestConfig, walletPaymentScope, hasUnconfirmedWalletPayment, retainUnconfirmedWalletPayment } from './use-wallet-session'
import { toast } from 'sonner'

import {
  isUserFundingUnavailableError,
  USER_FUNDING_UNAVAILABLE_MESSAGE_KEY,
} from '@/lib/self-use-build'

import { requestWaffoPayment, isApiSuccess } from '../api'

function getPaymentUrl(data: unknown): string | null {
  if (!data || typeof data !== 'object') {
    return null
  }

  if ('payment_url' in data && typeof data.payment_url === 'string') {
    return data.payment_url
  }

  return null
}

function getErrorMessage(message: string | undefined, data: unknown): string {
  if (typeof data === 'string' && data.trim()) {
    return data
  }

  return message || i18next.t('Payment request failed')
}

/**
 * Hook for handling Waffo payment processing
 */
export function useWaffoPayment() {
  const isCurrent = useWalletSession()
  const paymentFlight = useRef(false)
  const paymentOwner = useRef(walletPaymentScope())
  const unknownRef = useRef(hasUnconfirmedWalletPayment('wallet-payment', paymentOwner.current))
  const [outcomeUnknown, setOutcomeUnknown] = useState(unknownRef.current)
  useLayoutEffect(() => () => {
    if (paymentFlight.current) retainUnconfirmedWalletPayment('wallet-payment', paymentOwner.current)
  }, [])
  const [processing, setProcessing] = useState(false)

  const processWaffoPayment = useCallback(
    async (topupAmount: number, payMethodIndex?: number, taskCurrent?: () => boolean) => {
      const owns = () => isCurrent() && (taskCurrent?.() ?? true)
      if (!owns() || paymentFlight.current) return false
      if (unknownRef.current || hasUnconfirmedWalletPayment('wallet-payment', paymentOwner.current)) {
        unknownRef.current = true
        setOutcomeUnknown(true)
        return false
      }
      paymentFlight.current = true
      setOutcomeUnknown(false)
      setProcessing(true)

      try {
        const response = await requestWaffoPayment({
          amount: Math.floor(topupAmount),
          pay_method_index: payMethodIndex,
        }, walletRequestConfig(owns))

        if (!owns()) return false
        if (isApiSuccess(response)) {
          const paymentUrl = getPaymentUrl(response.data)

          if (paymentUrl) {
            window.open(paymentUrl, '_blank')
            toast.success(i18next.t('Redirecting to payment page...'))
            return true
          }
        }

        toast.error(
          isUserFundingUnavailableError(response)
            ? i18next.t(USER_FUNDING_UNAVAILABLE_MESSAGE_KEY)
            : getErrorMessage(response.message, response.data)
        )
        return false
      } catch (error) {
        if (!owns()) return false
        const unknown = walletWriteIsUnknown(error)
        if (unknown) {
          unknownRef.current = true
          retainUnconfirmedWalletPayment('wallet-payment', paymentOwner.current)
        }
        setOutcomeUnknown(unknown)
        let message = i18next.t('Payment request failed')
        if (isUserFundingUnavailableError(error)) message = i18next.t(USER_FUNDING_UNAVAILABLE_MESSAGE_KEY)
        if (unknown) message = i18next.t('Payment request outcome is unknown. Check billing history before trying again.')
        toast.error(walletWriteErrorMessage(error, message))
        return false
      } finally {
        paymentFlight.current = false
        const unconfirmed = !owns()
        if (unconfirmed) {
          unknownRef.current = true
          retainUnconfirmedWalletPayment('wallet-payment', paymentOwner.current)
        }
        if (isCurrent()) {
          setProcessing(false)
          if (unconfirmed) setOutcomeUnknown(true)
        }
      }
    },
    [isCurrent]
  )

  return { processing, outcomeUnknown, processWaffoPayment }
}
