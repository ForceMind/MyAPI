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
import { useState, useEffect, useCallback, useMemo, useRef, useLayoutEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useStatus } from '@/hooks/use-status'
import { useFundingPresentation } from '@/hooks/use-funding-presentation'
import { useSystemConfig } from '@/hooks/use-system-config'
import { getWalletAccount } from './api'
import { useAuthStore } from '@/stores/auth-store'
import {
  areUserFundingCapabilitiesReady,
  resolveUserFundingCapabilities,
} from '@/lib/self-use-build'

import { AffiliateRewardsCard } from './components/affiliate-rewards-card'
import { BillingHistoryDialog } from './components/dialogs/billing-history-dialog'
import { CreemConfirmDialog } from './components/dialogs/creem-confirm-dialog'
import { PaymentConfirmDialog } from './components/dialogs/payment-confirm-dialog'
import { TransferDialog } from './components/dialogs/transfer-dialog'
import { RechargeFormCard } from './components/recharge-form-card'
import { SubscriptionPlansCard } from './components/subscription-plans-card'
import { WalletStatsCard } from './components/wallet-stats-card'
import { DEFAULT_DISCOUNT_RATE, PAYMENT_TYPES } from './constants'
import { useWalletSession, walletRequestConfig, walletWriteErrorMessage, hasUnconfirmedWalletPayment } from './hooks/use-wallet-session'
import { readWalletAccount, type WalletAccountSnapshot } from './lib/wallet-account'
import {
  useTopupInfo,
  usePayment,
  useAffiliate,
  useRedemption,
  useCreemPayment,
  useWaffoPayment,
  useWaffoPancakePayment,
} from './hooks'
import {
  getDefaultPaymentType,
  getMinTopupAmount,
  dispatchSelectedPayment,
} from './lib'
import type {
  PaymentMethod,
  PresetAmount,
  CreemProduct,
  WaffoPayMethod,
} from './types'

interface WalletProps {
  initialShowHistory?: boolean
}

export function Wallet(props: WalletProps) {
  const actor = useAuthStore((state) => state.auth.user)
  const sid = useAuthStore((state) => state.auth.session?.sid ?? null)
  if (!actor) return null
  return <WalletSession key={`${actor.id}:${actor.role}:${sid}`} {...props} />
}

function WalletSession(props: WalletProps) {
  const { t } = useTranslation()
  const isCurrent = useWalletSession()
  const actorId = useAuthStore((state) => state.auth.user?.id)
  const userRead = useRef(0)
  const [userError, setUserError] = useState<string | null>(null)
  const [user, setUser] = useState<WalletAccountSnapshot | null>(null)
  const [userLoading, setUserLoading] = useState(true)
  const [topupAmount, setTopupAmount] = useState(0)
  const [selectedPreset, setSelectedPreset] = useState<number | null>(null)
  const [selectedPaymentMethod, setSelectedPaymentMethod] =
    useState<PaymentMethod>()
  const [selectedWaffoMethodIndex, setSelectedWaffoMethodIndex] = useState<
    number | null
  >(null)
  const [paymentLoading, setPaymentLoading] = useState<string | null>(null)
  const [confirmDialogOpen, setConfirmDialogOpen] = useState(false)
  const [transferDialogOpen, setTransferDialogOpen] = useState(false)
  const [billingDialogOpen, setBillingDialogOpen] = useState(false)
  const [redemptionCode, setRedemptionCode] = useState('')
  const [creemDialogOpen, setCreemDialogOpen] = useState(false)
  const [selectedCreemProduct, setSelectedCreemProduct] =
    useState<CreemProduct | null>(null)
  const [showSubscriptionPanel, setShowSubscriptionPanel] = useState(true)
  const paymentOpening = useRef<object>({})
  const creemOpening = useRef<object>({})
  const transferOpening = useRef<object>({})
  useLayoutEffect(() => { paymentOpening.current = {} }, [confirmDialogOpen])
  useLayoutEffect(() => { creemOpening.current = {} }, [creemDialogOpen, selectedCreemProduct?.productId])

  const { status } = useStatus()
  const fundingPresentation = useFundingPresentation()
  const { currency } = useSystemConfig()
  const { topupInfo, presetAmounts, loading: topupLoading, error: topupError, refetch: retryTopupInfo } = useTopupInfo()
  const fundingCapabilities = resolveUserFundingCapabilities(topupInfo)
  const fundingCapabilitiesReady = areUserFundingCapabilitiesReady(
    topupInfo,
    topupLoading
  )
  const actorVerified = user !== null && user.id === actorId
  const commerceReady = fundingCapabilitiesReady && fundingPresentation.commercialEnabled && fundingCapabilities.epoch === fundingPresentation.capabilities.epoch
  const showTopUpActions =
    commerceReady &&
    (actorVerified && (fundingCapabilities.can_top_up || fundingCapabilities.can_redeem))
  const canTopUp = commerceReady && actorVerified && fundingCapabilities.can_top_up
  const canRedeem = commerceReady && actorVerified && fundingCapabilities.can_redeem
  const canTransferAffiliateRewards =
    commerceReady &&
    fundingCapabilities.can_transfer_affiliate_rewards && user?.aff_quota !== undefined
  useLayoutEffect(() => { transferOpening.current = {} }, [transferDialogOpen, canTransferAffiliateRewards])
  const canPurchaseSubscription =
    commerceReady && actorVerified && fundingCapabilities.can_purchase_subscription
  let fundingStatusMessage = ''
  if (!fundingCapabilitiesReady || (!commerceReady && fundingCapabilities.mode === 'enabled')) {
    fundingStatusMessage = t(
      'User funding status is not ready. Funding actions remain disabled until the server confirms them.'
    )
  } else if (fundingCapabilities.mode === 'retirement') {
    fundingStatusMessage = t(
      'User funding is in retirement. New funding actions are disabled; funding history remains available.'
    )
  } else if (fundingCapabilities.mode === 'disabled') {
    fundingStatusMessage = t(
      'User funding is disabled. Funding history remains available.'
    )
  }

  // Calculate effective exchange rate - when display type is USD, use rate of 1
  const effectiveUsdExchangeRate = useMemo(() => {
    return currency?.quotaDisplayType === 'USD'
      ? 1
      : currency?.usdExchangeRate || 1
  }, [currency?.quotaDisplayType, currency?.usdExchangeRate])
  const {
    amount: paymentAmount,
    quoteKnown,
    isQuoteCurrent,
    quoteError,
    outcomeUnknown: paymentUnknown,
    calculating,
    processing,
    calculatePaymentAmount,
    processPayment,
  } = usePayment()
  const {
    affiliateLink,
    loading: affiliateLoading,
    transferQuota,
    transferring,
    outcomeUnknown: transferUnknown,
    error: affiliateError,
    refetch: retryAffiliate,
  } = useAffiliate()
  const { redeeming, redeemCode, outcomeUnknown: redemptionUnknown } = useRedemption()
  const { processing: creemProcessing, outcomeUnknown: creemUnknown, processCreemPayment } = useCreemPayment()
  const { processing: waffoProcessing, outcomeUnknown: waffoUnknown, processWaffoPayment } = useWaffoPayment()
  const { processing: pancakeProcessing, outcomeUnknown: pancakeUnknown, processWaffoPancakePayment } =
    useWaffoPancakePayment()

  // Each field may be unknown, but the response must belong to this actor.
  const fetchUser = useCallback(async () => {
    if (!isCurrent()) return
    const generation = ++userRead.current
    setUserLoading(true)
    setUser(null)
    setUserError(null)
    try {
      const response = await getWalletAccount(walletRequestConfig(() => isCurrent() && generation === userRead.current))
      if (!isCurrent() || generation !== userRead.current) return
      if (!response.success) {
        setUser(null)
        setTransferDialogOpen(false)
        setUserError(response.message || t('Wallet account could not be read. Please retry.'))
        return
      }
      const snapshot = readWalletAccount(response.data, actorId)
      if (!snapshot) throw new Error('Wallet snapshot unavailable')
      setUser(snapshot)
    } catch (error) {
      if (!isCurrent() || generation !== userRead.current) return
      setUser(null)
      setTransferDialogOpen(false)
      setUserError(walletWriteErrorMessage(error, t('Wallet account could not be read. Please retry.')))
    } finally {
      if (isCurrent() && generation === userRead.current) setUserLoading(false)
    }
  }, [actorId, isCurrent, t])

  useEffect(() => {
    fetchUser()
  }, [fetchUser])

  useEffect(() => {
    if (props.initialShowHistory) {
      setBillingDialogOpen(true)
      window.history.replaceState({}, '', window.location.pathname)
    }
  }, [props.initialShowHistory])

  // Initialize topup amount when topup info is loaded
  const topupAmountInitializedRef = useRef(false)
  useEffect(() => {
    if (topupInfo && !topupAmountInitializedRef.current) {
      topupAmountInitializedRef.current = true
      const minTopup = getMinTopupAmount(topupInfo)
      setTopupAmount(minTopup)

      // Calculate initial payment amount with default payment type
      if (canTopUp) {
        const defaultPaymentType = getDefaultPaymentType(topupInfo)
        calculatePaymentAmount(minTopup, defaultPaymentType)
      }
    }
  }, [topupInfo, canTopUp, calculatePaymentAmount])

  // Get current payment type (selected or default)
  const getCurrentPaymentType = useCallback(() => {
    return selectedPaymentMethod?.type || getDefaultPaymentType(topupInfo)
  }, [selectedPaymentMethod, topupInfo])

  // Handle preset selection
  const handleSelectPreset = (preset: PresetAmount) => {
    if (!isCurrent() || hasUnconfirmedWalletPayment('wallet-payment')) return
    setConfirmDialogOpen(false)
    setTopupAmount(preset.value)
    setSelectedPreset(preset.value)
    calculatePaymentAmount(preset.value, getCurrentPaymentType())
  }

  // Handle topup amount change
  const handleTopupAmountChange = (amount: number) => {
    if (!isCurrent() || hasUnconfirmedWalletPayment('wallet-payment')) return
    setConfirmDialogOpen(false)
    setTopupAmount(amount)
    setSelectedPreset(null)
    calculatePaymentAmount(amount, getCurrentPaymentType())
  }

  // Handle payment method selection
  const handlePaymentMethodSelect = async (method: PaymentMethod) => {
    if (!isCurrent() || hasUnconfirmedWalletPayment('wallet-payment')) return
    if (!canTopUp) return
    setConfirmDialogOpen(false)
    setSelectedPaymentMethod(method)
    setSelectedWaffoMethodIndex(null)
    setPaymentLoading(method.type)

    try {
      // Validate minimum topup
      const minTopup = getMinTopupAmount(topupInfo)
      if (topupAmount < minTopup) {
        return
      }

      // Calculate payment amount and show confirmation dialog
      const quote = await calculatePaymentAmount(topupAmount, method.type)
      if (!isCurrent() || quote === undefined) return
      setConfirmDialogOpen(true)
    } finally {
      if (isCurrent()) setPaymentLoading(null)
    }
  }

  // Handle payment confirmation
  const handlePaymentConfirm = async () => {
    if (!isCurrent()) return
    if (!canTopUp || !selectedPaymentMethod || !quoteKnown || paymentUnknown || creemUnknown || waffoUnknown || pancakeUnknown || calculating || !isQuoteCurrent(topupAmount, selectedPaymentMethod.type)) return

    const operation = paymentOpening.current
    const owns = () => isCurrent() && paymentOpening.current === operation
    const success = await dispatchSelectedPayment(
      selectedPaymentMethod,
      topupAmount,
      selectedWaffoMethodIndex,
      {
        regular: (amount, type) => processPayment(amount, type, owns),
        waffo: (amount, index) => processWaffoPayment(amount, index, owns),
        waffoPancake: (amount) => processWaffoPancakePayment(amount, owns),
      }
    )

    if (!owns()) return
    if (success) {
      setConfirmDialogOpen(false)
      await fetchUser()
    }
  }

  // Handle redemption
  const handleRedeem = async () => {
    if (!isCurrent()) return
    if (!canRedeem || !redemptionCode) return

    const submittedCode = redemptionCode
    const success = await redeemCode(submittedCode)
    if (!isCurrent()) return
    if (success) {
      setRedemptionCode((current) => current === submittedCode ? '' : current)
      await fetchUser()
    }
  }

  // Handle transfer
  const handleTransfer = async (amount: number) => {
    if (!isCurrent()) return false
    if (!canTransferAffiliateRewards) return false
    const operation = transferOpening.current
    const owns = () => isCurrent() && transferOpening.current === operation
    const success = await transferQuota(amount, owns)
    if (!owns()) return false
    if (success) {
      await fetchUser()
    }
    return success
  }

  // Handle Creem product selection
  const handleCreemProductSelect = (product: CreemProduct) => {
    if (!isCurrent() || hasUnconfirmedWalletPayment('wallet-payment')) return
    if (!canTopUp) return
    setSelectedCreemProduct(product)
    setCreemDialogOpen(true)
  }

  // Handle Creem payment confirmation
  const handleCreemConfirm = async () => {
    if (!isCurrent()) return
    if (!canTopUp || !selectedCreemProduct || paymentUnknown || creemUnknown || waffoUnknown || pancakeUnknown) return

    const operation = creemOpening.current
    const owns = () => isCurrent() && creemOpening.current === operation
    const success = await processCreemPayment(selectedCreemProduct.productId, owns)
    if (!owns()) return
    if (success) {
      setCreemDialogOpen(false)
      setSelectedCreemProduct(null)
      await fetchUser()
    }
  }

  const handleWaffoMethodSelect = async (
    method: WaffoPayMethod,
    index: number
  ) => {
    if (!isCurrent() || !canTopUp || hasUnconfirmedWalletPayment('wallet-payment')) return
    setConfirmDialogOpen(false)
    const loadingKey = `waffo-${index}`
    setSelectedPaymentMethod({
      name: method.name,
      type: PAYMENT_TYPES.WAFFO,
      icon: method.icon,
    })
    setSelectedWaffoMethodIndex(index)
    setPaymentLoading(loadingKey)

    try {
      const quote = await calculatePaymentAmount(topupAmount, PAYMENT_TYPES.WAFFO)
      if (!isCurrent() || quote === undefined) return
      setConfirmDialogOpen(true)
    } finally {
      if (isCurrent()) setPaymentLoading(null)
    }
  }

  // Get discount rate for current topup amount
  const getDiscountRate = useCallback(() => {
    return topupInfo?.discount?.[topupAmount] || DEFAULT_DISCOUNT_RATE
  }, [topupInfo, topupAmount])

  const handleSubscriptionAvailabilityChange = useCallback(
    (available: boolean) => {
      setShowSubscriptionPanel(available)
    },
    []
  )

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {commerceReady ? t('Wallet') : t('Funding history')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {commerceReady
            ? t('Balance, top-up, subscriptions and referral rewards.')
            : t('Recorded usage and past orders stay available.')}
        </SectionPageLayout.Description>
        {!showTopUpActions ? (
          <SectionPageLayout.Actions>
            <Button
              type='button'
              variant='outline'
              onClick={() => setBillingDialogOpen(true)}
            >
              {t('Billing History')}
            </Button>
          </SectionPageLayout.Actions>
        ) : null}
        <SectionPageLayout.Content>
          <div className='flex w-full min-w-0 flex-col gap-4 sm:gap-5'>
            {userError && <Alert variant='destructive'><AlertDescription>{userError}<Button className='mt-2 min-h-11' variant='outline' disabled={userLoading} onClick={() => void fetchUser()}>{t('Retry')}</Button></AlertDescription></Alert>}
            {topupError && <Alert variant='destructive'><AlertDescription>{topupError}<Button className='mt-2 min-h-11' variant='outline' disabled={topupLoading} onClick={() => void retryTopupInfo()}>{t('Retry')}</Button></AlertDescription></Alert>}
            <WalletStatsCard
              user={user}
              loading={userLoading}
              showBalance={commerceReady}
            />

            {(paymentUnknown || creemUnknown || waffoUnknown || pancakeUnknown) && <Alert><AlertDescription>{t('Payment outcome is unconfirmed. New payments are locked; review billing history.')}<Button className='mt-2 min-h-11' variant='outline' onClick={() => setBillingDialogOpen(true)}>{t('Billing History')}</Button></AlertDescription></Alert>}
            {quoteError && <Alert variant='destructive'><AlertDescription>{quoteError}</AlertDescription></Alert>}
            {!showTopUpActions && fundingStatusMessage ? (
              <Alert>
                <AlertDescription>{fundingStatusMessage}</AlertDescription>
              </Alert>
            ) : null}

            <div
              className={
                showTopUpActions && showSubscriptionPanel
                  ? 'grid gap-4 xl:grid-cols-[minmax(0,1.05fr)_minmax(360px,0.95fr)] xl:items-start'
                  : 'grid gap-4'
              }
            >
              {showTopUpActions ? (
                <div id='wallet-add-funds' className='scroll-mt-4'>
                  <RechargeFormCard
                    topupInfo={topupInfo}
                    presetAmounts={presetAmounts}
                    selectedPreset={selectedPreset}
                    onSelectPreset={handleSelectPreset}
                    topupAmount={topupAmount}
                    onTopupAmountChange={handleTopupAmountChange}
                    paymentAmount={quoteKnown ? paymentAmount : undefined}
                    calculating={calculating}
                    paymentLocked={paymentUnknown || creemUnknown || waffoUnknown || pancakeUnknown}
                    onPaymentMethodSelect={handlePaymentMethodSelect}
                    paymentLoading={paymentLoading}
                    redemptionCode={redemptionCode}
                    onRedemptionCodeChange={setRedemptionCode}
                    onRedeem={handleRedeem}
                    redeeming={redeeming}
                    topupLink={topupInfo?.topup_link}
                    loading={topupLoading}
                    priceRatio={(status?.price as number) || 1}
                    usdExchangeRate={effectiveUsdExchangeRate}
                    onOpenBilling={() => setBillingDialogOpen(true)}
                    creemProducts={topupInfo?.creem_products}
                    enableCreemTopup={topupInfo?.enable_creem_topup}
                    onCreemProductSelect={handleCreemProductSelect}
                    enableWaffoTopup={topupInfo?.enable_waffo_topup}
                    waffoPayMethods={topupInfo?.waffo_pay_methods}
                    waffoMinTopup={topupInfo?.waffo_min_topup}
                    onWaffoMethodSelect={handleWaffoMethodSelect}
                    enableWaffoPancakeTopup={
                      topupInfo?.enable_waffo_pancake_topup
                    }
                  />
                </div>
              ) : null}

              <SubscriptionPlansCard
                topupInfo={topupInfo}
                onAvailabilityChange={handleSubscriptionAvailabilityChange}
                userQuota={user?.quota}
                onPurchaseSuccess={fetchUser}
                canPurchaseSubscription={canPurchaseSubscription}
              />
            </div>

            {(transferUnknown || redemptionUnknown) && <Alert><AlertDescription>{t('Funding request outcome is unknown. Check current records and balances before retrying.')}<Button className='mt-2 min-h-11' variant='outline' disabled={userLoading} onClick={() => void fetchUser()}>{t('Refresh')}</Button></AlertDescription></Alert>}
            {affiliateError && <Alert variant='destructive'><AlertDescription>{affiliateError}<Button className='mt-2 min-h-11' variant='outline' disabled={affiliateLoading} onClick={() => void retryAffiliate()}>{t('Retry')}</Button></AlertDescription></Alert>}
            <AffiliateRewardsCard
              historyOnly={!commerceReady}
              user={user}
              affiliateLink={affiliateLink}
              onTransfer={() => setTransferDialogOpen(true)}
              complianceConfirmed={
                topupInfo?.payment_compliance_confirmed !== false
              }
              canTransfer={canTransferAffiliateRewards}
              loading={affiliateLoading}
            />
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      {canTopUp ? (
        <PaymentConfirmDialog
          open={confirmDialogOpen}
          onOpenChange={setConfirmDialogOpen}
          onConfirm={handlePaymentConfirm}
          onOpenHistory={() => { setConfirmDialogOpen(false); setBillingDialogOpen(true) }}
          topupAmount={topupAmount}
          paymentAmount={paymentAmount}
          quoteKnown={quoteKnown}
          outcomeUnknown={paymentUnknown || creemUnknown || waffoUnknown || pancakeUnknown}
          paymentMethod={selectedPaymentMethod}
          calculating={calculating}
          processing={processing || waffoProcessing || pancakeProcessing}
          discountRate={getDiscountRate()}
          usdExchangeRate={effectiveUsdExchangeRate}
        />
      ) : null}

      {canTransferAffiliateRewards ? (
        <TransferDialog
          open={transferDialogOpen}
          onOpenChange={setTransferDialogOpen}
          onConfirm={handleTransfer}
          availableQuota={user?.aff_quota ?? 0}
          transferring={transferring}
        />
      ) : null}

      <BillingHistoryDialog
        open={billingDialogOpen}
        onOpenChange={setBillingDialogOpen}
      />

      {canTopUp ? (
        <CreemConfirmDialog
          open={creemDialogOpen}
          onOpenChange={setCreemDialogOpen}
          onConfirm={handleCreemConfirm}
          onOpenHistory={() => { setCreemDialogOpen(false); setBillingDialogOpen(true) }}
          product={selectedCreemProduct}
          outcomeUnknown={paymentUnknown || creemUnknown || waffoUnknown || pancakeUnknown}
          processing={creemProcessing}
        />
      ) : null}
    </>
  )
}
