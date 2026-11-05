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
import { toast } from 'sonner'

import {
  isUserFundingUnavailableError,
  USER_FUNDING_UNAVAILABLE_MESSAGE_KEY,
} from '@/lib/self-use-build'

import {
  calculateAmount,
  calculateStripeAmount,
  calculateWaffoAmount,
  calculateWaffoPancakeAmount,
  requestPayment,
  requestStripePayment,
  isApiSuccess,
} from '../api'
import {
  isStripePayment,
  isWaffoPayment,
  isWaffoPancakePayment,
  submitPaymentForm,
} from '../lib'
import type { AmountRequest, AmountResponse } from '../types'
import { useWalletSession, walletWriteIsUnknown, walletWriteErrorMessage, walletRequestConfig, walletPaymentScope, hasUnconfirmedWalletPayment, retainUnconfirmedWalletPayment } from './use-wallet-session'

// ============================================================================
// Payment Hook
// ============================================================================

type AmountCalculator = (request: AmountRequest) => Promise<AmountResponse>

export interface PaymentAmountCalculators {
  regular: AmountCalculator
  stripe: AmountCalculator
  waffo: AmountCalculator
  waffoPancake: AmountCalculator
}

const defaultPaymentAmountCalculators: PaymentAmountCalculators = {
  regular: calculateAmount,
  stripe: calculateStripeAmount,
  waffo: calculateWaffoAmount,
  waffoPancake: calculateWaffoPancakeAmount,
}

async function requestPaymentQuote(
  topupAmount: number,
  paymentType: string,
  calculators: PaymentAmountCalculators = defaultPaymentAmountCalculators
): Promise<{ amount: number; known: boolean; message?: string }> {
  let calculator = calculators.regular
  if (isStripePayment(paymentType)) {
    calculator = calculators.stripe
  } else if (isWaffoPayment(paymentType)) {
    calculator = calculators.waffo
  } else if (isWaffoPancakePayment(paymentType)) {
    calculator = calculators.waffoPancake
  }

  const response = await calculator({ amount: topupAmount })
  const amount = !isApiSuccess(response) || !response.data ? 0 : Number.parseFloat(response.data)
  const raw = response.data as unknown
  const hasNumericValue = (typeof raw === 'number' || (typeof raw === 'string' && raw.trim() !== '')) && Number.isFinite(Number(raw))
  return { amount, known: isApiSuccess(response) && hasNumericValue && Number(raw) >= 0, message: response.message }
}

// Keep the existing pure calculator contract for callers that need only its amount.
export async function requestPaymentAmount(
  topupAmount: number,
  paymentType: string,
  calculators: PaymentAmountCalculators = defaultPaymentAmountCalculators
): Promise<number> {
  return (await requestPaymentQuote(topupAmount, paymentType, calculators)).amount
}

export function usePayment() {
  const isCurrent = useWalletSession()
  const paymentFlight = useRef(false)
  const paymentOwner = useRef(walletPaymentScope())
  const unknownRef = useRef(hasUnconfirmedWalletPayment('wallet-payment', paymentOwner.current))
  const [outcomeUnknown, setOutcomeUnknown] = useState(unknownRef.current)
  useLayoutEffect(() => () => {
    if (paymentFlight.current) retainUnconfirmedWalletPayment('wallet-payment', paymentOwner.current)
  }, [])
  const quoteGeneration = useRef(0)
  const confirmedQuote = useRef<{ amount: number; paymentType: string } | null>(null)
  const [quoteKnown, setQuoteKnown] = useState(false)
  const [quoteError, setQuoteError] = useState<string | null>(null)
  const [amount, setAmount] = useState<number>(0)
  const [calculating, setCalculating] = useState(false)
  const [processing, setProcessing] = useState(false)

  // Quotes are observations. Only the latest valid quote can open a confirmation.
  const calculatePaymentAmount = useCallback(
    async (topupAmount: number, paymentType: string): Promise<number | undefined> => {
      if (!isCurrent()) return undefined
      const generation = ++quoteGeneration.current
      confirmedQuote.current = null
      setQuoteKnown(false)
      setQuoteError(null)
      setCalculating(true)
      try {
        const config = walletRequestConfig(() => isCurrent() && generation === quoteGeneration.current)
        const quote = await requestPaymentQuote(topupAmount, paymentType, {
          regular: (request) => calculateAmount(request, config),
          stripe: (request) => calculateStripeAmount(request, config),
          waffo: (request) => calculateWaffoAmount(request, config),
          waffoPancake: (request) => calculateWaffoPancakeAmount(request, config),
        })
        if (!isCurrent() || generation !== quoteGeneration.current) return undefined
        setAmount(quote.amount)
        setQuoteKnown(quote.known)
        if (!quote.known) {
          setQuoteError(quote.message || i18next.t('Payment quote could not be confirmed. Please try again.'))
          return undefined
        }
        confirmedQuote.current = { amount: topupAmount, paymentType }
        return quote.amount
      } catch (error) {
        if (!isCurrent() || generation !== quoteGeneration.current) return undefined
        setQuoteKnown(false)
        setQuoteError(walletWriteErrorMessage(error, i18next.t('Payment quote could not be confirmed. Please try again.')))
        return undefined
      } finally {
        if (isCurrent() && generation === quoteGeneration.current) setCalculating(false)
      }
    },
    [isCurrent]
  )

  const isQuoteCurrent = useCallback((topupAmount: number, paymentType: string) => {
    const quote = confirmedQuote.current
    return isCurrent() && quote !== null && quote.amount === topupAmount && quote.paymentType === paymentType
  }, [isCurrent])

  // Process payment
  const processPayment = useCallback(
    async (topupAmount: number, paymentType: string, taskCurrent?: () => boolean) => {
      const owns = () => isCurrent() && (taskCurrent?.() ?? true)
      if (!owns() || paymentFlight.current) return false
      if (unknownRef.current || hasUnconfirmedWalletPayment('wallet-payment', paymentOwner.current)) {
        unknownRef.current = true
        setOutcomeUnknown(true)
        return false
      }
      paymentFlight.current = true
      setOutcomeUnknown(false)
      try {
        setProcessing(true)

        const isStripe = isStripePayment(paymentType)
        const amount = Math.floor(topupAmount)

        const response = isStripe
          ? await requestStripePayment({
              amount,
              payment_method: 'stripe',
            }, walletRequestConfig(owns))
          : await requestPayment({
              amount,
              payment_method: paymentType,
            }, walletRequestConfig(owns))

        if (!owns()) return false
        if (!isApiSuccess(response)) {
          toast.error(
            isUserFundingUnavailableError(response)
              ? i18next.t(USER_FUNDING_UNAVAILABLE_MESSAGE_KEY)
              : response.message || i18next.t('Payment request failed')
          )
          return false
        }

        // Handle Stripe payment
        if (isStripe && response.data?.pay_link) {
          window.open(response.data.pay_link as string, '_blank')
          toast.success(i18next.t('Redirecting to payment page...'))
          return true
        }

        // Handle non-Stripe payment
        if (!isStripe && response.data) {
          const url = (response as unknown as { url?: string }).url
          if (url) {
            submitPaymentForm(url, response.data)
            toast.success(i18next.t('Redirecting to payment page...'))
            return true
          }
        }

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

  return {
    amount,
    quoteKnown,
    isQuoteCurrent,
    quoteError,
    outcomeUnknown,
    calculating,
    processing,
    calculatePaymentAmount,
    processPayment,
    setAmount,
  }
}
