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
import { Calendar03Icon, CrownIcon, PackageIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useState, useEffect, useLayoutEffect, useRef } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useWalletSession, walletRequestConfig, walletWriteIsUnknown, walletWriteErrorMessage, walletPaymentScope, hasUnconfirmedWalletPayment, retainUnconfirmedWalletPayment } from '@/features/wallet/hooks/use-wallet-session'
import { useAuthStore } from '@/stores/auth-store'
import { toast } from 'sonner'

import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { GroupBadge } from '@/components/group-badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { useSystemConfig } from '@/hooks/use-system-config'
import { formatQuota } from '@/lib/format'
import {
  isUserFundingUnavailableError,
  USER_FUNDING_UNAVAILABLE_MESSAGE_KEY,
} from '@/lib/self-use-build'
import { DEFAULT_CURRENCY_CONFIG } from '@/stores/system-config-store'

import {
  paySubscriptionStripe,
  paySubscriptionCreem,
  paySubscriptionEpay,
  paySubscriptionWaffoPancake,
  paySubscriptionBalance,
} from '../../api'
import { formatDuration, formatResetPeriod } from '../../lib'
import type { PlanRecord } from '../../types'

interface PaymentMethod {
  type: string
  name?: string
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  plan: PlanRecord | null
  returnFocus?: () => HTMLElement | null
  enableStripe?: boolean
  enableCreem?: boolean
  enableWaffoPancake?: boolean
  enableOnlineTopUp?: boolean
  epayMethods?: PaymentMethod[]
  purchaseLimit?: number
  purchaseCount?: number
  userQuota?: number
  onPurchaseSuccess?: () => void | Promise<void>
  onOpenHistory?: () => void
}

export function SubscriptionPurchaseDialog(props: Props) {
  const isOwner = useWalletSession()
  const identity = useAuthStore((state) => [state.auth.user?.id, state.auth.user?.role, state.auth.session?.sid ?? null].join(':'))
  if (!isOwner()) return null
  return <SubscriptionPurchaseTask key={`${identity}:${props.plan?.plan.id}`} {...props} />
}

function SubscriptionPurchaseTask(props: Props) {
  const isCurrentSession = useWalletSession()
  const { t } = useTranslation()
  const { currency } = useSystemConfig()
  const active = useRef({ open: props.open, planId: props.plan?.plan.id })
  useLayoutEffect(() => { active.current = { open: props.open, planId: props.plan?.plan.id } }, [props.open, props.plan?.plan.id])
  const flight = useRef<object | null>(null)
  const paymentOwner = useRef(walletPaymentScope())
  const paymentKey = `subscription-plan:${props.plan?.plan.id}`
  const unknownRef = useRef(hasUnconfirmedWalletPayment(paymentKey, paymentOwner.current))
  const [outcomeUnknown, setOutcomeUnknown] = useState(unknownRef.current)
  useLayoutEffect(() => () => {
    if (flight.current) retainUnconfirmedWalletPayment(paymentKey, paymentOwner.current)
  }, [paymentKey])
  const [paying, setPaying] = useState(false)
  const [selectedEpayMethod, setSelectedEpayMethod] = useState('')
  const paymentErrorMessage = (error: unknown) => {
    if (walletWriteIsUnknown(error)) return walletWriteErrorMessage(error, t('Payment request outcome is unknown. Check billing history before trying again.'))
    return walletWriteErrorMessage(error, isUserFundingUnavailableError(error) ? t(USER_FUNDING_UNAVAILABLE_MESSAGE_KEY) : t('Payment request failed'))
  }

  useEffect(() => {
    if (props.open && props.epayMethods && props.epayMethods.length > 0) {
      setSelectedEpayMethod(props.epayMethods[0].type)
    } else if (!props.open) {
      setSelectedEpayMethod('')
    }
  }, [props.open, props.epayMethods])

  const plan = props.plan?.plan
  if (!plan) return null

  const isCurrent = (operation: typeof active.current) => isCurrentSession() && active.current === operation && operation.open && operation.planId === plan.id

  const finishPayment = (operation: typeof active.current) => {
    if (flight.current !== operation) return
    flight.current = null
    if (!isCurrentSession()) return
    setPaying(false)
    if (!isCurrent(operation)) {
      unknownRef.current = true
      retainUnconfirmedWalletPayment(paymentKey, paymentOwner.current)
      setOutcomeUnknown(true)
    }
  }

  const hasStripe = props.enableStripe && !!plan.stripe_price_id
  const hasCreem = props.enableCreem && !!plan.creem_product_id
  const hasWaffoPancake =
    props.enableWaffoPancake && !!plan.waffo_pancake_product_id
  const hasEpay =
    props.enableOnlineTopUp && (props.epayMethods || []).length > 0
  const hasAnyPayment = hasStripe || hasCreem || hasWaffoPancake || hasEpay
  const selectedEpayMethodLabel =
    (props.epayMethods || []).find((m) => m.type === selectedEpayMethod)
      ?.name ||
    selectedEpayMethod ||
    t('Select payment method')
  const totalAmount = Number(plan.total_amount || 0)
  const price = Number(plan.price_amount || 0).toFixed(2)
  const quotaPerUnit =
    currency?.quotaPerUnit && currency.quotaPerUnit > 0
      ? currency.quotaPerUnit
      : DEFAULT_CURRENCY_CONFIG.quotaPerUnit
  const balanceCost = Math.max(
    0,
    Math.ceil(Number(plan.price_amount || 0) * quotaPerUnit)
  )
  const balanceKnown = typeof props.userQuota === 'number' && Number.isFinite(props.userQuota)
  const userQuota = Math.max(0, Number(props.userQuota || 0))
  const allowBalancePay = plan.allow_balance_pay !== false
  const insufficientBalance = userQuota < balanceCost
  const limitReached =
    (props.purchaseLimit || 0) > 0 &&
    (props.purchaseCount || 0) >= (props.purchaseLimit || 0)

  const handlePayStripe = async () => {
    const operation = active.current
    if (!isCurrent(operation) || flight.current) return
    if (unknownRef.current || hasUnconfirmedWalletPayment(paymentKey, paymentOwner.current)) { setOutcomeUnknown(true); return }
    flight.current = operation
    setOutcomeUnknown(false)
    setPaying(true)
    try {
      const res = await paySubscriptionStripe({ plan_id: plan.id }, walletRequestConfig(() => isCurrent(operation)))
      if (!isCurrent(operation)) return
      if (res.message === 'success' && res.data?.pay_link) {
        window.open(res.data.pay_link, '_blank')
        toast.success(t('Payment page opened'))
        props.onOpenChange(false)
      } else {
        toast.error(
          res.message && res.message !== 'success'
            ? res.message
            : t('Payment request failed')
        )
      }
    } catch (error) {
      if (!isCurrent(operation)) return
      const unknown = walletWriteIsUnknown(error)
      if (unknown) { unknownRef.current = true; retainUnconfirmedWalletPayment(paymentKey, paymentOwner.current) }
      setOutcomeUnknown(unknown)
      toast.error(paymentErrorMessage(error))
    } finally {
      finishPayment(operation)
    }
  }

  const handlePayCreem = async () => {
    const operation = active.current
    if (!isCurrent(operation) || flight.current) return
    if (unknownRef.current || hasUnconfirmedWalletPayment(paymentKey, paymentOwner.current)) { setOutcomeUnknown(true); return }
    flight.current = operation
    setOutcomeUnknown(false)
    setPaying(true)
    try {
      const res = await paySubscriptionCreem({ plan_id: plan.id }, walletRequestConfig(() => isCurrent(operation)))
      if (!isCurrent(operation)) return
      if (res.message === 'success' && res.data?.checkout_url) {
        window.open(res.data.checkout_url, '_blank')
        toast.success(t('Payment page opened'))
        props.onOpenChange(false)
      } else {
        toast.error(
          res.message && res.message !== 'success'
            ? res.message
            : t('Payment request failed')
        )
      }
    } catch (error) {
      if (!isCurrent(operation)) return
      const unknown = walletWriteIsUnknown(error)
      if (unknown) { unknownRef.current = true; retainUnconfirmedWalletPayment(paymentKey, paymentOwner.current) }
      setOutcomeUnknown(unknown)
      toast.error(paymentErrorMessage(error))
    } finally {
      finishPayment(operation)
    }
  }

  // In-tab redirect (not window.open) — user-gesture context is lost
  // across the await, so a popup would be blocked. Same as the wallet hook.
  const handlePayWaffoPancake = async () => {
    const operation = active.current
    if (!isCurrent(operation) || flight.current) return
    if (unknownRef.current || hasUnconfirmedWalletPayment(paymentKey, paymentOwner.current)) { setOutcomeUnknown(true); return }
    flight.current = operation
    setOutcomeUnknown(false)
    setPaying(true)
    try {
      const res = await paySubscriptionWaffoPancake({ plan_id: plan.id }, walletRequestConfig(() => isCurrent(operation)))
      if (!isCurrent(operation)) return
      if (res.message === 'success' && res.data?.checkout_url) {
        toast.success(t('Redirecting to payment page...'))
        window.location.href = res.data.checkout_url
      } else {
        toast.error(
          res.message && res.message !== 'success'
            ? res.message
            : t('Payment request failed')
        )
      }
    } catch (error) {
      if (!isCurrent(operation)) return
      const unknown = walletWriteIsUnknown(error)
      if (unknown) { unknownRef.current = true; retainUnconfirmedWalletPayment(paymentKey, paymentOwner.current) }
      setOutcomeUnknown(unknown)
      toast.error(paymentErrorMessage(error))
    } finally {
      finishPayment(operation)
    }
  }

  const isSafari =
    typeof navigator !== 'undefined' &&
    /^((?!chrome|android).)*safari/i.test(navigator.userAgent)

  const handlePayEpay = async () => {
    const operation = active.current
    if (!isCurrent(operation) || flight.current) return
    if (unknownRef.current || hasUnconfirmedWalletPayment(paymentKey, paymentOwner.current)) { setOutcomeUnknown(true); return }
    if (!selectedEpayMethod) {
      toast.error(t('Please select a payment method'))
      return
    }
    flight.current = operation
    setOutcomeUnknown(false)
    setPaying(true)
    try {
      const res = await paySubscriptionEpay({
        plan_id: plan.id,
        payment_method: selectedEpayMethod,
      }, walletRequestConfig(() => isCurrent(operation)))
      if (!isCurrent(operation)) return
      if (res.message === 'success' && res.url) {
        const form = document.createElement('form')
        form.action = res.url
        form.method = 'POST'
        if (!isSafari) {
          form.target = '_blank'
        }
        Object.entries(res.data || {}).forEach(([key, value]) => {
          const input = document.createElement('input')
          input.type = 'hidden'
          input.name = key
          input.value = String(value)
          form.appendChild(input)
        })
        document.body.appendChild(form)
        form.submit()
        document.body.removeChild(form)
        toast.success(t('Payment initiated'))
        props.onOpenChange(false)
      } else {
        toast.error(
          res.message && res.message !== 'success'
            ? res.message
            : t('Payment request failed')
        )
      }
    } catch (error) {
      if (!isCurrent(operation)) return
      const unknown = walletWriteIsUnknown(error)
      if (unknown) { unknownRef.current = true; retainUnconfirmedWalletPayment(paymentKey, paymentOwner.current) }
      setOutcomeUnknown(unknown)
      toast.error(paymentErrorMessage(error))
    } finally {
      finishPayment(operation)
    }
  }

  const handlePayBalance = async () => {
    const operation = active.current
    if (!isCurrent(operation) || flight.current) return
    if (unknownRef.current || hasUnconfirmedWalletPayment(paymentKey, paymentOwner.current)) { setOutcomeUnknown(true); return }
    if (!balanceKnown) return
    if (!allowBalancePay) {
      toast.error(t('This plan does not allow balance redemption'))
      return
    }
    flight.current = operation
    setOutcomeUnknown(false)
    setPaying(true)
    try {
      const res = await paySubscriptionBalance({ plan_id: plan.id }, walletRequestConfig(() => isCurrent(operation)))
      if (!isCurrent(operation)) return
      if (res.success) {
        toast.success(t('Subscription purchased successfully'))
        void props.onPurchaseSuccess?.()
        props.onOpenChange(false)
      } else {
        toast.error(
          res.message && res.message !== 'success'
            ? res.message
            : t('Payment request failed')
        )
      }
    } catch (error) {
      if (!isCurrent(operation)) return
      const unknown = walletWriteIsUnknown(error)
      if (unknown) { unknownRef.current = true; retainUnconfirmedWalletPayment(paymentKey, paymentOwner.current) }
      setOutcomeUnknown(unknown)
      toast.error(paymentErrorMessage(error))
    } finally {
      finishPayment(operation)
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={(nextOpen) => (nextOpen || !paying) && props.onOpenChange(nextOpen)}>
      <DialogContent finalFocus={props.returnFocus} showCloseButton={!paying} className='flex h-dvh max-h-dvh w-full max-w-none flex-col gap-0 overflow-hidden rounded-none p-0 sm:h-auto sm:max-h-[90dvh] sm:max-w-lg sm:rounded-xl [&_[data-slot=button]]:min-h-11 [&_[data-slot=button]]:min-w-11 [&_[data-slot=select-trigger]]:min-h-11 [&_[data-slot=dialog-close]]:min-h-11 [&_[data-slot=dialog-close]]:min-w-11'>
        <DialogHeader className='shrink-0 border-b px-4 py-4 pr-16 sm:px-6 sm:pr-16'><DialogTitle className='flex items-center gap-2'><HugeiconsIcon icon={CrownIcon} aria-hidden='true' className='size-5' />{t('Purchase Subscription')}</DialogTitle></DialogHeader>
        <section role='region' tabIndex={0} aria-label={t('Subscription purchase details')} className='min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-contain px-4 py-4 [overflow-wrap:anywhere] sm:px-6'>
      <div className='space-y-3 sm:space-y-4'>
        <div className='bg-muted/50 space-y-2.5 rounded-lg border p-3 sm:space-y-3 sm:p-4'>
          <div className='flex min-w-0 flex-wrap justify-between gap-2'>
            <span className='text-muted-foreground text-sm'>
              {t('Plan Name')}
            </span>
            <span className='min-w-0 text-right text-sm font-medium break-words [overflow-wrap:anywhere]'>
              {plan.title}
            </span>
          </div>
          <div className='flex min-w-0 flex-wrap items-start justify-between gap-2'>
            <span className='text-muted-foreground text-sm'>
              {t('Validity Period')}
            </span>
            <span className='flex items-center gap-1 text-sm'>
              <HugeiconsIcon icon={Calendar03Icon} className='h-3.5 w-3.5' />
              {formatDuration(plan, t)}
            </span>
          </div>
          {formatResetPeriod(plan, t) !== t('No Reset') && (
            <div className='flex min-w-0 flex-wrap justify-between gap-2'>
              <span className='text-muted-foreground text-sm'>
                {t('Reset Period')}
              </span>
              <span className='text-sm'>{formatResetPeriod(plan, t)}</span>
            </div>
          )}
          <div className='flex min-w-0 flex-wrap items-start justify-between gap-2'>
            <span className='text-muted-foreground text-sm'>
              {t('Plan Quota')}
            </span>
            <span className='flex items-center gap-1 text-sm'>
              <HugeiconsIcon icon={PackageIcon} className='h-3.5 w-3.5' />
              {totalAmount > 0 ? formatQuota(totalAmount) : t('Unlimited')}
            </span>
          </div>
          {plan.upgrade_group && (
            <div className='flex min-w-0 flex-wrap items-start justify-between gap-2'>
              <span className='text-muted-foreground text-sm'>
                {t('Upgrade Group')}
              </span>
              <GroupBadge group={plan.upgrade_group} />
            </div>
          )}
          <Separator />
          <div role='group' aria-label={t('Amount Due')} className='flex min-w-0 flex-wrap items-start justify-between gap-2'>
            <span className='text-sm font-medium'>{t('Amount Due')}</span>
            <span className='text-primary text-lg font-bold'>${price}</span>
          </div>
        </div>

        {limitReached && (
          <Alert variant='destructive'>
            <AlertDescription>
              {t('Purchase limit reached')} ({props.purchaseCount}/
              {props.purchaseLimit})
            </AlertDescription>
          </Alert>
        )}

        <p className='text-muted-foreground text-xs'>{t('Closing a window does not cancel a submitted payment. Check billing history if the outcome is unknown.')}</p>
        {outcomeUnknown && <Alert><AlertDescription>{t('Payment outcome is unconfirmed. New payments are locked; review billing history.')}{props.onOpenHistory ? <Button variant='link' className='min-h-11' onClick={props.onOpenHistory}>{t('Billing History')}</Button> : <Link to='/wallet' search={{ show_history: true }} preload={false} onClick={() => props.onOpenChange(false)} className='text-primary mt-2 inline-flex min-h-11 items-center underline'>{t('Billing History')}</Link>}</AlertDescription></Alert>}
        <div className='flex flex-col gap-2 rounded-md border p-3'>
          <div role='group' aria-label={t('Required')} className='flex items-center justify-between gap-2 text-xs'>
            <span className='text-muted-foreground'>{t('Required')}</span>
            <span>{formatQuota(balanceCost)}</span>
          </div>
          <div className='flex items-center justify-between gap-2 text-xs'>
            <span className='text-muted-foreground'>{t('Available')}</span>
            <span>{balanceKnown ? formatQuota(userQuota) : t('Unknown')}</span>
          </div>
          {!allowBalancePay ? (
            <Alert variant='destructive'>
              <AlertDescription>
                {t('This plan does not allow balance redemption')}
              </AlertDescription>
            </Alert>
          ) : (
            balanceKnown && insufficientBalance && (
              <Alert variant='destructive'>
                <AlertDescription>{t('Insufficient balance')}</AlertDescription>
              </Alert>
            )
          )}

        </div>

      </div>
        </section>
        <DialogFooter role='group' aria-label={t('Actions')} className='m-0 shrink-0 flex-col gap-3 rounded-none px-4 py-4 sm:flex-col sm:px-6'>
          <div className='flex w-full flex-col gap-3 [&_[data-myapi-button]]:h-auto [&_[data-myapi-button]]:min-h-11 [&_[data-slot=select-trigger]]:min-h-11 [&_[role=combobox]]:min-h-11 [&_[data-myapi-button]]:whitespace-normal'>
          <div className='grid grid-cols-2 gap-2'>
          <Button
            variant='outline'
            onClick={handlePayBalance}
            disabled={
              paying || outcomeUnknown || limitReached || !allowBalancePay || !balanceKnown || insufficientBalance
            }
          >
            {t('Pay with Balance')}
          </Button>
            <Button className='min-h-11' variant='outline' disabled={paying} onClick={() => props.onOpenChange(false)}>{t('Cancel')}</Button>
          </div>
        {hasAnyPayment && (
          <div className='space-y-3'>
            <p className='text-muted-foreground text-xs'>
              {t('Select payment method')}
            </p>
            {(hasStripe || hasCreem || hasWaffoPancake) && (
              <div className='grid grid-cols-3 gap-2 sm:flex'>
                {hasStripe && (
                  <Button
                    variant='outline'
                    className='flex-1'
                    onClick={handlePayStripe}
                    disabled={paying || outcomeUnknown || limitReached}
                  >
                    Stripe
                  </Button>
                )}
                {hasCreem && (
                  <Button
                    variant='outline'
                    className='flex-1'
                    onClick={handlePayCreem}
                    disabled={paying || outcomeUnknown || limitReached}
                  >
                    Creem
                  </Button>
                )}
                {hasWaffoPancake && (
                  <Button
                    variant='outline'
                    className='flex-1'
                    onClick={handlePayWaffoPancake}
                    disabled={paying || outcomeUnknown || limitReached}
                  >
                    Waffo Pancake
                  </Button>
                )}
              </div>
            )}
            {hasEpay && (
              <div className='grid grid-cols-[minmax(0,1fr)_auto] gap-2'>
                <Select
                  items={(props.epayMethods || []).map((m) => ({
                    value: m.type,
                    label: m.name || m.type,
                  }))}
                  value={selectedEpayMethod}
                  onValueChange={(v) => v !== null && setSelectedEpayMethod(v)}
                  disabled={limitReached}
                >
                  <SelectTrigger aria-label={t('Select payment method')} className='h-auto min-h-11 flex-1 whitespace-normal'>
                    <SelectValue className='min-w-0 whitespace-normal break-words [overflow-wrap:anywhere]'>{selectedEpayMethodLabel}</SelectValue>
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false} className='[&_[data-slot=select-item]]:min-h-11'>
                    <SelectGroup>
                      {(props.epayMethods || []).map((m) => (
                        <SelectItem key={m.type} value={m.type}>
                          {m.name || m.type}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <Button
                  onClick={handlePayEpay}
                  disabled={paying || outcomeUnknown || !selectedEpayMethod || limitReached}
                >
                  {t('Pay')}
                </Button>
              </div>
            )}
          </div>
        )}


          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
